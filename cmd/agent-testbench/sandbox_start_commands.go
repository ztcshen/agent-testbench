package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"agent-testbench/internal/store"
)

type sandboxStartReport struct {
	OK         bool                        `json:"ok"`
	DryRun     bool                        `json:"dryRun,omitempty"`
	WorkflowID string                      `json:"workflowId,omitempty"`
	StorePath  string                      `json:"storePath"`
	Runtime    statusRuntimeReport         `json:"runtime"`
	Services   []sandboxStartServiceResult `json:"services"`
	Counts     sandboxStartReportCounts    `json:"counts"`
	Error      string                      `json:"error,omitempty"`
}

type sandboxStartReportCounts struct {
	Total   int `json:"total"`
	Started int `json:"started"`
	Planned int `json:"planned,omitempty"`
	Skipped int `json:"skipped"`
	Failed  int `json:"failed"`
}

type sandboxStartServiceResult struct {
	ID              string `json:"id"`
	DisplayName     string `json:"displayName"`
	Kind            string `json:"kind"`
	ContainerName   string `json:"containerName,omitempty"`
	ServicePort     int    `json:"servicePort,omitempty"`
	ManagementPort  int    `json:"managementPort,omitempty"`
	Command         string `json:"command,omitempty"`
	RecoveryCommand string `json:"recoveryCommand,omitempty"`
	Readiness       string `json:"readiness,omitempty"`
	Skipped         bool   `json:"skipped"`
	Planned         bool   `json:"planned,omitempty"`
	SkipReason      string `json:"skipReason,omitempty"`
	ExitCode        int    `json:"exitCode"`
	Output          string `json:"output,omitempty"`
	Warning         string `json:"warning,omitempty"`
	Error           string `json:"error,omitempty"`
}

type sandboxStartFilters struct {
	ServiceID  string
	WorkflowID string
	Kind       string
}

func runSandboxStart(ctx context.Context, args []string) error {
	flags := flag.NewFlagSet("sandbox start", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	storeRef := flags.String("store", "", "Named Store config or Store DSN")
	storeURL := flags.String("store-url", "", legacyStoreURLFlagHelp)
	serviceID := flags.String("service", "", "Only start one registered service")
	workflowID := flags.String("workflow", "", "Start only services required by a workflow")
	serviceKind := flags.String("kind", "", "Only start services of this kind; default includes all kinds")
	timeoutSeconds := flags.Int("timeout-seconds", 300, "Per-service startup command timeout")
	dryRun := flags.Bool("dry-run", false, "Plan service startup without running startup commands")
	outputFormat := flags.String("output-format", "", "Output format: text, json, or stream-json")
	jsonOutput := flags.Bool("json", false, "Emit a machine-readable JSON report")
	if err := flags.Parse(args); err != nil {
		return err
	}
	resolvedOutputFormat, err := resolveCLIOutputFormat(*outputFormat, *jsonOutput)
	if err != nil {
		return err
	}
	if *timeoutSeconds <= 0 {
		return errors.New("--timeout-seconds must be greater than 0")
	}
	if strings.TrimSpace(*workflowID) != "" && strings.TrimSpace(*serviceID) != "" {
		return errors.New("--workflow cannot be combined with --service")
	}
	if strings.TrimSpace(*workflowID) != "" && strings.TrimSpace(*serviceKind) != "" {
		return errors.New("--workflow cannot be combined with --kind")
	}
	resolvedStoreURL, err := resolveRequiredDailyStoreReference(*storeRef, *storeURL)
	if err != nil {
		return err
	}
	if resolvedOutputFormat == cliOutputFormatStreamJSON {
		ctx = contextWithAgentEventStream(ctx, os.Stdout)
	}
	report := sandboxStartReport{
		OK:         true,
		DryRun:     *dryRun,
		WorkflowID: strings.TrimSpace(*workflowID),
		StorePath:  maskStoreURL(resolvedStoreURL),
		Runtime:    sandboxStartRuntimeReport(ctx),
	}
	agentEmitRunStarted(ctx, newSandboxStartRunID(), "sandbox.start", sandboxStartTarget(report), "sandbox start started")
	runtime, err := openStore(ctx, resolvedStoreURL)
	if err != nil {
		emitSandboxStartStreamFailure(ctx, report, err)
		return err
	}
	defer closeCLIStore(runtime)
	if err := validateSandboxStartWorkflowNotEnvironmentBound(ctx, runtime, report.WorkflowID); err != nil {
		report.OK = false
		report.Error = err.Error()
		emitSandboxStartFailure(ctx, report, resolvedOutputFormat)
		return err
	}
	catalog, err := runtime.GetProfileCatalog(ctx)
	if err != nil {
		emitSandboxStartStreamFailure(ctx, report, err)
		return err
	}
	workflowRequired, err := sandboxWorkflowRequiredServiceReasons(catalog, report.WorkflowID)
	if err != nil {
		emitSandboxStartStreamFailure(ctx, report, err)
		return err
	}
	filters := sandboxStartFilters{
		ServiceID:  strings.TrimSpace(*serviceID),
		WorkflowID: report.WorkflowID,
		Kind:       strings.TrimSpace(*serviceKind),
	}
	startSandboxServices(ctx, &report, catalog.Services, workflowRequired, filters, time.Duration(*timeoutSeconds)*time.Second, *dryRun)
	if err := validateSandboxStartSelection(report, filters); err != nil {
		report.OK = false
		report.Error = err.Error()
		emitSandboxStartFailure(ctx, report, resolvedOutputFormat)
		return err
	}
	switch resolvedOutputFormat {
	case cliOutputFormatStreamJSON:
		agentEmitRunCompleted(ctx, "sandbox.start", sandboxStartStatus(report), sandboxStartTarget(report), "sandbox start completed", sandboxStartError(report), report)
	case cliOutputFormatJSON:
		if err := writeIndentedJSON(report); err != nil {
			return err
		}
	default:
		printSandboxStartReport(report)
	}
	if !report.OK {
		return errors.New("one or more sandbox services failed to start")
	}
	return nil
}

func emitSandboxStartFailure(ctx context.Context, report sandboxStartReport, outputFormat string) {
	switch outputFormat {
	case cliOutputFormatStreamJSON:
		agentEmitRunCompleted(ctx, "sandbox.start", sandboxStartStatus(report), sandboxStartTarget(report), "sandbox start failed", sandboxStartError(report), report)
	case cliOutputFormatJSON:
		if err := writeIndentedJSON(report); err != nil {
			fmt.Fprintf(os.Stderr, "warning: write sandbox start failure report: %v\n", err)
		}
	}
}

func sandboxStartRuntimeReport(_ context.Context) statusRuntimeReport {
	statusCtx := context.Background()
	return statusRuntime(statusCtx, statusRepo(statusCtx))
}

func validateSandboxStartWorkflowNotEnvironmentBound(ctx context.Context, runtime store.Store, workflowID string) error {
	workflowID = strings.TrimSpace(workflowID)
	if workflowID == "" {
		return nil
	}
	environments, err := runtime.ListEnvironments(ctx)
	if err != nil {
		return err
	}
	ids := []string{}
	for _, env := range environments {
		if strings.TrimSpace(env.VerificationWorkflowID) == workflowID {
			ids = append(ids, strings.TrimSpace(env.ID))
		}
	}
	if len(ids) == 0 {
		return nil
	}
	return fmt.Errorf("workflow %s is bound to environment %s; start and verify the environment with the same Store: agent-testbench environment restore %s --store STORE_NAME_OR_DSN --workspace WORKSPACE --execute --run-workflow --server-url URL", workflowID, strings.Join(ids, ", "), ids[0])
}

func emitSandboxStartStreamFailure(ctx context.Context, report sandboxStartReport, err error) {
	if !agentHasEventStream(ctx) {
		return
	}
	report.OK = false
	agentEmitRunCompleted(ctx, "sandbox.start", "failed", sandboxStartTarget(report), "sandbox start failed", err.Error(), report)
}

func startSandboxServices(ctx context.Context, report *sandboxStartReport, services []store.CatalogService, workflowRequired map[string]string, filters sandboxStartFilters, timeout time.Duration, dryRun bool) {
	for _, service := range services {
		if !sandboxStartServiceMatches(service, workflowRequired[service.ID], filters) {
			continue
		}
		requiredReason := sandboxRequiredStartupReason(service.ID, filters.ServiceID, workflowRequired[service.ID])
		agentEmitStep(ctx, "step_started", "sandbox.service", "running", service.ID, "service startup started", "")
		result := runSandboxServiceStartup(ctx, service, timeout, dryRun, requiredReason)
		agentEmitStep(ctx, "step_completed", "sandbox.service", sandboxStartServiceStatus(result), service.ID, sandboxStartServiceMessage(result), result.Error)
		addSandboxStartResult(report, result)
	}
}

func sandboxStartServiceMatches(service store.CatalogService, workflowReason string, filters sandboxStartFilters) bool {
	if filters.ServiceID != "" && service.ID != filters.ServiceID {
		return false
	}
	if filters.WorkflowID != "" && workflowReason == "" {
		return false
	}
	return filters.Kind == "" || strings.TrimSpace(service.Kind) == filters.Kind
}

func addSandboxStartResult(report *sandboxStartReport, result sandboxStartServiceResult) {
	report.Services = append(report.Services, result)
	report.Counts.Total++
	switch {
	case result.Planned:
		report.Counts.Planned++
	case result.Skipped:
		report.Counts.Skipped++
	case result.ExitCode == 0:
		report.Counts.Started++
	default:
		report.Counts.Failed++
		report.OK = false
	}
}

func validateSandboxStartSelection(report sandboxStartReport, filters sandboxStartFilters) error {
	if filters.ServiceID != "" && report.Counts.Total == 0 {
		return fmt.Errorf("registered service not found in profile service registry: %s (sandbox start does not read the environment component graph; use environment restore for component-graph Docker startup or register the service with sandbox service register)", filters.ServiceID)
	}
	if filters.WorkflowID != "" && report.Counts.Total == 0 {
		return fmt.Errorf("workflow has no registered startable services: %s", filters.WorkflowID)
	}
	return nil
}

func newSandboxStartRunID() string {
	return "sandbox.start." + time.Now().UTC().Format("20060102T150405.000000000Z")
}

func sandboxStartTarget(report sandboxStartReport) string {
	if strings.TrimSpace(report.WorkflowID) != "" {
		return report.WorkflowID
	}
	return "profile-service-registry"
}

func sandboxStartStatus(report sandboxStartReport) string {
	if report.OK {
		return "passed"
	}
	return "failed"
}

func sandboxStartError(report sandboxStartReport) string {
	if report.OK {
		return ""
	}
	if strings.TrimSpace(report.Error) != "" {
		return report.Error
	}
	return "one or more sandbox services failed to start"
}

func sandboxStartServiceStatus(result sandboxStartServiceResult) string {
	switch {
	case result.Planned:
		return "planned"
	case result.Skipped:
		return "skipped"
	case result.ExitCode == 0:
		return "passed"
	default:
		return "failed"
	}
}

func sandboxStartServiceMessage(result sandboxStartServiceResult) string {
	switch sandboxStartServiceStatus(result) {
	case "planned":
		return "service startup planned"
	case "skipped":
		return result.SkipReason
	case "failed":
		return "service startup failed"
	default:
		return "service startup completed"
	}
}

func printSandboxStartReport(report sandboxStartReport) {
	fmt.Println("Sandbox Start")
	fmt.Printf("OK: %t\n", report.OK)
	fmt.Printf("Store: %s\n", report.StorePath)
	if report.Error != "" {
		fmt.Printf("Error: %s\n", report.Error)
	}
	if report.DryRun {
		fmt.Println("Mode: dry-run")
		fmt.Printf("Total: %d Planned: %d Skipped: %d Failed: %d\n", report.Counts.Total, report.Counts.Planned, report.Counts.Skipped, report.Counts.Failed)
	} else {
		fmt.Printf("Total: %d Started: %d Skipped: %d Failed: %d\n", report.Counts.Total, report.Counts.Started, report.Counts.Skipped, report.Counts.Failed)
	}
	for _, service := range report.Services {
		state := "started"
		if service.Planned {
			state = "planned"
		}
		if service.Skipped {
			state = "skipped"
		}
		if service.ExitCode != 0 {
			state = "failed"
		}
		fmt.Printf("- %s [%s]\n", service.ID, state)
		if service.Command != "" {
			fmt.Printf("  command: %s\n", service.Command)
		}
		if service.SkipReason != "" {
			fmt.Printf("  reason: %s\n", service.SkipReason)
		}
		if service.Error != "" {
			fmt.Printf("  error: %s\n", service.Error)
		}
		if service.Output != "" {
			fmt.Printf("  output: %s\n", service.Output)
		}
	}
}
