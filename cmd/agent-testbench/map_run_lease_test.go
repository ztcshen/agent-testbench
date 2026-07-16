package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"agent-testbench/internal/domain/mapplanner"
	"agent-testbench/internal/store"
)

func TestMapRunStopsAllCheckpointsWhenLeaseIsLost(t *testing.T) {
	ctx, cancel := context.WithCancelCause(context.Background())
	defer cancel(nil)
	runtime := &mapLeaseLostStore{}
	record := store.TestMapPlanRecord{
		Instance: store.TestMapPlanInstance{ID: "plan.lease-lost", MapID: "map.lease-lost", Status: store.StatusRunning},
		Tasks: []store.TestMapPlanTask{{
			ID: "task.lease-lost", PlanID: "plan.lease-lost", Kind: mapplanner.TaskRunCase,
			CaseID: "case.must-not-run", Status: mapplanner.TaskStatusPlanned,
		}},
	}
	executor := newMapRunExecutor(ctx, runtime, store.TestPlanGraph{}, mapRunOptions{})
	executor.attachLease(runtime, store.TestMapPlanLease{
		PlanID: record.Instance.ID, OwnerID: "owner.lost", Token: "token.lost", ExpiresAt: time.Now().UTC().Add(time.Minute),
	}, cancel)

	executed := executor.execute(record)

	if !errors.Is(executor.checkpointError(), store.ErrTestMapPlanLeaseLost) || !errors.Is(context.Cause(ctx), store.ErrTestMapPlanLeaseLost) {
		t.Fatalf("lease loss should cancel execution, checkpoint=%v cause=%v", executor.checkpointError(), context.Cause(ctx))
	}
	if runtime.taskUpdates.Load() != 1 || runtime.instanceUpdates.Load() != 0 || runtime.releases.Load() != 0 {
		t.Fatalf("writes after lease loss: task=%d instance=%d releases=%d", runtime.taskUpdates.Load(), runtime.instanceUpdates.Load(), runtime.releases.Load())
	}
	if executed.Tasks[0].APICaseRunID != "" || !strings.Contains(executed.Tasks[0].Reason, "persist running task checkpoint") {
		t.Fatalf("external case must not run after lease loss: %#v", executed.Tasks[0])
	}
}

func TestMapRunFencesEveryExternalCaseExecution(t *testing.T) {
	ctx, cancel := context.WithCancelCause(context.Background())
	defer cancel(nil)
	runtime := &mapExternalFenceStore{}
	executor := newMapRunExecutor(ctx, runtime, store.TestPlanGraph{}, mapRunOptions{})
	executor.attachLease(runtime, store.TestMapPlanLease{
		PlanID: "plan.external-fence", OwnerID: "owner.external-fence", Token: "token.external-fence", ExpiresAt: time.Now().UTC().Add(time.Minute),
	}, cancel)
	instance := store.TestMapPlanInstance{ID: "plan.external-fence", ProfileID: "profile.external-fence"}
	task := store.TestMapPlanTask{ID: "task.external-fence", PlanID: instance.ID, CaseID: "case.external-fence"}

	if _, err := executor.runCatalogCase(instance, task, store.TestPlanPathStep{}, "run.external-fence.1", nil); err == nil || !strings.Contains(err.Error(), "unsupported map case runner") {
		t.Fatalf("first external execution should pass its fence and reach the selected runner: %v", err)
	}
	if _, err := executor.runCatalogCase(instance, task, store.TestPlanPathStep{}, "run.external-fence.2", nil); !errors.Is(err, store.ErrTestMapPlanLeaseLost) {
		t.Fatalf("second external fence error = %v, want lease lost", err)
	}
	if runtime.renews.Load() != 2 || !errors.Is(context.Cause(ctx), store.ErrTestMapPlanLeaseLost) {
		t.Fatalf("external fences renews=%d cause=%v", runtime.renews.Load(), context.Cause(ctx))
	}
}

type mapExternalFenceStore struct {
	mapLeaseLostStore
	renews atomic.Int32
}

func (s *mapExternalFenceStore) RenewTestMapPlanLease(_ context.Context, lease store.TestMapPlanLease, now time.Time, expiresAt time.Time) (store.TestMapPlanLease, error) {
	if s.renews.Add(1) == 2 {
		return store.TestMapPlanLease{}, fmt.Errorf("%w: simulated takeover before next external case", store.ErrTestMapPlanLeaseLost)
	}
	lease.UpdatedAt = now
	lease.ExpiresAt = expiresAt
	return lease, nil
}

func (s *mapExternalFenceStore) GetProfileCatalog(context.Context) (store.ProfileCatalog, error) {
	return store.ProfileCatalog{APICases: []store.CatalogAPICase{{
		ID: "case.external-fence", SourceKind: "mq", ExecutorID: "executor.mq",
	}}}, nil
}

type mapLeaseLostStore struct {
	store.Store
	taskUpdates     atomic.Int32
	instanceUpdates atomic.Int32
	releases        atomic.Int32
}

func (s *mapLeaseLostStore) ClaimTestMapPlanLease(context.Context, store.TestMapPlanLease, time.Time, bool) (store.TestMapPlanLease, error) {
	return store.TestMapPlanLease{}, errors.New("unexpected claim")
}

func (s *mapLeaseLostStore) RenewTestMapPlanLease(context.Context, store.TestMapPlanLease, time.Time, time.Time) (store.TestMapPlanLease, error) {
	return store.TestMapPlanLease{}, errors.New("unexpected renewal")
}

func (s *mapLeaseLostStore) ResetTestMapPlanWithLease(context.Context, store.TestMapPlanLease, store.TestMapPlanRecord, time.Time, time.Time) (store.TestMapPlanLease, error) {
	return store.TestMapPlanLease{}, errors.New("unexpected reset")
}

func (s *mapLeaseLostStore) UpdateTestMapPlanTaskWithLease(context.Context, store.TestMapPlanLease, store.TestMapPlanTask, time.Time, time.Time) (store.TestMapPlanLease, error) {
	s.taskUpdates.Add(1)
	return store.TestMapPlanLease{}, fmt.Errorf("%w: simulated takeover", store.ErrTestMapPlanLeaseLost)
}

func (s *mapLeaseLostStore) UpdateTestMapPlanInstanceWithLease(context.Context, store.TestMapPlanLease, store.TestMapPlanInstance, time.Time, time.Time) (store.TestMapPlanLease, error) {
	s.instanceUpdates.Add(1)
	return store.TestMapPlanLease{}, nil
}

func (s *mapLeaseLostStore) ReleaseTestMapPlanLease(context.Context, store.TestMapPlanLease, store.TestMapPlanInstance, time.Time) error {
	s.releases.Add(1)
	return nil
}

func TestMapRunLegacyRunningPlanRequiresResumeAndExpiredLeaseDoesNotReplayUnknownTask(t *testing.T) {
	ctx := context.Background()
	storeRef := seedExecutableMapCommandStore(t, ctx)
	runCLI(t, "map", "import-workflows", "--store", storeRef, "--json")
	planID := saveMapLeaseTestPlan(t, storeRef, "plan.legacy-running", mapplanner.TaskRunPath)

	runtime, err := openStore(ctx, storeRef)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	record, err := runtime.GetTestMapPlan(ctx, planID)
	if err != nil {
		t.Fatalf("get saved map plan: %v", err)
	}
	record.Instance.Status = store.StatusRunning
	record.Instance.StartedAt = time.Now().UTC().Add(-time.Minute)
	record.Tasks[0].Status = store.StatusRunning
	record.Tasks[0].StartedAt = record.Instance.StartedAt
	checkpoint := runtime.(store.MapPlannerCheckpointStore)
	if err := checkpoint.UpdateTestMapPlanInstance(ctx, record.Instance); err != nil {
		t.Fatalf("mark legacy plan running: %v", err)
	}
	if err := checkpoint.UpdateTestMapPlanTask(ctx, record.Tasks[0]); err != nil {
		t.Fatalf("mark legacy task running: %v", err)
	}
	closeCLIStore(runtime)

	withoutResume := runCLIFails(t, "map", "run", "--store", storeRef, "--plan", planID, "--json")
	if !strings.Contains(withoutResume, "already running without a lease") || !strings.Contains(withoutResume, "use --resume") {
		t.Fatalf("legacy running plan error = %s", withoutResume)
	}

	runtime, err = openStore(ctx, storeRef)
	if err != nil {
		t.Fatalf("reopen store: %v", err)
	}
	oldNow := time.Now().UTC().Add(-2 * time.Minute)
	_, err = runtime.(store.MapPlannerLeaseStore).ClaimTestMapPlanLease(ctx, store.TestMapPlanLease{
		PlanID: planID, OwnerID: "owner.expired", Token: "token.expired", ExpiresAt: oldNow.Add(time.Second),
	}, oldNow, true)
	if err != nil {
		t.Fatalf("seed expired map plan lease: %v", err)
	}
	closeCLIStore(runtime)

	resumed := runCLIFails(t, "map", "run", "--store", storeRef, "--plan", planID, "--resume", "--json")
	if !strings.Contains(resumed, "interrupted-unknown-outcome") || !strings.Contains(resumed, "map run failed") {
		t.Fatalf("expired lease recovery output = %s", resumed)
	}
	runtime, err = openStore(ctx, storeRef)
	if err != nil {
		t.Fatalf("reopen recovered store: %v", err)
	}
	defer closeCLIStore(runtime)
	recovered, err := runtime.GetTestMapPlan(ctx, planID)
	if err != nil {
		t.Fatalf("get recovered map plan: %v", err)
	}
	if recovered.Tasks[0].Status != store.StatusFailed || recovered.Tasks[0].Reason != "interrupted-unknown-outcome" || recovered.Tasks[0].WorkflowRunID != "" || recovered.Tasks[0].APICaseRunID != "" {
		t.Fatalf("expired lease task should remain unknown and not replay: %#v", recovered.Tasks[0])
	}
}

func TestConcurrentMapRunCLIExecutesClaimedPlanOnce(t *testing.T) {
	ctx := context.Background()
	var hits atomic.Int32
	requestStarted := make(chan struct{}, 1)
	releaseRequest := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		select {
		case requestStarted <- struct{}{}:
		default:
		}
		<-releaseRequest
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprint(w, `{"ok":true}`)
	}))
	t.Cleanup(server.Close)
	storeRef := seedMapLeaseCommandStore(t, ctx, server.URL)
	planID := saveMapLeaseTestPlan(t, storeRef, "plan.concurrent-cli", mapplanner.TaskRunCase)

	first := newMapRunTestCommand(storeRef, planID)
	var firstOutput bytes.Buffer
	first.Stdout = &firstOutput
	first.Stderr = &firstOutput
	if err := first.Start(); err != nil {
		t.Fatalf("start first map run: %v", err)
	}
	select {
	case <-requestStarted:
	case <-time.After(10 * time.Second):
		close(releaseRequest)
		_ = first.Wait()
		t.Fatal("first map run did not reach the external case")
	}

	second := newMapRunTestCommand(storeRef, planID)
	secondOutput, secondErr := second.CombinedOutput()
	if secondErr == nil || !strings.Contains(string(secondOutput), "lease conflict") {
		close(releaseRequest)
		_ = first.Wait()
		t.Fatalf("second map run should lose the claim: err=%v out=%s", secondErr, secondOutput)
	}
	close(releaseRequest)
	if err := first.Wait(); err != nil {
		t.Fatalf("first map run failed: %v\n%s", err, firstOutput.String())
	}
	if hits.Load() != 1 {
		t.Fatalf("concurrent CLI map runs triggered %d external requests, want 1", hits.Load())
	}
}

func seedMapLeaseCommandStore(t *testing.T, ctx context.Context, baseURL string) string {
	t.Helper()
	storeRef := "sqlite://" + filepath.Join(t.TempDir(), "map-run-lease.sqlite")
	runtime, err := openStore(ctx, storeRef)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	catalog := mapCommandExecutableProfileCatalogFixture(baseURL)
	if err := runtime.ReplaceProfileCatalog(ctx, catalog); err != nil {
		t.Fatalf("seed map lease profile catalog: %v", err)
	}
	closeCLIStore(runtime)
	runCLI(t, "map", "import-workflows", "--store", storeRef, "--json")
	return storeRef
}

func saveMapLeaseTestPlan(t *testing.T, storeRef string, planID string, kind string) string {
	t.Helper()
	ctx := context.Background()
	runtime, err := openStore(ctx, storeRef)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer closeCLIStore(runtime)
	now := time.Now().UTC()
	task := store.TestMapPlanTask{
		ID: "task." + strings.TrimPrefix(planID, "plan."), PlanID: planID, Index: 1, Kind: kind, Operation: kind,
		Status: mapplanner.TaskStatusPlanned, CaseID: "case.prepare", NodeID: "case.prepare", CreatedAt: now,
	}
	if kind == mapplanner.TaskRunPath {
		task.PathID = "workflow.flow.create"
		task.WorkflowID = "workflow.flow.create"
		task.CaseID = ""
	}
	record := store.TestMapPlanRecord{
		Instance: store.TestMapPlanInstance{
			ID: planID, MapID: "map.profile.flow", ProfileID: "profile.fixture", Scope: mapplanner.ScopeCase,
			TargetKind: mapplanner.TargetCase, TargetID: "case.prepare", Mode: mapplanner.ModeExplain,
			Status: mapplanner.TaskStatusPlanned, CreatedAt: now,
		},
		Tasks: []store.TestMapPlanTask{task},
	}
	if err := runtime.SaveTestMapPlan(ctx, record); err != nil {
		t.Fatalf("save map lease test plan: %v", err)
	}
	return planID
}

func newMapRunTestCommand(storeRef string, planID string) *exec.Cmd {
	cmd := exec.Command(os.Args[0], "map", "run", "--store", storeRef, "--plan", planID, "--json")
	cmd.Env = append(os.Environ(), "AGENT_TESTBENCH_TEST_CLI=1")
	return cmd
}
