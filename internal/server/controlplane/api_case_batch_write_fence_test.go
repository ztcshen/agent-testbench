package controlplane

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

	"agent-testbench/internal/runner/apicase"
	"agent-testbench/internal/store"
	"agent-testbench/internal/store/sqlite"
)

const apiCaseBatchFenceTestLease = 40 * time.Millisecond

type apiCaseBatchFenceTestHarness struct {
	ctx    context.Context
	store  *sqlite.Store
	runner *apiCaseBatchRunner
	report apiCaseBatchRunReport
}

func newAPICaseBatchFenceTestHarness(t *testing.T) apiCaseBatchFenceTestHarness {
	t.Helper()
	ctx := context.Background()
	runtime, err := sqlite.Open(ctx, sqlite.Config{Path: filepath.Join(t.TempDir(), "sandbox.sqlite")})
	if err != nil {
		t.Fatalf("open fence test Store: %v", err)
	}
	t.Cleanup(func() { _ = runtime.Close() })
	runner := newAPICaseBatchRunner(apiCaseBatchFenceTestLease)
	now := time.Now().UTC()
	report := apiCaseBatchRunReport{
		OK:         true,
		BatchRunID: "batch.write-fence." + strings.ReplaceAll(t.Name(), "/", "."),
		RequestID:  "write-fence",
		ProfileID:  "sample",
		Status:     store.StatusRunning,
		StartedAt:  now.Format(time.RFC3339Nano),
	}
	report.lease = runner.newLease(now)
	if err := createAPICaseBatchRunParent(ctx, runtime, report); err != nil {
		t.Fatalf("create fence test batch parent: %v", err)
	}
	runner.save(report)
	return apiCaseBatchFenceTestHarness{ctx: ctx, store: runtime, runner: runner, report: report}
}

func (h apiCaseBatchFenceTestHarness) recoverExpiredOwner(t *testing.T) {
	t.Helper()
	time.Sleep(3 * apiCaseBatchFenceTestLease)
	recovered, ok, err := storedAPICaseBatchRunReport(h.ctx, h.store, h.report.BatchRunID)
	if err != nil {
		t.Fatalf("recover expired batch owner: %v", err)
	}
	if !ok || recovered.Status != store.StatusFailed || recovered.FailureCategory != apiCaseBatchInterruptedFailureCategory {
		t.Fatalf("recovered batch = %#v", recovered)
	}
}

func TestAPICaseBatchFencesEveryChildStoreWriteAfterTakeover(t *testing.T) {
	h := newAPICaseBatchFenceTestHarness(t)
	blocked := newBlockingAPICaseBatchMutationStore(h.store, "child-run")
	t.Cleanup(blocked.release)
	evidenceRoot := t.TempDir()
	for name, raw := range map[string]string{
		"case.json":       `{"id":"case.write-fence"}`,
		"request.json":    `{"method":"GET","path":"/v1/fence"}`,
		"assertions.json": `{"status":"passed","errors":[]}`,
		"summary.json":    `{"status":"passed"}`,
	} {
		if err := os.WriteFile(filepath.Join(evidenceRoot, name), []byte(raw), 0o644); err != nil {
			t.Fatalf("write child Evidence fixture %s: %v", name, err)
		}
	}
	now := time.Now().UTC()
	result := apicase.RunResult{
		OK: true, RunID: h.report.BatchRunID + ".case.write-fence", CaseID: "case.write-fence", Status: store.StatusPassed,
		EvidencePath: evidenceRoot, StartedAt: now.Format(time.RFC3339Nano), FinishedAt: now.Add(time.Second).Format(time.RFC3339Nano),
	}
	done := make(chan error, 1)
	go func() {
		done <- recordAPICaseRunWithContext(h.ctx, h.runner.fencedStore(blocked, h.report.BatchRunID), recordAPICaseRunContext{ProfileID: "sample"}, result)
	}()
	waitAPICaseBatchFenceSignal(t, blocked.started, "blocked child Run")
	h.recoverExpiredOwner(t)
	blocked.release()
	if err := waitAPICaseBatchFenceResult(t, done, "child composite persistence"); !errors.Is(err, errAPICaseBatchLeaseLost) {
		t.Fatalf("child composite error = %v, want lease lost", err)
	}
	if blocked.childRuns.Load() != 1 || blocked.caseRuns.Load() != 0 || blocked.evidence.Load() != 0 {
		t.Fatalf("delegated child writes after takeover: child=%d case=%d evidence=%d", blocked.childRuns.Load(), blocked.caseRuns.Load(), blocked.evidence.Load())
	}
	if rows, err := h.store.ListAPICaseRuns(h.ctx, result.RunID); err != nil || len(rows) != 0 {
		t.Fatalf("stale owner API Case Runs = %#v err=%v", rows, err)
	}
	if rows, err := h.store.ListEvidence(h.ctx, result.RunID); err != nil || len(rows) != 0 {
		t.Fatalf("stale owner child Evidence = %#v err=%v", rows, err)
	}
}

func TestAPICaseBatchFencesEveryReportArtifactWriteAfterTakeover(t *testing.T) {
	h := newAPICaseBatchFenceTestHarness(t)
	blocked := newBlockingAPICaseBatchMutationStore(h.store, "report-evidence")
	t.Cleanup(blocked.release)
	root := t.TempDir()
	h.report.FinishedAt = time.Now().UTC().Format(time.RFC3339Nano)
	h.report.HTMLReportPath = writeAPICaseBatchFenceArtifact(t, root, "report.html")
	h.report.JUnitReportPath = writeAPICaseBatchFenceArtifact(t, root, "report.junit.xml")
	h.report.ArtifactManifestPath = writeAPICaseBatchFenceArtifact(t, root, "artifacts.json")
	h.report.FailureSummaryPath = writeAPICaseBatchFenceArtifact(t, root, "failures.json")
	done := make(chan error, 1)
	go func() {
		done <- recordAPICaseBatchReportArtifacts(h.ctx, h.runner.fencedStore(blocked, h.report.BatchRunID), h.report)
	}()
	waitAPICaseBatchFenceSignal(t, blocked.started, "blocked report Evidence")
	h.recoverExpiredOwner(t)
	blocked.release()
	if err := waitAPICaseBatchFenceResult(t, done, "report artifact persistence"); !errors.Is(err, errAPICaseBatchLeaseLost) {
		t.Fatalf("report artifact error = %v, want lease lost", err)
	}
	if blocked.evidence.Load() != 1 {
		t.Fatalf("delegated report Evidence writes = %d, want only blocked first write", blocked.evidence.Load())
	}
	if rows, err := h.store.ListEvidence(h.ctx, h.report.BatchRunID); err != nil || len(rows) != 1 {
		t.Fatalf("stale owner report Evidence = %#v err=%v", rows, err)
	}
}

func TestAPICaseBatchFencesEveryCopiedTopologyWriteAfterTakeover(t *testing.T) {
	h := newAPICaseBatchFenceTestHarness(t)
	const sourceRunID = "run.write-fence.topology"
	for index := 1; index <= 2; index++ {
		if _, err := h.store.SaveTraceTopology(h.ctx, store.TraceTopology{
			ID: sourceRunID + "." + time.Duration(index).String(), WorkflowRunID: sourceRunID, WorkflowID: "workflow.write-fence",
			StepID: "step.write-fence", CaseID: "case.write-fence", Status: store.StatusPassed,
			TopologyJSON: `{"provider":"skywalking"}`, CreatedAt: time.Now().UTC(),
		}); err != nil {
			t.Fatalf("seed source topology %d: %v", index, err)
		}
	}
	h.report.WorkflowID = "workflow.write-fence"
	h.report.Cases = []apiCaseBatchCaseReport{{CaseID: "case.write-fence", StepID: "step.write-fence", RunID: sourceRunID, Status: store.StatusPassed}}
	blocked := newBlockingAPICaseBatchMutationStore(h.store, "topology")
	t.Cleanup(blocked.release)
	done := make(chan error, 1)
	go func() {
		done <- copyAPICaseBatchTraceTopologies(h.ctx, h.runner.fencedStore(blocked, h.report.BatchRunID), h.report)
	}()
	waitAPICaseBatchFenceSignal(t, blocked.started, "blocked topology copy")
	h.recoverExpiredOwner(t)
	blocked.release()
	if err := waitAPICaseBatchFenceResult(t, done, "topology copy"); !errors.Is(err, errAPICaseBatchLeaseLost) {
		t.Fatalf("topology copy error = %v, want lease lost", err)
	}
	if blocked.topologies.Load() != 1 {
		t.Fatalf("delegated topology writes = %d, want only blocked first write", blocked.topologies.Load())
	}
	if rows, err := h.store.ListTraceTopologies(h.ctx, h.report.BatchRunID); err != nil || len(rows) != 1 {
		t.Fatalf("stale owner copied topologies = %#v err=%v", rows, err)
	}
}

func TestAPICaseBatchFencesTraceCollectorPostProcessWriteAfterTakeover(t *testing.T) {
	h := newAPICaseBatchFenceTestHarness(t)
	now := time.Now().UTC()
	childRunID := h.report.BatchRunID + ".case.trace-fence"
	if _, err := h.store.CreateRun(h.ctx, store.Run{
		ID: childRunID, ProfileID: "sample", WorkflowID: "workflow.write-fence", Status: store.StatusPassed,
		StartedAt: now.Add(-time.Second), FinishedAt: now, CreatedAt: now.Add(-time.Second), UpdatedAt: now,
	}); err != nil {
		t.Fatalf("seed trace child Run: %v", err)
	}
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		var payload struct {
			Query string `json:"query"`
		}
		if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
			t.Errorf("decode trace query: %v", err)
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.Contains(payload.Query, "queryBasicTraces"):
			_, _ = w.Write([]byte(`{"data":{"queryBasicTraces":{"traces":[{"endpointNames":["GET:/v1/fence"],"duration":10,"start":"2026-07-16 0836","isError":false,"traceIds":["trace.write-fence"]}]}}}`))
		case strings.Contains(payload.Query, "queryTrace"):
			_, _ = w.Write([]byte(`{"data":{"queryTrace":{"spans":[{"traceId":"trace.write-fence","segmentId":"segment.write-fence","spanId":0,"parentSpanId":-1,"refs":[],"serviceCode":"service.write-fence","endpointName":"/v1/fence","type":"Entry","component":"Server"}]}}}`))
		default:
			http.Error(w, "unexpected trace query", http.StatusBadRequest)
		}
	}))
	t.Cleanup(provider.Close)
	blocked := newBlockingAPICaseBatchMutationStore(h.store, "trace-collector")
	t.Cleanup(blocked.release)
	fenced := h.runner.fencedStore(blocked, h.report.BatchRunID)
	done := make(chan error, 1)
	go func() {
		collectAndRecordTestKitTraceTopology(h.ctx, fenced, traceCollector{GraphQLURL: provider.URL}, childRunID, map[string]any{
			"workflowId": "workflow.write-fence", "stepId": "step.write-fence", "traceEndpoint": "/v1/fence",
		}, map[string]any{
			"ok": true, "caseId": "case.trace-fence", "stepId": "step.write-fence", "startedAt": now.Add(-time.Second).Format(time.RFC3339Nano), "finishedAt": now.Format(time.RFC3339Nano),
			"result": map[string]any{"request": map[string]any{"path": "/v1/fence"}, "response": map[string]any{"headers": map[string]any{"Request-Id": "request.write-fence"}}},
		})
		done <- fenced.FenceError()
	}()
	waitAPICaseBatchFenceSignal(t, blocked.started, "blocked trace collector topology")
	h.recoverExpiredOwner(t)
	blocked.release()
	if err := waitAPICaseBatchFenceResult(t, done, "trace collector"); !errors.Is(err, errAPICaseBatchLeaseLost) {
		t.Fatalf("trace collector fence error = %v, want lease lost", err)
	}
	if blocked.topologies.Load() != 1 || blocked.tasks.Load() != 0 {
		t.Fatalf("delegated trace writes after takeover: topology=%d task=%d", blocked.topologies.Load(), blocked.tasks.Load())
	}
	if tasks, err := h.store.ListPostProcessTasks(h.ctx, childRunID); err != nil || len(tasks) != 0 {
		t.Fatalf("stale owner post-process tasks = %#v err=%v", tasks, err)
	}
}

func TestAPICaseBatchFencesEnvironmentWriteAfterBlockedReadAndTakeover(t *testing.T) {
	h := newAPICaseBatchFenceTestHarness(t)
	now := time.Now().UTC()
	before, err := h.store.UpsertEnvironment(h.ctx, store.Environment{
		ID: "env.write-fence", Status: "registered", VerificationWorkflowID: "workflow.write-fence", CreatedAt: now, UpdatedAt: now,
	})
	if err != nil {
		t.Fatalf("seed fence environment: %v", err)
	}
	h.report.EnvironmentID = before.ID
	h.report.WorkflowID = before.VerificationWorkflowID
	h.report.Acceptance.OK = true
	blocked := newBlockingAPICaseBatchMutationStore(h.store, "environment-read")
	t.Cleanup(blocked.release)
	done := make(chan error, 1)
	go func() {
		done <- finalizeEnvironmentAcceptanceRun(h.ctx, h.runner.fencedStore(blocked, h.report.BatchRunID), h.report)
	}()
	waitAPICaseBatchFenceSignal(t, blocked.started, "blocked environment read")
	h.recoverExpiredOwner(t)
	blocked.release()
	if err := waitAPICaseBatchFenceResult(t, done, "environment finalization"); !errors.Is(err, errAPICaseBatchLeaseLost) {
		t.Fatalf("environment finalization error = %v, want lease lost", err)
	}
	if blocked.environments.Load() != 0 {
		t.Fatalf("delegated environment writes after takeover = %d", blocked.environments.Load())
	}
	after, err := h.store.GetEnvironment(h.ctx, before.ID)
	if err != nil {
		t.Fatalf("load fence environment: %v", err)
	}
	if after.Status != before.Status || after.LastVerificationRunID != before.LastVerificationRunID {
		t.Fatalf("stale owner changed environment: before=%#v after=%#v", before, after)
	}
}

func TestAPICaseBatchFencesEachReportFileWriteAfterTakeover(t *testing.T) {
	h := newAPICaseBatchFenceTestHarness(t)
	h.report.Total = 1
	h.report.Cases = []apiCaseBatchCaseReport{{CaseID: "case.report-write-fence", Status: store.StatusRunning}}
	h.runner.save(h.report)
	started := make(chan struct{})
	release := make(chan struct{})
	var releaseOnce sync.Once
	t.Cleanup(func() { releaseOnce.Do(func() { close(release) }) })
	var laterWrites atomic.Int32
	h.runner.reportWriters = []func(apiCaseBatchRunReport) error{
		func(apiCaseBatchRunReport) error {
			close(started)
			<-release
			return nil
		},
		func(apiCaseBatchRunReport) error {
			laterWrites.Add(1)
			return nil
		},
	}
	done := make(chan error, 1)
	go func() {
		done <- h.runner.updateCase(h.ctx, h.store, h.report.BatchRunID, 0, apiCaseBatchCaseReport{CaseID: "case.report-write-fence", Status: store.StatusPassed})
	}()
	waitAPICaseBatchFenceSignal(t, started, "blocked report file")
	h.recoverExpiredOwner(t)
	releaseOnce.Do(func() { close(release) })
	if err := waitAPICaseBatchFenceResult(t, done, "report file writes"); !errors.Is(err, errAPICaseBatchLeaseLost) {
		t.Fatalf("report file error = %v, want lease lost", err)
	}
	if laterWrites.Load() != 0 {
		t.Fatalf("report file writes after takeover = %d", laterWrites.Load())
	}
	if stale, ok := h.runner.get(h.report.BatchRunID); ok {
		t.Fatalf("stale owner report was re-added after takeover: %#v", stale)
	}
	request := httptest.NewRequest(http.MethodGet, "/api/cases/batch-runs/"+h.report.BatchRunID, nil)
	response := httptest.NewRecorder()
	handleAPICaseBatchRunReport(response, request, h.store, h.runner)
	if response.Code != http.StatusOK {
		t.Fatalf("Store-backed recovered GET status = %d body=%s", response.Code, response.Body.String())
	}
	var payload map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode Store-backed recovered GET: %v", err)
	}
	if payload["status"] != store.StatusFailed || payload["failureCategory"] != apiCaseBatchInterruptedFailureCategory {
		t.Fatalf("Store-backed recovered GET = %#v", payload)
	}
}

type blockingAPICaseBatchMutationStore struct {
	store.Store
	mode         string
	started      chan struct{}
	releaseCh    chan struct{}
	releaseOnce  sync.Once
	blocked      atomic.Bool
	childRuns    atomic.Int32
	caseRuns     atomic.Int32
	evidence     atomic.Int32
	topologies   atomic.Int32
	tasks        atomic.Int32
	environments atomic.Int32
}

func newBlockingAPICaseBatchMutationStore(runtime store.Store, mode string) *blockingAPICaseBatchMutationStore {
	return &blockingAPICaseBatchMutationStore{Store: runtime, mode: mode, started: make(chan struct{}), releaseCh: make(chan struct{})}
}

func (s *blockingAPICaseBatchMutationStore) block(mode string) {
	if s.mode != mode || !s.blocked.CompareAndSwap(false, true) {
		return
	}
	close(s.started)
	<-s.releaseCh
}

func (s *blockingAPICaseBatchMutationStore) release() {
	s.releaseOnce.Do(func() { close(s.releaseCh) })
}

func (s *blockingAPICaseBatchMutationStore) CompareAndSwapRun(ctx context.Context, expectedUpdatedAt time.Time, expectedStatus string, run store.Run) (store.Run, error) {
	return s.Store.(store.RunCompareAndSwapStore).CompareAndSwapRun(ctx, expectedUpdatedAt, expectedStatus, run)
}

func (s *blockingAPICaseBatchMutationStore) CreateRun(ctx context.Context, run store.Run) (store.Run, error) {
	if s.mode == "child-run" {
		s.block("child-run")
		s.childRuns.Add(1)
	}
	return s.Store.CreateRun(ctx, run)
}

func (s *blockingAPICaseBatchMutationStore) RecordAPICaseRun(ctx context.Context, run store.APICaseRun) (store.APICaseRun, error) {
	s.caseRuns.Add(1)
	return s.Store.RecordAPICaseRun(ctx, run)
}

func (s *blockingAPICaseBatchMutationStore) RecordEvidence(ctx context.Context, evidence store.EvidenceRecord) (store.EvidenceRecord, error) {
	s.block("report-evidence")
	s.evidence.Add(1)
	return s.Store.RecordEvidence(ctx, evidence)
}

func (s *blockingAPICaseBatchMutationStore) SaveTraceTopology(ctx context.Context, topology store.TraceTopology) (store.TraceTopology, error) {
	s.block("topology")
	s.block("trace-collector")
	s.topologies.Add(1)
	return s.Store.SaveTraceTopology(ctx, topology)
}

func (s *blockingAPICaseBatchMutationStore) RecordPostProcessTask(ctx context.Context, task store.PostProcessTask) (store.PostProcessTask, error) {
	s.tasks.Add(1)
	return s.Store.RecordPostProcessTask(ctx, task)
}

func (s *blockingAPICaseBatchMutationStore) GetEnvironment(ctx context.Context, id string) (store.Environment, error) {
	s.block("environment-read")
	return s.Store.GetEnvironment(ctx, id)
}

func (s *blockingAPICaseBatchMutationStore) UpsertEnvironment(ctx context.Context, environment store.Environment) (store.Environment, error) {
	s.environments.Add(1)
	return s.Store.UpsertEnvironment(ctx, environment)
}

func writeAPICaseBatchFenceArtifact(t *testing.T, root string, name string) string {
	t.Helper()
	path := filepath.Join(root, name)
	if err := os.WriteFile(path, []byte(name), 0o644); err != nil {
		t.Fatalf("write fence artifact %s: %v", name, err)
	}
	return path
}

func waitAPICaseBatchFenceSignal(t *testing.T, signal <-chan struct{}, label string) {
	t.Helper()
	select {
	case <-signal:
	case <-time.After(3 * time.Second):
		t.Fatalf("timed out waiting for %s", label)
	}
}

func waitAPICaseBatchFenceResult(t *testing.T, result <-chan error, label string) error {
	t.Helper()
	select {
	case err := <-result:
		return err
	case <-time.After(3 * time.Second):
		t.Fatalf("timed out waiting for %s", label)
		return nil
	}
}
