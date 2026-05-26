package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net/url"
	"os"
	"strings"
	"time"

	"agent-testbench/internal/server/controlplane"
	"agent-testbench/internal/store"
)

func runEnvironmentAcceptance(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return errors.New("missing environment acceptance command")
	}
	switch args[0] {
	case "start":
		return runEnvironmentAcceptanceStart(ctx, args[1:])
	case "report":
		return runEnvironmentAcceptanceReport(ctx, args[1:])
	default:
		return fmt.Errorf("unknown environment acceptance command: %s", args[0])
	}
}

func runEnvironmentAcceptanceStart(ctx context.Context, args []string) error {
	flags := flag.NewFlagSet("environment acceptance start", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	serverURL := flags.String("server-url", "", "Running control plane base URL")
	requestID := flags.String("request-id", "", "Acceptance request id")
	baseURL := flags.String("base-url", "", "Base URL for live request execution")
	evidenceDir := flags.String("evidence-dir", "", "Evidence output directory")
	timeoutSeconds := flags.Int("timeout-seconds", 0, "Per-step timeout in seconds")
	jsonOutput := flags.Bool("json", false, "Emit machine-readable JSON")
	if err := parseInterspersedFlags(flags, args); err != nil {
		return err
	}
	envID := strings.TrimSpace(flags.Arg(0))
	if envID == "" || strings.TrimSpace(*serverURL) == "" || strings.TrimSpace(*requestID) == "" {
		return errors.New("environment id, --server-url, and --request-id are required")
	}
	payload := map[string]any{"requestId": strings.TrimSpace(*requestID)}
	if strings.TrimSpace(*baseURL) != "" {
		payload["baseUrl"] = strings.TrimSpace(*baseURL)
	}
	if strings.TrimSpace(*evidenceDir) != "" {
		payload["evidenceDir"] = strings.TrimSpace(*evidenceDir)
	}
	if *timeoutSeconds > 0 {
		payload["timeoutSeconds"] = *timeoutSeconds
	}
	result, err := postWorkflowAcceptanceJSON(ctx, environmentAcceptanceRunURL(*serverURL, envID, ""), payload)
	if err != nil {
		return err
	}
	if *jsonOutput {
		return writeIndentedJSON(result)
	}
	printEnvironmentAcceptanceStart(result)
	return nil
}

func runEnvironmentAcceptanceReport(ctx context.Context, args []string) error {
	flags := flag.NewFlagSet("environment acceptance report", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	serverURL := flags.String("server-url", "", "Running control plane base URL")
	runID := flags.String("run", "", "Acceptance batch run id")
	jsonOutput := flags.Bool("json", false, "Emit machine-readable JSON")
	if err := parseInterspersedFlags(flags, args); err != nil {
		return err
	}
	envID := strings.TrimSpace(flags.Arg(0))
	if envID == "" || strings.TrimSpace(*serverURL) == "" || strings.TrimSpace(*runID) == "" {
		return errors.New("environment id, --server-url, and --run are required")
	}
	result, err := fetchWorkflowAcceptanceJSON(ctx, environmentAcceptanceRunURL(*serverURL, envID, *runID))
	if err != nil {
		return err
	}
	if *jsonOutput {
		return writeIndentedJSON(result)
	}
	printEnvironmentAcceptanceReport(result)
	return nil
}

func runEnvironmentVerify(ctx context.Context, args []string) error {
	flags := flag.NewFlagSet("environment verify", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	storeRef := flags.String("store", "", "Named Store config or Store DSN")
	storeURL := flags.String("store-url", "", legacyStoreURLFlagHelp)
	runID := flags.String("run", "", "Verification run id")
	status := flags.String("status", "", "Verification status")
	evidenceComplete := flags.Bool("evidence-complete", false, "Evidence is complete for the verification run")
	topologyComplete := flags.Bool("topology-complete", false, "SkyWalking topology is complete for the verification run")
	jsonOutput := flags.Bool("json", false, "Emit a machine-readable JSON report")
	if err := parseInterspersedFlags(flags, args); err != nil {
		return err
	}
	id := strings.TrimSpace(flags.Arg(0))
	if id == "" {
		return errors.New("environment id is required")
	}
	if strings.TrimSpace(*runID) == "" || strings.TrimSpace(*status) == "" {
		return errors.New("--run and --status are required")
	}
	runtime, cleanup, err := openRequiredCLIStore(ctx, *storeRef, *storeURL)
	if err != nil {
		return err
	}
	defer cleanup()
	env, err := runtime.GetEnvironment(ctx, id)
	if err != nil {
		return err
	}
	env.LastVerificationRunID = strings.TrimSpace(*runID)
	env.LastVerificationStatus = strings.TrimSpace(*status)
	env.EvidenceComplete = *evidenceComplete
	env.TopologyComplete = *topologyComplete
	env.Verified = false
	env.Status = "verification-recorded"
	if env.LastVerificationStatus == store.StatusPassed && env.EvidenceComplete && env.TopologyComplete {
		env.Status = "verified-ready"
		env.LastVerifiedAt = time.Now().UTC()
	}
	env, err = runtime.UpsertEnvironment(ctx, env)
	if err != nil {
		return err
	}
	return printEnvironmentCommandResult(env, *jsonOutput)
}

func runEnvironmentPublishVerified(ctx context.Context, args []string) error {
	options, err := parseEnvironmentIDFlags("environment publish-verified", args)
	if err != nil {
		return err
	}
	runtime, cleanup, err := openRequiredCLIStore(ctx, options.StoreRef, options.StoreURL)
	if err != nil {
		return err
	}
	defer cleanup()
	env, err := runtime.GetEnvironment(ctx, options.ID)
	if err != nil {
		return err
	}
	if err := controlplane.ValidateEnvironmentPublishable(ctx, runtime, env); err != nil {
		return err
	}
	env.Verified = true
	env.Status = "verified"
	if env.LastVerifiedAt.IsZero() {
		env.LastVerifiedAt = time.Now().UTC()
	}
	env, err = runtime.UpsertEnvironment(ctx, env)
	if err != nil {
		return err
	}
	return printEnvironmentCommandResult(env, options.JSONOutput)
}

func environmentAcceptanceRunURL(serverURL string, envID string, runID string) string {
	base := strings.TrimRight(strings.TrimSpace(serverURL), "/") + "/api/environments/" + url.PathEscape(strings.TrimSpace(envID)) + "/acceptance-runs"
	if strings.TrimSpace(runID) != "" {
		base += "/" + url.PathEscape(strings.TrimSpace(runID))
	}
	return base
}

func printEnvironmentAcceptanceStart(payload map[string]any) {
	fmt.Printf("Environment Acceptance Run: %s\n", valueString(payload["batchRunId"]))
	fmt.Printf("Environment: %s\n", valueString(payload["environmentId"]))
	fmt.Printf("Workflow: %s\n", valueString(payload["workflowId"]))
	fmt.Printf("Status: %s\n", valueString(payload["status"]))
	fmt.Printf("Report: %s\n", valueString(payload["reportUrl"]))
}

func printEnvironmentAcceptanceReport(payload map[string]any) {
	acceptance := mapFromReportAny(payload["acceptance"])
	health := mapFromReportAny(acceptance["healthSummary"])
	fmt.Printf("Environment Acceptance Report: %s\n", valueString(payload["batchRunId"]))
	fmt.Printf("Environment: %s\n", valueString(payload["environmentId"]))
	fmt.Printf("Workflow: %s\n", firstNonEmpty(valueString(acceptance["workflowId"]), valueString(payload["workflowId"])))
	fmt.Printf("Status: %s\n", valueString(payload["status"]))
	fmt.Printf("Accepted: %t\n", boolFromReportAny(acceptance["ok"]))
	fmt.Printf("Health: %d/%d\n", intFromReportAny(health["passed"]), intFromReportAny(health["total"]))
}
