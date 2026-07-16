package controlplane

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"agent-testbench/internal/domain/profile"
	"agent-testbench/internal/store"
)

func TestAPICaseBatchMaterializationEvidenceWriteFailureStopsBatchAsPersistenceError(t *testing.T) {
	evidenceRoot := filepath.Join(t.TempDir(), "occupied-evidence-root")
	if err := os.WriteFile(evidenceRoot, []byte("occupied"), 0o644); err != nil {
		t.Fatalf("write occupied Evidence root: %v", err)
	}
	reportDir := filepath.Join(t.TempDir(), "batch-report")
	batchRunID := "batch.materialization.evidence-write-failure"
	plans := []apiCaseBatchCasePlan{
		{
			ID:          "case.materialization.evidence-write-failure",
			DisplayName: "Evidence write failure",
			NodeID:      "node.materialization.missing-runtime",
			Method:      "POST",
			Path:        "/v1/materialize",
			EvidenceDir: evidenceRoot,
			Execution: &caseExecutionConfig{
				Method: "POST",
				NodeID: "node.materialization.missing-runtime",
				Path:   "/v1/materialize",
				Body:   map[string]any{"value": "sample"},
			},
		},
		{ID: "case.must-not-run", DisplayName: "Must not run"},
	}
	runner := newAPICaseBatchRunner(0)
	runner.save(apiCaseBatchRunReport{
		OK:                   true,
		BatchRunID:           batchRunID,
		RequestID:            "materialization-evidence-write-failure",
		ProfileID:            "sample",
		Status:               store.StatusRunning,
		Total:                len(plans),
		StartedAt:            time.Now().UTC().Format(time.RFC3339Nano),
		HTMLReportPath:       filepath.Join(reportDir, "report.html"),
		JUnitReportPath:      filepath.Join(reportDir, "report.junit.xml"),
		ArtifactManifestPath: filepath.Join(reportDir, "artifacts.json"),
		FailureSummaryPath:   filepath.Join(reportDir, "failures.json"),
		Cases: []apiCaseBatchCaseReport{
			{CaseID: plans[0].ID, Status: store.StatusRunning},
			{CaseID: plans[1].ID, Status: store.StatusRunning},
		},
	})

	runner.run(context.Background(), batchRunID, profile.Bundle{ID: "sample"}, "", "", plans, nil, nil, traceCollector{})
	report, ok := runner.get(batchRunID)
	if !ok {
		t.Fatal("materialization Evidence write failure report was removed")
	}
	if report.Status != store.StatusFailed || report.FailureCategory != apiCaseBatchPersistenceFailureCategory || !strings.Contains(report.Error, "create failure evidence directory") {
		t.Fatalf("materialization Evidence write failure batch = %#v", report)
	}
	failed := report.Cases[0]
	if failed.Status != store.StatusFailed || failed.FailurePhase != apiCaseBatchPersistenceFailurePhase || failed.FailureCategory != apiCaseBatchPersistenceFailureCategory {
		t.Fatalf("materialization Evidence write failure case = %#v", failed)
	}
	if failed.RunID == "" || failed.CaseRunID == "" || failed.EvidencePath == "" || failed.DetailURL == "" {
		t.Fatalf("materialization Evidence write failure handles = %#v", failed)
	}
	if report.Cases[1].Status != store.StatusSkipped || report.Cases[1].FailureCategory != apiCaseBatchPersistenceFailureCategory {
		t.Fatalf("remaining case should be skipped after persistence failure: %#v", report.Cases[1])
	}
}
