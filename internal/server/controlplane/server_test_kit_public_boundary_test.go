package controlplane_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"agent-testbench/internal/domain/profile"
	"agent-testbench/internal/server/controlplane"
	"agent-testbench/internal/store"
)

func TestPublicTestKitRunRejectsTrustedExecutionFields(t *testing.T) {
	fixture := newPublicTestKitBoundaryFixture(t)
	t.Run("rejects trusted context without side effects", fixture.assertTrustedContextRejected)
	t.Run("uses Store catalog execution context", fixture.assertStoreCatalogExecution)
}

type publicTestKitBoundaryFixture struct {
	ctx                 context.Context
	runtime             store.Store
	serverURL           string
	injectedTargetURL   string
	victimRoot          string
	victimPath          string
	catalogEvidenceRoot string
	catalogTargetCalls  atomic.Int32
	injectedTargetCalls atomic.Int32
}

func newPublicTestKitBoundaryFixture(t *testing.T) *publicTestKitBoundaryFixture {
	t.Helper()
	fixture := &publicTestKitBoundaryFixture{ctx: context.Background()}
	catalogTarget := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fixture.catalogTargetCalls.Add(1)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	t.Cleanup(catalogTarget.Close)
	injectedTarget := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fixture.injectedTargetCalls.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(injectedTarget.Close)
	fixture.injectedTargetURL = injectedTarget.URL

	casePath := writePublicTestKitBoundaryCase(t)
	fixture.catalogEvidenceRoot = filepath.Join(t.TempDir(), "catalog-evidence")
	fixture.victimRoot = filepath.Join(t.TempDir(), "victim")
	fixture.victimPath = filepath.Join(fixture.victimRoot, "request.json")
	if err := os.MkdirAll(fixture.victimRoot, 0o755); err != nil {
		t.Fatalf("create victim root: %v", err)
	}
	if err := os.WriteFile(fixture.victimPath, []byte("keep-me"), 0o600); err != nil {
		t.Fatalf("write victim marker: %v", err)
	}

	fixture.runtime = openTestKitSQLiteStore(t, fixture.ctx, "public-boundary.sqlite")
	replacePublicTestKitBoundaryCatalog(t, fixture, catalogTarget.URL, casePath)
	server := httptest.NewServer(controlplane.NewWithStore(profile.Bundle{ID: "bootstrap"}, fixture.runtime))
	t.Cleanup(server.Close)
	fixture.serverURL = server.URL
	return fixture
}

func writePublicTestKitBoundaryCase(t *testing.T) string {
	t.Helper()
	casePath := filepath.Join(t.TempDir(), "case.json")
	if err := os.WriteFile(casePath, []byte(`{
		"id":"case.public-boundary",
		"request":{"method":"GET","path":"/health"},
		"assertions":{"expectedStatusCodes":[200]}
	}`), 0o644); err != nil {
		t.Fatalf("write public boundary case: %v", err)
	}
	return casePath
}

func replacePublicTestKitBoundaryCatalog(t *testing.T, fixture *publicTestKitBoundaryFixture, catalogTargetURL string, casePath string) {
	t.Helper()
	err := fixture.runtime.ReplaceProfileCatalog(fixture.ctx, store.ProfileCatalog{
		ProfileID:      "sample",
		IndexedAt:      time.Now().UTC(),
		InterfaceNodes: []store.CatalogInterfaceNode{{ID: "node.public-boundary", Status: "active"}},
		APICases: []store.CatalogAPICase{{
			ID: "case.public-boundary", NodeID: "node.public-boundary", Status: "active", CasePath: casePath,
			BaseURL: catalogTargetURL, EvidenceDir: fixture.catalogEvidenceRoot,
		}},
	})
	if err != nil {
		t.Fatalf("replace public boundary catalog: %v", err)
	}
}

func (fixture *publicTestKitBoundaryFixture) assertTrustedContextRejected(t *testing.T) {
	malicious := map[string]any{
		"caseId": "case.public-boundary", "baseUrl": fixture.injectedTargetURL,
		"evidenceDir": fixture.victimRoot, "runId": ".", "environmentId": "forged-environment",
		"testPlanMapId": "forged-map", "testPlanPathId": "forged-path", "testPlanNodeId": "forged-node",
		"testPlanOperation": "forged-operation", "plannerSummary": map[string]any{"forged": true},
	}
	raw, err := json.Marshal(malicious)
	if err != nil {
		t.Fatalf("encode malicious test-kit request: %v", err)
	}
	var rejected map[string]any
	postJSONInto(t, fixture.serverURL+"/api/test-kit/run", string(raw), http.StatusBadRequest, &rejected)
	if rejected["code"] != "trusted_execution_context_rejected" {
		t.Fatalf("malicious rejection = %#v", rejected)
	}
	fixture.assertTargetCalls(t, 0, 0)
	if marker, err := os.ReadFile(fixture.victimPath); err != nil || string(marker) != "keep-me" {
		t.Fatalf("victim marker = %q err=%v", marker, err)
	}
	if runs, err := fixture.runtime.ListRuns(fixture.ctx); err != nil || len(runs) != 0 {
		t.Fatalf("rejected request runs = %#v err=%v", runs, err)
	}
}

func (fixture *publicTestKitBoundaryFixture) assertStoreCatalogExecution(t *testing.T) {
	var accepted map[string]any
	postJSONInto(t, fixture.serverURL+"/api/test-kit/run", `{"caseId":"case.public-boundary"}`, http.StatusOK, &accepted)
	if accepted["ok"] != true || strings.TrimSpace(fmt.Sprint(accepted["runId"])) == "" || accepted["runId"] == "." {
		t.Fatalf("Store-native accepted result = %#v", accepted)
	}
	fixture.assertTargetCalls(t, 1, 0)
	runs, err := fixture.runtime.ListRuns(fixture.ctx)
	if err != nil || len(runs) != 1 {
		t.Fatalf("Store-native runs = %#v err=%v", runs, err)
	}
	run := runs[0]
	if run.EnvironmentID != "" || run.TestPlanMapID != "" || run.TestPlanPathID != "" || !strings.HasPrefix(run.EvidenceRoot, fixture.catalogEvidenceRoot) {
		t.Fatalf("Store-native run accepted untrusted context: %#v", run)
	}
}

func (fixture *publicTestKitBoundaryFixture) assertTargetCalls(t *testing.T, wantCatalog int32, wantInjected int32) {
	t.Helper()
	if got := fixture.catalogTargetCalls.Load(); got != wantCatalog {
		t.Fatalf("catalog target calls = %d, want %d", got, wantCatalog)
	}
	if got := fixture.injectedTargetCalls.Load(); got != wantInjected {
		t.Fatalf("injected target calls = %d, want %d", got, wantInjected)
	}
}

func TestPublicTestKitRunCannotEscapeEvidenceRoot(t *testing.T) {
	server := httptest.NewServer(controlplane.New(profile.Bundle{ID: "sample"}))
	defer server.Close()
	for name, payload := range map[string]string{
		"dot run id":        `{"caseId":"case.any","runId":"."}`,
		"parent run id":     `{"caseId":"case.any","runId":".."}`,
		"escaping run id":   `{"caseId":"case.any","runId":"../escape"}`,
		"absolute evidence": `{"caseId":"case.any","evidenceDir":"/tmp/escape"}`,
	} {
		t.Run(name, func(t *testing.T) {
			var result map[string]any
			postJSONInto(t, server.URL+"/api/test-kit/run", payload, http.StatusBadRequest, &result)
			if result["code"] != "trusted_execution_context_rejected" {
				t.Fatalf("public path rejection = %#v", result)
			}
		})
	}
}

func TestPublicTestKitBatchRejectsTrustedExecutionFields(t *testing.T) {
	server := httptest.NewServer(controlplane.New(profile.Bundle{ID: "sample"}))
	defer server.Close()
	for _, payload := range []string{
		`{"caseIds":["case.any"],"baseUrl":"http://127.0.0.1:1"}`,
		`{"caseIds":["case.any"],"evidenceDir":"/tmp/escape"}`,
		`{"caseIds":["case.any"],"runId":"."}`,
	} {
		var result map[string]any
		postJSONInto(t, server.URL+"/api/test-kit/run-batch", payload, http.StatusBadRequest, &result)
		if result["code"] != "trusted_execution_context_rejected" {
			t.Fatalf("public batch rejection = %#v", result)
		}
	}
}
