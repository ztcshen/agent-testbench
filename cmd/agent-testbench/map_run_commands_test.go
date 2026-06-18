package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
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

func TestMapRunReportsUnsupportedNonHTTPRunner(t *testing.T) {
	ctx := context.Background()
	storeRef := seedExecutableMapCommandStoreWithMQCase(t, ctx)

	runCLI(t, "map", "import-workflows", "--store", storeRef, "--json")
	out := runCLIFails(t, "map", "run", "--store", storeRef, "--map", "map.profile.flow", "--case", "case.submit.success", "--json")
	report := decodeMapRunCommandReport(t, out)

	if report.OK || report.Status != store.StatusFailed || report.Summary.FailedTasks != 1 {
		t.Fatalf("unsupported MQ case should fail one task = %#v", report)
	}
	if len(report.Tasks) != 1 || !strings.Contains(report.Tasks[0].Reason, "unsupported map case runner") || !strings.Contains(report.Tasks[0].Reason, "executor.mq") {
		t.Fatalf("unsupported runner reason = %#v", report.Tasks)
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

func TestMapRunNextActionsQuotePlanID(t *testing.T) {
	actions := mapRunNextActions("plan with space")
	if len(actions) != 1 || !strings.Contains(actions[0], "--plan 'plan with space'") {
		t.Fatalf("next actions should quote plan id = %#v", actions)
	}
}

func TestPrepareExistingMapRunRecordResumeKeepsPassedTasks(t *testing.T) {
	record := mapRunResumeFixture()

	prepared := prepareExistingMapRunRecord(record, mapRunOptions{resumeRun: true})

	if prepared.Tasks[0].Status != store.StatusPassed || prepared.Tasks[0].WorkflowRunID != "run.already.passed" || prepared.Tasks[0].EvidenceRoot != "evidence/already" {
		t.Fatalf("resume should keep passed task execution metadata = %#v", prepared.Tasks[0])
	}
	if prepared.Tasks[1].Status != mapplanner.TaskStatusPlanned || prepared.Tasks[1].APICaseRunID != "" || prepared.Tasks[1].Reason != "" || !prepared.Tasks[1].FinishedAt.IsZero() {
		t.Fatalf("resume should reset failed task for execution = %#v", prepared.Tasks[1])
	}
	if prepared.Tasks[2].Status != mapplanner.TaskStatusPlanned || !prepared.Tasks[2].StartedAt.IsZero() || !prepared.Tasks[2].FinishedAt.IsZero() {
		t.Fatalf("resume should leave planned task runnable without erasing started time = %#v", prepared.Tasks[2])
	}
}

func TestPrepareExistingMapRunRecordResetsInstanceStartedAt(t *testing.T) {
	record := mapRunResumeFixture()
	record.Instance.Mode = mapplanner.ModeExplain
	record.Instance.StartedAt = time.Now().UTC().Add(-2 * time.Hour)

	prepared := prepareExistingMapRunRecord(record, mapRunOptions{})

	if !prepared.Instance.StartedAt.After(record.Instance.StartedAt) || prepared.Instance.Mode != mapplanner.ModeRun {
		t.Fatalf("existing plan execution should reset instance start time = %#v", prepared.Instance)
	}
}

func TestPrepareExistingMapRunRecordRetryFailedOnlyResetsFailedTasks(t *testing.T) {
	record := mapRunResumeFixture()

	prepared := prepareExistingMapRunRecord(record, mapRunOptions{retryFailed: true})

	if prepared.Tasks[0].Status != store.StatusPassed || prepared.Tasks[0].WorkflowRunID != "run.already.passed" {
		t.Fatalf("retry failed should keep passed task = %#v", prepared.Tasks[0])
	}
	if prepared.Tasks[1].Status != store.StatusFailed || prepared.Tasks[1].APICaseRunID != "" || prepared.Tasks[1].Reason != "" {
		t.Fatalf("retry failed should reset failed task = %#v", prepared.Tasks[1])
	}
	if prepared.Tasks[2].Status != mapplanner.TaskStatusPlanned || prepared.Tasks[2].APICaseRunID != "" || !prepared.Tasks[2].StartedAt.IsZero() {
		t.Fatalf("retry failed should park unrelated planned task without executing it = %#v", prepared.Tasks[2])
	}
}

func TestPrepareExistingMapRunRecordRerunTaskOnlyResetsSelectedTasks(t *testing.T) {
	record := mapRunResumeFixture()

	prepared := prepareExistingMapRunRecord(record, mapRunOptions{rerunTaskIDs: []string{"task.path"}})

	if prepared.Tasks[0].Status != mapplanner.TaskStatusPlanned || prepared.Tasks[0].WorkflowRunID != "" || prepared.Tasks[0].EvidenceRoot != "" {
		t.Fatalf("rerun task should reset selected task = %#v", prepared.Tasks[0])
	}
	if prepared.Tasks[1].Status != store.StatusFailed || prepared.Tasks[1].APICaseRunID != "run.failed.case" || prepared.Tasks[1].Reason == "" {
		t.Fatalf("rerun task should keep unselected failed task metadata = %#v", prepared.Tasks[1])
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
	WorkflowRunID string `json:"workflowRunId"`
	APICaseRunID  string `json:"apiCaseRunId"`
}

func mapRunResumeFixture() store.TestMapPlanRecord {
	started := time.Now().UTC().Add(-2 * time.Minute)
	finished := started.Add(time.Minute)
	return store.TestMapPlanRecord{
		Instance: store.TestMapPlanInstance{
			ID:        "plan.resume",
			MapID:     "map.resume",
			ProfileID: "profile.resume",
			Mode:      mapplanner.ModeRun,
			Status:    store.StatusFailed,
			StartedAt: started,
		},
		Tasks: []store.TestMapPlanTask{
			{
				ID:            "task.path",
				PlanID:        "plan.resume",
				Index:         0,
				Kind:          mapplanner.TaskRunPath,
				Status:        store.StatusPassed,
				PathID:        "path.empty",
				WorkflowRunID: "run.already.passed",
				EvidenceRoot:  "evidence/already",
				StartedAt:     started,
				FinishedAt:    finished,
			},
			{
				ID:           "task.case.failed",
				PlanID:       "plan.resume",
				Index:        1,
				Kind:         mapplanner.TaskRunCase,
				Status:       store.StatusFailed,
				CaseID:       "case.failed",
				APICaseRunID: "run.failed.case",
				EvidenceRoot: "evidence/failed",
				Reason:       "assertion failed",
				StartedAt:    started,
				FinishedAt:   finished,
			},
			{
				ID:     "task.case.planned",
				PlanID: "plan.resume",
				Index:  2,
				Kind:   mapplanner.TaskRunCase,
				Status: mapplanner.TaskStatusPlanned,
				CaseID: "case.planned",
			},
		},
	}
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

func seedExecutableMapCommandStore(t *testing.T, ctx context.Context) string {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/prepare", "/submit":
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, `{"ok":true}`)
		case "/validate":
			w.WriteHeader(http.StatusBadRequest)
			_, _ = fmt.Fprint(w, `{"error":"field required"}`)
		default:
			t.Fatalf("unexpected request path: %s", r.URL.Path)
		}
	}))
	t.Cleanup(server.Close)

	storePath := filepath.Join(t.TempDir(), "map-run.sqlite")
	storeRef := "sqlite://" + storePath
	runtime, err := openStore(ctx, storeRef)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	if err := runtime.ReplaceProfileCatalog(ctx, mapCommandExecutableProfileCatalogFixture(server.URL)); err != nil {
		t.Fatalf("seed executable profile catalog: %v", err)
	}
	closeCLIStore(runtime)
	return storeRef
}

func seedExecutableMapCommandStoreWithMQCase(t *testing.T, ctx context.Context) string {
	t.Helper()
	submitSeen := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/prepare":
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, `{"ok":true}`)
		case "/submit":
			submitSeen = true
			t.Fatalf("MQ case should not be executed through HTTP: %s", r.URL.Path)
		default:
			t.Fatalf("unexpected request path: %s", r.URL.Path)
		}
	}))
	t.Cleanup(func() {
		server.Close()
		if submitSeen {
			t.Fatalf("MQ case reached HTTP server")
		}
	})
	storePath := filepath.Join(t.TempDir(), "map-run-mq.sqlite")
	storeRef := "sqlite://" + storePath
	runtime, err := openStore(ctx, storeRef)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	catalog := mapCommandExecutableProfileCatalogFixture(server.URL)
	for i := range catalog.APICases {
		if catalog.APICases[i].ID == "case.submit.success" {
			catalog.APICases[i].ExecutorID = "executor.mq"
			catalog.APICases[i].SourceKind = "mq"
		}
	}
	if err := runtime.ReplaceProfileCatalog(ctx, catalog); err != nil {
		t.Fatalf("seed mq profile catalog: %v", err)
	}
	closeCLIStore(runtime)
	return storeRef
}

func seedExecutableMapCommandStoreWithExports(t *testing.T, ctx context.Context) string {
	t.Helper()
	submitSeen := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/prepare":
			w.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprint(w, `{"item_id":"item-123"}`)
		case "/submit":
			if r.URL.Query().Get("item_id") != "item-123" {
				t.Fatalf("submit should receive exported item_id, query=%s", r.URL.RawQuery)
			}
			submitSeen = true
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, `{"ok":true}`)
		default:
			t.Fatalf("unexpected request path: %s", r.URL.Path)
		}
	}))
	t.Cleanup(func() {
		server.Close()
		if !submitSeen {
			t.Fatalf("submit endpoint was not called")
		}
	})

	storePath := filepath.Join(t.TempDir(), "map-run-exports.sqlite")
	storeRef := "sqlite://" + storePath
	runtime, err := openStore(ctx, storeRef)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	catalog := mapCommandExecutableProfileCatalogFixture(server.URL)
	for i := range catalog.TemplateConfigs {
		switch catalog.TemplateConfigs[i].ScopeID {
		case "case.prepare":
			catalog.TemplateConfigs[i].ConfigJSON = `{"caseId":"case.prepare","caseExecution":{"method":"GET","nodeId":"node.prepare","path":"/prepare","expectedHttpCodes":[200]},"exports":[{"name":"item_id","from":"responseBody","path":"item_id"}]}`
		case "case.submit.success":
			catalog.TemplateConfigs[i].ConfigJSON = `{"caseId":"case.submit.success","caseExecution":{"method":"POST","nodeId":"node.submit","path":"/submit","query":{"item_id":"{{override:item_id}}"},"body":{"field":"ok"},"expectedHttpCodes":[200]},"inputs":[{"name":"item_id","source":"previous"}]}`
		}
	}
	if err := runtime.ReplaceProfileCatalog(ctx, catalog); err != nil {
		t.Fatalf("seed executable profile catalog: %v", err)
	}
	closeCLIStore(runtime)
	return storeRef
}

func decodeMapRunCommandReport(t *testing.T, out string) mapRunCommandReport {
	t.Helper()
	var report mapRunCommandReport
	if err := json.NewDecoder(strings.NewReader(out)).Decode(&report); err != nil {
		t.Fatalf("decode map run json: %v\n%s", err, out)
	}
	return report
}

func assertMapRunCommandReport(t *testing.T, report mapRunCommandReport) {
	t.Helper()
	if !report.OK || report.PlanID == "" || report.MapID != "map.profile.flow" || report.Scope != "all" || report.EnvironmentID != "env.local" || report.Status != "passed" {
		t.Fatalf("map run report = %#v", report)
	}
	if report.Summary.TotalTasks != 3 || report.Summary.PassedTasks != 3 || report.Summary.SkippedTasks != 0 || report.Summary.FailedTasks != 0 {
		t.Fatalf("map run summary = %#v", report.Summary)
	}
	if len(report.Tasks) != 3 {
		t.Fatalf("map run tasks = %#v", report.Tasks)
	}
	if report.Tasks[0].Kind != "run_path" || report.Tasks[0].Status != "passed" || report.Tasks[0].WorkflowRunID == "" {
		t.Fatalf("workflow task = %#v", report.Tasks[0])
	}
	if report.Tasks[1].Kind != "reuse_materialization" || report.Tasks[1].Status != "passed" {
		t.Fatalf("materialized replay task = %#v", report.Tasks[1])
	}
	if report.Tasks[2].Kind != "run_case" || report.Tasks[2].Status != "passed" || report.Tasks[2].APICaseRunID == "" {
		t.Fatalf("case task = %#v", report.Tasks[2])
	}
}

func assertStoredMapRunPlan(t *testing.T, ctx context.Context, storeRef string, planID string) {
	t.Helper()
	runtime, err := openStore(ctx, storeRef)
	if err != nil {
		t.Fatalf("reopen store: %v", err)
	}
	defer closeCLIStore(runtime)
	saved, err := runtime.GetTestMapPlan(ctx, planID)
	if err != nil {
		t.Fatalf("get map run plan: %v", err)
	}
	if saved.Instance.Mode != "run" || saved.Instance.Status != "passed" || saved.Instance.StartedAt.IsZero() || saved.Instance.FinishedAt.IsZero() {
		t.Fatalf("saved run instance = %#v", saved.Instance)
	}
	caseTask := saved.Tasks[2]
	if caseTask.EvidenceRoot == "" {
		t.Fatalf("case task should keep evidence root: %#v", caseTask)
	}
	caseRuns, err := runtime.ListAPICaseRuns(ctx, strings.TrimSuffix(caseTask.APICaseRunID, ".case"))
	if err != nil {
		t.Fatalf("list map case runs: %v", err)
	}
	if len(caseRuns) != 1 || caseRuns[0].TestPlanNodeID != "case.submit.field.required" || caseRuns[0].TestPlanOperation != "run_case" || !strings.Contains(caseRuns[0].PlannerSummaryJSON, planID) {
		t.Fatalf("map case run planner metadata = %#v", caseRuns)
	}
}

func assertMapRunExplainCommandReport(t *testing.T, out string, planID string) {
	t.Helper()
	var explain mapRunExplainCommandReport
	if err := json.Unmarshal([]byte(out), &explain); err != nil {
		t.Fatalf("decode map run explain json: %v\n%s", err, out)
	}
	if !explain.OK || explain.PlanID != planID || explain.Status != "passed" || explain.Summary.TotalTasks != 3 || explain.Summary.PassedTasks != 3 || explain.Summary.SkippedTasks != 0 {
		t.Fatalf("map run explain = %#v", explain)
	}
	if len(explain.NextActions) == 0 || !strings.Contains(strings.Join(explain.NextActions, "\n"), "map run explain --plan '"+planID+"'") {
		t.Fatalf("map run explain next actions = %#v", explain.NextActions)
	}
}

func mapCommandExecutableProfileCatalogFixture(baseURL string) store.ProfileCatalog {
	catalog := mapCommandProfileCatalogFixture()
	for i := range catalog.APICases {
		catalog.APICases[i].BaseURL = baseURL
		if catalog.APICases[i].ID == "case.submit.field.required" {
			catalog.APICases[i].PatchJSON = `[{"op":"remove","path":"$.field"}]`
		}
	}
	catalog.TemplateConfigs = []store.CatalogTemplateConfig{
		{
			ID:         "cfg.case.prepare",
			ScopeType:  "api-case",
			ScopeID:    "case.prepare",
			ConfigJSON: `{"caseId":"case.prepare","caseExecution":{"method":"GET","nodeId":"node.prepare","path":"/prepare","expectedHttpCodes":[200]}}`,
			Status:     "active",
		},
		{
			ID:         "cfg.case.submit.success",
			ScopeType:  "api-case",
			ScopeID:    "case.submit.success",
			ConfigJSON: `{"caseId":"case.submit.success","caseExecution":{"method":"POST","nodeId":"node.submit","path":"/submit","body":{"field":"ok"},"expectedHttpCodes":[200]}}`,
			Status:     "active",
		},
		{
			ID:         "cfg.case.submit.field.required",
			ScopeType:  "api-case",
			ScopeID:    "case.submit.field.required",
			ConfigJSON: `{"caseId":"case.submit.field.required","caseExecution":{"method":"POST","nodeId":"node.submit","path":"/validate","body":{"field":"ok"},"expectedHttpCodes":[400],"expectedResponseContains":["field required"]}}`,
			Status:     "active",
		},
	}
	return catalog
}

func seedExecutableMapCommandStoreWithMaterializedFixture(t *testing.T, ctx context.Context) string {
	t.Helper()
	validateSeen := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/validate":
			if r.URL.Query().Get("item_id") != "item-123" {
				t.Fatalf("validate should receive materialized item_id, query=%s", r.URL.RawQuery)
			}
			validateSeen = true
			w.WriteHeader(http.StatusBadRequest)
			_, _ = fmt.Fprint(w, `{"error":"field required"}`)
		default:
			t.Fatalf("unexpected request path: %s", r.URL.Path)
		}
	}))
	t.Cleanup(func() {
		server.Close()
		if !validateSeen {
			t.Fatalf("validate endpoint was not called")
		}
	})

	storePath := filepath.Join(t.TempDir(), "map-run-materialized.sqlite")
	storeRef := "sqlite://" + storePath
	runtime, err := openStore(ctx, storeRef)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	catalog := mapCommandExecutableProfileCatalogFixture(server.URL)
	for i := range catalog.Fixtures {
		if catalog.Fixtures[i].ID == "fixture.before.submit" {
			catalog.Fixtures[i].DataJSON = `{"item_id":"item-123"}`
		}
	}
	for i := range catalog.CaseDependencies {
		if catalog.CaseDependencies[i].CaseID == "case.submit.field.required" {
			catalog.CaseDependencies[i].MappingsJSON = `[{"from":"$.item_id","to":"$.request.item_id"}]`
		}
	}
	for i := range catalog.TemplateConfigs {
		if catalog.TemplateConfigs[i].ScopeID == "case.submit.field.required" {
			catalog.TemplateConfigs[i].ConfigJSON = `{"caseId":"case.submit.field.required","caseExecution":{"method":"POST","nodeId":"node.submit","path":"/validate","query":{"item_id":"{{override:item_id}}"},"body":{"field":"ok"},"expectedHttpCodes":[400],"expectedResponseContains":["field required"]},"inputs":[{"name":"item_id","source":"fixture"}]}`
		}
	}
	if err := runtime.ReplaceProfileCatalog(ctx, catalog); err != nil {
		t.Fatalf("seed executable profile catalog: %v", err)
	}
	closeCLIStore(runtime)
	return storeRef
}
