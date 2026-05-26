package controlplane_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"agent-testbench/internal/domain/profile"
	"agent-testbench/internal/server/controlplane"
	"agent-testbench/internal/store"
	"agent-testbench/internal/store/sqlite"
)

func TestServerExposesPostProcessTasks(t *testing.T) {
	ctx := context.Background()
	s := openTestKitSQLiteStore(t, ctx, "store.sqlite")
	recordPostProcessTaskRouteFixture(t, ctx, s)

	server := httptest.NewServer(controlplane.NewWithOptions(profile.Bundle{ID: "sample"}, controlplane.Options{Runtime: s}))
	defer server.Close()

	requireFilteredPostProcessTaskRoute(t, server.URL)
	requireAllPostProcessTaskRoute(t, server.URL)
	requireMissingPostProcessTaskRunID(t, server.URL)
}

func recordPostProcessTaskRouteFixture(t *testing.T, ctx context.Context, s *sqlite.Store) {
	t.Helper()

	base := time.Date(2026, 5, 17, 2, 3, 4, 0, time.UTC)
	if _, err := s.CreateRun(ctx, store.Run{
		ID:         "run.tasks",
		ProfileID:  "sample",
		WorkflowID: "workflow.alpha",
		Status:     store.StatusPassed,
		StartedAt:  base,
		FinishedAt: base.Add(time.Second),
		CreatedAt:  base,
		UpdatedAt:  base.Add(time.Second),
	}); err != nil {
		t.Fatalf("create task run: %v", err)
	}
	recordPostProcessTask(t, ctx, s, store.PostProcessTask{
		ID:         "task.trace",
		RunID:      "run.tasks",
		WorkflowID: "workflow.alpha",
		StepID:     "step-a",
		CaseID:     "case.alpha",
		Kind:       "trace_topology_collect",
		Status:     store.StatusPassed,
		StartedAt:  base.Add(10 * time.Millisecond),
		FinishedAt: base.Add(135 * time.Millisecond),
		CreatedAt:  base.Add(10 * time.Millisecond),
	})
	recordPostProcessTask(t, ctx, s, store.PostProcessTask{
		ID:          "task.logs",
		RunID:       "run.tasks",
		WorkflowID:  "workflow.alpha",
		StepID:      "step-b",
		CaseID:      "case.beta",
		Kind:        "runtime_log_collect",
		Status:      store.StatusFailed,
		StartedAt:   base.Add(200 * time.Millisecond),
		FinishedAt:  base.Add(500 * time.Millisecond),
		Error:       "log source missing",
		SummaryJSON: `{"source":"runtime-log"}`,
		CreatedAt:   base.Add(200 * time.Millisecond),
	})
	recordPostProcessTask(t, ctx, s, store.PostProcessTask{
		ID:          "task.trace.skip",
		RunID:       "run.tasks",
		WorkflowID:  "workflow.alpha",
		StepID:      "step-c",
		CaseID:      "case.gamma",
		Kind:        "trace_topology_collect",
		Status:      store.StatusSkipped,
		StartedAt:   base.Add(600 * time.Millisecond),
		FinishedAt:  base.Add(600 * time.Millisecond),
		SummaryJSON: `{"reason":"SkyWalking provider unavailable"}`,
		CreatedAt:   base.Add(600 * time.Millisecond),
	})
}

func recordPostProcessTask(t *testing.T, ctx context.Context, s *sqlite.Store, task store.PostProcessTask) {
	t.Helper()

	if _, err := s.RecordPostProcessTask(ctx, task); err != nil {
		t.Fatalf("record post process task %s: %v", task.ID, err)
	}
}

func requireFilteredPostProcessTaskRoute(t *testing.T, serverURL string) {
	t.Helper()

	payload := decodeJSONResponse(t, serverURL+"/api/post-process-tasks?runId=run.tasks&stepId=step-a&kind=trace_topology_collect", http.StatusOK)
	if payload["ok"] != true || payload["runId"] != "run.tasks" {
		t.Fatalf("post process task payload = %#v", payload)
	}
	counts := payload["counts"].(map[string]any)
	if counts["total"].(float64) != 1 || counts["passed"].(float64) != 1 || counts["durationMs"].(float64) != 125 {
		t.Fatalf("post process task counts = %#v", counts)
	}
	tasks := payload["tasks"].([]any)
	if len(tasks) != 1 {
		t.Fatalf("post process tasks = %#v", tasks)
	}
	task := tasks[0].(map[string]any)
	if task["id"] != "task.trace" || task["kind"] != "trace_topology_collect" || task["stepId"] != "step-a" {
		t.Fatalf("post process task = %#v", task)
	}
	if task["outcome"] != "success" || task["reason"] != "completed" || task["displayStatus"] != "passed: completed" {
		t.Fatalf("post process task readable status = %#v", task)
	}
}

func requireAllPostProcessTaskRoute(t *testing.T, serverURL string) {
	t.Helper()

	all := decodeJSONResponse(t, serverURL+"/api/post-process-tasks?runId=run.tasks", http.StatusOK)
	allTasks := all["tasks"].([]any)
	if len(allTasks) != 3 {
		t.Fatalf("all post process tasks = %#v", allTasks)
	}
	byID := map[string]map[string]any{}
	for _, raw := range allTasks {
		task := raw.(map[string]any)
		byID[task["id"].(string)] = task
	}
	if byID["task.logs"]["outcome"] != "failed" || byID["task.logs"]["reason"] != "log source missing" || byID["task.logs"]["displayStatus"] != "failed: log source missing" {
		t.Fatalf("failed task readable status = %#v", byID["task.logs"])
	}
	if byID["task.trace.skip"]["outcome"] != "skipped" || byID["task.trace.skip"]["reason"] != "SkyWalking provider unavailable" || byID["task.trace.skip"]["displayStatus"] != "skipped: SkyWalking provider unavailable" {
		t.Fatalf("skipped task readable status = %#v", byID["task.trace.skip"])
	}
}

func requireMissingPostProcessTaskRunID(t *testing.T, serverURL string) {
	t.Helper()

	missing := decodeJSONResponse(t, serverURL+"/api/post-process-tasks", http.StatusBadRequest)
	if missing["ok"] != false || !strings.Contains(missing["error"].(string), "runId") {
		t.Fatalf("missing runId response = %#v", missing)
	}
}
