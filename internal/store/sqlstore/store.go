package sqlstore

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"open-test-sandbox/internal/store"
)

type Store struct {
	db      *sql.DB
	dialect Dialect
}

func New(db *sql.DB, dialect Dialect) *Store {
	return &Store{db: db, dialect: dialect}
}

func (s *Store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

func (s *Store) CreateRun(ctx context.Context, r store.Run) (store.Run, error) {
	now := utcNow()
	if r.CreatedAt.IsZero() {
		r.CreatedAt = now
	}
	if r.UpdatedAt.IsZero() {
		r.UpdatedAt = r.CreatedAt
	}
	query := fmt.Sprintf(`
insert into runs (id, profile_id, workflow_id, status, evidence_root, summary_json, started_at, finished_at, created_at, updated_at)
values (%s);`, s.bindVars(10))
	if _, err := s.db.ExecContext(ctx, query,
		r.ID, r.ProfileID, r.WorkflowID, r.Status, r.EvidenceRoot, r.SummaryJSON,
		dbTimeArg(s.dialect, r.StartedAt), dbTimeArg(s.dialect, r.FinishedAt), dbTimeArg(s.dialect, r.CreatedAt), dbTimeArg(s.dialect, r.UpdatedAt)); err != nil {
		return store.Run{}, fmt.Errorf("create run %q: %w", r.ID, err)
	}
	return r, nil
}

func (s *Store) GetRun(ctx context.Context, id string) (store.Run, error) {
	query := fmt.Sprintf(`
select id, profile_id, workflow_id, status, evidence_root, summary_json, started_at, finished_at, created_at, updated_at
from runs where id = %s;`, s.dialect.BindVar(1))
	r, err := scanRun(s.db.QueryRowContext(ctx, query, id))
	if err != nil {
		return store.Run{}, err
	}
	return r, nil
}

func (s *Store) ListRuns(ctx context.Context) ([]store.Run, error) {
	rows, err := s.db.QueryContext(ctx, `
select id, profile_id, workflow_id, status, evidence_root, summary_json, started_at, finished_at, created_at, updated_at
from runs order by created_at, id;`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []store.Run
	for rows.Next() {
		r, err := scanRun(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (s *Store) RecordAPICaseRun(ctx context.Context, r store.APICaseRun) (store.APICaseRun, error) {
	if r.CreatedAt.IsZero() {
		r.CreatedAt = utcNow()
	}
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
	rows, err := s.db.QueryContext(ctx, query, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []store.APICaseRun
	for rows.Next() {
		r, err := scanAPICaseRun(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (s *Store) ListLatestAPICaseRuns(ctx context.Context) ([]store.APICaseRun, error) {
	rows, err := s.db.QueryContext(ctx, `
select id, run_id, case_id, status, request_summary_json, assertion_summary_json, started_at, finished_at, created_at
from (
  select id, run_id, case_id, status, request_summary_json, assertion_summary_json, started_at, finished_at, created_at,
    row_number() over (partition by case_id order by created_at desc, id desc) as row_number
  from api_case_runs
  where case_id <> ''
) latest
where row_number = 1
order by created_at, id;`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []store.APICaseRun
	for rows.Next() {
		r, err := scanAPICaseRun(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (s *Store) RecordEvidence(ctx context.Context, r store.EvidenceRecord) (store.EvidenceRecord, error) {
	if r.CreatedAt.IsZero() {
		r.CreatedAt = utcNow()
	}
	query := fmt.Sprintf(`
insert into evidence_records (id, run_id, case_run_id, step_id, kind, uri, media_type, sha256, size_bytes, summary, category, visibility, labels_json, created_at)
values (%s);`, s.bindVars(14))
	if _, err := s.db.ExecContext(ctx, query,
		r.ID, r.RunID, r.CaseRunID, r.StepID, r.Kind, r.URI, r.MediaType, r.SHA256, r.SizeBytes, r.Summary,
		r.Category, r.Visibility, r.LabelsJSON, dbTimeArg(s.dialect, r.CreatedAt)); err != nil {
		return store.EvidenceRecord{}, fmt.Errorf("record evidence %q: %w", r.ID, err)
	}
	return r, nil
}

func (s *Store) ListEvidence(ctx context.Context, runID string) ([]store.EvidenceRecord, error) {
	query := fmt.Sprintf(`
select id, run_id, case_run_id, step_id, kind, uri, media_type, sha256, size_bytes, summary, category, visibility, labels_json, created_at
from evidence_records where run_id = %s order by created_at, id;`, s.dialect.BindVar(1))
	rows, err := s.db.QueryContext(ctx, query, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []store.EvidenceRecord
	for rows.Next() {
		r, err := scanEvidenceRecord(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (s *Store) SaveTraceTopology(ctx context.Context, r store.TraceTopology) (store.TraceTopology, error) {
	if r.CreatedAt.IsZero() {
		r.CreatedAt = utcNow()
	}
	query := fmt.Sprintf(`
insert into trace_topologies (id, workflow_run_id, workflow_id, step_id, case_id, request_id, trace_id, status, topology_json, text_topology, created_at)
values (%s)
%s;`, s.bindVars(11), s.dialect.UpsertClause("id", []string{
		"workflow_run_id", "workflow_id", "step_id", "case_id", "request_id", "trace_id", "status", "topology_json", "text_topology", "created_at",
	}))
	if _, err := s.db.ExecContext(ctx, query,
		r.ID, r.WorkflowRunID, r.WorkflowID, r.StepID, r.CaseID, r.RequestID, r.TraceID, r.Status,
		r.TopologyJSON, r.TextTopology, dbTimeArg(s.dialect, r.CreatedAt)); err != nil {
		return store.TraceTopology{}, fmt.Errorf("save trace topology %q: %w", r.ID, err)
	}
	return r, nil
}

func (s *Store) ListTraceTopologies(ctx context.Context, workflowRunID string) ([]store.TraceTopology, error) {
	query := fmt.Sprintf(`
select id, workflow_run_id, workflow_id, step_id, case_id, request_id, trace_id, status, topology_json, text_topology, created_at
from trace_topologies where workflow_run_id = %s order by created_at, id;`, s.dialect.BindVar(1))
	rows, err := s.db.QueryContext(ctx, query, workflowRunID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []store.TraceTopology
	for rows.Next() {
		r, err := scanTraceTopology(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (s *Store) RecordPostProcessTask(ctx context.Context, r store.PostProcessTask) (store.PostProcessTask, error) {
	if r.CreatedAt.IsZero() {
		r.CreatedAt = utcNow()
	}
	if r.StartedAt.IsZero() {
		r.StartedAt = r.CreatedAt
	}
	if r.FinishedAt.IsZero() && r.Status != store.StatusRunning {
		r.FinishedAt = r.StartedAt
	}
	if r.DurationMs == 0 && !r.StartedAt.IsZero() && !r.FinishedAt.IsZero() {
		r.DurationMs = r.FinishedAt.Sub(r.StartedAt).Milliseconds()
	}
	query := fmt.Sprintf(`
insert into post_process_tasks (id, run_id, workflow_id, step_id, case_id, kind, status, started_at, finished_at, duration_ms, error, summary_json, created_at)
values (%s)
%s;`, s.bindVars(13), s.dialect.UpsertClause("id", []string{
		"run_id", "workflow_id", "step_id", "case_id", "kind", "status", "started_at", "finished_at", "duration_ms", "error", "summary_json", "created_at",
	}))
	if _, err := s.db.ExecContext(ctx, query,
		r.ID, r.RunID, r.WorkflowID, r.StepID, r.CaseID, r.Kind, r.Status, dbTimeArg(s.dialect, r.StartedAt),
		dbTimeArg(s.dialect, r.FinishedAt), r.DurationMs, r.Error, r.SummaryJSON, dbTimeArg(s.dialect, r.CreatedAt)); err != nil {
		return store.PostProcessTask{}, fmt.Errorf("record post-process task %q: %w", r.ID, err)
	}
	return r, nil
}

func (s *Store) ListPostProcessTasks(ctx context.Context, runID string) ([]store.PostProcessTask, error) {
	query := fmt.Sprintf(`
select id, run_id, workflow_id, step_id, case_id, kind, status, started_at, finished_at, duration_ms, error, summary_json, created_at
from post_process_tasks where run_id = %s order by created_at, id;`, s.dialect.BindVar(1))
	rows, err := s.db.QueryContext(ctx, query, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []store.PostProcessTask
	for rows.Next() {
		r, err := scanPostProcessTask(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (s *Store) UpsertBaselineGate(ctx context.Context, r store.BaselineGate) (store.BaselineGate, error) {
	if r.UpdatedAt.IsZero() {
		r.UpdatedAt = utcNow()
	}
	query := fmt.Sprintf(`
insert into baseline_gates (profile_id, subject_id, status, required, summary_json, checked_at, updated_at)
values (%s)
%s;`, s.bindVars(7), s.dialect.UpsertClause("profile_id, subject_id", []string{"status", "required", "summary_json", "checked_at", "updated_at"}))
	if _, err := s.db.ExecContext(ctx, query,
		r.ProfileID, r.SubjectID, r.Status, r.Required, r.SummaryJSON, dbTimeArg(s.dialect, r.CheckedAt), dbTimeArg(s.dialect, r.UpdatedAt)); err != nil {
		return store.BaselineGate{}, fmt.Errorf("upsert baseline gate %q/%q: %w", r.ProfileID, r.SubjectID, err)
	}
	return r, nil
}

func (s *Store) GetBaselineGate(ctx context.Context, profileID string, subjectID string) (store.BaselineGate, error) {
	query := fmt.Sprintf(`
select profile_id, subject_id, status, required, summary_json, checked_at, updated_at
from baseline_gates where profile_id = %s and subject_id = %s;`, s.dialect.BindVar(1), s.dialect.BindVar(2))
	r, err := scanBaselineGate(s.db.QueryRowContext(ctx, query, profileID, subjectID))
	if err != nil {
		return store.BaselineGate{}, err
	}
	return r, nil
}

func (s *Store) UpsertProfileIndex(ctx context.Context, r store.ProfileIndex) (store.ProfileIndex, error) {
	if r.UpdatedAt.IsZero() {
		r.UpdatedAt = utcNow()
	}
	query := fmt.Sprintf(`
insert into profile_indexes (profile_id, bundle_path, bundle_digest, summary_json, imported_at, updated_at)
values (%s)
%s;`, s.bindVars(6), s.dialect.UpsertClause("profile_id", []string{"bundle_path", "bundle_digest", "summary_json", "imported_at", "updated_at"}))
	if _, err := s.db.ExecContext(ctx, query,
		r.ProfileID, r.BundlePath, r.BundleDigest, r.SummaryJSON, dbTimeArg(s.dialect, r.ImportedAt), dbTimeArg(s.dialect, r.UpdatedAt)); err != nil {
		return store.ProfileIndex{}, fmt.Errorf("upsert profile index %q: %w", r.ProfileID, err)
	}
	return r, nil
}

func (s *Store) GetProfileIndex(ctx context.Context, profileID string) (store.ProfileIndex, error) {
	query := fmt.Sprintf(`
select profile_id, bundle_path, bundle_digest, summary_json, imported_at, updated_at
from profile_indexes where profile_id = %s;`, s.dialect.BindVar(1))
	r, err := scanProfileIndex(s.db.QueryRowContext(ctx, query, profileID))
	if err != nil {
		return store.ProfileIndex{}, err
	}
	return r, nil
}

func (s *Store) UpsertConfigVersion(ctx context.Context, r store.ConfigVersion) (store.ConfigVersion, error) {
	if r.CreatedAt.IsZero() {
		r.CreatedAt = utcNow()
	}
	if r.PublishedAt.IsZero() {
		r.PublishedAt = r.CreatedAt
	}
	if r.Active {
		query := fmt.Sprintf(`update config_versions set active = %s;`, s.dialect.BindVar(1))
		if _, err := s.db.ExecContext(ctx, query, false); err != nil {
			return store.ConfigVersion{}, fmt.Errorf("reset active config versions: %w", err)
		}
	}
	query := fmt.Sprintf(`
insert into config_versions (id, profile_id, source_path, bundle_digest, summary_json, active, published_at, created_at)
values (%s)
%s;`, s.bindVars(8), s.dialect.UpsertClause("id", []string{"profile_id", "source_path", "bundle_digest", "summary_json", "active", "published_at"}))
	if _, err := s.db.ExecContext(ctx, query,
		r.ID, r.ProfileID, r.SourcePath, r.BundleDigest, r.SummaryJSON, r.Active, dbTimeArg(s.dialect, r.PublishedAt), dbTimeArg(s.dialect, r.CreatedAt)); err != nil {
		return store.ConfigVersion{}, fmt.Errorf("upsert config version %q: %w", r.ID, err)
	}
	return r, nil
}

func (s *Store) GetActiveConfigVersion(ctx context.Context) (store.ConfigVersion, error) {
	query := fmt.Sprintf(`
select id, profile_id, source_path, bundle_digest, summary_json, active, published_at, created_at
from config_versions
where active = %s
order by published_at desc, id desc
limit 1;`, s.dialect.BindVar(1))
	r, err := scanConfigVersion(s.db.QueryRowContext(ctx, query, true))
	if err != nil {
		return store.ConfigVersion{}, err
	}
	return r, nil
}

func (s *Store) UpsertReadModel(ctx context.Context, r store.ReadModel) (store.ReadModel, error) {
	if r.UpdatedAt.IsZero() {
		r.UpdatedAt = utcNow()
	}
	if r.GeneratedAt.IsZero() {
		r.GeneratedAt = r.UpdatedAt
	}
	query := fmt.Sprintf(`
insert into config_read_model (profile_id, model_key, config_version_id, payload_json, generated_at, updated_at)
values (%s)
%s;`, s.bindVars(6), s.dialect.UpsertClause("profile_id, model_key", []string{"config_version_id", "payload_json", "generated_at", "updated_at"}))
	if _, err := s.db.ExecContext(ctx, query,
		r.ProfileID, r.Key, r.ConfigVersionID, r.PayloadJSON, dbTimeArg(s.dialect, r.GeneratedAt), dbTimeArg(s.dialect, r.UpdatedAt)); err != nil {
		return store.ReadModel{}, fmt.Errorf("upsert read model %q/%q: %w", r.ProfileID, r.Key, err)
	}
	return r, nil
}

func (s *Store) GetReadModel(ctx context.Context, profileID string, key string) (store.ReadModel, error) {
	query := fmt.Sprintf(`
select profile_id, model_key, config_version_id, payload_json, generated_at, updated_at
from config_read_model
where profile_id = %s and model_key = %s;`, s.dialect.BindVar(1), s.dialect.BindVar(2))
	r, err := scanReadModel(s.db.QueryRowContext(ctx, query, profileID, key))
	if err != nil {
		return store.ReadModel{}, err
	}
	return r, nil
}

func (s *Store) ReplaceProfileCatalog(ctx context.Context, catalog store.ProfileCatalog) error {
	if catalog.IndexedAt.IsZero() {
		catalog.IndexedAt = utcNow()
	}
	counts := catalogCounts(catalog)
	payload, err := json.Marshal(catalog)
	if err != nil {
		return fmt.Errorf("encode profile catalog %q: %w", catalog.ProfileID, err)
	}
	query := fmt.Sprintf(`
insert into profile_catalogs (
  profile_id, indexed_at, catalog_json, services, workflows, interface_nodes, api_cases,
  request_templates, workflow_bindings, case_dependencies, fixtures, templates, template_configs
)
values (%s)
%s;`, s.bindVars(13), s.dialect.UpsertClause("profile_id", []string{
		"indexed_at", "catalog_json", "services", "workflows", "interface_nodes", "api_cases",
		"request_templates", "workflow_bindings", "case_dependencies", "fixtures", "templates", "template_configs",
	}))
	if _, err := s.db.ExecContext(ctx, query,
		catalog.ProfileID, dbTimeArg(s.dialect, catalog.IndexedAt), string(payload), counts.Services, counts.Workflows, counts.InterfaceNodes,
		counts.APICases, counts.RequestTemplates, counts.WorkflowBindings, counts.CaseDependencies, counts.Fixtures, counts.Templates, counts.TemplateConfigs,
	); err != nil {
		return fmt.Errorf("replace profile catalog %q: %w", catalog.ProfileID, err)
	}
	return nil
}

func (s *Store) GetProfileCatalog(ctx context.Context) (store.ProfileCatalog, error) {
	var payload string
	err := s.db.QueryRowContext(ctx, `
select catalog_json
from profile_catalogs
order by indexed_at desc, profile_id desc
limit 1;`).Scan(&payload)
	if err != nil {
		if err == sql.ErrNoRows {
			return store.ProfileCatalog{}, store.ErrNotFound
		}
		return store.ProfileCatalog{}, err
	}
	var catalog store.ProfileCatalog
	if err := json.Unmarshal([]byte(payload), &catalog); err != nil {
		return store.ProfileCatalog{}, fmt.Errorf("decode profile catalog: %w", err)
	}
	return catalog, nil
}

func (s *Store) GetProfileCatalogIndex(ctx context.Context) (store.ProfileCatalogIndex, error) {
	row := s.db.QueryRowContext(ctx, `
select profile_id, indexed_at, services, workflows, interface_nodes, api_cases, request_templates,
  workflow_bindings, case_dependencies, fixtures, templates, template_configs
from profile_catalogs
order by indexed_at desc, profile_id desc
limit 1;`)
	index, err := scanProfileCatalogIndex(row)
	if err != nil {
		return store.ProfileCatalogIndex{}, err
	}
	return index, nil
}

func (s *Store) UpsertEnvironment(ctx context.Context, e store.Environment) (store.Environment, error) {
	now := utcNow()
	if e.CreatedAt.IsZero() {
		e.CreatedAt = now
	}
	if e.UpdatedAt.IsZero() {
		e.UpdatedAt = now
	}
	query := fmt.Sprintf(`
insert into environments (
  id, display_name, description, status, verified, services_json, repos_json, compose_json,
  health_checks_json, verification_workflow_id, last_verification_run_id, last_verification_status,
  evidence_complete, topology_complete, last_verified_at, summary_json, created_at, updated_at
)
values (%s)
%s;`, s.bindVars(18), s.dialect.UpsertClause("id", []string{
		"display_name", "description", "status", "verified", "services_json", "repos_json", "compose_json",
		"health_checks_json", "verification_workflow_id", "last_verification_run_id", "last_verification_status",
		"evidence_complete", "topology_complete", "last_verified_at", "summary_json", "updated_at",
	}))
	if _, err := s.db.ExecContext(ctx, query,
		e.ID, e.DisplayName, e.Description, e.Status, e.Verified, stringDefault(e.ServicesJSON, "[]"),
		stringDefault(e.ReposJSON, "{}"), stringDefault(e.ComposeJSON, "{}"), stringDefault(e.HealthChecksJSON, "[]"),
		e.VerificationWorkflowID, e.LastVerificationRunID, e.LastVerificationStatus, e.EvidenceComplete, e.TopologyComplete,
		dbTimeArg(s.dialect, e.LastVerifiedAt), stringDefault(e.SummaryJSON, "{}"), dbTimeArg(s.dialect, e.CreatedAt), dbTimeArg(s.dialect, e.UpdatedAt),
	); err != nil {
		return store.Environment{}, fmt.Errorf("upsert environment %q: %w", e.ID, err)
	}
	return e, nil
}

func (s *Store) GetEnvironment(ctx context.Context, id string) (store.Environment, error) {
	query := fmt.Sprintf(`
select id, display_name, description, status, verified, services_json, repos_json, compose_json,
  health_checks_json, verification_workflow_id, last_verification_run_id, last_verification_status,
  evidence_complete, topology_complete, last_verified_at, summary_json, created_at, updated_at
from environments where id = %s;`, s.dialect.BindVar(1))
	return scanEnvironment(s.db.QueryRowContext(ctx, query, id))
}

func (s *Store) ListEnvironments(ctx context.Context) ([]store.Environment, error) {
	rows, err := s.db.QueryContext(ctx, `
select id, display_name, description, status, verified, services_json, repos_json, compose_json,
  health_checks_json, verification_workflow_id, last_verification_run_id, last_verification_status,
  evidence_complete, topology_complete, last_verified_at, summary_json, created_at, updated_at
from environments order by verified desc, updated_at desc, id;`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []store.Environment
	for rows.Next() {
		item, err := scanEnvironment(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (s *Store) bindVars(count int) string {
	vars := make([]string, 0, count)
	for i := 1; i <= count; i++ {
		vars = append(vars, s.dialect.BindVar(i))
	}
	return strings.Join(vars, ", ")
}

type scanner interface {
	Scan(dest ...any) error
}

func scanRun(row scanner) (store.Run, error) {
	var r store.Run
	var startedAt, finishedAt, createdAt, updatedAt any
	if err := row.Scan(
		&r.ID, &r.ProfileID, &r.WorkflowID, &r.Status, &r.EvidenceRoot, &r.SummaryJSON,
		&startedAt, &finishedAt, &createdAt, &updatedAt,
	); err != nil {
		if err == sql.ErrNoRows {
			return store.Run{}, store.ErrNotFound
		}
		return store.Run{}, err
	}
	r.SummaryJSON = normalizeJSONText(r.SummaryJSON)
	r.StartedAt = decodeDBTime(startedAt)
	r.FinishedAt = decodeDBTime(finishedAt)
	r.CreatedAt = decodeDBTime(createdAt)
	r.UpdatedAt = decodeDBTime(updatedAt)
	return r, nil
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

func scanEvidenceRecord(row scanner) (store.EvidenceRecord, error) {
	var r store.EvidenceRecord
	var createdAt any
	if err := row.Scan(
		&r.ID, &r.RunID, &r.CaseRunID, &r.StepID, &r.Kind, &r.URI, &r.MediaType, &r.SHA256, &r.SizeBytes,
		&r.Summary, &r.Category, &r.Visibility, &r.LabelsJSON, &createdAt,
	); err != nil {
		if err == sql.ErrNoRows {
			return store.EvidenceRecord{}, store.ErrNotFound
		}
		return store.EvidenceRecord{}, err
	}
	r.LabelsJSON = normalizeJSONText(r.LabelsJSON)
	r.CreatedAt = decodeDBTime(createdAt)
	return r, nil
}

func scanTraceTopology(row scanner) (store.TraceTopology, error) {
	var r store.TraceTopology
	var createdAt any
	if err := row.Scan(
		&r.ID, &r.WorkflowRunID, &r.WorkflowID, &r.StepID, &r.CaseID, &r.RequestID, &r.TraceID,
		&r.Status, &r.TopologyJSON, &r.TextTopology, &createdAt,
	); err != nil {
		if err == sql.ErrNoRows {
			return store.TraceTopology{}, store.ErrNotFound
		}
		return store.TraceTopology{}, err
	}
	r.TopologyJSON = normalizeJSONText(r.TopologyJSON)
	r.CreatedAt = decodeDBTime(createdAt)
	return r, nil
}

func scanPostProcessTask(row scanner) (store.PostProcessTask, error) {
	var r store.PostProcessTask
	var startedAt, finishedAt, createdAt any
	if err := row.Scan(
		&r.ID, &r.RunID, &r.WorkflowID, &r.StepID, &r.CaseID, &r.Kind, &r.Status,
		&startedAt, &finishedAt, &r.DurationMs, &r.Error, &r.SummaryJSON, &createdAt,
	); err != nil {
		if err == sql.ErrNoRows {
			return store.PostProcessTask{}, store.ErrNotFound
		}
		return store.PostProcessTask{}, err
	}
	r.SummaryJSON = normalizeJSONText(r.SummaryJSON)
	r.StartedAt = decodeDBTime(startedAt)
	r.FinishedAt = decodeDBTime(finishedAt)
	r.CreatedAt = decodeDBTime(createdAt)
	return r, nil
}

func scanBaselineGate(row scanner) (store.BaselineGate, error) {
	var r store.BaselineGate
	var checkedAt, updatedAt any
	if err := row.Scan(
		&r.ProfileID, &r.SubjectID, &r.Status, &r.Required, &r.SummaryJSON, &checkedAt, &updatedAt,
	); err != nil {
		if err == sql.ErrNoRows {
			return store.BaselineGate{}, store.ErrNotFound
		}
		return store.BaselineGate{}, err
	}
	r.SummaryJSON = normalizeJSONText(r.SummaryJSON)
	r.CheckedAt = decodeDBTime(checkedAt)
	r.UpdatedAt = decodeDBTime(updatedAt)
	return r, nil
}

func scanProfileIndex(row scanner) (store.ProfileIndex, error) {
	var r store.ProfileIndex
	var importedAt, updatedAt any
	if err := row.Scan(&r.ProfileID, &r.BundlePath, &r.BundleDigest, &r.SummaryJSON, &importedAt, &updatedAt); err != nil {
		if err == sql.ErrNoRows {
			return store.ProfileIndex{}, store.ErrNotFound
		}
		return store.ProfileIndex{}, err
	}
	r.SummaryJSON = normalizeJSONText(r.SummaryJSON)
	r.ImportedAt = decodeDBTime(importedAt)
	r.UpdatedAt = decodeDBTime(updatedAt)
	return r, nil
}

func scanConfigVersion(row scanner) (store.ConfigVersion, error) {
	var r store.ConfigVersion
	var publishedAt, createdAt any
	if err := row.Scan(&r.ID, &r.ProfileID, &r.SourcePath, &r.BundleDigest, &r.SummaryJSON, &r.Active, &publishedAt, &createdAt); err != nil {
		if err == sql.ErrNoRows {
			return store.ConfigVersion{}, store.ErrNotFound
		}
		return store.ConfigVersion{}, err
	}
	r.SummaryJSON = normalizeJSONText(r.SummaryJSON)
	r.PublishedAt = decodeDBTime(publishedAt)
	r.CreatedAt = decodeDBTime(createdAt)
	return r, nil
}

func scanReadModel(row scanner) (store.ReadModel, error) {
	var r store.ReadModel
	var generatedAt, updatedAt any
	if err := row.Scan(&r.ProfileID, &r.Key, &r.ConfigVersionID, &r.PayloadJSON, &generatedAt, &updatedAt); err != nil {
		if err == sql.ErrNoRows {
			return store.ReadModel{}, store.ErrNotFound
		}
		return store.ReadModel{}, err
	}
	r.PayloadJSON = normalizeJSONText(r.PayloadJSON)
	r.GeneratedAt = decodeDBTime(generatedAt)
	r.UpdatedAt = decodeDBTime(updatedAt)
	return r, nil
}

func scanProfileCatalogIndex(row scanner) (store.ProfileCatalogIndex, error) {
	var r store.ProfileCatalogIndex
	var indexedAt any
	if err := row.Scan(
		&r.ProfileID, &indexedAt, &r.Counts.Services, &r.Counts.Workflows, &r.Counts.InterfaceNodes,
		&r.Counts.APICases, &r.Counts.RequestTemplates, &r.Counts.WorkflowBindings, &r.Counts.CaseDependencies,
		&r.Counts.Fixtures, &r.Counts.Templates, &r.Counts.TemplateConfigs,
	); err != nil {
		if err == sql.ErrNoRows {
			return store.ProfileCatalogIndex{}, store.ErrNotFound
		}
		return store.ProfileCatalogIndex{}, err
	}
	r.IndexedAt = decodeDBTime(indexedAt)
	return r, nil
}

func scanEnvironment(row scanner) (store.Environment, error) {
	var e store.Environment
	var lastVerifiedAt, createdAt, updatedAt any
	if err := row.Scan(
		&e.ID, &e.DisplayName, &e.Description, &e.Status, &e.Verified, &e.ServicesJSON, &e.ReposJSON, &e.ComposeJSON,
		&e.HealthChecksJSON, &e.VerificationWorkflowID, &e.LastVerificationRunID, &e.LastVerificationStatus,
		&e.EvidenceComplete, &e.TopologyComplete, &lastVerifiedAt, &e.SummaryJSON, &createdAt, &updatedAt,
	); err != nil {
		if err == sql.ErrNoRows {
			return store.Environment{}, store.ErrNotFound
		}
		return store.Environment{}, err
	}
	e.ServicesJSON = normalizeJSONText(e.ServicesJSON)
	e.ReposJSON = normalizeJSONText(e.ReposJSON)
	e.ComposeJSON = normalizeJSONText(e.ComposeJSON)
	e.HealthChecksJSON = normalizeJSONText(e.HealthChecksJSON)
	e.SummaryJSON = normalizeJSONText(e.SummaryJSON)
	e.LastVerifiedAt = decodeDBTime(lastVerifiedAt)
	e.CreatedAt = decodeDBTime(createdAt)
	e.UpdatedAt = decodeDBTime(updatedAt)
	return e, nil
}

func catalogCounts(catalog store.ProfileCatalog) store.ProfileCatalogCounts {
	return store.ProfileCatalogCounts{
		Services:         len(catalog.Services),
		Workflows:        len(catalog.Workflows),
		InterfaceNodes:   len(catalog.InterfaceNodes),
		APICases:         len(catalog.APICases),
		RequestTemplates: len(catalog.RequestTemplates),
		WorkflowBindings: len(catalog.WorkflowBindings),
		CaseDependencies: len(catalog.CaseDependencies),
		Fixtures:         len(catalog.Fixtures),
		Templates:        len(catalog.Workflows) + len(catalog.RequestTemplates) + len(catalog.TemplateConfigs),
		TemplateConfigs:  len(catalog.Workflows) + len(catalog.RequestTemplates) + len(catalog.TemplateConfigs),
	}
}

func utcNow() time.Time {
	return time.Now().UTC()
}

func dbTimeArg(d Dialect, t time.Time) any {
	if t.IsZero() {
		if d.Name() == "sqlite" {
			return ""
		}
		return nil
	}
	if d.Name() == "sqlite" {
		return encodeTime(t)
	}
	return t.UTC()
}

func encodeTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339Nano)
}

func stringDefault(value string, defaultValue string) string {
	if strings.TrimSpace(value) == "" {
		return defaultValue
	}
	return value
}

func decodeTime(value string) time.Time {
	if value == "" {
		return time.Time{}
	}
	t, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return time.Time{}
	}
	return t
}

func decodeDBTime(value any) time.Time {
	switch v := value.(type) {
	case nil:
		return time.Time{}
	case time.Time:
		return v.UTC()
	case string:
		return decodeTime(v)
	case []byte:
		return decodeTime(string(v))
	default:
		return time.Time{}
	}
}

func normalizeJSONText(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return value
	}
	var decoded any
	if err := json.Unmarshal([]byte(value), &decoded); err != nil {
		return value
	}
	encoded, err := json.Marshal(decoded)
	if err != nil {
		return value
	}
	return string(encoded)
}
