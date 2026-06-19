package mapplanner

import (
	"fmt"

	"agent-testbench/internal/domain/plangraph"
)

func Explain(graph plangraph.Graph, query Query) (Plan, error) {
	query = normalizeQuery(graph, query)
	if query.MapID == "" {
		return Plan{}, fmt.Errorf("map id is required")
	}
	if err := validateQueryScopeTarget(query); err != nil {
		return Plan{}, err
	}
	builder := planBuilder{
		graph:             graph,
		query:             query,
		pathByID:          map[string]plangraph.Path{},
		nodeByID:          map[string]plangraph.Node{},
		pathSteps:         map[string][]plangraph.PathStep{},
		replayGroupByKey:  map[string]int{},
		replayTaskByGroup: map[string]string{},
	}
	builder.indexGraph()
	plan := builder.basePlan()
	switch query.Scope {
	case ScopeAll:
		if err := builder.planWorkflowScope(&plan); err != nil {
			return Plan{}, err
		}
		if err := builder.planCaseScope(&plan); err != nil {
			return Plan{}, err
		}
	case ScopeWorkflows:
		if err := builder.planWorkflowScope(&plan); err != nil {
			return Plan{}, err
		}
	case ScopeCases:
		if err := builder.planCaseScope(&plan); err != nil {
			return Plan{}, err
		}
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
	graph             plangraph.Graph
	query             Query
	pathByID          map[string]plangraph.Path
	nodeByID          map[string]plangraph.Node
	pathSteps         map[string][]plangraph.PathStep
	replayGroupByKey  map[string]int
	replayTaskByGroup map[string]string
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
			"scope":            b.query.Scope,
			"targetKind":       b.query.TargetKind,
			"targetId":         b.query.TargetID,
			"environmentId":    b.query.EnvironmentID,
			"interfaceNodeId":  b.query.InterfaceNodeID,
			"validationFamily": b.query.ValidationFamily,
			"role":             b.query.Role,
			"graphFingerprint": GraphFingerprint(b.graph),
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

func (b *planBuilder) planWorkflowScope(plan *Plan) error {
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
	matched := 0
	for _, path := range b.graph.Paths {
		if !b.pathMatchesQuery(path) {
			continue
		}
		matched++
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
	if b.workflowTargeted() && matched == 0 {
		return fmt.Errorf("map workflow target not found: %s %s", b.query.TargetKind, b.query.TargetID)
	}
	return nil
}

func (b *planBuilder) planCaseScope(plan *Plan) error {
	plan.LogicalPlan = append(plan.LogicalPlan, LogicalOp{
		ID:         "logical.validation_cases",
		Op:         "scan_validation_cases",
		TargetKind: TargetMap,
		TargetID:   b.query.MapID,
		Children:   []string{"logical.scan_map"},
	})
	matched := 0
	for _, node := range b.graph.Nodes {
		if !isValidationNode(node) {
			continue
		}
		if !b.validationNodeMatchesFilters(node) {
			continue
		}
		if b.caseTargeted() && !b.nodeMatchesQuery(node) {
			continue
		}
		matched++
		if err := b.planCaseNode(plan, node); err != nil {
			plan.RejectedPlans = append(plan.RejectedPlans, RejectedPlan{
				ID:     "candidate.case." + safeID(node.ID),
				Kind:   "case",
				Reason: err.Error(),
			})
		}
	}
	if b.caseTargeted() && matched == 0 {
		return fmt.Errorf("map case target not found: %s %s", b.query.TargetKind, b.query.TargetID)
	}
	return nil
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
	b.appendCaseCandidates(plan, node, explain)
	b.appendCaseRejections(plan, node, explain)
	taskResult := b.appendCaseTasks(plan, node, explain.Operations)
	b.tagCaseTasksWithReplayGroup(plan, node, taskResult)
	b.appendCaseRuleTraces(plan, node, explain, taskResult)
	plan.Operations = append(plan.Operations, convertOperations(explain.Operations, b.pathByID)...)
	return nil
}

type casePlanTaskResult struct {
	startIndex              int
	materializedReplayTasks int
	caseReplayGroupID       string
}

func (b *planBuilder) appendCaseCandidates(plan *Plan, node plangraph.Node, explain plangraph.Explanation) {
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
}

func (b *planBuilder) appendCaseRejections(plan *Plan, node plangraph.Node, explain plangraph.Explanation) {
	for _, rejected := range explain.RejectedReasons {
		plan.RejectedPlans = append(plan.RejectedPlans, RejectedPlan{
			ID:     "rejected." + safeID(node.ID) + "." + safeID(rejected.PathID),
			Kind:   "case_replay",
			Reason: rejected.Reason,
		})
	}
}

func (b *planBuilder) appendCaseTasks(plan *Plan, node plangraph.Node, operations []plangraph.PhysicalOperation) casePlanTaskResult {
	startIndex := len(plan.PhysicalTasks)
	var previousTaskID string
	materializedReplayTasks := 0
	caseReplayGroupID := ""
	for _, operation := range operations {
		task := b.taskFromOperation(operation)
		groupID, reusedTaskID, reusable := b.prepareReplayGroup(plan, node, task)
		if groupID != "" {
			task.ReplayGroupID = groupID
			task.InterfaceNodeID = node.InterfaceNodeID
			task.AnchorNodeID = node.AnchorNodeID
			task.ValidationFamily = plangraph.ValidationFamilyForNode(node)
			caseReplayGroupID = groupID
		}
		if reusable && reusedTaskID != "" {
			previousTaskID = reusedTaskID
			continue
		}
		b.appendTask(plan, task)
		currentTaskID := plan.PhysicalTasks[len(plan.PhysicalTasks)-1].ID
		if groupID != "" {
			b.addReplayGroupTask(plan, groupID, currentTaskID)
		}
		if task.Kind == TaskReuseMaterialized {
			materializedReplayTasks++
		}
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
	return casePlanTaskResult{
		startIndex:              startIndex,
		materializedReplayTasks: materializedReplayTasks,
		caseReplayGroupID:       caseReplayGroupID,
	}
}

func (b *planBuilder) tagCaseTasksWithReplayGroup(plan *Plan, node plangraph.Node, result casePlanTaskResult) {
	if result.caseReplayGroupID != "" && len(plan.PhysicalTasks) > result.startIndex {
		for index := result.startIndex; index < len(plan.PhysicalTasks); index++ {
			task := &plan.PhysicalTasks[index]
			if task.Kind != TaskRunCase || task.ReplayGroupID != "" {
				continue
			}
			task.ReplayGroupID = result.caseReplayGroupID
			task.InterfaceNodeID = node.InterfaceNodeID
			task.AnchorNodeID = node.AnchorNodeID
			task.ValidationFamily = plangraph.ValidationFamilyForNode(node)
			b.addReplayGroupTask(plan, result.caseReplayGroupID, task.ID)
		}
	}
}

func (b *planBuilder) appendCaseRuleTraces(plan *Plan, node plangraph.Node, explain plangraph.Explanation, result casePlanTaskResult) {
	if len(plan.PhysicalTasks) > result.startIndex {
		if result.materializedReplayTasks > 0 {
			plan.RulesApplied = append(plan.RulesApplied, RuleTrace{
				Rule:   "prefer_materialized_replay",
				Status: RuleStatusApplied,
				Before: "run_path_prefix",
				After:  TaskReuseMaterialized,
				Reason: "reuse Store-backed materialized precondition instead of replaying the full prefix",
				Details: map[string]any{
					"nodeId": node.ID,
					"caseId": node.CaseID,
					"tasks":  result.materializedReplayTasks,
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
				"tasks":  len(plan.PhysicalTasks) - result.startIndex,
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
}

func (b *planBuilder) finish(plan *Plan) {
	plan.OptimizedPlan = append([]LogicalOp(nil), plan.LogicalPlan...)
	if len(plan.ReplayGroups) > 0 {
		plan.OptimizedPlan = append(plan.OptimizedPlan, LogicalOp{
			ID:         "logical.replay_groups",
			Op:         "group_replay_checkpoints",
			TargetKind: TargetMap,
			TargetID:   b.query.MapID,
			Children:   []string{"logical.validation_cases"},
			Properties: map[string]any{
				"groups": len(plan.ReplayGroups),
			},
		})
		plan.RulesApplied = append(plan.RulesApplied, RuleTrace{
			Rule:   "group_replay_checkpoints",
			Status: RuleStatusApplied,
			Reason: "validation cases sharing interface, anchor, family, and replay source are grouped for reuse",
			Details: map[string]any{
				"groups": len(plan.ReplayGroups),
			},
		})
	}
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
