// Package apicaserunstate derives per-case status from stored API Case runs.
package apicaserunstate

import (
	"context"
	"strings"

	"agent-testbench/internal/store"
)

type Store interface {
	ListRuns(ctx context.Context) ([]store.Run, error)
	ListAPICaseRuns(ctx context.Context, runID string) ([]store.APICaseRun, error)
}

func StatusByCase(ctx context.Context, runtime Store, profileID string) (map[string]bool, map[string]string, error) {
	passed := map[string]bool{}
	latest := map[string]string{}
	profileID = strings.TrimSpace(profileID)
	runs, err := runtime.ListRuns(ctx)
	if err != nil {
		return nil, nil, err
	}
	for i := len(runs) - 1; i >= 0; i-- {
		if profileID != "" && strings.TrimSpace(runs[i].ProfileID) != profileID {
			continue
		}
		caseRuns, err := runtime.ListAPICaseRuns(ctx, runs[i].ID)
		if err != nil {
			return nil, nil, err
		}
		for _, item := range caseRuns {
			if latest[item.CaseID] == "" {
				latest[item.CaseID] = item.Status
			}
			if strings.EqualFold(item.Status, store.StatusPassed) {
				passed[item.CaseID] = true
			}
		}
	}
	return passed, latest, nil
}
