package controlplane

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"agent-testbench/internal/domain/profile"
	"agent-testbench/internal/store"
)

func recordTestKitRun(r *http.Request, bundle profile.Bundle, runtime store.Store, payload map[string]any, result map[string]any) (string, error) {
	return recordTestKitRunWithContext(r.Context(), bundle, runtime, payload, result)
}

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
	runID := firstNonEmpty(valueString(payload["runId"]), workflowRunID(now))
	evidenceRoot, err := writeTestKitEvidenceFiles(result, status, valueString(payload["evidenceDir"]), runID)
	if err != nil {
		return "", err
	}
	_, err = runtime.CreateRun(ctx, store.Run{
		ID:            runID,
		ProfileID:     bundle.ID,
		EnvironmentID: valueString(payload["environmentId"]),
		WorkflowID:    workflowID,
		Status:        status,
		EvidenceRoot:  evidenceRoot,
		SummaryJSON:   string(raw),
		StartedAt:     startedAt,
		FinishedAt:    finishedAt,
		CreatedAt:     startedAt,
		UpdatedAt:     finishedAt,
	})
	if err != nil {
		return "", err
	}
	caseID := valueString(result["caseId"])
	if caseID == "" {
		return runID, nil
	}
	_, err = runtime.RecordAPICaseRun(ctx, store.APICaseRun{
		ID:                   runID + ".case",
		RunID:                runID,
		CaseID:               caseID,
		Status:               status,
		RequestSummaryJSON:   compactJSON(testKitRequestSummary(result, valueString(payload["stepId"]), caseID)),
		AssertionSummaryJSON: compactJSON(map[string]any{"status": status}),
		StartedAt:            startedAt,
		FinishedAt:           finishedAt,
		CreatedAt:            startedAt,
	})
	if err != nil {
		return "", err
	}
	if err := recordTestKitEvidence(ctx, runtime, runID, runID+".case", valueString(payload["stepId"]), caseID, evidenceRoot, finishedAt); err != nil {
		return "", err
	}
	return runID, nil
}

func writeTestKitEvidenceFiles(result map[string]any, status string, evidenceDir string, runID string) (string, error) {
	root := ""
	var err error
	if strings.TrimSpace(evidenceDir) != "" {
		root = filepath.Join(evidenceDir, runID)
		if err := os.MkdirAll(root, 0o755); err != nil {
			return "", fmt.Errorf("create test-kit evidence directory: %w", err)
		}
	} else {
		root, err = os.MkdirTemp("", "agent-testbench-test-kit-evidence-*")
		if err != nil {
			return "", fmt.Errorf("create test-kit evidence dir: %w", err)
		}
	}
	request := mapFromAny(mapFromAny(result["result"])["request"])
	response := mapFromAny(mapFromAny(result["result"])["response"])
	assertions := map[string]any{
		"status": status,
		"passed": status == store.StatusPassed,
	}
	if reason := strings.TrimSpace(valueString(result["failureReason"])); reason != "" {
		assertions["errors"] = []string{reason}
	}
	for name, payload := range map[string]map[string]any{
		"request.json":    request,
		"response.json":   response,
		"assertions.json": assertions,
	} {
		raw, err := json.MarshalIndent(payload, "", "  ")
		if err != nil {
			return "", err
		}
		if err := os.WriteFile(filepath.Join(root, name), append(raw, '\n'), 0o644); err != nil {
			return "", err
		}
	}
	return root, nil
}

func recordTestKitEvidence(ctx context.Context, runtime store.Store, runID string, caseRunID string, stepID string, caseID string, evidenceRoot string, createdAt time.Time) error {
	for _, name := range []string{"request.json", "response.json", "assertions.json"} {
		path := filepath.Join(evidenceRoot, name)
		info, err := os.Stat(path)
		if err != nil {
			return err
		}
		kind := strings.TrimSuffix(name, ".json")
		summary, err := apiCaseEvidenceSummary(path, kind, info.Size())
		if err != nil {
			return err
		}
		labels := map[string]any{
			"caseId": caseID,
			"kind":   kind,
			"runId":  runID,
		}
		if strings.TrimSpace(stepID) != "" {
			labels["stepId"] = stepID
		}
		if _, err := runtime.RecordEvidence(ctx, store.EvidenceRecord{
			ID:         runID + "." + name,
			RunID:      runID,
			CaseRunID:  caseRunID,
			StepID:     stepID,
			Kind:       kind,
			URI:        path,
			MediaType:  "application/json",
			SizeBytes:  info.Size(),
			Summary:    summary,
			Category:   apiCaseEvidenceCategory(kind),
			Visibility: "public",
			LabelsJSON: compactJSON(labels),
			CreatedAt:  createdAt,
		}); err != nil {
			return err
		}
	}
	return nil
}

func testKitResultTimes(result map[string]any, finishedAt time.Time) (time.Time, time.Time) {
	elapsed := intValue(result["elapsedMs"])
	if elapsed <= 0 {
		elapsed = intValue(mapFromAny(mapFromAny(result["result"])["response"])["elapsedMs"])
	}
	if elapsed <= 0 {
		return finishedAt, finishedAt
	}
	return finishedAt.Add(-time.Duration(elapsed) * time.Millisecond), finishedAt
}
