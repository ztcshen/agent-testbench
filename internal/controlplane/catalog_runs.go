package controlplane

import (
	"context"

	"open-test-sandbox/internal/profile"
	"open-test-sandbox/internal/store"
)

func catalogPayloadFromBundleWithStore(ctx context.Context, bundle profile.Bundle, runtime store.Store) (catalogPayload, error) {
	payload := catalogPayloadFromBundle(bundle)
	if runtime == nil {
		return payload, nil
	}
	runs, err := runtime.ListRuns(ctx)
	if err != nil {
		return catalogPayload{}, err
	}
	byWorkflow := catalogWorkflowRuns(runs)
	for i := range payload.Workflows {
		state := byWorkflow[payload.Workflows[i].ID]
		payload.Workflows[i].RunCount = state.Count
		payload.Workflows[i].LatestRun = state.Latest
	}
	return payload, nil
}

type catalogWorkflowRunState struct {
	Count  int
	Latest map[string]any
}

func catalogWorkflowRuns(runs []store.Run) map[string]catalogWorkflowRunState {
	byWorkflow := map[string]catalogWorkflowRunState{}
	for i := len(runs) - 1; i >= 0; i-- {
		run := runs[i]
		state := byWorkflow[run.WorkflowID]
		state.Count++
		if state.Latest == nil {
			state.Latest = workflowRunListItem(run)
		}
		byWorkflow[run.WorkflowID] = state
	}
	return byWorkflow
}
