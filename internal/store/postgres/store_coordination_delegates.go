package postgres

import (
	"context"
	"time"

	"agent-testbench/internal/store"
)

func (s *Store) UpdateRun(ctx context.Context, r store.Run) (store.Run, error) {
	return s.core.UpdateRun(ctx, r)
}

func (s *Store) CompareAndSwapRun(ctx context.Context, expectedUpdatedAt time.Time, expectedStatus string, r store.Run) (store.Run, error) {
	return s.core.CompareAndSwapRun(ctx, expectedUpdatedAt, expectedStatus, r)
}

func (s *Store) ClaimScheduledAgentTask(ctx context.Context, taskID string, expectedUpdatedAt time.Time, claimedAt time.Time) (store.AgentTaskClaim, bool, error) {
	return s.core.ClaimScheduledAgentTask(ctx, taskID, expectedUpdatedAt, claimedAt)
}

func (s *Store) ReleaseScheduledAgentTask(ctx context.Context, claim store.AgentTaskClaim, releasedAt time.Time) (bool, error) {
	return s.core.ReleaseScheduledAgentTask(ctx, claim, releasedAt)
}

func (s *Store) RecoverScheduledAgentTask(ctx context.Context, taskID string, recoveredAt time.Time) (bool, error) {
	return s.core.RecoverScheduledAgentTask(ctx, taskID, recoveredAt)
}

func (s *Store) ReplaceProfileCatalog(ctx context.Context, catalog store.ProfileCatalog) error {
	return s.core.ReplaceProfileCatalog(ctx, catalog)
}

func (s *Store) GetProfileCatalog(ctx context.Context) (store.ProfileCatalog, error) {
	return s.core.GetProfileCatalog(ctx)
}

func (s *Store) GetProfileCatalogByID(ctx context.Context, profileID string) (store.ProfileCatalog, error) {
	return s.core.GetProfileCatalogByID(ctx, profileID)
}

func (s *Store) GetProfileCatalogIndex(ctx context.Context) (store.ProfileCatalogIndex, error) {
	return s.core.GetProfileCatalogIndex(ctx)
}

func (s *Store) ListProfileCatalogIndexes(ctx context.Context) ([]store.ProfileCatalogIndex, error) {
	return s.core.ListProfileCatalogIndexes(ctx)
}

func (s *Store) GetProfileCatalogSnapshot(ctx context.Context, profileID string) (store.ProfileCatalogSnapshot, error) {
	return s.core.GetProfileCatalogSnapshot(ctx, profileID)
}

func (s *Store) CompareAndSwapProfileCatalog(ctx context.Context, expectedRevision int64, catalog store.ProfileCatalog, mutation store.ProfileCatalogMutation) (store.ProfileCatalogSnapshot, error) {
	return s.core.CompareAndSwapProfileCatalog(ctx, expectedRevision, catalog, mutation)
}

func (s *Store) ListProfileCatalogVersions(ctx context.Context, profileID string, limit int) ([]store.ProfileCatalogVersion, error) {
	return s.core.ListProfileCatalogVersions(ctx, profileID, limit)
}

func (s *Store) GetProfileCatalogVersion(ctx context.Context, profileID string, revision int64) (store.ProfileCatalogVersion, error) {
	return s.core.GetProfileCatalogVersion(ctx, profileID, revision)
}

func (s *Store) ReplaceTestPlanGraph(ctx context.Context, graph store.TestPlanGraph) error {
	return s.core.ReplaceTestPlanGraph(ctx, graph)
}

func (s *Store) GetTestPlanGraph(ctx context.Context, mapID string) (store.TestPlanGraph, error) {
	return s.core.GetTestPlanGraph(ctx, mapID)
}

func (s *Store) ListTestPlanMaps(ctx context.Context) ([]store.TestPlanMapSummary, error) {
	return s.core.ListTestPlanMaps(ctx)
}

func (s *Store) SaveTestPlanMapVersion(ctx context.Context, item store.TestPlanMapVersion) (store.TestPlanMapVersion, error) {
	return s.core.SaveTestPlanMapVersion(ctx, item)
}

func (s *Store) ListTestPlanMapVersions(ctx context.Context, mapID string) ([]store.TestPlanMapVersion, error) {
	return s.core.ListTestPlanMapVersions(ctx, mapID)
}

func (s *Store) SaveTestMapPlan(ctx context.Context, record store.TestMapPlanRecord) error {
	return s.core.SaveTestMapPlan(ctx, record)
}

func (s *Store) GetTestMapPlan(ctx context.Context, planID string) (store.TestMapPlanRecord, error) {
	return s.core.GetTestMapPlan(ctx, planID)
}

func (s *Store) ListTestMapPlans(ctx context.Context, mapID string, limit int) ([]store.TestMapPlanInstance, error) {
	return s.core.ListTestMapPlans(ctx, mapID, limit)
}

func (s *Store) UpdateTestMapPlanInstance(ctx context.Context, instance store.TestMapPlanInstance) error {
	return s.core.UpdateTestMapPlanInstance(ctx, instance)
}

func (s *Store) UpdateTestMapPlanTask(ctx context.Context, task store.TestMapPlanTask) error {
	return s.core.UpdateTestMapPlanTask(ctx, task)
}

func (s *Store) ClaimTestMapPlanLease(ctx context.Context, lease store.TestMapPlanLease, now time.Time, allowUnleasedRunning bool) (store.TestMapPlanLease, error) {
	return s.core.ClaimTestMapPlanLease(ctx, lease, now, allowUnleasedRunning)
}

func (s *Store) RenewTestMapPlanLease(ctx context.Context, lease store.TestMapPlanLease, now time.Time, expiresAt time.Time) (store.TestMapPlanLease, error) {
	return s.core.RenewTestMapPlanLease(ctx, lease, now, expiresAt)
}

func (s *Store) ResetTestMapPlanWithLease(ctx context.Context, lease store.TestMapPlanLease, record store.TestMapPlanRecord, now time.Time, expiresAt time.Time) (store.TestMapPlanLease, error) {
	return s.core.ResetTestMapPlanWithLease(ctx, lease, record, now, expiresAt)
}

func (s *Store) UpdateTestMapPlanTaskWithLease(ctx context.Context, lease store.TestMapPlanLease, task store.TestMapPlanTask, now time.Time, expiresAt time.Time) (store.TestMapPlanLease, error) {
	return s.core.UpdateTestMapPlanTaskWithLease(ctx, lease, task, now, expiresAt)
}

func (s *Store) UpdateTestMapPlanInstanceWithLease(ctx context.Context, lease store.TestMapPlanLease, instance store.TestMapPlanInstance, now time.Time, expiresAt time.Time) (store.TestMapPlanLease, error) {
	return s.core.UpdateTestMapPlanInstanceWithLease(ctx, lease, instance, now, expiresAt)
}

func (s *Store) ReleaseTestMapPlanLease(ctx context.Context, lease store.TestMapPlanLease, final store.TestMapPlanInstance, now time.Time) error {
	return s.core.ReleaseTestMapPlanLease(ctx, lease, final, now)
}
