package mapplanner_test

import (
	"testing"

	"agent-testbench/internal/domain/mapplanner"
	"agent-testbench/internal/domain/plangraph"
)

func TestExplainBuildsPhysicalTaskDAGForValidationCase(t *testing.T) {
	graph := validationCaseGraph()

	plan, err := mapplanner.Explain(graph, mapplanner.Query{
		MapID:       "map.contract",
		Scope:       mapplanner.ScopeCase,
		TargetKind:  mapplanner.TargetCase,
		TargetID:    "case.submit.missing-days",
		PlannerMode: mapplanner.ModeExplain,
	})
	if err != nil {
		t.Fatalf("explain validation case: %v", err)
	}

	if plan.MapID != "map.contract" || plan.Scope != mapplanner.ScopeCase || plan.TargetKind != mapplanner.TargetCase {
		t.Fatalf("plan identity = %#v", plan)
	}
	if len(plan.LogicalPlan) == 0 || len(plan.RulesApplied) == 0 || len(plan.CandidatePlans) == 0 {
		t.Fatalf("planner should expose logical plan, rules, and candidates: %#v", plan)
	}
	if len(plan.PhysicalTasks) != 2 {
		t.Fatalf("physical tasks = %#v", plan.PhysicalTasks)
	}
	if plan.PhysicalTasks[0].Kind != mapplanner.TaskReuseMaterialized || plan.PhysicalTasks[0].PathID != "workflow.submit.success" || plan.PhysicalTasks[0].UntilNodeID != "case.prepare" || plan.PhysicalTasks[0].MaterializationID != "fixture.prepare" {
		t.Fatalf("materialized prefix task = %#v", plan.PhysicalTasks[0])
	}
	if plan.PhysicalTasks[1].Kind != mapplanner.TaskRunCase || plan.PhysicalTasks[1].CaseID != "case.submit.missing-days" {
		t.Fatalf("case task = %#v", plan.PhysicalTasks[1])
	}
	if len(plan.TaskEdges) != 1 || plan.TaskEdges[0].FromTaskID != plan.PhysicalTasks[0].ID || plan.TaskEdges[0].ToTaskID != plan.PhysicalTasks[1].ID {
		t.Fatalf("task edges = %#v", plan.TaskEdges)
	}
	if plan.Cost.EstimatedTasks != 2 || plan.Cost.ReplayTasks != 0 || plan.Cost.CaseTasks != 1 {
		t.Fatalf("cost = %#v", plan.Cost)
	}
}

func TestExplainPrefersMaterializedReplayAndAddsEvidenceGateTrace(t *testing.T) {
	graph := validationCaseGraph()

	plan, err := mapplanner.Explain(graph, mapplanner.Query{
		MapID:       "map.contract",
		Scope:       mapplanner.ScopeCase,
		TargetKind:  mapplanner.TargetCase,
		TargetID:    "case.submit.missing-days",
		PlannerMode: mapplanner.ModeExplain,
	})
	if err != nil {
		t.Fatalf("explain validation case: %v", err)
	}

	if plan.PhysicalTasks[0].Kind != mapplanner.TaskReuseMaterialized || plan.PhysicalTasks[0].MaterializationID != "fixture.prepare" {
		t.Fatalf("materialized replay task = %#v", plan.PhysicalTasks[0])
	}
	if plan.PhysicalTasks[0].Cost.ReplayTasks != 0 || plan.PhysicalTasks[0].Cost.EstimatedTasks != 1 {
		t.Fatalf("materialized replay should avoid replay cost = %#v", plan.PhysicalTasks[0].Cost)
	}
	if !hasPlannerRule(plan.RulesApplied, "prefer_materialized_replay") || !hasPlannerRule(plan.RulesApplied, "plan_evidence_gate") {
		t.Fatalf("planner rules = %#v", plan.RulesApplied)
	}
}

func TestExplainScopeAllPlansWorkflowAndCaseTasks(t *testing.T) {
	graph := validationCaseGraph()

	plan, err := mapplanner.Explain(graph, mapplanner.Query{
		MapID:       "map.contract",
		Scope:       mapplanner.ScopeAll,
		PlannerMode: mapplanner.ModeExplain,
	})
	if err != nil {
		t.Fatalf("explain all: %v", err)
	}

	if len(plan.PhysicalTasks) != 3 {
		t.Fatalf("all-scope physical tasks = %#v", plan.PhysicalTasks)
	}
	if plan.PhysicalTasks[0].Kind != mapplanner.TaskRunPath || plan.PhysicalTasks[0].PathID != "workflow.submit.success" {
		t.Fatalf("workflow task = %#v", plan.PhysicalTasks[0])
	}
	if plan.PhysicalTasks[1].Kind != mapplanner.TaskReuseMaterialized || plan.PhysicalTasks[2].Kind != mapplanner.TaskRunCase {
		t.Fatalf("validation optimized tasks = %#v", plan.PhysicalTasks)
	}
	if len(plan.TaskEdges) != 1 {
		t.Fatalf("all-scope task edges = %#v", plan.TaskEdges)
	}
	if plan.Summary.WorkflowTasks != 1 || plan.Summary.CaseTasks != 1 || plan.Summary.ReplayTasks != 0 {
		t.Fatalf("summary = %#v", plan.Summary)
	}
}

func TestExplainRejectsUnmatchedWorkflowTarget(t *testing.T) {
	graph := validationCaseGraph()

	_, err := mapplanner.Explain(graph, mapplanner.Query{
		MapID:       "map.contract",
		Scope:       mapplanner.ScopeWorkflows,
		PathID:      "workflow.missing",
		PlannerMode: mapplanner.ModeRun,
	})
	if err == nil {
		t.Fatalf("expected unmatched path target to fail")
	}
}

func TestExplainCasesScopeHonorsConcreteCaseTarget(t *testing.T) {
	graph := validationCaseGraph()

	plan, err := mapplanner.Explain(graph, mapplanner.Query{
		MapID:       "map.contract",
		Scope:       mapplanner.ScopeCases,
		CaseID:      "case.submit.missing-days",
		PlannerMode: mapplanner.ModeRun,
	})
	if err != nil {
		t.Fatalf("explain targeted case under cases scope: %v", err)
	}
	if plan.Scope != mapplanner.ScopeCase || plan.TargetCaseID != "case.submit.missing-days" {
		t.Fatalf("targeted case query should normalize to single case = %#v", plan)
	}
	if len(plan.PhysicalTasks) != 2 {
		t.Fatalf("targeted case should not plan all validation cases = %#v", plan.PhysicalTasks)
	}
}

func hasPlannerRule(rules []mapplanner.RuleTrace, name string) bool {
	for _, rule := range rules {
		if rule.Rule == name && rule.Status == mapplanner.RuleStatusApplied {
			return true
		}
	}
	return false
}

func validationCaseGraph() plangraph.Graph {
	return plangraph.Graph{
		Map: plangraph.Map{ID: "map.contract", ProfileID: "profile.contract", Status: "active"},
		Nodes: []plangraph.Node{
			{MapID: "map.contract", ID: "case.prepare", CaseID: "case.prepare", Role: "primary", StateEffect: "advance", ProvidedPropertyJSON: `{"state":"prepared"}`, SummaryJSON: `{}`, SortOrder: 1},
			{MapID: "map.contract", ID: "case.submit", CaseID: "case.submit", Role: "primary", StateEffect: "advance", AnchorNodeID: "case.prepare", ProvidedPropertyJSON: `{"state":"submitted"}`, SummaryJSON: `{}`, SortOrder: 2},
			{MapID: "map.contract", ID: "case.submit.missing-days", CaseID: "case.submit.missing-days", Role: "validation", StateEffect: "unchanged", BaseCaseID: "case.submit", AnchorNodeID: "case.submit", RequiredPropertyJSON: `{"samePreconditionAsCase":"case.submit"}`, ProvidedPropertyJSON: `{"state":"prepared"}`, SummaryJSON: `{}`, SortOrder: 3},
		},
		Edges: []plangraph.Edge{
			{MapID: "map.contract", ID: "edge.prepare.submit", FromNodeID: "case.prepare", ToNodeID: "case.submit", Kind: "control", PathID: "workflow.submit.success", Required: true, MappingsJSON: `[]`, SummaryJSON: `{}`, SortOrder: 1},
			{MapID: "map.contract", ID: "edge.fixture.missing-days", FromNodeID: "case.prepare", ToNodeID: "case.submit.missing-days", Kind: "fixture", MaterializationID: "fixture.prepare", Required: true, MappingsJSON: `[]`, SummaryJSON: `{}`, SortOrder: 2},
		},
		Paths: []plangraph.Path{
			{MapID: "map.contract", ID: "workflow.submit.success", WorkflowID: "workflow.submit.success", Status: "active", SummaryJSON: `{}`, SortOrder: 1},
		},
		PathSteps: []plangraph.PathStep{
			{MapID: "map.contract", PathID: "workflow.submit.success", StepIndex: 1, StepID: "prepare", NodeID: "case.prepare", CaseID: "case.prepare", Required: true, SummaryJSON: `{}`},
			{MapID: "map.contract", PathID: "workflow.submit.success", StepIndex: 2, StepID: "submit", NodeID: "case.submit", CaseID: "case.submit", Required: true, SummaryJSON: `{}`},
		},
		Materializations: []plangraph.Materialization{
			{MapID: "map.contract", ID: "fixture.prepare", FixtureID: "fixture.prepare", SourcePathID: "workflow.submit.success", SourceWorkflowID: "workflow.submit.success", SourceUntilStep: "prepare", SourceUntilNodeID: "case.prepare", SnapshotKind: "workflow_prefix", Status: "active", SummaryJSON: `{}`, SortOrder: 1},
		},
	}
}
