package sqlstore_test

import (
	"context"
	"database/sql/driver"
	"testing"
	"time"

	"agent-testbench/internal/store"
	"agent-testbench/internal/store/sqlstore"
)

func TestStoreRecordsAgentTasksAndRunsThroughDatabaseSQL(t *testing.T) {
	for _, tt := range []agentTaskSQLDialectCase{
		{
			name:      "postgres",
			dialect:   sqlstore.PostgresDialect{},
			bindVar10: "$10",
		},
		{
			name:      "mysql",
			dialect:   sqlstore.MySQLDialect{},
			bindVar10: "?",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			requireAgentTaskSQLDialect(t, tt)
		})
	}
}

type agentTaskSQLDialectCase struct {
	name      string
	dialect   sqlstore.Dialect
	bindVar10 string
}

func requireAgentTaskSQLDialect(t *testing.T, tt agentTaskSQLDialectCase) {
	t.Helper()
	ctx := context.Background()
	db, state := openFakeSQLDB(t)
	defer db.Close()
	s := sqlstore.New(db, tt.dialect)

	task := requireAgentTaskUpsertSQL(t, ctx, s, state, tt)
	requireAgentTaskTransitionsSQL(t, ctx, s, state, tt, task)
	requireAgentTaskQueriesSQL(t, ctx, s, state, tt, task)
}

func requireAgentTaskUpsertSQL(t *testing.T, ctx context.Context, s *sqlstore.Store, state *fakeSQLState, tt agentTaskSQLDialectCase) store.AgentTask {
	t.Helper()
	state.queueExecRowsAffected(0)
	state.queueRows(fakeRows{})
	task, err := s.UpsertAgentTask(ctx, agentTaskFixture())
	if err != nil {
		t.Fatalf("upsert agent task: %v", err)
	}
	exec := state.lastExec(t)
	assertSQLContains(t, exec.query, tt.name+" task upsert", "insert into agent_tasks", sqlValuesClause(tt.dialect, 11))
	if task.CreatedAt.IsZero() || exec.args[1] != "catalog-smoke" || exec.args[7] != `{"owner":"qa"}` {
		t.Fatalf("%s agent task/args = %#v %#v", tt.name, task, exec.args)
	}
	upsertCalls := state.lastExecs(t, 2)
	assertSQLContains(t, upsertCalls[0].query, tt.name+" task update guard", "update agent_tasks", "status <>", tt.bindVar10)
	return task
}

func requireAgentTaskTransitionsSQL(t *testing.T, ctx context.Context, s *sqlstore.Store, state *fakeSQLState, tt agentTaskSQLDialectCase, task store.AgentTask) {
	t.Helper()
	claim, claimed, err := s.ClaimScheduledAgentTask(ctx, task.ID, task.UpdatedAt, task.UpdatedAt.Add(time.Second))
	if err != nil || !claimed {
		t.Fatalf("%s claim agent task: claimed=%t err=%v", tt.name, claimed, err)
	}
	exec := state.lastExec(t)
	assertSQLContains(t, exec.query, tt.name+" task claim", "update agent_tasks", "where id =", "and status =", "and updated_at =")
	if exec.args[0] != store.StatusRunning || exec.args[3] != task.ID || exec.args[4] != "scheduled" || exec.args[5] != task.UpdatedAt || exec.args[2] != claim.Token || claim.Token == "" {
		t.Fatalf("%s claim args = %#v", tt.name, exec.args)
	}
	released, err := s.ReleaseScheduledAgentTask(ctx, claim, task.UpdatedAt.Add(2*time.Second))
	if err != nil || !released {
		t.Fatalf("%s release agent task: released=%t err=%v", tt.name, released, err)
	}
	exec = state.lastExec(t)
	if exec.args[0] != "scheduled" || exec.args[2] != task.ID || exec.args[3] != store.StatusRunning || exec.args[4] != claim.Token {
		t.Fatalf("%s release args = %#v", tt.name, exec.args)
	}
	recovered, err := s.RecoverScheduledAgentTask(ctx, task.ID, task.UpdatedAt.Add(3*time.Second))
	if err != nil || !recovered {
		t.Fatalf("%s recover agent task: recovered=%t err=%v", tt.name, recovered, err)
	}
	exec = state.lastExec(t)
	assertSQLContains(t, exec.query, tt.name+" task recovery", "claim_token = ''", "and status =")
}

func requireAgentTaskQueriesSQL(t *testing.T, ctx context.Context, s *sqlstore.Store, state *fakeSQLState, tt agentTaskSQLDialectCase, task store.AgentTask) {
	t.Helper()
	queueAgentTaskRows(state, task)
	loaded, err := s.GetAgentTask(ctx, "catalog-smoke")
	if err != nil {
		t.Fatalf("get agent task: %v", err)
	}
	if loaded.ID != "agent-task-001" || loaded.Name != "catalog-smoke" || loaded.LatestStatus != store.StatusPassed {
		t.Fatalf("%s loaded task = %#v", tt.name, loaded)
	}
	query := state.lastQuery(t)
	assertSQLContains(t, query.query, tt.name+" task get", "from agent_tasks")

	run, err := s.RecordAgentTaskRun(ctx, agentTaskRunFixture(task))
	if err != nil {
		t.Fatalf("record agent task run: %v", err)
	}
	exec := state.lastExec(t)
	assertSQLContains(t, exec.query, tt.name+" task run insert", "insert into agent_task_runs", sqlValuesClause(tt.dialect, 12))
	if run.DurationMs != 250 || exec.args[1] != "agent-task-001" || exec.args[6] != int64(250) {
		t.Fatalf("%s agent task run/args = %#v %#v", tt.name, run, exec.args)
	}

	queueAgentTaskRows(state, task)
	tasks, err := s.ListAgentTasks(ctx)
	if err != nil {
		t.Fatalf("list agent tasks: %v", err)
	}
	if len(tasks) != 1 || tasks[0].RunCount != 1 || tasks[0].LatestStatus != store.StatusPassed {
		t.Fatalf("%s tasks = %#v", tt.name, tasks)
	}

	queueAgentTaskRunRows(state, run)
	runs, err := s.ListAgentTaskRuns(ctx, task.ID, 1)
	if err != nil {
		t.Fatalf("list agent task runs: %v", err)
	}
	if len(runs) != 1 || runs[0].ID != run.ID || runs[0].Output != `{"ok":true}` {
		t.Fatalf("%s task runs = %#v", tt.name, runs)
	}
}

func agentTaskFixture() store.AgentTask {
	now := time.Date(2026, 6, 1, 9, 30, 0, 0, time.UTC)
	return store.AgentTask{
		ID:          "agent-task-001",
		Name:        "catalog-smoke",
		Kind:        "cli",
		Command:     "commands --filter task --json",
		Schedule:    "interval:15m",
		Status:      "scheduled",
		NotifyJSON:  `{"file":"notify.jsonl"}`,
		SummaryJSON: `{"owner":"qa"}`,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
}

func agentTaskRunFixture(task store.AgentTask) store.AgentTaskRun {
	started := task.CreatedAt.Add(time.Minute)
	return store.AgentTaskRun{
		ID:          "agent-task-run-001",
		TaskID:      task.ID,
		Status:      store.StatusPassed,
		Command:     task.Command,
		StartedAt:   started,
		FinishedAt:  started.Add(250 * time.Millisecond),
		ExitCode:    0,
		Output:      `{"ok":true}`,
		SummaryJSON: `{"attempt":1}`,
		CreatedAt:   started,
	}
}

func queueAgentTaskRows(state *fakeSQLState, task store.AgentTask) {
	state.queueRows(fakeRows{
		columns: []string{"id", "name", "kind", "command", "schedule", "status", "notify_json", "summary_json", "created_at", "updated_at", "latest_status", "latest_run_id", "last_run_at", "run_count"},
		values: [][]driver.Value{{
			task.ID, task.Name, task.Kind, task.Command, task.Schedule, task.Status, task.NotifyJSON, task.SummaryJSON,
			task.CreatedAt.Format(time.RFC3339Nano), task.UpdatedAt.Format(time.RFC3339Nano),
			store.StatusPassed, "agent-task-run-001", task.UpdatedAt.Format(time.RFC3339Nano), int64(1),
		}},
	})
}

func queueAgentTaskRunRows(state *fakeSQLState, run store.AgentTaskRun) {
	state.queueRows(fakeRows{
		columns: []string{"id", "task_id", "status", "command", "started_at", "finished_at", "duration_ms", "exit_code", "output", "error", "summary_json", "created_at"},
		values: [][]driver.Value{{
			run.ID, run.TaskID, run.Status, run.Command, run.StartedAt.Format(time.RFC3339Nano), run.FinishedAt.Format(time.RFC3339Nano),
			run.DurationMs, int64(run.ExitCode), run.Output, run.Error, run.SummaryJSON, run.CreatedAt.Format(time.RFC3339Nano),
		}},
	})
}
