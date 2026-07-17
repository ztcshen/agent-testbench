package storecontract

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"agent-testbench/internal/store"
	"agent-testbench/internal/store/sqlite"
)

func TestSQLiteStoreContract(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "store.sqlite")

	s, err := sqlite.Open(ctx, sqlite.Config{Path: dbPath})
	if err != nil {
		t.Fatalf("open sqlite store: %v", err)
	}
	defer s.Close()

	exerciseStoreContract(t, ctx, s)
}

func TestSQLiteStoreUsesDefaultPathWhenURLIsEmpty(t *testing.T) {
	cfg := sqlite.Config{BaseDir: t.TempDir()}

	resolved := cfg.Resolve()

	if resolved.Path != filepath.Join(cfg.BaseDir, "runtime", "store.sqlite") {
		t.Fatalf("default sqlite path = %q", resolved.Path)
	}
}

func TestSQLiteStoreCanBeDisabledForPostgresOnlyValidation(t *testing.T) {
	t.Setenv("AGENT_TESTBENCH_DISABLE_SQLITE_STORE", "1")

	_, err := sqlite.Open(context.Background(), sqlite.Config{Path: filepath.Join(t.TempDir(), "store.sqlite")})
	if err == nil {
		t.Fatal("expected sqlite store open to fail when disabled")
	}
	if got := err.Error(); got == "" || !strings.Contains(got, "SQLite Store is disabled") {
		t.Fatalf("sqlite disabled error = %q", got)
	}
}

const (
	contractProfileID  = "empty"
	contractRunID      = "run-001"
	contractWorkflowID = "workflow.smoke"
	contractCaseRunID  = "case-run-001"
	contractCaseID     = "case.health"
	contractStepID     = "step.health"
)

func exerciseStoreContract(t *testing.T, ctx context.Context, s store.Store) {
	t.Helper()

	started := time.Date(2026, 5, 14, 9, 30, 0, 0, time.UTC)
	requireRunContract(t, ctx, s, started)
	requireAPICaseRunContract(t, ctx, s, started)
	requireEvidenceContract(t, ctx, s)
	requirePostProcessTaskContract(t, ctx, s)
	requireAgentTaskContract(t, ctx, s)
	requireTraceTopologyContract(t, ctx, s, started)
	env := requireEnvironmentContract(t, ctx, s, started)
	requireEnvironmentComponentGraphContract(t, ctx, s, env.ID)
	requireBaselineGateContract(t, ctx, s, started)
	requireProfileIndexContract(t, ctx, s, started)
	activeVersion := requireConfigVersionContract(t, ctx, s, started)
	requireReadModelContract(t, ctx, s, activeVersion, started)
	requireProfileCatalogContract(t, ctx, s, started)
	requirePlanGraphContract(t, ctx, s, started)
	requireMapPlannerContract(t, ctx, s, started)
	requireMissingRunError(t, ctx, s)
}

func requireRunContract(t *testing.T, ctx context.Context, s store.Store, started time.Time) {
	t.Helper()

	run, err := s.CreateRun(ctx, store.Run{
		ID:                 contractRunID,
		ProfileID:          contractProfileID,
		WorkflowID:         contractWorkflowID,
		Status:             store.StatusRunning,
		EvidenceRoot:       "evidence/run-001",
		SummaryJSON:        `{"stepCount":1}`,
		TestPlanMapID:      "map.contract",
		TestPlanPathID:     "workflow.alpha",
		PlannerSummaryJSON: `{"selectedPath":"workflow.alpha"}`,
		StartedAt:          started,
	})
	if err != nil {
		t.Fatalf("create run: %v", err)
	}
	if run.CreatedAt.IsZero() {
		t.Fatalf("created run should have CreatedAt: %#v", run)
	}

	loadedRun, err := s.GetRun(ctx, contractRunID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if loadedRun.ProfileID != contractProfileID || loadedRun.WorkflowID != contractWorkflowID || loadedRun.Status != store.StatusRunning {
		t.Fatalf("loaded run = %#v", loadedRun)
	}
	if loadedRun.SummaryJSON != `{"stepCount":1}` {
		t.Fatalf("loaded run summary = %q", loadedRun.SummaryJSON)
	}
	if loadedRun.TestPlanMapID != "map.contract" || loadedRun.TestPlanPathID != "workflow.alpha" || loadedRun.PlannerSummaryJSON != `{"selectedPath":"workflow.alpha"}` {
		t.Fatalf("loaded run planner fields = %#v", loadedRun)
	}
	runs, err := s.ListRuns(ctx)
	if err != nil {
		t.Fatalf("list runs: %v", err)
	}
	if len(runs) != 1 || runs[0].ID != contractRunID {
		t.Fatalf("runs = %#v", runs)
	}
	if runs[0].SummaryJSON != `{"stepCount":1}` {
		t.Fatalf("listed run summary = %q", runs[0].SummaryJSON)
	}
	if runs[0].TestPlanMapID != "map.contract" || runs[0].TestPlanPathID != "workflow.alpha" || runs[0].PlannerSummaryJSON != `{"selectedPath":"workflow.alpha"}` {
		t.Fatalf("listed run planner fields = %#v", runs[0])
	}
}

func requireAPICaseRunContract(t *testing.T, ctx context.Context, s store.Store, started time.Time) {
	t.Helper()

	caseRun, err := s.RecordAPICaseRun(ctx, store.APICaseRun{
		ID:                   contractCaseRunID,
		RunID:                contractRunID,
		CaseID:               contractCaseID,
		Status:               store.StatusPassed,
		RequestSummaryJSON:   `{"method":"GET"}`,
		AssertionSummaryJSON: `{"passed":1}`,
		TestPlanNodeID:       "case.alpha",
		TestPlanOperation:    "run_case",
		PlannerSummaryJSON:   `{"nodeId":"case.alpha"}`,
		StartedAt:            started,
		FinishedAt:           started.Add(250 * time.Millisecond),
	})
	if err != nil {
		t.Fatalf("record api case run: %v", err)
	}
	if caseRun.CreatedAt.IsZero() {
		t.Fatalf("case run should have CreatedAt: %#v", caseRun)
	}

	caseRuns, err := s.ListAPICaseRuns(ctx, contractRunID)
	if err != nil {
		t.Fatalf("list case runs: %v", err)
	}
	if len(caseRuns) != 1 || caseRuns[0].CaseID != contractCaseID || caseRuns[0].Status != store.StatusPassed {
		t.Fatalf("case runs = %#v", caseRuns)
	}
	if caseRuns[0].RequestSummaryJSON != `{"method":"GET"}` || caseRuns[0].AssertionSummaryJSON != `{"passed":1}` {
		t.Fatalf("case run summaries = %#v", caseRuns[0])
	}
	if caseRuns[0].TestPlanNodeID != "case.alpha" || caseRuns[0].TestPlanOperation != "run_case" || caseRuns[0].PlannerSummaryJSON != `{"nodeId":"case.alpha"}` {
		t.Fatalf("case run planner fields = %#v", caseRuns[0])
	}
	latestCaseStore, ok := s.(interface {
		ListLatestAPICaseRuns(context.Context) ([]store.APICaseRun, error)
	})
	if ok {
		latestCaseRuns, err := latestCaseStore.ListLatestAPICaseRuns(ctx)
		if err != nil {
			t.Fatalf("list latest api case runs: %v", err)
		}
		if len(latestCaseRuns) != 1 || latestCaseRuns[0].ID != contractCaseRunID || latestCaseRuns[0].CaseID != contractCaseID {
			t.Fatalf("latest case runs = %#v", latestCaseRuns)
		}
		if latestCaseRuns[0].TestPlanNodeID != "case.alpha" || latestCaseRuns[0].PlannerSummaryJSON != `{"nodeId":"case.alpha"}` {
			t.Fatalf("latest case run planner fields = %#v", latestCaseRuns[0])
		}
	}
}

func requireEvidenceContract(t *testing.T, ctx context.Context, s store.Store) {
	t.Helper()

	evidence, err := s.RecordEvidence(ctx, store.EvidenceRecord{
		ID:         "evidence-001",
		RunID:      contractRunID,
		CaseRunID:  contractCaseRunID,
		StepID:     contractStepID,
		Kind:       "http-response",
		URI:        "evidence/run-001/response.json",
		MediaType:  "application/json",
		SHA256:     "abc123",
		SizeBytes:  42,
		Summary:    "response body",
		Category:   "runtime-attachment",
		Visibility: "public",
		LabelsJSON: `{"owner":"qa","severity":"critical"}`,
	})
	if err != nil {
		t.Fatalf("record evidence: %v", err)
	}
	if evidence.CreatedAt.IsZero() {
		t.Fatalf("evidence should have CreatedAt: %#v", evidence)
	}

	evidenceRecords, err := s.ListEvidence(ctx, contractRunID)
	if err != nil {
		t.Fatalf("list evidence: %v", err)
	}
	if len(evidenceRecords) != 1 || evidenceRecords[0].URI != "evidence/run-001/response.json" {
		t.Fatalf("evidence records = %#v", evidenceRecords)
	}
	if evidenceRecords[0].Kind != "http-response" || evidenceRecords[0].MediaType != "application/json" || evidenceRecords[0].SHA256 != "abc123" || evidenceRecords[0].SizeBytes != 42 || evidenceRecords[0].Summary != "response body" {
		t.Fatalf("evidence metadata = %#v", evidenceRecords[0])
	}
	if evidenceRecords[0].Category != "runtime-attachment" || evidenceRecords[0].Visibility != "public" || evidenceRecords[0].LabelsJSON != `{"owner":"qa","severity":"critical"}` {
		t.Fatalf("evidence attachment metadata = %#v", evidenceRecords[0])
	}
	if evidenceRecords[0].StepID != contractStepID {
		t.Fatalf("evidence step relation = %#v", evidenceRecords[0])
	}
}

func requirePostProcessTaskContract(t *testing.T, ctx context.Context, s store.Store) {
	t.Helper()

	taskStarted := time.Now().UTC().Add(-150 * time.Millisecond)
	taskFinished := taskStarted.Add(125 * time.Millisecond)
	task, err := s.RecordPostProcessTask(ctx, store.PostProcessTask{
		ID:          "task-001",
		RunID:       contractRunID,
		WorkflowID:  "workflow.health",
		StepID:      contractStepID,
		CaseID:      contractCaseID,
		Kind:        "runtime_log_collect",
		Status:      store.StatusPassed,
		StartedAt:   taskStarted,
		FinishedAt:  taskFinished,
		SummaryJSON: `{"systems":2}`,
	})
	if err != nil {
		t.Fatalf("record post process task: %v", err)
	}
	if task.DurationMs != 125 {
		t.Fatalf("task duration should be derived from timestamps: %#v", task)
	}
	tasks, err := s.ListPostProcessTasks(ctx, contractRunID)
	if err != nil {
		t.Fatalf("list post process tasks: %v", err)
	}
	if len(tasks) != 1 || tasks[0].Kind != "runtime_log_collect" || tasks[0].DurationMs != 125 {
		t.Fatalf("post process tasks = %#v", tasks)
	}
}

func requireAgentTaskContract(t *testing.T, ctx context.Context, s store.Store) {
	t.Helper()

	firstClaimAt := time.Date(2026, 6, 1, 10, 0, 0, 123456000, time.UTC)
	task, listedRevision := requireRegisteredAgentTask(t, ctx, s, firstClaimAt.Add(-15*time.Minute))
	firstClaim := requireInitialAgentTaskClaim(t, ctx, s, task, listedRevision, firstClaimAt)
	releasedTask := requireAgentTaskReleaseRevision(t, ctx, s, task, listedRevision, firstClaim, firstClaimAt)
	secondClaim, pausedTask, secondClaimAt := requireRecoveredAgentTask(t, ctx, s, task, releasedTask, firstClaim)
	requireExplicitAgentTaskReschedule(t, ctx, s, task, pausedTask, firstClaim, secondClaim, secondClaimAt)
	requireAgentTaskRunHistory(t, ctx, s, task)
}

func requireRegisteredAgentTask(t *testing.T, ctx context.Context, s store.Store, scheduledAt time.Time) (store.AgentTask, time.Time) {
	t.Helper()
	task, err := s.UpsertAgentTask(ctx, store.AgentTask{
		ID:          "agent-task-001",
		Name:        "catalog-smoke",
		Kind:        "cli",
		Command:     "commands --filter case --json",
		Schedule:    "interval:15m",
		Status:      "scheduled",
		NotifyJSON:  `{"file":"notify.jsonl"}`,
		SummaryJSON: `{"owner":"qa"}`,
		CreatedAt:   scheduledAt,
		UpdatedAt:   scheduledAt,
	})
	if err != nil {
		t.Fatalf("upsert agent task: %v", err)
	}
	if task.CreatedAt.IsZero() || task.UpdatedAt.IsZero() {
		t.Fatalf("agent task should have timestamps: %#v", task)
	}
	loaded, err := s.GetAgentTask(ctx, "catalog-smoke")
	if err != nil {
		t.Fatalf("get agent task by name: %v", err)
	}
	if loaded.ID != "agent-task-001" || loaded.Command != "commands --filter case --json" || loaded.Schedule != "interval:15m" {
		t.Fatalf("loaded agent task = %#v", loaded)
	}
	return task, loaded.UpdatedAt
}

func requireInitialAgentTaskClaim(t *testing.T, ctx context.Context, s store.Store, task store.AgentTask, listedRevision time.Time, firstClaimAt time.Time) store.AgentTaskClaim {
	t.Helper()
	firstClaim, claimed, err := s.ClaimScheduledAgentTask(ctx, task.ID, listedRevision, firstClaimAt)
	if err != nil || !claimed || firstClaim.Token == "" {
		t.Fatalf("claim scheduled agent task: claim=%#v claimed=%t err=%v", firstClaim, claimed, err)
	}
	_, claimedAgain, err := s.ClaimScheduledAgentTask(ctx, task.ID, listedRevision, firstClaimAt.Add(time.Second))
	if err != nil || claimedAgain {
		t.Fatalf("claim scheduled agent task twice: claimed=%t err=%v", claimedAgain, err)
	}
	claimedTask, err := s.GetAgentTask(ctx, task.ID)
	if err != nil || claimedTask.Status != store.StatusRunning {
		t.Fatalf("claimed agent task = %#v err=%v", claimedTask, err)
	}
	for _, status := range []string{"paused", "scheduled"} {
		maintenance := claimedTask
		maintenance.Status = status
		maintenance.Command = "commands --filter changed --json"
		maintenance.UpdatedAt = firstClaimAt.Add(2 * time.Second)
		if _, maintenanceErr := s.UpsertAgentTask(ctx, maintenance); !errors.Is(maintenanceErr, store.ErrAgentTaskClaimed) {
			t.Fatalf("upsert running task as %s error = %v, want ErrAgentTaskClaimed", status, maintenanceErr)
		}
	}
	stillClaimed, err := s.GetAgentTask(ctx, task.ID)
	if err != nil || stillClaimed.Status != store.StatusRunning || stillClaimed.Command != task.Command {
		t.Fatalf("running claim changed during maintenance: task=%#v err=%v", stillClaimed, err)
	}
	return firstClaim
}

func requireAgentTaskReleaseRevision(t *testing.T, ctx context.Context, s store.Store, task store.AgentTask, listedRevision time.Time, firstClaim store.AgentTaskClaim, releasedAt time.Time) store.AgentTask {
	t.Helper()
	// Even if claim and release timestamps collide at Store precision, the
	// durable revision must advance so an old scheduled snapshot stays stale.
	released, err := s.ReleaseScheduledAgentTask(ctx, firstClaim, releasedAt)
	if err != nil || !released {
		t.Fatalf("release scheduled agent task: released=%t err=%v", released, err)
	}
	releasedAgain, err := s.ReleaseScheduledAgentTask(ctx, firstClaim, releasedAt.Add(4*time.Second))
	if err != nil || releasedAgain {
		t.Fatalf("release scheduled agent task twice: released=%t err=%v", releasedAgain, err)
	}
	_, staleClaimed, err := s.ClaimScheduledAgentTask(ctx, task.ID, listedRevision, releasedAt.Add(time.Second))
	if err != nil || staleClaimed {
		t.Fatalf("stale poll reclaimed completed interval: claimed=%t err=%v", staleClaimed, err)
	}
	releasedTask, err := s.GetAgentTask(ctx, task.ID)
	if err != nil || releasedTask.Status != "scheduled" || !releasedTask.UpdatedAt.After(firstClaim.Revision) {
		t.Fatalf("released agent task revision = %#v err=%v", releasedTask, err)
	}
	return releasedTask
}

func requireRecoveredAgentTask(t *testing.T, ctx context.Context, s store.Store, task store.AgentTask, releasedTask store.AgentTask, firstClaim store.AgentTaskClaim) (store.AgentTaskClaim, store.AgentTask, time.Time) {
	t.Helper()
	secondClaimAt := releasedTask.UpdatedAt.Add(15 * time.Minute)
	secondClaim, claimed, err := s.ClaimScheduledAgentTask(ctx, task.ID, releasedTask.UpdatedAt, secondClaimAt)
	if err != nil || !claimed || secondClaim.Token == "" || secondClaim.Token == firstClaim.Token {
		t.Fatalf("claim scheduled agent task after release: claim=%#v claimed=%t err=%v", secondClaim, claimed, err)
	}
	staleRelease, err := s.ReleaseScheduledAgentTask(ctx, firstClaim, secondClaimAt.Add(time.Second))
	if err != nil || staleRelease {
		t.Fatalf("stale owner released new claim: released=%t err=%v", staleRelease, err)
	}
	newOwnerTask, err := s.GetAgentTask(ctx, task.ID)
	if err != nil || newOwnerTask.Status != store.StatusRunning {
		t.Fatalf("new owner claim after stale release = %#v err=%v", newOwnerTask, err)
	}
	recovered, err := s.RecoverScheduledAgentTask(ctx, task.ID, secondClaimAt.Add(2*time.Second))
	if err != nil || !recovered {
		t.Fatalf("recover new owner claim: recovered=%t err=%v", recovered, err)
	}
	pausedTask, err := s.GetAgentTask(ctx, task.ID)
	if err != nil || pausedTask.Status != "paused" {
		t.Fatalf("recovered task should be paused without replay: %#v err=%v", pausedTask, err)
	}
	secondReleased, err := s.ReleaseScheduledAgentTask(ctx, secondClaim, secondClaimAt.Add(3*time.Second))
	if err != nil || secondReleased {
		t.Fatalf("recovered owner released paused task: released=%t err=%v", secondReleased, err)
	}
	_, claimedWhilePaused, err := s.ClaimScheduledAgentTask(ctx, task.ID, pausedTask.UpdatedAt, secondClaimAt.Add(3*time.Second))
	if err != nil || claimedWhilePaused {
		t.Fatalf("recovered paused task replayed automatically: claimed=%t err=%v", claimedWhilePaused, err)
	}
	return secondClaim, pausedTask, secondClaimAt
}

func requireExplicitAgentTaskReschedule(t *testing.T, ctx context.Context, s store.Store, task store.AgentTask, pausedTask store.AgentTask, firstClaim store.AgentTaskClaim, secondClaim store.AgentTaskClaim, secondClaimAt time.Time) {
	t.Helper()
	pausedTask.Status = "scheduled"
	pausedTask.UpdatedAt = secondClaimAt.Add(4 * time.Second)
	rescheduledTask, err := s.UpsertAgentTask(ctx, pausedTask)
	if err != nil {
		t.Fatalf("explicitly reschedule recovered task: %v", err)
	}
	thirdClaimAt := rescheduledTask.UpdatedAt.Add(15 * time.Minute)
	thirdClaim, claimed, err := s.ClaimScheduledAgentTask(ctx, task.ID, rescheduledTask.UpdatedAt, thirdClaimAt)
	if err != nil || !claimed || thirdClaim.Token == firstClaim.Token || thirdClaim.Token == secondClaim.Token {
		t.Fatalf("claim explicitly rescheduled task: claim=%#v claimed=%t err=%v", thirdClaim, claimed, err)
	}
	thirdReleased, err := s.ReleaseScheduledAgentTask(ctx, thirdClaim, thirdClaimAt.Add(time.Second))
	if err != nil || !thirdReleased {
		t.Fatalf("release explicitly rescheduled task: released=%t err=%v", thirdReleased, err)
	}
}

func requireAgentTaskRunHistory(t *testing.T, ctx context.Context, s store.Store, task store.AgentTask) {
	t.Helper()
	run, err := s.RecordAgentTaskRun(ctx, store.AgentTaskRun{
		ID:          "agent-task-run-001",
		TaskID:      task.ID,
		Status:      store.StatusPassed,
		Command:     task.Command,
		ExitCode:    0,
		Output:      `{"ok":true}`,
		SummaryJSON: `{"attempt":1}`,
	})
	if err != nil {
		t.Fatalf("record agent task run: %v", err)
	}
	if run.StartedAt.IsZero() || run.FinishedAt.IsZero() {
		t.Fatalf("agent task run should have timestamps: %#v", run)
	}
	tasks, err := s.ListAgentTasks(ctx)
	if err != nil {
		t.Fatalf("list agent tasks: %v", err)
	}
	if len(tasks) != 1 || tasks[0].Name != "catalog-smoke" || tasks[0].LatestStatus != store.StatusPassed || tasks[0].RunCount != 1 {
		t.Fatalf("agent tasks = %#v", tasks)
	}
	runs, err := s.ListAgentTaskRuns(ctx, task.ID, 5)
	if err != nil {
		t.Fatalf("list agent task runs: %v", err)
	}
	if len(runs) != 1 || runs[0].ID != "agent-task-run-001" || runs[0].Output != `{"ok":true}` {
		t.Fatalf("agent task runs = %#v", runs)
	}
}

func requireTraceTopologyContract(t *testing.T, ctx context.Context, s store.Store, started time.Time) {
	t.Helper()

	topology, err := s.SaveTraceTopology(ctx, store.TraceTopology{
		ID:            "topology-001",
		WorkflowRunID: contractRunID,
		WorkflowID:    contractWorkflowID,
		StepID:        contractStepID,
		CaseID:        contractCaseID,
		RequestID:     "request-001",
		TraceID:       "trace-001",
		Status:        "complete",
		TopologyJSON:  `{"confirmedEdges":[{"source":"service.alpha","target":"service.beta"}],"observedNodes":["service.alpha","service.beta"]}`,
		TextTopology:  "service.alpha -> service.beta",
		CreatedAt:     started.Add(500 * time.Millisecond),
	})
	if err != nil {
		t.Fatalf("save trace topology: %v", err)
	}
	if topology.CreatedAt.IsZero() {
		t.Fatalf("trace topology should have CreatedAt: %#v", topology)
	}
	topologies, err := s.ListTraceTopologies(ctx, contractRunID)
	if err != nil {
		t.Fatalf("list trace topologies: %v", err)
	}
	if len(topologies) != 1 || topologies[0].TraceID != "trace-001" || topologies[0].Status != "complete" {
		t.Fatalf("trace topologies = %#v", topologies)
	}
	if topologies[0].TopologyJSON != topology.TopologyJSON || topologies[0].TextTopology != "service.alpha -> service.beta" {
		t.Fatalf("trace topology payload = %#v", topologies[0])
	}
}

func requireEnvironmentComponentGraphContract(t *testing.T, ctx context.Context, s store.Store, envID string) {
	t.Helper()

	graph := contractEnvironmentComponentGraph()
	if err := s.ReplaceEnvironmentComponentGraph(ctx, envID, graph); err != nil {
		t.Fatalf("replace environment component graph: %v", err)
	}
	loadedGraph, err := s.GetEnvironmentComponentGraph(ctx, envID)
	if err != nil {
		t.Fatalf("get environment component graph: %v", err)
	}
	if len(loadedGraph.Components) != 2 || len(loadedGraph.Dependencies) != 1 || len(loadedGraph.Assets) != 1 {
		t.Fatalf("loaded component graph = %#v", loadedGraph)
	}
	if loadedGraph.Dependencies[0].ConsumerComponentID != "service.alpha" || loadedGraph.Dependencies[0].ProviderComponentID != "mysql" || loadedGraph.Dependencies[0].Phase != "startup" {
		t.Fatalf("loaded component dependency = %#v", loadedGraph.Dependencies[0])
	}
	if loadedGraph.Assets[0].OwnerComponentID != "service.alpha" || loadedGraph.Assets[0].TargetComponentID != "mysql" || !jsonEqual(loadedGraph.Assets[0].SummaryJSON, graph.Assets[0].SummaryJSON) {
		t.Fatalf("loaded component asset = %#v", loadedGraph.Assets[0])
	}
}

func contractEnvironmentComponentGraph() store.EnvironmentComponentGraph {
	return store.EnvironmentComponentGraph{
		Components: []store.EnvironmentComponent{
			{
				ComponentID:     "mysql",
				DisplayName:     "MySQL",
				Kind:            "middleware",
				Role:            "database",
				ComposeService:  "mysql",
				Image:           "mysql:8",
				Required:        true,
				RuntimeJSON:     `{"ports":[3306]}`,
				HealthCheckJSON: `{"type":"tcp","address":"127.0.0.1:3306"}`,
				SummaryJSON:     `{}`,
			},
			{
				ComponentID:     "service.alpha",
				DisplayName:     "Service Alpha",
				Kind:            "app",
				Role:            "business-service",
				ComposeService:  "service-alpha",
				Required:        true,
				RuntimeJSON:     `{}`,
				HealthCheckJSON: `{"type":"url","url":"http://127.0.0.1:18080/health"}`,
				SummaryJSON:     `{}`,
			},
		},
		Dependencies: []store.ComponentDependency{
			{
				ConsumerComponentID: "service.alpha",
				ProviderComponentID: "mysql",
				Phase:               "startup",
				Capability:          "sql",
				Required:            true,
				ProfileJSON:         `{"database":"alpha"}`,
			},
		},
		Assets: []store.ComponentConfigAsset{
			{
				OwnerComponentID:  "service.alpha",
				AssetID:           "alpha.mysql.ddl",
				AssetKind:         "mysql-ddl",
				TargetComponentID: "mysql",
				TargetPath:        "compose/mysql/init/alpha.sql",
				ContentInline:     "create table alpha_smoke (id bigint primary key);",
				SizeBytes:         int64(len("create table alpha_smoke (id bigint primary key);")),
				ApplyOrder:        10,
				SummaryJSON:       `{"ownedBy":"service.alpha"}`,
			},
		},
	}
}

func requireBaselineGateContract(t *testing.T, ctx context.Context, s store.Store, started time.Time) {
	t.Helper()

	gate, err := s.UpsertBaselineGate(ctx, store.BaselineGate{
		ProfileID:   contractProfileID,
		SubjectID:   contractWorkflowID,
		Status:      store.StatusPassed,
		Required:    false,
		SummaryJSON: `{"reason":"first green run"}`,
		CheckedAt:   started.Add(time.Minute),
	})
	if err != nil {
		t.Fatalf("upsert baseline gate: %v", err)
	}
	if gate.UpdatedAt.IsZero() {
		t.Fatalf("baseline gate should have UpdatedAt: %#v", gate)
	}

	loadedGate, err := s.GetBaselineGate(ctx, contractProfileID, contractWorkflowID)
	if err != nil {
		t.Fatalf("get baseline gate: %v", err)
	}
	if loadedGate.Status != store.StatusPassed || loadedGate.Required {
		t.Fatalf("loaded baseline gate = %#v", loadedGate)
	}
}

func requireProfileIndexContract(t *testing.T, ctx context.Context, s store.Store, started time.Time) {
	t.Helper()

	profile, err := s.UpsertProfileIndex(ctx, store.ProfileIndex{
		ProfileID:    contractProfileID,
		BundlePath:   "/tmp/external-profile-bundles/empty",
		BundleDigest: "sha256:bundle",
		SummaryJSON:  `{"workflows":0}`,
		ImportedAt:   started.Add(2 * time.Minute),
	})
	if err != nil {
		t.Fatalf("upsert profile index: %v", err)
	}
	if profile.UpdatedAt.IsZero() {
		t.Fatalf("profile index should have UpdatedAt: %#v", profile)
	}

	loadedProfile, err := s.GetProfileIndex(ctx, contractProfileID)
	if err != nil {
		t.Fatalf("get profile index: %v", err)
	}
	if loadedProfile.BundlePath != "/tmp/external-profile-bundles/empty" || loadedProfile.BundleDigest != "sha256:bundle" {
		t.Fatalf("loaded profile index = %#v", loadedProfile)
	}
}

func requireConfigVersionContract(t *testing.T, ctx context.Context, s store.Store, started time.Time) store.ConfigVersion {
	t.Helper()

	version, err := s.UpsertConfigVersion(ctx, store.ConfigVersion{
		ID:           "config.empty.001",
		ProfileID:    contractProfileID,
		SourcePath:   "/tmp/external-profile-bundles/empty",
		BundleDigest: "sha256:bundle",
		SummaryJSON:  `{"services":1}`,
		Active:       true,
		PublishedAt:  started.Add(3 * time.Minute),
	})
	if err != nil {
		t.Fatalf("upsert config version: %v", err)
	}
	if version.CreatedAt.IsZero() {
		t.Fatalf("config version should have CreatedAt: %#v", version)
	}
	activeVersion, err := s.GetActiveConfigVersion(ctx)
	if err != nil {
		t.Fatalf("get active config version: %v", err)
	}
	if activeVersion.ID != "config.empty.001" || activeVersion.ProfileID != "empty" || activeVersion.BundleDigest != "sha256:bundle" || !activeVersion.Active {
		t.Fatalf("active config version = %#v", activeVersion)
	}
	_, err = s.UpsertConfigVersion(ctx, store.ConfigVersion{
		ID:           "config.other.001",
		ProfileID:    "other",
		SourcePath:   "/tmp/external-profile-bundles/other",
		BundleDigest: "sha256:other",
		SummaryJSON:  `{"services":0}`,
		Active:       true,
		PublishedAt:  started.Add(4 * time.Minute),
	})
	if err != nil {
		t.Fatalf("upsert other active config version: %v", err)
	}
	reactivated, err := s.ActivateLatestConfigVersion(ctx, contractProfileID)
	if err != nil {
		t.Fatalf("activate latest config version: %v", err)
	}
	if reactivated.ID != "config.empty.001" || reactivated.ProfileID != contractProfileID || !reactivated.Active {
		t.Fatalf("reactivated config version = %#v", reactivated)
	}
	activeVersion, err = s.GetActiveConfigVersion(ctx)
	if err != nil {
		t.Fatalf("get reactivated config version: %v", err)
	}
	if activeVersion.ID != "config.empty.001" || activeVersion.ProfileID != contractProfileID || !activeVersion.Active {
		t.Fatalf("active config version after restore = %#v", activeVersion)
	}
	return activeVersion
}

func requireReadModelContract(t *testing.T, ctx context.Context, s store.Store, activeVersion store.ConfigVersion, started time.Time) {
	t.Helper()

	readModel, err := s.UpsertReadModel(ctx, store.ReadModel{
		ProfileID:       contractProfileID,
		Key:             "interface-nodes",
		ConfigVersionID: activeVersion.ID,
		PayloadJSON:     `{"ok":true,"items":[]}`,
		GeneratedAt:     started.Add(4 * time.Minute),
	})
	if err != nil {
		t.Fatalf("upsert read model: %v", err)
	}
	if readModel.UpdatedAt.IsZero() {
		t.Fatalf("read model should have UpdatedAt: %#v", readModel)
	}
	loadedReadModel, err := s.GetReadModel(ctx, contractProfileID, "interface-nodes")
	if err != nil {
		t.Fatalf("get read model: %v", err)
	}
	if loadedReadModel.ConfigVersionID != activeVersion.ID || !jsonEqual(loadedReadModel.PayloadJSON, `{"ok":true,"items":[]}`) {
		t.Fatalf("loaded read model = %#v", loadedReadModel)
	}
}

func requireProfileCatalogContract(t *testing.T, ctx context.Context, s store.Store, started time.Time) {
	t.Helper()

	if err := s.ReplaceProfileCatalog(ctx, contractProfileCatalog(started)); err != nil {
		t.Fatalf("replace profile catalog index: %v", err)
	}
	catalogIndex, err := s.GetProfileCatalogIndex(ctx)
	if err != nil {
		t.Fatalf("get profile catalog index: %v", err)
	}
	if catalogIndex.ProfileID != contractProfileID || catalogIndex.IndexedAt.IsZero() {
		t.Fatalf("profile catalog index identity = %#v", catalogIndex)
	}
	if catalogIndex.Counts.Services != 1 || catalogIndex.Counts.Workflows != 1 || catalogIndex.Counts.APICases != 1 || catalogIndex.Counts.Templates != 2 {
		t.Fatalf("profile catalog index counts = %#v", catalogIndex.Counts)
	}
	catalog, err := s.GetProfileCatalog(ctx)
	if err != nil {
		t.Fatalf("get profile catalog: %v", err)
	}
	if len(catalog.Services) != 1 || catalog.Services[0].SourcePath != "/tmp/source/service.alpha" {
		t.Fatalf("profile catalog services = %#v", catalog.Services)
	}
	if len(catalog.APICases) != 1 || catalog.APICases[0].CasePath != "cases/case.alpha.json" || catalog.APICases[0].SourceKind != "karate" || catalog.APICases[0].SourcePath != "tests/api.feature" || catalog.APICases[0].ExecutorID != "executor.karate" || catalog.APICases[0].BaseURL != "http://127.0.0.1:18080" || catalog.APICases[0].EvidenceDir != ".runtime/cases" || catalog.APICases[0].TimeoutSeconds != 12 || catalog.APICases[0].DefaultOverridesJSON != `{"itemId":"item-001"}` {
		t.Fatalf("profile catalog api case run config = %#v", catalog.APICases)
	}
	catalogByID, err := s.GetProfileCatalogByID(ctx, contractProfileID)
	if err != nil {
		t.Fatalf("get profile catalog by id: %v", err)
	}
	if catalogByID.ProfileID != contractProfileID || len(catalogByID.Workflows) != 1 {
		t.Fatalf("profile catalog by id = %#v", catalogByID)
	}
	indexes, err := s.ListProfileCatalogIndexes(ctx)
	if err != nil {
		t.Fatalf("list profile catalog indexes: %v", err)
	}
	if len(indexes) != 1 || indexes[0].ProfileID != contractProfileID || indexes[0].Counts.APICases != 1 {
		t.Fatalf("profile catalog indexes = %#v", indexes)
	}
}

func requireMissingRunError(t *testing.T, ctx context.Context, s store.Store) {
	t.Helper()

	_, err := s.GetRun(ctx, "missing")
	if !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("missing run error = %v, want ErrNotFound", err)
	}
}

func jsonEqual(left string, right string) bool {
	var leftValue any
	var rightValue any
	if err := json.Unmarshal([]byte(left), &leftValue); err != nil {
		return false
	}
	if err := json.Unmarshal([]byte(right), &rightValue); err != nil {
		return false
	}
	return reflect.DeepEqual(leftValue, rightValue)
}
