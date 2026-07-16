package controlplane_test

import (
	"context"
	"errors"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"agent-testbench/internal/domain/profile"
	"agent-testbench/internal/server/controlplane"
	"agent-testbench/internal/store"
)

func TestAPICaseBatchPersistsMaterializationFailureEvidence(t *testing.T) {
	ctx, runtime := openAPICaseBatchSQLiteStore(t)
	bundle := apiCaseBatchMaterializationFailureBundle(t, ctx, runtime)
	server := httptest.NewServer(controlplane.NewWithStore(bundle, runtime))
	t.Cleanup(server.Close)

	created := startAPICaseBatchPersistenceRun(t, server.URL, []string{"case.materialization.failure"})
	payload := waitForAPICaseBatchPayload(t, server.URL+created.ReportURL, func(payload map[string]any) bool {
		return payload["status"] != store.StatusRunning
	})
	item := onlyMaterializationFailureCase(t, payload)
	if item["status"] != store.StatusFailed || item["failurePhase"] != "materialization" || item["failureCategory"] != "materialization-error" {
		t.Fatalf("materialization failure item = %#v", item)
	}
	if !strings.Contains(valueStringForBatchPersistenceTest(item["error"]), "service runtime is not available") {
		t.Fatalf("materialization failure error = %#v", item)
	}
	runID := valueStringForBatchPersistenceTest(item["runId"])
	caseRunID := valueStringForBatchPersistenceTest(item["caseRunId"])
	evidencePath := valueStringForBatchPersistenceTest(item["evidencePath"])
	if runID == "" || caseRunID != runID+".case" || evidencePath == "" {
		t.Fatalf("materialization failure handles = %#v", item)
	}
	if valueStringForBatchPersistenceTest(item["detailUrl"]) == "" || valueStringForBatchPersistenceTest(item["viewerUrl"]) == "" {
		t.Fatalf("materialization failure links = %#v", item)
	}

	child, err := runtime.GetRun(ctx, runID)
	if err != nil {
		t.Fatalf("load materialization failure child Run: %v", err)
	}
	if child.Status != store.StatusFailed || !strings.Contains(child.SummaryJSON, `"failure_phase":"materialization"`) || !strings.Contains(child.SummaryJSON, `"failure_category":"materialization-error"`) {
		t.Fatalf("materialization failure child Run = %#v", child)
	}
	caseRuns, err := runtime.ListAPICaseRuns(ctx, runID)
	if err != nil {
		t.Fatalf("list materialization failure API Case Runs: %v", err)
	}
	if len(caseRuns) != 1 || caseRuns[0].ID != caseRunID || caseRuns[0].Status != store.StatusFailed {
		t.Fatalf("materialization failure API Case Runs = %#v", caseRuns)
	}
	records, err := runtime.ListEvidence(ctx, runID)
	if err != nil {
		t.Fatalf("list materialization failure Evidence: %v", err)
	}
	kinds := make([]string, 0, len(records))
	for _, record := range records {
		kinds = append(kinds, record.Kind)
		if record.RunID != runID || record.CaseRunID != caseRunID || !strings.HasPrefix(record.URI, evidencePath+string(filepath.Separator)) {
			t.Fatalf("materialization failure Evidence record = %#v", record)
		}
		if _, err := os.Stat(record.URI); err != nil {
			t.Fatalf("materialization failure Evidence URI %s: %v", record.URI, err)
		}
	}
	sort.Strings(kinds)
	if strings.Join(kinds, ",") != "assertions,case,error,request,summary" {
		t.Fatalf("materialization failure Evidence kinds = %v", kinds)
	}
}

func TestAPICaseBatchClassifiesMaterializationEvidenceStoreFailureAsBatchPersistence(t *testing.T) {
	ctx, runtime := openAPICaseBatchSQLiteStore(t)
	bundle := apiCaseBatchMaterializationFailureBundle(t, ctx, runtime)
	failing := reportEvidenceFailingStore{Store: runtime, err: errors.New("child Evidence index unavailable")}
	server := httptest.NewServer(controlplane.NewWithStore(bundle, failing))
	t.Cleanup(server.Close)

	created := startAPICaseBatchPersistenceRun(t, server.URL, []string{"case.materialization.failure"})
	payload := waitForAPICaseBatchPayload(t, server.URL+created.ReportURL, func(payload map[string]any) bool {
		return payload["status"] != store.StatusRunning
	})
	if payload["status"] != store.StatusFailed || payload["failureCategory"] != "batch-persistence-error" {
		t.Fatalf("materialization Store failure batch = %#v", payload)
	}
	item := onlyMaterializationFailureCase(t, payload)
	if item["status"] != store.StatusFailed || item["failurePhase"] != "persistence" || item["failureCategory"] != "batch-persistence-error" {
		t.Fatalf("materialization Store failure item = %#v", item)
	}
	if !strings.Contains(valueStringForBatchPersistenceTest(item["error"]), "child Evidence index unavailable") {
		t.Fatalf("materialization Store failure error = %#v", item)
	}
	runID := valueStringForBatchPersistenceTest(item["runId"])
	if runID == "" || valueStringForBatchPersistenceTest(item["caseRunId"]) == "" || valueStringForBatchPersistenceTest(item["evidencePath"]) == "" {
		t.Fatalf("materialization Store failure handles = %#v", item)
	}
	child, err := runtime.GetRun(ctx, runID)
	if err != nil || child.Status != store.StatusFailed {
		t.Fatalf("partially persisted materialization child Run = %#v, %v", child, err)
	}
	records, err := runtime.ListEvidence(ctx, runID)
	if err != nil {
		t.Fatalf("list partially persisted materialization Evidence: %v", err)
	}
	if len(records) != 0 {
		t.Fatalf("failed Evidence index should not be reported as complete: %#v", records)
	}
	parent, err := runtime.GetRun(ctx, created.BatchRunID)
	if err != nil {
		t.Fatalf("load failed materialization batch parent: %v", err)
	}
	if parent.Status != store.StatusFailed || !strings.Contains(parent.SummaryJSON, "batch-persistence-error") {
		t.Fatalf("failed materialization batch parent = %#v", parent)
	}
}

func TestAPICaseBatchPersistsCaseFileLoadFailureEvidence(t *testing.T) {
	tests := []struct {
		name      string
		writeCase bool
		contents  string
		wantError string
	}{
		{name: "missing", wantError: "no such file"},
		{name: "invalid-json", writeCase: true, contents: `{"id":`, wantError: "unexpected end of JSON input"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, runtime := openAPICaseBatchSQLiteStore(t)
			bundle := apiCaseBatchPersistenceBundle(t, "http://127.0.0.1",
				apiCaseBatchPersistenceCase{id: "case.file-load.failure", path: "/v1/file-load", order: 1},
			)
			casePath := filepath.Join(t.TempDir(), "case-file-load.json")
			if tt.writeCase {
				if err := os.WriteFile(casePath, []byte(tt.contents), 0o644); err != nil {
					t.Fatalf("write invalid case file: %v", err)
				}
			}
			bundle.APICases[0].CasePath = casePath
			server := httptest.NewServer(controlplane.NewWithStore(bundle, runtime))
			t.Cleanup(server.Close)

			created := startAPICaseBatchPersistenceRun(t, server.URL, []string{"case.file-load.failure"})
			payload := waitForAPICaseBatchPayload(t, server.URL+created.ReportURL, func(payload map[string]any) bool {
				return payload["status"] != store.StatusRunning
			})
			item := onlyMaterializationFailureCase(t, payload)
			if item["status"] != store.StatusFailed || item["failurePhase"] != "materialization" || item["failureCategory"] != "materialization-error" {
				t.Fatalf("case file load failure item = %#v", item)
			}
			if !strings.Contains(valueStringForBatchPersistenceTest(item["error"]), tt.wantError) {
				t.Fatalf("case file load failure error = %#v", item)
			}
			runID := valueStringForBatchPersistenceTest(item["runId"])
			caseRunID := valueStringForBatchPersistenceTest(item["caseRunId"])
			if runID == "" || caseRunID != runID+".case" || valueStringForBatchPersistenceTest(item["evidencePath"]) == "" {
				t.Fatalf("case file load failure handles = %#v", item)
			}
			child, err := runtime.GetRun(ctx, runID)
			if err != nil || child.Status != store.StatusFailed || !strings.Contains(child.SummaryJSON, `"failure_category":"materialization-error"`) {
				t.Fatalf("case file load failure child Run = %#v err=%v", child, err)
			}
			records, err := runtime.ListEvidence(ctx, runID)
			if err != nil {
				t.Fatalf("list case file load failure Evidence: %v", err)
			}
			kinds := make([]string, 0, len(records))
			for _, record := range records {
				kinds = append(kinds, record.Kind)
			}
			sort.Strings(kinds)
			if strings.Join(kinds, ",") != "assertions,case,error,request,summary" {
				t.Fatalf("case file load failure Evidence kinds = %v", kinds)
			}
		})
	}
}

func apiCaseBatchMaterializationFailureBundle(t *testing.T, ctx context.Context, runtime store.Store) profile.Bundle {
	t.Helper()
	evidenceDir := filepath.Join(t.TempDir(), "evidence")
	const (
		caseID = "case.materialization.failure"
		nodeID = "node.materialization.missing-runtime"
	)
	if err := runtime.ReplaceProfileCatalog(ctx, store.ProfileCatalog{
		ProfileID: "sample",
		IndexedAt: time.Now().UTC(),
		InterfaceNodes: []store.CatalogInterfaceNode{{
			ID: nodeID, Method: "POST", Path: "/v1/materialize", Status: "active",
		}},
		APICases: []store.CatalogAPICase{{
			ID: caseID, DisplayName: "Materialization failure", NodeID: nodeID, Status: "active", EvidenceDir: evidenceDir,
		}},
		TemplateConfigs: []store.CatalogTemplateConfig{{
			ID:         "config.case.materialization.failure",
			ScopeType:  "api-case",
			ScopeID:    caseID,
			Status:     "active",
			ConfigJSON: `{"caseId":"case.materialization.failure","caseExecution":{"method":"POST","nodeId":"node.materialization.missing-runtime","path":"/v1/materialize","body":{"value":"{{override:value|default}}"},"expectedHttpCodes":[200]}}`,
		}},
	}); err != nil {
		t.Fatalf("seed materialization failure catalog: %v", err)
	}
	return profile.Bundle{
		ID: "sample",
		InterfaceNodes: []profile.InterfaceNode{{
			ID: nodeID, Method: "POST", Path: "/v1/materialize", Status: "active",
		}},
		APICases: []profile.APICase{{
			ID: caseID, DisplayName: "Materialization failure", NodeID: nodeID, Status: "active", EvidenceDir: evidenceDir,
		}},
	}
}

func onlyMaterializationFailureCase(t *testing.T, payload map[string]any) map[string]any {
	t.Helper()
	cases, _ := payload["cases"].([]any)
	if len(cases) != 1 {
		t.Fatalf("materialization failure cases = %#v", cases)
	}
	item, _ := cases[0].(map[string]any)
	if item == nil {
		t.Fatalf("materialization failure case payload = %#v", cases[0])
	}
	return item
}
