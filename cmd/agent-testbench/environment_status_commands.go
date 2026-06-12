package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
)

type environmentStatusReport struct {
	OK                   bool                             `json:"ok"`
	Environment          map[string]any                   `json:"environment"`
	VerificationWorkflow string                           `json:"verificationWorkflow,omitempty"`
	Docker               environmentStatusDockerReport    `json:"docker"`
	ComponentGraph       environmentRestoreComponentGraph `json:"componentGraph,omitempty"`
	Error                string                           `json:"error,omitempty"`
}

type environmentStatusDockerReport struct {
	OK          bool                             `json:"ok"`
	Action      string                           `json:"action"`
	ComposeFile string                           `json:"composeFile,omitempty"`
	Summary     environmentStatusHealthSummary   `json:"summary"`
	Services    []environmentStatusServiceReport `json:"services,omitempty"`
	Error       string                           `json:"error,omitempty"`
}

type environmentStatusHealthSummary struct {
	Total   int `json:"total"`
	Running int `json:"running"`
	Healthy int `json:"healthy"`
	Ready   int `json:"ready"`
	Failed  int `json:"failed"`
	Unknown int `json:"unknown,omitempty"`
}

type environmentStatusServiceReport struct {
	Service   string `json:"service"`
	Container string `json:"container,omitempty"`
	State     string `json:"state,omitempty"`
	Health    string `json:"health,omitempty"`
	ExitCode  int    `json:"exitCode,omitempty"`
	OK        bool   `json:"ok"`
	Error     string `json:"error,omitempty"`
}

func runEnvironmentStatus(ctx context.Context, args []string) error {
	options, err := parseEnvironmentLifecycleOptions("environment status", args)
	if err != nil {
		return err
	}
	runtime, env, graph, plan, cleanup, err := loadEnvironmentLifecyclePlan(ctx, options)
	if err != nil {
		return err
	}
	defer cleanup()
	_ = runtime
	report := environmentStatusReport{
		OK:                   true,
		Environment:          environmentPayload(env),
		VerificationWorkflow: plan.WorkflowID,
		ComponentGraph:       environmentRestoreComponentGraphReport(env.ID, graph),
	}
	healthChecks := environmentRestoreEffectiveHealthChecks(jsonArrayString(env.HealthChecksJSON), plan.Compose, graph, plan.Workspace)
	report.Docker = environmentStatusDocker(ctx, plan.Compose, plan.Workspace, healthChecks)
	if !report.Docker.OK {
		report.OK = false
		report.Error = report.Docker.Error
	}
	if options.JSONOutput {
		if err := writeIndentedJSON(report); err != nil {
			return err
		}
	} else {
		printEnvironmentStatusReport(report)
	}
	if !report.OK {
		return errors.New("environment status did not pass")
	}
	return nil
}

func environmentStatusDocker(ctx context.Context, compose map[string]any, workspace string, healthChecks []any) environmentStatusDockerReport {
	report := environmentStatusDockerReport{OK: true, Action: "inspect-compose-services", ComposeFile: strings.Join(environmentRestoreResolvedComposeFiles(workspace, environmentRestoreComposeFiles(compose)), ",")}
	composeBaseArgs := environmentRestoreComposeBaseArgs(compose, workspace, environmentRestoreResolvedComposeFiles(workspace, environmentRestoreComposeFiles(compose)))
	if len(composeBaseArgs) == 0 {
		report.Action = "no-compose-plan"
		report.Error = "environment status requires a recorded composeFile"
		report.OK = false
		return report
	}
	if !prepareEnvironmentLifecycleComposeFiles(&report, compose, workspace) {
		return report
	}
	services := environmentLifecycleComposeServices(compose, workspace)
	for _, item := range inspectEnvironmentComposeServices(ctx, services, workspace, composeBaseArgs, healthChecks) {
		report.Services = append(report.Services, environmentStatusServiceFromHealth(item))
		if !item.OK {
			report.OK = false
			if report.Error == "" {
				report.Error = environmentRestoreHealthFailureError(item)
			}
		}
	}
	if len(report.Services) == 0 {
		report.OK = false
		report.Error = "environment status found no compose services; record compose services or provide a compose file with services"
	}
	report.Summary = environmentStatusSummarizeServices(report.Services)
	return report
}

func inspectEnvironmentComposeServices(ctx context.Context, services []string, workspace string, composeBaseArgs []string, healthChecks []any) []environmentRestoreHealthCheckReport {
	command := append(append([]string{"docker", "compose"}, composeBaseArgs...), "ps", "-a", "--format", "json")
	command = append(command, services...)
	output, errText := runRestoreCommand(ctx, workspace, command)
	out := make([]environmentRestoreHealthCheckReport, 0, len(services))
	expectations := environmentStatusComposeServiceExpectations(healthChecks)
	if errText != "" {
		if len(services) == 0 {
			return []environmentRestoreHealthCheckReport{{
				Kind:    "compose-service",
				Service: "docker compose ps",
				Output:  truncateReportText(output, 200),
				Error:   errText,
			}}
		}
		for _, service := range services {
			out = append(out, environmentRestoreHealthCheckReport{
				Kind:    "compose-service",
				Service: service,
				Output:  truncateReportText(output, 200),
				Error:   errText,
			})
		}
		return out
	}
	observed := parseComposeServiceStatusReports(output)
	if len(services) == 0 {
		services = make([]string, 0, len(observed))
		for service := range observed {
			services = append(services, service)
		}
		sort.Strings(services)
	}
	for _, service := range services {
		check, ok := observed[service]
		if !ok {
			check = environmentRestoreHealthCheckReport{
				Kind:    "compose-service",
				Service: service,
				Output:  truncateReportText(output, 200),
				Error:   "compose service was not returned by docker compose ps",
			}
			out = append(out, check)
			continue
		}
		check.Output = truncateReportText(output, 200)
		check = environmentStatusApplyComposeServiceExpectation(check, expectations[service])
		out = append(out, check)
	}
	return out
}

func environmentStatusComposeServiceExpectations(healthChecks []any) map[string]environmentRestoreHealthCheckReport {
	out := map[string]environmentRestoreHealthCheckReport{}
	for _, raw := range healthChecks {
		check, ok := environmentRestoreHealthCheckFromAny(raw)
		if !ok || check.Kind != "compose-service" || strings.TrimSpace(check.Service) == "" {
			continue
		}
		out[check.Service] = check
	}
	return out
}

func environmentStatusApplyComposeServiceExpectation(check environmentRestoreHealthCheckReport, expected environmentRestoreHealthCheckReport) environmentRestoreHealthCheckReport {
	if strings.TrimSpace(expected.Service) == "" {
		return check
	}
	check.Expect = expected.Expect
	check.OneShot = expected.OneShot
	check.OK = check.State == "running" && (check.Health == "" || check.Health == "healthy") || environmentRestoreExitedCompleted(&check, check.State, check.ExitCode, check.ExitCode != 0 || check.State == environmentRestoreDockerStateExited)
	if !check.OK && check.Error == "" {
		check.Error = "compose service is not ready"
	}
	if check.OK {
		check.Error = ""
	}
	return check
}

func parseComposeServiceStatusReports(output string) map[string]environmentRestoreHealthCheckReport {
	out := map[string]environmentRestoreHealthCheckReport{}
	output = strings.TrimSpace(output)
	if output == "" {
		return out
	}
	var array []map[string]any
	if err := json.Unmarshal([]byte(output), &array); err == nil {
		for _, object := range array {
			addComposeServiceStatusReport(out, object)
		}
		return out
	}
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var object map[string]any
		if err := json.Unmarshal([]byte(line), &object); err == nil {
			addComposeServiceStatusReport(out, object)
		}
	}
	if len(out) > 0 {
		return out
	}
	var object map[string]any
	if err := json.Unmarshal([]byte(output), &object); err == nil {
		addComposeServiceStatusReport(out, object)
	}
	return out
}

func addComposeServiceStatusReport(out map[string]environmentRestoreHealthCheckReport, object map[string]any) {
	service := strings.TrimSpace(valueString(firstNonNil(object["Service"], object["service"])))
	if service == "" {
		service = strings.TrimSpace(valueString(firstNonNil(object["Name"], object["name"])))
	}
	if service == "" {
		return
	}
	state, health, exitCode, hasExitCode := composeServiceHealthFromObject(object)
	check := environmentRestoreHealthCheckReport{
		Kind:      "compose-service",
		Service:   service,
		Container: strings.TrimSpace(valueString(firstNonNil(object["Name"], object["name"], object["Container"], object["container"]))),
		State:     state,
		Health:    health,
	}
	if hasExitCode {
		check.ExitCode = exitCode
	}
	check.OK = state == "running" && (health == "" || health == "healthy") || environmentRestoreExitedCompleted(&check, state, exitCode, hasExitCode)
	if !check.OK {
		check.Error = "compose service is not ready"
	}
	out[service] = check
}

func environmentStatusServiceFromHealth(check environmentRestoreHealthCheckReport) environmentStatusServiceReport {
	return environmentStatusServiceReport{
		Service:   check.Service,
		Container: check.Container,
		State:     check.State,
		Health:    check.Health,
		ExitCode:  check.ExitCode,
		OK:        check.OK,
		Error:     check.Error,
	}
}

func environmentStatusSummarizeServices(services []environmentStatusServiceReport) environmentStatusHealthSummary {
	summary := environmentStatusHealthSummary{Total: len(services)}
	for _, service := range services {
		switch strings.ToLower(strings.TrimSpace(service.State)) {
		case "running":
			summary.Running++
		case "":
			summary.Unknown++
		}
		switch strings.ToLower(strings.TrimSpace(service.Health)) {
		case "healthy":
			summary.Healthy++
		}
		if service.OK {
			summary.Ready++
		} else {
			summary.Failed++
		}
	}
	return summary
}

func printEnvironmentStatusReport(report environmentStatusReport) {
	fmt.Printf("Environment Status: %s\n", valueString(report.Environment["id"]))
	fmt.Printf("OK: %t\n", report.OK)
	fmt.Printf("Verification Workflow: %s\n", report.VerificationWorkflow)
	fmt.Printf("Docker: %s ok=%t\n", report.Docker.Action, report.Docker.OK)
	for _, service := range report.Docker.Services {
		fmt.Printf("- %s state=%s health=%s ok=%t\n", service.Service, service.State, service.Health, service.OK)
		if service.Error != "" {
			fmt.Printf("  error: %s\n", service.Error)
		}
	}
	if report.Error != "" {
		fmt.Printf("Error: %s\n", report.Error)
	}
}
