package sqlstore

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"agent-testbench/internal/store"
)

func (s *Store) UpsertAgentTask(ctx context.Context, t store.AgentTask) (store.AgentTask, error) {
	t = store.PrepareAgentTaskForUpsert(t, utcNow())
	updated, err := s.updateAgentTaskDefinition(ctx, t)
	if err != nil {
		return store.AgentTask{}, err
	}
	if updated {
		return t, nil
	}
	if existing, existingErr := s.GetAgentTask(ctx, t.ID); existingErr == nil {
		if existing.Status == store.StatusRunning {
			return store.AgentTask{}, fmt.Errorf("%w: %s", store.ErrAgentTaskClaimed, t.ID)
		}
		updated, retryErr := s.updateAgentTaskDefinition(ctx, t)
		if retryErr != nil {
			return store.AgentTask{}, retryErr
		}
		if updated {
			return t, nil
		}
		return store.AgentTask{}, fmt.Errorf("%w: %s", store.ErrAgentTaskClaimed, t.ID)
	} else if !errors.Is(existingErr, store.ErrNotFound) {
		return store.AgentTask{}, existingErr
	}
	query := fmt.Sprintf(`
insert into agent_tasks (id, name, kind, command, schedule, status, notify_json, summary_json, claim_token, created_at, updated_at)
values (%s);`, s.bindVars(11))
	if _, err := s.db.ExecContext(ctx, query,
		t.ID, t.Name, t.Kind, t.Command, t.Schedule, t.Status, t.NotifyJSON, t.SummaryJSON, "",
		dbTimeArg(s.dialect, t.CreatedAt), dbTimeArg(s.dialect, t.UpdatedAt)); err != nil {
		if existing, existingErr := s.GetAgentTask(ctx, t.ID); existingErr == nil && existing.Status == store.StatusRunning {
			return store.AgentTask{}, fmt.Errorf("%w: %s", store.ErrAgentTaskClaimed, t.ID)
		}
		return store.AgentTask{}, fmt.Errorf("insert agent task %q: %w", t.ID, err)
	}
	return t, nil
}

func (s *Store) updateAgentTaskDefinition(ctx context.Context, t store.AgentTask) (bool, error) {
	bindVar := s.dialect.BindVar
	query := fmt.Sprintf(`
update agent_tasks
set name = %s, kind = %s, command = %s, schedule = %s, status = %s,
    notify_json = %s, summary_json = %s, claim_token = '', updated_at = %s
where id = %s`,
		bindVar(1), bindVar(2), bindVar(3), bindVar(4),
		bindVar(5), bindVar(6), bindVar(7), bindVar(8), bindVar(9))
	args := make([]any, 0, 10)
	args = append(args, t.Name, t.Kind, t.Command, t.Schedule, t.Status, t.NotifyJSON, t.SummaryJSON, dbTimeArg(s.dialect, t.UpdatedAt), t.ID)
	query += " and status <> " + bindVar(10)
	args = append(args, store.StatusRunning)
	query += ";"
	changed, err := execStoreRowMutation(ctx, s.db, fmt.Sprintf("update agent task %q", t.ID), query, args...)
	if err != nil {
		return false, err
	}
	return changed == 1, nil
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

// ClaimScheduledAgentTask atomically moves the scheduled revision observed by
// a worker into running. Matching both status and expectedUpdatedAt prevents a
// stale poll from claiming a later scheduled revision after a completed run.
func (s *Store) ClaimScheduledAgentTask(ctx context.Context, taskID string, expectedUpdatedAt time.Time, claimedAt time.Time) (store.AgentTaskClaim, bool, error) {
	token, err := newAgentTaskClaimToken()
	if err != nil {
		return store.AgentTaskClaim{}, false, fmt.Errorf("create agent task claim token: %w", err)
	}
	revision := nextAgentTaskRevision(claimedAt, expectedUpdatedAt)
	changed, err := s.transitionAgentTaskStatus(ctx, taskID, "scheduled", store.StatusRunning, expectedUpdatedAt, revision, token, "claim")
	return store.AgentTaskClaim{TaskID: taskID, Revision: revision, Token: token}, changed, err
}

// ReleaseScheduledAgentTask returns a completed claim to scheduled only when
// the caller still owns the exact running row revision.
func (s *Store) ReleaseScheduledAgentTask(ctx context.Context, claim store.AgentTaskClaim, releasedAt time.Time) (bool, error) {
	if strings.TrimSpace(claim.Token) == "" {
		return false, nil
	}
	query := fmt.Sprintf(`
update agent_tasks
set status = %s, updated_at = %s, claim_token = ''
where id = %s and status = %s and claim_token = %s;`,
		s.dialect.BindVar(1), s.dialect.BindVar(2), s.dialect.BindVar(3), s.dialect.BindVar(4), s.dialect.BindVar(5))
	revision := nextAgentTaskRevision(releasedAt, claim.Revision)
	changed, err := execStoreRowMutation(
		ctx,
		s.db,
		fmt.Sprintf("release agent task %q", claim.TaskID),
		query,
		"scheduled", dbTimeArg(s.dialect, revision), claim.TaskID, store.StatusRunning, claim.Token,
	)
	if err != nil {
		return false, err
	}
	return changed == 1, nil
}

// RecoverScheduledAgentTask explicitly moves an abandoned running claim to
// paused. It never reschedules the task, so recovery cannot automatically
// replay a command with an unknown outcome.
func (s *Store) RecoverScheduledAgentTask(ctx context.Context, taskID string, recoveredAt time.Time) (bool, error) {
	query := fmt.Sprintf(`
update agent_tasks
set status = %s, updated_at = %s, claim_token = ''
where id = %s and status = %s;`,
		s.dialect.BindVar(1), s.dialect.BindVar(2), s.dialect.BindVar(3), s.dialect.BindVar(4))
	changed, err := execStoreRowMutation(
		ctx,
		s.db,
		fmt.Sprintf("recover agent task %q", taskID),
		query,
		"paused", dbTimeArg(s.dialect, recoveredAt.UTC().Truncate(time.Microsecond)), taskID, store.StatusRunning,
	)
	if err != nil {
		return false, err
	}
	return changed == 1, nil
}

func (s *Store) transitionAgentTaskStatus(ctx context.Context, taskID string, from string, to string, expectedUpdatedAt time.Time, at time.Time, token string, operation string) (bool, error) {
	query := fmt.Sprintf(`
update agent_tasks
set status = %s, updated_at = %s, claim_token = %s
where id = %s and status = %s and updated_at = %s;`,
		s.dialect.BindVar(1), s.dialect.BindVar(2), s.dialect.BindVar(3), s.dialect.BindVar(4), s.dialect.BindVar(5), s.dialect.BindVar(6))
	changed, err := execStoreRowMutation(
		ctx,
		s.db,
		fmt.Sprintf("%s agent task %q", operation, taskID),
		query,
		to, dbTimeArg(s.dialect, at), token, taskID, from, dbTimeArg(s.dialect, expectedUpdatedAt),
	)
	if err != nil {
		return false, err
	}
	return changed == 1, nil
}

func newAgentTaskClaimToken() (string, error) {
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return hex.EncodeToString(raw), nil
}

func nextAgentTaskRevision(candidate time.Time, previous time.Time) time.Time {
	revision := candidate.UTC().Truncate(time.Microsecond)
	previous = previous.UTC()
	if !revision.After(previous) {
		revision = previous.Truncate(time.Microsecond).Add(time.Microsecond)
	}
	return revision
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
