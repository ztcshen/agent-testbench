package plangraph

import (
	"strings"
	"testing"
	"time"

	"agent-testbench/internal/domain/catalog"
)

func TestImportCatalogBuildsSharedMapAndAnchoredValidationCase(t *testing.T) {
	catalogSnapshot := catalog.ProfileCatalog{
		ProfileID: "profile.flow",
		IndexedAt: time.Date(2026, 6, 16, 8, 0, 0, 0, time.UTC),
		Workflows: []catalog.Workflow{
			{ID: "workflow.flow.create", DisplayName: "Create Flow"},
			{ID: "workflow.flow.audit", DisplayName: "Audit Flow"},
		},
		InterfaceNodes: []catalog.InterfaceNode{
			{ID: "node.prepare", DisplayName: "Prepare"},
			{ID: "node.submit", DisplayName: "Submit"},
			{ID: "node.verify", DisplayName: "Verify"},
		},
		APICases: []catalog.APICase{
			{ID: "case.prepare", DisplayName: "Prepare", NodeID: "node.prepare", RequestTemplateID: "template.prepare", Status: "active", SortOrder: 1},
			{ID: "case.submit.success", DisplayName: "Submit success", NodeID: "node.submit", RequestTemplateID: "template.submit", Status: "active", SortOrder: 2},
			{
				ID: "case.submit.field.required", DisplayName: "Submit field required", NodeID: "node.submit",
				CaseType: "negative", RequestTemplateID: "template.submit", RenderMode: "template_patch",
				PatchJSON: `[{"op":"remove","path":"$.body.field"}]`, ExpectedJSON: `{"status":400}`,
				Status: "active", SortOrder: 3,
			},
			{ID: "case.verify.success", DisplayName: "Verify success", NodeID: "node.verify", RequestTemplateID: "template.verify", Status: "active", SortOrder: 4},
			{ID: "case.verify.query", DisplayName: "Verify query", NodeID: "node.verify", RequestTemplateID: "template.verify", Status: "active", SortOrder: 5},
		},
		WorkflowBindings: []catalog.WorkflowBinding{
			{WorkflowID: "workflow.flow.create", StepID: "step.prepare", NodeID: "node.prepare", CaseID: "case.prepare", Required: true, SortOrder: 1},
			{WorkflowID: "workflow.flow.create", StepID: "step.submit", NodeID: "node.submit", CaseID: "case.submit.success", Required: true, SortOrder: 2},
			{WorkflowID: "workflow.flow.create", StepID: "step.verify", NodeID: "node.verify", CaseID: "case.verify.success", Required: true, SortOrder: 3},
			{WorkflowID: "workflow.flow.audit", StepID: "step.prepare", NodeID: "node.prepare", CaseID: "case.prepare", Required: true, SortOrder: 1},
			{WorkflowID: "workflow.flow.audit", StepID: "step.submit", NodeID: "node.submit", CaseID: "case.submit.success", Required: true, SortOrder: 2},
			{WorkflowID: "workflow.flow.audit", StepID: "step.audit", NodeID: "node.verify", CaseID: "case.verify.query", Required: true, SortOrder: 3},
		},
		Fixtures: []catalog.Fixture{{
			ID: "fixture.before.submit", DisplayName: "Before submit", Kind: "workflow_prefix",
			SourceWorkflowID: "workflow.flow.create", SourceUntilStep: "step.prepare", Status: "active",
		}},
		CaseDependencies: []catalog.CaseDependency{{
			ID: "dependency.field.required", CaseID: "case.submit.field.required", FixtureID: "fixture.before.submit",
			MappingsJSON: `[{"from":"$.prepare.entityId","to":"$.body.entityId"}]`, Required: true,
		}},
	}

	graph, err := ImportCatalog(catalogSnapshot, ImportOptions{})
	if err != nil {
		t.Fatalf("import catalog: %v", err)
	}
	if graph.Map.ID != "map.profile.flow" || graph.Map.ProfileID != "profile.flow" {
		t.Fatalf("map identity = %#v", graph.Map)
	}
	if len(graph.Paths) != 2 {
		t.Fatalf("paths = %#v", graph.Paths)
	}
	if len(graph.Nodes) != 5 {
		t.Fatalf("nodes = %#v", graph.Nodes)
	}
	if countPathSteps(graph, "case.prepare") != 2 {
		t.Fatalf("shared prepare node should be reused by both paths: %#v", graph.PathSteps)
	}

	validation := requireNode(t, graph, "case.submit.field.required")
	if validation.Role != NodeRoleValidation || validation.StateEffect != StateEffectUnchanged {
		t.Fatalf("validation node role/effect = %#v", validation)
	}
	if validation.BaseCaseID != "case.submit.success" || validation.AnchorNodeID != "case.submit.success" {
		t.Fatalf("validation anchor = %#v", validation)
	}
	if !strings.Contains(validation.RequiredPropertyJSON, `"samePreconditionAsCase":"case.submit.success"`) {
		t.Fatalf("validation required property = %s", validation.RequiredPropertyJSON)
	}

	materialization := requireMaterialization(t, graph, "fixture.before.submit")
	if materialization.SourcePathID != "workflow.flow.create" || materialization.SourceUntilNodeID != "case.prepare" {
		t.Fatalf("materialization source = %#v", materialization)
	}
	edge := requireEdgeTo(t, graph, "case.submit.field.required", EdgeKindFixture)
	if edge.FromNodeID != "case.prepare" || edge.MappingsJSON == "" {
		t.Fatalf("fixture edge = %#v", edge)
	}
}

func TestExplainCaseSelectsReplayPrefixForValidationDiff(t *testing.T) {
	graph, err := ImportCatalog(plannerFixtureCatalog(), ImportOptions{})
	if err != nil {
		t.Fatalf("import catalog: %v", err)
	}

	explain, err := ExplainCase(graph, ExplainOptions{CaseID: "case.submit.field.required"})
	if err != nil {
		t.Fatalf("explain case: %v", err)
	}
	if explain.TargetCaseID != "case.submit.field.required" || explain.TargetNodeID != "case.submit.field.required" {
		t.Fatalf("explain target = %#v", explain)
	}
	if len(explain.Operations) != 2 {
		t.Fatalf("operations = %#v", explain.Operations)
	}
	if explain.Operations[0].Kind != OperationRunPathPrefix || explain.Operations[0].PathID != "workflow.flow.create" || explain.Operations[0].UntilNodeID != "case.prepare" {
		t.Fatalf("prefix operation = %#v", explain.Operations[0])
	}
	if explain.Operations[1].Kind != OperationRunCase || explain.Operations[1].CaseID != "case.submit.field.required" || explain.Operations[1].PatchJSON == "" {
		t.Fatalf("run-case operation = %#v", explain.Operations[1])
	}
	if len(explain.CandidatePaths) != 2 {
		t.Fatalf("candidate paths = %#v", explain.CandidatePaths)
	}
	if explain.CandidatePaths[0].PathID != "workflow.flow.create" || !explain.CandidatePaths[0].Selected {
		t.Fatalf("selected candidate = %#v", explain.CandidatePaths)
	}
	if len(explain.RejectedReasons) != 1 || explain.RejectedReasons[0].PathID != "workflow.flow.audit" {
		t.Fatalf("rejected reasons = %#v", explain.RejectedReasons)
	}
}

func TestExplainCaseIgnoresOptionalFixtureEdge(t *testing.T) {
	source := plannerFixtureCatalog()
	source.Fixtures[0].SourceWorkflowID = "workflow.flow.audit"
	source.Fixtures[0].SourceUntilStep = "step.audit"
	source.CaseDependencies[0].Required = false
	graph, err := ImportCatalog(source, ImportOptions{})
	if err != nil {
		t.Fatalf("import catalog: %v", err)
	}
	edge := requireEdgeTo(t, graph, "case.submit.field.required", EdgeKindFixture)
	if edge.Required {
		t.Fatalf("fixture edge should be optional: %#v", edge)
	}

	explain, err := ExplainCase(graph, ExplainOptions{CaseID: "case.submit.field.required"})
	if err != nil {
		t.Fatalf("explain case: %v", err)
	}
	if len(explain.Operations) != 2 {
		t.Fatalf("operations = %#v", explain.Operations)
	}
	prefix := explain.Operations[0]
	if prefix.Kind != OperationRunPathPrefix || prefix.UntilNodeID == "case.audit" || strings.Contains(prefix.Reason, "fixture") {
		t.Fatalf("optional fixture should not drive mandatory replay prefix: %#v", prefix)
	}
}

func TestExplainCaseDoesNotRejectAlternatePathsThatContainPrimaryTarget(t *testing.T) {
	graph, err := ImportCatalog(plannerFixtureCatalog(), ImportOptions{})
	if err != nil {
		t.Fatalf("import catalog: %v", err)
	}

	explain, err := ExplainCase(graph, ExplainOptions{CaseID: "case.submit.success"})
	if err != nil {
		t.Fatalf("explain case: %v", err)
	}
	for _, rejected := range explain.RejectedReasons {
		if rejected.PathID == "workflow.flow.audit" {
			t.Fatalf("alternate path containing target should not be rejected: %#v", explain.RejectedReasons)
		}
	}
	selectedCount := 0
	for _, candidate := range explain.CandidatePaths {
		if candidate.Selected {
			selectedCount++
		}
	}
	if len(explain.CandidatePaths) != 2 || selectedCount != 1 {
		t.Fatalf("candidate paths = %#v", explain.CandidatePaths)
	}
}

func TestImportCatalogUsesBoundedControlEdgeIDs(t *testing.T) {
	longPathID := "workflow." + strings.Repeat("very-long-segment.", 5)
	longFirstCaseID := "case.first." + strings.Repeat("stateful-", 7)
	longSecondCaseID := "case.second." + strings.Repeat("stateful-", 7)
	graph, err := ImportCatalog(catalog.ProfileCatalog{
		ProfileID: "profile.longids",
		Workflows: []catalog.Workflow{
			{ID: longPathID},
		},
		APICases: []catalog.APICase{
			{ID: longFirstCaseID, NodeID: "node.first", Status: "active"},
			{ID: longSecondCaseID, NodeID: "node.second", Status: "active"},
		},
		WorkflowBindings: []catalog.WorkflowBinding{
			{WorkflowID: longPathID, StepID: "step.first", CaseID: longFirstCaseID, Required: true, SortOrder: 1},
			{WorkflowID: longPathID, StepID: "step.second", CaseID: longSecondCaseID, Required: true, SortOrder: 2},
		},
	}, ImportOptions{})
	if err != nil {
		t.Fatalf("import catalog: %v", err)
	}

	edge := requireEdgeTo(t, graph, longSecondCaseID, EdgeKindControl)
	if len(edge.ID) > 128 {
		t.Fatalf("control edge id should fit store key width, got %d chars: %s", len(edge.ID), edge.ID)
	}
}

func TestImportCatalogUsesBoundedFixtureEdgeIDs(t *testing.T) {
	longWorkflowID := "workflow." + strings.Repeat("very-long-segment.", 5)
	longFixtureID := "fixture." + strings.Repeat("before-submit-", 8)
	longValidationCaseID := "case.validation." + strings.Repeat("field-required-", 8)
	graph, err := ImportCatalog(catalog.ProfileCatalog{
		ProfileID: "profile.longfixtures",
		Workflows: []catalog.Workflow{
			{ID: longWorkflowID},
		},
		APICases: []catalog.APICase{
			{ID: "case.prepare", NodeID: "node.prepare", Status: "active"},
			{ID: longValidationCaseID, NodeID: "node.submit", CaseType: "negative", Status: "active"},
		},
		WorkflowBindings: []catalog.WorkflowBinding{
			{WorkflowID: longWorkflowID, StepID: "step.prepare", CaseID: "case.prepare", Required: true, SortOrder: 1},
		},
		Fixtures: []catalog.Fixture{{
			ID: longFixtureID, Kind: "workflow_prefix", SourceWorkflowID: longWorkflowID, SourceUntilStep: "step.prepare", Status: "active",
		}},
		CaseDependencies: []catalog.CaseDependency{{
			CaseID: longValidationCaseID, FixtureID: longFixtureID, Required: true,
		}},
	}, ImportOptions{})
	if err != nil {
		t.Fatalf("import catalog: %v", err)
	}

	edge := requireEdgeTo(t, graph, longValidationCaseID, EdgeKindFixture)
	if len(edge.ID) > 128 {
		t.Fatalf("fixture edge id should fit store key width, got %d chars: %s", len(edge.ID), edge.ID)
	}
}

func TestImportCatalogImportsStandaloneValidationCase(t *testing.T) {
	graph, err := ImportCatalog(catalog.ProfileCatalog{
		ProfileID: "profile.validation",
		APICases: []catalog.APICase{
			{ID: "case.create.success", NodeID: "node.create", Status: "active", SortOrder: 1},
			{ID: "case.create.invalid-name", NodeID: "node.create", CaseType: "negative", Status: "active", SortOrder: 2},
		},
	}, ImportOptions{})
	if err != nil {
		t.Fatalf("import catalog: %v", err)
	}

	node := requireNode(t, graph, "case.create.invalid-name")
	if node.Role != NodeRoleValidation || node.BaseCaseID != "case.create.success" {
		t.Fatalf("standalone validation node = %#v", node)
	}
}

func TestImportCatalogRejectsRequiredInactiveWorkflowBindingCase(t *testing.T) {
	_, err := ImportCatalog(catalog.ProfileCatalog{
		ProfileID: "profile.inactive",
		Workflows: []catalog.Workflow{
			{ID: "workflow.flow.retired"},
		},
		APICases: []catalog.APICase{
			{ID: "case.cart", NodeID: "node.cart", Status: "active", SortOrder: 1},
			{ID: "case.retired", NodeID: "node.retired", Status: "inactive", SortOrder: 2},
		},
		WorkflowBindings: []catalog.WorkflowBinding{
			{WorkflowID: "workflow.flow.retired", StepID: "cart", CaseID: "case.cart", Required: true, SortOrder: 1},
			{WorkflowID: "workflow.flow.retired", StepID: "retired", CaseID: "case.retired", Required: true, SortOrder: 2},
		},
	}, ImportOptions{})
	if err == nil || !strings.Contains(err.Error(), "required workflow binding") || !strings.Contains(err.Error(), "case.retired") {
		t.Fatalf("expected required inactive binding error, got %v", err)
	}
}

func TestImportCatalogSkipsWorkflowPrefixFixtureWithUnknownStep(t *testing.T) {
	source := plannerFixtureCatalog()
	source.Fixtures[0].SourceUntilStep = "step.missing"

	graph, err := ImportCatalog(source, ImportOptions{})
	if err != nil {
		t.Fatalf("import catalog: %v", err)
	}
	if len(graph.Materializations) != 0 {
		t.Fatalf("fixture with unknown source step should be skipped: %#v", graph.Materializations)
	}
	for _, edge := range graph.Edges {
		if edge.Kind == EdgeKindFixture {
			t.Fatalf("dependency edge should not be imported without a valid materialization: %#v", edge)
		}
	}
}

func TestValidateDAGRejectsControlCycles(t *testing.T) {
	graph := Graph{
		Map: Map{ID: "map.cycle"},
		Nodes: []Node{
			{MapID: "map.cycle", ID: "case.a", CaseID: "case.a"},
			{MapID: "map.cycle", ID: "case.b", CaseID: "case.b"},
		},
		Edges: []Edge{
			{MapID: "map.cycle", ID: "edge.a.b", FromNodeID: "case.a", ToNodeID: "case.b", Kind: EdgeKindControl},
			{MapID: "map.cycle", ID: "edge.b.a", FromNodeID: "case.b", ToNodeID: "case.a", Kind: EdgeKindFixture},
		},
	}

	err := ValidateDAG(graph)
	if err == nil || !strings.Contains(err.Error(), "cycle") {
		t.Fatalf("validate dag error = %v", err)
	}
}

func plannerFixtureCatalog() catalog.ProfileCatalog {
	return catalog.ProfileCatalog{
		ProfileID: "profile.flow",
		Workflows: []catalog.Workflow{
			{ID: "workflow.flow.create"},
			{ID: "workflow.flow.audit"},
		},
		APICases: []catalog.APICase{
			{ID: "case.prepare", NodeID: "node.prepare", RequestTemplateID: "template.prepare", Status: "active", SortOrder: 1},
			{ID: "case.submit.success", NodeID: "node.submit", RequestTemplateID: "template.submit", Status: "active", SortOrder: 2},
			{ID: "case.submit.field.required", NodeID: "node.submit", CaseType: "negative", RequestTemplateID: "template.submit", RenderMode: "template_patch", PatchJSON: `[{"op":"remove","path":"$.body.field"}]`, ExpectedJSON: `{"status":400}`, Status: "active", SortOrder: 3},
			{ID: "case.audit", NodeID: "node.query", RequestTemplateID: "template.query", Status: "active", SortOrder: 4},
		},
		WorkflowBindings: []catalog.WorkflowBinding{
			{WorkflowID: "workflow.flow.create", StepID: "step.prepare", NodeID: "node.prepare", CaseID: "case.prepare", Required: true, SortOrder: 1},
			{WorkflowID: "workflow.flow.create", StepID: "step.submit", NodeID: "node.submit", CaseID: "case.submit.success", Required: true, SortOrder: 2},
			{WorkflowID: "workflow.flow.audit", StepID: "step.prepare", NodeID: "node.prepare", CaseID: "case.prepare", Required: true, SortOrder: 1},
			{WorkflowID: "workflow.flow.audit", StepID: "step.submit", NodeID: "node.submit", CaseID: "case.submit.success", Required: true, SortOrder: 2},
			{WorkflowID: "workflow.flow.audit", StepID: "step.audit", NodeID: "node.query", CaseID: "case.audit", Required: true, SortOrder: 3},
		},
		Fixtures: []catalog.Fixture{{
			ID: "fixture.before.submit", Kind: "workflow_prefix", SourceWorkflowID: "workflow.flow.create", SourceUntilStep: "step.prepare", Status: "active",
		}},
		CaseDependencies: []catalog.CaseDependency{{
			ID: "dependency.field.required", CaseID: "case.submit.field.required", FixtureID: "fixture.before.submit", Required: true,
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
