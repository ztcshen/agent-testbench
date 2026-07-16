package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"agent-testbench/internal/domain/mapplanner"
	"agent-testbench/internal/store"
)

const (
	mapRunLeaseDuration          = 30 * time.Second
	mapRunLeaseHeartbeatInterval = 10 * time.Second
)

type mapRunLeaseState struct {
	mu sync.Mutex

	store  store.MapPlannerLeaseStore
	lease  store.TestMapPlanLease
	cancel context.CancelCauseFunc

	heartbeatCancel context.CancelFunc
	heartbeatDone   chan struct{}
	heartbeatEvery  time.Duration
	released        bool
}

func claimMapRunPlan(ctx context.Context, runtime store.Store, record store.TestMapPlanRecord, options mapRunOptions) (store.MapPlannerLeaseStore, store.TestMapPlanLease, store.TestMapPlanRecord, error) {
	leaseStore, ok := runtime.(store.MapPlannerLeaseStore)
	if !ok {
		return nil, store.TestMapPlanLease{}, store.TestMapPlanRecord{}, errors.New("Store does not support claimed test map plan execution")
	}
	if options.planID == "" {
		if err := runtime.SaveTestMapPlan(ctx, record); err != nil {
			return nil, store.TestMapPlanLease{}, store.TestMapPlanRecord{}, err
		}
	}
	now := time.Now().UTC()
	ownerID, token, err := newMapRunLeaseIdentity()
	if err != nil {
		return nil, store.TestMapPlanLease{}, store.TestMapPlanRecord{}, err
	}
	lease, err := leaseStore.ClaimTestMapPlanLease(ctx, store.TestMapPlanLease{
		PlanID: record.Instance.ID, OwnerID: ownerID, Token: token, ExpiresAt: now.Add(mapRunLeaseDuration),
	}, now, options.planID != "" && options.resumeRun)
	if err != nil {
		return nil, store.TestMapPlanLease{}, store.TestMapPlanRecord{}, err
	}
	if options.planID != "" {
		record = prepareExistingMapRunRecordAt(record, options, now)
		lease, err = leaseStore.ResetTestMapPlanWithLease(ctx, lease, record, now, now.Add(mapRunLeaseDuration))
		if err != nil {
			return nil, store.TestMapPlanLease{}, store.TestMapPlanRecord{}, err
		}
	} else {
		record.Instance.Mode = mapplanner.ModeRun
		record.Instance.Status = mapplanner.TaskStatusRunning
		record.Instance.StartedAt = now
		record.Instance.FinishedAt = time.Time{}
	}
	return leaseStore, lease, record, nil
}

func newMapRunLeaseIdentity() (string, string, error) {
	ownerRandom, err := randomMapRunLeaseID(8)
	if err != nil {
		return "", "", err
	}
	token, err := randomMapRunLeaseID(16)
	if err != nil {
		return "", "", err
	}
	hostname, _ := os.Hostname()
	hostname = strings.TrimSpace(hostname)
	if hostname == "" {
		hostname = "local"
	}
	ownerID := fmt.Sprintf("%s:%d:%s", safeBoundedReportID(hostname, 80), os.Getpid(), ownerRandom)
	return ownerID, token, nil
}

func randomMapRunLeaseID(byteCount int) (string, error) {
	raw := make([]byte, byteCount)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("generate test map plan lease identity: %w", err)
	}
	return hex.EncodeToString(raw), nil
}

func (e *mapRunExecutor) attachLease(leaseStore store.MapPlannerLeaseStore, lease store.TestMapPlanLease, cancel context.CancelCauseFunc) {
	e.lease = &mapRunLeaseState{store: leaseStore, lease: lease, cancel: cancel, heartbeatEvery: mapRunLeaseHeartbeatInterval}
}

func (e mapRunExecutor) startLeaseHeartbeat() {
	if e.lease == nil {
		return
	}
	heartbeatCtx, cancel := context.WithCancel(e.ctx)
	done := make(chan struct{})
	e.lease.mu.Lock()
	e.lease.heartbeatCancel = cancel
	e.lease.heartbeatDone = done
	e.lease.mu.Unlock()
	go func() {
		defer close(done)
		e.lease.mu.Lock()
		heartbeatEvery := e.lease.heartbeatEvery
		e.lease.mu.Unlock()
		if heartbeatEvery <= 0 {
			heartbeatEvery = mapRunLeaseHeartbeatInterval
		}
		ticker := time.NewTicker(heartbeatEvery)
		defer ticker.Stop()
		for {
			select {
			case <-heartbeatCtx.Done():
				return
			case now := <-ticker.C:
				if err := e.renewLease(now.UTC()); err != nil {
					e.recordLeaseError(err)
					return
				}
			}
		}
	}()
}

func (e mapRunExecutor) renewLease(now time.Time) error {
	state := e.lease
	if state == nil {
		return nil
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	if state.released {
		return nil
	}
	next, err := state.store.RenewTestMapPlanLease(e.ctx, state.lease, now, now.Add(mapRunLeaseDuration))
	if err != nil {
		return err
	}
	state.lease = next
	return nil
}

// fenceMapRunOwnership renews the claimed plan immediately before an external
// request or a durable child write. The running-task checkpoint remains the
// unknown-outcome marker; this additional fence closes the gap between that
// checkpoint and each step in a multi-step path.
func (e mapRunExecutor) fenceMapRunOwnership() error {
	if err := e.executionError(); err != nil {
		return err
	}
	if e.lease == nil {
		return nil
	}
	if err := e.renewLease(time.Now().UTC()); err != nil {
		e.recordLeaseError(err)
		return err
	}
	return e.executionError()
}

func (e mapRunExecutor) checkpointClaimedTask(task store.TestMapPlanTask) error {
	state := e.lease
	state.mu.Lock()
	defer state.mu.Unlock()
	if state.released {
		return fmt.Errorf("%w: plan %q lease was already released", store.ErrTestMapPlanLeaseLost, state.lease.PlanID)
	}
	now := time.Now().UTC()
	next, err := state.store.UpdateTestMapPlanTaskWithLease(e.ctx, state.lease, task, now, now.Add(mapRunLeaseDuration))
	if err != nil {
		return err
	}
	state.lease = next
	return nil
}

func (e mapRunExecutor) finishClaimedInstance(instance store.TestMapPlanInstance) error {
	if e.lease == nil {
		return nil
	}
	e.stopLeaseHeartbeat()
	if err := e.checkpointError(); err != nil {
		return err
	}
	state := e.lease
	state.mu.Lock()
	defer state.mu.Unlock()
	if state.released {
		return fmt.Errorf("%w: plan %q lease was already released", store.ErrTestMapPlanLeaseLost, state.lease.PlanID)
	}
	if err := state.store.ReleaseTestMapPlanLease(e.ctx, state.lease, instance, time.Now().UTC()); err != nil {
		return err
	}
	state.released = true
	return nil
}

func (e mapRunExecutor) stopLeaseHeartbeat() {
	state := e.lease
	if state == nil {
		return
	}
	state.mu.Lock()
	cancel := state.heartbeatCancel
	done := state.heartbeatDone
	state.heartbeatCancel = nil
	state.heartbeatDone = nil
	state.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	if done != nil {
		<-done
	}
}

func (e mapRunExecutor) recordLeaseError(err error) {
	if err == nil {
		return
	}
	e.recordCheckpointError(err)
	if e.lease != nil && e.lease.cancel != nil {
		e.lease.cancel(err)
	}
}
