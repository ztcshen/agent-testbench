package main

import (
	"context"
	"errors"
	"flag"
	"net/url"
	"os"
	"strings"
)

func addWorkflowAcceptanceOptionalPayloadFields(payload map[string]any, baseURL string, evidenceDir string, timeoutSeconds int) {
	if strings.TrimSpace(baseURL) != "" {
		payload["baseUrl"] = strings.TrimSpace(baseURL)
	}
	if strings.TrimSpace(evidenceDir) != "" {
		payload["evidenceDir"] = strings.TrimSpace(evidenceDir)
	}
	if timeoutSeconds > 0 {
		payload["timeoutSeconds"] = timeoutSeconds
	}
}

func postWorkflowAcceptanceRunResult(ctx context.Context, serverURL string, payload map[string]any, jsonOutput bool, printer func(map[string]any)) error {
	result, err := postWorkflowAcceptanceJSON(ctx, workflowAcceptanceURL(serverURL, "/api/cases/batch-runs"), payload)
	if err != nil {
		return err
	}
	return writeOrPrintWorkflowAcceptanceResult(result, jsonOutput, printer)
}

func runWorkflowAcceptanceReportCommand(ctx context.Context, args []string, commandName string, runHelp string, printer func(map[string]any)) error {
	flags := flag.NewFlagSet(commandName, flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	serverURL := flags.String("server-url", "", "Running control plane base URL")
	runID := flags.String("run", "", runHelp)
	jsonOutput := flags.Bool("json", false, "Emit machine-readable JSON")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if strings.TrimSpace(*serverURL) == "" || strings.TrimSpace(*runID) == "" {
		return errors.New("--server-url and --run are required")
	}
	path := "/api/cases/batch-runs/" + url.PathEscape(strings.TrimSpace(*runID))
	result, err := fetchWorkflowAcceptanceJSON(ctx, workflowAcceptanceURL(*serverURL, path))
	if err != nil {
		return err
	}
	return writeOrPrintWorkflowAcceptanceResult(result, *jsonOutput, printer)
}

func writeOrPrintWorkflowAcceptanceResult(result map[string]any, jsonOutput bool, printer func(map[string]any)) error {
	if jsonOutput {
		return writeIndentedJSON(result)
	}
	printer(result)
	return nil
}
