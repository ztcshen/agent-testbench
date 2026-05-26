package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"agent-testbench/internal/domain/profile"
	"agent-testbench/internal/domain/profilecatalog"
	"agent-testbench/internal/runner/apicase"
	"agent-testbench/internal/server/controlplane"
	"agent-testbench/internal/store"
)

func caseExecutionConfigIDs(configs []profile.TemplateConfig) map[string]string {
	out := map[string]string{}
	for _, config := range configs {
		if config.Status != "" && config.Status != "active" {
			continue
		}
		caseID, ok := caseExecutionConfigCaseID(config.ConfigJSON)
		if ok {
			out[caseID] = config.ID
		}
	}
	return out
}

func caseExecutionConfigCaseID(configJSON string) (string, bool) {
	var parsed struct {
		CaseID        string `json:"caseId"`
		CaseExecution struct {
			Method string `json:"method"`
			NodeID string `json:"nodeId"`
			Path   string `json:"path"`
		} `json:"caseExecution"`
	}
	if err := json.Unmarshal([]byte(configJSON), &parsed); err != nil {
		return "", false
	}
	if strings.TrimSpace(parsed.CaseID) == "" {
		return "", false
	}
	if parsed.CaseExecution.Method == "" && parsed.CaseExecution.NodeID == "" && parsed.CaseExecution.Path == "" {
		return "", false
	}
	return parsed.CaseID, true
}

func runCase(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return errors.New("missing case command")
	}
	switch args[0] {
	case "discover":
		return runCaseDiscover(ctx, args[1:])
	case "suite":
		return runCaseSuite(ctx, args[1:])
	case "run":
		return runCaseRun(ctx, args[1:])
	case "runs":
		return runCaseRuns(ctx, args[1:])
	case "evidence":
		return runCaseEvidence(ctx, args[1:])
	case "diagnose":
		return runCaseDiagnose(ctx, args[1:])
	case "gate":
		return runCaseGate(ctx, args[1:])
	case "timing":
		return runCaseTiming(ctx, args[1:])
	case "batch":
		return runCaseBatch(ctx, args[1:])
	case "incomplete-batches":
		return runCaseIncompleteBatches(ctx, args[1:])
	default:
		return fmt.Errorf("unknown case command: %s", args[0])
	}
}

func runCaseBatch(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return errors.New("missing case batch command")
	}
	switch args[0] {
	case "start":
		return runCaseBatchStart(ctx, args[1:])
	case "report":
		return runCaseBatchReport(ctx, args[1:])
	default:
		return fmt.Errorf("unknown case batch command: %s", args[0])
	}
}

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
	if strings.TrimSpace(*baseURL) != "" {
		payload["baseUrl"] = strings.TrimSpace(*baseURL)
	}
	if strings.TrimSpace(*evidenceDir) != "" {
		payload["evidenceDir"] = strings.TrimSpace(*evidenceDir)
	}
	if *timeoutSeconds > 0 {
		payload["timeoutSeconds"] = *timeoutSeconds
	}
	result, err := postWorkflowAcceptanceJSON(ctx, workflowAcceptanceURL(*serverURL, "/api/cases/batch-runs"), payload)
	if err != nil {
		return err
	}
	if *jsonOutput {
		return writeIndentedJSON(result)
	}
	printCaseBatchStart(result)
	return nil
}

func runCaseBatchReport(ctx context.Context, args []string) error {
	flags := flag.NewFlagSet("case batch report", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	serverURL := flags.String("server-url", "", "Running control plane base URL")
	runID := flags.String("run", "", "Case batch run id")
	jsonOutput := flags.Bool("json", false, "Emit machine-readable JSON")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if strings.TrimSpace(*serverURL) == "" || strings.TrimSpace(*runID) == "" {
		return errors.New("--server-url and --run are required")
	}
	result, err := fetchWorkflowAcceptanceJSON(ctx, workflowAcceptanceURL(*serverURL, "/api/cases/batch-runs/"+url.PathEscape(strings.TrimSpace(*runID))))
	if err != nil {
		return err
	}
	if *jsonOutput {
		return writeIndentedJSON(result)
	}
	printCaseBatchReport(result)
	return nil
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

type caseListReport struct {
	OK        bool           `json:"ok"`
	ProfileID string         `json:"profileId"`
	Count     int            `json:"count"`
	Filters   caseListFilter `json:"filters"`
	Items     []caseListItem `json:"items"`
}

type caseListFilter struct {
	Filter   string   `json:"filter,omitempty"`
	NodeID   string   `json:"nodeId,omitempty"`
	Tags     []string `json:"tags,omitempty"`
	Status   string   `json:"status,omitempty"`
	Owner    string   `json:"owner,omitempty"`
	Priority string   `json:"priority,omitempty"`
}

type caseListItem struct {
	ID                   string   `json:"id"`
	DisplayName          string   `json:"displayName,omitempty"`
	Description          string   `json:"description,omitempty"`
	NodeID               string   `json:"nodeId,omitempty"`
	CaseType             string   `json:"caseType,omitempty"`
	Scenario             string   `json:"scenario,omitempty"`
	Tags                 []string `json:"tags,omitempty"`
	Priority             string   `json:"priority,omitempty"`
	Owner                string   `json:"owner,omitempty"`
	Status               string   `json:"status,omitempty"`
	RequiredForAdmission bool     `json:"requiredForAdmission"`
	SortOrder            int      `json:"sortOrder,omitempty"`
	HasRunnableFile      bool     `json:"hasRunnableFile"`
	HasExecutionConfig   bool     `json:"hasExecutionConfig"`
}

func runCaseTiming(ctx context.Context, args []string) error {
	flags := flag.NewFlagSet("case timing", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	storeRef := flags.String("store", "", "Named Store config or Store DSN")
	storeURL := flags.String("store-url", "", legacyStoreURLFlagHelp)
	kind := flags.String("kind", "", "Timing kind")
	maxAgeMinutes := flags.String("max-age-minutes", "", "Only include case runs created within this many minutes")
	jsonOutput := flags.Bool("json", false, "Emit a machine-readable JSON report")
	if err := flags.Parse(args); err != nil {
		return err
	}
	runtime, cleanup, err := openRequiredCLIStore(ctx, *storeRef, *storeURL)
	if err != nil {
		return err
	}
	defer cleanup()
	payload, err := controlplane.CaseTimingPayload(ctx, runtime, *kind, *maxAgeMinutes)
	if err != nil {
		return err
	}
	if *jsonOutput {
		return writeIndentedJSON(payload)
	}
	printCaseTiming(payload)
	return nil
}

func printCaseTiming(payload map[string]any) {
	summary := mapFromReportAny(payload["summary"])
	fmt.Println("Case Timing")
	fmt.Printf("Case Runs: %s\n", valueString(summary["caseRunCount"]))
	fmt.Printf("Measured: %s\n", valueString(summary["durationMeasuredCount"]))
	fmt.Printf("Max Duration: %s ms\n", valueString(summary["maxDurationMs"]))
	if slowest := mapFromReportAny(summary["slowestRows"]); len(slowest) > 0 {
		if row := mapFromReportAny(slowest["caseRun"]); len(row) > 0 {
			fmt.Printf("Slowest: %s %s ms\n", valueString(row["id"]), valueString(row["durationMs"]))
		}
	}
}

func runCaseEvidence(ctx context.Context, args []string) error {
	flags := flag.NewFlagSet("case evidence", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	storeRef := flags.String("store", "", "Named Store config or Store DSN")
	storeURL := flags.String("store-url", "", legacyStoreURLFlagHelp)
	caseRunID := flags.String("case-run", "", "Case run id")
	runID := flags.String("run", "", "Run id")
	caseID := flags.String("case-id", "", "Case id within the run")
	stepID := flags.String("step-id", "", "Workflow step id within the run")
	jsonOutput := flags.Bool("json", false, "Emit a machine-readable JSON report")
	if err := flags.Parse(args); err != nil {
		return err
	}
	runtime, cleanup, err := openRequiredCLIStore(ctx, *storeRef, *storeURL)
	if err != nil {
		return err
	}
	defer cleanup()
	payload, err := readCaseEvidence(ctx, runtime, *caseRunID, *runID, *caseID, *stepID)
	if err != nil {
		return err
	}
	if *jsonOutput {
		return writeIndentedJSON(payload)
	}
	printCaseEvidence(payload)
	return nil
}

func readCaseEvidence(ctx context.Context, runtime store.Store, caseRunID string, runID string, caseID string, stepID string) (map[string]any, error) {
	var payload map[string]any
	var ok bool
	var err error
	if strings.TrimSpace(caseRunID) != "" {
		payload, ok, err = controlplane.CaseEvidencePayloadForCaseRunID(ctx, runtime, caseRunID)
	} else if strings.TrimSpace(runID) != "" {
		payload, ok, err = controlplane.CaseEvidencePayloadForRunID(ctx, runtime, runID, caseID, stepID)
	} else {
		return nil, errors.New("--case-run or --run is required")
	}
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, errors.New("case evidence not found")
	}
	return payload, nil
}

func printCaseEvidence(payload map[string]any) {
	evidence := mapFromReportAny(payload["evidence"])
	summary := mapFromReportAny(evidence["summary"])
	fmt.Println("Case Evidence")
	fmt.Printf("Case Run: %s\n", valueString(summary["case_run_id"]))
	fmt.Printf("Case: %s\n", valueString(summary["case_id"]))
	fmt.Printf("Run: %s\n", valueString(summary["run_id"]))
	fmt.Printf("Status: %s\n", valueString(summary["status"]))
	fmt.Printf("Operation: %s\n", valueString(summary["operation"]))
	if evidencePath := valueString(summary["evidence_path"]); evidencePath != "" {
		fmt.Printf("Evidence: %s\n", evidencePath)
	}
}

type caseRunsCLIReport struct {
	OK       bool              `json:"ok"`
	CaseRuns []caseRunsCLIItem `json:"caseRuns"`
	Warnings []string          `json:"warnings"`
}

type caseRunsCLIItem struct {
	ID            string    `json:"id"`
	RunID         string    `json:"runId"`
	CaseID        string    `json:"caseId"`
	Status        string    `json:"status"`
	Operation     string    `json:"operation"`
	EvidencePath  string    `json:"evidencePath"`
	EvidenceCount int       `json:"evidenceCount"`
	UpdatedAt     time.Time `json:"updatedAt"`
}

type caseGateReport struct {
	OK              bool              `json:"ok"`
	RunID           string            `json:"runId,omitempty"`
	Counts          caseGateCounts    `json:"counts"`
	Gates           caseGateGates     `json:"gates"`
	FailedCaseRuns  []caseRunsCLIItem `json:"failedCaseRuns"`
	MissingEvidence []caseRunsCLIItem `json:"missingEvidence"`
	NextActions     []string          `json:"nextActions"`
	Warnings        []string          `json:"warnings"`
}

type caseGateCounts struct {
	Total            int `json:"total"`
	Passed           int `json:"passed"`
	Failed           int `json:"failed"`
	Other            int `json:"other"`
	EvidenceComplete int `json:"evidenceComplete"`
}

type caseGateGates struct {
	HasCaseRuns      bool `json:"hasCaseRuns"`
	NoFailures       bool `json:"noFailures"`
	MinPassed        bool `json:"minPassed"`
	EvidenceComplete bool `json:"evidenceComplete"`
}

type caseGateOptions struct {
	RunID             string
	RequireNoFailures bool
	RequireEvidence   bool
	MinPassed         int
}

func runCaseGate(ctx context.Context, args []string) error {
	flags := flag.NewFlagSet("case gate", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	storeRef := flags.String("store", "", "Named Store config or Store DSN")
	storeURL := flags.String("store-url", "", legacyStoreURLFlagHelp)
	runFilter := flags.String("run", "", "Only gate case runs for one run id")
	requireNoFailures := flags.Bool("require-no-failures", false, "Fail when any selected case run is not passed")
	requireEvidence := flags.Bool("require-evidence", false, "Fail when any selected case run has no indexed Evidence")
	minPassed := flags.Int("min-passed", 0, "Fail unless at least this many selected case runs passed")
	jsonOutput := flags.Bool("json", false, "Emit a machine-readable JSON report")
	if err := flags.Parse(args); err != nil {
		return err
	}
	runtime, cleanup, err := openRequiredCLIStore(ctx, *storeRef, *storeURL)
	if err != nil {
		return err
	}
	defer cleanup()
	report, err := buildCaseGateReport(ctx, runtime, caseGateOptions{
		RunID:             *runFilter,
		RequireNoFailures: *requireNoFailures,
		RequireEvidence:   *requireEvidence,
		MinPassed:         *minPassed,
	})
	if err != nil {
		return err
	}
	if *jsonOutput {
		if err := writeIndentedJSON(report); err != nil {
			return err
		}
	} else {
		printCaseGate(report)
	}
	if !report.OK {
		return errors.New("case gate failed")
	}
	return nil
}

func caseGateNextActions(report caseGateReport, options caseGateOptions) []string {
	actions := []string{}
	if !report.Gates.HasCaseRuns {
		base := "agent-testbench case runs --json"
		if report.RunID != "" {
			base = "agent-testbench case runs --run " + report.RunID + " --json"
		}
		return []string{base}
	}
	for index, item := range report.FailedCaseRuns {
		if index >= 3 {
			break
		}
		actions = append(actions, "agent-testbench case diagnose --case-run "+item.ID+" --json")
	}
	if options.RequireEvidence {
		for index, item := range report.MissingEvidence {
			if index >= 3 {
				break
			}
			actions = append(actions, "agent-testbench case evidence --case-run "+item.ID+" --json")
		}
	}
	if options.MinPassed > 0 && !report.Gates.MinPassed {
		actions = append(actions, fmt.Sprintf("Run or repair enough cases to reach min-passed=%d", options.MinPassed))
	}
	if len(actions) == 0 {
		actions = append(actions, "Case gate passed; no action needed")
	}
	return actions
}

func printCaseGate(report caseGateReport) {
	fmt.Println("Case Gate")
	fmt.Printf("OK: %t\n", report.OK)
	if report.RunID != "" {
		fmt.Printf("Run: %s\n", report.RunID)
	}
	fmt.Printf("Total: %d Passed: %d Failed: %d Other: %d EvidenceComplete: %d\n", report.Counts.Total, report.Counts.Passed, report.Counts.Failed, report.Counts.Other, report.Counts.EvidenceComplete)
	fmt.Printf("Gates: hasCaseRuns=%t noFailures=%t minPassed=%t evidenceComplete=%t\n", report.Gates.HasCaseRuns, report.Gates.NoFailures, report.Gates.MinPassed, report.Gates.EvidenceComplete)
	for _, item := range report.FailedCaseRuns {
		fmt.Printf("Failed: %s %s %s\n", item.ID, item.CaseID, item.Status)
	}
	for _, item := range report.MissingEvidence {
		fmt.Printf("Missing Evidence: %s %s\n", item.ID, item.CaseID)
	}
	for _, action := range report.NextActions {
		fmt.Printf("Next: %s\n", action)
	}
}

func runCaseRuns(ctx context.Context, args []string) error {
	flags := flag.NewFlagSet("case runs", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	storeRef := flags.String("store", "", "Named Store config or Store DSN")
	storeURL := flags.String("store-url", "", legacyStoreURLFlagHelp)
	runFilter := flags.String("run", "", "Only list case runs for one run id")
	jsonOutput := flags.Bool("json", false, "Emit a machine-readable JSON report")
	if err := flags.Parse(args); err != nil {
		return err
	}
	runtime, cleanup, err := openRequiredCLIStore(ctx, *storeRef, *storeURL)
	if err != nil {
		return err
	}
	defer cleanup()
	report, err := listCaseRunsFromStore(ctx, runtime, *runFilter)
	if err != nil {
		return err
	}
	if *jsonOutput {
		return writeIndentedJSON(report)
	}
	printCaseRuns(report)
	return nil
}

func caseRunsCLIItemFrom(run store.Run, item store.APICaseRun, evidence []store.EvidenceRecord) caseRunsCLIItem {
	evidenceCount := 0
	for _, record := range evidence {
		if record.CaseRunID == item.ID {
			evidenceCount++
		}
	}
	request := rawJSONObject(item.RequestSummaryJSON)
	return caseRunsCLIItem{
		ID:            item.ID,
		RunID:         item.RunID,
		CaseID:        item.CaseID,
		Status:        item.Status,
		Operation:     caseRunOperationFromRequest(request, item.CaseID),
		EvidencePath:  run.EvidenceRoot,
		EvidenceCount: evidenceCount,
		UpdatedAt:     firstNonZeroTime(item.CreatedAt, run.UpdatedAt, run.CreatedAt),
	}
}

func caseRunOperationFromRequest(request map[string]any, defaultValue string) string {
	method := strings.ToUpper(strings.TrimSpace(valueString(request["method"])))
	path := strings.TrimSpace(valueString(request["path"]))
	if method != "" && path != "" {
		return method + " " + path
	}
	if method != "" {
		return method
	}
	if path != "" {
		return path
	}
	return defaultValue
}

func firstNonZeroTime(values ...time.Time) time.Time {
	for _, value := range values {
		if !value.IsZero() {
			return value
		}
	}
	return time.Time{}
}

func printCaseRuns(report caseRunsCLIReport) {
	fmt.Println("Case Runs")
	fmt.Printf("Total: %d\n", len(report.CaseRuns))
	for _, item := range report.CaseRuns {
		fmt.Printf("- %s [%s] %s %s evidence=%d\n", item.ID, item.Status, item.CaseID, item.Operation, item.EvidenceCount)
	}
}

func runCaseDiscover(ctx context.Context, args []string) error {
	flags := flag.NewFlagSet("case discover", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	profilePath := flags.String("profile", "", "Profile bundle path or installed profile id")
	profileHome := flags.String("profile-home", "", "Installed profile bundle home")
	storeRef := flags.String("store", "", "Named Store config or Store DSN")
	storeURL := flags.String("store-url", "", legacyStoreURLFlagHelp)
	filter := flags.String("filter", "", "Filter by id, display name, scenario, description, tag, owner, or priority")
	nodeID := flags.String("node", "", "Only include cases attached to this interface node id")
	status := flags.String("status", "", "Only include cases with this status")
	owner := flags.String("owner", "", "Only include cases owned by this value")
	priority := flags.String("priority", "", "Only include cases with this priority")
	offlineTemplatePackage := flags.Bool("offline-template-package", false, "Read the template package directly for offline review")
	jsonOutput := flags.Bool("json", false, "Emit a machine-readable JSON report")
	var tags stringListFlag
	flags.Var(&tags, "tag", "Only include cases with this tag; repeat for multiple tags")
	if err := flags.Parse(args); err != nil {
		return err
	}
	discoveryProfileRef, resolvedStoreURL, err := resolveDiscoveryInputs(*profilePath, *storeRef, *storeURL, *offlineTemplatePackage)
	if err != nil {
		return err
	}
	bundle, sourceStore, cleanup, err := loadInterfaceNodeReportBundle(ctx, discoveryProfileRef, *profileHome, resolvedStoreURL)
	if err != nil {
		return err
	}
	defer cleanup()
	report := caseList(ctx, bundle, sourceStore, caseListFilter{
		Filter:   *filter,
		NodeID:   *nodeID,
		Tags:     tags.Values(),
		Status:   *status,
		Owner:    *owner,
		Priority: *priority,
	})
	if *jsonOutput {
		return writeIndentedJSON(report)
	}
	for _, item := range report.Items {
		fmt.Printf("%s\t%s\t%s\t%s\t%s\t%s\n", item.ID, item.DisplayName, item.NodeID, item.Status, item.Priority, strings.Join(item.Tags, ","))
	}
	return nil
}

func caseList(ctx context.Context, bundle profile.Bundle, runtime store.Store, filters caseListFilter) caseListReport {
	cases := append([]profile.APICase(nil), bundle.APICases...)
	sort.SliceStable(cases, func(i, j int) bool {
		if cases[i].NodeID != cases[j].NodeID {
			return cases[i].NodeID < cases[j].NodeID
		}
		if cases[i].SortOrder != cases[j].SortOrder {
			return cases[i].SortOrder < cases[j].SortOrder
		}
		return cases[i].ID < cases[j].ID
	})
	executionConfigs := caseExecutionConfigSet(ctx, runtime)
	report := caseListReport{OK: true, ProfileID: bundle.ID, Filters: normalizeCaseListFilter(filters)}
	for _, item := range cases {
		if !matchesCaseFilters(item, filters) {
			continue
		}
		report.Items = append(report.Items, caseListItem{
			ID:                   item.ID,
			DisplayName:          item.DisplayName,
			Description:          item.Description,
			NodeID:               item.NodeID,
			CaseType:             item.CaseType,
			Scenario:             item.Scenario,
			Tags:                 append([]string(nil), item.Tags...),
			Priority:             item.Priority,
			Owner:                item.Owner,
			Status:               effectiveCaseStatus(item),
			RequiredForAdmission: item.RequiredForAdmission,
			SortOrder:            item.SortOrder,
			HasRunnableFile:      strings.TrimSpace(item.CasePath) != "",
			HasExecutionConfig:   executionConfigs[item.ID],
		})
	}
	report.Count = len(report.Items)
	return report
}

func effectiveCaseStatus(item profile.APICase) string {
	status := strings.TrimSpace(item.Status)
	if status == "" {
		return "active"
	}
	return status
}

func caseHasAllTags(actual []string, required []string) bool {
	actualSet := map[string]bool{}
	for _, tag := range actual {
		normalized := normalizedDiscoveryText(tag)
		if normalized != "" {
			actualSet[normalized] = true
		}
	}
	for _, tag := range required {
		normalized := normalizedDiscoveryText(tag)
		if normalized != "" && !actualSet[normalized] {
			return false
		}
	}
	return true
}

func caseExecutionConfigSet(ctx context.Context, runtime store.Store) map[string]bool {
	out := map[string]bool{}
	if runtime == nil {
		return out
	}
	catalog, err := runtime.GetProfileCatalog(ctx)
	if err != nil {
		return out
	}
	for _, config := range catalog.TemplateConfigs {
		if config.ScopeType == "case" && strings.TrimSpace(config.ScopeID) != "" {
			out[strings.TrimSpace(config.ScopeID)] = true
			continue
		}
		var payload struct {
			CaseID string `json:"caseId"`
		}
		if json.Unmarshal([]byte(config.ConfigJSON), &payload) == nil && strings.TrimSpace(payload.CaseID) != "" {
			out[strings.TrimSpace(payload.CaseID)] = true
		}
	}
	return out
}

func runCaseIncompleteBatches(ctx context.Context, args []string) error {
	flags := flag.NewFlagSet("case incomplete-batches", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	profilePath := flags.String("profile", "", "Profile bundle path")
	storeRef := flags.String("store", "", "Named Store config or Store DSN")
	storeURL := flags.String("store-url", "", legacyStoreURLFlagHelp)
	jsonOutput := flags.Bool("json", false, "Print JSON")
	if err := flags.Parse(args); err != nil {
		return err
	}
	resolvedStoreURL, err := resolveRequiredDailyStoreReference(*storeRef, *storeURL)
	if err != nil {
		return err
	}
	s, err := openStore(ctx, resolvedStoreURL)
	if err != nil {
		return err
	}
	defer s.Close()

	bundle, err := incompleteCaseBundle(ctx, strings.TrimSpace(*profilePath), s)
	if err != nil {
		return err
	}
	report, err := incompleteCaseReportForStore(ctx, bundle, s)
	if err != nil {
		return err
	}
	if *jsonOutput {
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		return encoder.Encode(report)
	}
	printIncompleteCaseReport(report)
	return nil
}

func incompleteCaseBundle(ctx context.Context, profilePath string, runtime store.Store) (profile.Bundle, error) {
	if profilePath != "" {
		return profile.Load(profilePath)
	}
	catalog, err := runtime.GetProfileCatalog(ctx)
	if err != nil {
		return profile.Bundle{}, err
	}
	return profilecatalog.ToBundle(catalog), nil
}

type incompleteCaseReport struct {
	OK       bool                 `json:"ok"`
	Count    int                  `json:"count"`
	Items    []incompleteCaseItem `json:"items"`
	Warnings []string             `json:"warnings"`
}

type incompleteCaseItem struct {
	ID               string `json:"id"`
	Title            string `json:"title"`
	Reason           string `json:"reason"`
	Source           string `json:"source"`
	Message          string `json:"message"`
	SuggestedCommand string `json:"suggestedCommand"`
}

func incompleteCaseReportForStore(ctx context.Context, bundle profile.Bundle, s store.Store) (incompleteCaseReport, error) {
	passed, latest, err := apiCaseRunStatusByCase(ctx, s)
	if err != nil {
		return incompleteCaseReport{}, err
	}
	items := make([]incompleteCaseItem, 0)
	for _, item := range bundle.APICases {
		if strings.TrimSpace(item.ID) == "" || passed[item.ID] {
			continue
		}
		reason := "not-run"
		if status := latest[item.ID]; status != "" {
			reason = "latest-" + status
		}
		items = append(items, incompleteCaseItem{
			ID:               item.ID,
			Title:            firstNonEmpty(item.DisplayName, item.ID),
			Reason:           reason,
			Source:           "profile:" + bundle.ID,
			Message:          "no passed Store run found for this API Case",
			SuggestedCommand: apiCaseSuggestedCommand(item),
		})
	}
	return incompleteCaseReport{
		OK:       true,
		Count:    len(items),
		Items:    items,
		Warnings: []string{},
	}, nil
}

func apiCaseRunStatusByCase(ctx context.Context, s store.Store) (map[string]bool, map[string]string, error) {
	runs, err := s.ListRuns(ctx)
	if err != nil {
		return nil, nil, err
	}
	passed := map[string]bool{}
	latest := map[string]string{}
	for i := len(runs) - 1; i >= 0; i-- {
		caseRuns, err := s.ListAPICaseRuns(ctx, runs[i].ID)
		if err != nil {
			return nil, nil, err
		}
		for _, item := range caseRuns {
			if latest[item.CaseID] == "" {
				latest[item.CaseID] = item.Status
			}
			if strings.EqualFold(item.Status, store.StatusPassed) {
				passed[item.CaseID] = true
			}
		}
	}
	return passed, latest, nil
}

func apiCaseSuggestedCommand(item profile.APICase) string {
	casePath := strings.TrimSpace(item.CasePath)
	if casePath == "" {
		return ""
	}
	parts := []string{"agent-testbench case run --case " + strconv.Quote(casePath)}
	if strings.TrimSpace(item.BaseURL) != "" {
		parts = append(parts, "--base-url "+strconv.Quote(item.BaseURL))
	}
	if strings.TrimSpace(item.EvidenceDir) != "" {
		parts = append(parts, "--evidence-dir "+strconv.Quote(item.EvidenceDir))
	}
	return strings.Join(parts, " ")
}

func quoteCommandValue(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return `''`
	}
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}

func runCaseRun(ctx context.Context, args []string) error {
	flags := flag.NewFlagSet("case run", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	overrides := mapFlag{}
	casePath := flags.String("case", "", "API case file path")
	caseID := flags.String("case-id", "", "API case id from the active Store catalog")
	baseURL := flags.String("base-url", "", "Base URL for live request execution")
	evidenceDir := flags.String("evidence-dir", filepath.Join(".runtime", "cases"), "Evidence output directory")
	runID := flags.String("run-id", "", "Run id")
	storeRef := flags.String("store", "", "Named Store config or Store DSN")
	storeURL := flags.String("store-url", "", legacyStoreURLFlagHelp)
	profileID := flags.String("profile", "default", "Profile id for store records")
	timeoutSeconds := flags.Int("timeout-seconds", 0, "Request timeout in seconds for Store catalog case execution")
	dryRun := flags.Bool("dry-run", false, "Preview the file-backed case run without sending HTTP, writing Evidence, or indexing Store records")
	jsonOutput := flags.Bool("json", false, "Emit a machine-readable JSON report")
	flags.Var(&overrides, "override", "Request body override as key=value; repeat for multiple values")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *dryRun {
		if strings.TrimSpace(*caseID) != "" {
			return errors.New("case run --dry-run currently supports --case PATH")
		}
		if strings.TrimSpace(*casePath) == "" {
			return errors.New("case run --dry-run requires --case PATH")
		}
		plan, err := apicase.Plan(apicase.RunOptions{
			CasePath:    *casePath,
			BaseURL:     *baseURL,
			EvidenceDir: *evidenceDir,
			RunID:       *runID,
			Overrides:   overrides.Values(),
		})
		if err != nil {
			return err
		}
		if *jsonOutput {
			return writeIndentedJSON(plan)
		}
		printCaseRunDryRun(plan)
		return nil
	}
	resolvedStoreURL, err := resolveRequiredDailyStoreReference(*storeRef, *storeURL)
	if err != nil {
		return err
	}
	if strings.TrimSpace(*caseID) != "" {
		result, err := runStoreCatalogCase(ctx, resolvedStoreURL, *profileID, *caseID, *baseURL, *evidenceDir, *runID, *timeoutSeconds, overrides.Values())
		if err != nil {
			return err
		}
		if *jsonOutput {
			return writeIndentedJSON(result)
		}
		printStoreCatalogCaseRun(result)
		return nil
	}
	if strings.TrimSpace(*casePath) == "" {
		return errors.New("case run requires --case PATH or --case-id ID")
	}
	result, err := apicase.Run(ctx, apicase.RunOptions{
		CasePath:    *casePath,
		BaseURL:     *baseURL,
		EvidenceDir: *evidenceDir,
		RunID:       *runID,
		Overrides:   overrides.Values(),
	})
	if err != nil {
		return err
	}
	if err := indexCaseRun(ctx, resolvedStoreURL, *profileID, result); err != nil {
		return err
	}
	if *jsonOutput {
		return writeIndentedJSON(result)
	}
	fmt.Printf("Case Run: %s\n", result.RunID)
	fmt.Printf("Case: %s\n", result.CaseID)
	fmt.Printf("Status: %s\n", result.Status)
	fmt.Printf("Evidence: %s\n", result.EvidencePath)
	return nil
}

func printCaseRunDryRun(plan apicase.DryRunPlan) {
	fmt.Printf("Case Run Dry Run: %s\n", plan.RunID)
	fmt.Printf("Case: %s\n", plan.CaseID)
	fmt.Printf("Request: %s %s\n", plan.Request.Method, plan.Request.Path)
	if plan.Request.URL != "" {
		fmt.Printf("URL: %s\n", plan.Request.URL)
	}
	fmt.Printf("Headers: %d\n", len(plan.Request.HeaderKeys))
	fmt.Printf("Body: %t", plan.Request.HasBody)
	if len(plan.Request.BodyKeys) > 0 {
		fmt.Printf(" keys=%s", strings.Join(plan.Request.BodyKeys, ","))
	}
	fmt.Println()
	if len(plan.Assertions.ExpectedStatusCodes) > 0 {
		fmt.Printf("Expected Status: %s\n", intListString(plan.Assertions.ExpectedStatusCodes))
	}
	if plan.Assertions.ResponseContainsCount > 0 {
		fmt.Printf("Response Contains Checks: %d\n", plan.Assertions.ResponseContainsCount)
	}
	fmt.Printf("Will Send HTTP: %t\n", plan.Effects.HTTPRequest)
	fmt.Printf("Will Write Evidence: %t\n", plan.Effects.WritesEvidence)
	fmt.Printf("Will Write Store: %t\n", plan.Effects.WritesStore)
	fmt.Printf("Planned Evidence: %s\n", plan.Effects.PlannedEvidencePath)
	for _, warning := range plan.Warnings {
		fmt.Printf("Warning: %s\n", warning)
	}
}

func intListString(values []int) string {
	parts := make([]string, 0, len(values))
	for _, value := range values {
		parts = append(parts, strconv.Itoa(value))
	}
	return strings.Join(parts, ",")
}

func indexCaseRun(ctx context.Context, storeURL string, profileID string, result apicase.RunResult) error {
	s, err := openStore(ctx, storeURL)
	if err != nil {
		return err
	}
	defer s.Close()

	now := time.Now().UTC()
	startedAt := runResultTime(result.StartedAt, now)
	finishedAt := runResultTime(result.FinishedAt, now)
	if finishedAt.Before(startedAt) {
		finishedAt = startedAt
	}
	requestSummary, assertionSummary, err := apiCaseRunSummaries(result.EvidencePath)
	if err != nil {
		return err
	}
	if _, err := s.CreateRun(ctx, store.Run{
		ID:           result.RunID,
		ProfileID:    profileID,
		WorkflowID:   "",
		Status:       result.Status,
		EvidenceRoot: result.EvidencePath,
		SummaryJSON:  caseRunSummaryJSON(result),
		StartedAt:    startedAt,
		FinishedAt:   finishedAt,
		CreatedAt:    startedAt,
		UpdatedAt:    finishedAt,
	}); err != nil {
		return err
	}
	if _, err := s.RecordAPICaseRun(ctx, store.APICaseRun{
		ID:                   result.RunID + ".case",
		RunID:                result.RunID,
		CaseID:               result.CaseID,
		Status:               result.Status,
		RequestSummaryJSON:   requestSummary,
		AssertionSummaryJSON: assertionSummary,
		StartedAt:            startedAt,
		FinishedAt:           finishedAt,
		CreatedAt:            startedAt,
	}); err != nil {
		return err
	}
	for _, name := range []string{"case.json", "request.json", "response.json", "assertions.json", "summary.json"} {
		path := filepath.Join(result.EvidencePath, name)
		if _, err := os.Stat(path); err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			return err
		}
		summary, err := evidenceSummary(path, strings.TrimSuffix(name, ".json"))
		if err != nil {
			return err
		}
		if _, err := s.RecordEvidence(ctx, store.EvidenceRecord{
			ID:        result.RunID + "." + name,
			RunID:     result.RunID,
			CaseRunID: result.RunID + ".case",
			Kind:      strings.TrimSuffix(name, ".json"),
			URI:       path,
			MediaType: "application/json",
			Summary:   summary,
			CreatedAt: now,
		}); err != nil {
			return err
		}
	}
	return nil
}

func caseRunSummaryJSON(result apicase.RunResult) string {
	path := filepath.Join(result.EvidencePath, "summary.json")
	if raw, err := os.ReadFile(path); err == nil && json.Valid(raw) {
		return strings.TrimSpace(string(raw))
	}
	raw, err := json.Marshal(result)
	if err != nil {
		return "{}"
	}
	return string(raw)
}

func runResultTime(value string, defaultValue time.Time) time.Time {
	if strings.TrimSpace(value) == "" {
		return defaultValue
	}
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return defaultValue
	}
	return parsed.UTC()
}

type requestSummary struct {
	Method      string `json:"method"`
	Path        string `json:"path"`
	HeaderCount int    `json:"headerCount"`
	HasBody     bool   `json:"hasBody"`
}

type assertionSummary struct {
	Status     string `json:"status"`
	ErrorCount int    `json:"errorCount"`
}

type responseSummary struct {
	StatusCode  int `json:"statusCode"`
	HeaderCount int `json:"headerCount"`
	BodyBytes   int `json:"bodyBytes"`
}

func apiCaseRunSummaries(evidencePath string) (string, string, error) {
	request, err := requestSummaryJSON(filepath.Join(evidencePath, "request.json"))
	if err != nil {
		return "", "", err
	}
	assertions, err := assertionSummaryJSON(filepath.Join(evidencePath, "assertions.json"))
	if err != nil {
		return "", "", err
	}
	return request, assertions, nil
}

func requestSummaryJSON(path string) (string, error) {
	var request apicase.Request
	if err := readJSONFile(path, &request); err != nil {
		return "", err
	}
	return compactJSON(requestSummary{
		Method:      strings.ToUpper(request.Method),
		Path:        request.Path,
		HeaderCount: len(request.Headers),
		HasBody:     request.Body != nil,
	})
}

func responseSummaryJSON(path string) (string, error) {
	var response apicase.ResponseEvidence
	if err := readJSONFile(path, &response); err != nil {
		return "", err
	}
	return compactJSON(responseSummary{
		StatusCode:  response.StatusCode,
		HeaderCount: len(response.Headers),
		BodyBytes:   len([]byte(response.Body)),
	})
}

func assertionSummaryJSON(path string) (string, error) {
	var assertions apicase.AssertionEvidence
	if err := readJSONFile(path, &assertions); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return compactJSON(assertionSummary{Status: "not-run"})
		}
		return "", err
	}
	return compactJSON(assertionSummary{
		Status:     assertions.Status,
		ErrorCount: len(assertions.Errors),
	})
}
