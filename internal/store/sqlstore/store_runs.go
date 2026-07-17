package sqlstore

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"agent-testbench/internal/store"
)

func (s *Store) CreateRun(ctx context.Context, r store.Run) (store.Run, error) {
	now := utcNow()
	if r.CreatedAt.IsZero() {
		r.CreatedAt = now
	}
	if r.UpdatedAt.IsZero() {
		r.UpdatedAt = r.CreatedAt
	}
	r.SummaryJSON = stringDefault(r.SummaryJSON, "{}")
	r.PlannerSummaryJSON = stringDefault(r.PlannerSummaryJSON, "{}")
	query := fmt.Sprintf(`
insert into runs (id, profile_id, environment_id, workflow_id, status, evidence_root, summary_json, test_plan_map_id, test_plan_path_id, planner_summary_json, started_at, finished_at, created_at, updated_at)
values (%s);`, s.bindVars(14))
	if _, err := s.db.ExecContext(ctx, query,
		r.ID, r.ProfileID, r.EnvironmentID, r.WorkflowID, r.Status, r.EvidenceRoot, r.SummaryJSON,
		r.TestPlanMapID, r.TestPlanPathID, r.PlannerSummaryJSON,
		dbTimeArg(s.dialect, r.StartedAt), dbTimeArg(s.dialect, r.FinishedAt), dbTimeArg(s.dialect, r.CreatedAt), dbTimeArg(s.dialect, r.UpdatedAt)); err != nil {
		return store.Run{}, fmt.Errorf("create run %q: %w", r.ID, err)
	}
	return r, nil
}

func (s *Store) UpdateRun(ctx context.Context, r store.Run) (store.Run, error) {
	return s.updateRun(ctx, r, fmt.Sprintf("id = %s", s.dialect.BindVar(13)), []any{r.ID}, store.ErrNotFound)
}

func (s *Store) CompareAndSwapRun(ctx context.Context, expectedUpdatedAt time.Time, expectedStatus string, r store.Run) (store.Run, error) {
	if r.ID == "" {
		return store.Run{}, fmt.Errorf("compare and swap run: id is required")
	}
	if expectedUpdatedAt.IsZero() {
		return store.Run{}, fmt.Errorf("compare and swap run %q: expected updated_at is required", r.ID)
	}
	if expectedStatus == "" {
		return store.Run{}, fmt.Errorf("compare and swap run %q: expected status is required", r.ID)
	}
	if !r.UpdatedAt.After(expectedUpdatedAt) {
		r.UpdatedAt = utcNow().Truncate(time.Microsecond)
		if !r.UpdatedAt.After(expectedUpdatedAt) {
			r.UpdatedAt = expectedUpdatedAt.Add(time.Microsecond)
		}
	}
	conflict := &store.RunRevisionConflictError{
		RunID:             r.ID,
		ExpectedStatus:    expectedStatus,
		ExpectedUpdatedAt: expectedUpdatedAt,
	}
	condition := fmt.Sprintf("id = %s and updated_at = %s and status = %s", s.dialect.BindVar(13), s.dialect.BindVar(14), s.dialect.BindVar(15))
	return s.updateRun(ctx, r, condition, []any{r.ID, dbTimeArg(s.dialect, expectedUpdatedAt), expectedStatus}, conflict)
}

func (s *Store) updateRun(ctx context.Context, r store.Run, condition string, conditionArgs []any, noRowsErr error) (store.Run, error) {
	if r.ID == "" {
		return store.Run{}, fmt.Errorf("update run: id is required")
	}
	if r.UpdatedAt.IsZero() {
		r.UpdatedAt = utcNow()
	}
	r.SummaryJSON = stringDefault(r.SummaryJSON, "{}")
	r.PlannerSummaryJSON = stringDefault(r.PlannerSummaryJSON, "{}")
	query := fmt.Sprintf(`
update runs
set profile_id = %s,
    environment_id = %s,
    workflow_id = %s,
    status = %s,
    evidence_root = %s,
    summary_json = %s,
    test_plan_map_id = %s,
    test_plan_path_id = %s,
    planner_summary_json = %s,
    started_at = %s,
    finished_at = %s,
    updated_at = %s
where %s;`,
		s.dialect.BindVar(1),
		s.dialect.BindVar(2),
		s.dialect.BindVar(3),
		s.dialect.BindVar(4),
		s.dialect.BindVar(5),
		s.dialect.BindVar(6),
		s.dialect.BindVar(7),
		s.dialect.BindVar(8),
		s.dialect.BindVar(9),
		s.dialect.BindVar(10),
		s.dialect.BindVar(11),
		s.dialect.BindVar(12),
		condition,
	)
	args := make([]any, 0, 12+len(conditionArgs))
	args = append(args,
		r.ProfileID, r.EnvironmentID, r.WorkflowID, r.Status, r.EvidenceRoot, r.SummaryJSON,
		r.TestPlanMapID, r.TestPlanPathID, r.PlannerSummaryJSON,
		dbTimeArg(s.dialect, r.StartedAt), dbTimeArg(s.dialect, r.FinishedAt), dbTimeArg(s.dialect, r.UpdatedAt),
	)
	args = append(args, conditionArgs...)
	affected, err := execStoreRowMutation(ctx, s.db, fmt.Sprintf("update run %q", r.ID), query, args...)
	if err != nil {
		return store.Run{}, err
	}
	if affected == 0 {
		return store.Run{}, noRowsErr
	}
	return r, nil
}

func (s *Store) GetRun(ctx context.Context, id string) (store.Run, error) {
	query := fmt.Sprintf(`
	select id, profile_id, environment_id, workflow_id, status, evidence_root, summary_json, test_plan_map_id, test_plan_path_id, planner_summary_json, started_at, finished_at, created_at, updated_at
from runs where id = %s;`, s.dialect.BindVar(1))
	r, err := scanRun(s.db.QueryRowContext(ctx, query, id))
	if err != nil {
		return store.Run{}, err
	}
	return r, nil
}

func (s *Store) ListRuns(ctx context.Context) ([]store.Run, error) {
	return queryStoreRows(ctx, s.db, `
select id, profile_id, environment_id, workflow_id, status, evidence_root, summary_json, test_plan_map_id, test_plan_path_id, planner_summary_json, started_at, finished_at, created_at, updated_at
from runs order by created_at, id;`, scanRun)
}

func scanRun(row scanner) (store.Run, error) {
	var r store.Run
	var startedAt, finishedAt, createdAt, updatedAt any
	if err := row.Scan(
		&r.ID, &r.ProfileID, &r.EnvironmentID, &r.WorkflowID, &r.Status, &r.EvidenceRoot, &r.SummaryJSON,
		&r.TestPlanMapID, &r.TestPlanPathID, &r.PlannerSummaryJSON,
		&startedAt, &finishedAt, &createdAt, &updatedAt,
	); err != nil {
		if err == sql.ErrNoRows {
			return store.Run{}, store.ErrNotFound
		}
		return store.Run{}, err
	}
	r.SummaryJSON = normalizeJSONText(r.SummaryJSON)
	r.PlannerSummaryJSON = normalizeJSONText(r.PlannerSummaryJSON)
	r.StartedAt = decodeDBTime(startedAt)
	r.FinishedAt = decodeDBTime(finishedAt)
	r.CreatedAt = decodeDBTime(createdAt)
	r.UpdatedAt = decodeDBTime(updatedAt)
	return r, nil
}
