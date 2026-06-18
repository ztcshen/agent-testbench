package storecontract

import (
	"context"
	"testing"
	"time"

	"agent-testbench/internal/store"
)

func requirePlanGraphContract(t *testing.T, ctx context.Context, s store.Store, started time.Time) {
	t.Helper()

	graph := store.TestPlanGraph{
		Map: store.TestPlanMap{
			ID:          "map.contract",
			ProfileID:   contractProfileID,
			DisplayName: "Contract Map",
			Status:      "active",
			SummaryJSON: `{"paths":1}`,
			CreatedAt:   started,
			UpdatedAt:   started,
		},
		Nodes: []store.TestPlanNode{
			{MapID: "map.contract", ID: "case.alpha", CaseID: "case.alpha", InterfaceNodeID: "node.alpha", RequestTemplateID: "template.alpha", Role: "primary", StateEffect: "advance", RequiredPropertyJSON: `{}`, ProvidedPropertyJSON: `{"stateEffect":"advance"}`, SummaryJSON: `{}`, SortOrder: 1},
			{MapID: "map.contract", ID: "case.alpha.missing-param", CaseID: "case.alpha.missing-param", InterfaceNodeID: "node.alpha", RequestTemplateID: "template.alpha", BaseCaseID: "case.alpha", AnchorNodeID: "case.alpha", Role: "validation", StateEffect: "unchanged", RenderMode: "template_patch", PatchJSON: `[{"op":"remove","path":"$.body.days"}]`, ExpectedJSON: `{"status":400}`, RequiredPropertyJSON: `{"samePreconditionAsCase":"case.alpha"}`, ProvidedPropertyJSON: `{"stateEffect":"unchanged"}`, SummaryJSON: `{}`, SortOrder: 2},
		},
		Edges: []store.TestPlanEdge{
			{MapID: "map.contract", ID: "edge.contract", FromNodeID: "case.alpha", ToNodeID: "case.alpha.missing-param", Kind: "fixture", Required: true, MappingsJSON: `[]`, SummaryJSON: `{}`, SortOrder: 1},
		},
		Paths: []store.TestPlanPath{
			{MapID: "map.contract", ID: "workflow.alpha", WorkflowID: "workflow.alpha", DisplayName: "Workflow Alpha", Status: "active", RequiredPropertyJSON: `{}`, ProvidedPropertyJSON: `{}`, SummaryJSON: `{}`, SortOrder: 1},
		},
		PathSteps: []store.TestPlanPathStep{
			{MapID: "map.contract", PathID: "workflow.alpha", StepIndex: 1, StepID: "step.alpha", NodeID: "case.alpha", CaseID: "case.alpha", Required: true, SummaryJSON: `{}`},
		},
		Materializations: []store.TestPlanMaterialization{
			{MapID: "map.contract", ID: "fixture.alpha", FixtureID: "fixture.alpha", SourcePathID: "workflow.alpha", SourceWorkflowID: "workflow.alpha", SourceUntilStep: "step.alpha", SourceUntilNodeID: "case.alpha", SnapshotKind: "workflow_prefix", TTLSeconds: 3600, Status: "active", SummaryJSON: `{}`, SortOrder: 1},
		},
	}

	if err := s.ReplaceTestPlanGraph(ctx, graph); err != nil {
		t.Fatalf("replace test plan graph: %v", err)
	}
	loaded, err := s.GetTestPlanGraph(ctx, "map.contract")
	if err != nil {
		t.Fatalf("get test plan graph: %v", err)
	}
	if loaded.Map.ID != "map.contract" || loaded.Map.ProfileID != contractProfileID || loaded.Map.CreatedAt.IsZero() {
		t.Fatalf("loaded map = %#v", loaded.Map)
	}
	if len(loaded.Nodes) != 2 || loaded.Nodes[1].BaseCaseID != "case.alpha" || loaded.Nodes[1].PatchJSON == "" {
		t.Fatalf("loaded nodes = %#v", loaded.Nodes)
	}
	if len(loaded.Edges) != 1 || loaded.Edges[0].Kind != "fixture" {
		t.Fatalf("loaded edges = %#v", loaded.Edges)
	}
	if len(loaded.Paths) != 1 || len(loaded.PathSteps) != 1 || len(loaded.Materializations) != 1 {
		t.Fatalf("loaded graph counts: paths=%#v steps=%#v materializations=%#v", loaded.Paths, loaded.PathSteps, loaded.Materializations)
	}
	maps, err := s.ListTestPlanMaps(ctx)
	if err != nil {
		t.Fatalf("list test plan maps: %v", err)
	}
	var summary store.TestPlanMapSummary
	for _, item := range maps {
		if item.ID == "map.contract" {
			summary = item
			break
		}
	}
	if summary.ID != "map.contract" || summary.ProfileID != contractProfileID || summary.NodeCount != 2 || summary.EdgeCount != 1 || summary.PathCount != 1 || summary.MaterializationCount != 1 {
		t.Fatalf("map summary = %#v", summary)
	}
	version, err := s.SaveTestPlanMapVersion(ctx, store.TestPlanMapVersion{
		MapID:     "map.contract",
		Version:   "v1",
		Status:    "published",
		Summary:   "contract snapshot",
		GraphJSON: `{"map":{"id":"map.contract"}}`,
		CreatedAt: started,
	})
	if err != nil {
		t.Fatalf("save test plan map version: %v", err)
	}
	if version.ID == "" || version.CreatedAt.IsZero() {
		t.Fatalf("saved map version defaults = %#v", version)
	}
	versions, err := s.ListTestPlanMapVersions(ctx, "map.contract")
	if err != nil {
		t.Fatalf("list test plan map versions: %v", err)
	}
	if len(versions) != 1 || versions[0].Version != "v1" || versions[0].Status != "published" || versions[0].Summary != "contract snapshot" || versions[0].GraphJSON == "" {
		t.Fatalf("listed map versions = %#v", versions)
	}
}
