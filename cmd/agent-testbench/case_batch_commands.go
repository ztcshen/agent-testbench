package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
)

func runCaseBatchStart(ctx context.Context, args []string) error {
	flags := flag.NewFlagSet("case batch start", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	serverURL := flags.String("server-url", "", "Running control plane base URL")
	workflowID := flags.String("workflow", "", "Workflow id")
	suite := flags.String("suite", "", "Suite selector")
	requestID := flags.String("request-id", "", "Batch request id")
	baseURL := flags.String("base-url", "", "Base URL for live request execution")
	evidenceDir := flags.String("evidence-dir", "", "Evidence output directory")
	timeoutSeconds := flags.Int("timeout-seconds", 0, "Per-case timeout in seconds")
	jsonOutput := flags.Bool("json", false, "Emit machine-readable JSON")
	var caseIDs, nodeIDs stringListFlag
	flags.Var(&caseIDs, "case", "Case id; repeat for multiple cases")
	flags.Var(&nodeIDs, "node", "Interface node id; repeat for multiple nodes")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if strings.TrimSpace(*serverURL) == "" {
		return errors.New("--server-url is required")
	}
	payload := map[string]any{}
	if values := caseIDs.Values(); len(values) > 0 {
		payload["caseIds"] = values
	}
	if values := nodeIDs.Values(); len(values) > 0 {
		payload["nodeIds"] = values
	}
	if strings.TrimSpace(*workflowID) != "" {
		payload["workflowId"] = strings.TrimSpace(*workflowID)
	}
	if strings.TrimSpace(*suite) != "" {
		payload["suite"] = strings.TrimSpace(*suite)
	}
	if len(payload) == 0 {
		return errors.New("at least one of --case, --node, --workflow, or --suite is required")
	}
	if strings.TrimSpace(*requestID) != "" {
		payload["requestId"] = strings.TrimSpace(*requestID)
	}
	addWorkflowAcceptanceOptionalPayloadFields(payload, *baseURL, *evidenceDir, *timeoutSeconds)
	return postWorkflowAcceptanceRunResult(ctx, *serverURL, payload, *jsonOutput, printCaseBatchStart)
}

func runCaseBatchReport(ctx context.Context, args []string) error {
	return runWorkflowAcceptanceReportCommand(ctx, args, "case batch report", "Case batch run id", printCaseBatchReport)
}

func printCaseBatchStart(payload map[string]any) {
	fmt.Printf("Case Batch Run: %s\n", valueString(payload["batchRunId"]))
	fmt.Printf("Status: %s\n", valueString(payload["status"]))
	if workflowID := valueString(payload["workflowId"]); workflowID != "" {
		fmt.Printf("Workflow: %s\n", workflowID)
	}
	if total := intFromReportAny(payload["total"]); total > 0 {
		fmt.Printf("Total: %d\n", total)
	}
	fmt.Printf("Report: %s\n", valueString(payload["reportUrl"]))
}

func printCaseBatchReport(payload map[string]any) {
	fmt.Printf("Case Batch Report: %s\n", valueString(payload["batchRunId"]))
	fmt.Printf("Status: %s\n", valueString(payload["status"]))
	fmt.Printf("OK: %t\n", boolFromReportAny(payload["ok"]))
	if total := intFromReportAny(payload["total"]); total > 0 {
		fmt.Printf("Total: %d\n", total)
	}
	if passed := intFromReportAny(payload["passed"]); passed > 0 {
		fmt.Printf("Passed: %d\n", passed)
	}
	if failed := intFromReportAny(payload["failed"]); failed > 0 {
		fmt.Printf("Failed: %d\n", failed)
	}
}
