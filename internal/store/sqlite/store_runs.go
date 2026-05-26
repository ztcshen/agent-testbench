package sqlite

import (
	"context"
	"fmt"

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
	if err := s.exec(ctx, fmt.Sprintf(`
insert into runs (id, profile_id, environment_id, workflow_id, status, evidence_root, summary_json, started_at, finished_at, created_at, updated_at)
values (%s, %s, %s, %s, %s, %s, %s, %s, %s, %s, %s);`,
		sqlString(r.ID), sqlString(r.ProfileID), sqlString(r.EnvironmentID), sqlString(r.WorkflowID), sqlString(r.Status), sqlString(r.EvidenceRoot),
		sqlString(r.SummaryJSON), sqlString(encodeTime(r.StartedAt)), sqlString(encodeTime(r.FinishedAt)),
		sqlString(encodeTime(r.CreatedAt)), sqlString(encodeTime(r.UpdatedAt)))); err != nil {
		return store.Run{}, fmt.Errorf("create run %q: %w", r.ID, err)
	}
	return r, nil
}

func (s *Store) GetRun(ctx context.Context, id string) (store.Run, error) {
	var rows []runRow
	if err := s.query(ctx, fmt.Sprintf(`
select id, profile_id, environment_id, workflow_id, status, evidence_root, summary_json, started_at, finished_at, created_at, updated_at
from runs where id = %s;`, sqlString(id)), &rows); err != nil {
		return store.Run{}, err
	}
	if len(rows) == 0 {
		return store.Run{}, store.ErrNotFound
	}
	return rows[0].toStore(), nil
}

func (s *Store) ListRuns(ctx context.Context) ([]store.Run, error) {
	var rows []runRow
	if err := s.query(ctx, `
select id, profile_id, environment_id, workflow_id, status, evidence_root, summary_json, started_at, finished_at, created_at, updated_at
from runs order by created_at, id;`, &rows); err != nil {
		return nil, err
	}
	out := make([]store.Run, 0, len(rows))
	for _, row := range rows {
		out = append(out, row.toStore())
	}
	return out, nil
}

func (s *Store) WorkflowStepRun(ctx context.Context, runID string, stepID string) (store.Run, error) {
	var rows []runRow
	if err := s.query(ctx, fmt.Sprintf(`
select r.id, r.profile_id, r.environment_id, r.workflow_id, r.status, r.evidence_root,
  json_object(
    'summary', coalesce(json_extract(r.summary_json, '$.summary'), json('{}')),
    'steps', json_array(json(step.value))
  ) as summary_json,
  r.started_at, r.finished_at, r.created_at, r.updated_at
from runs r, json_each(r.summary_json, '$.steps') as step
where r.id = %s
  and json_valid(r.summary_json)
  and json_extract(step.value, '$.stepId') = %s
limit 1;`, sqlString(runID), sqlString(stepID)), &rows); err != nil {
		return store.Run{}, err
	}
	if len(rows) == 0 {
		return store.Run{}, store.ErrNotFound
	}
	return rows[0].toStore(), nil
}

func (s *Store) LatestWorkflowStepRun(ctx context.Context, workflowID string, stepID string, requireHTTPResult bool) (store.Run, error) {
	httpFilter := ""
	if requireHTTPResult {
		httpFilter = `
  and (
    coalesce(json_extract(step.value, '$.result.response.statusCode'), 0) > 0
    or coalesce(json_extract(step.value, '$.summary.httpCode'), 0) > 0
  )`
	}
	var rows []runRow
	if err := s.query(ctx, fmt.Sprintf(`
select r.id, r.profile_id, r.environment_id, r.workflow_id, r.status, r.evidence_root,
  json_object(
    'summary', coalesce(json_extract(r.summary_json, '$.summary'), json('{}')),
    'steps', json_array(json(step.value))
  ) as summary_json,
  r.started_at, r.finished_at, r.created_at, r.updated_at
from runs r, json_each(r.summary_json, '$.steps') as step
where r.workflow_id = %s
  and json_valid(r.summary_json)
  and json_extract(step.value, '$.stepId') = %s%s
order by
  case
    when coalesce(json_extract(r.summary_json, '$.kind'), '') <> 'apiCase'
      and coalesce(
        json_extract(r.summary_json, '$.summary.expectedStepCount'),
        json_extract(r.summary_json, '$.summary.stepCount'),
        json_extract(r.summary_json, '$.stepCount'),
        json_array_length(r.summary_json, '$.steps'),
        0
      ) > 1
    then 0 else 1
  end,
  r.created_at desc, r.id desc
limit 1;`, sqlString(workflowID), sqlString(stepID), httpFilter), &rows); err != nil {
		return store.Run{}, err
	}
	if len(rows) == 0 {
		return store.Run{}, store.ErrNotFound
	}
	return rows[0].toStore(), nil
}

func (s *Store) ListRunHeaders(ctx context.Context) ([]store.Run, error) {
	var rows []runRow
	if err := s.query(ctx, `
select id, profile_id, environment_id, workflow_id, status, evidence_root,
  case
    when json_valid(summary_json) then json_object(
      'kind', json_extract(summary_json, '$.kind'),
      'summary', json_object(
        'caseId', json_extract(summary_json, '$.summary.caseId'),
        'expectedStepCount', json_extract(summary_json, '$.summary.expectedStepCount'),
        'stepCount', coalesce(
          json_extract(summary_json, '$.summary.stepCount'),
          json_extract(summary_json, '$.stepCount'),
          json_array_length(summary_json, '$.steps'),
          0
        )
      )
    )
    else '{}'
  end as summary_json,
  started_at, finished_at, created_at, updated_at
from runs order by created_at, id;`, &rows); err != nil {
		return nil, err
	}
	out := make([]store.Run, 0, len(rows))
	for _, row := range rows {
		out = append(out, row.toStore())
	}
	return out, nil
}

type runRow struct {
	ID            string `json:"id"`
	ProfileID     string `json:"profile_id"`
	EnvironmentID string `json:"environment_id"`
	WorkflowID    string `json:"workflow_id"`
	Status        string `json:"status"`
	EvidenceRoot  string `json:"evidence_root"`
	SummaryJSON   string `json:"summary_json"`
	StartedAt     string `json:"started_at"`
	FinishedAt    string `json:"finished_at"`
	CreatedAt     string `json:"created_at"`
	UpdatedAt     string `json:"updated_at"`
}

func (r runRow) toStore() store.Run {
	return store.Run{
		ID:            r.ID,
		ProfileID:     r.ProfileID,
		EnvironmentID: r.EnvironmentID,
		WorkflowID:    r.WorkflowID,
		Status:        r.Status,
		EvidenceRoot:  r.EvidenceRoot,
		SummaryJSON:   r.SummaryJSON,
		StartedAt:     decodeTime(r.StartedAt),
		FinishedAt:    decodeTime(r.FinishedAt),
		CreatedAt:     decodeTime(r.CreatedAt),
		UpdatedAt:     decodeTime(r.UpdatedAt),
	}
}
