package controlplane_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"agent-testbench/internal/domain/profile"
	"agent-testbench/internal/server/controlplane"
	"agent-testbench/internal/store"
)

func TestAPICaseBatchPersistsRunningParentBeforeExecutionCompletes(t *testing.T) {
	ctx, runtime := openAPICaseBatchSQLiteStore(t)
	target, started, release := newBlockingAPICaseBatchTarget(t, "/v1/persist/start")
	bundle := apiCaseBatchPersistenceBundle(t, target.URL,
		apiCaseBatchPersistenceCase{id: "case.persist.start", path: "/v1/persist/start", order: 1},
	)
	server := httptest.NewServer(controlplane.NewWithStore(bundle, runtime))
	t.Cleanup(server.Close)

	created := startAPICaseBatchPersistenceRun(t, server.URL, []string{"case.persist.start"})
	waitForAPICaseBatchSignal(t, started, "target request")

	run, err := runtime.GetRun(ctx, created.BatchRunID)
	if err != nil {
		t.Fatalf("running batch parent should already be stored: %v", err)
	}
	if run.Status != store.StatusRunning || run.ProfileID != bundle.ID {
		t.Fatalf("running batch parent = %#v", run)
	}
	summary := decodeAPICaseBatchStoreSummary(t, run.SummaryJSON)
	if summary["batchRunId"] != created.BatchRunID || summary["status"] != store.StatusRunning || summary["completed"] != float64(0) {
		t.Fatalf("running batch summary = %#v", summary)
	}

	release()
	report := waitAPICaseBatchReport(t, server.URL+created.ReportURL)
	if !report.OK || report.Status != store.StatusPassed {
		payload := decodeJSONResponse(t, server.URL+created.ReportURL, http.StatusOK)
		t.Fatalf("released batch report = %#v; payload = %#v", report, payload)
	}
}

func TestAPICaseBatchRunningReportIsReadOnlyAcrossControlPlaneInstances(t *testing.T) {
	ctx, runtime := openAPICaseBatchSQLiteStore(t)
	target, started, release := newBlockingAPICaseBatchTarget(t, "/v1/persist/cross-instance")
	bundle := apiCaseBatchPersistenceBundle(t, target.URL,
		apiCaseBatchPersistenceCase{id: "case.persist.cross-instance", path: "/v1/persist/cross-instance", order: 1},
	)
	owner := httptest.NewServer(controlplane.NewWithOptions(bundle, controlplane.Options{
		Runtime:                   runtime,
		APICaseBatchLeaseDuration: 120 * time.Millisecond,
	}))
	t.Cleanup(owner.Close)
	reader := httptest.NewServer(controlplane.NewWithStore(bundle, runtime))
	t.Cleanup(reader.Close)

	created := startAPICaseBatchPersistenceRun(t, owner.URL, []string{"case.persist.cross-instance"})
	waitForAPICaseBatchSignal(t, started, "cross-instance target request")
	initial, err := runtime.GetRun(ctx, created.BatchRunID)
	if err != nil {
		t.Fatalf("load initial cross-instance parent: %v", err)
	}
	initialSummary := decodeAPICaseBatchStoreSummary(t, initial.SummaryJSON)
	initialLease := initialSummary["_lease"].(map[string]any)
	initialRenewTime := initialLease["renewTime"]
	waitForStoredAPICaseBatchSummary(t, ctx, runtime, created.BatchRunID, func(summary map[string]any) bool {
		lease, _ := summary["_lease"].(map[string]any)
		return lease["renewTime"] != initialRenewTime
	})

	payload := decodeJSONResponse(t, reader.URL+created.ReportURL, http.StatusOK)
	runningReadOnly := payload["status"] == store.StatusRunning && payload["completed"] == float64(0)
	stored, err := runtime.GetRun(ctx, created.BatchRunID)
	if err != nil {
		t.Fatalf("load cross-instance running parent: %v", err)
	}
	release()
	if !runningReadOnly {
		t.Fatalf("cross-instance running report = %#v", payload)
	}
	if stored.Status != store.StatusRunning {
		t.Fatalf("cross-instance GET mutated running parent = %#v", stored)
	}
	ownerReport := waitAPICaseBatchReport(t, owner.URL+created.ReportURL)
	if !ownerReport.OK || ownerReport.Status != store.StatusPassed {
		t.Fatalf("owner completed report = %#v", ownerReport)
	}
	readerPayload := decodeJSONResponse(t, reader.URL+created.ReportURL, http.StatusOK)
	if readerPayload["status"] != store.StatusPassed || readerPayload["completed"] != float64(1) {
		t.Fatalf("cross-instance completed report = %#v", readerPayload)
	}
}

func TestAPICaseBatchOriginalOwnerCannotCheckpointAfterLeaseTakeover(t *testing.T) {
	ctx, runtime := openAPICaseBatchSQLiteStore(t)
	target, started, release := newBlockingAPICaseBatchTarget(t, "/v1/persist/lease-takeover")
	bundle := apiCaseBatchPersistenceBundle(t, target.URL,
		apiCaseBatchPersistenceCase{id: "case.persist.lease-takeover", path: "/v1/persist/lease-takeover", order: 1},
	)
	owner := httptest.NewServer(controlplane.NewWithStore(bundle, runtime))
	t.Cleanup(owner.Close)

	created := startAPICaseBatchPersistenceRun(t, owner.URL, []string{"case.persist.lease-takeover"})
	waitForAPICaseBatchSignal(t, started, "lease takeover target request")

	current, err := runtime.GetRun(ctx, created.BatchRunID)
	if err != nil {
		t.Fatalf("load batch before lease takeover: %v", err)
	}
	summary := decodeAPICaseBatchStoreSummary(t, current.SummaryJSON)
	summary["requestId"] = "replacement-owner-state"
	summary["_lease"] = map[string]any{
		"holderIdentity":      "replacement-control-plane",
		"token":               "replacement-owner-token",
		"renewTime":           time.Now().UTC().Format(time.RFC3339Nano),
		"leaseDurationMillis": 30000,
	}
	raw, err := json.Marshal(summary)
	if err != nil {
		t.Fatalf("marshal replacement owner summary: %v", err)
	}
	expectedUpdatedAt := current.UpdatedAt
	current.SummaryJSON = string(raw)
	current.UpdatedAt = expectedUpdatedAt.Add(time.Second)
	if _, err := runtime.CompareAndSwapRun(ctx, expectedUpdatedAt, store.StatusRunning, current); err != nil {
		t.Fatalf("replace batch owner lease: %v", err)
	}
	staleReplacement := current
	staleReplacement.UpdatedAt = current.UpdatedAt.Add(time.Second)
	if _, err := runtime.CompareAndSwapRun(ctx, expectedUpdatedAt, store.StatusRunning, staleReplacement); !errors.Is(err, store.ErrRunRevisionConflict) {
		t.Fatalf("stale batch owner replacement error = %v, want revision conflict", err)
	}

	release()
	payload := waitForAPICaseBatchPayload(t, owner.URL+created.ReportURL, func(payload map[string]any) bool {
		return payload["requestId"] == "replacement-owner-state"
	})
	if payload["status"] != store.StatusRunning || payload["completed"] != float64(0) {
		t.Fatalf("report after owner lease takeover = %#v", payload)
	}
	stored, err := runtime.GetRun(ctx, created.BatchRunID)
	if err != nil {
		t.Fatalf("load batch after old owner checkpoint attempt: %v", err)
	}
	storedSummary := decodeAPICaseBatchStoreSummary(t, stored.SummaryJSON)
	if stored.Status != store.StatusRunning || storedSummary["requestId"] != "replacement-owner-state" || storedSummary["completed"] != float64(0) {
		t.Fatalf("old owner overwrote replacement owner state = %#v; summary = %#v", stored, storedSummary)
	}
	childRunID := created.BatchRunID + ".case.persist.lease-takeover"
	if child, err := runtime.GetRun(ctx, childRunID); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("old owner wrote child Run after takeover: %#v err=%v", child, err)
	}
	if caseRuns, err := runtime.ListAPICaseRuns(ctx, childRunID); err != nil || len(caseRuns) != 0 {
		t.Fatalf("old owner wrote API Case Run after takeover: %#v err=%v", caseRuns, err)
	}
	if evidence, err := runtime.ListEvidence(ctx, childRunID); err != nil || len(evidence) != 0 {
		t.Fatalf("old owner wrote child Evidence after takeover: %#v err=%v", evidence, err)
	}
}

func TestAPICaseBatchCheckpointsEachCompletedCase(t *testing.T) {
	ctx, runtime := openAPICaseBatchSQLiteStore(t)
	secondStarted := make(chan struct{})
	secondRelease := make(chan struct{})
	var signalOnce sync.Once
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/checkpoint/second" {
			signalOnce.Do(func() { close(secondStarted) })
			<-secondRelease
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	t.Cleanup(target.Close)
	var releaseOnce sync.Once
	t.Cleanup(func() { releaseOnce.Do(func() { close(secondRelease) }) })
	bundle := apiCaseBatchPersistenceBundle(t, target.URL,
		apiCaseBatchPersistenceCase{id: "case.checkpoint.first", path: "/v1/checkpoint/first", order: 1},
		apiCaseBatchPersistenceCase{id: "case.checkpoint.second", path: "/v1/checkpoint/second", order: 2},
	)
	server := httptest.NewServer(controlplane.NewWithStore(bundle, runtime))
	t.Cleanup(server.Close)

	created := startAPICaseBatchPersistenceRun(t, server.URL, []string{"case.checkpoint.first", "case.checkpoint.second"})
	waitForAPICaseBatchSignal(t, secondStarted, "second target request")
	summary := waitForStoredAPICaseBatchSummary(t, ctx, runtime, created.BatchRunID, func(summary map[string]any) bool {
		return summary["completed"] == float64(1)
	})
	cases, _ := summary["cases"].([]any)
	if len(cases) != 2 || cases[0].(map[string]any)["status"] != store.StatusPassed || cases[1].(map[string]any)["status"] != store.StatusRunning {
		t.Fatalf("checkpointed case states = %#v", cases)
	}

	releaseOnce.Do(func() { close(secondRelease) })
	report := waitAPICaseBatchReport(t, server.URL+created.ReportURL)
	if report.Completed != 2 || report.Passed != 2 {
		t.Fatalf("completed checkpoint batch = %#v", report)
	}
}

func TestAPICaseBatchReportRecoversFromStoreAfterRestart(t *testing.T) {
	ctx, runtime := openAPICaseBatchSQLiteStore(t)
	startedAt := time.Now().UTC().Add(-time.Minute)
	report := map[string]any{
		"ok":         true,
		"batchRunId": "batch.restart.running",
		"requestId":  "restart-running",
		"profileId":  "sample",
		"status":     store.StatusRunning,
		"total":      1,
		"completed":  0,
		"startedAt":  startedAt.Format(time.RFC3339Nano),
		"_lease": map[string]any{
			"holderIdentity":      "stopped-control-plane",
			"token":               "expired-owner-token",
			"renewTime":           startedAt.Format(time.RFC3339Nano),
			"leaseDurationMillis": 1000,
		},
		"cases": []map[string]any{{
			"caseId": "case.restart.running",
			"status": store.StatusRunning,
		}},
	}
	raw, err := json.Marshal(report)
	if err != nil {
		t.Fatalf("marshal running batch fixture: %v", err)
	}
	if _, err := runtime.CreateRun(ctx, store.Run{
		ID:          "batch.restart.running",
		ProfileID:   "sample",
		Status:      store.StatusRunning,
		SummaryJSON: string(raw),
		StartedAt:   startedAt,
		CreatedAt:   startedAt,
		UpdatedAt:   startedAt,
	}); err != nil {
		t.Fatalf("seed running batch parent: %v", err)
	}

	restarted := httptest.NewServer(controlplane.NewWithStore(profile.Bundle{ID: "sample"}, runtime))
	t.Cleanup(restarted.Close)
	payload := decodeJSONResponse(t, restarted.URL+"/api/cases/batch-runs/batch.restart.running", http.StatusOK)
	if payload["status"] != store.StatusFailed || payload["failureCategory"] != "interrupted-unknown-outcome" {
		t.Fatalf("recovered running batch = %#v", payload)
	}
	cases := payload["cases"].([]any)
	item := cases[0].(map[string]any)
	if item["status"] != store.StatusFailed || item["failureCategory"] != "interrupted-unknown-outcome" {
		t.Fatalf("recovered running case = %#v", item)
	}
	stored, err := runtime.GetRun(ctx, "batch.restart.running")
	if err != nil {
		t.Fatalf("load persisted interrupted batch: %v", err)
	}
	if stored.Status != store.StatusFailed || !strings.Contains(stored.SummaryJSON, "interrupted-unknown-outcome") {
		t.Fatalf("persisted interrupted batch = %#v", stored)
	}

	report["batchRunId"] = "batch.restart.checkpoint-failure"
	raw, err = json.Marshal(report)
	if err != nil {
		t.Fatalf("marshal checkpoint failure fixture: %v", err)
	}
	if _, err := runtime.CreateRun(ctx, store.Run{
		ID:          "batch.restart.checkpoint-failure",
		ProfileID:   "sample",
		Status:      store.StatusRunning,
		SummaryJSON: string(raw),
		StartedAt:   startedAt,
		CreatedAt:   startedAt,
		UpdatedAt:   startedAt,
	}); err != nil {
		t.Fatalf("seed recovery checkpoint failure parent: %v", err)
	}
	failing := checkpointFailingStore{Store: runtime, err: errors.New("recovery checkpoint unavailable")}
	failingServer := httptest.NewServer(controlplane.NewWithStore(profile.Bundle{ID: "sample"}, failing))
	t.Cleanup(failingServer.Close)
	errorPayload := decodeJSONResponse(t, failingServer.URL+"/api/cases/batch-runs/batch.restart.checkpoint-failure", http.StatusInternalServerError)
	if !strings.Contains(valueStringForBatchPersistenceTest(errorPayload["error"]), "recovery checkpoint unavailable") {
		t.Fatalf("recovery checkpoint error = %#v", errorPayload)
	}
}

func TestAPICaseBatchSurfacesCheckpointPersistenceFailure(t *testing.T) {
	_, runtime := openAPICaseBatchSQLiteStore(t)
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	t.Cleanup(target.Close)
	bundle := apiCaseBatchPersistenceBundle(t, target.URL,
		apiCaseBatchPersistenceCase{id: "case.checkpoint.failure", path: "/v1/checkpoint/failure", order: 1},
	)
	failing := checkpointFailingStore{Store: runtime, err: errors.New("checkpoint unavailable")}
	server := httptest.NewServer(controlplane.NewWithStore(bundle, failing))
	t.Cleanup(server.Close)

	created := startAPICaseBatchPersistenceRun(t, server.URL, []string{"case.checkpoint.failure"})
	payload := waitForAPICaseBatchPayload(t, server.URL+created.ReportURL, func(payload map[string]any) bool {
		return payload["status"] != store.StatusRunning
	})
	if payload["ok"] != false || payload["status"] != store.StatusFailed || !strings.Contains(valueStringForBatchPersistenceTest(payload["error"]), "checkpoint unavailable") {
		t.Fatalf("checkpoint persistence failure report = %#v", payload)
	}
}

func TestAPICaseBatchSurfacesReportEvidencePersistenceFailure(t *testing.T) {
	ctx, runtime := openAPICaseBatchSQLiteStore(t)
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	t.Cleanup(target.Close)
	bundle := apiCaseBatchPersistenceBundle(t, target.URL,
		apiCaseBatchPersistenceCase{id: "case.evidence.failure", path: "/v1/evidence/failure", order: 1},
	)
	failing := reportEvidenceFailingStore{Store: runtime, err: errors.New("report evidence unavailable")}
	server := httptest.NewServer(controlplane.NewWithStore(bundle, failing))
	t.Cleanup(server.Close)

	created := startAPICaseBatchPersistenceRun(t, server.URL, []string{"case.evidence.failure"})
	payload := waitForAPICaseBatchPayload(t, server.URL+created.ReportURL, func(payload map[string]any) bool {
		return payload["status"] != store.StatusRunning
	})
	if payload["ok"] != false || payload["status"] != store.StatusFailed || !strings.Contains(valueStringForBatchPersistenceTest(payload["error"]), "report evidence unavailable") {
		t.Fatalf("report evidence persistence failure = %#v", payload)
	}
	run, err := runtime.GetRun(ctx, created.BatchRunID)
	if err != nil {
		t.Fatalf("load failed report parent: %v", err)
	}
	if run.Status != store.StatusFailed || !strings.Contains(run.SummaryJSON, "report evidence unavailable") {
		t.Fatalf("stored failed report parent = %#v", run)
	}
}

func TestAPICaseBatchPersistsStructuredExecutionFailureResult(t *testing.T) {
	ctx, runtime := openAPICaseBatchSQLiteStore(t)
	bundle := apiCaseBatchPersistenceBundle(t, "://invalid-base-url",
		apiCaseBatchPersistenceCase{id: "case.execution.failure", path: "/v1/execution/failure", order: 1},
	)
	server := httptest.NewServer(controlplane.NewWithStore(bundle, runtime))
	t.Cleanup(server.Close)

	created := startAPICaseBatchPersistenceRun(t, server.URL, []string{"case.execution.failure"})
	payload := waitForAPICaseBatchPayload(t, server.URL+created.ReportURL, func(payload map[string]any) bool {
		return payload["status"] != store.StatusRunning
	})
	cases, _ := payload["cases"].([]any)
	if len(cases) != 1 {
		t.Fatalf("structured execution failure cases = %#v", cases)
	}
	item := cases[0].(map[string]any)
	if item["status"] != store.StatusFailed || item["failureCategory"] != "configuration-error" || !strings.Contains(valueStringForBatchPersistenceTest(item["error"]), "parse base url") {
		t.Fatalf("structured execution failure item = %#v", item)
	}
	runID := valueStringForBatchPersistenceTest(item["runId"])
	if runID == "" || valueStringForBatchPersistenceTest(item["caseRunId"]) == "" {
		t.Fatalf("structured execution failure ids = %#v", item)
	}
	child, err := runtime.GetRun(ctx, runID)
	if err != nil {
		t.Fatalf("load structured execution failure child: %v", err)
	}
	if child.Status != store.StatusFailed {
		t.Fatalf("stored structured execution failure child = %#v", child)
	}
}

func TestAPICaseBatchEvidenceWriteFailureStopsRemainingCases(t *testing.T) {
	ctx, runtime := openAPICaseBatchSQLiteStore(t)
	var hits atomic.Int32
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	t.Cleanup(target.Close)
	bundle := apiCaseBatchPersistenceBundle(t, target.URL,
		apiCaseBatchPersistenceCase{id: "case.evidence.before", path: "/v1/evidence/before", order: 1},
		apiCaseBatchPersistenceCase{id: "case.evidence.write-failure", path: "/v1/evidence/write-failure", order: 2},
		apiCaseBatchPersistenceCase{id: "case.evidence.must-not-run", path: "/v1/evidence/must-not-run", order: 3},
	)
	occupied := filepath.Join(t.TempDir(), "occupied-evidence-root")
	if err := os.WriteFile(occupied, []byte("not a directory"), 0o644); err != nil {
		t.Fatalf("write occupied Evidence root: %v", err)
	}
	bundle.APICases[1].EvidenceDir = occupied
	server := httptest.NewServer(controlplane.NewWithStore(bundle, runtime))
	t.Cleanup(server.Close)

	created := startAPICaseBatchPersistenceRun(t, server.URL, []string{
		"case.evidence.before", "case.evidence.write-failure", "case.evidence.must-not-run",
	})
	payload := waitForAPICaseBatchPayload(t, server.URL+created.ReportURL, func(payload map[string]any) bool {
		return payload["status"] != store.StatusRunning
	})
	if hits.Load() != 1 {
		t.Fatalf("requests after Evidence write failure = %d, want only the first case", hits.Load())
	}
	cases := payload["cases"].([]any)
	failed := cases[1].(map[string]any)
	skipped := cases[2].(map[string]any)
	if failed["status"] != store.StatusFailed || failed["failurePhase"] != "persistence" || failed["failureCategory"] != "batch-persistence-error" {
		t.Fatalf("Evidence write failure case = %#v", failed)
	}
	if valueStringForBatchPersistenceTest(failed["runId"]) == "" || valueStringForBatchPersistenceTest(failed["evidencePath"]) == "" {
		t.Fatalf("Evidence write failure lost partial result handles = %#v", failed)
	}
	if skipped["status"] != store.StatusSkipped || skipped["failureCategory"] != "batch-persistence-error" {
		t.Fatalf("case after Evidence write failure = %#v", skipped)
	}
	parent, err := runtime.GetRun(ctx, created.BatchRunID)
	if err != nil || parent.Status != store.StatusFailed || !strings.Contains(parent.SummaryJSON, "batch-persistence-error") {
		t.Fatalf("Evidence write failure parent = %#v err=%v", parent, err)
	}
}

func TestAPICaseBatchChildIndexFailureStopsRemainingCases(t *testing.T) {
	ctx, runtime := openAPICaseBatchSQLiteStore(t)
	var hits atomic.Int32
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	t.Cleanup(target.Close)
	bundle := apiCaseBatchPersistenceBundle(t, target.URL,
		apiCaseBatchPersistenceCase{id: "case.index.failure", path: "/v1/index/failure", order: 1},
		apiCaseBatchPersistenceCase{id: "case.index.must-not-run", path: "/v1/index/must-not-run", order: 2},
	)
	failing := reportEvidenceFailingStore{Store: runtime, err: errors.New("child Evidence index unavailable")}
	server := httptest.NewServer(controlplane.NewWithStore(bundle, failing))
	t.Cleanup(server.Close)

	created := startAPICaseBatchPersistenceRun(t, server.URL, []string{"case.index.failure", "case.index.must-not-run"})
	payload := waitForAPICaseBatchPayload(t, server.URL+created.ReportURL, func(payload map[string]any) bool {
		return payload["status"] != store.StatusRunning
	})
	if hits.Load() != 1 {
		t.Fatalf("requests after child index failure = %d, want 1", hits.Load())
	}
	cases := payload["cases"].([]any)
	failed := cases[0].(map[string]any)
	skipped := cases[1].(map[string]any)
	if failed["failurePhase"] != "persistence" || failed["failureCategory"] != "batch-persistence-error" || !strings.Contains(valueStringForBatchPersistenceTest(failed["error"]), "child Evidence index unavailable") {
		t.Fatalf("child index failure case = %#v", failed)
	}
	if skipped["status"] != store.StatusSkipped || skipped["failureCategory"] != "batch-persistence-error" {
		t.Fatalf("case after child index failure = %#v", skipped)
	}
	childRunID := valueStringForBatchPersistenceTest(failed["runId"])
	child, err := runtime.GetRun(ctx, childRunID)
	if err != nil || child.Status != store.StatusPassed {
		t.Fatalf("partially indexed child Run = %#v err=%v", child, err)
	}
	parent, err := runtime.GetRun(ctx, created.BatchRunID)
	if err != nil || parent.Status != store.StatusFailed || !strings.Contains(parent.SummaryJSON, "batch-persistence-error") {
		t.Fatalf("child index failure parent = %#v err=%v", parent, err)
	}
}

func TestAPICaseBatchFinalizationStopsAfterLeaseTakeover(t *testing.T) {
	ctx, runtime := openAPICaseBatchSQLiteStore(t)
	target := newEnvironmentAcceptanceTarget(t)
	t.Cleanup(target.Close)
	bundle := environmentAcceptanceBundle(t, target.URL)
	replaceEnvironmentAcceptanceCatalog(t, ctx, runtime, bundle)
	fenced := &takeoverAfterCompletedCaseStore{Store: runtime}
	server := httptest.NewServer(controlplane.NewWithStore(bundle, fenced))
	t.Cleanup(server.Close)
	registerEnvironmentAcceptance(t, server.URL)
	before, err := runtime.GetEnvironment(ctx, "env.acceptance")
	if err != nil {
		t.Fatalf("load environment before takeover: %v", err)
	}

	started := postJSONResponse(t, server.URL+"/api/environments/env.acceptance/acceptance-runs", `{"requestId":"finalization-takeover"}`, http.StatusAccepted)
	batchRunID := valueStringForBatchPersistenceTest(started["batchRunId"])
	reportURL := valueStringForBatchPersistenceTest(started["reportUrl"])
	if batchRunID == "" || reportURL == "" {
		t.Fatalf("start finalization takeover batch = %#v", started)
	}
	payload := waitForAPICaseBatchPayload(t, server.URL+reportURL, func(payload map[string]any) bool {
		return payload["requestId"] == "replacement-finalizer-state"
	})
	if payload["status"] != store.StatusRunning {
		t.Fatalf("replacement finalizer parent = %#v", payload)
	}
	if err := fenced.takeoverError(); err != nil {
		t.Fatalf("replace finalizer lease: %v", err)
	}

	childRunID := batchRunID + ".step.env.acceptance.case.env.acceptance"
	if child, err := runtime.GetRun(ctx, childRunID); err != nil || child.Status != store.StatusPassed {
		t.Fatalf("child should be indexed before finalization takeover: %#v err=%v", child, err)
	}
	after, err := runtime.GetEnvironment(ctx, "env.acceptance")
	if err != nil {
		t.Fatalf("load environment after takeover: %v", err)
	}
	if after.Status != before.Status || after.LastVerificationRunID != before.LastVerificationRunID || after.LastVerificationStatus != before.LastVerificationStatus || after.EvidenceComplete != before.EvidenceComplete || after.TopologyComplete != before.TopologyComplete {
		t.Fatalf("old owner finalized environment after takeover: before=%#v after=%#v", before, after)
	}
	if records, err := runtime.ListEvidence(ctx, batchRunID); err != nil || len(records) != 0 {
		t.Fatalf("old owner indexed report artifacts after takeover: %#v err=%v", records, err)
	}
	if topologies, err := runtime.ListTraceTopologies(ctx, batchRunID); err != nil || len(topologies) != 0 {
		t.Fatalf("old owner copied batch topology after takeover: %#v err=%v", topologies, err)
	}
}

type takeoverAfterCompletedCaseStore struct {
	store.Store
	mu  sync.Mutex
	err error
}

func (s *takeoverAfterCompletedCaseStore) CompareAndSwapRun(ctx context.Context, expectedUpdatedAt time.Time, expectedStatus string, run store.Run) (store.Run, error) {
	updated, err := s.Store.(store.RunCompareAndSwapStore).CompareAndSwapRun(ctx, expectedUpdatedAt, expectedStatus, run)
	if err != nil || run.Status != store.StatusRunning || !strings.HasPrefix(run.ID, "batch.") {
		return updated, err
	}
	summary := decodeAPICaseBatchStoreSummaryForTakeover(run.SummaryJSON)
	if summary["completed"] != float64(1) {
		return updated, nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.err != nil {
		return updated, nil
	}
	if lease, _ := summary["_lease"].(map[string]any); lease["holderIdentity"] == "replacement-finalizer" {
		return updated, nil
	}
	summary["requestId"] = "replacement-finalizer-state"
	summary["_lease"] = map[string]any{
		"holderIdentity":      "replacement-finalizer",
		"token":               "replacement-finalizer-token",
		"renewTime":           time.Now().UTC().Format(time.RFC3339Nano),
		"leaseDurationMillis": 30000,
	}
	raw, marshalErr := json.Marshal(summary)
	if marshalErr != nil {
		s.err = marshalErr
		return updated, nil
	}
	replacement := updated
	replacement.SummaryJSON = string(raw)
	replacement.UpdatedAt = updated.UpdatedAt.Add(time.Second)
	_, s.err = s.Store.(store.RunCompareAndSwapStore).CompareAndSwapRun(ctx, updated.UpdatedAt, store.StatusRunning, replacement)
	return updated, nil
}

func (s *takeoverAfterCompletedCaseStore) takeoverError() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.err
}

func decodeAPICaseBatchStoreSummaryForTakeover(raw string) map[string]any {
	var summary map[string]any
	_ = json.Unmarshal([]byte(raw), &summary)
	return summary
}

type checkpointFailingStore struct {
	store.Store
	err error
}

func (s checkpointFailingStore) UpdateRun(context.Context, store.Run) (store.Run, error) {
	return store.Run{}, s.err
}

func (s checkpointFailingStore) CompareAndSwapRun(context.Context, time.Time, string, store.Run) (store.Run, error) {
	return store.Run{}, s.err
}

type reportEvidenceFailingStore struct {
	store.Store
	err error
}

func (s reportEvidenceFailingStore) UpdateRun(ctx context.Context, run store.Run) (store.Run, error) {
	return s.Store.(store.RunUpdater).UpdateRun(ctx, run)
}

func (s reportEvidenceFailingStore) CompareAndSwapRun(ctx context.Context, expectedUpdatedAt time.Time, expectedStatus string, run store.Run) (store.Run, error) {
	return s.Store.(store.RunCompareAndSwapStore).CompareAndSwapRun(ctx, expectedUpdatedAt, expectedStatus, run)
}

func (s reportEvidenceFailingStore) RecordEvidence(context.Context, store.EvidenceRecord) (store.EvidenceRecord, error) {
	return store.EvidenceRecord{}, s.err
}

type apiCaseBatchPersistenceCase struct {
	id    string
	path  string
	order int
}

func apiCaseBatchPersistenceBundle(t *testing.T, targetURL string, cases ...apiCaseBatchPersistenceCase) profile.Bundle {
	t.Helper()
	dir := t.TempDir()
	items := make([]profile.APICase, 0, len(cases))
	for _, item := range cases {
		items = append(items, profile.APICase{
			ID:          item.id,
			DisplayName: item.id,
			NodeID:      "node." + item.id,
			CasePath:    writeAPICaseBatchGETCase(t, dir, item.id, item.path),
			BaseURL:     targetURL,
			EvidenceDir: filepath.Join(dir, "evidence"),
			SortOrder:   item.order,
		})
	}
	return profile.Bundle{ID: "sample", APICases: items}
}

func newBlockingAPICaseBatchTarget(t *testing.T, path string) (*httptest.Server, <-chan struct{}, func()) {
	t.Helper()
	started := make(chan struct{})
	release := make(chan struct{})
	var startedOnce sync.Once
	var releaseOnce sync.Once
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != path {
			http.NotFound(w, r)
			return
		}
		startedOnce.Do(func() { close(started) })
		<-release
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	releaseFunc := func() { releaseOnce.Do(func() { close(release) }) }
	t.Cleanup(target.Close)
	t.Cleanup(releaseFunc)
	return target, started, releaseFunc
}

func startAPICaseBatchPersistenceRun(t *testing.T, serverURL string, caseIDs []string) apiCaseBatchRunCreatedForTest {
	t.Helper()
	rawIDs, err := json.Marshal(caseIDs)
	if err != nil {
		t.Fatalf("marshal case ids: %v", err)
	}
	var created apiCaseBatchRunCreatedForTest
	postJSONInto(t, serverURL+"/api/cases/batch-runs", `{"requestId":"batch-persistence","caseIds":`+string(rawIDs)+`}`, http.StatusAccepted, &created)
	if created.BatchRunID == "" || created.ReportURL == "" {
		t.Fatalf("start batch persistence run = %#v", created)
	}
	return created
}

func waitForAPICaseBatchSignal(t *testing.T, signal <-chan struct{}, label string) {
	t.Helper()
	select {
	case <-signal:
	case <-time.After(5 * time.Second):
		t.Fatalf("timed out waiting for %s", label)
	}
}

func waitForStoredAPICaseBatchSummary(t *testing.T, ctx context.Context, runtime store.Store, runID string, ready func(map[string]any) bool) map[string]any {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		run, err := runtime.GetRun(ctx, runID)
		if err == nil {
			summary := decodeAPICaseBatchStoreSummary(t, run.SummaryJSON)
			if ready(summary) {
				return summary
			}
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for stored batch summary %s: %v", runID, err)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func decodeAPICaseBatchStoreSummary(t *testing.T, raw string) map[string]any {
	t.Helper()
	var summary map[string]any
	if err := json.Unmarshal([]byte(raw), &summary); err != nil {
		t.Fatalf("decode stored batch summary: %v\n%s", err, raw)
	}
	return summary
}

func waitForAPICaseBatchPayload(t *testing.T, reportURL string, ready func(map[string]any) bool) map[string]any {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		payload := decodeJSONResponse(t, reportURL, http.StatusOK)
		if ready(payload) {
			return payload
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for batch payload: %#v", payload)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func valueStringForBatchPersistenceTest(value any) string {
	text, _ := value.(string)
	return text
}
