package controlplane

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	"agent-testbench/internal/domain/profile"
	"agent-testbench/internal/runner/apicase"
	"agent-testbench/internal/store"
)

var testKitRunCounter uint64

func recordTestKitRunWithContext(ctx context.Context, bundle profile.Bundle, runtime store.Store, payload map[string]any, result map[string]any) (string, error) {
	if runtime == nil {
		return "", nil
	}
	status := store.StatusFailed
	if result["ok"] == true {
		status = store.StatusPassed
	}
	workflowID := firstNonEmpty(valueString(payload["workflowId"]), valueString(result["caseId"]))
	summary := map[string]any{
		"kind":    "apiCase",
		"summary": result["summary"],
		"steps":   []map[string]any{result},
	}
	raw, err := json.Marshal(summary)
	if err != nil {
		return "", err
	}
	now := time.Now().UTC()
	startedAt, finishedAt := testKitResultTimes(result, now)
	runID := firstNonEmpty(valueString(payload["runId"]), nextTestKitRunID(now))
	caseID := valueString(result["caseId"])
	caseResult, reused, err := takeTestKitAPICaseRunResult(payload, runID, caseID, status)
	if err != nil {
		return "", err
	}
	if reused {
		times := apiCaseRunRecordTimesFromResult(caseResult, now)
		startedAt, finishedAt = times.StartedAt, times.FinishedAt
	} else {
		caseResult, err = writeTestKitEvidenceFiles(result, status, valueString(payload[apiFieldEvidenceDir]), runID, startedAt, finishedAt)
		if err != nil {
			return "", err
		}
	}
	evidenceRoot := caseResult.EvidencePath
	result["evidenceRoot"] = evidenceRoot
	_, err = runtime.CreateRun(ctx, store.Run{
		ID:                 runID,
		ProfileID:          bundle.ID,
		EnvironmentID:      valueString(payload["environmentId"]),
		WorkflowID:         workflowID,
		Status:             status,
		EvidenceRoot:       evidenceRoot,
		SummaryJSON:        string(raw),
		TestPlanMapID:      valueString(payload["testPlanMapId"]),
		TestPlanPathID:     valueString(payload["testPlanPathId"]),
		PlannerSummaryJSON: testKitPlannerSummaryJSON(payload),
		StartedAt:          startedAt,
		FinishedAt:         finishedAt,
		CreatedAt:          startedAt,
		UpdatedAt:          finishedAt,
	})
	if err != nil {
		return "", err
	}
	if caseID == "" {
		return runID, nil
	}
	assertionSummary, err := apiCaseEvidenceSummary(filepath.Join(evidenceRoot, apiCaseEvidenceFileAssertions), apiCaseEvidenceKindAssertions, 0)
	if err != nil {
		return "", err
	}
	_, err = runtime.RecordAPICaseRun(ctx, store.APICaseRun{
		ID:                   runID + ".case",
		RunID:                runID,
		CaseID:               caseID,
		Status:               status,
		RequestSummaryJSON:   compactJSON(testKitRequestSummary(result, valueString(payload["stepId"]), caseID)),
		AssertionSummaryJSON: assertionSummary,
		TestPlanNodeID:       valueString(payload["testPlanNodeId"]),
		TestPlanOperation:    valueString(payload["testPlanOperation"]),
		PlannerSummaryJSON:   testKitPlannerSummaryJSON(payload),
		StartedAt:            startedAt,
		FinishedAt:           finishedAt,
		CreatedAt:            startedAt,
	})
	if err != nil {
		return "", err
	}
	if err := recordAPICaseEvidenceRecords(ctx, runtime, caseResult, runID+".case", valueString(payload["stepId"]), finishedAt); err != nil {
		return "", err
	}
	return runID, nil
}

func nextTestKitRunID(now time.Time) string {
	return fmt.Sprintf("%s.%d", workflowRunID(now), atomic.AddUint64(&testKitRunCounter, 1))
}

func testKitPlannerSummaryJSON(payload map[string]any) string {
	if raw := strings.TrimSpace(valueString(payload["plannerSummaryJson"])); raw != "" {
		return raw
	}
	if summary, ok := payload["plannerSummary"]; ok {
		return compactJSON(summary)
	}
	return "{}"
}

func takeTestKitAPICaseRunResult(payload map[string]any, runID string, caseID string, status string) (apicase.RunResult, bool, error) {
	value, ok := payload[testKitAPICaseRunResultKey]
	delete(payload, testKitAPICaseRunResultKey)
	if !ok {
		return apicase.RunResult{}, false, nil
	}
	result, ok := value.(apicase.RunResult)
	if !ok {
		return apicase.RunResult{}, false, fmt.Errorf("invalid in-process api case Evidence handoff")
	}
	if result.RunID != runID || result.CaseID != caseID || result.Status != status || strings.TrimSpace(result.EvidencePath) == "" {
		return apicase.RunResult{}, false, fmt.Errorf(
			"api case Evidence handoff does not match recorded run: run=%q case=%q status=%q",
			result.RunID,
			result.CaseID,
			result.Status,
		)
	}
	return result, true, nil
}

func writeTestKitEvidenceFiles(result map[string]any, status string, evidenceDir string, runID string, startedAt time.Time, finishedAt time.Time) (apicase.RunResult, error) {
	root := ""
	var err error
	if strings.TrimSpace(evidenceDir) != "" {
		root = filepath.Join(evidenceDir, runID)
		if err := os.MkdirAll(root, 0o755); err != nil {
			return apicase.RunResult{}, fmt.Errorf("create test-kit evidence directory: %w", err)
		}
	} else {
		root, err = os.MkdirTemp("", "agent-testbench-test-kit-evidence-*")
		if err != nil {
			return apicase.RunResult{}, fmt.Errorf("create test-kit evidence dir: %w", err)
		}
	}
	request := mapFromAny(mapFromAny(result["result"])["request"])
	response := mapFromAny(mapFromAny(result["result"])["response"])
	caseID := valueString(result["caseId"])
	failureReason := testKitFailureReason(result)
	failurePhase, failureCategory := testKitFailureClassification(result, status)
	assertions := map[string]any{
		"status": status,
		"passed": status == store.StatusPassed,
	}
	if failureReason != "" {
		assertions["errors"] = []string{failureReason}
	}
	caseResult := apicase.RunResult{
		OK:              status == store.StatusPassed,
		RunID:           runID,
		CaseID:          caseID,
		Status:          status,
		FailurePhase:    failurePhase,
		FailureCategory: failureCategory,
		Error:           failureReason,
		EvidencePath:    root,
		StartedAt:       startedAt.UTC().Format(time.RFC3339Nano),
		FinishedAt:      finishedAt.UTC().Format(time.RFC3339Nano),
		ElapsedMs:       finishedAt.Sub(startedAt).Milliseconds(),
		CreatedAt:       startedAt.UTC().Format(time.RFC3339Nano),
	}
	files := map[string]any{
		apiCaseEvidenceFileCase: map[string]any{
			"id":          caseID,
			apiFieldTitle: valueString(result[apiFieldTitle]),
			"request":     request,
		},
		apiCaseEvidenceFileRequest:    request,
		apiCaseEvidenceFileResponse:   response,
		apiCaseEvidenceFileAssertions: assertions,
		apiCaseEvidenceFileSummary:    caseResult,
	}
	if status == store.StatusFailed {
		files[apiCaseEvidenceFileError] = apicase.ErrorEvidence{
			Status:   status,
			Phase:    failurePhase,
			Category: failureCategory,
			Message:  failureReason,
		}
	}
	for _, name := range apiCaseEvidenceFiles() {
		payload, ok := files[name]
		if !ok {
			continue
		}
		raw, err := json.MarshalIndent(payload, "", "  ")
		if err != nil {
			return apicase.RunResult{}, err
		}
		if err := os.WriteFile(filepath.Join(root, name), append(raw, '\n'), 0o644); err != nil {
			return apicase.RunResult{}, err
		}
	}
	return caseResult, nil
}

func testKitFailureReason(result map[string]any) string {
	return firstNonEmpty(
		valueString(result["error"]),
		valueString(result["failureReason"]),
		valueString(mapFromAny(result["summary"])["failureReason"]),
	)
}

func testKitFailureClassification(result map[string]any, status string) (string, string) {
	if status == store.StatusPassed {
		return "", ""
	}
	if intValue(mapFromAny(result["summary"])["httpCode"]) > 0 {
		return "assertion", "assertion-mismatch"
	}
	return "execution", "execution-error"
}

func testKitResultTimes(result map[string]any, finishedAt time.Time) (time.Time, time.Time) {
	elapsed := intValue(result["elapsedMs"])
	if elapsed <= 0 {
		elapsed = intValue(mapFromAny(mapFromAny(result["result"])["response"])["elapsedMs"])
	}
	if elapsed <= 0 {
		elapsed = 10
	}
	return finishedAt.Add(-time.Duration(elapsed) * time.Millisecond), finishedAt
}
