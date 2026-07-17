package controlplane

import (
	"context"
	"sync"

	"agent-testbench/internal/store"
)

// apiCaseBatchFencedStore renews the batch owner lease immediately before
// every Store mutation performed by a composite persistence operation. Reads
// are delegated directly to the wrapped Store.
type apiCaseBatchFencedStore struct {
	store.Store
	beforeWrite func(context.Context) error

	mu       sync.Mutex
	fenceErr error
}

func (r *apiCaseBatchRunner) fencedStore(runtime store.Store, batchRunID string) *apiCaseBatchFencedStore {
	return &apiCaseBatchFencedStore{
		Store: runtime,
		beforeWrite: func(ctx context.Context) error {
			return r.requireOwnerFence(ctx, runtime, batchRunID)
		},
	}
}

func (s *apiCaseBatchFencedStore) guard(ctx context.Context) error {
	if s == nil || s.beforeWrite == nil {
		return nil
	}
	if err := s.beforeWrite(ctx); err != nil {
		s.mu.Lock()
		if s.fenceErr == nil {
			s.fenceErr = err
		}
		s.mu.Unlock()
		return err
	}
	return nil
}

func (s *apiCaseBatchFencedStore) FenceError() error {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.fenceErr
}

func (s *apiCaseBatchFencedStore) CreateRun(ctx context.Context, run store.Run) (store.Run, error) {
	if err := s.guard(ctx); err != nil {
		return store.Run{}, err
	}
	return s.Store.CreateRun(ctx, run)
}

func (s *apiCaseBatchFencedStore) RecordAPICaseRun(ctx context.Context, run store.APICaseRun) (store.APICaseRun, error) {
	if err := s.guard(ctx); err != nil {
		return store.APICaseRun{}, err
	}
	return s.Store.RecordAPICaseRun(ctx, run)
}

func (s *apiCaseBatchFencedStore) RecordEvidence(ctx context.Context, evidence store.EvidenceRecord) (store.EvidenceRecord, error) {
	if err := s.guard(ctx); err != nil {
		return store.EvidenceRecord{}, err
	}
	return s.Store.RecordEvidence(ctx, evidence)
}

func (s *apiCaseBatchFencedStore) SaveTraceTopology(ctx context.Context, topology store.TraceTopology) (store.TraceTopology, error) {
	if err := s.guard(ctx); err != nil {
		return store.TraceTopology{}, err
	}
	return s.Store.SaveTraceTopology(ctx, topology)
}

func (s *apiCaseBatchFencedStore) RecordPostProcessTask(ctx context.Context, task store.PostProcessTask) (store.PostProcessTask, error) {
	if err := s.guard(ctx); err != nil {
		return store.PostProcessTask{}, err
	}
	return s.Store.RecordPostProcessTask(ctx, task)
}

func (s *apiCaseBatchFencedStore) UpsertEnvironment(ctx context.Context, environment store.Environment) (store.Environment, error) {
	if err := s.guard(ctx); err != nil {
		return store.Environment{}, err
	}
	return s.Store.UpsertEnvironment(ctx, environment)
}
