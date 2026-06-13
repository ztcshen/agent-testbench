package sqlstore

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"agent-testbench/internal/store"
)

func (s *Store) RecordAPICaseRun(ctx context.Context, r store.APICaseRun) (store.APICaseRun, error) {
	if r.CreatedAt.IsZero() {
		r.CreatedAt = utcNow()
	}
	r.RequestSummaryJSON = stringDefault(r.RequestSummaryJSON, "{}")
	r.AssertionSummaryJSON = stringDefault(r.AssertionSummaryJSON, "{}")
	query := fmt.Sprintf(`
insert into api_case_runs (id, run_id, case_id, status, request_summary_json, assertion_summary_json, started_at, finished_at, created_at)
values (%s);`, s.bindVars(9))
	if _, err := s.db.ExecContext(ctx, query,
		r.ID, r.RunID, r.CaseID, r.Status, r.RequestSummaryJSON, r.AssertionSummaryJSON,
		dbTimeArg(s.dialect, r.StartedAt), dbTimeArg(s.dialect, r.FinishedAt), dbTimeArg(s.dialect, r.CreatedAt)); err != nil {
		return store.APICaseRun{}, fmt.Errorf("record api case run %q: %w", r.ID, err)
	}
	return r, nil
}

func (s *Store) ListAPICaseRuns(ctx context.Context, runID string) ([]store.APICaseRun, error) {
	query := fmt.Sprintf(`
select id, run_id, case_id, status, request_summary_json, assertion_summary_json, started_at, finished_at, created_at
from api_case_runs where run_id = %s order by created_at, id;`, s.dialect.BindVar(1))
	return s.queryAPICaseRuns(ctx, query, runID)
}

func (s *Store) ListLatestAPICaseRuns(ctx context.Context) ([]store.APICaseRun, error) {
	return s.queryAPICaseRuns(ctx, `
select id, run_id, case_id, status, request_summary_json, assertion_summary_json, started_at, finished_at, created_at
from (
  select id, run_id, case_id, status, request_summary_json, assertion_summary_json, started_at, finished_at, created_at,
    row_number() over (partition by case_id order by created_at desc, id desc) as rn
  from api_case_runs
  where case_id <> ''
) latest
where rn = 1
order by created_at, id;`)
}

func (s *Store) ListAPICaseRunRecordsForCaseIDs(ctx context.Context, caseIDs []string) ([]store.APICaseRunRecord, error) {
	args := make([]any, 0, len(caseIDs))
	for _, id := range caseIDs {
		id = strings.TrimSpace(id)
		if id != "" {
			args = append(args, id)
		}
	}
	if len(args) == 0 {
		return []store.APICaseRunRecord{}, nil
	}
	query := fmt.Sprintf(`
select
  r.id, r.profile_id, r.environment_id, r.workflow_id, r.status, r.evidence_root, r.summary_json,
  r.started_at, r.finished_at, r.created_at, r.updated_at,
  acr.id, acr.run_id, acr.case_id, acr.status, acr.request_summary_json, acr.assertion_summary_json,
  acr.started_at, acr.finished_at, acr.created_at
from api_case_runs acr
join runs r on r.id = acr.run_id
where acr.case_id in (%s)
order by acr.created_at desc, acr.id desc;`, bindVars(s.dialect, len(args)))
	return queryStoreRows(ctx, s.db, query, scanAPICaseRunRecord, args...)
}

func (s *Store) queryAPICaseRuns(ctx context.Context, query string, args ...any) ([]store.APICaseRun, error) {
	return queryStoreRows(ctx, s.db, query, scanAPICaseRun, args...)
}

func scanAPICaseRunRecord(row scanner) (store.APICaseRunRecord, error) {
	var record store.APICaseRunRecord
	var runStartedAt, runFinishedAt, runCreatedAt, runUpdatedAt any
	var caseStartedAt, caseFinishedAt, caseCreatedAt any
	if err := row.Scan(
		&record.Run.ID, &record.Run.ProfileID, &record.Run.EnvironmentID, &record.Run.WorkflowID,
		&record.Run.Status, &record.Run.EvidenceRoot, &record.Run.SummaryJSON,
		&runStartedAt, &runFinishedAt, &runCreatedAt, &runUpdatedAt,
		&record.CaseRun.ID, &record.CaseRun.RunID, &record.CaseRun.CaseID, &record.CaseRun.Status,
		&record.CaseRun.RequestSummaryJSON, &record.CaseRun.AssertionSummaryJSON,
		&caseStartedAt, &caseFinishedAt, &caseCreatedAt,
	); err != nil {
		return store.APICaseRunRecord{}, err
	}
	record.Run.SummaryJSON = normalizeJSONText(record.Run.SummaryJSON)
	record.Run.StartedAt = decodeDBTime(runStartedAt)
	record.Run.FinishedAt = decodeDBTime(runFinishedAt)
	record.Run.CreatedAt = decodeDBTime(runCreatedAt)
	record.Run.UpdatedAt = decodeDBTime(runUpdatedAt)
	record.CaseRun.RequestSummaryJSON = normalizeJSONText(record.CaseRun.RequestSummaryJSON)
	record.CaseRun.AssertionSummaryJSON = normalizeJSONText(record.CaseRun.AssertionSummaryJSON)
	record.CaseRun.StartedAt = decodeDBTime(caseStartedAt)
	record.CaseRun.FinishedAt = decodeDBTime(caseFinishedAt)
	record.CaseRun.CreatedAt = decodeDBTime(caseCreatedAt)
	return record, nil
}

func scanAPICaseRun(row scanner) (store.APICaseRun, error) {
	var r store.APICaseRun
	var startedAt, finishedAt, createdAt any
	if err := row.Scan(
		&r.ID, &r.RunID, &r.CaseID, &r.Status, &r.RequestSummaryJSON, &r.AssertionSummaryJSON,
		&startedAt, &finishedAt, &createdAt,
	); err != nil {
		if err == sql.ErrNoRows {
			return store.APICaseRun{}, store.ErrNotFound
		}
		return store.APICaseRun{}, err
	}
	r.RequestSummaryJSON = normalizeJSONText(r.RequestSummaryJSON)
	r.AssertionSummaryJSON = normalizeJSONText(r.AssertionSummaryJSON)
	r.StartedAt = decodeDBTime(startedAt)
	r.FinishedAt = decodeDBTime(finishedAt)
	r.CreatedAt = decodeDBTime(createdAt)
	return r, nil
}
