package main

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"agent-testbench/internal/domain/mapplanner"
	"agent-testbench/internal/store"
)

func TestMapRunExecutorRestoresPassedDependencyExports(t *testing.T) {
	ctx := context.Background()
	storeRef := seedExecutableMapCommandStoreWithExports(t, ctx)
	runtime, err := openStore(ctx, storeRef)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer closeCLIStore(runtime)
	now := time.Now().UTC()
	record := store.TestMapPlanRecord{
		Instance: store.TestMapPlanInstance{
			ID:        "plan.resume.exports",
			MapID:     "map.profile.flow",
			ProfileID: "profile.flow",
			Mode:      mapplanner.ModeRun,
			Status:    mapplanner.TaskStatusRunning,
			StartedAt: now,
		},
		Tasks: []store.TestMapPlanTask{
			{
				ID:          "task.prefix",
				PlanID:      "plan.resume.exports",
				Index:       1,
				Kind:        mapplanner.TaskRunPathPrefix,
				Operation:   mapplanner.TaskRunPathPrefix,
				Status:      store.StatusPassed,
				PathID:      "workflow.flow.create",
				WorkflowID:  "workflow.flow.create",
				SummaryJSON: `{"exports":{"item_id":"item-123"}}`,
				StartedAt:   now.Add(-time.Minute),
				FinishedAt:  now.Add(-30 * time.Second),
			},
			{
				ID:        "task.case",
				PlanID:    "plan.resume.exports",
				Index:     2,
				Kind:      mapplanner.TaskRunCase,
				Operation: mapplanner.TaskRunCase,
				Status:    mapplanner.TaskStatusPlanned,
				NodeID:    "case.submit.success",
				CaseID:    "case.submit.success",
			},
		},
		TaskEdges: []store.TestMapPlanTaskEdge{{
			PlanID:     "plan.resume.exports",
			FromTaskID: "task.prefix",
			ToTaskID:   "task.case",
			Kind:       "control",
			Required:   true,
			SortOrder:  1,
		}},
	}

	executed := newMapRunExecutor(ctx, runtime, store.TestPlanGraph{}, mapRunOptions{}).execute(record)

	if executed.Tasks[0].Status != store.StatusPassed {
		t.Fatalf("passed dependency should stay passed = %#v", executed.Tasks[0])
	}
	if executed.Tasks[1].Status != store.StatusPassed || executed.Tasks[1].APICaseRunID == "" {
		t.Fatalf("case task should receive restored dependency exports = %#v", executed.Tasks[1])
	}
}

func TestMapRunExecutorFailsUnsupportedTaskKind(t *testing.T) {
	record := mapRunResumeFixture()
	record.Tasks = []store.TestMapPlanTask{{
		ID:        "task.unsupported",
		PlanID:    record.Instance.ID,
		Index:     1,
		Kind:      "probe_external_queue",
		Operation: "probe_external_queue",
		Status:    mapplanner.TaskStatusPlanned,
	}}

	executed := newMapRunExecutor(context.Background(), nil, store.TestPlanGraph{}, mapRunOptions{}).execute(record)

	if executed.Tasks[0].Status != store.StatusFailed || !strings.Contains(executed.Tasks[0].Reason, "unsupported map task kind") {
		t.Fatalf("unsupported task kind should fail = %#v", executed.Tasks[0])
	}
	if executed.Instance.Status != store.StatusFailed {
		t.Fatalf("unsupported task should fail plan = %#v", executed.Instance)
	}
}

func TestMapRunExecutorEmptyPrefixDoesNotReplayFullPath(t *testing.T) {
	graph := store.TestPlanGraph{
		Paths: []store.TestPlanPath{{ID: "path.flow"}},
		PathSteps: []store.TestPlanPathStep{{
			PathID:    "path.flow",
			StepIndex: 1,
			StepID:    "step.prepare",
			NodeID:    "case.prepare",
			CaseID:    "case.prepare",
		}},
	}
	executor := newMapRunExecutor(context.Background(), nil, graph, mapRunOptions{})

	prefix := executor.stepsForTask(store.TestMapPlanTask{Kind: mapplanner.TaskRunPathPrefix, PathID: "path.flow"}, "")
	full := executor.stepsForTask(store.TestMapPlanTask{Kind: mapplanner.TaskRunPath, PathID: "path.flow"}, "")

	if len(prefix) != 0 {
		t.Fatalf("empty prefix should not replay full path = %#v", prefix)
	}
	if len(full) != 1 {
		t.Fatalf("full path should still include all steps = %#v", full)
	}
}

func TestMapRunExecutorScopesMaterializationMappingsToTargetCase(t *testing.T) {
	ctx := context.Background()
	storeRef := "sqlite://" + filepath.Join(t.TempDir(), "map-run-materialized-scope.sqlite")
	runtime, err := openStore(ctx, storeRef)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer closeCLIStore(runtime)
	catalog := mapCommandExecutableProfileCatalogFixture("http://127.0.0.1")
	for i := range catalog.Fixtures {
		if catalog.Fixtures[i].ID == "fixture.before.submit" {
			catalog.Fixtures[i].DataJSON = `{"item_id":"item-123","other_id":"leak"}`
		}
	}
	catalog.CaseDependencies = []store.CatalogCaseDependency{
		{ID: "dependency.field.required", CaseID: "case.submit.field.required", FixtureID: "fixture.before.submit", Required: true, Status: "active", MappingsJSON: `[{"from":"$.item_id","to":"$.request.item_id"}]`},
		{ID: "dependency.other", CaseID: "case.other", FixtureID: "fixture.before.submit", Required: true, Status: "active", MappingsJSON: `[{"from":"$.other_id","to":"$.request.other_id"}]`},
	}
	if err := runtime.ReplaceProfileCatalog(ctx, catalog); err != nil {
		t.Fatalf("replace catalog: %v", err)
	}
	graph := store.TestPlanGraph{
		Nodes: []store.TestPlanNode{
			{ID: "case.submit.field.required", CaseID: "case.submit.field.required"},
			{ID: "case.other", CaseID: "case.other"},
		},
		Edges: []store.TestPlanEdge{{
			ID:                "edge.fixture.field.required",
			ToNodeID:          "case.submit.field.required",
			Kind:              "fixture",
			MaterializationID: "fixture.before.submit",
			Required:          true,
		}},
	}

	mappings := newMapRunExecutor(ctx, runtime, graph, mapRunOptions{}).materializationMappings("fixture.before.submit", "fixture.before.submit")
	overrides, err := mapRunMaterializedOverrides(`{"item_id":"item-123","other_id":"leak"}`, mappings)
	if err != nil {
		t.Fatalf("materialized overrides: %v", err)
	}

	if overrides["item_id"] != "item-123" {
		t.Fatalf("target case mapping should keep item_id override = %#v", overrides)
	}
	if _, ok := overrides["request.other_id"]; ok {
		t.Fatalf("unrelated case dependency should not leak into overrides = %#v", overrides)
	}
}
