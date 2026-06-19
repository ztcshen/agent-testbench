package main

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"agent-testbench/internal/domain/mapplanner"
	"agent-testbench/internal/store"
)

func TestMapRunExecutesPlanTasksAndExplainReadsResult(t *testing.T) {
	ctx := context.Background()
	storeRef := seedExecutableMapCommandStore(t, ctx)

	runCLI(t, "map", "import-workflows", "--store", storeRef, "--json")
	report := decodeMapRunCommandReport(t, runCLI(t, "map", "run", "--store", storeRef, "--map", "map.profile.flow", "--scope", "all", "--environment", "env.local", "--json"))
	assertMapRunCommandReport(t, report)
	assertStoredMapRunPlan(t, ctx, storeRef, report.PlanID)
	assertMapRunExplainCommandReport(t, runCLI(t, "map", "run", "explain", "--store", storeRef, "--plan", report.PlanID, "--json"), report.PlanID)
	assertMapRunExplainCommandReport(t, runCLI(t, "map", "plan", "inspect", "--store", storeRef, "--plan", report.PlanID, "--json"), report.PlanID)
}

func TestMapRunPropagatesWorkflowStepExports(t *testing.T) {
	ctx := context.Background()
	storeRef := seedExecutableMapCommandStoreWithExports(t, ctx)

	runCLI(t, "map", "import-workflows", "--store", storeRef, "--json")
	report := decodeMapRunCommandReport(t, runCLI(t, "map", "run", "--store", storeRef, "--map", "map.profile.flow", "--scope", "workflows", "--json"))

	if !report.OK || report.Status != store.StatusPassed || len(report.Tasks) != 1 {
		t.Fatalf("map run should pass exported workflow inputs = %#v", report)
	}
	if report.Tasks[0].WorkflowRunID == "" {
		t.Fatalf("workflow task should record run id = %#v", report.Tasks[0])
	}
}

func TestMapRunRerunTaskUsesFreshChildRunID(t *testing.T) {
	ctx := context.Background()
	storeRef := seedExecutableMapCommandStore(t, ctx)

	runCLI(t, "map", "import-workflows", "--store", storeRef, "--json")
	first := decodeMapRunCommandReport(t, runCLI(t, "map", "run", "--store", storeRef, "--map", "map.profile.flow", "--scope", "workflows", "--json"))
	if len(first.Tasks) != 1 || first.Tasks[0].WorkflowRunID == "" {
		t.Fatalf("first map run = %#v", first)
	}

	rerun := decodeMapRunCommandReport(t, runCLI(t, "map", "run", "--store", storeRef, "--plan", first.PlanID, "--rerun-task", first.Tasks[0].ID, "--json"))
	if !rerun.OK || rerun.Tasks[0].Status != store.StatusPassed {
		t.Fatalf("rerun task should pass = %#v", rerun)
	}
	if rerun.Tasks[0].WorkflowRunID == first.Tasks[0].WorkflowRunID {
		t.Fatalf("rerun should use a fresh workflow run id, first=%q rerun=%q", first.Tasks[0].WorkflowRunID, rerun.Tasks[0].WorkflowRunID)
	}
}

func TestMapRunRejectsUnknownRerunTask(t *testing.T) {
	ctx := context.Background()
	storeRef := seedExecutableMapCommandStore(t, ctx)

	runCLI(t, "map", "import-workflows", "--store", storeRef, "--json")
	first := decodeMapRunCommandReport(t, runCLI(t, "map", "run", "--store", storeRef, "--map", "map.profile.flow", "--scope", "workflows", "--json"))
	out := runCLIFails(t, "map", "run", "--store", storeRef, "--plan", first.PlanID, "--rerun-task", "task.missing", "--json")
	if !strings.Contains(out, "rerun task not found: task.missing") {
		t.Fatalf("unknown rerun task error = %s", out)
	}
}

func TestMapRunPropagatesReplayPrefixExportsToCaseTask(t *testing.T) {
	ctx := context.Background()
	storeRef := seedExecutableMapCommandStoreWithExports(t, ctx)
	runCLI(t, "map", "import-workflows", "--store", storeRef, "--json")
	runtime, err := openStore(ctx, storeRef)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	now := time.Now().UTC()
	record := store.TestMapPlanRecord{
		Instance: store.TestMapPlanInstance{
			ID:         "plan.replay.exports",
			MapID:      "map.profile.flow",
			ProfileID:  "profile.flow",
			Mode:       mapplanner.ModeExplain,
			Status:     mapplanner.TaskStatusPlanned,
			Scope:      mapplanner.ScopeCase,
			TargetKind: mapplanner.TargetCase,
			TargetID:   "case.submit.success",
			StartedAt:  now,
		},
		Tasks: []store.TestMapPlanTask{
			{
				ID:          "task.prefix",
				PlanID:      "plan.replay.exports",
				Index:       1,
				Kind:        mapplanner.TaskRunPathPrefix,
				Operation:   mapplanner.TaskRunPathPrefix,
				Status:      mapplanner.TaskStatusPlanned,
				PathID:      "workflow.flow.create",
				WorkflowID:  "workflow.flow.create",
				SummaryJSON: `{"untilNodeId":"case.prepare"}`,
			},
			{
				ID:        "task.case",
				PlanID:    "plan.replay.exports",
				Index:     2,
				Kind:      mapplanner.TaskRunCase,
				Operation: mapplanner.TaskRunCase,
				Status:    mapplanner.TaskStatusPlanned,
				NodeID:    "case.submit.success",
				CaseID:    "case.submit.success",
			},
		},
		TaskEdges: []store.TestMapPlanTaskEdge{{
			PlanID:     "plan.replay.exports",
			FromTaskID: "task.prefix",
			ToTaskID:   "task.case",
			Kind:       "control",
			Required:   true,
			SortOrder:  1,
		}},
	}
	if err := runtime.SaveTestMapPlan(ctx, record); err != nil {
		t.Fatalf("save map plan: %v", err)
	}
	closeCLIStore(runtime)

	report := decodeMapRunCommandReport(t, runCLI(t, "map", "run", "--store", storeRef, "--plan", "plan.replay.exports", "--json"))
	if !report.OK || report.Status != store.StatusPassed || len(report.Tasks) != 2 {
		t.Fatalf("replay exports should feed case task = %#v", report)
	}
}

func TestMapRunReusesMaterializationFixtureDataForCaseTask(t *testing.T) {
	ctx := context.Background()
	storeRef := seedExecutableMapCommandStoreWithMaterializedFixture(t, ctx)

	runCLI(t, "map", "import-workflows", "--store", storeRef, "--json")
	report := decodeMapRunCommandReport(t, runCLI(t, "map", "run", "--store", storeRef, "--map", "map.profile.flow", "--scope", "cases", "--json"))

	if !report.OK || report.Status != store.StatusPassed || report.Summary.TotalTasks != 2 || report.Summary.PassedTasks != 2 {
		t.Fatalf("materialized fixture data should feed case task = %#v", report)
	}
	if report.Tasks[0].Kind != mapplanner.TaskReuseMaterialized || report.Tasks[0].Status != store.StatusPassed {
		t.Fatalf("materialized replay task should pass = %#v", report.Tasks[0])
	}
	if report.Tasks[1].Kind != mapplanner.TaskRunCase || report.Tasks[1].Status != store.StatusPassed || report.Tasks[1].APICaseRunID == "" {
		t.Fatalf("case task should run with materialized overrides = %#v", report.Tasks[1])
	}
}

func TestMapRunRejectsMismatchedPlanAndMap(t *testing.T) {
	ctx := context.Background()
	storeRef := seedExecutableMapCommandStore(t, ctx)

	runCLI(t, "map", "import-workflows", "--store", storeRef, "--json")
	report := decodeMapRunCommandReport(t, runCLI(t, "map", "run", "--store", storeRef, "--map", "map.profile.flow", "--scope", "workflows", "--json"))
	out := runCLIFails(t, "map", "run", "--store", storeRef, "--plan", report.PlanID, "--map", "map.other", "--json")
	if !strings.Contains(out, "--map map.other does not match plan map map.profile.flow") {
		t.Fatalf("mismatched plan/map error = %s", out)
	}
}

func TestMapRunRejectsSavedPlanWhenGraphChanged(t *testing.T) {
	ctx := context.Background()
	storeRef := seedExecutableMapCommandStore(t, ctx)
	runCLI(t, "map", "import-workflows", "--store", storeRef, "--json")
	out := runCLI(t, "map", "explain", "--store", storeRef, "--map", "map.profile.flow", "--scope", "workflows", "--save", "--json")
	var saved struct {
		PlanID string `json:"planId"`
	}
	if err := json.Unmarshal([]byte(out), &saved); err != nil {
		t.Fatalf("decode saved explain: %v\n%s", err, out)
	}

	runtime, err := openStore(ctx, storeRef)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	graph, err := runtime.GetTestPlanGraph(ctx, "map.profile.flow")
	if err != nil {
		t.Fatalf("get graph: %v", err)
	}
	graph.PathSteps = append(graph.PathSteps, store.TestPlanPathStep{
		MapID:     "map.profile.flow",
		PathID:    "workflow.flow.create",
		StepIndex: 3,
		StepID:    "step.extra",
		NodeID:    "case.prepare",
		CaseID:    "case.prepare",
		Required:  true,
	})
	if err := runtime.ReplaceTestPlanGraph(ctx, graph); err != nil {
		t.Fatalf("replace changed graph: %v", err)
	}
	closeCLIStore(runtime)

	failed := runCLIFails(t, "map", "run", "--store", storeRef, "--plan", saved.PlanID, "--json")
	if !strings.Contains(failed, "saved plan graph fingerprint does not match current map graph") {
		t.Fatalf("changed graph error = %s", failed)
	}
}

func TestMapRunPreflightRejectsMapWhenActiveCatalogLostCases(t *testing.T) {
	ctx := context.Background()
	storeRef := seedExecutableMapCommandStore(t, ctx)
	runCLI(t, "map", "import-workflows", "--store", storeRef, "--json")

	runtime, err := openStore(ctx, storeRef)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	if err := runtime.ReplaceProfileCatalog(ctx, store.ProfileCatalog{ProfileID: "profile.fixture"}); err != nil {
		t.Fatalf("replace active catalog: %v", err)
	}
	closeCLIStore(runtime)

	out := runCLIFails(t, "map", "run", "--store", storeRef, "--map", "map.profile.flow", "--scope", "all", "--json")
	for _, want := range []string{"map run preflight failed", "missing catalog cases", "case.prepare", "case.submit.success", "missing catalog workflows", "workflow.flow.create"} {
		if !strings.Contains(out, want) {
			t.Fatalf("preflight error missing %q:\n%s", want, out)
		}
	}
}

func TestMapRunExplainPreservesFailedStatus(t *testing.T) {
	ctx := context.Background()
	storeRef := "sqlite://" + filepath.Join(t.TempDir(), "map-run-explain-failed.sqlite")
	runtime, err := openStore(ctx, storeRef)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	now := time.Now().UTC()
	record := store.TestMapPlanRecord{
		Instance: store.TestMapPlanInstance{
			ID:        "plan.failed",
			MapID:     "map.failed",
			ProfileID: "profile.failed",
			Mode:      mapplanner.ModeRun,
			Status:    store.StatusFailed,
			Scope:     mapplanner.ScopeWorkflows,
			StartedAt: now,
		},
		Tasks: []store.TestMapPlanTask{{
			ID:        "task.failed",
			PlanID:    "plan.failed",
			Index:     1,
			Kind:      mapplanner.TaskRunCase,
			Status:    store.StatusFailed,
			CaseID:    "case.failed",
			StartedAt: now,
		}},
	}
	if err := runtime.SaveTestMapPlan(ctx, record); err != nil {
		t.Fatalf("save failed plan: %v", err)
	}
	closeCLIStore(runtime)

	out := runCLI(t, "map", "run", "explain", "--store", storeRef, "--plan", "plan.failed", "--json")
	var report mapRunExplainCommandReport
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatalf("decode explain: %v\n%s", err, out)
	}
	if report.OK || report.Status != store.StatusFailed {
		t.Fatalf("explain should preserve failed OK/status = %#v", report)
	}
}

func TestMapRunExecutorSkipsAlreadyTerminalTasks(t *testing.T) {
	started := time.Now().UTC().Add(-time.Minute)
	record := mapRunResumeFixture()
	record.Tasks = []store.TestMapPlanTask{record.Tasks[0], record.Tasks[2]}
	record.Tasks[1].Status = mapplanner.TaskStatusPlanned
	record.Tasks[1].Kind = mapplanner.TaskRunPath
	record.Tasks[1].PathID = "path.empty"
	record.Tasks[1].CaseID = ""
	record.TaskEdges = []store.TestMapPlanTaskEdge{{
		PlanID:     record.Instance.ID,
		FromTaskID: record.Tasks[0].ID,
		ToTaskID:   record.Tasks[1].ID,
		Kind:       "control",
		Required:   true,
	}}
	graph := store.TestPlanGraph{Paths: []store.TestPlanPath{{ID: "path.empty"}}, PathSteps: nil}
	executed := newMapRunExecutor(context.Background(), nil, graph, mapRunOptions{}).execute(record)

	if executed.Tasks[0].Status != store.StatusPassed || executed.Tasks[0].WorkflowRunID != "run.already.passed" {
		t.Fatalf("executor should not rerun already passed task = %#v", executed.Tasks[0])
	}
	if executed.Tasks[1].Status != mapplanner.TaskStatusSkipped || !executed.Tasks[1].FinishedAt.After(started) {
		t.Fatalf("executor should execute only planned task = %#v", executed.Tasks[1])
	}
}

func TestMapRunExecutorRetryFailedRunsOnlyFailedTasks(t *testing.T) {
	record := mapRunResumeFixture()
	record.Tasks[1].Kind = mapplanner.TaskRunPath
	record.Tasks[1].PathID = "path.empty"
	record.Tasks[1].CaseID = ""
	record.Tasks[2].Kind = mapplanner.TaskRunPath
	record.Tasks[2].PathID = "path.empty"
	prepared := prepareExistingMapRunRecord(record, mapRunOptions{retryFailed: true})

	graph := store.TestPlanGraph{Paths: []store.TestPlanPath{{ID: "path.empty"}}, PathSteps: nil}
	executed := newMapRunExecutor(context.Background(), nil, graph, mapRunOptions{retryFailed: true}).execute(prepared)

	if executed.Tasks[1].Status != mapplanner.TaskStatusSkipped || executed.Tasks[1].APICaseRunID != "" || executed.Tasks[1].Reason != "" {
		t.Fatalf("retry failed should execute and rewrite selected failed task = %#v", executed.Tasks[1])
	}
	if executed.Tasks[2].Status != mapplanner.TaskStatusPlanned || !executed.Tasks[2].StartedAt.IsZero() {
		t.Fatalf("retry failed should not execute unrelated planned task = %#v", executed.Tasks[2])
	}
}

func TestMapRunExecutorSchedulesTaskDAGBeforeStoredOrder(t *testing.T) {
	record := mapRunResumeFixture()
	record.Tasks = []store.TestMapPlanTask{
		{
			ID:     "task.second",
			PlanID: "plan.resume",
			Index:  2,
			Kind:   mapplanner.TaskRunPath,
			Status: mapplanner.TaskStatusPlanned,
			PathID: "path.empty",
		},
		{
			ID:     "task.first",
			PlanID: "plan.resume",
			Index:  1,
			Kind:   mapplanner.TaskRunPath,
			Status: mapplanner.TaskStatusPlanned,
			PathID: "path.empty",
		},
	}
	record.TaskEdges = []store.TestMapPlanTaskEdge{{
		PlanID:     record.Instance.ID,
		FromTaskID: "task.first",
		ToTaskID:   "task.second",
		Kind:       "control",
		Required:   true,
	}}
	graph := store.TestPlanGraph{Paths: []store.TestPlanPath{{ID: "path.empty"}}}

	executed := newMapRunExecutor(context.Background(), nil, graph, mapRunOptions{}).execute(record)

	if executed.Tasks[0].Status == mapplanner.TaskStatusBlocked {
		t.Fatalf("dependent task should wait for dependency instead of blocking from stored order = %#v", executed.Tasks)
	}
	if executed.Tasks[0].Status != mapplanner.TaskStatusSkipped || executed.Tasks[1].Status != mapplanner.TaskStatusSkipped {
		t.Fatalf("topological execution statuses = %#v", executed.Tasks)
	}
}

type mapRunCommandReport struct {
	OK            bool   `json:"ok"`
	PlanID        string `json:"planId"`
	MapID         string `json:"mapId"`
	Scope         string `json:"scope"`
	Status        string `json:"status"`
	EnvironmentID string `json:"environmentId"`
	Summary       struct {
		TotalTasks   int `json:"totalTasks"`
		PassedTasks  int `json:"passedTasks"`
		SkippedTasks int `json:"skippedTasks"`
		FailedTasks  int `json:"failedTasks"`
	} `json:"summary"`
	Tasks []mapRunCommandTask `json:"tasks"`
}

type mapRunCommandTask struct {
	ID            string `json:"id"`
	Kind          string `json:"kind"`
	Status        string `json:"status"`
	Reason        string `json:"reason"`
	CaseID        string `json:"caseId"`
	WorkflowRunID string `json:"workflowRunId"`
	APICaseRunID  string `json:"apiCaseRunId"`
}

type mapRunExplainCommandReport struct {
	OK      bool   `json:"ok"`
	PlanID  string `json:"planId"`
	Status  string `json:"status"`
	Summary struct {
		TotalTasks   int `json:"totalTasks"`
		PassedTasks  int `json:"passedTasks"`
		SkippedTasks int `json:"skippedTasks"`
	} `json:"summary"`
	NextActions []string `json:"nextActions"`
}
