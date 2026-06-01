package sqlstore

import (
	"context"
	"database/sql"
	"fmt"

	"agent-testbench/internal/store"
)

func (s *Store) UpsertAgentTask(ctx context.Context, t store.AgentTask) (store.AgentTask, error) {
	t = store.PrepareAgentTaskForUpsert(t, utcNow())
	query := fmt.Sprintf(`
insert into agent_tasks (id, name, kind, command, schedule, status, notify_json, summary_json, created_at, updated_at)
values (%s)
%s;`, s.bindVars(10), s.dialect.UpsertClause("id", []string{
		"name", "kind", "command", "schedule", "status", "notify_json", "summary_json", "updated_at",
	}))
	if _, err := s.db.ExecContext(ctx, query,
		t.ID, t.Name, t.Kind, t.Command, t.Schedule, t.Status, t.NotifyJSON, t.SummaryJSON,
		dbTimeArg(s.dialect, t.CreatedAt), dbTimeArg(s.dialect, t.UpdatedAt)); err != nil {
		return store.AgentTask{}, fmt.Errorf("upsert agent task %q: %w", t.ID, err)
	}
	return t, nil
}

func (s *Store) GetAgentTask(ctx context.Context, ref string) (store.AgentTask, error) {
	query := fmt.Sprintf(`
select %s
from agent_tasks t
where t.id = %s or t.name = %s
order by t.updated_at desc, t.id
limit 1;`, s.agentTaskSelectColumns(), s.dialect.BindVar(1), s.dialect.BindVar(2))
	rows, err := queryStoreRows(ctx, s.db, query, scanAgentTask, ref, ref)
	if err != nil {
		return store.AgentTask{}, err
	}
	if len(rows) == 0 {
		return store.AgentTask{}, store.ErrNotFound
	}
	return rows[0], nil
}

func (s *Store) ListAgentTasks(ctx context.Context) ([]store.AgentTask, error) {
	query := fmt.Sprintf(`
select %s
from agent_tasks t
order by t.updated_at desc, t.id;`, s.agentTaskSelectColumns())
	return queryStoreRows(ctx, s.db, query, scanAgentTask)
}

func (s *Store) RecordAgentTaskRun(ctx context.Context, r store.AgentTaskRun) (store.AgentTaskRun, error) {
	r = store.PrepareAgentTaskRunForRecord(r, utcNow())
	query := fmt.Sprintf(`
insert into agent_task_runs (id, task_id, status, command, started_at, finished_at, duration_ms, exit_code, output, error, summary_json, created_at)
values (%s);`, s.bindVars(12))
	if _, err := s.db.ExecContext(ctx, query,
		r.ID, r.TaskID, r.Status, r.Command, dbTimeArg(s.dialect, r.StartedAt), dbTimeArg(s.dialect, r.FinishedAt),
		r.DurationMs, r.ExitCode, r.Output, r.Error, r.SummaryJSON, dbTimeArg(s.dialect, r.CreatedAt)); err != nil {
		return store.AgentTaskRun{}, fmt.Errorf("record agent task run %q: %w", r.ID, err)
	}
	return r, nil
}

func (s *Store) ListAgentTaskRuns(ctx context.Context, taskID string, limit int) ([]store.AgentTaskRun, error) {
	if limit <= 0 {
		limit = 20
	}
	query := fmt.Sprintf(`
select id, task_id, status, command, started_at, finished_at, duration_ms, exit_code, output, error, summary_json, created_at
from agent_task_runs where task_id = %s order by started_at desc, created_at desc, id desc limit %d;`, s.dialect.BindVar(1), limit)
	return queryStoreRows(ctx, s.db, query, scanAgentTaskRun, taskID)
}

func (s *Store) agentTaskSelectColumns() string {
	return fmt.Sprintf(`
t.id, t.name, t.kind, t.command, t.schedule, t.status, t.notify_json, t.summary_json, t.created_at, t.updated_at,
coalesce((select r.status from agent_task_runs r where r.task_id = t.id order by r.started_at desc, r.created_at desc, r.id desc limit 1), '') as latest_status,
coalesce((select r.id from agent_task_runs r where r.task_id = t.id order by r.started_at desc, r.created_at desc, r.id desc limit 1), '') as latest_run_id,
coalesce((select r.finished_at from agent_task_runs r where r.task_id = t.id order by r.started_at desc, r.created_at desc, r.id desc limit 1), %s) as last_run_at,
(select count(*) from agent_task_runs r where r.task_id = t.id) as run_count`, agentTaskEmptyTimeLiteral(s.dialect))
}

func agentTaskEmptyTimeLiteral(d Dialect) string {
	if d.Name() == "sqlite" {
		return "''"
	}
	return "null"
}

func scanAgentTask(row scanner) (store.AgentTask, error) {
	var t store.AgentTask
	var createdAt, updatedAt, lastRunAt any
	if err := row.Scan(
		&t.ID, &t.Name, &t.Kind, &t.Command, &t.Schedule, &t.Status, &t.NotifyJSON, &t.SummaryJSON,
		&createdAt, &updatedAt, &t.LatestStatus, &t.LatestRunID, &lastRunAt, &t.RunCount,
	); err != nil {
		if err == sql.ErrNoRows {
			return store.AgentTask{}, store.ErrNotFound
		}
		return store.AgentTask{}, err
	}
	t.NotifyJSON = normalizeJSONText(t.NotifyJSON)
	t.SummaryJSON = normalizeJSONText(t.SummaryJSON)
	t.CreatedAt = decodeDBTime(createdAt)
	t.UpdatedAt = decodeDBTime(updatedAt)
	t.LastRunAt = decodeDBTime(lastRunAt)
	return t, nil
}

func scanAgentTaskRun(row scanner) (store.AgentTaskRun, error) {
	var r store.AgentTaskRun
	var startedAt, finishedAt, createdAt any
	if err := row.Scan(
		&r.ID, &r.TaskID, &r.Status, &r.Command, &startedAt, &finishedAt, &r.DurationMs, &r.ExitCode,
		&r.Output, &r.Error, &r.SummaryJSON, &createdAt,
	); err != nil {
		if err == sql.ErrNoRows {
			return store.AgentTaskRun{}, store.ErrNotFound
		}
		return store.AgentTaskRun{}, err
	}
	r.SummaryJSON = normalizeJSONText(r.SummaryJSON)
	r.StartedAt = decodeDBTime(startedAt)
	r.FinishedAt = decodeDBTime(finishedAt)
	r.CreatedAt = decodeDBTime(createdAt)
	return r, nil
}
