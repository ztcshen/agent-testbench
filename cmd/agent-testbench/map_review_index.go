package main

import (
	"sort"

	"agent-testbench/internal/store"
)

func mapReviewCasesByID(cases []store.CatalogAPICase) map[string]store.CatalogAPICase {
	out := map[string]store.CatalogAPICase{}
	for _, item := range cases {
		out[item.ID] = item
	}
	return out
}

func mapReviewTemplatesByID(templates []store.CatalogRequestTemplate) map[string]mapReviewRequestTemplate {
	out := map[string]mapReviewRequestTemplate{}
	for _, item := range templates {
		out[item.ID] = mapReviewRequestTemplate{
			ID:           item.ID,
			DisplayName:  item.DisplayName,
			NodeID:       item.NodeID,
			Method:       item.Method,
			Path:         item.Path,
			TemplateJSON: item.TemplateJSON,
			Version:      item.Version,
			Status:       item.Status,
		}
	}
	return out
}

func mapReviewPathsByID(paths []store.TestPlanPath) map[string]store.TestPlanPath {
	out := map[string]store.TestPlanPath{}
	for _, item := range paths {
		out[item.ID] = item
	}
	return out
}

func mapReviewStepsByPath(steps []store.TestPlanPathStep) map[string][]store.TestPlanPathStep {
	out := map[string][]store.TestPlanPathStep{}
	for _, step := range steps {
		out[step.PathID] = append(out[step.PathID], step)
	}
	for pathID := range out {
		sort.SliceStable(out[pathID], func(i, j int) bool {
			return out[pathID][i].StepIndex < out[pathID][j].StepIndex
		})
	}
	return out
}

func mapReviewUsageByNode(steps []store.TestPlanPathStep, pathByID map[string]store.TestPlanPath) map[string][]mapReviewNodePath {
	out := map[string][]mapReviewNodePath{}
	for _, step := range steps {
		path := pathByID[step.PathID]
		out[step.NodeID] = append(out[step.NodeID], mapReviewNodePath{
			PathID:      step.PathID,
			WorkflowID:  path.WorkflowID,
			DisplayName: path.DisplayName,
			StepIndex:   step.StepIndex,
			StepID:      step.StepID,
			Required:    step.Required,
		})
	}
	for nodeID := range out {
		sort.SliceStable(out[nodeID], func(i, j int) bool {
			if out[nodeID][i].PathID == out[nodeID][j].PathID {
				return out[nodeID][i].StepIndex < out[nodeID][j].StepIndex
			}
			return out[nodeID][i].PathID < out[nodeID][j].PathID
		})
	}
	return out
}
