package apicase_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"agent-testbench/internal/runner/apicase"
)

func TestWriteFailureEvidenceCreatesCanonicalMaterializationFailure(t *testing.T) {
	started := time.Date(2026, 7, 16, 8, 30, 0, 0, time.UTC)
	result, err := apicase.WriteFailureEvidence(apicase.FailureEvidenceOptions{
		RunID:       "run.materialization.failure",
		EvidenceDir: t.TempDir(),
		Case: apicase.Case{
			ID:    "case.materialization.failure",
			Title: "Materialization failure",
			Request: apicase.Request{
				Method: "POST",
				Path:   "/v1/materialize",
			},
		},
		Phase:     apicase.FailurePhaseMaterialization,
		Category:  apicase.FailureCategoryMaterialization,
		Message:   "service runtime is unavailable",
		StartedAt: started,
	})
	if err != nil {
		t.Fatalf("write materialization failure Evidence: %v", err)
	}
	if result.OK || result.Status != "failed" || result.RunID != "run.materialization.failure" || result.CaseID != "case.materialization.failure" {
		t.Fatalf("materialization failure result = %#v", result)
	}
	if result.FailurePhase != "materialization" || result.FailureCategory != "materialization-error" || result.Error != "service runtime is unavailable" {
		t.Fatalf("materialization failure details = %#v", result)
	}
	if result.StartedAt != started.Format(time.RFC3339Nano) || result.FinishedAt == "" {
		t.Fatalf("materialization failure timing = %#v", result)
	}
	for _, name := range []string{"case.json", "request.json", "assertions.json", "error.json", "summary.json"} {
		if _, err := os.Stat(filepath.Join(result.EvidencePath, name)); err != nil {
			t.Fatalf("missing %s: %v", name, err)
		}
	}

	var request apicase.Request
	readJSONFile(t, filepath.Join(result.EvidencePath, "request.json"), &request)
	if request.Method != "POST" || request.Path != "/v1/materialize" {
		t.Fatalf("materialization request Evidence = %#v", request)
	}
	var assertions apicase.AssertionEvidence
	readJSONFile(t, filepath.Join(result.EvidencePath, "assertions.json"), &assertions)
	if assertions.Status != "failed" || len(assertions.Errors) != 1 || assertions.Errors[0] != result.Error {
		t.Fatalf("materialization assertion Evidence = %#v", assertions)
	}
	var errorEvidence apicase.ErrorEvidence
	readJSONFile(t, filepath.Join(result.EvidencePath, "error.json"), &errorEvidence)
	if errorEvidence.Phase != result.FailurePhase || errorEvidence.Category != result.FailureCategory || errorEvidence.Message != result.Error {
		t.Fatalf("materialization error Evidence = %#v", errorEvidence)
	}
}

func TestWriteFailureEvidenceReturnsEvidenceStorageError(t *testing.T) {
	root := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(root, []byte("occupied"), 0o644); err != nil {
		t.Fatalf("write occupied Evidence root: %v", err)
	}
	result, err := apicase.WriteFailureEvidence(apicase.FailureEvidenceOptions{
		RunID:       "run.materialization.storage-failure",
		EvidenceDir: root,
		Case: apicase.Case{
			ID:      "case.materialization.storage-failure",
			Request: apicase.Request{Method: "GET", Path: "/"},
		},
		Phase:    apicase.FailurePhaseMaterialization,
		Category: apicase.FailureCategoryMaterialization,
		Message:  "materialization failed",
	})
	if err == nil || !strings.Contains(err.Error(), "create failure evidence directory") {
		t.Fatalf("failure Evidence storage error = %v", err)
	}
	if result.RunID == "" || result.CaseID == "" || result.EvidencePath == "" {
		t.Fatalf("partial failed result should retain handles: %#v", result)
	}
	if result.FailurePhase != apicase.FailurePhasePersistence || result.FailureCategory != apicase.FailureCategoryEvidenceWrite {
		t.Fatalf("failure Evidence storage classification = %#v", result)
	}
}

func TestWriteFailureEvidenceRejectsUnsafeExplicitRunIDBeforeWriting(t *testing.T) {
	for _, runID := range []string{".", "..", "nested/run", `nested\\run`, "run:escape", filepath.Join(string(filepath.Separator), "absolute-run")} {
		t.Run(strings.ReplaceAll(runID, string(filepath.Separator), "_"), func(t *testing.T) {
			evidenceDir := filepath.Join(t.TempDir(), "evidence")
			result, err := apicase.WriteFailureEvidence(apicase.FailureEvidenceOptions{
				RunID: runID, EvidenceDir: evidenceDir,
				Case:    apicase.Case{ID: "case.unsafe-run-id", Request: apicase.Request{Method: "GET", Path: "/"}},
				Message: "must not be written",
			})
			if err == nil || !strings.Contains(err.Error(), "single path segment") {
				t.Fatalf("unsafe run id %q error = %v", runID, err)
			}
			if result.RunID != "" || result.EvidencePath != "" {
				t.Fatalf("unsafe run id %q result = %#v", runID, result)
			}
			if _, statErr := os.Stat(evidenceDir); !errors.Is(statErr, os.ErrNotExist) {
				t.Fatalf("unsafe run id %q created Evidence root: %v", runID, statErr)
			}
		})
	}
}

func TestWriteFailureEvidenceChecksLeaseBeforeEveryWrite(t *testing.T) {
	var checks int
	leaseLost := errors.New("failure Evidence owner lease lost")
	result, err := apicase.WriteFailureEvidence(apicase.FailureEvidenceOptions{
		Context:     context.Background(),
		RunID:       "run.materialization.fence",
		EvidenceDir: t.TempDir(),
		Case: apicase.Case{
			ID:      "case.materialization.fence",
			Request: apicase.Request{Method: "GET", Path: "/"},
		},
		Phase:    apicase.FailurePhaseMaterialization,
		Category: apicase.FailureCategoryMaterialization,
		Message:  "materialization failed",
		BeforeEvidenceWrite: func(context.Context) error {
			checks++
			if checks == 3 {
				return leaseLost
			}
			return nil
		},
	})
	if !errors.Is(err, leaseLost) {
		t.Fatalf("failure Evidence fence error = %v, want lease lost", err)
	}
	if result.FailurePhase != apicase.FailurePhasePersistence || result.FailureCategory != apicase.FailureCategoryEvidenceWrite {
		t.Fatalf("failure Evidence fence result = %#v", result)
	}
	if _, err := os.Stat(filepath.Join(result.EvidencePath, "case.json")); err != nil {
		t.Fatalf("expected pre-takeover case Evidence: %v", err)
	}
	for _, name := range []string{"request.json", "assertions.json", "error.json", "summary.json"} {
		if _, err := os.Stat(filepath.Join(result.EvidencePath, name)); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("post-takeover failure Evidence %s error = %v, want not exist", name, err)
		}
	}
}
