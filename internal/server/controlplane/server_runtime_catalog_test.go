package controlplane_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"agent-testbench/internal/domain/profile"
	"agent-testbench/internal/server/controlplane"
	"agent-testbench/internal/store"
	"agent-testbench/internal/store/sqlite"
)

func TestServerUsesRuntimeCatalogForWorkflowDirectory(t *testing.T) {
	fixture := newRuntimeCatalogWorkflowFixture(t)
	server := newRuntimeCatalogWorkflowServer(t, fixture.Store)
	payload := getRuntimeCatalogPayload(t, server.URL)
	assertRuntimeCatalogEnvelope(t, payload, fixture)
	workflow := assertRuntimeCatalogWorkflowSummary(t, payload, fixture)
	assertRuntimeCatalogWorkflowPresentation(t, workflow)
	assertRuntimeCatalogWorkflowSteps(t, workflow, fixture)
}

func TestServerPersistsBatchCaseRunsForInterfaceNodeGreenState(t *testing.T) {
	fixture := newInterfaceGreenStateFixture(t)
	batchPayload := postInterfaceGreenStateBatch(t, fixture)
	assertInterfaceGreenStateBatchEvidence(t, fixture.Server.URL, batchPayload)
	assertInterfaceNodeGreenState(t, fixture.Server.URL)
	assertInterfaceNodeListGreenState(t, fixture.Server.URL)
}

func TestServerUsesLatestPassingCacheForDirectInterfaceAdmission(t *testing.T) {
	ctx := context.Background()
	s, err := sqlite.Open(ctx, sqlite.Config{Path: filepath.Join(t.TempDir(), "sandbox.sqlite")})
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer s.Close()
	now := time.Date(2026, 5, 15, 10, 0, 0, 0, time.UTC)
	if err := s.ReplaceProfileCatalog(ctx, store.ProfileCatalog{
		ProfileID: "sample",
		IndexedAt: now,
		InterfaceNodes: []store.CatalogInterfaceNode{
			{ID: "interface.alpha", DisplayName: "Alpha", Status: "active"},
		},
		APICases: []store.CatalogAPICase{
			{ID: "case.alpha", DisplayName: "Alpha", NodeID: "interface.alpha", CaseType: "success", RequiredForAdmission: true, Status: "active"},
			{ID: "case.alpha.optional", DisplayName: "Alpha optional", NodeID: "interface.alpha", CaseType: "failure", RequiredForAdmission: false, Status: "active"},
		},
	}); err != nil {
		t.Fatalf("replace profile catalog: %v", err)
	}
	for _, run := range []store.Run{
		{ID: "run.pass", ProfileID: "sample", WorkflowID: "workflow.alpha", Status: store.StatusPassed, StartedAt: now, FinishedAt: now.Add(100 * time.Millisecond), CreatedAt: now, UpdatedAt: now},
		{ID: "run.fail", ProfileID: "sample", WorkflowID: "case.alpha", Status: store.StatusFailed, StartedAt: now.Add(time.Minute), FinishedAt: now.Add(time.Minute + 200*time.Millisecond), CreatedAt: now.Add(time.Minute), UpdatedAt: now.Add(time.Minute)},
	} {
		if _, err := s.CreateRun(ctx, run); err != nil {
			t.Fatalf("create run %s: %v", run.ID, err)
		}
	}
	for _, item := range []store.APICaseRun{
		{ID: "run.pass.case", RunID: "run.pass", CaseID: "case.alpha", Status: store.StatusPassed, AssertionSummaryJSON: `{"status":"passed"}`, StartedAt: now, FinishedAt: now.Add(100 * time.Millisecond), CreatedAt: now},
		{ID: "run.fail.case", RunID: "run.fail", CaseID: "case.alpha", Status: store.StatusFailed, AssertionSummaryJSON: `{"status":"failed"}`, StartedAt: now.Add(time.Minute), FinishedAt: now.Add(time.Minute + 200*time.Millisecond), CreatedAt: now.Add(time.Minute)},
		{ID: "run.fail.optional", RunID: "run.fail", CaseID: "case.alpha.optional", Status: store.StatusFailed, AssertionSummaryJSON: `{"status":"failed"}`, StartedAt: now.Add(time.Minute), FinishedAt: now.Add(time.Minute + 300*time.Millisecond), CreatedAt: now.Add(time.Minute)},
	} {
		if _, err := s.RecordAPICaseRun(ctx, item); err != nil {
			t.Fatalf("record case run %s: %v", item.ID, err)
		}
	}
	server := httptest.NewServer(controlplane.NewWithStore(profile.Bundle{ID: "sample"}, s))
	defer server.Close()

	list := decodeJSONResponse(t, server.URL+"/api/interface-nodes", http.StatusOK)
	row := list["items"].([]any)[0].(map[string]any)
	if row["admissionStatus"] != store.StatusPassed || row["latestRunId"] != "run.pass" || row["latestElapsedMs"] != float64(100) {
		t.Fatalf("interface list should prefer cached pass: %#v", row)
	}
	detail := decodeJSONResponse(t, server.URL+"/api/interface-node?id=interface.alpha", http.StatusOK)
	admission := detail["admission"].(map[string]any)
	if admission["status"] != store.StatusPassed || admission["latestRunId"] != "run.pass" {
		t.Fatalf("interface detail should prefer cached pass: %#v", admission)
	}
}

func TestServerExplainsInterfaceAdmissionBlockersFromStoreRuns(t *testing.T) {
	ctx := context.Background()
	s, err := sqlite.Open(ctx, sqlite.Config{Path: filepath.Join(t.TempDir(), "sandbox.sqlite")})
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer s.Close()
	if err := s.ReplaceProfileCatalog(ctx, store.ProfileCatalog{
		ProfileID: "sample",
		IndexedAt: time.Now().UTC(),
		InterfaceNodes: []store.CatalogInterfaceNode{
			{ID: "interface.blocked", DisplayName: "Blocked", Status: "active"},
		},
		APICases: []store.CatalogAPICase{
			{ID: "case.passed", DisplayName: "Passed case", NodeID: "interface.blocked", CaseType: "success", RequiredForAdmission: true, Status: "active", SortOrder: 1},
			{ID: "case.failed", DisplayName: "Failed case", NodeID: "interface.blocked", CaseType: "success", RequiredForAdmission: true, Status: "active", SortOrder: 2},
			{ID: "case.missing", DisplayName: "Missing case", NodeID: "interface.blocked", CaseType: "failure", RequiredForAdmission: true, Status: "active", SortOrder: 3},
			{ID: "case.optional", DisplayName: "Optional case", NodeID: "interface.blocked", CaseType: "failure", RequiredForAdmission: false, Status: "active", SortOrder: 4},
		},
	}); err != nil {
		t.Fatalf("replace profile catalog: %v", err)
	}
	_, err = s.CreateRun(ctx, store.Run{ID: "run.blocked", ProfileID: "sample", WorkflowID: "workflow.blocked", Status: store.StatusFailed, SummaryJSON: `{}`})
	if err != nil {
		t.Fatalf("create run: %v", err)
	}
	for _, item := range []store.APICaseRun{
		{ID: "run.blocked.case.passed", RunID: "run.blocked", CaseID: "case.passed", Status: store.StatusPassed, AssertionSummaryJSON: `{"status":"passed"}`},
		{ID: "run.blocked.case.failed", RunID: "run.blocked", CaseID: "case.failed", Status: store.StatusFailed, AssertionSummaryJSON: `{"status":"failed","errorCount":1,"failureKind":"assertion"}`},
		{ID: "run.blocked.case.optional", RunID: "run.blocked", CaseID: "case.optional", Status: store.StatusFailed, AssertionSummaryJSON: `{"status":"failed","errorCount":1}`},
	} {
		if _, err := s.RecordAPICaseRun(ctx, item); err != nil {
			t.Fatalf("record api case run: %v", err)
		}
	}
	server := httptest.NewServer(controlplane.NewWithStore(profile.Bundle{ID: "sample", DisplayName: "Sample Profile"}, s))
	defer server.Close()

	payload := decodeJSONResponse(t, server.URL+"/api/interface-node?id=interface.blocked", http.StatusOK)
	admission := payload["admission"].(map[string]any)
	if admission["status"] != store.StatusFailed || admission["requiredCaseCount"] != float64(3) || admission["passedCaseCount"] != float64(1) {
		t.Fatalf("admission = %#v", admission)
	}
	blockers := admission["blockers"].([]any)
	if len(blockers) != 2 {
		t.Fatalf("blockers = %#v", admission)
	}
	failed := blockers[0].(map[string]any)
	missing := blockers[1].(map[string]any)
	if failed["caseId"] != "case.failed" || failed["status"] != store.StatusFailed || failed["runId"] != "run.blocked" || failed["failureReason"] != "assertion errors: 1" || failed["failureKind"] != "assertion" || failed["evidenceHref"] != "/evidence-viewer.html?caseRun=run.blocked&caseId=case.failed" {
		t.Fatalf("failed blocker = %#v", failed)
	}
	if missing["caseId"] != "case.missing" || missing["status"] != "missing_run" || missing["failureReason"] != "required case has no run" {
		t.Fatalf("missing blocker = %#v", missing)
	}
	attention := payload["attention"].(map[string]any)
	if attention["status"] != store.StatusFailed || attention["blockerCount"] != float64(2) {
		t.Fatalf("attention = %#v", attention)
	}
}

func TestServerDoesNotServeLegacyTopLevelScripts(t *testing.T) {
	server := httptest.NewServer(controlplane.New(loadEmptyProfile(t)))
	defer server.Close()

	for _, path := range []string{"/dashboard.js", "/workflows.js"} {
		resp, err := http.Get(server.URL + path)
		if err != nil {
			t.Fatalf("get %s: %v", path, err)
		}
		_, _ = io.Copy(io.Discard, resp.Body)
		if err := resp.Body.Close(); err != nil {
			t.Fatalf("close %s: %v", path, err)
		}
		if resp.StatusCode != http.StatusNotFound {
			t.Fatalf("%s status = %d, want 404", path, resp.StatusCode)
		}
	}
}
