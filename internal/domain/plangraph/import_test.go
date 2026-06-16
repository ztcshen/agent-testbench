package plangraph

import (
	"strings"
	"testing"
	"time"

	"agent-testbench/internal/domain/catalog"
)

func TestImportCatalogBuildsSharedMapAndAnchoredValidationCase(t *testing.T) {
	catalogSnapshot := catalog.ProfileCatalog{
		ProfileID: "profile.withdraw",
		IndexedAt: time.Date(2026, 6, 16, 8, 0, 0, 0, time.UTC),
		Workflows: []catalog.Workflow{
			{ID: "workflow.withdraw.success", DisplayName: "Withdraw Success"},
			{ID: "workflow.withdraw.query", DisplayName: "Withdraw Query"},
		},
		InterfaceNodes: []catalog.InterfaceNode{
			{ID: "node.quote", DisplayName: "Quote"},
			{ID: "node.apply", DisplayName: "Apply"},
			{ID: "node.result", DisplayName: "Result"},
		},
		APICases: []catalog.APICase{
			{ID: "case.quote", DisplayName: "Quote", NodeID: "node.quote", RequestTemplateID: "template.quote", Status: "active", SortOrder: 1},
			{ID: "case.apply.success", DisplayName: "Apply success", NodeID: "node.apply", RequestTemplateID: "template.apply", Status: "active", SortOrder: 2},
			{
				ID: "case.apply.days.required", DisplayName: "Apply days required", NodeID: "node.apply",
				CaseType: "negative", RequestTemplateID: "template.apply", RenderMode: "template_patch",
				PatchJSON: `[{"op":"remove","path":"$.body.days"}]`, ExpectedJSON: `{"status":400}`,
				Status: "active", SortOrder: 3,
			},
			{ID: "case.result.success", DisplayName: "Result success", NodeID: "node.result", RequestTemplateID: "template.result", Status: "active", SortOrder: 4},
			{ID: "case.result.query", DisplayName: "Result query", NodeID: "node.result", RequestTemplateID: "template.result", Status: "active", SortOrder: 5},
		},
		WorkflowBindings: []catalog.WorkflowBinding{
			{WorkflowID: "workflow.withdraw.success", StepID: "step.quote", NodeID: "node.quote", CaseID: "case.quote", Required: true, SortOrder: 1},
			{WorkflowID: "workflow.withdraw.success", StepID: "step.apply", NodeID: "node.apply", CaseID: "case.apply.success", Required: true, SortOrder: 2},
			{WorkflowID: "workflow.withdraw.success", StepID: "step.result", NodeID: "node.result", CaseID: "case.result.success", Required: true, SortOrder: 3},
			{WorkflowID: "workflow.withdraw.query", StepID: "step.quote", NodeID: "node.quote", CaseID: "case.quote", Required: true, SortOrder: 1},
			{WorkflowID: "workflow.withdraw.query", StepID: "step.apply", NodeID: "node.apply", CaseID: "case.apply.success", Required: true, SortOrder: 2},
			{WorkflowID: "workflow.withdraw.query", StepID: "step.query", NodeID: "node.result", CaseID: "case.result.query", Required: true, SortOrder: 3},
		},
		Fixtures: []catalog.Fixture{{
			ID: "fixture.before.apply", DisplayName: "Before apply", Kind: "workflow_prefix",
			SourceWorkflowID: "workflow.withdraw.success", SourceUntilStep: "step.quote", Status: "active",
		}},
		CaseDependencies: []catalog.CaseDependency{{
			ID: "dependency.days.required", CaseID: "case.apply.days.required", FixtureID: "fixture.before.apply",
			MappingsJSON: `[{"from":"$.quote.orderId","to":"$.body.orderId"}]`, Required: true,
		}},
	}

	graph, err := ImportCatalog(catalogSnapshot, ImportOptions{})
	if err != nil {
		t.Fatalf("import catalog: %v", err)
	}
	if graph.Map.ID != "map.profile.withdraw" || graph.Map.ProfileID != "profile.withdraw" {
		t.Fatalf("map identity = %#v", graph.Map)
	}
	if len(graph.Paths) != 2 {
		t.Fatalf("paths = %#v", graph.Paths)
	}
	if len(graph.Nodes) != 5 {
		t.Fatalf("nodes = %#v", graph.Nodes)
	}
	if countPathSteps(graph, "case.quote") != 2 {
		t.Fatalf("shared quote node should be reused by both paths: %#v", graph.PathSteps)
	}

	validation := requireNode(t, graph, "case.apply.days.required")
	if validation.Role != NodeRoleValidation || validation.StateEffect != StateEffectUnchanged {
		t.Fatalf("validation node role/effect = %#v", validation)
	}
	if validation.BaseCaseID != "case.apply.success" || validation.AnchorNodeID != "case.apply.success" {
		t.Fatalf("validation anchor = %#v", validation)
	}
	if !strings.Contains(validation.RequiredPropertyJSON, `"samePreconditionAsCase":"case.apply.success"`) {
		t.Fatalf("validation required property = %s", validation.RequiredPropertyJSON)
	}

	materialization := requireMaterialization(t, graph, "fixture.before.apply")
	if materialization.SourcePathID != "workflow.withdraw.success" || materialization.SourceUntilNodeID != "case.quote" {
		t.Fatalf("materialization source = %#v", materialization)
	}
	edge := requireEdgeTo(t, graph, "case.apply.days.required", EdgeKindFixture)
	if edge.FromNodeID != "case.quote" || edge.MappingsJSON == "" {
		t.Fatalf("fixture edge = %#v", edge)
	}
}

func TestExplainCaseSelectsReplayPrefixForValidationDiff(t *testing.T) {
	graph, err := ImportCatalog(plannerFixtureCatalog(), ImportOptions{})
	if err != nil {
		t.Fatalf("import catalog: %v", err)
	}

	explain, err := ExplainCase(graph, ExplainOptions{CaseID: "case.apply.days.required"})
	if err != nil {
		t.Fatalf("explain case: %v", err)
	}
	if explain.TargetCaseID != "case.apply.days.required" || explain.TargetNodeID != "case.apply.days.required" {
		t.Fatalf("explain target = %#v", explain)
	}
	if len(explain.Operations) != 2 {
		t.Fatalf("operations = %#v", explain.Operations)
	}
	if explain.Operations[0].Kind != OperationRunPathPrefix || explain.Operations[0].PathID != "workflow.withdraw.success" || explain.Operations[0].UntilNodeID != "case.quote" {
		t.Fatalf("prefix operation = %#v", explain.Operations[0])
	}
	if explain.Operations[1].Kind != OperationRunCase || explain.Operations[1].CaseID != "case.apply.days.required" || explain.Operations[1].PatchJSON == "" {
		t.Fatalf("run-case operation = %#v", explain.Operations[1])
	}
}

func plannerFixtureCatalog() catalog.ProfileCatalog {
	return catalog.ProfileCatalog{
		ProfileID: "profile.withdraw",
		Workflows: []catalog.Workflow{{ID: "workflow.withdraw.success"}},
		APICases: []catalog.APICase{
			{ID: "case.quote", NodeID: "node.quote", RequestTemplateID: "template.quote", Status: "active", SortOrder: 1},
			{ID: "case.apply.success", NodeID: "node.apply", RequestTemplateID: "template.apply", Status: "active", SortOrder: 2},
			{ID: "case.apply.days.required", NodeID: "node.apply", CaseType: "negative", RequestTemplateID: "template.apply", RenderMode: "template_patch", PatchJSON: `[{"op":"remove","path":"$.body.days"}]`, ExpectedJSON: `{"status":400}`, Status: "active", SortOrder: 3},
		},
		WorkflowBindings: []catalog.WorkflowBinding{
			{WorkflowID: "workflow.withdraw.success", StepID: "step.quote", NodeID: "node.quote", CaseID: "case.quote", Required: true, SortOrder: 1},
			{WorkflowID: "workflow.withdraw.success", StepID: "step.apply", NodeID: "node.apply", CaseID: "case.apply.success", Required: true, SortOrder: 2},
		},
		Fixtures: []catalog.Fixture{{
			ID: "fixture.before.apply", Kind: "workflow_prefix", SourceWorkflowID: "workflow.withdraw.success", SourceUntilStep: "step.quote", Status: "active",
		}},
		CaseDependencies: []catalog.CaseDependency{{
			ID: "dependency.days.required", CaseID: "case.apply.days.required", FixtureID: "fixture.before.apply", Required: true,
		}},
	}
}

func requireNode(t *testing.T, graph Graph, nodeID string) Node {
	t.Helper()
	for _, node := range graph.Nodes {
		if node.ID == nodeID {
			return node
		}
	}
	t.Fatalf("node %s not found in %#v", nodeID, graph.Nodes)
	return Node{}
}

func requireMaterialization(t *testing.T, graph Graph, id string) Materialization {
	t.Helper()
	for _, item := range graph.Materializations {
		if item.ID == id {
			return item
		}
	}
	t.Fatalf("materialization %s not found in %#v", id, graph.Materializations)
	return Materialization{}
}

func requireEdgeTo(t *testing.T, graph Graph, toNodeID string, kind string) Edge {
	t.Helper()
	for _, edge := range graph.Edges {
		if edge.ToNodeID == toNodeID && edge.Kind == kind {
			return edge
		}
	}
	t.Fatalf("edge to %s kind %s not found in %#v", toNodeID, kind, graph.Edges)
	return Edge{}
}

func countPathSteps(graph Graph, nodeID string) int {
	count := 0
	for _, step := range graph.PathSteps {
		if step.NodeID == nodeID {
			count++
		}
	}
	return count
}
