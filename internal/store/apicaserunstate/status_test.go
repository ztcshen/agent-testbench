package apicaserunstate

import (
	"context"
	"testing"

	"agent-testbench/internal/store"
)

type fakeStore struct {
	runs     []store.Run
	caseRuns map[string][]store.APICaseRun
}

func (s fakeStore) ListRuns(context.Context) ([]store.Run, error) {
	return s.runs, nil
}

func (s fakeStore) ListAPICaseRuns(_ context.Context, runID string) ([]store.APICaseRun, error) {
	return s.caseRuns[runID], nil
}

func TestStatusByCaseKeepsLatestAndAnyPassed(t *testing.T) {
	passed, latest, err := StatusByCase(context.Background(), fakeStore{
		runs: []store.Run{{ID: "old"}, {ID: "new"}},
		caseRuns: map[string][]store.APICaseRun{
			"old": {
				{CaseID: "case.alpha", Status: store.StatusPassed},
				{CaseID: "case.beta", Status: store.StatusFailed},
			},
			"new": {
				{CaseID: "case.alpha", Status: store.StatusFailed},
				{CaseID: "case.beta", Status: "PASSED"},
			},
		},
	})

	if err != nil {
		t.Fatalf("status by case: %v", err)
	}
	if !passed["case.alpha"] || !passed["case.beta"] {
		t.Fatalf("passed map = %#v", passed)
	}
	if latest["case.alpha"] != store.StatusFailed || latest["case.beta"] != "PASSED" {
		t.Fatalf("latest map = %#v", latest)
	}
}
