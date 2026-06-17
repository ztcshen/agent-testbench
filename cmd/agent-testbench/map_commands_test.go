package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"agent-testbench/internal/store"
)

func TestMapImportWorkflowsAndExplainUsesStoreCatalog(t *testing.T) {
	ctx := context.Background()
	storePath := filepath.Join(t.TempDir(), "map.sqlite")
	storeRef := "sqlite://" + storePath
	runtime, err := openStore(ctx, storeRef)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	if err := runtime.ReplaceProfileCatalog(ctx, mapCommandProfileCatalogFixture()); err != nil {
		t.Fatalf("seed profile catalog: %v", err)
	}
	closeCLIStore(runtime)

	importOut := runCLI(t, "map", "import-workflows", "--store", storeRef, "--json")
	var importReport struct {
		OK  bool `json:"ok"`
		Map struct {
			ID        string `json:"id"`
			ProfileID string `json:"profileId"`
		} `json:"map"`
		Counts struct {
			Nodes            int `json:"nodes"`
			Paths            int `json:"paths"`
			Materializations int `json:"materializations"`
		} `json:"counts"`
	}
	if err := json.Unmarshal([]byte(importOut), &importReport); err != nil {
		t.Fatalf("decode map import json: %v\n%s", err, importOut)
	}
	if !importReport.OK || importReport.Map.ID != "map.profile.flow" || importReport.Counts.Nodes != 3 || importReport.Counts.Paths != 1 || importReport.Counts.Materializations != 1 {
		t.Fatalf("map import report = %#v", importReport)
	}

	explainOut := runCLI(t, "map", "explain", "--store", storeRef, "--map", "map.profile.flow", "--case", "case.submit.field.required", "--json")
	var explainReport struct {
		OK           bool   `json:"ok"`
		TargetCaseID string `json:"targetCaseId"`
		TargetNodeID string `json:"targetNodeId"`
		Operations   []struct {
			Kind        string `json:"kind"`
			PathID      string `json:"pathId"`
			UntilNodeID string `json:"untilNodeId"`
			CaseID      string `json:"caseId"`
		} `json:"operations"`
	}
	if err := json.Unmarshal([]byte(explainOut), &explainReport); err != nil {
		t.Fatalf("decode map explain json: %v\n%s", err, explainOut)
	}
	if !explainReport.OK || explainReport.TargetCaseID != "case.submit.field.required" || len(explainReport.Operations) != 2 {
		t.Fatalf("map explain report = %#v", explainReport)
	}
	if explainReport.Operations[0].Kind != "run_path_prefix" || explainReport.Operations[0].PathID != "workflow.flow.create" || explainReport.Operations[0].UntilNodeID != "case.prepare" {
		t.Fatalf("prefix operation = %#v", explainReport.Operations[0])
	}
	if explainReport.Operations[1].Kind != "run_case" || explainReport.Operations[1].CaseID != "case.submit.field.required" {
		t.Fatalf("run case operation = %#v", explainReport.Operations[1])
	}
}

func TestMapCommandsAreDiscoverable(t *testing.T) {
	out := runCLI(t, "commands", "--filter", "map", "--json")
	var report struct {
		Count    int `json:"count"`
		Commands []struct {
			Command    string `json:"command"`
			StoreAware bool   `json:"storeAware"`
		} `json:"commands"`
	}
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatalf("decode commands json: %v\n%s", err, out)
	}
	if report.Count != 4 {
		t.Fatalf("map command count = %#v", report)
	}
	if report.Commands[0].Command != "map import-workflows" || !report.Commands[0].StoreAware {
		t.Fatalf("map import command = %#v", report.Commands)
	}
	if report.Commands[1].Command != "map workflows" || !report.Commands[1].StoreAware {
		t.Fatalf("map workflows command = %#v", report.Commands)
	}
	if report.Commands[2].Command != "map explain" || !report.Commands[2].StoreAware {
		t.Fatalf("map explain command = %#v", report.Commands)
	}
	if report.Commands[3].Command != "map review-html" || !report.Commands[3].StoreAware {
		t.Fatalf("map review-html command = %#v", report.Commands)
	}
}

func TestMapWorkflowsSearchesNamedPaths(t *testing.T) {
	ctx := context.Background()
	storePath := filepath.Join(t.TempDir(), "map.sqlite")
	storeRef := "sqlite://" + storePath
	runtime, err := openStore(ctx, storeRef)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	graph := store.TestPlanGraph{
		Map: store.TestPlanMap{ID: "map.contract", ProfileID: "profile.contract", DisplayName: "Contract Map", Status: "active"},
		Nodes: []store.TestPlanNode{
			{MapID: "map.contract", ID: "case.prepare", CaseID: "case.prepare", Role: "primary", StateEffect: "advance", SummaryJSON: `{}`, SortOrder: 1},
			{MapID: "map.contract", ID: "case.submit", CaseID: "case.submit", Role: "primary", StateEffect: "advance", SummaryJSON: `{}`, SortOrder: 2},
			{MapID: "map.contract", ID: "case.cancel", CaseID: "case.cancel", Role: "primary", StateEffect: "advance", SummaryJSON: `{}`, SortOrder: 3},
		},
		Paths: []store.TestPlanPath{
			{MapID: "map.contract", ID: "workflow.create.success", WorkflowID: "workflow.create.success", DisplayName: "Create Success", Status: "active", SummaryJSON: `{}`, SortOrder: 1},
			{MapID: "map.contract", ID: "workflow.cancel.success", WorkflowID: "workflow.cancel.success", DisplayName: "Cancel Success", Status: "active", SummaryJSON: `{}`, SortOrder: 2},
		},
		PathSteps: []store.TestPlanPathStep{
			{MapID: "map.contract", PathID: "workflow.create.success", StepIndex: 1, StepID: "prepare", NodeID: "case.prepare", CaseID: "case.prepare", Required: true, SummaryJSON: `{}`},
			{MapID: "map.contract", PathID: "workflow.create.success", StepIndex: 2, StepID: "submit", NodeID: "case.submit", CaseID: "case.submit", Required: true, SummaryJSON: `{}`},
			{MapID: "map.contract", PathID: "workflow.cancel.success", StepIndex: 1, StepID: "submit", NodeID: "case.submit", CaseID: "case.submit", Required: true, SummaryJSON: `{}`},
			{MapID: "map.contract", PathID: "workflow.cancel.success", StepIndex: 2, StepID: "cancel", NodeID: "case.cancel", CaseID: "case.cancel", Required: true, SummaryJSON: `{}`},
		},
		Edges: []store.TestPlanEdge{
			{MapID: "map.contract", ID: "edge.create.prepare.submit", FromNodeID: "case.prepare", ToNodeID: "case.submit", Kind: "control", PathID: "workflow.create.success", Required: true, SummaryJSON: `{}`},
			{MapID: "map.contract", ID: "edge.cancel.submit.cancel", FromNodeID: "case.submit", ToNodeID: "case.cancel", Kind: "control", PathID: "workflow.cancel.success", Required: true, SummaryJSON: `{}`},
		},
	}
	if err := runtime.ReplaceTestPlanGraph(ctx, graph); err != nil {
		t.Fatalf("seed test plan graph: %v", err)
	}
	closeCLIStore(runtime)

	out := runCLI(t, "map", "workflows", "--store", storeRef, "--map", "map.contract", "--filter", "cancel", "--json")
	var report struct {
		OK        bool   `json:"ok"`
		MapID     string `json:"mapId"`
		Filter    string `json:"filter"`
		Count     int    `json:"count"`
		Workflows []struct {
			PathID      string `json:"pathId"`
			WorkflowID  string `json:"workflowId"`
			DisplayName string `json:"displayName"`
			StepCount   int    `json:"stepCount"`
			FirstNodeID string `json:"firstNodeId"`
			LastNodeID  string `json:"lastNodeId"`
		} `json:"workflows"`
	}
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatalf("decode map workflows json: %v\n%s", err, out)
	}
	if !report.OK || report.MapID != "map.contract" || report.Filter != "cancel" || report.Count != 1 {
		t.Fatalf("map workflows report = %#v", report)
	}
	workflow := report.Workflows[0]
	if workflow.WorkflowID != "workflow.cancel.success" || workflow.StepCount != 2 || workflow.FirstNodeID != "case.submit" || workflow.LastNodeID != "case.cancel" {
		t.Fatalf("workflow row = %#v", workflow)
	}
}

func TestMapReviewHTMLWritesInteractiveArtifact(t *testing.T) {
	ctx := context.Background()
	storePath := filepath.Join(t.TempDir(), "map-review.sqlite")
	storeRef := "sqlite://" + storePath
	runtime, err := openStore(ctx, storeRef)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	if err := runtime.ReplaceProfileCatalog(ctx, mapCommandProfileCatalogFixture()); err != nil {
		t.Fatalf("seed profile catalog: %v", err)
	}
	closeCLIStore(runtime)

	runCLI(t, "map", "import-workflows", "--store", storeRef, "--json")
	outputPath := filepath.Join(t.TempDir(), "flow-map-review.html")
	out := runCLI(t, "map", "review-html", "--store", storeRef, "--map", "map.profile.flow", "--output", outputPath, "--json")
	var report struct {
		OK     bool   `json:"ok"`
		MapID  string `json:"mapId"`
		Output string `json:"output"`
		Counts struct {
			Nodes int `json:"nodes"`
			Paths int `json:"paths"`
		} `json:"counts"`
	}
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatalf("decode map review-html json: %v\n%s", err, out)
	}
	if !report.OK || report.MapID != "map.profile.flow" || report.Output != outputPath || report.Counts.Nodes != 3 || report.Counts.Paths != 1 {
		t.Fatalf("map review-html report = %#v", report)
	}
	raw, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("read review html: %v", err)
	}
	html := string(raw)
	for _, want := range []string{
		`id="map-review-data"`,
		`function selectNode`,
		`id="workflow-filter"`,
		`case.submit.field.required`,
		`Field required`,
		`template.submit`,
		`Interface reverse cases`,
		`function interfaceReverseCases`,
		`id="map-review-minimap"`,
		`id="node-history"`,
		`Path Finder`,
		`function navigateToState`,
		`function openPathFinder`,
		`function findPath`,
		`function toggleFocusNode`,
		`run_path_prefix`,
		`run validation case as a patched single request`,
	} {
		if !strings.Contains(html, want) {
			t.Fatalf("review html missing %q\n%s", want, html)
		}
	}
	if strings.Contains(html, `adj.get(e.toNodeId).push(e.fromNodeId)`) {
		t.Fatalf("path finder should not traverse directed edges backwards:\n%s", html)
	}
	for _, unsafeHandler := range []string{
		`showInterfaceCases(\''+esc`,
		`highlightPath(\''+esc`,
		`selectNode(\''+esc`,
	} {
		if strings.Contains(html, unsafeHandler) {
			t.Fatalf("review html should use JavaScript-string escaping for handler arguments %q:\n%s", unsafeHandler, html)
		}
	}
}

func TestMapReviewHTMLCanFilterWorkflowPaths(t *testing.T) {
	ctx := context.Background()
	storePath := filepath.Join(t.TempDir(), "map-review-filter.sqlite")
	storeRef := "sqlite://" + storePath
	runtime, err := openStore(ctx, storeRef)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	graph := store.TestPlanGraph{
		Map: store.TestPlanMap{ID: "map.contract", ProfileID: "profile.contract", DisplayName: "Contract Map", Status: "active"},
		Nodes: []store.TestPlanNode{
			{MapID: "map.contract", ID: "case.prepare", CaseID: "case.prepare", Role: "primary", StateEffect: "advance", SummaryJSON: `{}`, SortOrder: 1},
			{MapID: "map.contract", ID: "case.submit", CaseID: "case.submit", Role: "primary", StateEffect: "advance", SummaryJSON: `{}`, SortOrder: 2},
			{MapID: "map.contract", ID: "case.cancel", CaseID: "case.cancel", Role: "primary", StateEffect: "advance", SummaryJSON: `{}`, SortOrder: 3},
		},
		Paths: []store.TestPlanPath{
			{MapID: "map.contract", ID: "workflow.create.success", WorkflowID: "workflow.create.success", DisplayName: "Create Success", Status: "active", SummaryJSON: `{}`, SortOrder: 1},
			{MapID: "map.contract", ID: "workflow.cancel.success", WorkflowID: "workflow.cancel.success", DisplayName: "Cancel Success", Status: "active", SummaryJSON: `{}`, SortOrder: 2},
		},
		PathSteps: []store.TestPlanPathStep{
			{MapID: "map.contract", PathID: "workflow.create.success", StepIndex: 1, StepID: "prepare", NodeID: "case.prepare", CaseID: "case.prepare", Required: true, SummaryJSON: `{}`},
			{MapID: "map.contract", PathID: "workflow.create.success", StepIndex: 2, StepID: "submit", NodeID: "case.submit", CaseID: "case.submit", Required: true, SummaryJSON: `{}`},
			{MapID: "map.contract", PathID: "workflow.cancel.success", StepIndex: 1, StepID: "submit", NodeID: "case.submit", CaseID: "case.submit", Required: true, SummaryJSON: `{}`},
			{MapID: "map.contract", PathID: "workflow.cancel.success", StepIndex: 2, StepID: "cancel", NodeID: "case.cancel", CaseID: "case.cancel", Required: true, SummaryJSON: `{}`},
		},
		Edges: []store.TestPlanEdge{
			{MapID: "map.contract", ID: "edge.create.prepare.submit", FromNodeID: "case.prepare", ToNodeID: "case.submit", Kind: "control", PathID: "workflow.create.success", Required: true, SummaryJSON: `{}`},
			{MapID: "map.contract", ID: "edge.cancel.submit.cancel", FromNodeID: "case.submit", ToNodeID: "case.cancel", Kind: "control", PathID: "workflow.cancel.success", Required: true, SummaryJSON: `{}`},
		},
	}
	if err := runtime.ReplaceTestPlanGraph(ctx, graph); err != nil {
		t.Fatalf("seed test plan graph: %v", err)
	}
	closeCLIStore(runtime)

	outputPath := filepath.Join(t.TempDir(), "cancel-map-review.html")
	out := runCLI(t, "map", "review-html", "--store", storeRef, "--map", "map.contract", "--filter", "cancel", "--output", outputPath, "--json")
	var report struct {
		OK     bool   `json:"ok"`
		Filter string `json:"filter"`
		Counts struct {
			Paths int `json:"paths"`
			Nodes int `json:"nodes"`
		} `json:"counts"`
	}
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatalf("decode filtered map review-html json: %v\n%s", err, out)
	}
	if !report.OK || report.Filter != "cancel" || report.Counts.Paths != 1 || report.Counts.Nodes != 2 {
		t.Fatalf("filtered review report = %#v", report)
	}
	raw, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("read filtered review html: %v", err)
	}
	html := string(raw)
	if !strings.Contains(html, "workflow.cancel.success") {
		t.Fatalf("filtered review html missing cancel workflow:\n%s", html)
	}
	if strings.Contains(html, "workflow.create.success") {
		t.Fatalf("filtered review html should not include create workflow:\n%s", html)
	}
}

func TestMapReviewFilterIncludesReplayPathForFixtureTarget(t *testing.T) {
	graph := store.TestPlanGraph{
		Map: store.TestPlanMap{ID: "map.contract", ProfileID: "profile.contract", DisplayName: "Contract Map", Status: "active"},
		Nodes: []store.TestPlanNode{
			{MapID: "map.contract", ID: "case.prepare", CaseID: "case.prepare", Role: "primary", StateEffect: "advance", SummaryJSON: `{}`, SortOrder: 1},
			{MapID: "map.contract", ID: "case.submit", CaseID: "case.submit", Role: "primary", StateEffect: "advance", SummaryJSON: `{}`, SortOrder: 2},
			{MapID: "map.contract", ID: "case.submit.field.required", CaseID: "case.submit.field.required", Role: "validation", StateEffect: "unchanged", SummaryJSON: `{}`, SortOrder: 3},
		},
		Paths: []store.TestPlanPath{
			{MapID: "map.contract", ID: "workflow.create.success", WorkflowID: "workflow.create.success", DisplayName: "Create Success", Status: "active", SummaryJSON: `{}`, SortOrder: 1},
		},
		PathSteps: []store.TestPlanPathStep{
			{MapID: "map.contract", PathID: "workflow.create.success", StepIndex: 1, StepID: "prepare", NodeID: "case.prepare", CaseID: "case.prepare", Required: true, SummaryJSON: `{}`},
			{MapID: "map.contract", PathID: "workflow.create.success", StepIndex: 2, StepID: "submit", NodeID: "case.submit", CaseID: "case.submit", Required: true, SummaryJSON: `{}`},
		},
		Materializations: []store.TestPlanMaterialization{
			{MapID: "map.contract", ID: "fixture.before.submit", SourcePathID: "workflow.create.success", SourceUntilNodeID: "case.prepare", Status: "active", SummaryJSON: `{}`},
		},
		Edges: []store.TestPlanEdge{
			{MapID: "map.contract", ID: "edge.create.prepare.submit", FromNodeID: "case.prepare", ToNodeID: "case.submit", Kind: "control", PathID: "workflow.create.success", Required: true, SummaryJSON: `{}`},
			{MapID: "map.contract", ID: "edge.fixture.field.required", FromNodeID: "case.prepare", ToNodeID: "case.submit.field.required", Kind: "fixture", MaterializationID: "fixture.before.submit", Required: true, SummaryJSON: `{}`},
		},
	}

	filtered := filterMapReviewGraph(graph, "field.required")
	if len(filtered.Paths) != 1 || filtered.Paths[0].ID != "workflow.create.success" {
		t.Fatalf("filtered fixture target should retain replay path: %#v", filtered.Paths)
	}
	if len(filtered.PathSteps) != 2 {
		t.Fatalf("filtered fixture target should retain source path steps: %#v", filtered.PathSteps)
	}
	if !mapCommandHasNode(filtered.Nodes, "case.prepare") || !mapCommandHasNode(filtered.Nodes, "case.submit.field.required") {
		t.Fatalf("filtered fixture target should keep replay and target nodes: %#v", filtered.Nodes)
	}
	if !mapCommandHasEdge(filtered.Edges, "edge.create.prepare.submit") || !mapCommandHasEdge(filtered.Edges, "edge.fixture.field.required") {
		t.Fatalf("filtered fixture target should keep replay and fixture edges: %#v", filtered.Edges)
	}
}

func mapCommandProfileCatalogFixture() store.ProfileCatalog {
	return store.ProfileCatalog{
		ProfileID: "profile.flow",
		Workflows: []store.CatalogWorkflow{{ID: "workflow.flow.create", DisplayName: "Create Flow"}},
		APICases: []store.CatalogAPICase{
			{ID: "case.prepare", DisplayName: "Prepare", NodeID: "node.prepare", RequestTemplateID: "template.prepare", Status: "active", SortOrder: 1},
			{ID: "case.submit.success", DisplayName: "Submit success", NodeID: "node.submit", RequestTemplateID: "template.submit", Status: "active", SortOrder: 2},
			{ID: "case.submit.field.required", DisplayName: "Field required", NodeID: "node.submit", CaseType: "negative", RequestTemplateID: "template.submit", RenderMode: "template_patch", PatchJSON: `[{"op":"remove","path":"$.body.field"}]`, ExpectedJSON: `{"status":400}`, Status: "active", SortOrder: 3},
		},
		WorkflowBindings: []store.CatalogWorkflowBinding{
			{WorkflowID: "workflow.flow.create", StepID: "step.prepare", NodeID: "node.prepare", CaseID: "case.prepare", Required: true, SortOrder: 1},
			{WorkflowID: "workflow.flow.create", StepID: "step.submit", NodeID: "node.submit", CaseID: "case.submit.success", Required: true, SortOrder: 2},
		},
		Fixtures: []store.CatalogFixture{{
			ID: "fixture.before.submit", DisplayName: "Before submit", Kind: "workflow_prefix", SourceWorkflowID: "workflow.flow.create", SourceUntilStep: "step.prepare", Status: "active",
		}},
		CaseDependencies: []store.CatalogCaseDependency{{
			ID: "dependency.field.required", CaseID: "case.submit.field.required", FixtureID: "fixture.before.submit", Required: true, MappingsJSON: `[]`,
		}},
	}
}

func mapCommandHasNode(nodes []store.TestPlanNode, id string) bool {
	for _, node := range nodes {
		if node.ID == id {
			return true
		}
	}
	return false
}

func mapCommandHasEdge(edges []store.TestPlanEdge, id string) bool {
	for _, edge := range edges {
		if edge.ID == id {
			return true
		}
	}
	return false
}
