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
		runs: []store.Run{{ID: "old", ProfileID: "current"}, {ID: "new", ProfileID: "current"}},
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
	}, "current")

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

func TestStatusByCaseKeepsLatestWithinRun(t *testing.T) {
	passed, latest, err := StatusByCase(context.Background(), fakeStore{
		runs: []store.Run{{ID: "run-with-retry", ProfileID: "current"}},
		caseRuns: map[string][]store.APICaseRun{
			"run-with-retry": {
				{CaseID: "case.alpha", Status: store.StatusSkipped},
				{CaseID: "case.alpha", Status: store.StatusFailed},
			},
		},
	}, "current")

	if err != nil {
		t.Fatalf("status by case: %v", err)
	}
	if passed["case.alpha"] {
		t.Fatalf("passed map = %#v", passed)
	}
	if latest["case.alpha"] != store.StatusFailed {
		t.Fatalf("latest map should use the newest case run in the workflow run: %#v", latest)
	}
}

func TestStatusByCaseFiltersRunsByProfile(t *testing.T) {
	passed, latest, err := StatusByCase(context.Background(), fakeStore{
		runs: []store.Run{
			{ID: "other-old", ProfileID: "other"},
			{ID: "current-new", ProfileID: "current"},
			{ID: "other-new", ProfileID: "other"},
		},
		caseRuns: map[string][]store.APICaseRun{
			"other-old": {
				{CaseID: "case.shared", Status: store.StatusPassed},
			},
			"current-new": {
				{CaseID: "case.shared", Status: store.StatusFailed},
			},
			"other-new": {
				{CaseID: "case.current-only", Status: store.StatusPassed},
			},
		},
	}, "current")

	if err != nil {
		t.Fatalf("status by case: %v", err)
	}
	if passed["case.shared"] || passed["case.current-only"] {
		t.Fatalf("passed map should ignore other profile runs: %#v", passed)
	}
	if latest["case.shared"] != store.StatusFailed || latest["case.current-only"] != "" {
		t.Fatalf("latest map should ignore other profile runs: %#v", latest)
	}
}
