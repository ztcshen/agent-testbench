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
	for _, tt := range []struct {
		name    string
		dialect sqlstore.Dialect
		upsert  []string
	}{
		{
			name:    "postgres",
			dialect: sqlstore.PostgresDialect{},
			upsert:  []string{"on conflict(id) do update", "summary_json = excluded.summary_json"},
		},
		{
			name:    "mysql",
			dialect: sqlstore.MySQLDialect{},
			upsert:  []string{"on duplicate key update", "summary_json = values(summary_json)"},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			db, state := openFakeSQLDB(t)
			defer db.Close()
			s := sqlstore.New(db, tt.dialect)

			task, err := s.UpsertAgentTask(ctx, agentTaskFixture())
			if err != nil {
				t.Fatalf("upsert agent task: %v", err)
			}
			exec := state.lastExec(t)
			assertSQLContains(t, exec.query, tt.name+" task upsert", "insert into agent_tasks", sqlValuesClause(tt.dialect, 10))
			assertSQLContains(t, exec.query, tt.name+" task upsert", tt.upsert...)
			if task.CreatedAt.IsZero() || exec.args[1] != "catalog-smoke" || exec.args[7] != `{"owner":"qa"}` {
				t.Fatalf("%s agent task/args = %#v %#v", tt.name, task, exec.args)
			}

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
			exec = state.lastExec(t)
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
		})
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
