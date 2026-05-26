package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"html"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"agent-testbench/internal/domain/profile"
	"agent-testbench/internal/domain/profilecatalog"
	"agent-testbench/internal/domain/workflowaudit"
	"agent-testbench/internal/server/controlplane"
	"agent-testbench/internal/store"
)

type workflowCaseReport struct {
	OK            bool                     `json:"ok"`
	ProfileID     string                   `json:"profileId"`
	WorkflowID    string                   `json:"workflowId"`
	WorkflowName  string                   `json:"workflowName"`
	RunID         string                   `json:"runId,omitempty"`
	ReportURL     string                   `json:"reportUrl"`
	JSONReportURL string                   `json:"jsonReportUrl"`
	ElapsedMs     int64                    `json:"elapsedMs"`
	GeneratedAt   time.Time                `json:"generatedAt"`
	Counts        workflowCaseReportCounts `json:"counts"`
	Steps         []workflowCaseReportStep `json:"steps"`
}

type workflowCaseReportCounts struct {
	Total  int `json:"total"`
	Passed int `json:"passed"`
	Failed int `json:"failed"`
}

type workflowCaseReportStep struct {
	StepID    string `json:"stepId"`
	Title     string `json:"title"`
	CaseID    string `json:"caseId"`
	RunID     string `json:"runId,omitempty"`
	CaseRunID string `json:"caseRunId,omitempty"`
	ViewerURL string `json:"viewerUrl,omitempty"`
	DetailURL string `json:"detailUrl,omitempty"`
	Status    string `json:"status"`
	HTTPCode  int    `json:"httpCode,omitempty"`
	ElapsedMs int64  `json:"elapsedMs"`
	Method    string `json:"method,omitempty"`
	FullURL   string `json:"fullUrl,omitempty"`
	Error     string `json:"error,omitempty"`
}

func runWorkflowReport(ctx context.Context, args []string) error {
	flags := flag.NewFlagSet("workflow report", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	workflowID := flags.String("workflow", "", "Workflow id")
	profilePath := flags.String("profile", "", "Profile bundle path or installed profile id")
	profileHome := flags.String("profile-home", "", "Installed profile bundle home")
	storeRef := flags.String("store", "", "Named Store config or Store DSN")
	storeURL := flags.String("store-url", "", legacyStoreURLFlagHelp)
	baseURL := flags.String("base-url", "", "Base URL for live request execution")
	outputDir := flags.String("output-dir", "", "Report output directory")
	jsonOutput := flags.Bool("json", false, "Emit a machine-readable JSON report")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if strings.TrimSpace(*workflowID) == "" {
		return errors.New("--workflow is required")
	}
	resolvedStoreURL, err := resolveRequiredDailyStoreReference(*storeRef, *storeURL)
	if err != nil {
		return err
	}
	bundle, sourceStore, cleanup, err := loadInterfaceNodeReportBundle(ctx, *profilePath, *profileHome, resolvedStoreURL)
	if err != nil {
		return err
	}
	defer cleanup()
	if strings.TrimSpace(*outputDir) == "" {
		*outputDir = filepath.Join(".runtime", "reports", "workflow."+safeReportID(*workflowID)+"."+time.Now().UTC().Format("20060102T150405.000000000Z"))
	}
	absOutputDir, err := filepath.Abs(*outputDir)
	if err != nil {
		return err
	}
	report, err := executeWorkflowCaseReport(ctx, bundle, sourceStore, *workflowID, absOutputDir, *baseURL)
	if err != nil {
		return err
	}
	if *jsonOutput {
		return writeIndentedJSON(report)
	}
	fmt.Printf("Workflow Report: %s\n", report.WorkflowID)
	fmt.Printf("OK: %t\n", report.OK)
	fmt.Printf("Total: %d Passed: %d Failed: %d\n", report.Counts.Total, report.Counts.Passed, report.Counts.Failed)
	fmt.Printf("Elapsed: %d ms\n", report.ElapsedMs)
	fmt.Printf("Report: %s\n", report.ReportURL)
	return nil
}

func executeWorkflowCaseReport(ctx context.Context, bundle profile.Bundle, sourceStore store.Store, workflowID string, outputDir string, baseURL string) (workflowCaseReport, error) {
	started := time.Now()
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return workflowCaseReport{}, err
	}
	runtime, err := requiredReportStore(sourceStore)
	if err != nil {
		return workflowCaseReport{}, err
	}
	if err := runtime.ReplaceProfileCatalog(ctx, profilecatalog.FromBundle(bundle, time.Now().UTC())); err != nil {
		return workflowCaseReport{}, err
	}
	handler := controlplane.NewWithOptions(bundle, controlplane.Options{Runtime: runtime})
	server := httptest.NewServer(handler)
	defer server.Close()
	catalog, err := fetchReportMap(server.URL + "/api/catalog")
	if err != nil {
		return workflowCaseReport{}, err
	}
	workflow, err := findWorkflowByIDFromCatalog(catalog, workflowID)
	if err != nil {
		return workflowCaseReport{}, err
	}
	bindingCaseIDs := workflowBindingCaseIDs(bundle.WorkflowBindings, workflowID)
	contextValues := map[string]any{}
	rawSteps := listFromReportAny(workflow["steps"])
	steps := make([]map[string]any, 0, len(rawSteps))
	stepReports := make([]workflowCaseReportStep, 0, len(rawSteps))
	for _, rawStep := range rawSteps {
		step := mapFromReportAny(rawStep)
		caseID := runnableWorkflowCaseID(bundle.APICases, valueString(step["caseId"]), bindingCaseIDs[valueString(step["id"])])
		if caseID == "" {
			continue
		}
		timeoutSeconds := workflowStepTimeoutSeconds(workflow, step)
		payload := map[string]any{
			"caseId":         caseID,
			"workflowId":     workflowID,
			"stepId":         valueString(step["id"]),
			"overrides":      contextValues,
			"timeoutSeconds": timeoutSeconds,
			"baseUrl":        baseURL,
		}
		result, err := postReportMap(server.URL+"/api/test-kit/run", payload)
		if err != nil {
			return workflowCaseReport{}, err
		}
		result["stepId"] = valueString(step["id"])
		result["title"] = firstNonEmpty(valueString(step["displayName"]), valueString(step["id"]))
		result["stepOk"] = boolFromReportAny(result["ok"])
		steps = append(steps, result)
		stepReports = append(stepReports, workflowReportStepItem(step, result))
		for key, value := range workflowExportedValues(step, result) {
			contextValues[key] = value
		}
		if !boolFromReportAny(result["ok"]) {
			break
		}
	}
	report := workflowCaseReport{
		OK:           len(stepReports) == len(rawSteps),
		ProfileID:    bundle.ID,
		WorkflowID:   workflowID,
		WorkflowName: firstNonEmpty(valueString(workflow["displayName"]), workflowID),
		ElapsedMs:    time.Since(started).Milliseconds(),
		GeneratedAt:  time.Now().UTC(),
		Steps:        stepReports,
		Counts:       workflowCaseReportCounts{Total: len(rawSteps)},
	}
	for _, item := range stepReports {
		if item.Status == store.StatusPassed {
			report.Counts.Passed++
		} else {
			report.Counts.Failed++
			report.OK = false
		}
	}
	if len(stepReports) != len(rawSteps) {
		report.Counts.Failed += len(rawSteps) - len(stepReports)
		report.OK = false
	}
	snapshot := map[string]any{
		"workflowId": workflowID,
		"status":     statusText(report.OK),
		"ok":         report.OK,
		"elapsedMs":  report.ElapsedMs,
		"summary": map[string]any{
			"expectedStepCount": len(rawSteps),
			"stepCount":         len(stepReports),
			"passed":            report.Counts.Passed,
			"elapsedMs":         report.ElapsedMs,
		},
		"steps": steps,
	}
	if len(steps) > 0 {
		if saved, err := postReportMap(server.URL+"/api/workflow-runs", snapshot); err == nil {
			report.RunID = valueString(saved["workflowRunId"])
		}
	}
	if err := writeWorkflowCaseReportFiles(outputDir, &report); err != nil {
		return workflowCaseReport{}, err
	}
	return report, nil
}

func workflowBindingCaseIDs(bindings []profile.WorkflowBinding, workflowID string) map[string]string {
	out := map[string]string{}
	for _, item := range bindings {
		if item.WorkflowID == workflowID && strings.TrimSpace(item.StepID) != "" && strings.TrimSpace(item.CaseID) != "" {
			out[item.StepID] = item.CaseID
		}
	}
	return out
}

func runnableWorkflowCaseID(cases []profile.APICase, candidates ...string) string {
	known := map[string]bool{}
	for _, item := range cases {
		known[item.ID] = true
	}
	for _, candidate := range candidates {
		candidate = strings.TrimSpace(candidate)
		if candidate != "" && known[candidate] {
			return candidate
		}
	}
	for _, candidate := range candidates {
		if strings.TrimSpace(candidate) != "" {
			return candidate
		}
	}
	return ""
}

func fetchReportMap(endpoint string) (map[string]any, error) {
	response, err := http.Get(endpoint)
	if err != nil {
		return nil, err
	}
	defer func() {
		if closeErr := response.Body.Close(); closeErr != nil {
			fmt.Fprintf(os.Stderr, "warning: close report response body: %v\n", closeErr)
		}
	}()
	var payload map[string]any
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		return nil, err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, fmt.Errorf("GET %s failed with http status %d", endpoint, response.StatusCode)
	}
	return payload, nil
}

func postReportMap(endpoint string, payload map[string]any) (map[string]any, error) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	response, err := http.Post(endpoint, "application/json", strings.NewReader(string(raw)))
	if err != nil {
		return nil, err
	}
	defer func() {
		if closeErr := response.Body.Close(); closeErr != nil {
			fmt.Fprintf(os.Stderr, "warning: close report response body: %v\n", closeErr)
		}
	}()
	var result map[string]any
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		return nil, err
	}
	result["httpStatus"] = response.StatusCode
	return result, nil
}

func findWorkflowByIDFromCatalog(catalog map[string]any, id string) (map[string]any, error) {
	id = strings.TrimSpace(id)
	for _, raw := range listFromReportAny(catalog["workflows"]) {
		workflow := mapFromReportAny(raw)
		if valueString(workflow["id"]) == id {
			return workflow, nil
		}
	}
	return nil, fmt.Errorf("workflow not found: %s", id)
}

func workflowStepTimeoutSeconds(workflow map[string]any, step map[string]any) int {
	timeoutMs := firstPositiveInt(intFromReportAny(step["timeoutMs"]), intFromReportAny(workflow["baseStepTimeoutMs"]), 3000)
	seconds := timeoutMs / 1000
	if timeoutMs%1000 != 0 {
		seconds++
	}
	if seconds <= 0 {
		return 3
	}
	return seconds
}

func workflowReportStepItem(step map[string]any, result map[string]any) workflowCaseReportStep {
	item := interfaceNodeCaseReportItems([]any{result})
	status := store.StatusFailed
	httpCode := 0
	elapsedMs := int64(intFromReportAny(result["elapsedMs"]))
	method := ""
	fullURL := ""
	errText := ""
	runID := valueString(result["runId"])
	caseRunID := valueString(result["caseRunId"])
	viewerURL := valueString(result["viewerUrl"])
	detailURL := valueString(result["detailUrl"])
	if len(item) > 0 {
		status = item[0].Status
		httpCode = item[0].HTTPCode
		elapsedMs = item[0].ElapsedMs
		method = item[0].Method
		fullURL = item[0].FullURL
		errText = item[0].Error
		runID = item[0].RunID
		caseRunID = item[0].CaseRunID
		viewerURL = item[0].ViewerURL
		detailURL = item[0].DetailURL
	}
	return workflowCaseReportStep{
		StepID:    valueString(step["id"]),
		Title:     firstNonEmpty(valueString(step["displayName"]), valueString(step["id"])),
		CaseID:    valueString(result["caseId"]),
		RunID:     runID,
		CaseRunID: caseRunID,
		ViewerURL: viewerURL,
		DetailURL: detailURL,
		Status:    status,
		HTTPCode:  httpCode,
		ElapsedMs: elapsedMs,
		Method:    method,
		FullURL:   fullURL,
		Error:     errText,
	}
}

func workflowExportedValues(step map[string]any, result map[string]any) map[string]any {
	out := map[string]any{}
	for _, rawExport := range listFromReportAny(step["exports"]) {
		item := mapFromReportAny(rawExport)
		name := valueString(item["name"])
		if name == "" {
			continue
		}
		value := workflowValueAtPath(workflowExportRoot(result, valueString(item["from"])), valueString(item["path"]))
		if value != nil && valueString(value) != "" {
			out[name] = value
		}
	}
	return out
}

func workflowExportRoot(result map[string]any, source string) any {
	resultBlock := mapFromReportAny(result["result"])
	request := mapFromReportAny(resultBlock["request"])
	response := mapFromReportAny(resultBlock["response"])
	responseBody := rawJSONObject(valueString(response["body"]))
	switch source {
	case "request", "requestBody":
		return firstReportValue(request, "body")
	case "requestQuery":
		return firstReportValue(request, "query")
	case "responseHeaders":
		return firstReportValue(response, "headers")
	case "response", "responseBody", "":
		return responseBody
	default:
		return responseBody
	}
}

func workflowValueAtPath(root any, path string) any {
	if strings.TrimSpace(path) == "" {
		return root
	}
	current := root
	for _, part := range strings.Split(path, ".") {
		switch typed := current.(type) {
		case map[string]any:
			current = typed[part]
		case []any:
			index, err := strconv.Atoi(part)
			if err != nil || index < 0 || index >= len(typed) {
				return nil
			}
			current = typed[index]
		default:
			return nil
		}
		if current == nil {
			return nil
		}
	}
	return current
}

func writeWorkflowCaseReportFiles(outputDir string, report *workflowCaseReport) error {
	jsonPath := filepath.Join(outputDir, "report.json")
	htmlPath := filepath.Join(outputDir, "report.html")
	report.JSONReportURL = jsonPath
	report.ReportURL = htmlPath
	raw, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(jsonPath, append(raw, '\n'), 0o644); err != nil {
		return err
	}
	return os.WriteFile(htmlPath, []byte(renderWorkflowCaseReportHTML(*report)), 0o644)
}

func renderWorkflowCaseReportHTML(report workflowCaseReport) string {
	var b strings.Builder
	b.WriteString(`<!doctype html><html><head><meta charset="utf-8"><title>Workflow Report</title><style>`)
	b.WriteString(`body{font-family:-apple-system,BlinkMacSystemFont,"Segoe UI",sans-serif;margin:24px;color:#111827;background:#f8fafc}main{max-width:1280px;margin:auto}h1{font-size:24px;margin:0 0 4px}.meta{color:#4b5563;margin-bottom:16px}.summary{display:flex;gap:8px;flex-wrap:wrap;margin:12px 0}.pill{border:1px solid #d1d5db;background:white;border-radius:6px;padding:6px 10px;font-size:13px}.ok{color:#047857}.bad{color:#b91c1c}table{width:100%;border-collapse:collapse;background:white;border:1px solid #d1d5db}th,td{border-bottom:1px solid #e5e7eb;text-align:left;vertical-align:top;padding:7px 8px;font-size:13px}th{background:#f3f4f6;color:#374151}.mono{font-family:ui-monospace,SFMono-Regular,Menlo,monospace;font-size:12px}.wrap{word-break:break-all}.small{font-size:12px;color:#6b7280}`)
	b.WriteString(`</style></head><body><main>`)
	b.WriteString(`<h1>` + html.EscapeString(report.WorkflowName) + `</h1>`)
	b.WriteString(`<div class="meta">` + html.EscapeString(report.WorkflowID))
	if report.RunID != "" {
		b.WriteString(` · ` + html.EscapeString(report.RunID))
	}
	b.WriteString(`</div><div class="summary">`)
	b.WriteString(reportPill("status", statusText(report.OK)))
	b.WriteString(reportPill("steps", strconv.Itoa(report.Counts.Total)))
	b.WriteString(reportPill("passed", strconv.Itoa(report.Counts.Passed)))
	b.WriteString(reportPill("failed", strconv.Itoa(report.Counts.Failed)))
	b.WriteString(reportPill("elapsed", fmt.Sprintf("%d ms", report.ElapsedMs)))
	b.WriteString(`</div><table><thead><tr><th>#</th><th>Step</th><th>Case</th><th>Status</th><th>HTTP</th><th>Elapsed</th><th>Evidence</th><th>Request</th><th>Error</th></tr></thead><tbody>`)
	for index, item := range report.Steps {
		statusClass := "bad"
		if item.Status == store.StatusPassed {
			statusClass = "ok"
		}
		b.WriteString(`<tr><td class="mono">` + strconv.Itoa(index+1) + `</td>`)
		b.WriteString(`<td><div>` + html.EscapeString(item.Title) + `</div><div class="mono small wrap">` + html.EscapeString(item.StepID) + `</div></td>`)
		b.WriteString(`<td class="mono wrap">` + html.EscapeString(item.CaseID) + `</td>`)
		b.WriteString(`<td class="` + statusClass + `">` + html.EscapeString(item.Status) + `</td>`)
		b.WriteString(`<td class="mono">` + strconv.Itoa(item.HTTPCode) + `</td>`)
		b.WriteString(`<td class="mono">` + fmt.Sprintf("%d ms", item.ElapsedMs) + `</td>`)
		b.WriteString(`<td class="mono wrap">`)
		if item.DetailURL != "" {
			b.WriteString(`<a href="` + html.EscapeString(item.DetailURL) + `">caseRunId</a><br>`)
		}
		b.WriteString(html.EscapeString(item.CaseRunID))
		b.WriteString(`</td>`)
		b.WriteString(`<td class="mono wrap">` + html.EscapeString(strings.TrimSpace(item.Method+" "+item.FullURL)) + `</td>`)
		b.WriteString(`<td class="wrap">` + html.EscapeString(item.Error) + `</td></tr>`)
	}
	b.WriteString(`</tbody></table></main></body></html>`)
	return b.String()
}

func runWorkflowAudit(ctx context.Context, args []string) error {
	flags := flag.NewFlagSet("workflow audit", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	profilePath := flags.String("profile", "", "Profile bundle path")
	workflowID := flags.String("workflow", "", "Workflow id")
	storeRef := flags.String("store", "", "Named Store config or Store DSN")
	storeURL := flags.String("store-url", "", legacyStoreURLFlagHelp)
	offlineTemplatePackage := flags.Bool("offline-template-package", false, "Read the template package directly for offline review")
	jsonOutput := flags.Bool("json", false, "Emit a machine-readable JSON report")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if strings.TrimSpace(*workflowID) == "" {
		return errors.New("--workflow is required")
	}

	var (
		bundle  profile.Bundle
		runtime store.Store
		cleanup = func() {}
		err     error
	)
	if *offlineTemplatePackage {
		if strings.TrimSpace(*profilePath) == "" {
			return errors.New("--offline-template-package requires --profile")
		}
		bundle, err = profile.Load(*profilePath)
		if err != nil {
			return err
		}
		resolvedStoreURL, err := resolveStoreReference(*storeRef, *storeURL)
		if err != nil {
			return err
		}
		if strings.TrimSpace(resolvedStoreURL) != "" {
			runtime, err = openStore(ctx, resolvedStoreURL)
			if err != nil {
				return err
			}
			cleanup = func() {
				closeCLIStore(runtime)
			}
		}
	} else {
		if strings.TrimSpace(*profilePath) != "" {
			return errors.New("--profile is for offline template package review; add --offline-template-package or use --store NAME_OR_DSN")
		}
		resolvedStoreURL, err := resolveRequiredDailyStoreReference(*storeRef, *storeURL)
		if err != nil {
			return err
		}
		runtime, err = openStore(ctx, resolvedStoreURL)
		if err != nil {
			return err
		}
		cleanup = func() {
			closeCLIStore(runtime)
		}
		bundle, err = serveBundle(ctx, runtime)
		if err != nil {
			cleanup()
			return err
		}
	}
	defer cleanup()
	if _, ok := findWorkflow(bundle, *workflowID); !ok {
		return fmt.Errorf("workflow not found: %s", *workflowID)
	}

	options := workflowaudit.Options{
		Bundle:     bundle,
		WorkflowID: *workflowID,
		Store:      runtime,
	}
	report, err := workflowaudit.Audit(ctx, options)
	if err != nil {
		return err
	}
	if *jsonOutput {
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		return encoder.Encode(report)
	}
	printWorkflowAudit(report)
	return nil
}

func printWorkflowAudit(report workflowaudit.Report) {
	fmt.Printf("Workflow Audit: %s\n", report.WorkflowID)
	fmt.Printf("Profile: %s\n", report.ProfileID)
	if report.DisplayName != "" {
		fmt.Printf("Display Name: %s\n", report.DisplayName)
	}
	fmt.Printf("OK: %t\n", report.OK)
	fmt.Printf("Issues: %d\n", report.IssueCount)
	fmt.Printf("Bindings: %d\n", report.BindingCount)
	for _, item := range report.Bindings {
		fmt.Printf("Binding: %s Node: %s", item.StepID, item.NodeID)
		if item.CaseID != "" {
			fmt.Printf(" Case: %s", item.CaseID)
		}
		fmt.Printf(" Required: %t\n", item.Required)
	}
	for _, item := range report.Issues {
		fmt.Printf("- [%s] %s %s %s: %s\n", item.Severity, item.Code, item.SubjectType, item.SubjectID, item.Message)
	}
	if report.Store == nil {
		return
	}
	if report.Store.LatestRun == nil {
		fmt.Println("Latest Run: not-run")
	} else {
		fmt.Printf("Latest Run: %s [%s]\n", report.Store.LatestRun.ID, report.Store.LatestRun.Status)
	}
	for _, item := range report.Store.BindingCases {
		status := item.LatestStatus
		if status == "" {
			status = "not-run"
		}
		fmt.Printf("Binding Case: %s %s Status: %s Passed: %t\n", item.StepID, item.CaseID, status, item.HasPassed)
	}
}

func runWorkflowPlan(args []string) error {
	flags := flag.NewFlagSet("workflow plan", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	profilePath := flags.String("profile", "", "Profile bundle path or installed profile id")
	profileHome := flags.String("profile-home", "", "Installed profile bundle home")
	storeRef := flags.String("store", "", "Named Store config or Store DSN")
	storeURL := flags.String("store-url", "", legacyStoreURLFlagHelp)
	workflowID := flags.String("workflow", "", "Workflow id")
	jsonOutput := flags.Bool("json", false, "Emit a machine-readable JSON report")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if strings.TrimSpace(*workflowID) == "" {
		return errors.New("--workflow is required")
	}
	bundle, runtime, _, cleanup, err := loadRequiredInterfaceNodeReportBundleFromStoreFlags(context.Background(), *profilePath, *profileHome, *storeRef, *storeURL)
	if err != nil {
		return err
	}
	defer cleanup()
	var planStore store.Store
	if runtime != nil {
		planStore = runtime
	}
	payload, ok, err := controlplane.WorkflowPlanPayload(context.Background(), bundle, *workflowID, planStore)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("workflow not found: %s", *workflowID)
	}
	if *jsonOutput {
		return writeIndentedJSON(payload)
	}

	fmt.Printf("Workflow: %s\n", *workflowID)
	for _, raw := range listFromReportAny(payload["steps"]) {
		step := mapFromReportAny(raw)
		fmt.Printf("Step: %s\n", valueString(step["stepId"]))
		fmt.Printf("Node: %s\n", valueString(step["nodeId"]))
		if caseID := valueString(step["caseId"]); caseID != "" {
			fmt.Printf("Case: %s\n", caseID)
		}
		fmt.Printf("Required: %t\n", boolFromReportAny(step["required"]))
	}
	return nil
}

func findWorkflow(bundle profile.Bundle, id string) (profile.Workflow, bool) {
	for _, workflow := range bundle.Workflows {
		if workflow.ID == id {
			return workflow, true
		}
	}
	return profile.Workflow{}, false
}
