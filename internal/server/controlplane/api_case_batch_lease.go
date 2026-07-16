package controlplane

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync/atomic"
	"time"

	"agent-testbench/internal/store"
)

const defaultAPICaseBatchLeaseDuration = 30 * time.Second

var (
	errAPICaseBatchLeaseLost  = errors.New("API case batch owner lease lost")
	apiCaseBatchOwnerSequence atomic.Uint64
)

type apiCaseBatchLease struct {
	HolderIdentity      string `json:"holderIdentity"`
	Token               string `json:"token"`
	RenewTime           string `json:"renewTime"`
	LeaseDurationMillis int64  `json:"leaseDurationMillis"`
}

func normalizeAPICaseBatchLeaseDuration(value time.Duration) time.Duration {
	if value <= 0 {
		return defaultAPICaseBatchLeaseDuration
	}
	if value < 30*time.Millisecond {
		return 30 * time.Millisecond
	}
	return value
}

func newAPICaseBatchOwnerID() string {
	now := time.Now().UTC().UnixNano()
	sequence := apiCaseBatchOwnerSequence.Add(1)
	return fmt.Sprintf("control-plane-%d-%d-%d", os.Getpid(), now, sequence)
}

func (r *apiCaseBatchRunner) newLease(now time.Time) apiCaseBatchLease {
	return apiCaseBatchLease{
		HolderIdentity:      r.ownerID,
		Token:               newAPICaseBatchOwnerID(),
		RenewTime:           now.UTC().Format(time.RFC3339Nano),
		LeaseDurationMillis: r.leaseDuration.Milliseconds(),
	}
}

func (lease apiCaseBatchLease) sameOwner(other apiCaseBatchLease) bool {
	return strings.TrimSpace(lease.HolderIdentity) != "" &&
		strings.TrimSpace(lease.Token) != "" &&
		lease.HolderIdentity == other.HolderIdentity &&
		lease.Token == other.Token
}

func (lease apiCaseBatchLease) activeAt(now time.Time) bool {
	if strings.TrimSpace(lease.HolderIdentity) == "" || strings.TrimSpace(lease.Token) == "" || lease.LeaseDurationMillis <= 0 {
		return false
	}
	renewedAt, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(lease.RenewTime))
	if err != nil {
		return false
	}
	return now.UTC().Before(renewedAt.UTC().Add(time.Duration(lease.LeaseDurationMillis) * time.Millisecond))
}

func decodeAPICaseBatchLease(raw string) (apiCaseBatchLease, error) {
	var envelope struct {
		Lease apiCaseBatchLease `json:"_lease"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(raw)), &envelope); err != nil {
		return apiCaseBatchLease{}, err
	}
	return envelope.Lease, nil
}

func apiCaseBatchPersistedSummary(report apiCaseBatchRunReport) map[string]any {
	out := apiCaseBatchRunStoreSummary(report)
	if strings.TrimSpace(report.lease.HolderIdentity) != "" {
		out["_lease"] = report.lease
	}
	return out
}

func nextAPICaseBatchRunRevision(previous time.Time, now time.Time) time.Time {
	next := now.UTC().Truncate(time.Microsecond)
	if !next.After(previous) {
		next = previous.UTC().Add(time.Microsecond)
	}
	return next
}

func (r *apiCaseBatchRunner) checkpoint(ctx context.Context, runtime store.Store, report apiCaseBatchRunReport) error {
	r.checkpointMu.Lock()
	defer r.checkpointMu.Unlock()
	return checkpointAPICaseBatchRun(ctx, runtime, report)
}

func (r *apiCaseBatchRunner) startHeartbeat(ctx context.Context, cancel context.CancelFunc, runtime store.Store, batchRunID string) {
	if runtime == nil {
		return
	}
	interval := r.leaseDuration / 3
	if interval < 10*time.Millisecond {
		interval = 10 * time.Millisecond
	}
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := r.renewLease(ctx, runtime, batchRunID); err != nil {
					r.remove(batchRunID)
					cancel()
					return
				}
			}
		}
	}()
}

func (r *apiCaseBatchRunner) renewLease(ctx context.Context, runtime store.Store, batchRunID string) error {
	r.checkpointMu.Lock()
	defer r.checkpointMu.Unlock()
	report, ok := r.get(batchRunID)
	if !ok {
		return fmt.Errorf("%w: batch run %q is not loaded", errAPICaseBatchLeaseLost, batchRunID)
	}
	cas, ok := runtime.(store.RunCompareAndSwapStore)
	if !ok {
		return fmt.Errorf("renew batch %q lease: Store does not support compare-and-swap Run updates", batchRunID)
	}
	current, err := runtime.GetRun(ctx, batchRunID)
	if err != nil {
		return fmt.Errorf("load batch %q for lease renewal: %w", batchRunID, err)
	}
	currentLease, err := decodeAPICaseBatchLease(current.SummaryJSON)
	if err != nil {
		return fmt.Errorf("decode batch %q lease: %w", batchRunID, err)
	}
	if current.Status != store.StatusRunning || !report.lease.sameOwner(currentLease) {
		return fmt.Errorf("%w: batch run %q is now held by %q", errAPICaseBatchLeaseLost, batchRunID, currentLease.HolderIdentity)
	}
	currentLease.RenewTime = time.Now().UTC().Format(time.RFC3339Nano)
	var summary map[string]any
	if err := json.Unmarshal([]byte(current.SummaryJSON), &summary); err != nil {
		return fmt.Errorf("decode batch %q summary for lease renewal: %w", batchRunID, err)
	}
	summary["_lease"] = currentLease
	current.SummaryJSON = compactJSON(summary)
	expectedUpdatedAt := current.UpdatedAt
	current.UpdatedAt = nextAPICaseBatchRunRevision(expectedUpdatedAt, time.Now().UTC())
	if _, err := cas.CompareAndSwapRun(ctx, expectedUpdatedAt, current.Status, current); err != nil {
		if errors.Is(err, store.ErrRunRevisionConflict) {
			return fmt.Errorf("%w: batch run %q changed during lease renewal", errAPICaseBatchLeaseLost, batchRunID)
		}
		return fmt.Errorf("renew batch %q lease: %w", batchRunID, err)
	}
	r.updateLease(batchRunID, currentLease)
	return nil
}

// requireOwnerFence performs a fresh owner-token CAS immediately before the
// caller starts a durable child or finalization write. A confirmed owner loss
// removes the stale in-memory copy. Other fence failures stop execution and are
// exposed locally without attempting another durable write.
func (r *apiCaseBatchRunner) requireOwnerFence(ctx context.Context, runtime store.Store, batchRunID string) error {
	if runtime == nil {
		return nil
	}
	if err := r.renewLease(ctx, runtime, batchRunID); err != nil {
		if errors.Is(err, errAPICaseBatchLeaseLost) {
			r.remove(batchRunID)
		} else {
			r.markPersistenceFailure(batchRunID, err)
		}
		return err
	}
	return nil
}

func (r *apiCaseBatchRunner) updateLease(batchRunID string, lease apiCaseBatchLease) {
	r.mu.Lock()
	defer r.mu.Unlock()
	report, ok := r.runs[batchRunID]
	if !ok || !report.lease.sameOwner(lease) {
		return
	}
	report.lease = lease
	r.runs[batchRunID] = report
}

func (r *apiCaseBatchRunner) remove(batchRunID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.runs, batchRunID)
}
