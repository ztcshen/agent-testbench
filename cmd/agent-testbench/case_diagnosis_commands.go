package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"agent-testbench/internal/runner/apicase"
	"agent-testbench/internal/server/controlplane"
	"agent-testbench/internal/store"
)

type caseDiagnosisReport struct {
	OK              bool                     `json:"ok"`
	CaseRunID       string                   `json:"caseRunId"`
	RunID           string                   `json:"runId"`
	CaseID          string                   `json:"caseId"`
	Status          string                   `json:"status"`
	Operation       string                   `json:"operation,omitempty"`
	Category        string                   `json:"category"`
	StepID          string                   `json:"stepId,omitempty"`
	PrimaryFinding  string                   `json:"primaryFinding"`
	EvidencePath    string                   `json:"evidencePath,omitempty"`
	AssertionErrors []string                 `json:"assertionErrors"`
	Diagnostics     caseDiagnosisDiagnostics `json:"diagnostics"`
	Signals         []caseDiagnosisSignal    `json:"signals"`
	NextActions     []string                 `json:"nextActions"`
	Warnings        []string                 `json:"warnings"`
}

type caseDiagnosisSignal struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type caseDiagnosisArtifacts struct {
	AssertionErrors      []string
	HTTPStatus           int
	Warnings             []string
	MissingLocalEvidence []string
}

type caseDiagnosisDiagnostics struct {
	LogRecords             int `json:"logRecords"`
	RuntimeLogSystems      int `json:"runtimeLogSystems"`
	RuntimeLogMatched      int `json:"runtimeLogMatched"`
	DependencyProbes       int `json:"dependencyProbes"`
	PostProcessTasks       int `json:"postProcessTasks"`
	FailedPostProcessTasks int `json:"failedPostProcessTasks"`
}

const caseDiagnosisEvidenceKindAssertions = "assertions"
const caseDiagnosisRuntimeLogTaskKind = "runtime_log_collect"

func runCaseDiagnose(ctx context.Context, args []string) error {
	return runCaseEvidenceReport(ctx, args, "case diagnose", diagnoseCaseEvidence, printCaseDiagnosis)
}

func diagnoseCaseEvidence(ctx context.Context, runtime store.Store, caseRunID string, runID string, caseID string, stepID string) (caseDiagnosisReport, error) {
	payload, err := readCaseEvidence(ctx, runtime, caseRunID, runID, caseID, stepID)
	if errors.Is(err, controlplane.ErrCaseEvidenceNotFound) {
		return diagnoseMissingCaseEvidence(ctx, runtime, caseRunID, runID, caseID, stepID), nil
	}
	if err != nil {
		return caseDiagnosisReport{}, err
	}
	evidence := mapFromReportAny(payload["evidence"])
	summary := mapFromReportAny(evidence["summary"])
	assertions := mapFromReportAny(evidence["assertions"])
	response := mapFromReportAny(evidence["response"])
	request := mapFromReportAny(evidence["request"])

	report := caseDiagnosisReport{
		CaseRunID:       valueString(summary["case_run_id"]),
		RunID:           valueString(summary["run_id"]),
		CaseID:          valueString(summary["case_id"]),
		Status:          valueString(summary["status"]),
		StepID:          valueString(summary["step_id"]),
		Operation:       firstNonEmpty(valueString(summary["operation"]), caseRunOperationFromRequest(request, valueString(summary["case_id"]))),
		EvidencePath:    valueString(summary["evidence_path"]),
		AssertionErrors: []string{},
		Signals:         []caseDiagnosisSignal{},
		NextActions:     []string{},
		Warnings:        []string{},
	}
	report.OK = strings.EqualFold(report.Status, store.StatusPassed)
	if !report.OK && len(listFromReportAny(evidence["logs"])) == 0 {
		if ensure, ensureErr := controlplane.EnsureCaseRuntimeLogs(ctx, runtime, report.RunID, report.CaseID, report.StepID); ensureErr == nil && (ensure.Collected || ensure.Cached) {
			if refreshed, readErr := readCaseEvidence(ctx, runtime, caseRunID, runID, caseID, stepID); readErr == nil {
				payload = refreshed
				evidence = mapFromReportAny(payload["evidence"])
				summary = mapFromReportAny(evidence["summary"])
				report.StepID = firstNonEmpty(valueString(summary["step_id"]), report.StepID, ensure.StepID)
			}
		} else if ensureErr != nil {
			report.Warnings = append(report.Warnings, "runtime log collection failed: "+ensureErr.Error())
		}
	}

	artifacts, err := readCaseDiagnosisArtifacts(ctx, runtime, report.RunID, report.CaseRunID)
	if err != nil {
		return caseDiagnosisReport{}, err
	}
	report.AssertionErrors = artifacts.AssertionErrors
	report.Warnings = append(report.Warnings, artifacts.Warnings...)
	httpStatus := firstPositiveInt(artifacts.HTTPStatus, intFromReportAny(response["http_code"]), intFromReportAny(summary["actual_http_code"]))
	assertionStatus := valueString(assertions["status"])
	errorCount := firstPositiveInt(len(report.AssertionErrors), intFromReportAny(assertions["errorCount"]))
	report.Diagnostics = caseDiagnosisDiagnosticsForEvidence(ctx, runtime, report, evidence)

	report.Category = caseDiagnosisCategory(report.Status, assertionStatus, errorCount, httpStatus)
	report.PrimaryFinding = caseDiagnosisPrimaryFinding(report.Category, report.AssertionErrors, httpStatus, report.Status)
	report.Signals = caseDiagnosisSignals(report, assertionStatus, errorCount, httpStatus)
	report.Signals = append(report.Signals, caseDiagnosisDiagnosticSignals(report.Diagnostics)...)
	report.Warnings = append(report.Warnings, caseDiagnosisDiagnosticWarnings(report, report.Diagnostics)...)
	report.NextActions = caseDiagnosisNextActions(report, httpStatus, errorCount, artifacts.MissingLocalEvidence, report.Diagnostics)
	return report, nil
}

func diagnoseMissingCaseEvidence(ctx context.Context, runtime store.Store, caseRunID string, runID string, caseID string, stepID string) caseDiagnosisReport {
	runID = strings.TrimSpace(runID)
	caseID = strings.TrimSpace(caseID)
	stepID = strings.TrimSpace(stepID)
	status := "no-evidence"
	if runID != "" {
		if run, err := runtime.GetRun(ctx, runID); err == nil {
			status = firstNonEmpty(run.Status, status)
			if caseID == "" && stepID != "" {
				caseID = caseIDForWorkflowStepSummary(run.SummaryJSON, stepID)
			}
			if stepID == "" && caseID != "" {
				stepID = stepIDForCaseInWorkflowSummary(run.SummaryJSON, caseID)
			}
		}
	}
	report := caseDiagnosisReport{
		OK:             false,
		CaseRunID:      strings.TrimSpace(caseRunID),
		RunID:          runID,
		CaseID:         caseID,
		Status:         status,
		StepID:         stepID,
		Category:       "no-evidence",
		PrimaryFinding: "no case evidence is persisted for the selected run",
		Diagnostics:    caseDiagnosisDiagnostics{},
		Signals: []caseDiagnosisSignal{
			{Name: "evidence.case_runs", Value: "0"},
		},
		NextActions: []string{
			"agent-testbench case batch report --run " + firstNonEmpty(runID, "<RUN_ID>") + " --json",
			"rerun the failed case with evidence capture enabled before diagnosing request/response details",
		},
		Warnings: []string{"no persisted case evidence matched the selected run/case/step"},
	}
	return report
}

func caseIDForWorkflowStepSummary(summaryJSON string, stepID string) string {
	summary := jsonObjectString(summaryJSON)
	for _, raw := range listFromReportAny(summary["steps"]) {
		step, ok := raw.(map[string]any)
		if !ok || strings.TrimSpace(valueString(step["stepId"])) != stepID {
			continue
		}
		return strings.TrimSpace(valueString(step["caseId"]))
	}
	return ""
}

func stepIDForCaseInWorkflowSummary(summaryJSON string, caseID string) string {
	summary := jsonObjectString(summaryJSON)
	for _, raw := range listFromReportAny(summary["steps"]) {
		step, ok := raw.(map[string]any)
		if !ok || strings.TrimSpace(valueString(step["caseId"])) != caseID {
			continue
		}
		return strings.TrimSpace(valueString(step["stepId"]))
	}
	return ""
}

func readCaseDiagnosisArtifacts(ctx context.Context, runtime store.Store, runID string, caseRunID string) (caseDiagnosisArtifacts, error) {
	out := caseDiagnosisArtifacts{AssertionErrors: []string{}, Warnings: []string{}, MissingLocalEvidence: []string{}}
	if strings.TrimSpace(runID) == "" || strings.TrimSpace(caseRunID) == "" {
		out.Warnings = append(out.Warnings, "case run evidence identity is incomplete")
		return out, nil
	}
	records, err := runtime.ListEvidence(ctx, runID)
	if err != nil {
		return caseDiagnosisArtifacts{}, err
	}
	evidenceRoot := ""
	if run, err := runtime.GetRun(ctx, runID); err == nil {
		evidenceRoot = run.EvidenceRoot
	}
	for _, record := range records {
		if record.CaseRunID != caseRunID {
			continue
		}
		kind := strings.ToLower(strings.TrimSpace(record.Kind))
		readPath := record.URI
		switch kind {
		case "request", "response", caseDiagnosisEvidenceKindAssertions:
			if path, missing, unreadable := caseDiagnosisLocalEvidenceState(record.URI, evidenceRoot); missing {
				out.Warnings = append(out.Warnings, "local "+kind+" evidence file is missing: "+path)
				out.MissingLocalEvidence = append(out.MissingLocalEvidence, path)
				continue
			} else if unreadable != "" {
				out.Warnings = append(out.Warnings, "local "+kind+" evidence file is unreadable: "+unreadable)
				continue
			} else if path != "" {
				readPath = path
			}
		default:
			continue
		}
		switch kind {
		case caseDiagnosisEvidenceKindAssertions:
			var assertions apicase.AssertionEvidence
			if err := readJSONFile(readPath, &assertions); err != nil {
				out.Warnings = append(out.Warnings, "could not read assertions evidence: "+err.Error())
				continue
			}
			out.AssertionErrors = append(out.AssertionErrors, assertions.Errors...)
		case "response":
			var response apicase.ResponseEvidence
			if err := readJSONFile(readPath, &response); err != nil {
				out.Warnings = append(out.Warnings, "could not read response evidence: "+err.Error())
				continue
			}
			out.HTTPStatus = response.StatusCode
		}
	}
	return out, nil
}

func caseDiagnosisLocalEvidenceState(uri string, evidenceRoot string) (path string, missing bool, unreadable string) {
	uri = strings.TrimSpace(uri)
	if uri == "" || !caseDiagnosisURIIsLocalFile(uri) {
		return "", false, ""
	}
	path = caseDiagnosisLocalEvidencePath(uri, evidenceRoot)
	if _, err := os.Stat(path); err == nil {
		return path, false, ""
	} else if errors.Is(err, os.ErrNotExist) {
		return path, true, ""
	} else {
		return path, false, err.Error()
	}
}

func caseDiagnosisURIIsLocalFile(uri string) bool {
	return strings.HasPrefix(uri, "file://") || !strings.Contains(uri, "://")
}

func caseDiagnosisLocalEvidencePath(uri string, evidenceRoot string) string {
	path := filepath.Clean(strings.TrimPrefix(strings.TrimSpace(uri), "file://"))
	if filepath.IsAbs(path) {
		return path
	}
	root := strings.TrimSpace(evidenceRoot)
	if root == "" {
		return path
	}
	root = filepath.Clean(strings.TrimPrefix(root, "file://"))
	if root == "." || root == "" {
		return path
	}
	if path == root || strings.HasPrefix(path, root+string(os.PathSeparator)) {
		return path
	}
	return filepath.Join(root, path)
}

func caseDiagnosisCategory(status string, assertionStatus string, errorCount int, httpStatus int) string {
	if strings.EqualFold(status, store.StatusPassed) {
		return "passed"
	}
	if strings.EqualFold(assertionStatus, store.StatusFailed) || errorCount > 0 {
		return "assertion-mismatch"
	}
	if httpStatus >= 500 {
		return "server-error"
	}
	if httpStatus >= 400 {
		return "client-error"
	}
	if httpStatus == 0 {
		return "missing-response-evidence"
	}
	return "case-failure"
}

func caseDiagnosisPrimaryFinding(category string, assertionErrors []string, httpStatus int, status string) string {
	if len(assertionErrors) > 0 {
		return "Assertion mismatch: " + assertionErrors[0]
	}
	switch category {
	case "passed":
		return "Case run passed"
	case "server-error":
		return fmt.Sprintf("Target returned HTTP %d", httpStatus)
	case "client-error":
		return fmt.Sprintf("Target rejected the request with HTTP %d", httpStatus)
	case "missing-response-evidence":
		return "Response evidence is missing"
	default:
		return "Case run finished with status " + firstNonEmpty(status, "unknown")
	}
}

func caseDiagnosisSignals(report caseDiagnosisReport, assertionStatus string, errorCount int, httpStatus int) []caseDiagnosisSignal {
	signals := []caseDiagnosisSignal{
		{Name: "case.status", Value: report.Status},
	}
	if report.Operation != "" {
		signals = append(signals, caseDiagnosisSignal{Name: "operation", Value: report.Operation})
	}
	if httpStatus > 0 {
		signals = append(signals, caseDiagnosisSignal{Name: "http.status", Value: strconv.Itoa(httpStatus)})
	}
	if assertionStatus != "" {
		signals = append(signals, caseDiagnosisSignal{Name: "assertion.status", Value: assertionStatus})
	}
	if errorCount > 0 {
		signals = append(signals, caseDiagnosisSignal{Name: "assertion.error_count", Value: strconv.Itoa(errorCount)})
	}
	return signals
}

func caseDiagnosisDiagnosticsForEvidence(ctx context.Context, runtime store.Store, report caseDiagnosisReport, evidence map[string]any) caseDiagnosisDiagnostics {
	diagnostics := caseDiagnosisDiagnostics{}
	logs := listFromReportAny(evidence["logs"])
	diagnostics.LogRecords = len(logs)
	for _, raw := range logs {
		log := mapFromReportAny(raw)
		systems := listFromReportAny(log["systems"])
		diagnostics.RuntimeLogSystems += len(systems)
		for _, rawSystem := range systems {
			system := mapFromReportAny(rawSystem)
			if found, ok := system["found"].(bool); ok && found {
				diagnostics.RuntimeLogMatched++
			}
		}
	}
	mysql := mapFromReportAny(evidence["mysql"])
	diagnostics.DependencyProbes += len(listFromReportAny(mysql["queries"]))
	if runtime != nil && strings.TrimSpace(report.RunID) != "" {
		rows, err := runtime.ListPostProcessTasks(ctx, report.RunID)
		if err == nil {
			for _, row := range rows {
				if !caseDiagnosisPostProcessTaskMatches(row, report) {
					continue
				}
				diagnostics.PostProcessTasks++
				if row.Status == store.StatusFailed {
					diagnostics.FailedPostProcessTasks++
				}
			}
		}
	}
	return diagnostics
}

func caseDiagnosisPostProcessTaskMatches(row store.PostProcessTask, report caseDiagnosisReport) bool {
	if strings.TrimSpace(row.RunID) != strings.TrimSpace(report.RunID) {
		return false
	}
	if report.StepID != "" && row.StepID != "" && row.StepID != report.StepID {
		return false
	}
	if report.CaseID != "" && row.CaseID != "" && row.CaseID != report.CaseID {
		return false
	}
	return true
}

func caseDiagnosisDiagnosticSignals(diagnostics caseDiagnosisDiagnostics) []caseDiagnosisSignal {
	signals := []caseDiagnosisSignal{
		{Name: "evidence.logs", Value: strconv.Itoa(diagnostics.LogRecords)},
		{Name: "runtime_log.systems", Value: strconv.Itoa(diagnostics.RuntimeLogSystems)},
		{Name: "runtime_log.matched_systems", Value: strconv.Itoa(diagnostics.RuntimeLogMatched)},
		{Name: "dependency.probes", Value: strconv.Itoa(diagnostics.DependencyProbes)},
		{Name: "post_process.tasks", Value: strconv.Itoa(diagnostics.PostProcessTasks)},
	}
	if diagnostics.FailedPostProcessTasks > 0 {
		signals = append(signals, caseDiagnosisSignal{Name: "post_process.failed", Value: strconv.Itoa(diagnostics.FailedPostProcessTasks)})
	}
	return signals
}

func caseDiagnosisDiagnosticWarnings(report caseDiagnosisReport, diagnostics caseDiagnosisDiagnostics) []string {
	if report.OK {
		return nil
	}
	warnings := []string{}
	if diagnostics.LogRecords == 0 {
		warnings = append(warnings, "no runtime log evidence is attached to this failed case or workflow step")
	}
	if diagnostics.DependencyProbes == 0 {
		warnings = append(warnings, "no dependency probe evidence is attached to this failed case; add post-run SQL, message, or custom probes for dependency-send failures")
	}
	return warnings
}

func caseDiagnosisNextActions(report caseDiagnosisReport, httpStatus int, errorCount int, missingLocalEvidence []string, diagnostics caseDiagnosisDiagnostics) []string {
	actions := []string{}
	if report.CaseRunID != "" {
		actions = append(actions, "agent-testbench case evidence --case-run "+report.CaseRunID+" --json")
	}
	if !report.OK && diagnostics.LogRecords == 0 && report.RunID != "" {
		if report.StepID != "" {
			actions = append(actions, "agent-testbench workflow step --run "+report.RunID+" --step "+report.StepID+" --json")
			actions = append(actions, "agent-testbench evidence tasks --run "+report.RunID+" --step "+report.StepID+" --kind "+caseDiagnosisRuntimeLogTaskKind+" --json")
		} else {
			actions = append(actions, "agent-testbench evidence tasks --run "+report.RunID+" --kind "+caseDiagnosisRuntimeLogTaskKind+" --json")
		}
	}
	if len(missingLocalEvidence) > 0 {
		actions = append(actions, "Rerun the case with --evidence-dir pointing to a durable directory, or copy/export local Evidence before temporary files are cleaned up; missing: "+strings.Join(missingLocalEvidence, ", "))
	}
	if errorCount > 0 {
		actions = append(actions, "Inspect request.json, response.json, and assertions.json under "+firstNonEmpty(report.EvidencePath, "the Evidence directory"))
	}
	if httpStatus >= 400 {
		actions = append(actions, "Compare the planned request with the target service contract and expected status codes")
	}
	if !report.OK && diagnostics.DependencyProbes == 0 {
		actions = append(actions, "Add Store-backed dependency probes for this case, then rerun and inspect agent-testbench evidence list --run "+report.RunID+" --json")
	}
	if len(actions) == 0 {
		actions = append(actions, "No failure action needed")
	}
	return actions
}

func printCaseDiagnosis(report caseDiagnosisReport) {
	fmt.Println("Case Diagnosis")
	fmt.Printf("Case Run: %s\n", report.CaseRunID)
	fmt.Printf("Case: %s\n", report.CaseID)
	fmt.Printf("Status: %s\n", report.Status)
	fmt.Printf("Category: %s\n", report.Category)
	fmt.Printf("Finding: %s\n", report.PrimaryFinding)
	for _, signal := range report.Signals {
		fmt.Printf("Signal: %s=%s\n", signal.Name, signal.Value)
	}
	for _, action := range report.NextActions {
		fmt.Printf("Next: %s\n", action)
	}
	for _, warning := range report.Warnings {
		fmt.Printf("Warning: %s\n", warning)
	}
}
