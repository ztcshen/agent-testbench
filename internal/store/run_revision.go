package store

import (
	"context"
	"errors"
	"fmt"
	"time"
)

var ErrRunRevisionConflict = errors.New("run revision conflict")

// RunRevisionConflictError reports that a Run changed after it was read.
// Callers must reload the Run before deciding whether they still own it.
type RunRevisionConflictError struct {
	RunID             string
	ExpectedStatus    string
	ExpectedUpdatedAt time.Time
}

func (e *RunRevisionConflictError) Error() string {
	return fmt.Sprintf("run %q changed after status %q at %s", e.RunID, e.ExpectedStatus, e.ExpectedUpdatedAt.UTC().Format(time.RFC3339Nano))
}

func (e *RunRevisionConflictError) Unwrap() error {
	return ErrRunRevisionConflict
}

// RunCompareAndSwapStore is an optional Store capability for owner-safe Run
// checkpoints. Both status and updated_at participate in the SQL condition so
// a stale worker cannot overwrite a newer owner or terminal state.
type RunCompareAndSwapStore interface {
	CompareAndSwapRun(context.Context, time.Time, string, Run) (Run, error)
}
