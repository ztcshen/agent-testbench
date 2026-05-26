package main

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"agent-testbench/internal/runner/apicase"
	"agent-testbench/internal/store"
)

type caseDiagnosisReport struct {
	OK              bool                  `json:"ok"`
	CaseRunID       string                `json:"caseRunId"`
	RunID           string                `json:"runId"`
	CaseID          string                `json:"caseId"`
	Status          string                `json:"status"`
	Operation       string                `json:"operation,omitempty"`
	Category        string                `json:"category"`
	PrimaryFinding  string                `json:"primaryFinding"`
	EvidencePath    string                `json:"evidencePath,omitempty"`
	AssertionErrors []string              `json:"assertionErrors"`
	Signals         []caseDiagnosisSignal `json:"signals"`
	NextActions     []string              `json:"nextActions"`
	Warnings        []string              `json:"warnings"`
}

type caseDiagnosisSignal struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type caseDiagnosisArtifacts struct {
	AssertionErrors []string
	HTTPStatus      int
	Warnings        []string
}

func runCaseDiagnose(ctx context.Context, args []string) error {
	selection := newCaseEvidenceCLIFlags("case diagnose")
	if err := selection.parse(args); err != nil {
		return err
	}
	runtime, cleanup, err := selection.openStore(ctx)
	if err != nil {
		return err
	}
	defer cleanup()
	report, err := diagnoseCaseEvidence(ctx, runtime, *selection.caseRunID, *selection.runID, *selection.caseID, *selection.stepID)
	if err != nil {
		return err
	}
	if *selection.json {
		return writeIndentedJSON(report)
	}
	printCaseDiagnosis(report)
	return nil
}

func diagnoseCaseEvidence(ctx context.Context, runtime store.Store, caseRunID string, runID string, caseID string, stepID string) (caseDiagnosisReport, error) {
	payload, err := readCaseEvidence(ctx, runtime, caseRunID, runID, caseID, stepID)
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
		Operation:       firstNonEmpty(valueString(summary["operation"]), caseRunOperationFromRequest(request, valueString(summary["case_id"]))),
		EvidencePath:    valueString(summary["evidence_path"]),
		AssertionErrors: []string{},
		Signals:         []caseDiagnosisSignal{},
		NextActions:     []string{},
		Warnings:        []string{},
	}
	report.OK = strings.EqualFold(report.Status, store.StatusPassed)

	artifacts, err := readCaseDiagnosisArtifacts(ctx, runtime, report.RunID, report.CaseRunID)
	if err != nil {
		return caseDiagnosisReport{}, err
	}
	report.AssertionErrors = artifacts.AssertionErrors
	report.Warnings = append(report.Warnings, artifacts.Warnings...)
	httpStatus := firstPositiveInt(artifacts.HTTPStatus, intFromReportAny(response["http_code"]), intFromReportAny(summary["actual_http_code"]))
	assertionStatus := valueString(assertions["status"])
	errorCount := firstPositiveInt(len(report.AssertionErrors), intFromReportAny(assertions["errorCount"]))

	report.Category = caseDiagnosisCategory(report.Status, assertionStatus, errorCount, httpStatus)
	report.PrimaryFinding = caseDiagnosisPrimaryFinding(report.Category, report.AssertionErrors, httpStatus, report.Status)
	report.Signals = caseDiagnosisSignals(report, assertionStatus, errorCount, httpStatus)
	report.NextActions = caseDiagnosisNextActions(report, httpStatus, errorCount)
	return report, nil
}

func readCaseDiagnosisArtifacts(ctx context.Context, runtime store.Store, runID string, caseRunID string) (caseDiagnosisArtifacts, error) {
	out := caseDiagnosisArtifacts{AssertionErrors: []string{}, Warnings: []string{}}
	if strings.TrimSpace(runID) == "" || strings.TrimSpace(caseRunID) == "" {
		out.Warnings = append(out.Warnings, "case run evidence identity is incomplete")
		return out, nil
	}
	records, err := runtime.ListEvidence(ctx, runID)
	if err != nil {
		return caseDiagnosisArtifacts{}, err
	}
	for _, record := range records {
		if record.CaseRunID != caseRunID {
			continue
		}
		switch strings.ToLower(strings.TrimSpace(record.Kind)) {
		case "assertions":
			var assertions apicase.AssertionEvidence
			if err := readJSONFile(record.URI, &assertions); err != nil {
				out.Warnings = append(out.Warnings, "could not read assertions evidence: "+err.Error())
				continue
			}
			out.AssertionErrors = append(out.AssertionErrors, assertions.Errors...)
		case "response":
			var response apicase.ResponseEvidence
			if err := readJSONFile(record.URI, &response); err != nil {
				out.Warnings = append(out.Warnings, "could not read response evidence: "+err.Error())
				continue
			}
			out.HTTPStatus = response.StatusCode
		}
	}
	return out, nil
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

func caseDiagnosisNextActions(report caseDiagnosisReport, httpStatus int, errorCount int) []string {
	actions := []string{}
	if report.CaseRunID != "" {
		actions = append(actions, "agent-testbench case evidence --case-run "+report.CaseRunID+" --json")
	}
	if errorCount > 0 {
		actions = append(actions, "Inspect request.json, response.json, and assertions.json under "+firstNonEmpty(report.EvidencePath, "the Evidence directory"))
	}
	if httpStatus >= 400 {
		actions = append(actions, "Compare the planned request with the target service contract and expected status codes")
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
