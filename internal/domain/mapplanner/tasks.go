package mapplanner

import (
	"fmt"
	"strings"

	"agent-testbench/internal/domain/plangraph"
)

func (b *planBuilder) taskFromOperation(operation plangraph.PhysicalOperation) PhysicalTask {
	kind := operation.Kind
	if kind == "" {
		kind = TaskRunCase
	}
	if kind == TaskRunPathPrefix && strings.TrimSpace(operation.MaterializationID) != "" {
		kind = TaskReuseMaterialized
	}
	path := b.pathByID[operation.PathID]
	task := PhysicalTask{
		Kind:               kind,
		Operation:          kind,
		PathID:             operation.PathID,
		WorkflowID:         path.WorkflowID,
		UntilNodeID:        operation.UntilNodeID,
		NodeID:             operation.NodeID,
		CaseID:             operation.CaseID,
		MaterializationID:  operation.MaterializationID,
		Status:             TaskStatusPlanned,
		Reason:             operation.Reason,
		RequiredProperties: jsonObject(operation.RequiredPropertyJSON),
		ProvidedProperties: jsonObject(operation.ProvidedPropertyJSON),
	}
	switch kind {
	case TaskRunPathPrefix:
		task.Cost = PlanCost{EstimatedTasks: 1, ReplayTasks: 1, TotalSteps: len(prefixSteps(b.pathSteps[operation.PathID], operation.UntilNodeID))}
	case TaskReuseMaterialized:
		task.Cost = PlanCost{EstimatedTasks: 1}
		task.Summary = map[string]any{
			"sourcePathId":      operation.PathID,
			"sourceWorkflowId":  path.WorkflowID,
			"sourceUntilNodeId": operation.UntilNodeID,
		}
	case TaskRunCase:
		task.Cost = PlanCost{EstimatedTasks: 1, CaseTasks: 1}
	default:
		task.Cost = PlanCost{EstimatedTasks: 1}
	}
	return task
}

func (b *planBuilder) appendTask(plan *Plan, task PhysicalTask) {
	task.Index = len(plan.PhysicalTasks) + 1
	if task.ID == "" {
		task.ID = fmt.Sprintf("task.%03d.%s", task.Index, safeID(firstNonEmpty(task.Kind, task.Operation)))
	}
	if task.Operation == "" {
		task.Operation = task.Kind
	}
	if task.Status == "" {
		task.Status = TaskStatusPlanned
	}
	plan.PhysicalTasks = append(plan.PhysicalTasks, task)
}

func replayOperationCount(operations []plangraph.PhysicalOperation) int {
	count := 0
	for _, operation := range operations {
		if operation.Kind == TaskRunPathPrefix && strings.TrimSpace(operation.MaterializationID) == "" {
			count++
		}
	}
	return count
}

func caseOperationCount(operations []plangraph.PhysicalOperation) int {
	count := 0
	for _, operation := range operations {
		if operation.Kind == TaskRunCase {
			count++
		}
	}
	return count
}

func prefixSteps(steps []plangraph.PathStep, untilNodeID string) []plangraph.PathStep {
	if untilNodeID == "" {
		return steps
	}
	for i, step := range steps {
		if step.NodeID == untilNodeID {
			return steps[:i+1]
		}
	}
	return nil
}

func convertOperations(operations []plangraph.PhysicalOperation, pathByID map[string]plangraph.Path) []PhysicalOperation {
	out := make([]PhysicalOperation, 0, len(operations))
	for _, operation := range operations {
		path := pathByID[operation.PathID]
		out = append(out, PhysicalOperation{
			Kind:                 operation.Kind,
			PathID:               operation.PathID,
			WorkflowID:           path.WorkflowID,
			UntilNodeID:          operation.UntilNodeID,
			NodeID:               operation.NodeID,
			CaseID:               operation.CaseID,
			MaterializationID:    operation.MaterializationID,
			Reason:               operation.Reason,
			PatchJSON:            operation.PatchJSON,
			RequiredPropertyJSON: operation.RequiredPropertyJSON,
			ProvidedPropertyJSON: operation.ProvidedPropertyJSON,
		})
	}
	return out
}
