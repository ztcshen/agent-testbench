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

func TestPublicAPICaseBatchRunRejectsTrustedExecutionFields(t *testing.T) {
	ctx, runtime := openAPICaseBatchSQLiteStore(t)
	var catalogTargetCalls atomic.Int32
	var injectedTargetCalls atomic.Int32
	catalogTarget := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		catalogTargetCalls.Add(1)
		if r.URL.Path != "/health" {
			t.Fatalf("catalog target path = %q", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer catalogTarget.Close()
	injectedTarget := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		injectedTargetCalls.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer injectedTarget.Close()

	casePath := writeAPICaseBatchGETCase(t, t.TempDir(), "case.public-boundary", "/health")
	catalogEvidenceRoot := filepath.Join(t.TempDir(), "catalog-evidence")
	if err := runtime.ReplaceProfileCatalog(ctx, store.ProfileCatalog{
		ProfileID:      "sample",
		IndexedAt:      time.Now().UTC(),
		InterfaceNodes: []store.CatalogInterfaceNode{{ID: "node.public-boundary", Status: "active"}},
		APICases: []store.CatalogAPICase{{
			ID: "case.public-boundary", NodeID: "node.public-boundary", Status: "active", CasePath: casePath,
			BaseURL: catalogTarget.URL, EvidenceDir: catalogEvidenceRoot,
		}},
	}); err != nil {
		t.Fatalf("replace public batch catalog: %v", err)
	}

	victimRoot := t.TempDir()
	victimMarker := filepath.Join(victimRoot, "keep.txt")
	if err := os.WriteFile(victimMarker, []byte("keep-me"), 0o600); err != nil {
		t.Fatalf("write victim marker: %v", err)
	}
	server := httptest.NewServer(controlplane.NewWithStore(profile.Bundle{ID: "bootstrap"}, runtime))
	defer server.Close()

	requests := []map[string]any{
		{"requestId": "reject-base-url", "caseIds": []string{"case.public-boundary"}, "baseUrl": injectedTarget.URL, "timeoutSeconds": 3, "overrides": map[string]any{"id": "declared"}},
		{"requestId": "reject-evidence-dir", "caseIds": []string{"case.public-boundary"}, "evidenceDir": victimRoot},
		{"requestId": "reject-environment-id", "caseIds": []string{"case.public-boundary"}, "environmentId": "forged-environment"},
		{"requestId": "reject-empty-trusted-fields", "caseIds": []string{"case.public-boundary"}, "baseUrl": nil, "evidenceDir": ""},
	}
	for _, request := range requests {
		raw, err := json.Marshal(request)
		if err != nil {
			t.Fatalf("encode malicious request: %v", err)
		}
		var rejected map[string]any
		postJSONInto(t, server.URL+"/api/cases/batch-runs", string(raw), http.StatusBadRequest, &rejected)
		if rejected["code"] != "trusted_execution_context_rejected" {
			t.Fatalf("trusted field rejection = %#v", rejected)
		}
		errorText := fmt.Sprint(rejected["error"])
		if strings.Contains(errorText, injectedTarget.URL) || strings.Contains(errorText, victimRoot) {
			t.Fatalf("rejection leaked client values: %q", errorText)
		}
	}
	for _, body := range []string{
		`{"requestId":"reject-negative-timeout","caseIds":["case.public-boundary"],"timeoutSeconds":-1}`,
		`{"requestId":"reject-long-timeout","caseIds":["case.public-boundary"],"timeoutSeconds":601}`,
		`{"requestId":"reject-overflow-timeout","caseIds":["case.public-boundary"],"timeoutSeconds":999999999999999999999999999999999999}`,
	} {
		var rejected map[string]any
		postJSONInto(t, server.URL+"/api/cases/batch-runs", body, http.StatusBadRequest, &rejected)
		if rejected["code"] != "invalid_timeout_seconds" {
			t.Fatalf("timeout rejection = %#v", rejected)
		}
	}
	mixedSelector := postJSONResponse(t, server.URL+"/api/cases/batch-runs", `{"requestId":"reject-mixed-selector","caseIds":["case.public-boundary"],"workflowId":"forged-workflow"}`, http.StatusBadRequest)
	if mixedSelector["code"] != "invalid_batch_selector" {
		t.Fatalf("mixed selector rejection = %#v", mixedSelector)
	}

	if catalogTargetCalls.Load() != 0 || injectedTargetCalls.Load() != 0 {
		t.Fatalf("rejected requests reached targets: catalog=%d injected=%d", catalogTargetCalls.Load(), injectedTargetCalls.Load())
	}
	if runs, err := runtime.ListRuns(ctx); err != nil || len(runs) != 0 {
		t.Fatalf("rejected requests created runs: %#v err=%v", runs, err)
	}
	requireUnchangedBatchVictimRoot(t, victimRoot, victimMarker)

	var created apiCaseBatchRunCreatedForTest
	postJSONInto(t, server.URL+"/api/cases/batch-runs", `{"requestId":"store-native","caseIds":["case.public-boundary"],"timeoutSeconds":3,"overrides":{"id":"declared"}}`, http.StatusAccepted, &created)
	report := waitAPICaseBatchReport(t, server.URL+created.ReportURL)
	if !report.OK || report.Passed != 1 || !strings.HasPrefix(report.HTMLReportPath, catalogEvidenceRoot) {
		t.Fatalf("Store-native batch report = %#v", report)
	}
	if catalogTargetCalls.Load() != 1 || injectedTargetCalls.Load() != 0 {
		t.Fatalf("Store-native targets: catalog=%d injected=%d", catalogTargetCalls.Load(), injectedTargetCalls.Load())
	}
	requireUnchangedBatchVictimRoot(t, victimRoot, victimMarker)
}

func TestStartTrustedAPICaseBatchRunAcceptsTypedLocalOverrides(t *testing.T) {
	ctx, runtime := openAPICaseBatchSQLiteStore(t)
	var catalogTargetCalls atomic.Int32
	var trustedTargetCalls atomic.Int32
	catalogTarget := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		catalogTargetCalls.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer catalogTarget.Close()
	trustedTarget := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		trustedTargetCalls.Add(1)
		if r.URL.Path != "/health" {
			t.Fatalf("trusted target path = %q", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer trustedTarget.Close()

	casePath := writeAPICaseBatchGETCase(t, t.TempDir(), "case.trusted-boundary", "/health")
	if err := runtime.ReplaceProfileCatalog(ctx, store.ProfileCatalog{
		ProfileID:      "sample",
		IndexedAt:      time.Now().UTC(),
		InterfaceNodes: []store.CatalogInterfaceNode{{ID: "node.trusted-boundary", Status: "active"}},
		APICases: []store.CatalogAPICase{{
			ID: "case.trusted-boundary", NodeID: "node.trusted-boundary", Status: "active", CasePath: casePath,
			BaseURL: catalogTarget.URL, EvidenceDir: filepath.Join(t.TempDir(), "catalog-evidence"),
		}},
	}); err != nil {
		t.Fatalf("replace trusted batch catalog: %v", err)
	}

	trustedEvidenceRoot := filepath.Join(t.TempDir(), "trusted-evidence")
	for _, timeoutSeconds := range []int{601, int(^uint(0) >> 1)} {
		rejected, status, err := controlplane.StartTrustedAPICaseBatchRun(ctx, profile.Bundle{ID: "bootstrap"}, runtime, controlplane.TrustedAPICaseBatchRunRequest{
			RequestID:      "typed-timeout-rejected",
			CaseIDs:        []string{"case.trusted-boundary"},
			TimeoutSeconds: timeoutSeconds,
		})
		if err == nil || status != http.StatusBadRequest || rejected.BatchRunID != "" {
			t.Fatalf("typed timeout %d rejection = %#v status=%d err=%v", timeoutSeconds, rejected, status, err)
		}
	}
	mixed, status, err := controlplane.StartTrustedAPICaseBatchRun(ctx, profile.Bundle{ID: "bootstrap"}, runtime, controlplane.TrustedAPICaseBatchRunRequest{
		RequestID:  "typed-mixed-selector",
		CaseIDs:    []string{"case.trusted-boundary"},
		WorkflowID: "forged-workflow",
	})
	if err == nil || status != http.StatusBadRequest || mixed.BatchRunID != "" || !strings.Contains(err.Error(), "exactly one") {
		t.Fatalf("typed mixed selector rejection = %#v status=%d err=%v", mixed, status, err)
	}
	started, status, err := controlplane.StartTrustedAPICaseBatchRun(ctx, profile.Bundle{ID: "bootstrap"}, runtime, controlplane.TrustedAPICaseBatchRunRequest{
		RequestID:      "typed-trusted",
		CaseIDs:        []string{"case.trusted-boundary"},
		BaseURL:        trustedTarget.URL,
		EvidenceDir:    trustedEvidenceRoot,
		TimeoutSeconds: 3,
		Overrides:      map[string]any{"id": "trusted"},
	})
	if err != nil || status != http.StatusAccepted || started.BatchRunID == "" || started.Total != 1 {
		t.Fatalf("start typed trusted batch = %#v status=%d err=%v", started, status, err)
	}
	waitForStoredAPICaseBatchSummary(t, context.Background(), runtime, started.BatchRunID, func(summary map[string]any) bool {
		return summary["status"] != store.StatusRunning
	})
	if trustedTargetCalls.Load() != 1 || catalogTargetCalls.Load() != 0 {
		t.Fatalf("typed trusted targets: trusted=%d catalog=%d", trustedTargetCalls.Load(), catalogTargetCalls.Load())
	}
	if _, err := os.Stat(filepath.Join(trustedEvidenceRoot, started.BatchRunID, "report.html")); err != nil {
		t.Fatalf("stat typed trusted report: %v", err)
	}
}

func TestPublicAPICaseBatchRejectsUnboundedCatalogTimeout(t *testing.T) {
	ctx, runtime := openAPICaseBatchSQLiteStore(t)
	var targetCalls atomic.Int32
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		targetCalls.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer target.Close()
	evidenceRoot := filepath.Join(t.TempDir(), "evidence")
	replaceAPICaseBatchBoundaryCatalog(t, ctx, runtime, "case.catalog-timeout", writeAPICaseBatchGETCase(t, t.TempDir(), "case.catalog-timeout", "/health"), target.URL, evidenceRoot, 601)
	server := httptest.NewServer(controlplane.NewWithStore(profile.Bundle{ID: "bootstrap"}, runtime))
	defer server.Close()

	rejected := postJSONResponse(t, server.URL+"/api/cases/batch-runs", `{"requestId":"catalog-timeout","caseIds":["case.catalog-timeout"]}`, http.StatusBadRequest)
	if rejected["code"] != "invalid_timeout_seconds" || targetCalls.Load() != 0 {
		t.Fatalf("catalog timeout rejection = %#v targetCalls=%d", rejected, targetCalls.Load())
	}
	if runs, err := runtime.ListRuns(ctx); err != nil || len(runs) != 0 {
		t.Fatalf("catalog timeout created runs: %#v err=%v", runs, err)
	}
	if _, err := os.Stat(evidenceRoot); !os.IsNotExist(err) {
		t.Fatalf("catalog timeout wrote Evidence root: %v", err)
	}
}

func TestPublicAPICaseBatchCancelsBlockingTargetAtStoreTimeout(t *testing.T) {
	ctx, runtime := openAPICaseBatchSQLiteStore(t)
	entered := make(chan struct{})
	canceled := make(chan struct{})
	target := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		close(entered)
		<-r.Context().Done()
		close(canceled)
	}))
	defer target.Close()
	evidenceRoot := filepath.Join(t.TempDir(), "evidence")
	replaceAPICaseBatchBoundaryCatalog(t, ctx, runtime, "case.store-timeout", writeAPICaseBatchGETCase(t, t.TempDir(), "case.store-timeout", "/blocking"), target.URL, evidenceRoot, 1)
	server := httptest.NewServer(controlplane.NewWithStore(profile.Bundle{ID: "bootstrap"}, runtime))
	defer server.Close()

	startedAt := time.Now()
	var created apiCaseBatchRunCreatedForTest
	postJSONInto(t, server.URL+"/api/cases/batch-runs", `{"requestId":"store-timeout","caseIds":["case.store-timeout"]}`, http.StatusAccepted, &created)
	select {
	case <-entered:
	case <-time.After(3 * time.Second):
		t.Fatal("blocking target was not called")
	}
	report := waitAPICaseBatchReport(t, server.URL+created.ReportURL)
	if report.OK || report.Status != store.StatusFailed || report.Failed != 1 || len(report.Cases) != 1 || !strings.Contains(report.Cases[0].Error, "deadline") {
		t.Fatalf("Store timeout report = %#v", report)
	}
	if elapsed := time.Since(startedAt); elapsed > 5*time.Second {
		t.Fatalf("Store timeout took %s", elapsed)
	}
	select {
	case <-canceled:
	case <-time.After(3 * time.Second):
		t.Fatal("blocking target context was not canceled")
	}
}

func replaceAPICaseBatchBoundaryCatalog(t *testing.T, ctx context.Context, runtime store.Store, caseID string, casePath string, baseURL string, evidenceDir string, timeoutSeconds int) {
	t.Helper()
	if err := runtime.ReplaceProfileCatalog(ctx, store.ProfileCatalog{
		ProfileID:      "sample",
		IndexedAt:      time.Now().UTC(),
		InterfaceNodes: []store.CatalogInterfaceNode{{ID: "node.boundary", Status: "active"}},
		APICases: []store.CatalogAPICase{{
			ID: caseID, NodeID: "node.boundary", Status: "active", CasePath: casePath,
			BaseURL: baseURL, EvidenceDir: evidenceDir, TimeoutSeconds: timeoutSeconds,
		}},
	}); err != nil {
		t.Fatalf("replace batch boundary catalog: %v", err)
	}
}

func requireUnchangedBatchVictimRoot(t *testing.T, root string, markerPath string) {
	t.Helper()
	marker, err := os.ReadFile(markerPath)
	if err != nil || string(marker) != "keep-me" {
		t.Fatalf("victim marker = %q err=%v", marker, err)
	}
	entries, err := os.ReadDir(root)
	if err != nil || len(entries) != 1 || entries[0].Name() != filepath.Base(markerPath) {
		t.Fatalf("victim root entries = %#v err=%v", entries, err)
	}
}
