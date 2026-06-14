package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"agent-testbench/internal/store"
)

func (s *Store) WorkflowStepRun(ctx context.Context, runID string, stepID string) (store.Run, error) {
	query := `
select r.id, r.profile_id, r.environment_id, r.workflow_id, r.status, r.evidence_root,
  json_object(
    'summary', coalesce(json_extract(r.summary_json, '$.summary'), json('{}')),
    'steps', json_array(json(step.value))
  ) as summary_json,
  r.started_at, r.finished_at, r.created_at, r.updated_at
from runs r, json_each(r.summary_json, '$.steps') as step
where r.id = ?
  and json_valid(r.summary_json)
  and json_extract(step.value, '$.stepId') = ?
limit 1;`
	return scanWorkflowStepRun(s.db.QueryRowContext(ctx, query, runID, stepID))
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
	query := fmt.Sprintf(`
select r.id, r.profile_id, r.environment_id, r.workflow_id, r.status, r.evidence_root,
  json_object(
    'summary', coalesce(json_extract(r.summary_json, '$.summary'), json('{}')),
    'steps', json_array(json(step.value))
  ) as summary_json,
  r.started_at, r.finished_at, r.created_at, r.updated_at
from runs r, json_each(r.summary_json, '$.steps') as step
where r.workflow_id = ?
  and json_valid(r.summary_json)
  and json_extract(step.value, '$.stepId') = ?%s
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
limit 1;`, httpFilter)
	return scanWorkflowStepRun(s.db.QueryRowContext(ctx, query, workflowID, stepID))
}

func scanWorkflowStepRun(row *sql.Row) (store.Run, error) {
	var r store.Run
	timestamps := make([]string, 4)
	if err := row.Scan(
		&r.ID, &r.ProfileID, &r.EnvironmentID, &r.WorkflowID, &r.Status, &r.EvidenceRoot, &r.SummaryJSON,
		&timestamps[0], &timestamps[1], &timestamps[2], &timestamps[3],
	); err != nil {
		if err == sql.ErrNoRows {
			return store.Run{}, store.ErrNotFound
		}
		return store.Run{}, err
	}
	applyWorkflowStepRunTimestamps(&r, timestamps)
	return r, nil
}

func applyWorkflowStepRunTimestamps(r *store.Run, timestamps []string) {
	if len(timestamps) != 4 {
		return
	}
	r.StartedAt = decodeSQLiteTime(timestamps[0])
	r.FinishedAt = decodeSQLiteTime(timestamps[1])
	r.CreatedAt = decodeSQLiteTime(timestamps[2])
	r.UpdatedAt = decodeSQLiteTime(timestamps[3])
}

func decodeSQLiteTime(value string) time.Time {
	if value == "" {
		return time.Time{}
	}
	t, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return time.Time{}
	}
	return t.UTC()
}
