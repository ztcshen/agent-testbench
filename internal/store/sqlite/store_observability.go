package sqlite

import (
	"context"
	"fmt"
	"strings"

	"agent-testbench/internal/store"
)

func (s *Store) RecordEvidence(ctx context.Context, r store.EvidenceRecord) (store.EvidenceRecord, error) {
	if r.CreatedAt.IsZero() {
		r.CreatedAt = utcNow()
	}
	if strings.TrimSpace(r.LabelsJSON) == "" {
		r.LabelsJSON = "{}"
	}
	if err := s.exec(ctx, fmt.Sprintf(`
insert into evidence_records (id, run_id, case_run_id, step_id, kind, uri, media_type, sha256, size_bytes, summary, category, visibility, labels_json, created_at)
values (%s, %s, %s, %s, %s, %s, %s, %s, %d, %s, %s, %s, %s, %s);`,
		sqlString(r.ID), sqlString(r.RunID), sqlString(r.CaseRunID), sqlString(r.StepID), sqlString(r.Kind), sqlString(r.URI),
		sqlString(r.MediaType), sqlString(r.SHA256), r.SizeBytes, sqlString(r.Summary), sqlString(r.Category),
		sqlString(r.Visibility), sqlString(r.LabelsJSON), sqlString(encodeTime(r.CreatedAt)))); err != nil {
		return store.EvidenceRecord{}, fmt.Errorf("record evidence %q: %w", r.ID, err)
	}
	return r, nil
}

func (s *Store) ListEvidence(ctx context.Context, runID string) ([]store.EvidenceRecord, error) {
	var rows []evidenceRecordRow
	if err := s.query(ctx, fmt.Sprintf(`
select id, run_id, case_run_id, step_id, kind, uri, media_type, sha256, size_bytes, summary, category, visibility, labels_json, created_at
from evidence_records where run_id = %s order by created_at, id;`, sqlString(runID)), &rows); err != nil {
		return nil, err
	}
	out := make([]store.EvidenceRecord, 0, len(rows))
	for _, row := range rows {
		out = append(out, row.toStore())
	}
	return out, nil
}

func (s *Store) SaveTraceTopology(ctx context.Context, t store.TraceTopology) (store.TraceTopology, error) {
	if t.CreatedAt.IsZero() {
		t.CreatedAt = utcNow()
	}
	if strings.TrimSpace(t.ID) == "" {
		t.ID = "trace-topology." + strings.ReplaceAll(t.CreatedAt.Format("20060102T150405.000000000Z"), ":", "")
	}
	if strings.TrimSpace(t.Status) == "" {
		t.Status = "unknown"
	}
	if strings.TrimSpace(t.TopologyJSON) == "" {
		t.TopologyJSON = "{}"
	}
	if err := s.exec(ctx, fmt.Sprintf(`
insert into trace_topologies (id, workflow_run_id, workflow_id, step_id, case_id, request_id, trace_id, status, topology_json, text_topology, created_at)
values (%s, %s, %s, %s, %s, %s, %s, %s, %s, %s, %s)
on conflict(id) do update set
  workflow_run_id = excluded.workflow_run_id,
  workflow_id = excluded.workflow_id,
  step_id = excluded.step_id,
  case_id = excluded.case_id,
  request_id = excluded.request_id,
  trace_id = excluded.trace_id,
  status = excluded.status,
  topology_json = excluded.topology_json,
  text_topology = excluded.text_topology,
  created_at = excluded.created_at;`,
		sqlString(t.ID), sqlString(t.WorkflowRunID), sqlString(t.WorkflowID), sqlString(t.StepID), sqlString(t.CaseID),
		sqlString(t.RequestID), sqlString(t.TraceID), sqlString(t.Status), sqlString(t.TopologyJSON), sqlString(t.TextTopology),
		sqlString(encodeTime(t.CreatedAt)))); err != nil {
		return store.TraceTopology{}, fmt.Errorf("save trace topology %q: %w", t.ID, err)
	}
	return t, nil
}

func (s *Store) ListTraceTopologies(ctx context.Context, workflowRunID string) ([]store.TraceTopology, error) {
	var rows []traceTopologyRow
	if err := s.query(ctx, fmt.Sprintf(`
select id, workflow_run_id, workflow_id, step_id, case_id, request_id, trace_id, status, topology_json, text_topology, created_at
from trace_topologies where workflow_run_id = %s order by created_at, id;`, sqlString(workflowRunID)), &rows); err != nil {
		return nil, err
	}
	out := make([]store.TraceTopology, 0, len(rows))
	for _, row := range rows {
		out = append(out, row.toStore())
	}
	return out, nil
}

func (s *Store) RecordPostProcessTask(ctx context.Context, t store.PostProcessTask) (store.PostProcessTask, error) {
	if t.CreatedAt.IsZero() {
		t.CreatedAt = utcNow()
	}
	if t.StartedAt.IsZero() {
		t.StartedAt = t.CreatedAt
	}
	if t.FinishedAt.IsZero() && t.Status != store.StatusRunning {
		t.FinishedAt = t.StartedAt
	}
	if t.DurationMs == 0 && !t.StartedAt.IsZero() && !t.FinishedAt.IsZero() {
		t.DurationMs = t.FinishedAt.Sub(t.StartedAt).Milliseconds()
		if t.DurationMs < 0 {
			t.DurationMs = 0
		}
	}
	if strings.TrimSpace(t.ID) == "" {
		t.ID = "post-process." + strings.ReplaceAll(t.CreatedAt.Format("20060102T150405.000000000Z"), ":", "")
	}
	if strings.TrimSpace(t.Status) == "" {
		t.Status = store.StatusPassed
	}
	if strings.TrimSpace(t.SummaryJSON) == "" {
		t.SummaryJSON = "{}"
	}
	if err := s.exec(ctx, fmt.Sprintf(`
insert into post_process_tasks (id, run_id, workflow_id, step_id, case_id, kind, status, started_at, finished_at, duration_ms, error, summary_json, created_at)
values (%s, %s, %s, %s, %s, %s, %s, %s, %s, %d, %s, %s, %s)
on conflict(id) do update set
  run_id = excluded.run_id,
  workflow_id = excluded.workflow_id,
  step_id = excluded.step_id,
  case_id = excluded.case_id,
  kind = excluded.kind,
  status = excluded.status,
  started_at = excluded.started_at,
  finished_at = excluded.finished_at,
  duration_ms = excluded.duration_ms,
  error = excluded.error,
  summary_json = excluded.summary_json,
  created_at = excluded.created_at;`,
		sqlString(t.ID), sqlString(t.RunID), sqlString(t.WorkflowID), sqlString(t.StepID), sqlString(t.CaseID),
		sqlString(t.Kind), sqlString(t.Status), sqlString(encodeTime(t.StartedAt)), sqlString(encodeTime(t.FinishedAt)),
		t.DurationMs, sqlString(t.Error), sqlString(t.SummaryJSON), sqlString(encodeTime(t.CreatedAt)))); err != nil {
		return store.PostProcessTask{}, fmt.Errorf("record post process task %q: %w", t.ID, err)
	}
	return t, nil
}

func (s *Store) ListPostProcessTasks(ctx context.Context, runID string) ([]store.PostProcessTask, error) {
	var rows []postProcessTaskRow
	if err := s.query(ctx, fmt.Sprintf(`
select id, run_id, workflow_id, step_id, case_id, kind, status, started_at, finished_at, duration_ms, error, summary_json, created_at
from post_process_tasks where run_id = %s order by created_at, id;`, sqlString(runID)), &rows); err != nil {
		return nil, err
	}
	out := make([]store.PostProcessTask, 0, len(rows))
	for _, row := range rows {
		out = append(out, row.toStore())
	}
	return out, nil
}

func (s *Store) UpsertBaselineGate(ctx context.Context, g store.BaselineGate) (store.BaselineGate, error) {
	if g.UpdatedAt.IsZero() {
		g.UpdatedAt = utcNow()
	}
	if err := s.exec(ctx, fmt.Sprintf(`
insert into baseline_gates (profile_id, subject_id, status, required, summary_json, checked_at, updated_at)
values (%s, %s, %s, %d, %s, %s, %s)
on conflict(profile_id, subject_id) do update set
  status = excluded.status,
  required = excluded.required,
  summary_json = excluded.summary_json,
  checked_at = excluded.checked_at,
  updated_at = excluded.updated_at;`,
		sqlString(g.ProfileID), sqlString(g.SubjectID), sqlString(g.Status), boolInt(g.Required), sqlString(g.SummaryJSON),
		sqlString(encodeTime(g.CheckedAt)), sqlString(encodeTime(g.UpdatedAt)))); err != nil {
		return store.BaselineGate{}, fmt.Errorf("upsert baseline gate %q/%q: %w", g.ProfileID, g.SubjectID, err)
	}
	return g, nil
}

func (s *Store) GetBaselineGate(ctx context.Context, profileID, subjectID string) (store.BaselineGate, error) {
	var rows []baselineGateRow
	if err := s.query(ctx, fmt.Sprintf(`
select profile_id, subject_id, status, required, summary_json, checked_at, updated_at
from baseline_gates where profile_id = %s and subject_id = %s;`, sqlString(profileID), sqlString(subjectID)), &rows); err != nil {
		return store.BaselineGate{}, err
	}
	if len(rows) == 0 {
		return store.BaselineGate{}, store.ErrNotFound
	}
	return rows[0].toStore(), nil
}

type evidenceRecordRow struct {
	ID         string `json:"id"`
	RunID      string `json:"run_id"`
	CaseRunID  string `json:"case_run_id"`
	StepID     string `json:"step_id"`
	Kind       string `json:"kind"`
	URI        string `json:"uri"`
	MediaType  string `json:"media_type"`
	SHA256     string `json:"sha256"`
	SizeBytes  int64  `json:"size_bytes"`
	Summary    string `json:"summary"`
	Category   string `json:"category"`
	Visibility string `json:"visibility"`
	LabelsJSON string `json:"labels_json"`
	CreatedAt  string `json:"created_at"`
}

func (r evidenceRecordRow) toStore() store.EvidenceRecord {
	return store.EvidenceRecord{
		ID:         r.ID,
		RunID:      r.RunID,
		CaseRunID:  r.CaseRunID,
		StepID:     r.StepID,
		Kind:       r.Kind,
		URI:        r.URI,
		MediaType:  r.MediaType,
		SHA256:     r.SHA256,
		SizeBytes:  r.SizeBytes,
		Summary:    r.Summary,
		Category:   r.Category,
		Visibility: r.Visibility,
		LabelsJSON: r.LabelsJSON,
		CreatedAt:  decodeTime(r.CreatedAt),
	}
}

type traceTopologyRow struct {
	ID            string `json:"id"`
	WorkflowRunID string `json:"workflow_run_id"`
	WorkflowID    string `json:"workflow_id"`
	StepID        string `json:"step_id"`
	CaseID        string `json:"case_id"`
	RequestID     string `json:"request_id"`
	TraceID       string `json:"trace_id"`
	Status        string `json:"status"`
	TopologyJSON  string `json:"topology_json"`
	TextTopology  string `json:"text_topology"`
	CreatedAt     string `json:"created_at"`
}

func (r traceTopologyRow) toStore() store.TraceTopology {
	return store.TraceTopology{
		ID:            r.ID,
		WorkflowRunID: r.WorkflowRunID,
		WorkflowID:    r.WorkflowID,
		StepID:        r.StepID,
		CaseID:        r.CaseID,
		RequestID:     r.RequestID,
		TraceID:       r.TraceID,
		Status:        r.Status,
		TopologyJSON:  r.TopologyJSON,
		TextTopology:  r.TextTopology,
		CreatedAt:     decodeTime(r.CreatedAt),
	}
}

type postProcessTaskRow struct {
	ID          string `json:"id"`
	RunID       string `json:"run_id"`
	WorkflowID  string `json:"workflow_id"`
	StepID      string `json:"step_id"`
	CaseID      string `json:"case_id"`
	Kind        string `json:"kind"`
	Status      string `json:"status"`
	StartedAt   string `json:"started_at"`
	FinishedAt  string `json:"finished_at"`
	DurationMs  int64  `json:"duration_ms"`
	Error       string `json:"error"`
	SummaryJSON string `json:"summary_json"`
	CreatedAt   string `json:"created_at"`
}

func (r postProcessTaskRow) toStore() store.PostProcessTask {
	return store.PostProcessTask{
		ID:          r.ID,
		RunID:       r.RunID,
		WorkflowID:  r.WorkflowID,
		StepID:      r.StepID,
		CaseID:      r.CaseID,
		Kind:        r.Kind,
		Status:      r.Status,
		StartedAt:   decodeTime(r.StartedAt),
		FinishedAt:  decodeTime(r.FinishedAt),
		DurationMs:  r.DurationMs,
		Error:       r.Error,
		SummaryJSON: r.SummaryJSON,
		CreatedAt:   decodeTime(r.CreatedAt),
	}
}

type baselineGateRow struct {
	ProfileID   string `json:"profile_id"`
	SubjectID   string `json:"subject_id"`
	Status      string `json:"status"`
	Required    int    `json:"required"`
	SummaryJSON string `json:"summary_json"`
	CheckedAt   string `json:"checked_at"`
	UpdatedAt   string `json:"updated_at"`
}

func (r baselineGateRow) toStore() store.BaselineGate {
	return store.BaselineGate{
		ProfileID:   r.ProfileID,
		SubjectID:   r.SubjectID,
		Status:      r.Status,
		Required:    r.Required != 0,
		SummaryJSON: r.SummaryJSON,
		CheckedAt:   decodeTime(r.CheckedAt),
		UpdatedAt:   decodeTime(r.UpdatedAt),
	}
}
