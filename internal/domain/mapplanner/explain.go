package mapplanner

import (
	"encoding/json"
	"fmt"
	"strings"

	"agent-testbench/internal/domain/plangraph"
)

func Explain(graph plangraph.Graph, query Query) (Plan, error) {
	query = normalizeQuery(graph, query)
	if query.MapID == "" {
		return Plan{}, fmt.Errorf("map id is required")
	}
	builder := planBuilder{
		graph:     graph,
		query:     query,
		pathByID:  map[string]plangraph.Path{},
		nodeByID:  map[string]plangraph.Node{},
		pathSteps: map[string][]plangraph.PathStep{},
	}
	builder.indexGraph()
	plan := builder.basePlan()
	switch query.Scope {
	case ScopeAll:
		builder.planWorkflowScope(&plan)
		builder.planCaseScope(&plan)
	case ScopeWorkflows:
		builder.planWorkflowScope(&plan)
	case ScopeCases:
		builder.planCaseScope(&plan)
	case ScopeCase:
		if err := builder.planTargetCase(&plan); err != nil {
			return Plan{}, err
		}
	default:
		return Plan{}, fmt.Errorf("unsupported map planner scope: %s", query.Scope)
	}
	builder.finish(&plan)
	return plan, nil
}

type planBuilder struct {
	graph     plangraph.Graph
	query     Query
	pathByID  map[string]plangraph.Path
	nodeByID  map[string]plangraph.Node
	pathSteps map[string][]plangraph.PathStep
}

func (b *planBuilder) indexGraph() {
	for _, path := range b.graph.Paths {
		b.pathByID[path.ID] = path
	}
	for _, node := range b.graph.Nodes {
		b.nodeByID[node.ID] = node
	}
	for _, step := range b.graph.PathSteps {
		b.pathSteps[step.PathID] = append(b.pathSteps[step.PathID], step)
	}
}

func (b *planBuilder) basePlan() Plan {
	return Plan{
		MapID:          b.query.MapID,
		ProfileID:      b.graph.Map.ProfileID,
		EnvironmentID:  b.query.EnvironmentID,
		Scope:          b.query.Scope,
		TargetKind:     b.query.TargetKind,
		TargetID:       b.query.TargetID,
		Mode:           stringDefault(b.query.PlannerMode, ModeExplain),
		Status:         TaskStatusPlanned,
		PlannerVersion: PlannerVersion,
		PlannerOptions: map[string]any{
			"scope":         b.query.Scope,
			"targetKind":    b.query.TargetKind,
			"targetId":      b.query.TargetID,
			"environmentId": b.query.EnvironmentID,
		},
		LogicalPlan: []LogicalOp{{
			ID:         "logical.scan_map",
			Op:         "scan_map",
			TargetKind: TargetMap,
			TargetID:   b.query.MapID,
			Properties: map[string]any{
				"paths": len(b.graph.Paths),
				"nodes": len(b.graph.Nodes),
			},
		}},
		RulesApplied: []RuleTrace{{
			Rule:   "build_logical_map_plan",
			Status: RuleStatusApplied,
			Reason: "load Store-backed map graph into planner input",
		}},
		RequiredProperties: map[string]any{},
		ProvidedProperties: map[string]any{},
	}
}

func (b *planBuilder) planWorkflowScope(plan *Plan) {
	plan.LogicalPlan = append(plan.LogicalPlan, LogicalOp{
		ID:         "logical.workflow_paths",
		Op:         "scan_workflow_paths",
		TargetKind: TargetMap,
		TargetID:   b.query.MapID,
		Children:   []string{"logical.scan_map"},
		Properties: map[string]any{"pathCount": len(b.graph.Paths)},
	})
	plan.RulesApplied = append(plan.RulesApplied, RuleTrace{
		Rule:   "select_workflow_paths",
		Status: RuleStatusApplied,
		Reason: "plan each mapped workflow path as an executable physical task",
	})
	for _, path := range b.graph.Paths {
		if !b.pathMatchesQuery(path) {
			continue
		}
		steps := b.pathSteps[path.ID]
		candidate := CandidatePlan{
			ID:         "candidate.path." + safeID(path.ID),
			Kind:       "workflow_path",
			PathID:     path.ID,
			WorkflowID: path.WorkflowID,
			Selected:   true,
			Cost:       PlanCost{EstimatedTasks: 1, WorkflowTasks: 1, TotalSteps: len(steps)},
			Reason:     "mapped workflow path selected by scope",
		}
		plan.CandidatePlans = append(plan.CandidatePlans, candidate)
		taskKind := TaskRunPath
		status := TaskStatusPlanned
		reason := "run mapped workflow path"
		if len(steps) == 0 {
			taskKind = TaskSkip
			status = TaskStatusSkipped
			reason = "workflow path has no mapped steps"
		}
		b.appendTask(plan, PhysicalTask{
			Kind:               taskKind,
			Operation:          taskKind,
			PathID:             path.ID,
			WorkflowID:         path.WorkflowID,
			Status:             status,
			Reason:             reason,
			Cost:               PlanCost{EstimatedTasks: 1, WorkflowTasks: 1, TotalSteps: len(steps)},
			RequiredProperties: jsonObject(path.RequiredPropertyJSON),
			ProvidedProperties: jsonObject(path.ProvidedPropertyJSON),
		})
	}
}

func (b *planBuilder) planCaseScope(plan *Plan) {
	plan.LogicalPlan = append(plan.LogicalPlan, LogicalOp{
		ID:         "logical.validation_cases",
		Op:         "scan_validation_cases",
		TargetKind: TargetMap,
		TargetID:   b.query.MapID,
		Children:   []string{"logical.scan_map"},
	})
	for _, node := range b.graph.Nodes {
		if !isValidationNode(node) {
			continue
		}
		if err := b.planCaseNode(plan, node); err != nil {
			plan.RejectedPlans = append(plan.RejectedPlans, RejectedPlan{
				ID:     "candidate.case." + safeID(node.ID),
				Kind:   "case",
				Reason: err.Error(),
			})
		}
	}
}

func (b *planBuilder) planTargetCase(plan *Plan) error {
	target, ok := b.findTargetNode()
	if !ok {
		return fmt.Errorf("plan graph node not found: %s", b.query.TargetID)
	}
	plan.TargetNodeID = target.ID
	plan.TargetCaseID = target.CaseID
	plan.RequiredProperties = jsonObject(target.RequiredPropertyJSON)
	plan.ProvidedProperties = jsonObject(target.ProvidedPropertyJSON)
	return b.planCaseNode(plan, target)
}

func (b *planBuilder) planCaseNode(plan *Plan, node plangraph.Node) error {
	explain, err := plangraph.ExplainCase(b.graph, plangraph.ExplainOptions{CaseID: node.CaseID, NodeID: node.ID})
	if err != nil {
		return err
	}
	plan.TargetNodeID = firstNonEmpty(plan.TargetNodeID, explain.TargetNodeID)
	plan.TargetCaseID = firstNonEmpty(plan.TargetCaseID, explain.TargetCaseID)
	for _, candidate := range explain.CandidatePaths {
		plan.CandidatePlans = append(plan.CandidatePlans, CandidatePlan{
			ID:         "candidate.case." + safeID(node.ID) + "." + safeID(candidate.PathID),
			Kind:       "case_replay",
			PathID:     candidate.PathID,
			WorkflowID: candidate.WorkflowID,
			NodeID:     node.ID,
			CaseID:     node.CaseID,
			Selected:   candidate.Selected,
			Cost:       PlanCost{EstimatedTasks: len(explain.Operations), ReplayTasks: replayOperationCount(explain.Operations), CaseTasks: caseOperationCount(explain.Operations), TotalSteps: len(explain.PathSteps)},
			Reason:     candidate.Reason,
		})
	}
	for _, rejected := range explain.RejectedReasons {
		plan.RejectedPlans = append(plan.RejectedPlans, RejectedPlan{
			ID:     "rejected." + safeID(node.ID) + "." + safeID(rejected.PathID),
			Kind:   "case_replay",
			Reason: rejected.Reason,
		})
	}
	startIndex := len(plan.PhysicalTasks)
	var previousTaskID string
	materializedReplayTasks := 0
	for _, operation := range explain.Operations {
		task := b.taskFromOperation(operation)
		b.appendTask(plan, task)
		if task.Kind == TaskReuseMaterialized {
			materializedReplayTasks++
		}
		currentTaskID := plan.PhysicalTasks[len(plan.PhysicalTasks)-1].ID
		if previousTaskID != "" {
			plan.TaskEdges = append(plan.TaskEdges, TaskEdge{
				FromTaskID: previousTaskID,
				ToTaskID:   currentTaskID,
				Kind:       "control",
				Required:   true,
				SortOrder:  len(plan.TaskEdges) + 1,
			})
		}
		previousTaskID = currentTaskID
	}
	if len(plan.PhysicalTasks) > startIndex {
		if materializedReplayTasks > 0 {
			plan.RulesApplied = append(plan.RulesApplied, RuleTrace{
				Rule:   "prefer_materialized_replay",
				Status: RuleStatusApplied,
				Before: "run_path_prefix",
				After:  TaskReuseMaterialized,
				Reason: "reuse Store-backed materialized precondition instead of replaying the full prefix",
				Details: map[string]any{
					"nodeId": node.ID,
					"caseId": node.CaseID,
					"tasks":  materializedReplayTasks,
				},
			})
		}
		plan.RulesApplied = append(plan.RulesApplied, RuleTrace{
			Rule:   "choose_replay_prefix",
			Status: RuleStatusApplied,
			Reason: "selected physical replay operations for validation case precondition",
			Details: map[string]any{
				"nodeId": node.ID,
				"caseId": node.CaseID,
				"tasks":  len(plan.PhysicalTasks) - startIndex,
			},
		})
		if caseOperationCount(explain.Operations) > 0 {
			plan.RulesApplied = append(plan.RulesApplied, RuleTrace{
				Rule:   "plan_evidence_gate",
				Status: RuleStatusApplied,
				Reason: "map gate can validate task Evidence after execution without changing the physical request plan",
				Details: map[string]any{
					"nodeId": node.ID,
					"caseId": node.CaseID,
				},
			})
		}
	}
	plan.Operations = append(plan.Operations, convertOperations(explain.Operations, b.pathByID)...)
	return nil
}

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

func (b *planBuilder) finish(plan *Plan) {
	plan.OptimizedPlan = append([]LogicalOp(nil), plan.LogicalPlan...)
	plan.RulesApplied = append(plan.RulesApplied, RuleTrace{
		Rule:   "build_physical_task_dag",
		Status: RuleStatusApplied,
		Reason: "lower optimized logical plan into executable task DAG",
		Details: map[string]any{
			"tasks": len(plan.PhysicalTasks),
			"edges": len(plan.TaskEdges),
		},
	})
	for _, task := range plan.PhysicalTasks {
		plan.Cost.EstimatedTasks++
		plan.Cost.WorkflowTasks += task.Cost.WorkflowTasks
		plan.Cost.ReplayTasks += task.Cost.ReplayTasks
		plan.Cost.CaseTasks += task.Cost.CaseTasks
		plan.Cost.SkippedTasks += task.Cost.SkippedTasks
		plan.Cost.TotalSteps += task.Cost.TotalSteps
		if task.Kind == TaskSkip {
			plan.Cost.SkippedTasks++
		}
	}
	plan.Summary = PlanSummary{
		WorkflowTasks: plan.Cost.WorkflowTasks,
		ReplayTasks:   plan.Cost.ReplayTasks,
		CaseTasks:     plan.Cost.CaseTasks,
		SkippedTasks:  plan.Cost.SkippedTasks,
		TotalTasks:    len(plan.PhysicalTasks),
		TotalSteps:    plan.Cost.TotalSteps,
	}
}

func normalizeQuery(graph plangraph.Graph, query Query) Query {
	query.MapID = firstNonEmpty(strings.TrimSpace(query.MapID), graph.Map.ID)
	query.PlannerMode = firstNonEmpty(strings.TrimSpace(query.PlannerMode), ModeExplain)
	query.Scope = strings.TrimSpace(query.Scope)
	query.TargetKind = strings.TrimSpace(query.TargetKind)
	query.TargetID = strings.TrimSpace(query.TargetID)
	switch {
	case strings.TrimSpace(query.CaseID) != "":
		query.Scope = stringDefault(query.Scope, ScopeCase)
		query.TargetKind = stringDefault(query.TargetKind, TargetCase)
		query.TargetID = stringDefault(query.TargetID, strings.TrimSpace(query.CaseID))
	case strings.TrimSpace(query.NodeID) != "":
		query.Scope = stringDefault(query.Scope, ScopeCase)
		query.TargetKind = stringDefault(query.TargetKind, TargetNode)
		query.TargetID = stringDefault(query.TargetID, strings.TrimSpace(query.NodeID))
	case strings.TrimSpace(query.PathID) != "":
		query.Scope = stringDefault(query.Scope, ScopeWorkflows)
		query.TargetKind = stringDefault(query.TargetKind, TargetPath)
		query.TargetID = stringDefault(query.TargetID, strings.TrimSpace(query.PathID))
	case strings.TrimSpace(query.WorkflowID) != "":
		query.Scope = stringDefault(query.Scope, ScopeWorkflows)
		query.TargetKind = stringDefault(query.TargetKind, TargetWorkflow)
		query.TargetID = stringDefault(query.TargetID, strings.TrimSpace(query.WorkflowID))
	default:
		query.Scope = stringDefault(query.Scope, ScopeAll)
		query.TargetKind = stringDefault(query.TargetKind, TargetMap)
		query.TargetID = stringDefault(query.TargetID, query.MapID)
	}
	if query.Scope == "workflow" {
		query.Scope = ScopeWorkflows
	}
	return query
}

func (b *planBuilder) pathMatchesQuery(path plangraph.Path) bool {
	switch b.query.TargetKind {
	case TargetPath:
		return path.ID == b.query.TargetID
	case TargetWorkflow:
		return path.WorkflowID == b.query.TargetID
	default:
		return true
	}
}

func (b *planBuilder) findTargetNode() (plangraph.Node, bool) {
	for _, node := range b.graph.Nodes {
		if b.query.TargetKind == TargetNode && node.ID == b.query.TargetID {
			return node, true
		}
		if b.query.TargetKind == TargetCase && node.CaseID == b.query.TargetID {
			return node, true
		}
	}
	return plangraph.Node{}, false
}

func isValidationNode(node plangraph.Node) bool {
	return node.Role == plangraph.NodeRoleValidation || node.StateEffect == plangraph.StateEffectUnchanged
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

func jsonObject(raw string) map[string]any {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return map[string]any{}
	}
	var out map[string]any
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return map[string]any{"raw": raw}
	}
	return out
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func safeID(value string) string {
	value = strings.TrimSpace(value)
	var out strings.Builder
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			out.WriteRune(r)
			continue
		}
		out.WriteByte('_')
	}
	if out.Len() == 0 {
		return "unknown"
	}
	return out.String()
}
