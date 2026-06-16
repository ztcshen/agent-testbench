package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"agent-testbench/internal/store"
)

type environmentServiceRestartOptions struct {
	environmentLifecycleOptions
	Service       string
	HealthTimeout time.Duration
}

type environmentServiceRestartReport struct {
	OK          bool                                  `json:"ok"`
	Environment map[string]any                        `json:"environment"`
	Docker      environmentServiceRestartDockerReport `json:"docker"`
	Error       string                                `json:"error,omitempty"`
}

type environmentServiceRestartDockerReport struct {
	OK           bool                                  `json:"ok"`
	Action       string                                `json:"action"`
	Service      string                                `json:"service"`
	ComponentID  string                                `json:"componentId,omitempty"`
	Command      []string                              `json:"command,omitempty"`
	Output       string                                `json:"output,omitempty"`
	HealthChecks []environmentRestoreHealthCheckReport `json:"healthChecks,omitempty"`
	Error        string                                `json:"error,omitempty"`
}

func runEnvironmentService(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return errors.New("missing environment service command")
	}
	switch args[0] {
	case "restart":
		return runEnvironmentServiceRestart(ctx, args[1:])
	default:
		return fmt.Errorf("unknown environment service command: %s", args[0])
	}
}

func runEnvironmentServiceRestart(ctx context.Context, args []string) error {
	options, err := parseEnvironmentServiceRestartOptions(args)
	if err != nil {
		return err
	}
	runtime, env, graph, plan, cleanup, err := loadEnvironmentLifecyclePlan(ctx, options.environmentLifecycleOptions)
	if err != nil {
		return err
	}
	defer cleanup()
	report := environmentServiceRestartReport{OK: true}
	report.Docker = environmentServiceRestartDocker(ctx, plan, graph, options)
	if !report.Docker.OK {
		report.OK = false
		report.Error = report.Docker.Error
	}
	persisted, persistErr := persistEnvironmentServiceRestartSummary(ctx, runtime, env, report, plan.Workspace)
	if persistErr != nil {
		report.OK = false
		report.Error = persistErr.Error()
	} else {
		env = persisted
	}
	report.Environment = environmentPayload(env)
	if options.JSONOutput {
		if err := writeIndentedJSON(report); err != nil {
			return err
		}
	} else {
		printEnvironmentServiceRestartReport(report)
	}
	if !report.OK {
		return errors.New("environment service restart did not complete")
	}
	return nil
}

func parseEnvironmentServiceRestartOptions(args []string) (environmentServiceRestartOptions, error) {
	flags := flag.NewFlagSet("environment service restart", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	storeRef := flags.String("store", "", "Named Store config or Store DSN")
	storeURL := flags.String("store-url", "", legacyStoreURLFlagHelp)
	workspace := flags.String("workspace", "", "Local workspace for generated compose artifacts")
	service := flags.String("service", "", "Compose service or component id to restart")
	healthTimeoutSeconds := flags.Int("health-timeout-seconds", 120, "Seconds to wait for restarted service health")
	jsonOutput := flags.Bool("json", false, "Emit a machine-readable JSON report")
	if err := parseInterspersedFlags(flags, args); err != nil {
		return environmentServiceRestartOptions{}, err
	}
	id := strings.TrimSpace(flags.Arg(0))
	if id == "" {
		return environmentServiceRestartOptions{}, errors.New("environment id is required")
	}
	if strings.TrimSpace(*workspace) == "" {
		return environmentServiceRestartOptions{}, errors.New("--workspace is required")
	}
	if strings.TrimSpace(*service) == "" {
		return environmentServiceRestartOptions{}, errors.New("--service is required")
	}
	if *healthTimeoutSeconds <= 0 {
		return environmentServiceRestartOptions{}, errors.New("--health-timeout-seconds must be positive")
	}
	resolvedStoreURL, err := resolveRequiredDailyStoreReference(*storeRef, *storeURL)
	if err != nil {
		return environmentServiceRestartOptions{}, err
	}
	return environmentServiceRestartOptions{
		environmentLifecycleOptions: environmentLifecycleOptions{
			EnvironmentID: id,
			StoreRef:      *storeRef,
			StoreURL:      resolvedStoreURL,
			Workspace:     *workspace,
			JSONOutput:    *jsonOutput,
		},
		Service:       *service,
		HealthTimeout: time.Duration(*healthTimeoutSeconds) * time.Second,
	}, nil
}

func environmentServiceRestartDocker(ctx context.Context, plan environmentRestoreBuildPlan, graph store.EnvironmentComponentGraph, options environmentServiceRestartOptions) environmentServiceRestartDockerReport {
	statusReport := environmentStatusDockerReport{OK: true}
	if !prepareEnvironmentLifecycleComposeFiles(&statusReport, plan.Compose, plan.Workspace) {
		return environmentServiceRestartDockerReport{OK: false, Action: statusReport.Action, Error: statusReport.Error}
	}
	composeFiles := environmentRestoreComposeFiles(plan.Compose)
	if len(composeFiles) == 0 {
		return environmentServiceRestartDockerReport{OK: false, Action: "no-compose-plan", Error: "environment service restart requires a recorded composeFile"}
	}
	service, componentID, err := resolveEnvironmentServiceRestartTarget(plan.Compose, graph, plan.Workspace, options.Service)
	if err != nil {
		return environmentServiceRestartDockerReport{OK: false, Action: "resolve-service", Error: err.Error()}
	}
	composeBaseArgs := environmentRestoreComposeBaseArgs(plan.Compose, plan.Workspace, environmentRestoreResolvedComposeFiles(plan.Workspace, composeFiles))
	command := append(append([]string{"docker", "compose"}, composeBaseArgs...), "restart", service)
	output, errText := runRestoreCommand(ctx, plan.Workspace, command)
	report := environmentServiceRestartDockerReport{
		OK:          errText == "",
		Action:      "compose-service-restart",
		Service:     service,
		ComponentID: componentID,
		Command:     command,
		Output:      output,
	}
	if errText != "" {
		report.Error = errText
		return report
	}
	report.HealthChecks = waitEnvironmentRestoreHealthChecks(ctx, environmentServiceRestartHealthChecks(plan.HealthChecks, service), options.HealthTimeout, plan.Workspace, composeBaseArgs)
	for _, check := range report.HealthChecks {
		if !check.OK {
			report.OK = false
			report.Error = firstNonEmpty(check.Error, "service health check did not pass")
			break
		}
	}
	return report
}

func resolveEnvironmentServiceRestartTarget(compose map[string]any, graph store.EnvironmentComponentGraph, workspace string, requested string) (string, string, error) {
	requested = strings.TrimSpace(requested)
	composeServices := environmentRestoreStringSet(environmentLifecycleComposeServices(compose, workspace))
	if composeServices[requested] {
		return requested, "", nil
	}
	for _, component := range graph.Components {
		if strings.TrimSpace(component.ComponentID) != requested {
			continue
		}
		service := strings.TrimSpace(component.ComposeService)
		if service == "" {
			return "", component.ComponentID, fmt.Errorf("component %s has no composeService", requested)
		}
		if len(composeServices) > 0 && !composeServices[service] {
			return "", component.ComponentID, fmt.Errorf("component %s maps to compose service %s, but the service is not recorded in the compose plan", requested, service)
		}
		return service, component.ComponentID, nil
	}
	known := make([]string, 0, len(composeServices))
	for service := range composeServices {
		known = append(known, service)
	}
	sort.Strings(known)
	if len(known) == 0 {
		return "", "", fmt.Errorf("compose plan has no restartable services")
	}
	return "", "", fmt.Errorf("service %s is not recorded; known compose services: %s", requested, strings.Join(known, ", "))
}

func environmentServiceRestartHealthChecks(checks []any, service string) []any {
	out := []any{}
	for _, raw := range checks {
		check, ok := environmentRestoreHealthCheckFromAny(raw)
		if !ok {
			continue
		}
		if strings.TrimSpace(check.Service) == service {
			out = append(out, raw)
		}
	}
	if len(out) == 0 {
		out = append(out, map[string]any{"id": "compose-service-" + safeReportID(service), "kind": "compose-service", "service": service})
	}
	return out
}

func persistEnvironmentServiceRestartSummary(ctx context.Context, runtime store.Store, env store.Environment, report environmentServiceRestartReport, workspace string) (store.Environment, error) {
	summary := jsonObjectString(env.SummaryJSON)
	summary["lastServiceRestart"] = map[string]any{
		"attemptedAt":  time.Now().UTC().Format(time.RFC3339Nano),
		"ok":           report.Docker.OK,
		"action":       report.Docker.Action,
		"workspace":    workspace,
		"service":      report.Docker.Service,
		"componentId":  report.Docker.ComponentID,
		"command":      report.Docker.Command,
		"healthChecks": report.Docker.HealthChecks,
		"error":        report.Docker.Error,
	}
	env.SummaryJSON = mustCompactJSON(summary)
	env.UpdatedAt = time.Now().UTC()
	return runtime.UpsertEnvironment(ctx, env)
}

func printEnvironmentServiceRestartReport(report environmentServiceRestartReport) {
	fmt.Printf("Environment Service Restart: %s\n", valueString(report.Environment["id"]))
	fmt.Printf("OK: %t\n", report.OK)
	fmt.Printf("Service: %s\n", report.Docker.Service)
	fmt.Printf("Docker: %s ok=%t\n", report.Docker.Action, report.Docker.OK)
	if len(report.Docker.Command) > 0 {
		fmt.Printf("Command: %s\n", strings.Join(report.Docker.Command, " "))
	}
	for _, check := range report.Docker.HealthChecks {
		fmt.Printf("Health: %s ok=%t\n", environmentRestoreHealthProgressTarget(check), check.OK)
	}
	if report.Error != "" {
		fmt.Printf("Error: %s\n", report.Error)
	}
}
