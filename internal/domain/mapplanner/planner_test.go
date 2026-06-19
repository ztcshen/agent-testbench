package mapplanner_test

import (
	"strings"
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

func TestExplainGroupsValidationCasesByReusableReplayCheckpoint(t *testing.T) {
	graph := validationCaseGraph()
	graph.Nodes[1].InterfaceNodeID = "interface.submit"
	graph.Nodes[2].InterfaceNodeID = "interface.submit"
	graph.Nodes[2].SummaryJSON = `{"displayName":"days required"}`
	graph.Nodes = append(graph.Nodes, plangraph.Node{
		MapID:                "map.contract",
		ID:                   "case.submit.amount.required",
		CaseID:               "case.submit.amount.required",
		InterfaceNodeID:      "interface.submit",
		Role:                 "validation",
		StateEffect:          "unchanged",
		BaseCaseID:           "case.submit",
		AnchorNodeID:         "case.submit",
		RequiredPropertyJSON: `{"samePreconditionAsCase":"case.submit"}`,
		ProvidedPropertyJSON: `{"state":"prepared"}`,
		SummaryJSON:          `{"displayName":"amount required"}`,
		SortOrder:            4,
	})
	graph.Edges = append(graph.Edges, plangraph.Edge{
		MapID:             "map.contract",
		ID:                "edge.fixture.amount-required",
		FromNodeID:        "case.prepare",
		ToNodeID:          "case.submit.amount.required",
		Kind:              "fixture",
		MaterializationID: "fixture.prepare",
		Required:          true,
		MappingsJSON:      `[]`,
		SummaryJSON:       `{}`,
		SortOrder:         3,
	})

	plan, err := mapplanner.Explain(graph, mapplanner.Query{
		MapID:            "map.contract",
		Scope:            mapplanner.ScopeCases,
		InterfaceNodeID:  "interface.submit",
		ValidationFamily: "empty/null",
		PlannerMode:      mapplanner.ModeExplain,
	})
	if err != nil {
		t.Fatalf("explain grouped validation cases: %v", err)
	}

	if len(plan.ReplayGroups) != 1 {
		t.Fatalf("replay groups = %#v", plan.ReplayGroups)
	}
	group := plan.ReplayGroups[0]
	if group.InterfaceNodeID != "interface.submit" || group.AnchorNodeID != "case.submit" || group.ValidationFamily != "empty/null" || group.MaterializationID != "fixture.prepare" || group.Decision != "reused" || group.Count != 2 {
		t.Fatalf("replay group = %#v", group)
	}
	if len(group.CaseIDs) != 2 || len(group.TaskIDs) != 3 {
		t.Fatalf("replay group members = %#v", group)
	}
	reuseTasks := 0
	caseTasks := 0
	for _, task := range plan.PhysicalTasks {
		switch task.Kind {
		case mapplanner.TaskReuseMaterialized:
			reuseTasks++
			if task.ReplayGroupID != group.ID {
				t.Fatalf("reuse task missing replay group = %#v", task)
			}
		case mapplanner.TaskRunCase:
			caseTasks++
			if task.ReplayGroupID != group.ID {
				t.Fatalf("case task missing replay group = %#v", task)
			}
		}
	}
	if reuseTasks != 1 || caseTasks != 2 {
		t.Fatalf("planner should reuse one checkpoint for two validation cases, tasks=%#v", plan.PhysicalTasks)
	}
}

func TestExplainSeparatesReplayGroupsByReplaySource(t *testing.T) {
	graph := validationCaseGraph()
	graph.Paths = append(graph.Paths, plangraph.Path{MapID: "map.contract", ID: "a", WorkflowID: "workflow.a", Status: "active", SummaryJSON: `{}`, SortOrder: 2})
	graph.PathSteps = append(graph.PathSteps,
		plangraph.PathStep{MapID: "map.contract", PathID: "a_b", StepIndex: 1, StepID: "c", NodeID: "c", CaseID: "case.c", Required: true, SummaryJSON: `{}`},
		plangraph.PathStep{MapID: "map.contract", PathID: "a", StepIndex: 1, StepID: "b", NodeID: "b", CaseID: "case.b", Required: true, SummaryJSON: `{}`},
	)
	graph.Paths[0].ID = "a_b"
	graph.Paths[0].WorkflowID = "workflow.a_b"
	graph.Edges[0].PathID = "a_b"
	graph.PathSteps[0].PathID = "a_b"
	graph.Materializations[0].ID = "m"
	graph.Materializations[0].FixtureID = "m"
	graph.Materializations[0].SourcePathID = "a_b"
	graph.Materializations[0].SourceWorkflowID = "workflow.a_b"
	graph.Materializations[0].SourceUntilStep = "c"
	graph.Materializations[0].SourceUntilNodeID = "c"
	graph.Edges[1].MaterializationID = "m"
	graph.Nodes[2].InterfaceNodeID = "interface.submit"
	graph.Nodes[2].SummaryJSON = `{"validationFamily":"empty/null"}`
	graph.Nodes = append(graph.Nodes, plangraph.Node{
		MapID:                "map.contract",
		ID:                   "case.submit.missing-amount",
		CaseID:               "case.submit.missing-amount",
		Role:                 "validation",
		StateEffect:          "unchanged",
		BaseCaseID:           "case.submit",
		AnchorNodeID:         "case.submit",
		InterfaceNodeID:      "interface.submit",
		RequiredPropertyJSON: `{"samePreconditionAsCase":"case.submit"}`,
		ProvidedPropertyJSON: `{"state":"prepared"}`,
		SummaryJSON:          `{"validationFamily":"empty/null"}`,
		SortOrder:            4,
	})
	graph.Edges = append(graph.Edges, plangraph.Edge{
		MapID:             "map.contract",
		ID:                "edge.fixture.amount",
		FromNodeID:        "case.prepare",
		ToNodeID:          "case.submit.missing-amount",
		Kind:              "fixture",
		MaterializationID: "c_m",
		Required:          true,
		MappingsJSON:      `[]`,
		SummaryJSON:       `{}`,
		SortOrder:         3,
	})
	graph.Materializations = append(graph.Materializations, plangraph.Materialization{
		MapID:             "map.contract",
		ID:                "c_m",
		FixtureID:         "c_m",
		SourcePathID:      "a",
		SourceWorkflowID:  "workflow.a",
		SourceUntilStep:   "b",
		SourceUntilNodeID: "b",
		SnapshotKind:      "workflow_prefix",
		Status:            "active",
		SummaryJSON:       `{}`,
		SortOrder:         2,
	})

	plan, err := mapplanner.Explain(graph, mapplanner.Query{
		MapID:            "map.contract",
		Scope:            mapplanner.ScopeCases,
		InterfaceNodeID:  "interface.submit",
		ValidationFamily: "empty/null",
		PlannerMode:      mapplanner.ModeExplain,
	})
	if err != nil {
		t.Fatalf("explain validation replay source groups: %v", err)
	}

	groupIDs := map[string]bool{}
	for _, group := range plan.ReplayGroups {
		if groupIDs[group.ID] {
			t.Fatalf("replay group id should include replay source, duplicate %q in %#v", group.ID, plan.ReplayGroups)
		}
		groupIDs[group.ID] = true
	}
	if len(plan.ReplayGroups) != 2 {
		t.Fatalf("distinct materializations should create two replay groups = %#v", plan.ReplayGroups)
	}
	reuseTasksByGroup := map[string]int{}
	for _, task := range plan.PhysicalTasks {
		if task.Kind == mapplanner.TaskReuseMaterialized {
			reuseTasksByGroup[task.ReplayGroupID]++
		}
	}
	if len(reuseTasksByGroup) != 2 {
		t.Fatalf("distinct replay groups should each keep their own reuse task, groups=%#v tasks=%#v", plan.ReplayGroups, plan.PhysicalTasks)
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

func TestExplainCasesScopeHonorsTargetKindCase(t *testing.T) {
	graph := validationCaseGraph()
	graph.Nodes = append(graph.Nodes, plangraph.Node{MapID: "map.contract", ID: "case.submit.bad-format", CaseID: "case.submit.bad-format", Role: "validation", StateEffect: "unchanged", AnchorNodeID: "case.submit", SortOrder: 4})

	plan, err := mapplanner.Explain(graph, mapplanner.Query{
		MapID:       "map.contract",
		Scope:       mapplanner.ScopeCases,
		TargetKind:  mapplanner.TargetCase,
		TargetID:    "case.submit.missing-days",
		PlannerMode: mapplanner.ModeRun,
	})
	if err != nil {
		t.Fatalf("explain targeted case under cases scope: %v", err)
	}
	if plan.Scope != mapplanner.ScopeCase || plan.TargetCaseID != "case.submit.missing-days" {
		t.Fatalf("target-kind case query should normalize to single case = %#v", plan)
	}
	for _, task := range plan.PhysicalTasks {
		if task.CaseID == "case.submit.bad-format" || task.NodeID == "case.submit.bad-format" {
			t.Fatalf("target-kind case query should not plan sibling validation case = %#v", plan.PhysicalTasks)
		}
	}
}

func TestExplainRejectsConflictingScopeAndTargetKind(t *testing.T) {
	graph := validationCaseGraph()

	_, err := mapplanner.Explain(graph, mapplanner.Query{
		MapID:       "map.contract",
		Scope:       mapplanner.ScopeWorkflows,
		TargetKind:  mapplanner.TargetCase,
		TargetID:    "case.submit.missing-days",
		PlannerMode: mapplanner.ModeRun,
	})
	if err == nil || !strings.Contains(err.Error(), "conflicts") {
		t.Fatalf("expected conflicting workflow scope and case target to fail, got %v", err)
	}
}

func TestGraphFingerprintTracksExecutableGraphFacts(t *testing.T) {
	graph := validationCaseGraph()
	base := mapplanner.GraphFingerprint(graph)

	nodeChanged := validationCaseGraph()
	nodeChanged.Nodes[2].PatchJSON = `[{"op":"remove","path":"$.days"}]`
	if got := mapplanner.GraphFingerprint(nodeChanged); got == base {
		t.Fatalf("node execution metadata should affect fingerprint: %s", got)
	}

	edgeChanged := validationCaseGraph()
	edgeChanged.Edges[1].MappingsJSON = `[{"from":"$.item_id","to":"$.request.item_id"}]`
	if got := mapplanner.GraphFingerprint(edgeChanged); got == base {
		t.Fatalf("edge mappings should affect fingerprint: %s", got)
	}

	materializationChanged := validationCaseGraph()
	materializationChanged.Materializations[0].FixtureID = "fixture.other"
	if got := mapplanner.GraphFingerprint(materializationChanged); got == base {
		t.Fatalf("materialization fixture should affect fingerprint: %s", got)
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
