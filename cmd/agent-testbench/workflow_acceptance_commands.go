package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"os"
	"strings"
)

func runWorkflowAcceptanceCommand(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return errors.New("missing workflow acceptance command")
	}
	switch args[0] {
	case "start":
		return runWorkflowAcceptanceStart(ctx, args[1:])
	case "report":
		return runWorkflowAcceptanceReport(ctx, args[1:])
	default:
		return fmt.Errorf("unknown workflow acceptance command: %s", args[0])
	}
}

func runWorkflowAcceptanceStart(ctx context.Context, args []string) error {
	flags := flag.NewFlagSet("workflow acceptance start", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	serverURL := flags.String("server-url", "", "Running control plane base URL")
	workflowID := flags.String("workflow", "", "Workflow id")
	requestID := flags.String("request-id", "", "Acceptance request id")
	baseURL := flags.String("base-url", "", "Base URL for live request execution")
	evidenceDir := flags.String("evidence-dir", "", "Evidence output directory")
	timeoutSeconds := flags.Int("timeout-seconds", 0, "Per-step timeout in seconds")
	jsonOutput := flags.Bool("json", false, "Emit machine-readable JSON")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if strings.TrimSpace(*serverURL) == "" || strings.TrimSpace(*workflowID) == "" || strings.TrimSpace(*requestID) == "" {
		return errors.New("--server-url, --workflow, and --request-id are required")
	}
	payload := map[string]any{
		"requestId":  strings.TrimSpace(*requestID),
		"workflowId": strings.TrimSpace(*workflowID),
	}
	addWorkflowAcceptanceOptionalPayloadFields(payload, *baseURL, *evidenceDir, *timeoutSeconds)
	return postWorkflowAcceptanceRunResult(ctx, *serverURL, payload, *jsonOutput, printWorkflowAcceptanceStart)
}

func runWorkflowAcceptanceReport(ctx context.Context, args []string) error {
	return runWorkflowAcceptanceReportCommand(ctx, args, "workflow acceptance report", "Acceptance batch run id", printWorkflowAcceptanceReport)
}

func workflowAcceptanceURL(serverURL string, apiPath string) string {
	return strings.TrimRight(strings.TrimSpace(serverURL), "/") + apiPath
}

func postWorkflowAcceptanceJSON(ctx context.Context, endpoint string, payload map[string]any) (map[string]any, error) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(string(raw)))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	return doWorkflowAcceptanceJSON(req)
}

func fetchWorkflowAcceptanceJSON(ctx context.Context, endpoint string) (map[string]any, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	return doWorkflowAcceptanceJSON(req)
}

func doWorkflowAcceptanceJSON(req *http.Request) (map[string]any, error) {
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			fmt.Fprintf(os.Stderr, "warning: close workflow acceptance response body: %v\n", closeErr)
		}
	}()
	var payload map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return payload, fmt.Errorf("%s %s failed with http status %d: %s", req.Method, req.URL.String(), resp.StatusCode, valueString(payload["error"]))
	}
	return payload, nil
}

func printWorkflowAcceptanceStart(payload map[string]any) {
	fmt.Printf("Workflow Acceptance Run: %s\n", valueString(payload["batchRunId"]))
	fmt.Printf("Workflow: %s\n", valueString(payload["workflowId"]))
	fmt.Printf("Status: %s\n", valueString(payload["status"]))
	fmt.Printf("Report: %s\n", valueString(payload["reportUrl"]))
}

func printWorkflowAcceptanceReport(payload map[string]any) {
	acceptance := mapFromReportAny(payload["acceptance"])
	fmt.Printf("Workflow Acceptance Report: %s\n", valueString(payload["batchRunId"]))
	fmt.Printf("Workflow: %s\n", firstNonEmpty(valueString(acceptance["workflowId"]), valueString(payload["workflowId"])))
	fmt.Printf("Status: %s\n", valueString(payload["status"]))
	fmt.Printf("Accepted: %t\n", boolFromReportAny(acceptance["ok"]))
	fmt.Printf("Template: %s\n", valueString(acceptance["templateId"]))
}
