package sqlite

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"agent-testbench/internal/store"
)

func TestOpenDBConstrainsSQLiteToConfiguredConnection(t *testing.T) {
	db, err := openDB(context.Background(), Config{Path: filepath.Join(t.TempDir(), "store.sqlite")})
	if err != nil {
		t.Fatalf("open sqlite db: %v", err)
	}
	defer db.Close()

	if got := db.Stats().MaxOpenConnections; got != 1 {
		t.Fatalf("sqlite db max open connections = %d, want 1 so connection-local pragmas stay enforced", got)
	}
}

func TestConcurrentMapPlanClaimsHaveSingleOwner(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "store.sqlite")
	first, err := Open(ctx, Config{Path: path})
	if err != nil {
		t.Fatalf("open first sqlite store: %v", err)
	}
	defer first.Close()
	second, err := Open(ctx, Config{Path: path})
	if err != nil {
		t.Fatalf("open second sqlite store: %v", err)
	}
	defer second.Close()
	if err := first.SaveTestMapPlan(ctx, store.TestMapPlanRecord{Instance: store.TestMapPlanInstance{
		ID: "plan.concurrent", MapID: "map.concurrent", ProfileID: "profile.concurrent", Status: "planned",
	}}); err != nil {
		t.Fatalf("save concurrent map plan: %v", err)
	}

	now := time.Now().UTC()
	start := make(chan struct{})
	results := make(chan error, 2)
	var group sync.WaitGroup
	for index, candidate := range []*Store{first, second} {
		index, candidate := index, candidate
		group.Add(1)
		go func() {
			defer group.Done()
			<-start
			_, claimErr := candidate.ClaimTestMapPlanLease(ctx, store.TestMapPlanLease{
				PlanID: "plan.concurrent", OwnerID: "owner.concurrent." + string(rune('a'+index)),
				Token: "token.concurrent." + string(rune('a'+index)), ExpiresAt: now.Add(time.Minute),
			}, now, false)
			results <- claimErr
		}()
	}
	close(start)
	group.Wait()
	close(results)

	succeeded := 0
	conflicted := 0
	for claimErr := range results {
		switch {
		case claimErr == nil:
			succeeded++
		case errors.Is(claimErr, store.ErrTestMapPlanLeaseConflict):
			conflicted++
		default:
			t.Fatalf("concurrent map plan claim error: %v", claimErr)
		}
	}
	if succeeded != 1 || conflicted != 1 {
		t.Fatalf("concurrent claims succeeded=%d conflicted=%d, want 1/1", succeeded, conflicted)
	}
}
