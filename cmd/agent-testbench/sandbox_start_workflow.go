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
	sandboxStartupCommandEmpty             = "startup command is empty"
	sandboxComposeServiceRunningReadiness  = "compose-service-running"
	sandboxComposeServiceStoppedReadiness  = "compose-service-not-running"
	sandboxDockerComposeLegacyCommandToken = "docker-compose"
)

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
			return runSandboxServiceComposeRecovery(ctx, service, timeout, started, composeService, result)
		}
		return result
	}
	return verifySandboxServiceStartupReadiness(ctx, service, timeout, started, composeService, result)
}

func runSandboxServiceComposeRecovery(ctx context.Context, service store.CatalogService, timeout time.Duration, started time.Time, composeService string, result sandboxStartServiceResult) sandboxStartServiceResult {
	recoveryCommand := "docker compose up -d " + composeService
	result.RecoveryCommand = recoveryCommand
	commandCtx, cancel := context.WithTimeout(ctx, sandboxStartRemainingTimeout(timeout, started))
	defer cancel()
	commandResult := runAgentObservedCommand(commandCtx, agentObservedCommandOptions{
		Command: []string{"/bin/sh", "-c", recoveryCommand},
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
	return verifySandboxServiceStartupReadiness(ctx, service, timeout, started, composeService, result)
}

func verifySandboxServiceStartupReadiness(ctx context.Context, service store.CatalogService, timeout time.Duration, started time.Time, composeService string, result sandboxStartServiceResult) sandboxStartServiceResult {
	if strings.TrimSpace(service.HealthURL) != "" {
		return verifySandboxServiceHealthURL(ctx, service.HealthURL, timeout, started, result)
	}
	if composeService == "" {
		result.Warning = "startup command completed; no healthUrl or compose service is recorded, so application readiness was not verified"
		return result
	}
	return verifySandboxComposeServiceRunning(ctx, composeService, timeout, started, result)
}

func verifySandboxServiceHealthURL(ctx context.Context, healthURL string, timeout time.Duration, started time.Time, result sandboxStartServiceResult) sandboxStartServiceResult {
	deadline := time.Now().Add(sandboxStartRemainingTimeout(timeout, started))
	client := &http.Client{Timeout: 2 * time.Second}
	var lastErr string
	for {
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

func verifySandboxComposeServiceRunning(ctx context.Context, composeService string, timeout time.Duration, started time.Time, result sandboxStartServiceResult) sandboxStartServiceResult {
	command := "docker compose ps -a --format json " + composeService
	commandCtx, cancel := context.WithTimeout(ctx, sandboxStartRemainingTimeout(timeout, started))
	defer cancel()
	commandResult := runAgentObservedCommand(commandCtx, agentObservedCommandOptions{
		Command: []string{"/bin/sh", "-c", command},
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
	detail := "state=" + firstNonEmpty(state, "unknown")
	if health != "" {
		detail += " health=" + health
	}
	if hasExitCode {
		detail += fmt.Sprintf(" exitCode=%d", exitCode)
	}
	result.Error = "compose service is not running after startup: " + detail + "; add healthUrl for application readiness"
	return result
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
		if field == "docker" && index+1 < len(fields) && fields[index+1] == "compose" {
			return true
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
