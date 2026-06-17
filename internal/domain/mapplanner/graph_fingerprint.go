package mapplanner

import (
	"crypto/sha1"
	"encoding/json"
	"fmt"
	"sort"

	"agent-testbench/internal/domain/plangraph"
)

func GraphFingerprint(graph plangraph.Graph) string {
	type pathItem struct {
		ID         string `json:"id"`
		WorkflowID string `json:"workflowId"`
		Status     string `json:"status"`
	}
	type stepItem struct {
		PathID   string `json:"pathId"`
		Index    int    `json:"index"`
		StepID   string `json:"stepId"`
		NodeID   string `json:"nodeId"`
		CaseID   string `json:"caseId"`
		Required bool   `json:"required"`
		Material bool   `json:"materializeAfter"`
	}
	type materializationItem struct {
		ID                string `json:"id"`
		SourcePathID      string `json:"sourcePathId"`
		SourceWorkflowID  string `json:"sourceWorkflowId"`
		SourceUntilStep   string `json:"sourceUntilStep"`
		SourceUntilNodeID string `json:"sourceUntilNodeId"`
		Status            string `json:"status"`
	}
	payload := struct {
		MapID            string                `json:"mapId"`
		Paths            []pathItem            `json:"paths"`
		Steps            []stepItem            `json:"steps"`
		Materializations []materializationItem `json:"materializations"`
	}{MapID: graph.Map.ID}
	for _, path := range graph.Paths {
		payload.Paths = append(payload.Paths, pathItem{ID: path.ID, WorkflowID: path.WorkflowID, Status: path.Status})
	}
	for _, step := range graph.PathSteps {
		payload.Steps = append(payload.Steps, stepItem{
			PathID:   step.PathID,
			Index:    step.StepIndex,
			StepID:   step.StepID,
			NodeID:   step.NodeID,
			CaseID:   step.CaseID,
			Required: step.Required,
			Material: step.MaterializeAfter,
		})
	}
	for _, item := range graph.Materializations {
		payload.Materializations = append(payload.Materializations, materializationItem{
			ID:                item.ID,
			SourcePathID:      item.SourcePathID,
			SourceWorkflowID:  item.SourceWorkflowID,
			SourceUntilStep:   item.SourceUntilStep,
			SourceUntilNodeID: item.SourceUntilNodeID,
			Status:            item.Status,
		})
	}
	sort.Slice(payload.Paths, func(i, j int) bool { return payload.Paths[i].ID < payload.Paths[j].ID })
	sort.Slice(payload.Steps, func(i, j int) bool {
		if payload.Steps[i].PathID != payload.Steps[j].PathID {
			return payload.Steps[i].PathID < payload.Steps[j].PathID
		}
		if payload.Steps[i].Index != payload.Steps[j].Index {
			return payload.Steps[i].Index < payload.Steps[j].Index
		}
		return payload.Steps[i].StepID < payload.Steps[j].StepID
	})
	sort.Slice(payload.Materializations, func(i, j int) bool { return payload.Materializations[i].ID < payload.Materializations[j].ID })
	raw, err := json.Marshal(payload)
	if err != nil {
		return ""
	}
	sum := sha1.Sum(raw)
	return fmt.Sprintf("sha1:%x", sum[:])
}
