package sqlite

import (
	"context"
	"fmt"
	"strings"

	"agent-testbench/internal/store"
)

func (s *Store) RecordAPICaseRun(ctx context.Context, r store.APICaseRun) (store.APICaseRun, error) {
	if r.CreatedAt.IsZero() {
		r.CreatedAt = utcNow()
	}
	if err := s.exec(ctx, fmt.Sprintf(`
insert into api_case_runs (id, run_id, case_id, status, request_summary_json, assertion_summary_json, started_at, finished_at, created_at)
values (%s, %s, %s, %s, %s, %s, %s, %s, %s);`,
		sqlString(r.ID), sqlString(r.RunID), sqlString(r.CaseID), sqlString(r.Status), sqlString(r.RequestSummaryJSON),
		sqlString(r.AssertionSummaryJSON), sqlString(encodeTime(r.StartedAt)), sqlString(encodeTime(r.FinishedAt)),
		sqlString(encodeTime(r.CreatedAt)))); err != nil {
		return store.APICaseRun{}, fmt.Errorf("record api case run %q: %w", r.ID, err)
	}
	return r, nil
}

func (s *Store) ListAPICaseRuns(ctx context.Context, runID string) ([]store.APICaseRun, error) {
	var rows []apiCaseRunRow
	if err := s.query(ctx, fmt.Sprintf(`
select id, run_id, case_id, status, request_summary_json, assertion_summary_json, started_at, finished_at, created_at
from api_case_runs where run_id = %s order by created_at, id;`, sqlString(runID)), &rows); err != nil {
		return nil, err
	}
	out := make([]store.APICaseRun, 0, len(rows))
	for _, row := range rows {
		out = append(out, row.toStore())
	}
	return out, nil
}

func (s *Store) ListLatestAPICaseRuns(ctx context.Context) ([]store.APICaseRun, error) {
	var rows []apiCaseRunRow
	if err := s.query(ctx, `
select id, run_id, case_id, status, request_summary_json, assertion_summary_json, started_at, finished_at, created_at
from (
  select id, run_id, case_id, status, request_summary_json, assertion_summary_json, started_at, finished_at, created_at,
    row_number() over (partition by case_id order by created_at desc, id desc) as row_number
  from api_case_runs
  where case_id <> ''
)
where row_number = 1
order by created_at, id;`, &rows); err != nil {
		return nil, err
	}
	out := make([]store.APICaseRun, 0, len(rows))
	for _, row := range rows {
		out = append(out, row.toStore())
	}
	return out, nil
}

func (s *Store) ListAPICaseRunRecordsForCaseIDs(ctx context.Context, caseIDs []string) ([]store.APICaseRunRecord, error) {
	if len(caseIDs) == 0 {
		return []store.APICaseRunRecord{}, nil
	}
	values := make([]string, 0, len(caseIDs))
	for _, id := range caseIDs {
		if strings.TrimSpace(id) == "" {
			continue
		}
		values = append(values, sqlString(id))
	}
	if len(values) == 0 {
		return []store.APICaseRunRecord{}, nil
	}
	var rows []apiCaseRunRecordRow
	if err := s.query(ctx, fmt.Sprintf(`
select
  r.id as run_id,
  r.profile_id as run_profile_id,
  r.workflow_id as run_workflow_id,
  r.status as run_status,
  r.evidence_root as run_evidence_root,
  case
    when json_valid(r.summary_json) then json_object(
      'kind', json_extract(r.summary_json, '$.kind'),
      'summary', json_object(
        'caseId', json_extract(r.summary_json, '$.summary.caseId'),
        'expectedStepCount', json_extract(r.summary_json, '$.summary.expectedStepCount'),
        'stepCount', coalesce(
          json_extract(r.summary_json, '$.summary.stepCount'),
          json_extract(r.summary_json, '$.stepCount'),
          json_array_length(r.summary_json, '$.steps'),
          0
        )
      )
    )
    else '{}'
  end as run_summary_json,
  r.started_at as run_started_at,
  r.finished_at as run_finished_at,
  r.created_at as run_created_at,
  r.updated_at as run_updated_at,
  acr.id as case_run_id,
  acr.run_id as case_run_run_id,
  acr.case_id as case_run_case_id,
  acr.status as case_run_status,
  acr.request_summary_json as case_run_request_summary_json,
  acr.assertion_summary_json as case_run_assertion_summary_json,
  acr.started_at as case_run_started_at,
  acr.finished_at as case_run_finished_at,
  acr.created_at as case_run_created_at
from api_case_runs acr
join runs r on r.id = acr.run_id
where acr.case_id in (%s)
order by acr.created_at desc, acr.id desc;`, strings.Join(values, ",")), &rows); err != nil {
		return nil, err
	}
	out := make([]store.APICaseRunRecord, 0, len(rows))
	for _, row := range rows {
		out = append(out, row.toStore())
	}
	return out, nil
}

type apiCaseRunRow struct {
	ID                   string `json:"id"`
	RunID                string `json:"run_id"`
	CaseID               string `json:"case_id"`
	Status               string `json:"status"`
	RequestSummaryJSON   string `json:"request_summary_json"`
	AssertionSummaryJSON string `json:"assertion_summary_json"`
	StartedAt            string `json:"started_at"`
	FinishedAt           string `json:"finished_at"`
	CreatedAt            string `json:"created_at"`
}

type apiCaseRunRecordRow struct {
	RunID           string `json:"run_id"`
	RunProfileID    string `json:"run_profile_id"`
	RunWorkflowID   string `json:"run_workflow_id"`
	RunStatus       string `json:"run_status"`
	RunEvidenceRoot string `json:"run_evidence_root"`
	RunSummaryJSON  string `json:"run_summary_json"`
	RunStartedAt    string `json:"run_started_at"`
	RunFinishedAt   string `json:"run_finished_at"`
	RunCreatedAt    string `json:"run_created_at"`
	RunUpdatedAt    string `json:"run_updated_at"`

	CaseRunID                   string `json:"case_run_id"`
	CaseRunRunID                string `json:"case_run_run_id"`
	CaseRunCaseID               string `json:"case_run_case_id"`
	CaseRunStatus               string `json:"case_run_status"`
	CaseRunRequestSummaryJSON   string `json:"case_run_request_summary_json"`
	CaseRunAssertionSummaryJSON string `json:"case_run_assertion_summary_json"`
	CaseRunStartedAt            string `json:"case_run_started_at"`
	CaseRunFinishedAt           string `json:"case_run_finished_at"`
	CaseRunCreatedAt            string `json:"case_run_created_at"`
}

func (r apiCaseRunRecordRow) toStore() store.APICaseRunRecord {
	return store.APICaseRunRecord{
		Run: store.Run{
			ID:           r.RunID,
			ProfileID:    r.RunProfileID,
			WorkflowID:   r.RunWorkflowID,
			Status:       r.RunStatus,
			EvidenceRoot: r.RunEvidenceRoot,
			SummaryJSON:  r.RunSummaryJSON,
			StartedAt:    decodeTime(r.RunStartedAt),
			FinishedAt:   decodeTime(r.RunFinishedAt),
			CreatedAt:    decodeTime(r.RunCreatedAt),
			UpdatedAt:    decodeTime(r.RunUpdatedAt),
		},
		CaseRun: store.APICaseRun{
			ID:                   r.CaseRunID,
			RunID:                r.CaseRunRunID,
			CaseID:               r.CaseRunCaseID,
			Status:               r.CaseRunStatus,
			RequestSummaryJSON:   r.CaseRunRequestSummaryJSON,
			AssertionSummaryJSON: r.CaseRunAssertionSummaryJSON,
			StartedAt:            decodeTime(r.CaseRunStartedAt),
			FinishedAt:           decodeTime(r.CaseRunFinishedAt),
			CreatedAt:            decodeTime(r.CaseRunCreatedAt),
		},
	}
}

func (r apiCaseRunRow) toStore() store.APICaseRun {
	return store.APICaseRun{
		ID:                   r.ID,
		RunID:                r.RunID,
		CaseID:               r.CaseID,
		Status:               r.Status,
		RequestSummaryJSON:   r.RequestSummaryJSON,
		AssertionSummaryJSON: r.AssertionSummaryJSON,
		StartedAt:            decodeTime(r.StartedAt),
		FinishedAt:           decodeTime(r.FinishedAt),
		CreatedAt:            decodeTime(r.CreatedAt),
	}
}
