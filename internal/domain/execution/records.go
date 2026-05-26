// Package execution defines run records and statuses shared across boundaries.
package execution

import (
	"errors"
	"time"
)

var ErrNotFound = errors.New("store record not found")

const (
	StatusRunning = "running"
	StatusPassed  = "passed"
	StatusFailed  = "failed"
	StatusSkipped = "skipped"
)

type Run struct {
	ID            string
	ProfileID     string
	EnvironmentID string
	WorkflowID    string
	Status        string
	EvidenceRoot  string
	SummaryJSON   string
	StartedAt     time.Time
	FinishedAt    time.Time
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

type APICaseRun struct {
	ID                   string
	RunID                string
	CaseID               string
	Status               string
	RequestSummaryJSON   string
	AssertionSummaryJSON string
	StartedAt            time.Time
	FinishedAt           time.Time
	CreatedAt            time.Time
}

type APICaseRunRecord struct {
	Run     Run
	CaseRun APICaseRun
}
