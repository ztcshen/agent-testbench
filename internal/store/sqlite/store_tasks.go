package sqlite

import (
	"context"
	"fmt"

	"agent-testbench/internal/store"
)

func (s *Store) UpsertAgentTask(ctx context.Context, t store.AgentTask) (store.AgentTask, error) {
	t = store.PrepareAgentTaskForUpsert(t, utcNow())
	if err := s.exec(ctx, fmt.Sprintf(`
insert into agent_tasks (id, name, kind, command, schedule, status, notify_json, summary_json, created_at, updated_at)
values (%s, %s, %s, %s, %s, %s, %s, %s, %s, %s)
on conflict(id) do update set
  name = excluded.name,
  kind = excluded.kind,
  command = excluded.command,
  schedule = excluded.schedule,
  status = excluded.status,
  notify_json = excluded.notify_json,
  summary_json = excluded.summary_json,
  updated_at = excluded.updated_at;`,
		sqlString(t.ID), sqlString(t.Name), sqlString(t.Kind), sqlString(t.Command), sqlString(t.Schedule),
		sqlString(t.Status), sqlString(t.NotifyJSON), sqlString(t.SummaryJSON), sqlString(encodeTime(t.CreatedAt)),
		sqlString(encodeTime(t.UpdatedAt)))); err != nil {
		return store.AgentTask{}, fmt.Errorf("upsert agent task %q: %w", t.ID, err)
	}
	return t, nil
}

func (s *Store) GetAgentTask(ctx context.Context, ref string) (store.AgentTask, error) {
	var rows []agentTaskRow
	if err := s.query(ctx, fmt.Sprintf(`
select %s
from agent_tasks t
where t.id = %s or t.name = %s
order by t.updated_at desc, t.id
limit 1;`, agentTaskSelectColumns(), sqlString(ref), sqlString(ref)), &rows); err != nil {
		return store.AgentTask{}, err
	}
	if len(rows) == 0 {
		return store.AgentTask{}, store.ErrNotFound
	}
	return rows[0].toStore(), nil
}

func (s *Store) ListAgentTasks(ctx context.Context) ([]store.AgentTask, error) {
	var rows []agentTaskRow
	if err := s.query(ctx, fmt.Sprintf(`
select %s
from agent_tasks t
order by t.updated_at desc, t.id;`, agentTaskSelectColumns()), &rows); err != nil {
		return nil, err
	}
	out := make([]store.AgentTask, 0, len(rows))
	for _, row := range rows {
		out = append(out, row.toStore())
	}
	return out, nil
}

func (s *Store) RecordAgentTaskRun(ctx context.Context, r store.AgentTaskRun) (store.AgentTaskRun, error) {
	r = store.PrepareAgentTaskRunForRecord(r, utcNow())
	if err := s.exec(ctx, fmt.Sprintf(`
insert into agent_task_runs (id, task_id, status, command, started_at, finished_at, duration_ms, exit_code, output, error, summary_json, created_at)
values (%s, %s, %s, %s, %s, %s, %d, %d, %s, %s, %s, %s);`,
		sqlString(r.ID), sqlString(r.TaskID), sqlString(r.Status), sqlString(r.Command), sqlString(encodeTime(r.StartedAt)),
		sqlString(encodeTime(r.FinishedAt)), r.DurationMs, r.ExitCode, sqlString(r.Output), sqlString(r.Error),
		sqlString(r.SummaryJSON), sqlString(encodeTime(r.CreatedAt)))); err != nil {
		return store.AgentTaskRun{}, fmt.Errorf("record agent task run %q: %w", r.ID, err)
	}
	return r, nil
}

func (s *Store) ListAgentTaskRuns(ctx context.Context, taskID string, limit int) ([]store.AgentTaskRun, error) {
	if limit <= 0 {
		limit = 20
	}
	var rows []agentTaskRunRow
	if err := s.query(ctx, fmt.Sprintf(`
select id, task_id, status, command, started_at, finished_at, duration_ms, exit_code, output, error, summary_json, created_at
from agent_task_runs where task_id = %s order by started_at desc, created_at desc, id desc limit %d;`, sqlString(taskID), limit), &rows); err != nil {
		return nil, err
	}
	out := make([]store.AgentTaskRun, 0, len(rows))
	for _, row := range rows {
		out = append(out, row.toStore())
	}
	return out, nil
}

func agentTaskSelectColumns() string {
	return `
t.id, t.name, t.kind, t.command, t.schedule, t.status, t.notify_json, t.summary_json, t.created_at, t.updated_at,
coalesce((select r.status from agent_task_runs r where r.task_id = t.id order by r.started_at desc, r.created_at desc, r.id desc limit 1), '') as latest_status,
coalesce((select r.id from agent_task_runs r where r.task_id = t.id order by r.started_at desc, r.created_at desc, r.id desc limit 1), '') as latest_run_id,
coalesce((select r.finished_at from agent_task_runs r where r.task_id = t.id order by r.started_at desc, r.created_at desc, r.id desc limit 1), '') as last_run_at,
(select count(*) from agent_task_runs r where r.task_id = t.id) as run_count`
}

type agentTaskRow struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	Kind         string `json:"kind"`
	Command      string `json:"command"`
	Schedule     string `json:"schedule"`
	Status       string `json:"status"`
	NotifyJSON   string `json:"notify_json"`
	SummaryJSON  string `json:"summary_json"`
	CreatedAt    string `json:"created_at"`
	UpdatedAt    string `json:"updated_at"`
	LatestStatus string `json:"latest_status"`
	LatestRunID  string `json:"latest_run_id"`
	LastRunAt    string `json:"last_run_at"`
	RunCount     int    `json:"run_count"`
}

func (r agentTaskRow) toStore() store.AgentTask {
	return store.AgentTask{
		ID:           r.ID,
		Name:         r.Name,
		Kind:         r.Kind,
		Command:      r.Command,
		Schedule:     r.Schedule,
		Status:       r.Status,
		NotifyJSON:   normalizeJSONText(r.NotifyJSON),
		SummaryJSON:  normalizeJSONText(r.SummaryJSON),
		CreatedAt:    decodeTime(r.CreatedAt),
		UpdatedAt:    decodeTime(r.UpdatedAt),
		LatestStatus: r.LatestStatus,
		LatestRunID:  r.LatestRunID,
		LastRunAt:    decodeTime(r.LastRunAt),
		RunCount:     r.RunCount,
	}
}

type agentTaskRunRow struct {
	ID          string `json:"id"`
	TaskID      string `json:"task_id"`
	Status      string `json:"status"`
	Command     string `json:"command"`
	StartedAt   string `json:"started_at"`
	FinishedAt  string `json:"finished_at"`
	DurationMs  int64  `json:"duration_ms"`
	ExitCode    int    `json:"exit_code"`
	Output      string `json:"output"`
	Error       string `json:"error"`
	SummaryJSON string `json:"summary_json"`
	CreatedAt   string `json:"created_at"`
}

func (r agentTaskRunRow) toStore() store.AgentTaskRun {
	return store.AgentTaskRun{
		ID:          r.ID,
		TaskID:      r.TaskID,
		Status:      r.Status,
		Command:     r.Command,
		StartedAt:   decodeTime(r.StartedAt),
		FinishedAt:  decodeTime(r.FinishedAt),
		DurationMs:  r.DurationMs,
		ExitCode:    r.ExitCode,
		Output:      r.Output,
		Error:       r.Error,
		SummaryJSON: normalizeJSONText(r.SummaryJSON),
		CreatedAt:   decodeTime(r.CreatedAt),
	}
}
