package store

import (
	"context"
	"errors"
	"fmt"
	"time"
)

var (
	ErrTestMapPlanLeaseConflict = errors.New("test map plan lease conflict")
	ErrTestMapPlanLeaseLost     = errors.New("test map plan lease lost")
)

// TestMapPlanLease is the Store-owned execution claim for one persisted map
// plan. Token prevents a stale process with the same owner id from writing
// checkpoints after a newer attempt takes ownership.
type TestMapPlanLease struct {
	PlanID    string
	OwnerID   string
	Token     string
	ExpiresAt time.Time
	UpdatedAt time.Time
}

type TestMapPlanLeaseConflictError struct {
	PlanID        string
	CurrentOwner  string
	CurrentExpiry time.Time
	Reason        string
}

func (e *TestMapPlanLeaseConflictError) Error() string {
	detail := e.Reason
	if detail == "" {
		detail = "another executor owns an active lease"
	}
	return fmt.Sprintf("test map plan %q lease conflict: %s", e.PlanID, detail)
}

func (e *TestMapPlanLeaseConflictError) Unwrap() error { return ErrTestMapPlanLeaseConflict }

// MapPlannerLeaseStore guards every execution checkpoint with the same Store
// lease that was atomically claimed before external requests begin.
type MapPlannerLeaseStore interface {
	ClaimTestMapPlanLease(context.Context, TestMapPlanLease, time.Time, bool) (TestMapPlanLease, error)
	RenewTestMapPlanLease(context.Context, TestMapPlanLease, time.Time, time.Time) (TestMapPlanLease, error)
	ResetTestMapPlanWithLease(context.Context, TestMapPlanLease, TestMapPlanRecord, time.Time, time.Time) (TestMapPlanLease, error)
	UpdateTestMapPlanTaskWithLease(context.Context, TestMapPlanLease, TestMapPlanTask, time.Time, time.Time) (TestMapPlanLease, error)
	UpdateTestMapPlanInstanceWithLease(context.Context, TestMapPlanLease, TestMapPlanInstance, time.Time, time.Time) (TestMapPlanLease, error)
	ReleaseTestMapPlanLease(context.Context, TestMapPlanLease, TestMapPlanInstance, time.Time) error
}
