package main

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"agent-testbench/internal/store"
)

type recordFailingTaskStore struct {
	store.Store
}

func (recordFailingTaskStore) RecordAgentTaskRun(context.Context, store.AgentTaskRun) (store.AgentTaskRun, error) {
	return store.AgentTaskRun{}, errors.New("injected task run persistence failure")
}

type staleListedTaskStore struct {
	store.Store
	tasks []store.AgentTask
}

func (s staleListedTaskStore) ListAgentTasks(context.Context) ([]store.AgentTask, error) {
	return append([]store.AgentTask(nil), s.tasks...), nil
}

func TestTaskWorkerOnceExecutesDueIntervalTaskAndNotifies(t *testing.T) {
	storeRef := "sqlite://" + filepath.Join(t.TempDir(), "tasks.sqlite")
	notifyPath := filepath.Join(t.TempDir(), "scheduled-notify.jsonl")

	scheduleOut := runCLI(t,
		"task", "schedule", "scheduled-shell",
		"--store", storeRef,
		"--command", "printf 'scheduled-worker-ok'",
		"--interval", "1ns",
		"--shell",
		"--notify-file", notifyPath,
		"--json",
	)
	if !strings.Contains(scheduleOut, `"kind": "shell"`) || !strings.Contains(scheduleOut, `"status": "scheduled"`) {
		t.Fatalf("scheduled task report = %s", scheduleOut)
	}

	workerOut := runCLI(t, "task", "worker", "--store", storeRef, "--once", "--json")
	var report taskWorkerReport
	if err := json.Unmarshal([]byte(workerOut), &report); err != nil {
		t.Fatalf("decode worker report: %v\n%s", err, workerOut)
	}
	if !report.OK || !report.Once || report.Due != 1 || report.Claimed != 1 || report.Executed != 1 || len(report.Tasks) != 1 {
		t.Fatalf("worker report = %#v", report)
	}
	if report.Tasks[0].Run.Status != store.StatusPassed || !strings.Contains(report.Tasks[0].Run.Output, "scheduled-worker-ok") {
		t.Fatalf("worker task run = %#v", report.Tasks[0])
	}
	if !strings.Contains(report.RecoveryBoundary, "automatic lease recovery is unsupported") {
		t.Fatalf("worker recovery boundary = %q", report.RecoveryBoundary)
	}
	statusOut := runCLI(t, "task", "status", "scheduled-shell", "--store", storeRef, "--json")
	if !strings.Contains(statusOut, `"status": "scheduled"`) || !strings.Contains(statusOut, `"runCount": 1`) {
		t.Fatalf("scheduled task status = %s", statusOut)
	}
	notice, err := os.ReadFile(notifyPath)
	if err != nil || !strings.Contains(string(notice), `"status":"passed"`) {
		t.Fatalf("scheduled notification = %q err=%v", notice, err)
	}
}

func TestScheduledAgentTaskDueUsesLatestMaintenanceOrRunTime(t *testing.T) {
	created := time.Date(2026, 7, 16, 8, 0, 0, 0, time.UTC)
	task := store.AgentTask{Schedule: "interval:15m", CreatedAt: created, UpdatedAt: created}
	if due, err := scheduledAgentTaskDue(task, created.Add(14*time.Minute)); err != nil || due {
		t.Fatalf("task should not yet be due: due=%t err=%v", due, err)
	}
	if due, err := scheduledAgentTaskDue(task, created.Add(15*time.Minute)); err != nil || !due {
		t.Fatalf("task should be due at interval boundary: due=%t err=%v", due, err)
	}
	task.LastRunAt = created.Add(10 * time.Minute)
	if due, err := scheduledAgentTaskDue(task, created.Add(20*time.Minute)); err != nil || due {
		t.Fatalf("last run should move due time: due=%t err=%v", due, err)
	}
}

func TestTaskScheduleRejectsCronAndNonPositiveIntervals(t *testing.T) {
	storeRef := "sqlite://" + filepath.Join(t.TempDir(), "tasks.sqlite")
	cronOut := runCLIFails(t, "task", "schedule", "cron-task", "--store", storeRef, "--command", "commands --json", "--cron", "0 * * * *", "--json")
	if !strings.Contains(cronOut, "--cron is unsupported") {
		t.Fatalf("cron rejection = %s", cronOut)
	}
	intervalOut := runCLIFails(t, "task", "schedule", "zero-task", "--store", storeRef, "--command", "commands --json", "--interval", "0s", "--json")
	if !strings.Contains(intervalOut, "--interval must be greater than zero") {
		t.Fatalf("interval rejection = %s", intervalOut)
	}
}

func TestCommandCatalogDescribesWorkerAndMapConcurrency(t *testing.T) {
	workerOut := runCLI(t, "commands", "--filter", "task worker", "--json")
	if !strings.Contains(workerOut, `"command": "task worker"`) || !strings.Contains(workerOut, `--poll-interval DURATION`) {
		t.Fatalf("task worker command catalog = %s", workerOut)
	}
	mapOut := runCLI(t, "commands", "--filter", "map run", "--json")
	if !strings.Contains(mapOut, `--concurrency N`) {
		t.Fatalf("map run command catalog should include concurrency: %s", mapOut)
	}
}

func TestScheduledTaskClaimIsAtomicAcrossWorkers(t *testing.T) {
	ctx := context.Background()
	storeRef := "sqlite://" + filepath.Join(t.TempDir(), "tasks.sqlite")
	runtime, cleanup, err := openTaskStore(ctx, storeRef)
	if err != nil {
		t.Fatalf("open task store: %v", err)
	}
	defer cleanup()
	now := time.Now().UTC()
	task, err := runtime.UpsertAgentTask(ctx, store.AgentTask{
		ID: "agent-task.atomic", Name: "atomic", Kind: "cli", Command: "commands --json",
		Schedule: "interval:1s", Status: "scheduled", CreatedAt: now.Add(-time.Minute), UpdatedAt: now.Add(-time.Minute),
	})
	if err != nil {
		t.Fatalf("upsert atomic task: %v", err)
	}

	const workers = 8
	start := make(chan struct{})
	type claimResult struct {
		claim   store.AgentTaskClaim
		claimed bool
		err     error
	}
	results := make(chan claimResult, workers)
	var wg sync.WaitGroup
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			claim, claimed, claimErr := runtime.ClaimScheduledAgentTask(ctx, task.ID, task.UpdatedAt, now)
			results <- claimResult{claim: claim, claimed: claimed, err: claimErr}
		}()
	}
	close(start)
	wg.Wait()
	close(results)
	claimedCount := 0
	for result := range results {
		if result.err != nil {
			t.Fatalf("atomic claim: %v", result.err)
		}
		if result.claimed {
			claimedCount++
			if result.claim.TaskID != task.ID || result.claim.Revision.IsZero() || result.claim.Token == "" {
				t.Fatalf("atomic claim token = %#v", result.claim)
			}
		}
	}
	if claimedCount != 1 {
		t.Fatalf("claimed workers = %d, want 1", claimedCount)
	}
	claimedTask, err := runtime.GetAgentTask(ctx, task.ID)
	if err != nil || claimedTask.Status != store.StatusRunning {
		t.Fatalf("claimed task = %#v err=%v", claimedTask, err)
	}
}

func TestTaskWorkerStalePollCannotReclaimCompletedInterval(t *testing.T) {
	ctx := context.Background()
	storeRef := "sqlite://" + filepath.Join(t.TempDir(), "tasks.sqlite")
	runtime, cleanup, err := openTaskStore(ctx, storeRef)
	if err != nil {
		t.Fatalf("open task store: %v", err)
	}
	defer cleanup()
	now := time.Now().UTC().Truncate(time.Microsecond)
	task, err := runtime.UpsertAgentTask(ctx, store.AgentTask{
		ID: "agent-task.stale-poll", Name: "stale-poll", Kind: "shell", Command: "printf stale-poll",
		Schedule: "interval:1h", Status: "scheduled", CreatedAt: now.Add(-2 * time.Hour), UpdatedAt: now.Add(-2 * time.Hour),
	})
	if err != nil {
		t.Fatalf("upsert stale-poll task: %v", err)
	}
	listed, err := runtime.GetAgentTask(ctx, task.ID)
	if err != nil {
		t.Fatalf("load stale-poll task: %v", err)
	}
	stalePoll := staleListedTaskStore{Store: runtime, tasks: []store.AgentTask{listed}}

	first, err := runTaskWorkerPass(ctx, stalePoll, now)
	if err != nil || !first.OK || first.Claimed != 1 || first.Executed != 1 {
		t.Fatalf("first worker pass = %#v err=%v", first, err)
	}
	released, err := runtime.GetAgentTask(ctx, task.ID)
	if err != nil || released.Status != "scheduled" || released.UpdatedAt.Equal(listed.UpdatedAt) {
		t.Fatalf("released task = %#v err=%v", released, err)
	}
	if due, dueErr := scheduledAgentTaskDue(released, time.Now().UTC()); dueErr != nil || due {
		t.Fatalf("completed interval should not immediately be due: due=%t err=%v task=%#v", due, dueErr, released)
	}

	second, err := runTaskWorkerPass(ctx, stalePoll, now)
	if err != nil || !second.OK || second.Due != 1 || second.Claimed != 0 || second.Executed != 0 || second.Skipped != 1 {
		t.Fatalf("stale worker pass = %#v err=%v", second, err)
	}
	runs, err := runtime.ListAgentTaskRuns(ctx, task.ID, 10)
	if err != nil || len(runs) != 1 {
		t.Fatalf("stale poll task runs = %#v err=%v", runs, err)
	}
}

func TestTaskWorkerLeavesHardCrashClaimsVisible(t *testing.T) {
	ctx := context.Background()
	storeRef := "sqlite://" + filepath.Join(t.TempDir(), "tasks.sqlite")
	runtime, cleanup, err := openTaskStore(ctx, storeRef)
	if err != nil {
		t.Fatalf("open task store: %v", err)
	}
	now := time.Now().UTC()
	task, err := runtime.UpsertAgentTask(ctx, store.AgentTask{
		ID: "agent-task.crash", Name: "crash", Kind: "cli", Command: "commands --json",
		Schedule: "interval:1s", Status: "scheduled", CreatedAt: now.Add(-time.Minute), UpdatedAt: now.Add(-time.Minute),
	})
	if err != nil {
		t.Fatalf("upsert crash task: %v", err)
	}
	_, claimed, err := runtime.ClaimScheduledAgentTask(ctx, task.ID, task.UpdatedAt, now)
	if err != nil || !claimed {
		t.Fatalf("claim crash task: claimed=%t err=%v", claimed, err)
	}
	cleanup()

	workerOut := runCLI(t, "task", "worker", "--store", storeRef, "--once", "--json")
	if !strings.Contains(workerOut, `"scheduled": 0`) || !strings.Contains(workerOut, "automatic lease recovery is unsupported") {
		t.Fatalf("worker crash-boundary report = %s", workerOut)
	}
	statusOut := runCLI(t, "task", "status", task.ID, "--store", storeRef, "--json")
	if !strings.Contains(statusOut, `"status": "running"`) {
		t.Fatalf("hard crash claim should remain visible: %s", statusOut)
	}
	missingConfirmation := runCLIFails(t, "task", "stop", task.ID, "--store", storeRef, "--recover-running-claim", "--json")
	if !strings.Contains(missingConfirmation, "--confirm-side-effects-reviewed") {
		t.Fatalf("running claim recovery should require explicit side-effect review: %s", missingConfirmation)
	}
	recoveryOut := runCLI(t, "task", "stop", task.ID,
		"--store", storeRef,
		"--recover-running-claim",
		"--confirm-side-effects-reviewed",
		"--json",
	)
	if !strings.Contains(recoveryOut, `"status": "paused"`) {
		t.Fatalf("hard crash recovery should pause without replay: %s", recoveryOut)
	}
	workerAfterRecovery := runCLI(t, "task", "worker", "--store", storeRef, "--once", "--json")
	if !strings.Contains(workerAfterRecovery, `"scheduled": 0`) || !strings.Contains(workerAfterRecovery, `"executed": 0`) {
		t.Fatalf("recovered paused task must not replay automatically: %s", workerAfterRecovery)
	}
}

func TestTaskWorkerRetainsClaimWhenAttemptedRunCannotBePersisted(t *testing.T) {
	ctx := context.Background()
	storeRef := "sqlite://" + filepath.Join(t.TempDir(), "tasks.sqlite")
	runtime, cleanup, err := openTaskStore(ctx, storeRef)
	if err != nil {
		t.Fatalf("open task store: %v", err)
	}
	defer cleanup()
	now := time.Now().UTC()
	task, err := runtime.UpsertAgentTask(ctx, store.AgentTask{
		ID: "agent-task.persist-fail", Name: "persist-fail", Kind: "shell", Command: "printf attempted",
		Schedule: "interval:1s", Status: "scheduled", CreatedAt: now.Add(-time.Minute), UpdatedAt: now.Add(-time.Minute),
	})
	if err != nil {
		t.Fatalf("upsert persistence failure task: %v", err)
	}

	report, passErr := runTaskWorkerPass(ctx, recordFailingTaskStore{Store: runtime}, now)
	if passErr == nil || !strings.Contains(passErr.Error(), "unknown outcome") {
		t.Fatalf("worker pass error = %v", passErr)
	}
	if report.OK || report.Claimed != 1 || report.Executed != 1 || len(report.Tasks) != 1 {
		t.Fatalf("worker persistence failure report = %#v", report)
	}
	item := report.Tasks[0]
	if !item.Attempted || !item.UnknownOutcome || item.Run.ID != "" || !strings.Contains(item.Error, "claim retained as running") {
		t.Fatalf("worker persistence failure item = %#v", item)
	}
	loaded, err := runtime.GetAgentTask(ctx, task.ID)
	if err != nil || loaded.Status != store.StatusRunning {
		t.Fatalf("unknown-outcome task should remain claimed: %#v err=%v", loaded, err)
	}
	runs, err := runtime.ListAgentTaskRuns(ctx, task.ID, 10)
	if err != nil || len(runs) != 0 {
		t.Fatalf("failed persistence should leave no durable task run: %#v err=%v", runs, err)
	}
}
