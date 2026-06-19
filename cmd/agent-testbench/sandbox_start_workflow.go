package main

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"agent-testbench/internal/store"
)

const (
	sandboxStartupCommandEmpty              = "startup command is empty"
	sandboxComposeServiceRunningReadiness   = "compose-service-running"
	sandboxComposeServiceStoppedReadiness   = "compose-service-not-running"
	sandboxDockerDaemonUnavailableReadiness = "docker-daemon-unavailable"
	sandboxDockerComposeLegacyCommandToken  = "docker-compose"
	sandboxDockerInfoCommand                = "info"
	sandboxComposeProfileOption             = "--profile"
	sandboxDockerContextOption              = "--context"
	sandboxSudoCommandToken                 = "sudo"
)

type sandboxDockerCommandSpec struct {
	InfoCommand    []string
	ComposeCommand []string
}

func sandboxWorkflowRequiredServiceReasons(catalog store.ProfileCatalog, workflowID string) (map[string]string, error) {
	workflowID = strings.TrimSpace(workflowID)
	if workflowID == "" {
		return nil, nil
	}
	nodes := make(map[string]store.CatalogInterfaceNode, len(catalog.InterfaceNodes))
	for _, node := range catalog.InterfaceNodes {
		nodes[node.ID] = node
	}
	services := make(map[string]bool, len(catalog.Services))
	for _, service := range catalog.Services {
		services[service.ID] = true
	}
	reasons := map[string]string{}
	bindingCount := 0
	for _, binding := range catalog.WorkflowBindings {
		if binding.WorkflowID != workflowID {
			continue
		}
		if !binding.Required {
			continue
		}
		bindingCount++
		nodeID := strings.TrimSpace(binding.NodeID)
		node, ok := nodes[nodeID]
		if !ok {
			return nil, fmt.Errorf("workflow %s binding %s references missing interface node %s", workflowID, strings.TrimSpace(binding.StepID), nodeID)
		}
		serviceID := strings.TrimSpace(node.ServiceID)
		if serviceID == "" {
			return nil, fmt.Errorf("workflow %s binding %s interface node %s has no service id", workflowID, strings.TrimSpace(binding.StepID), nodeID)
		}
		if !services[serviceID] {
			return nil, fmt.Errorf("workflow %s binding %s references service %s that is not registered in the profile service registry", workflowID, strings.TrimSpace(binding.StepID), serviceID)
		}
		reasons[serviceID] = strings.TrimSpace("required by workflow " + workflowID + " step " + strings.TrimSpace(binding.StepID))
	}
	if bindingCount == 0 {
		return nil, fmt.Errorf("workflow has no required service bindings: %s", workflowID)
	}
	return reasons, nil
}

func sandboxRequiredStartupReason(serviceID string, serviceFilter string, workflowReason string) string {
	if strings.TrimSpace(workflowReason) != "" {
		return strings.TrimSpace(workflowReason)
	}
	if strings.TrimSpace(serviceFilter) != "" && strings.TrimSpace(serviceID) == strings.TrimSpace(serviceFilter) {
		return "explicitly requested service"
	}
	return ""
}

func runSandboxServiceStartup(ctx context.Context, service store.CatalogService, timeout time.Duration, dryRun bool, requiredReason string) sandboxStartServiceResult {
	command := strings.TrimSpace(service.StartupCommand)
	composeService := sandboxStartComposeService(service, command)
	dockerSpec := sandboxDockerCommandSpecFor(command)
	result := sandboxStartServiceResult{
		ID:             service.ID,
		DisplayName:    service.DisplayName,
		Kind:           service.Kind,
		ContainerName:  service.ContainerName,
		ServicePort:    service.ServicePort,
		ManagementPort: service.ManagementPort,
		Command:        command,
		ExitCode:       0,
	}
	if strings.TrimSpace(service.Status) != "" && strings.TrimSpace(service.Status) != "active" {
		result.Skipped = true
		result.SkipReason = "service is not active"
		return result
	}
	if command == "" {
		if strings.TrimSpace(requiredReason) != "" {
			result.ExitCode = 1
			result.Error = fmt.Sprintf("%s (%s); repair with: agent-testbench sandbox service register --id %s --startup-command '...'", sandboxStartupCommandEmpty, requiredReason, service.ID)
			return result
		}
		result.Skipped = true
		result.SkipReason = sandboxStartupCommandEmpty
		return result
	}
	if dryRun {
		result.Planned = true
		return result
	}
	if sandboxStartNeedsDockerPreflight(service, command) {
		if detail, unavailable := preflightSandboxDockerDaemon(ctx, timeout, command); unavailable {
			result.ExitCode = 1
			result.Readiness = sandboxDockerDaemonUnavailableReadiness
			result.Error = "environment-not-ready: Docker daemon unavailable before sandbox start: " + detail
			return result
		}
	}
	started := time.Now()
	commandCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	commandResult := runAgentObservedCommand(commandCtx, agentObservedCommandOptions{
		Command: []string{"/bin/sh", "-c", command},
	})
	result.Output = commandResult.Output
	if commandCtx.Err() == context.DeadlineExceeded {
		result.ExitCode = 124
		result.Error = "startup command timed out"
		return result
	}
	if commandResult.Err != nil {
		result.ExitCode = commandResult.ExitCode
		result.Error = commandResult.Err.Error()
		if composeService != "" && sandboxStartMissingContainer(commandResult.Output, result.Error) {
			return runSandboxServiceComposeRecovery(ctx, service, timeout, started, composeService, dockerSpec, result)
		}
		return result
	}
	return verifySandboxServiceStartupReadiness(ctx, service, timeout, started, composeService, dockerSpec, result)
}

func runSandboxServiceComposeRecovery(ctx context.Context, service store.CatalogService, timeout time.Duration, started time.Time, composeService string, dockerSpec sandboxDockerCommandSpec, result sandboxStartServiceResult) sandboxStartServiceResult {
	recoveryCommand := sandboxDockerComposeCommand(dockerSpec, "up", "-d", composeService)
	result.RecoveryCommand = strings.Join(recoveryCommand, " ")
	commandCtx, cancel := context.WithTimeout(ctx, sandboxStartRemainingTimeout(timeout, started))
	defer cancel()
	commandResult := runAgentObservedCommand(commandCtx, agentObservedCommandOptions{
		Command: recoveryCommand,
	})
	result.Output = strings.TrimSpace(strings.TrimSpace(result.Output) + "\n" + strings.TrimSpace(commandResult.Output))
	if commandCtx.Err() == context.DeadlineExceeded {
		result.ExitCode = 124
		result.Error = "compose recovery command timed out"
		return result
	}
	if commandResult.Err != nil {
		result.ExitCode = commandResult.ExitCode
		result.Error = commandResult.Err.Error()
		return result
	}
	result.ExitCode = 0
	result.Error = ""
	return verifySandboxServiceStartupReadiness(ctx, service, timeout, started, composeService, dockerSpec, result)
}

func verifySandboxServiceStartupReadiness(ctx context.Context, service store.CatalogService, timeout time.Duration, started time.Time, composeService string, dockerSpec sandboxDockerCommandSpec, result sandboxStartServiceResult) sandboxStartServiceResult {
	if strings.TrimSpace(service.HealthURL) != "" {
		return verifySandboxServiceHealthURL(ctx, service.HealthURL, timeout, started, composeService, dockerSpec, result)
	}
	if composeService == "" {
		result.Warning = "startup command completed; no healthUrl or compose service is recorded, so application readiness was not verified"
		return result
	}
	return verifySandboxComposeServiceRunning(ctx, composeService, timeout, started, dockerSpec, result)
}

func verifySandboxServiceHealthURL(ctx context.Context, healthURL string, timeout time.Duration, started time.Time, composeService string, dockerSpec sandboxDockerCommandSpec, result sandboxStartServiceResult) sandboxStartServiceResult {
	deadline := time.Now().Add(sandboxStartRemainingTimeout(timeout, started))
	client := &http.Client{Timeout: 2 * time.Second}
	var lastErr string
	for {
		if composeService != "" {
			if next, stopped := checkSandboxComposeServiceStopped(ctx, composeService, sandboxStartRemainingTimeout(timeout, started), dockerSpec, result); stopped {
				return next
			}
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimSpace(healthURL), nil)
		if err != nil {
			result.ExitCode = 1
			result.Error = err.Error()
			result.Readiness = "health-url-invalid"
			return result
		}
		resp, err := client.Do(req)
		if err == nil {
			statusCode := resp.StatusCode
			closeErr := resp.Body.Close()
			if closeErr != nil {
				lastErr = "healthUrl response body close failed: " + closeErr.Error()
			} else if statusCode >= 200 && statusCode < 300 {
				result.Readiness = "health-url-ready"
				return result
			} else {
				lastErr = fmt.Sprintf("healthUrl returned HTTP %d", statusCode)
			}
		} else {
			lastErr = err.Error()
		}
		if time.Now().After(deadline) {
			result.ExitCode = 1
			result.Error = "healthUrl did not become ready after startup: " + lastErr
			result.Readiness = "health-url-not-ready"
			return result
		}
		select {
		case <-ctx.Done():
			result.ExitCode = 1
			result.Error = ctx.Err().Error()
			result.Readiness = "health-url-not-ready"
			return result
		case <-time.After(250 * time.Millisecond):
		}
	}
}

func checkSandboxComposeServiceStopped(ctx context.Context, composeService string, timeout time.Duration, dockerSpec sandboxDockerCommandSpec, result sandboxStartServiceResult) (sandboxStartServiceResult, bool) {
	if timeout <= 0 {
		timeout = time.Nanosecond
	}
	if timeout > 2*time.Second {
		timeout = 2 * time.Second
	}
	commandCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	commandResult := runAgentObservedCommand(commandCtx, agentObservedCommandOptions{
		Command: sandboxDockerComposeCommand(dockerSpec, "ps", "-a", "--format", "json", composeService),
	})
	if commandCtx.Err() == context.DeadlineExceeded || commandResult.Err != nil {
		return result, false
	}
	state, health, exitCode, hasExitCode := parseComposeServiceHealth(commandResult.Output)
	if state == "" || state == "running" {
		return result, false
	}
	result.ExitCode = 1
	result.Readiness = sandboxComposeServiceStoppedReadiness
	result.Error = sandboxComposeServiceNotRunningError(state, health, exitCode, hasExitCode)
	return result, true
}

func verifySandboxComposeServiceRunning(ctx context.Context, composeService string, timeout time.Duration, started time.Time, dockerSpec sandboxDockerCommandSpec, result sandboxStartServiceResult) sandboxStartServiceResult {
	command := sandboxDockerComposeCommand(dockerSpec, "ps", "-a", "--format", "json", composeService)
	commandCtx, cancel := context.WithTimeout(ctx, sandboxStartRemainingTimeout(timeout, started))
	defer cancel()
	commandResult := runAgentObservedCommand(commandCtx, agentObservedCommandOptions{
		Command: command,
	})
	if commandCtx.Err() == context.DeadlineExceeded {
		result.ExitCode = 124
		result.Error = "compose service readiness check timed out"
		result.Readiness = sandboxComposeServiceStoppedReadiness
		return result
	}
	if commandResult.Err != nil {
		result.ExitCode = commandResult.ExitCode
		result.Error = "compose service readiness check failed: " + commandResult.Err.Error()
		result.Readiness = sandboxComposeServiceStoppedReadiness
		return result
	}
	state, health, exitCode, hasExitCode := parseComposeServiceHealth(commandResult.Output)
	if state == "running" && (health == "" || health == "healthy") {
		result.Readiness = sandboxComposeServiceRunningReadiness
		result.Warning = "service has no healthUrl; readiness verified only by Docker Compose running state"
		return result
	}
	result.ExitCode = 1
	result.Readiness = sandboxComposeServiceStoppedReadiness
	result.Error = sandboxComposeServiceNotRunningError(state, health, exitCode, hasExitCode)
	return result
}

func sandboxComposeServiceNotRunningError(state string, health string, exitCode int, hasExitCode bool) string {
	detail := "state=" + firstNonEmpty(state, "unknown")
	if health != "" {
		detail += " health=" + health
	}
	if hasExitCode {
		detail += fmt.Sprintf(" exitCode=%d", exitCode)
	}
	return "compose service is not running after startup: " + detail + "; add healthUrl for application readiness"
}

func sandboxStartRemainingTimeout(timeout time.Duration, started time.Time) time.Duration {
	remaining := timeout - time.Since(started)
	if remaining <= 0 {
		return time.Nanosecond
	}
	return remaining
}

func sandboxStartMissingContainer(output string, errText string) bool {
	lower := strings.ToLower(output + "\n" + errText)
	return strings.Contains(lower, "no such container")
}

func sandboxStartComposeService(service store.CatalogService, command string) string {
	if strings.TrimSpace(service.DockerService) != "" {
		return strings.TrimSpace(service.DockerService)
	}
	fields := strings.Fields(command)
	if len(fields) < 4 || !sandboxStartCommandUsesCompose(fields) {
		return ""
	}
	if !sandboxStartCommandHasToken(fields, "up") {
		return ""
	}
	for i := len(fields) - 1; i >= 0; i-- {
		field := strings.TrimSpace(fields[i])
		if field == "up" {
			break
		}
		if field == "" || strings.HasPrefix(field, "-") {
			continue
		}
		if field == "docker" || field == "compose" || field == sandboxDockerComposeLegacyCommandToken {
			continue
		}
		return field
	}
	return ""
}

func sandboxStartCommandUsesCompose(fields []string) bool {
	for index, field := range fields {
		if field == sandboxDockerComposeLegacyCommandToken {
			return true
		}
		if sandboxStartDockerBinaryToken(field) {
			commandIndex := sandboxDockerSubcommandIndex(fields, index+1)
			return commandIndex >= 0 && commandIndex < len(fields) && fields[commandIndex] == "compose"
		}
	}
	return false
}

func sandboxStartCommandHasToken(fields []string, token string) bool {
	for _, field := range fields {
		if field == token {
			return true
		}
	}
	return false
}

func sandboxStartNeedsDockerPreflight(service store.CatalogService, command string) bool {
	if strings.TrimSpace(service.DockerService) != "" || strings.TrimSpace(service.ContainerName) != "" {
		return true
	}
	for _, field := range strings.Fields(command) {
		if sandboxStartDockerCommandToken(field) {
			return true
		}
	}
	return false
}

func sandboxStartDockerCommandToken(field string) bool {
	field = strings.TrimSpace(field)
	return sandboxStartDockerBinaryToken(field) ||
		field == sandboxDockerComposeLegacyCommandToken ||
		strings.HasSuffix(field, "/"+sandboxDockerComposeLegacyCommandToken)
}

func sandboxStartDockerBinaryToken(field string) bool {
	field = strings.TrimSpace(field)
	return field == "docker" || strings.HasSuffix(field, "/docker")
}

func preflightSandboxDockerDaemon(ctx context.Context, timeout time.Duration, startupCommand string) (string, bool) {
	preflightTimeout := 10 * time.Second
	if timeout > 0 && timeout < preflightTimeout {
		preflightTimeout = timeout
	}
	dockerSpec := sandboxDockerCommandSpecFor(startupCommand)
	commandCtx, cancel := context.WithTimeout(ctx, preflightTimeout)
	defer cancel()
	commandResult := runAgentObservedCommand(commandCtx, agentObservedCommandOptions{
		Command: dockerSpec.InfoCommand,
	})
	if commandResult.Err == nil && commandCtx.Err() == nil {
		return "", false
	}
	detail := strings.TrimSpace(commandResult.Output)
	if detail == "" && commandCtx.Err() != nil {
		detail = commandCtx.Err().Error()
	}
	if detail == "" && commandResult.Err != nil {
		detail = commandResult.Err.Error()
	}
	return truncateReportText(detail, 240), true
}

func sandboxDockerPreflightCommand(startupCommand string) []string {
	return sandboxDockerCommandSpecFor(startupCommand).InfoCommand
}

func sandboxDockerCommandSpecFor(startupCommand string) sandboxDockerCommandSpec {
	fields := strings.Fields(startupCommand)
	if len(fields) == 0 {
		return sandboxDefaultDockerCommandSpec()
	}
	if spec, ok := sandboxDockerCommandSpecFromDockerCLI(fields); ok {
		return spec
	}
	if spec, ok := sandboxDockerCommandSpecFromLegacyCompose(fields); ok {
		return spec
	}
	if spec, ok := sandboxDockerCommandSpecFromComposeWrapper(fields); ok {
		return spec
	}
	return sandboxDefaultDockerCommandSpec()
}

func sandboxDefaultDockerCommandSpec() sandboxDockerCommandSpec {
	return sandboxDockerCommandSpec{
		InfoCommand:    []string{"docker", sandboxDockerInfoCommand},
		ComposeCommand: []string{"docker", "compose"},
	}
}

func sandboxDockerCommandSpecFromDockerCLI(fields []string) (sandboxDockerCommandSpec, bool) {
	for index, field := range fields {
		if !sandboxStartDockerBinaryToken(field) {
			continue
		}
		commandIndex := sandboxDockerSubcommandIndex(fields, index+1)
		if commandIndex < 0 {
			commandIndex = len(fields)
		}
		commandPrefix := sandboxDockerExecutablePrefix(fields, index)
		infoCommand := append([]string{}, commandPrefix...)
		infoCommand = append(infoCommand, fields[index+1:commandIndex]...)
		infoCommand = append(infoCommand, sandboxDockerInfoCommand)
		composeCommand := []string{"docker", "compose"}
		if commandIndex < len(fields) && fields[commandIndex] == "compose" {
			composeEnd := sandboxComposeCommandBaseEnd(fields, commandIndex+1)
			composeCommand = append([]string{}, commandPrefix...)
			composeCommand = append(composeCommand, fields[index+1:composeEnd]...)
		}
		return sandboxDockerCommandSpec{InfoCommand: infoCommand, ComposeCommand: composeCommand}, true
	}
	return sandboxDockerCommandSpec{}, false
}

func sandboxDockerCommandSpecFromLegacyCompose(fields []string) (sandboxDockerCommandSpec, bool) {
	for index, field := range fields {
		field = strings.TrimSpace(field)
		if field != sandboxDockerComposeLegacyCommandToken && !strings.HasSuffix(field, "/"+sandboxDockerComposeLegacyCommandToken) {
			continue
		}
		composeEnd := sandboxComposeCommandBaseEnd(fields, index+1)
		commandPrefix := sandboxDockerExecutablePrefix(fields, index)
		composeCommand := append([]string{}, commandPrefix...)
		composeCommand = append(composeCommand, fields[index+1:composeEnd]...)
		return sandboxDockerCommandSpec{
			InfoCommand:    []string{"docker", sandboxDockerInfoCommand},
			ComposeCommand: composeCommand,
		}, true
	}
	return sandboxDockerCommandSpec{}, false
}

func sandboxDockerExecutablePrefix(fields []string, index int) []string {
	if index > 0 && len(fields) > index && fields[0] == sandboxSudoCommandToken {
		return append([]string{}, fields[:index+1]...)
	}
	return []string{fields[index]}
}

func sandboxDockerCommandSpecFromComposeWrapper(fields []string) (sandboxDockerCommandSpec, bool) {
	if len(fields) < 2 || strings.TrimSpace(fields[0]) == "" || fields[1] != "compose" {
		return sandboxDockerCommandSpec{}, false
	}
	composeEnd := sandboxComposeCommandBaseEnd(fields, 2)
	return sandboxDockerCommandSpec{
		InfoCommand:    []string{fields[0], sandboxDockerInfoCommand},
		ComposeCommand: append([]string{}, fields[:composeEnd]...),
	}, true
}

func sandboxDockerSubcommandIndex(fields []string, start int) int {
	for index := start; index < len(fields); index++ {
		field := strings.TrimSpace(fields[index])
		if field == "" {
			continue
		}
		if !strings.HasPrefix(field, "-") {
			return index
		}
		if sandboxDockerOptionTakesValue(field) && !strings.Contains(field, "=") && index+1 < len(fields) {
			index++
		}
	}
	return -1
}

func sandboxDockerOptionTakesValue(field string) bool {
	switch strings.TrimSpace(field) {
	case "--config", sandboxDockerContextOption, "--host", "-H", "--log-level", "--tlscacert", "--tlscert", "--tlskey":
		return true
	default:
		return false
	}
}

func sandboxComposeCommandBaseEnd(fields []string, start int) int {
	for index := start; index < len(fields); index++ {
		field := strings.TrimSpace(fields[index])
		if field == "" {
			continue
		}
		if !strings.HasPrefix(field, "-") {
			return index
		}
		if sandboxComposeOptionTakesValue(field) && !strings.Contains(field, "=") && index+1 < len(fields) {
			index++
		}
	}
	return len(fields)
}

func sandboxComposeOptionTakesValue(field string) bool {
	switch strings.TrimSpace(field) {
	case "--ansi", "--env-file", "--file", "-f", "--parallel", sandboxComposeProfileOption, "--progress", "--project-directory", "--project-name", "-p":
		return true
	default:
		return false
	}
}

func sandboxDockerComposeCommand(spec sandboxDockerCommandSpec, args ...string) []string {
	base := spec.ComposeCommand
	if len(base) == 0 {
		base = sandboxDefaultDockerCommandSpec().ComposeCommand
	}
	command := append([]string{}, base...)
	command = append(command, args...)
	return command
}
