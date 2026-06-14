// Package postgres adapts the shared SQL Store to PostgreSQL.
package postgres

import (
	"context"

	"agent-testbench/internal/store"
)

var _ store.Store = (*Store)(nil)

func (s *Store) CreateRun(ctx context.Context, r store.Run) (store.Run, error) {
	return s.core.CreateRun(ctx, r)
}

func (s *Store) GetRun(ctx context.Context, id string) (store.Run, error) {
	return s.core.GetRun(ctx, id)
}

func (s *Store) ListRuns(ctx context.Context) ([]store.Run, error) {
	return s.core.ListRuns(ctx)
}

func (s *Store) RecordAPICaseRun(ctx context.Context, r store.APICaseRun) (store.APICaseRun, error) {
	return s.core.RecordAPICaseRun(ctx, r)
}

func (s *Store) ListAPICaseRuns(ctx context.Context, runID string) ([]store.APICaseRun, error) {
	return s.core.ListAPICaseRuns(ctx, runID)
}

func (s *Store) ListLatestAPICaseRuns(ctx context.Context) ([]store.APICaseRun, error) {
	return s.core.ListLatestAPICaseRuns(ctx)
}

func (s *Store) RecordEvidence(ctx context.Context, r store.EvidenceRecord) (store.EvidenceRecord, error) {
	return s.core.RecordEvidence(ctx, r)
}

func (s *Store) ListEvidence(ctx context.Context, runID string) ([]store.EvidenceRecord, error) {
	return s.core.ListEvidence(ctx, runID)
}

func (s *Store) SaveTraceTopology(ctx context.Context, r store.TraceTopology) (store.TraceTopology, error) {
	return s.core.SaveTraceTopology(ctx, r)
}

func (s *Store) ListTraceTopologies(ctx context.Context, workflowRunID string) ([]store.TraceTopology, error) {
	return s.core.ListTraceTopologies(ctx, workflowRunID)
}

func (s *Store) RecordPostProcessTask(ctx context.Context, r store.PostProcessTask) (store.PostProcessTask, error) {
	return s.core.RecordPostProcessTask(ctx, r)
}

func (s *Store) ListPostProcessTasks(ctx context.Context, runID string) ([]store.PostProcessTask, error) {
	return s.core.ListPostProcessTasks(ctx, runID)
}

func (s *Store) UpsertAgentTask(ctx context.Context, t store.AgentTask) (store.AgentTask, error) {
	return s.core.UpsertAgentTask(ctx, t)
}

func (s *Store) GetAgentTask(ctx context.Context, ref string) (store.AgentTask, error) {
	return s.core.GetAgentTask(ctx, ref)
}

func (s *Store) ListAgentTasks(ctx context.Context) ([]store.AgentTask, error) {
	return s.core.ListAgentTasks(ctx)
}

func (s *Store) RecordAgentTaskRun(ctx context.Context, r store.AgentTaskRun) (store.AgentTaskRun, error) {
	return s.core.RecordAgentTaskRun(ctx, r)
}

func (s *Store) ListAgentTaskRuns(ctx context.Context, taskID string, limit int) ([]store.AgentTaskRun, error) {
	return s.core.ListAgentTaskRuns(ctx, taskID, limit)
}

func (s *Store) UpsertBaselineGate(ctx context.Context, r store.BaselineGate) (store.BaselineGate, error) {
	return s.core.UpsertBaselineGate(ctx, r)
}

func (s *Store) GetBaselineGate(ctx context.Context, profileID string, subjectID string) (store.BaselineGate, error) {
	return s.core.GetBaselineGate(ctx, profileID, subjectID)
}

func (s *Store) UpsertProfileIndex(ctx context.Context, r store.ProfileIndex) (store.ProfileIndex, error) {
	return s.core.UpsertProfileIndex(ctx, r)
}

func (s *Store) GetProfileIndex(ctx context.Context, profileID string) (store.ProfileIndex, error) {
	return s.core.GetProfileIndex(ctx, profileID)
}

func (s *Store) UpsertConfigVersion(ctx context.Context, r store.ConfigVersion) (store.ConfigVersion, error) {
	return s.core.UpsertConfigVersion(ctx, r)
}

func (s *Store) GetActiveConfigVersion(ctx context.Context) (store.ConfigVersion, error) {
	return s.core.GetActiveConfigVersion(ctx)
}

func (s *Store) UpsertReadModel(ctx context.Context, r store.ReadModel) (store.ReadModel, error) {
	return s.core.UpsertReadModel(ctx, r)
}

func (s *Store) GetReadModel(ctx context.Context, profileID string, key string) (store.ReadModel, error) {
	return s.core.GetReadModel(ctx, profileID, key)
}

func (s *Store) ReplaceProfileCatalog(ctx context.Context, catalog store.ProfileCatalog) error {
	return s.core.ReplaceProfileCatalog(ctx, catalog)
}

func (s *Store) GetProfileCatalog(ctx context.Context) (store.ProfileCatalog, error) {
	return s.core.GetProfileCatalog(ctx)
}

func (s *Store) GetProfileCatalogIndex(ctx context.Context) (store.ProfileCatalogIndex, error) {
	return s.core.GetProfileCatalogIndex(ctx)
}

func (s *Store) UpsertEnvironment(ctx context.Context, e store.Environment) (store.Environment, error) {
	return s.core.UpsertEnvironment(ctx, e)
}

func (s *Store) UpsertEnvironmentStructuredState(ctx context.Context, e store.Environment, files []store.EnvironmentFile, services []store.EnvironmentService, checks []store.EnvironmentHealthCheck) (store.Environment, error) {
	return s.core.UpsertEnvironmentStructuredState(ctx, e, files, services, checks)
}

func (s *Store) GetEnvironment(ctx context.Context, id string) (store.Environment, error) {
	return s.core.GetEnvironment(ctx, id)
}

func (s *Store) ListEnvironments(ctx context.Context) ([]store.Environment, error) {
	return s.core.ListEnvironments(ctx)
}

func (s *Store) ReplaceEnvironmentFiles(ctx context.Context, envID string, files []store.EnvironmentFile) error {
	return s.core.ReplaceEnvironmentFiles(ctx, envID, files)
}

func (s *Store) ListEnvironmentFiles(ctx context.Context, envID string) ([]store.EnvironmentFile, error) {
	return s.core.ListEnvironmentFiles(ctx, envID)
}

func (s *Store) ReplaceEnvironmentServices(ctx context.Context, envID string, services []store.EnvironmentService) error {
	return s.core.ReplaceEnvironmentServices(ctx, envID, services)
}

func (s *Store) ListEnvironmentServices(ctx context.Context, envID string) ([]store.EnvironmentService, error) {
	return s.core.ListEnvironmentServices(ctx, envID)
}

func (s *Store) ReplaceEnvironmentHealthChecks(ctx context.Context, envID string, checks []store.EnvironmentHealthCheck) error {
	return s.core.ReplaceEnvironmentHealthChecks(ctx, envID, checks)
}

func (s *Store) ListEnvironmentHealthChecks(ctx context.Context, envID string) ([]store.EnvironmentHealthCheck, error) {
	return s.core.ListEnvironmentHealthChecks(ctx, envID)
}

func (s *Store) ReplaceEnvironmentComponentGraph(ctx context.Context, envID string, graph store.EnvironmentComponentGraph) error {
	return s.core.ReplaceEnvironmentComponentGraph(ctx, envID, graph)
}

func (s *Store) GetEnvironmentComponentGraph(ctx context.Context, envID string) (store.EnvironmentComponentGraph, error) {
	return s.core.GetEnvironmentComponentGraph(ctx, envID)
}
