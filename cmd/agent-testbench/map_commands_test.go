package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"agent-testbench/internal/domain/mapplanner"
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

func TestMapImportWorkflowsCanLimitImportedWorkflowPaths(t *testing.T) {
	ctx := context.Background()
	storePath := filepath.Join(t.TempDir(), "map-filtered.sqlite")
	storeRef := "sqlite://" + storePath
	runtime, err := openStore(ctx, storeRef)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	if err := runtime.ReplaceProfileCatalog(ctx, mapCommandProfileCatalogFixture()); err != nil {
		t.Fatalf("seed profile catalog: %v", err)
	}
	closeCLIStore(runtime)

	importOut := runCLI(t, "map", "import-workflows", "--store", storeRef, "--map", "map.filtered", "--workflow", "workflow.flow.create", "--json")
	var importReport struct {
		OK     bool `json:"ok"`
		Counts struct {
			Paths     int `json:"paths"`
			PathSteps int `json:"pathSteps"`
		} `json:"counts"`
	}
	if err := json.Unmarshal([]byte(importOut), &importReport); err != nil {
		t.Fatalf("decode map import json: %v\n%s", err, importOut)
	}
	if !importReport.OK || importReport.Counts.Paths != 1 || importReport.Counts.PathSteps != 2 {
		t.Fatalf("filtered map import report = %#v", importReport)
	}

	workflowsOut := runCLI(t, "map", "workflows", "--store", storeRef, "--map", "map.filtered", "--json")
	var workflowsReport struct {
		OK        bool `json:"ok"`
		Count     int  `json:"count"`
		Workflows []struct {
			WorkflowID string `json:"workflowId"`
			StepCount  int    `json:"stepCount"`
		} `json:"workflows"`
	}
	if err := json.Unmarshal([]byte(workflowsOut), &workflowsReport); err != nil {
		t.Fatalf("decode map workflows json: %v\n%s", err, workflowsOut)
	}
	if !workflowsReport.OK || workflowsReport.Count != 1 || workflowsReport.Workflows[0].WorkflowID != "workflow.flow.create" || workflowsReport.Workflows[0].StepCount != 2 {
		t.Fatalf("filtered map workflows report = %#v", workflowsReport)
	}
}

func TestMapExplainScopeAllCanSavePlannerInstance(t *testing.T) {
	ctx := context.Background()
	storePath := filepath.Join(t.TempDir(), "map-explain-save.sqlite")
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
	out := runCLI(t, "map", "explain", "--store", storeRef, "--map", "map.profile.flow", "--scope", "all", "--environment", "env.local", "--save", "--json")
	var report struct {
		OK             bool   `json:"ok"`
		PlanID         string `json:"planId"`
		MapID          string `json:"mapId"`
		Scope          string `json:"scope"`
		EnvironmentID  string `json:"environmentId"`
		LogicalPlan    []any  `json:"logicalPlan"`
		RulesApplied   []any  `json:"rulesApplied"`
		CandidatePlans []any  `json:"candidatePlans"`
		PhysicalTasks  []struct {
			ID   string `json:"id"`
			Kind string `json:"kind"`
		} `json:"physicalTasks"`
		TaskEdges []any `json:"taskEdges"`
		Summary   struct {
			WorkflowTasks int `json:"workflowTasks"`
			CaseTasks     int `json:"caseTasks"`
			ReplayTasks   int `json:"replayTasks"`
		} `json:"summary"`
	}
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatalf("decode saved map explain json: %v\n%s", err, out)
	}
	if !report.OK || report.PlanID == "" || report.MapID != "map.profile.flow" || report.Scope != "all" || report.EnvironmentID != "env.local" {
		t.Fatalf("saved map explain report = %#v", report)
	}
	if len(report.LogicalPlan) == 0 || len(report.RulesApplied) == 0 || len(report.CandidatePlans) == 0 || len(report.PhysicalTasks) == 0 {
		t.Fatalf("planner explain should expose optimizer details: %#v", report)
	}
	if report.Summary.WorkflowTasks == 0 || report.Summary.CaseTasks == 0 || report.Summary.ReplayTasks != 0 {
		t.Fatalf("planner summary should include workflow and case tasks: %#v", report.Summary)
	}
	if !mapExplainHasTaskKind(report.PhysicalTasks, "reuse_materialization") {
		t.Fatalf("planner should include materialized replay task: %#v", report.PhysicalTasks)
	}

	runtime, err = openStore(ctx, storeRef)
	if err != nil {
		t.Fatalf("reopen store: %v", err)
	}
	defer closeCLIStore(runtime)
	saved, err := runtime.GetTestMapPlan(ctx, report.PlanID)
	if err != nil {
		t.Fatalf("get saved planner instance: %v", err)
	}
	if saved.Instance.MapID != "map.profile.flow" || saved.Instance.Scope != "all" || saved.Instance.EnvironmentID != "env.local" {
		t.Fatalf("saved instance = %#v", saved.Instance)
	}
	if len(saved.Tasks) != len(report.PhysicalTasks) {
		t.Fatalf("saved tasks = %#v, report tasks = %#v", saved.Tasks, report.PhysicalTasks)
	}
}

func mapExplainHasTaskKind(tasks []struct {
	ID   string `json:"id"`
	Kind string `json:"kind"`
}, kind string) bool {
	for _, task := range tasks {
		if task.Kind == kind {
			return true
		}
	}
	return false
}

func TestMapImportWorkflowsRejectsPositionalArgsBeforeOpeningStore(t *testing.T) {
	storePath := filepath.Join(t.TempDir(), "map.sqlite")
	out := runCLIFails(t, "map", "import-workflows", "typo", "--store", "sqlite://"+storePath, "--json")
	if !strings.Contains(out, "does not accept positional arguments") {
		t.Fatalf("unexpected import-workflows positional arg error:\n%s", out)
	}
}

func TestMapCommandsAreDiscoverable(t *testing.T) {
	out := runCLI(t, "commands", "--all", "--area", "map", "--filter", "map", "--json")
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
	wantCommands := []string{
		"map import-workflows",
		"map list",
		"map plans",
		"map update",
		"map snapshot",
		"map publish",
		"map versions",
		"map coverage",
		"map doctor",
		"map diff",
		"map validation list",
		"map validation attach",
		"map workflows",
		"map inspect",
		"map explain",
		"map gate",
		"map run",
		"map plan inspect",
		"map atlas",
	}
	if report.Count != len(wantCommands) {
		t.Fatalf("map command count = %#v", report)
	}
	for index, want := range wantCommands {
		if report.Commands[index].Command != want || !report.Commands[index].StoreAware {
			t.Fatalf("map command %d = %#v, want %s", index, report.Commands[index], want)
		}
	}
}

func TestMapListAndPlansExposeAtlasEntrypoints(t *testing.T) {
	ctx := context.Background()
	storePath := filepath.Join(t.TempDir(), "map-list.sqlite")
	storeRef := "sqlite://" + storePath
	runtime, err := openStore(ctx, storeRef)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	graph := store.TestPlanGraph{
		Map: store.TestPlanMap{ID: "map.atlas", ProfileID: "profile.atlas", DisplayName: "Capability Atlas", Status: "active", SummaryJSON: `{}`},
		Nodes: []store.TestPlanNode{
			{MapID: "map.atlas", ID: "node.submit", CaseID: "case.submit", SummaryJSON: `{}`},
			{MapID: "map.atlas", ID: "node.cancel", CaseID: "case.cancel", SummaryJSON: `{}`},
		},
		Paths: []store.TestPlanPath{
			{MapID: "map.atlas", ID: "path.submit", WorkflowID: "workflow.submit", DisplayName: "Submit", SummaryJSON: `{}`},
		},
		PathSteps: []store.TestPlanPathStep{
			{MapID: "map.atlas", PathID: "path.submit", StepIndex: 1, NodeID: "node.submit", CaseID: "case.submit", SummaryJSON: `{}`},
		},
		Edges: []store.TestPlanEdge{
			{MapID: "map.atlas", ID: "edge.submit.cancel", FromNodeID: "node.submit", ToNodeID: "node.cancel", Kind: "control", SummaryJSON: `{}`},
		},
		Materializations: []store.TestPlanMaterialization{
			{MapID: "map.atlas", ID: "mat.submit", FixtureID: "fixture.submit", SummaryJSON: `{}`},
		},
	}
	if err := runtime.ReplaceTestPlanGraph(ctx, graph); err != nil {
		t.Fatalf("seed test plan graph: %v", err)
	}
	record := store.TestMapPlanRecord{
		Instance: store.TestMapPlanInstance{
			ID: "plan.atlas.001", MapID: "map.atlas", ProfileID: "profile.atlas", EnvironmentID: "env.atlas",
			Scope: "all", TargetKind: "map", TargetID: "map.atlas", Mode: "run", Status: "failed", SummaryJSON: `{}`,
		},
		Tasks: []store.TestMapPlanTask{{
			PlanID: "plan.atlas.001", ID: "task.case", Index: 1, Kind: "case", Operation: "run_case",
			NodeID: "node.submit", CaseID: "case.submit", Status: "failed", Reason: "HTTP 400", SummaryJSON: `{}`,
		}},
	}
	if err := runtime.SaveTestMapPlan(ctx, record); err != nil {
		t.Fatalf("seed test map plan: %v", err)
	}
	closeCLIStore(runtime)

	listOut := runCLI(t, "map", "list", "--store", storeRef, "--json")
	var listReport struct {
		OK    bool `json:"ok"`
		Count int  `json:"count"`
		Maps  []struct {
			ID               string `json:"id"`
			ProfileID        string `json:"profileId"`
			DisplayName      string `json:"displayName"`
			Status           string `json:"status"`
			NodeCount        int    `json:"nodeCount"`
			PathCount        int    `json:"pathCount"`
			Materializations int    `json:"materializations"`
		} `json:"maps"`
	}
	if err := json.Unmarshal([]byte(listOut), &listReport); err != nil {
		t.Fatalf("decode map list json: %v\n%s", err, listOut)
	}
	if !listReport.OK || listReport.Count != 1 {
		t.Fatalf("map list report = %#v", listReport)
	}
	item := listReport.Maps[0]
	if item.ID != "map.atlas" || item.ProfileID != "profile.atlas" || item.DisplayName != "Capability Atlas" || item.Status != "active" || item.NodeCount != 2 || item.PathCount != 1 || item.Materializations != 1 {
		t.Fatalf("map list item = %#v", item)
	}

	plansOut := runCLI(t, "map", "plans", "--store", storeRef, "--map", "map.atlas", "--json")
	var plansReport struct {
		OK    bool   `json:"ok"`
		MapID string `json:"mapId"`
		Count int    `json:"count"`
		Plans []struct {
			ID            string `json:"id"`
			Status        string `json:"status"`
			Mode          string `json:"mode"`
			Scope         string `json:"scope"`
			EnvironmentID string `json:"environmentId"`
			AtlasCommand  string `json:"atlasCommand"`
			GateCommand   string `json:"gateCommand"`
		} `json:"plans"`
	}
	if err := json.Unmarshal([]byte(plansOut), &plansReport); err != nil {
		t.Fatalf("decode map plans json: %v\n%s", err, plansOut)
	}
	if !plansReport.OK || plansReport.MapID != "map.atlas" || plansReport.Count != 1 {
		t.Fatalf("map plans report = %#v", plansReport)
	}
	plan := plansReport.Plans[0]
	if plan.ID != "plan.atlas.001" || plan.Status != "failed" || plan.Mode != "run" || plan.Scope != "all" || plan.EnvironmentID != "env.atlas" {
		t.Fatalf("map plan item = %#v", plan)
	}
	if !strings.Contains(plan.AtlasCommand, "map atlas --map 'map.atlas' --plan 'plan.atlas.001'") || !strings.Contains(plan.GateCommand, "map gate --plan 'plan.atlas.001'") {
		t.Fatalf("map plan commands = %#v", plan)
	}
}

func TestMapInspectRoutesListWorkflowsCoverageAndPlansViews(t *testing.T) {
	storeRef := seedMapInspectFixture(t)

	listOut := runCLI(t, "map", "inspect", "--view", "list", "--store", storeRef, "--json")
	var listReport struct {
		OK    bool `json:"ok"`
		Count int  `json:"count"`
		Maps  []struct {
			ID string `json:"id"`
		} `json:"maps"`
	}
	if err := json.Unmarshal([]byte(listOut), &listReport); err != nil {
		t.Fatalf("decode map inspect list json: %v\n%s", err, listOut)
	}
	if !listReport.OK || listReport.Count != 1 || listReport.Maps[0].ID != "map.inspect" {
		t.Fatalf("map inspect list report = %#v", listReport)
	}

	workflowsOut := runCLI(t, "map", "inspect", "--view", "workflows", "--store", storeRef, "--map", "map.inspect", "--json")
	var workflowsReport struct {
		OK        bool `json:"ok"`
		Count     int  `json:"count"`
		Workflows []struct {
			WorkflowID string `json:"workflowId"`
			StepCount  int    `json:"stepCount"`
		} `json:"workflows"`
	}
	if err := json.Unmarshal([]byte(workflowsOut), &workflowsReport); err != nil {
		t.Fatalf("decode map inspect workflows json: %v\n%s", err, workflowsOut)
	}
	if !workflowsReport.OK || workflowsReport.Count != 1 || workflowsReport.Workflows[0].WorkflowID != "workflow.submit" || workflowsReport.Workflows[0].StepCount != 2 {
		t.Fatalf("map inspect workflows report = %#v", workflowsReport)
	}

	coverageOut := runCLI(t, "map", "inspect", "--view", "coverage", "--store", storeRef, "--map", "map.inspect", "--json")
	var coverageReport mapCoverageReport
	if err := json.Unmarshal([]byte(coverageOut), &coverageReport); err != nil {
		t.Fatalf("decode map inspect coverage json: %v\n%s", err, coverageOut)
	}
	if !coverageReport.OK || coverageReport.MapID != "map.inspect" || coverageReport.Cases.Nodes != 2 || coverageReport.Cases.PathReferences != 2 {
		t.Fatalf("map inspect coverage report = %#v", coverageReport)
	}

	plansOut := runCLI(t, "map", "inspect", "--view", "plans", "--store", storeRef, "--map", "map.inspect", "--json")
	var plansReport struct {
		OK    bool   `json:"ok"`
		MapID string `json:"mapId"`
		Plans []struct {
			ID string `json:"id"`
		} `json:"plans"`
	}
	if err := json.Unmarshal([]byte(plansOut), &plansReport); err != nil {
		t.Fatalf("decode map inspect plans json: %v\n%s", err, plansOut)
	}
	if !plansReport.OK || plansReport.MapID != "map.inspect" || len(plansReport.Plans) != 1 || plansReport.Plans[0].ID != "plan.inspect.001" {
		t.Fatalf("map inspect plans report = %#v", plansReport)
	}
}

func TestMapInspectRoutesPlanViewAndUnknownView(t *testing.T) {
	storeRef := seedMapInspectFixture(t)

	planOut := runCLI(t, "map", "inspect", "--view", "plan", "--store", storeRef, "--plan", "plan.inspect.001", "--json")
	var planReport mapRunExplainCommandReport
	if err := json.Unmarshal([]byte(planOut), &planReport); err != nil {
		t.Fatalf("decode map inspect plan json: %v\n%s", err, planOut)
	}
	if planReport.OK || planReport.PlanID != "plan.inspect.001" || planReport.Status != "failed" {
		t.Fatalf("map inspect plan report = %#v", planReport)
	}
	if !strings.Contains(strings.Join(planReport.NextActions, "\n"), "map inspect --view plan --plan 'plan.inspect.001'") {
		t.Fatalf("map inspect plan next actions = %#v", planReport.NextActions)
	}

	out := runCLIFails(t, "map", "inspect", "--view", "unknown", "--store", storeRef, "--map", "map.inspect")
	if !strings.Contains(out, "unknown map inspect view") {
		t.Fatalf("unknown map inspect view should fail clearly:\n%s", out)
	}
}

func seedMapInspectFixture(t *testing.T) string {
	t.Helper()
	ctx := context.Background()
	storePath := filepath.Join(t.TempDir(), "map-inspect.sqlite")
	storeRef := "sqlite://" + storePath
	runtime, err := openStore(ctx, storeRef)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	graph := store.TestPlanGraph{
		Map: store.TestPlanMap{ID: "map.inspect", ProfileID: "profile.inspect", DisplayName: "Inspect Map", Status: "active", SummaryJSON: `{}`},
		Nodes: []store.TestPlanNode{
			{MapID: "map.inspect", ID: "node.prepare", CaseID: "case.prepare", SummaryJSON: `{}`},
			{MapID: "map.inspect", ID: "node.submit", CaseID: "case.submit", SummaryJSON: `{}`},
		},
		Paths: []store.TestPlanPath{
			{MapID: "map.inspect", ID: "path.submit", WorkflowID: "workflow.submit", DisplayName: "Submit", SummaryJSON: `{}`},
		},
		PathSteps: []store.TestPlanPathStep{
			{MapID: "map.inspect", PathID: "path.submit", StepIndex: 1, NodeID: "node.prepare", CaseID: "case.prepare", SummaryJSON: `{}`},
			{MapID: "map.inspect", PathID: "path.submit", StepIndex: 2, NodeID: "node.submit", CaseID: "case.submit", SummaryJSON: `{}`},
		},
	}
	if err := runtime.ReplaceTestPlanGraph(ctx, graph); err != nil {
		t.Fatalf("seed test plan graph: %v", err)
	}
	if err := runtime.SaveTestMapPlan(ctx, store.TestMapPlanRecord{
		Instance: store.TestMapPlanInstance{
			ID: "plan.inspect.001", MapID: "map.inspect", ProfileID: "profile.inspect", EnvironmentID: "env.inspect",
			Scope: "all", TargetKind: "map", TargetID: "map.inspect", Mode: "run", Status: "failed", SummaryJSON: `{}`,
		},
		Tasks: []store.TestMapPlanTask{{
			PlanID: "plan.inspect.001", ID: "task.submit", Index: 1, Kind: "case", Operation: "run_case",
			NodeID: "node.submit", CaseID: "case.submit", Status: "failed", Reason: "HTTP 400", SummaryJSON: `{}`,
		}},
	}); err != nil {
		t.Fatalf("seed test map plan: %v", err)
	}
	closeCLIStore(runtime)
	return storeRef
}

func TestMapUpdateMaintainsMapMetadataWithoutLosingGraph(t *testing.T) {
	ctx := context.Background()
	storePath := filepath.Join(t.TempDir(), "map-update.sqlite")
	storeRef := "sqlite://" + storePath
	runtime, err := openStore(ctx, storeRef)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	graph := store.TestPlanGraph{
		Map: store.TestPlanMap{ID: "map.authoring", ProfileID: "profile.authoring", DisplayName: "Draft Map", Description: "old", Status: "draft", SummaryJSON: `{}`},
		Nodes: []store.TestPlanNode{
			{MapID: "map.authoring", ID: "node.prepare", CaseID: "case.prepare", SummaryJSON: `{}`},
			{MapID: "map.authoring", ID: "node.submit", CaseID: "case.submit", SummaryJSON: `{}`},
		},
		Paths: []store.TestPlanPath{
			{MapID: "map.authoring", ID: "path.submit", WorkflowID: "workflow.submit", DisplayName: "Submit", SummaryJSON: `{}`},
		},
		PathSteps: []store.TestPlanPathStep{
			{MapID: "map.authoring", PathID: "path.submit", StepIndex: 1, NodeID: "node.prepare", CaseID: "case.prepare", SummaryJSON: `{}`},
			{MapID: "map.authoring", PathID: "path.submit", StepIndex: 2, NodeID: "node.submit", CaseID: "case.submit", SummaryJSON: `{}`},
		},
		Edges: []store.TestPlanEdge{
			{MapID: "map.authoring", ID: "edge.prepare.submit", FromNodeID: "node.prepare", ToNodeID: "node.submit", Kind: "control", SummaryJSON: `{}`},
		},
	}
	if err := runtime.ReplaceTestPlanGraph(ctx, graph); err != nil {
		t.Fatalf("seed authoring graph: %v", err)
	}
	closeCLIStore(runtime)

	out := runCLI(t, "map", "update", "--store", storeRef, "--map", "map.authoring", "--display-name", "Published Atlas", "--description", "ready for reviewers", "--status", "review", "--json")
	var report struct {
		OK  bool `json:"ok"`
		Map struct {
			ID          string `json:"id"`
			DisplayName string `json:"displayName"`
			Description string `json:"description"`
			Status      string `json:"status"`
		} `json:"map"`
		Counts struct {
			Nodes     int `json:"nodes"`
			Edges     int `json:"edges"`
			Paths     int `json:"paths"`
			PathSteps int `json:"pathSteps"`
		} `json:"counts"`
	}
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatalf("decode map update json: %v\n%s", err, out)
	}
	if !report.OK || report.Map.ID != "map.authoring" || report.Map.DisplayName != "Published Atlas" || report.Map.Description != "ready for reviewers" || report.Map.Status != "review" {
		t.Fatalf("map update report = %#v", report)
	}
	if report.Counts.Nodes != 2 || report.Counts.Edges != 1 || report.Counts.Paths != 1 || report.Counts.PathSteps != 2 {
		t.Fatalf("map update should preserve graph counts: %#v", report.Counts)
	}

	runtime, err = openStore(ctx, storeRef)
	if err != nil {
		t.Fatalf("reopen store: %v", err)
	}
	loaded, err := runtime.GetTestPlanGraph(ctx, "map.authoring")
	closeCLIStore(runtime)
	if err != nil {
		t.Fatalf("load updated graph: %v", err)
	}
	if loaded.Map.DisplayName != "Published Atlas" || loaded.Map.Description != "ready for reviewers" || loaded.Map.Status != "review" || len(loaded.Nodes) != 2 || len(loaded.Edges) != 1 || len(loaded.Paths) != 1 || len(loaded.PathSteps) != 2 {
		t.Fatalf("updated graph = %#v", loaded)
	}
}

func TestMapSnapshotAndPublishCreateVersionedAtlas(t *testing.T) {
	ctx := context.Background()
	storePath := filepath.Join(t.TempDir(), "map-version.sqlite")
	storeRef := "sqlite://" + storePath
	runtime, err := openStore(ctx, storeRef)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	graph := store.TestPlanGraph{
		Map: store.TestPlanMap{ID: "map.versioned", ProfileID: "profile.versioned", DisplayName: "Versioned Atlas", Description: "draft", Status: "review", SummaryJSON: `{}`},
		Nodes: []store.TestPlanNode{
			{MapID: "map.versioned", ID: "node.prepare", CaseID: "case.prepare", SummaryJSON: `{}`},
			{MapID: "map.versioned", ID: "node.submit", CaseID: "case.submit", SummaryJSON: `{}`},
		},
		Paths: []store.TestPlanPath{
			{MapID: "map.versioned", ID: "path.submit", WorkflowID: "workflow.submit", DisplayName: "Submit", SummaryJSON: `{}`},
		},
		PathSteps: []store.TestPlanPathStep{
			{MapID: "map.versioned", PathID: "path.submit", StepIndex: 1, NodeID: "node.prepare", CaseID: "case.prepare", SummaryJSON: `{}`},
			{MapID: "map.versioned", PathID: "path.submit", StepIndex: 2, NodeID: "node.submit", CaseID: "case.submit", SummaryJSON: `{}`},
		},
		Edges: []store.TestPlanEdge{
			{MapID: "map.versioned", ID: "edge.prepare.submit", FromNodeID: "node.prepare", ToNodeID: "node.submit", Kind: "control", SummaryJSON: `{}`},
		},
	}
	if err := runtime.ReplaceTestPlanGraph(ctx, graph); err != nil {
		t.Fatalf("seed versioned graph: %v", err)
	}
	closeCLIStore(runtime)

	snapshotOut := runCLI(t, "map", "snapshot", "--store", storeRef, "--map", "map.versioned", "--version", "v1-review", "--status", "review", "--summary", "ready for review", "--json")
	var snapshot struct {
		OK      bool `json:"ok"`
		Version struct {
			ID      string `json:"id"`
			MapID   string `json:"mapId"`
			Version string `json:"version"`
			Status  string `json:"status"`
			Summary string `json:"summary"`
		} `json:"version"`
		Counts struct {
			Nodes int `json:"nodes"`
			Paths int `json:"paths"`
		} `json:"counts"`
	}
	if err := json.Unmarshal([]byte(snapshotOut), &snapshot); err != nil {
		t.Fatalf("decode map snapshot json: %v\n%s", err, snapshotOut)
	}
	if !snapshot.OK || snapshot.Version.MapID != "map.versioned" || snapshot.Version.Version != "v1-review" || snapshot.Version.Status != "review" || snapshot.Version.Summary != "ready for review" || snapshot.Counts.Nodes != 2 || snapshot.Counts.Paths != 1 {
		t.Fatalf("snapshot report = %#v", snapshot)
	}

	publishOut := runCLI(t, "map", "publish", "--store", storeRef, "--map", "map.versioned", "--version", "v1", "--summary", "accepted atlas", "--json")
	var publish struct {
		OK  bool `json:"ok"`
		Map struct {
			ID     string `json:"id"`
			Status string `json:"status"`
		} `json:"map"`
		Version struct {
			Version string `json:"version"`
			Status  string `json:"status"`
		} `json:"version"`
	}
	if err := json.Unmarshal([]byte(publishOut), &publish); err != nil {
		t.Fatalf("decode map publish json: %v\n%s", err, publishOut)
	}
	if !publish.OK || publish.Map.ID != "map.versioned" || publish.Map.Status != "active" || publish.Version.Version != "v1" || publish.Version.Status != "published" {
		t.Fatalf("publish report = %#v", publish)
	}

	versionsOut := runCLI(t, "map", "versions", "--store", storeRef, "--map", "map.versioned", "--json")
	var versions struct {
		OK       bool   `json:"ok"`
		MapID    string `json:"mapId"`
		Count    int    `json:"count"`
		Versions []struct {
			Version string `json:"version"`
			Status  string `json:"status"`
			Summary string `json:"summary"`
		} `json:"versions"`
	}
	if err := json.Unmarshal([]byte(versionsOut), &versions); err != nil {
		t.Fatalf("decode map versions json: %v\n%s", err, versionsOut)
	}
	if !versions.OK || versions.MapID != "map.versioned" || versions.Count != 2 {
		t.Fatalf("versions report = %#v", versions)
	}
	if versions.Versions[0].Version != "v1" || versions.Versions[0].Status != "published" || versions.Versions[1].Version != "v1-review" || versions.Versions[1].Status != "review" {
		t.Fatalf("version order = %#v", versions.Versions)
	}
}

func TestMapDoctorReportsBrokenGraphReferences(t *testing.T) {
	ctx := context.Background()
	storePath := filepath.Join(t.TempDir(), "map-doctor.sqlite")
	storeRef := "sqlite://" + storePath
	runtime, err := openStore(ctx, storeRef)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	graph := store.TestPlanGraph{
		Map: store.TestPlanMap{ID: "map.broken", ProfileID: "profile.broken", DisplayName: "Broken Map", Status: "draft", SummaryJSON: `{}`},
		Nodes: []store.TestPlanNode{
			{MapID: "map.broken", ID: "case.submit", CaseID: "case.submit", Role: "primary", StateEffect: "advance", SummaryJSON: `{}`},
			{MapID: "map.broken", ID: "case.submit.blank.invalid", CaseID: "case.submit.blank.invalid", Role: "validation", StateEffect: "unchanged", AnchorNodeID: "case.missing", BaseCaseID: "case.missing", SummaryJSON: `{}`},
		},
		Paths: []store.TestPlanPath{
			{MapID: "map.broken", ID: "workflow.submit", WorkflowID: "workflow.submit", DisplayName: "Submit", Status: "active", SummaryJSON: `{}`},
		},
		PathSteps: []store.TestPlanPathStep{
			{MapID: "map.broken", PathID: "workflow.submit", StepIndex: 1, StepID: "submit", NodeID: "case.submit", CaseID: "case.submit", Required: true, SummaryJSON: `{}`},
			{MapID: "map.broken", PathID: "workflow.submit", StepIndex: 2, StepID: "missing", NodeID: "case.missing", CaseID: "case.missing", Required: true, SummaryJSON: `{}`},
		},
		Edges: []store.TestPlanEdge{
			{MapID: "map.broken", ID: "edge.submit.missing", FromNodeID: "case.submit", ToNodeID: "case.missing", Kind: "control", PathID: "workflow.submit", Required: true, SummaryJSON: `{}`},
			{MapID: "map.broken", ID: "edge.fixture.invalid", FromNodeID: "case.submit", ToNodeID: "case.submit.blank.invalid", Kind: "fixture", MaterializationID: "fixture.missing", Required: true, SummaryJSON: `{}`},
		},
		Materializations: []store.TestPlanMaterialization{
			{MapID: "map.broken", ID: "fixture.replay", SourcePathID: "workflow.missing", SourceUntilNodeID: "case.submit", Status: "active", SummaryJSON: `{}`},
		},
	}
	if err := runtime.ReplaceTestPlanGraph(ctx, graph); err != nil {
		t.Fatalf("seed broken graph: %v", err)
	}
	closeCLIStore(runtime)

	out := runCLIFails(t, "map", "doctor", "--store", storeRef, "--map", "map.broken", "--json")
	var report struct {
		OK         bool `json:"ok"`
		IssueCount int  `json:"issueCount"`
		Checks     []struct {
			Code     string `json:"code"`
			OK       bool   `json:"ok"`
			Severity string `json:"severity"`
			Detail   string `json:"detail"`
			Fix      string `json:"fix"`
		} `json:"checks"`
	}
	if err := json.Unmarshal([]byte(jsonPrefix(out)), &report); err != nil {
		t.Fatalf("decode map doctor json: %v\n%s", err, out)
	}
	if report.OK || report.IssueCount < 4 {
		t.Fatalf("doctor should fail with structural issues: %#v", report)
	}
	for _, code := range []string{"path-step.node", "edge.to-node", "edge.materialization", "validation.anchor", "materialization.source-path"} {
		if !mapDoctorReportHasCode(report.Checks, code) {
			t.Fatalf("doctor report missing %s: %#v", code, report.Checks)
		}
	}
}

func TestMapDoctorAcceptsWorkflowPatchSmokeCaseWithoutValidationAnchor(t *testing.T) {
	ctx := context.Background()
	storePath := filepath.Join(t.TempDir(), "map-doctor-smoke.sqlite")
	storeRef := "sqlite://" + storePath
	runtime, err := openStore(ctx, storeRef)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	catalog := store.ProfileCatalog{
		ProfileID: "profile.smoke",
		Workflows: []store.CatalogWorkflow{
			{ID: "workflow.smoke", DisplayName: "Smoke"},
		},
		APICases: []store.CatalogAPICase{
			{
				ID: "case.smoke.patch", DisplayName: "Patch smoke", NodeID: "node.smoke", RequestTemplateID: "template.smoke",
				RenderMode: "template_patch", PatchJSON: `[{"op":"add","path":"$.body.trace","value":"smoke"}]`,
				Status: "active", SortOrder: 1,
			},
		},
		WorkflowBindings: []store.CatalogWorkflowBinding{
			{WorkflowID: "workflow.smoke", StepID: "step.smoke", NodeID: "node.smoke", CaseID: "case.smoke.patch", Required: true, SortOrder: 1},
		},
	}
	if err := runtime.ReplaceProfileCatalog(ctx, catalog); err != nil {
		t.Fatalf("seed catalog: %v", err)
	}
	closeCLIStore(runtime)

	runCLI(t, "map", "import-workflows", "--store", storeRef, "--json")
	out := runCLI(t, "map", "doctor", "--store", storeRef, "--map", "map.profile.smoke", "--json")
	var report struct {
		OK         bool `json:"ok"`
		IssueCount int  `json:"issueCount"`
		Counts     struct {
			Nodes int `json:"nodes"`
		} `json:"counts"`
	}
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatalf("decode map doctor json: %v\n%s", err, out)
	}
	if !report.OK || report.IssueCount != 0 || report.Counts.Nodes != 1 {
		t.Fatalf("workflow patch smoke case should not require validation anchor: %#v", report)
	}
}

func TestMapValidationAttachAndListGroupsByInterface(t *testing.T) {
	ctx := context.Background()
	storePath := filepath.Join(t.TempDir(), "map-validation.sqlite")
	storeRef := "sqlite://" + storePath
	runtime, err := openStore(ctx, storeRef)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	catalog := store.ProfileCatalog{
		ProfileID: "profile.validation",
		APICases: []store.CatalogAPICase{
			{ID: "case.submit.success", DisplayName: "Submit success", NodeID: "node.submit", RequestTemplateID: "template.submit", Status: "active", SortOrder: 1},
			{ID: "case.submit.length.invalid", DisplayName: "Submit length invalid", NodeID: "node.submit", RequestTemplateID: "template.submit", CaseType: "negative", RenderMode: "template_patch", PatchJSON: `[{"op":"replace","path":"$.body.name","value":"too-long"}]`, ExpectedJSON: `{"status":400}`, Status: "active", SortOrder: 2},
		},
	}
	if err := runtime.ReplaceProfileCatalog(ctx, catalog); err != nil {
		t.Fatalf("seed catalog: %v", err)
	}
	graph := store.TestPlanGraph{
		Map: store.TestPlanMap{ID: "map.validation", ProfileID: "profile.validation", DisplayName: "Validation Map", Status: "draft", SummaryJSON: `{}`},
		Nodes: []store.TestPlanNode{
			{MapID: "map.validation", ID: "case.submit.success", CaseID: "case.submit.success", InterfaceNodeID: "node.submit", RequestTemplateID: "template.submit", Role: "primary", StateEffect: "advance", SummaryJSON: `{}`},
		},
		Paths: []store.TestPlanPath{
			{MapID: "map.validation", ID: "workflow.submit", WorkflowID: "workflow.submit", DisplayName: "Submit", Status: "active", SummaryJSON: `{}`},
		},
		PathSteps: []store.TestPlanPathStep{
			{MapID: "map.validation", PathID: "workflow.submit", StepIndex: 1, StepID: "submit", NodeID: "case.submit.success", CaseID: "case.submit.success", Required: true, SummaryJSON: `{}`},
		},
	}
	if err := runtime.ReplaceTestPlanGraph(ctx, graph); err != nil {
		t.Fatalf("seed validation graph: %v", err)
	}
	closeCLIStore(runtime)

	attachOut := runCLI(t, "map", "validation", "attach", "--store", storeRef, "--map", "map.validation", "--anchor", "case.submit.success", "--case", "case.submit.length.invalid", "--json")
	var attach struct {
		OK   bool `json:"ok"`
		Node struct {
			ID           string `json:"id"`
			CaseID       string `json:"caseId"`
			AnchorNodeID string `json:"anchorNodeId"`
			BaseCaseID   string `json:"baseCaseId"`
			Role         string `json:"role"`
			StateEffect  string `json:"stateEffect"`
		} `json:"node"`
		Counts struct {
			Validation int `json:"validation"`
		} `json:"counts"`
	}
	if err := json.Unmarshal([]byte(attachOut), &attach); err != nil {
		t.Fatalf("decode validation attach json: %v\n%s", err, attachOut)
	}
	if !attach.OK || attach.Node.ID != "case.submit.length.invalid" || attach.Node.AnchorNodeID != "case.submit.success" || attach.Node.BaseCaseID != "case.submit.success" || attach.Node.Role != "validation" || attach.Node.StateEffect != "unchanged" || attach.Counts.Validation != 1 {
		t.Fatalf("validation attach report = %#v", attach)
	}

	listOut := runCLI(t, "map", "validation", "list", "--store", storeRef, "--map", "map.validation", "--interface", "node.submit", "--json")
	var list struct {
		OK     bool `json:"ok"`
		Groups []struct {
			InterfaceNodeID string `json:"interfaceNodeId"`
			AnchorNodeID    string `json:"anchorNodeId"`
			Count           int    `json:"count"`
			Families        []struct {
				Family string `json:"family"`
				Count  int    `json:"count"`
			} `json:"families"`
			Cases []struct {
				CaseID       string `json:"caseId"`
				AnchorNodeID string `json:"anchorNodeId"`
				Family       string `json:"family"`
			} `json:"cases"`
		} `json:"groups"`
	}
	if err := json.Unmarshal([]byte(listOut), &list); err != nil {
		t.Fatalf("decode validation list json: %v\n%s", err, listOut)
	}
	if !list.OK || len(list.Groups) != 1 || list.Groups[0].InterfaceNodeID != "node.submit" || list.Groups[0].AnchorNodeID != "case.submit.success" || list.Groups[0].Count != 1 {
		t.Fatalf("validation list groups = %#v", list.Groups)
	}
	if len(list.Groups[0].Cases) != 1 || list.Groups[0].Cases[0].CaseID != "case.submit.length.invalid" || list.Groups[0].Cases[0].Family != "length" {
		t.Fatalf("validation list cases = %#v", list.Groups[0].Cases)
	}
}

func TestMapDiffComparesVersionAgainstWorkingGraph(t *testing.T) {
	ctx := context.Background()
	storePath := filepath.Join(t.TempDir(), "map-diff.sqlite")
	storeRef := "sqlite://" + storePath
	runtime, err := openStore(ctx, storeRef)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	graph := store.TestPlanGraph{
		Map: store.TestPlanMap{ID: "map.diff", ProfileID: "profile.diff", DisplayName: "Diff Map", Status: "review", SummaryJSON: `{}`},
		Nodes: []store.TestPlanNode{
			{MapID: "map.diff", ID: "case.submit.success", CaseID: "case.submit.success", InterfaceNodeID: "node.submit", Role: "primary", StateEffect: "advance", SummaryJSON: `{}`},
		},
	}
	if err := runtime.ReplaceTestPlanGraph(ctx, graph); err != nil {
		t.Fatalf("seed diff graph: %v", err)
	}
	closeCLIStore(runtime)

	runCLI(t, "map", "snapshot", "--store", storeRef, "--map", "map.diff", "--version", "v1", "--status", "review", "--json")
	runtime, err = openStore(ctx, storeRef)
	if err != nil {
		t.Fatalf("reopen store: %v", err)
	}
	graph.Nodes = append(graph.Nodes, store.TestPlanNode{MapID: "map.diff", ID: "case.submit.blank.invalid", CaseID: "case.submit.blank.invalid", InterfaceNodeID: "node.submit", Role: "validation", StateEffect: "unchanged", BaseCaseID: "case.submit.success", AnchorNodeID: "case.submit.success", SummaryJSON: `{}`})
	if err := runtime.ReplaceTestPlanGraph(ctx, graph); err != nil {
		t.Fatalf("update working graph: %v", err)
	}
	closeCLIStore(runtime)

	out := runCLI(t, "map", "diff", "--store", storeRef, "--map", "map.diff", "--from", "v1", "--to", "working", "--json")
	var report struct {
		OK      bool   `json:"ok"`
		MapID   string `json:"mapId"`
		From    string `json:"from"`
		To      string `json:"to"`
		Changed bool   `json:"changed"`
		Nodes   struct {
			Before int      `json:"before"`
			After  int      `json:"after"`
			Added  []string `json:"added"`
		} `json:"nodes"`
	}
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatalf("decode map diff json: %v\n%s", err, out)
	}
	if !report.OK || !report.Changed || report.MapID != "map.diff" || report.From != "v1" || report.To != "working" || report.Nodes.Before != 1 || report.Nodes.After != 2 || strings.Join(report.Nodes.Added, ",") != "case.submit.blank.invalid" {
		t.Fatalf("map diff report = %#v", report)
	}
}

func mapDoctorReportHasCode(checks []struct {
	Code     string `json:"code"`
	OK       bool   `json:"ok"`
	Severity string `json:"severity"`
	Detail   string `json:"detail"`
	Fix      string `json:"fix"`
}, code string) bool {
	for _, check := range checks {
		if check.Code == code && !check.OK {
			return true
		}
	}
	return false
}

func TestMapCoverageReportsWorkflowConvergence(t *testing.T) {
	ctx := context.Background()
	storePath := filepath.Join(t.TempDir(), "map-coverage.sqlite")
	storeRef := "sqlite://" + storePath
	runtime, err := openStore(ctx, storeRef)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	catalog := store.ProfileCatalog{
		ProfileID: "profile.coverage",
		Workflows: []store.CatalogWorkflow{
			{ID: "workflow.apply", DisplayName: "Apply"},
			{ID: "workflow.cancel", DisplayName: "Cancel"},
		},
		APICases: []store.CatalogAPICase{
			{ID: "case.prepare", NodeID: "node.prepare", Status: "active"},
			{ID: "case.submit", NodeID: "node.submit", Status: "active"},
			{ID: "case.cancel", NodeID: "node.cancel", Status: "active"},
		},
	}
	if err := runtime.ReplaceProfileCatalog(ctx, catalog); err != nil {
		t.Fatalf("seed coverage catalog: %v", err)
	}
	graph := store.TestPlanGraph{
		Map: store.TestPlanMap{ID: "map.coverage", ProfileID: "profile.coverage", DisplayName: "Coverage Atlas", Status: "active", SummaryJSON: `{}`},
		Nodes: []store.TestPlanNode{
			{MapID: "map.coverage", ID: "node.prepare", CaseID: "case.prepare", SummaryJSON: `{}`},
			{MapID: "map.coverage", ID: "node.submit", CaseID: "case.submit", SummaryJSON: `{}`},
			{MapID: "map.coverage", ID: "node.cancel", CaseID: "case.cancel", SummaryJSON: `{}`},
		},
		Paths: []store.TestPlanPath{
			{MapID: "map.coverage", ID: "path.apply", WorkflowID: "workflow.apply", DisplayName: "Apply", SummaryJSON: `{}`},
			{MapID: "map.coverage", ID: "path.cancel", WorkflowID: "workflow.cancel", DisplayName: "Cancel", SummaryJSON: `{}`},
		},
		PathSteps: []store.TestPlanPathStep{
			{MapID: "map.coverage", PathID: "path.apply", StepIndex: 1, NodeID: "node.prepare", CaseID: "case.prepare", SummaryJSON: `{}`},
			{MapID: "map.coverage", PathID: "path.apply", StepIndex: 2, NodeID: "node.submit", CaseID: "case.submit", SummaryJSON: `{}`},
			{MapID: "map.coverage", PathID: "path.cancel", StepIndex: 1, NodeID: "node.submit", CaseID: "case.submit", SummaryJSON: `{}`},
			{MapID: "map.coverage", PathID: "path.cancel", StepIndex: 2, NodeID: "node.cancel", CaseID: "case.cancel", SummaryJSON: `{}`},
		},
		Edges: []store.TestPlanEdge{
			{MapID: "map.coverage", ID: "edge.prepare.submit", FromNodeID: "node.prepare", ToNodeID: "node.submit", Kind: "control", PathID: "path.apply", SummaryJSON: `{}`},
			{MapID: "map.coverage", ID: "edge.submit.cancel", FromNodeID: "node.submit", ToNodeID: "node.cancel", Kind: "control", PathID: "path.cancel", SummaryJSON: `{}`},
		},
		Materializations: []store.TestPlanMaterialization{
			{MapID: "map.coverage", ID: "mat.submit", FixtureID: "fixture.submit", SourcePathID: "path.apply", SourceUntilNodeID: "node.submit", SummaryJSON: `{}`},
		},
	}
	if err := runtime.ReplaceTestPlanGraph(ctx, graph); err != nil {
		t.Fatalf("seed coverage graph: %v", err)
	}
	closeCLIStore(runtime)

	out := runCLI(t, "map", "coverage", "--store", storeRef, "--map", "map.coverage", "--json")
	var report struct {
		OK        bool   `json:"ok"`
		MapID     string `json:"mapId"`
		Workflows struct {
			Catalog int `json:"catalog"`
			Mapped  int `json:"mapped"`
			Missing int `json:"missing"`
		} `json:"workflows"`
		Cases struct {
			Nodes          int `json:"nodes"`
			PathReferences int `json:"pathReferences"`
			Reused         int `json:"reused"`
		} `json:"cases"`
		Materializations int `json:"materializations"`
	}
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatalf("decode map coverage json: %v\n%s", err, out)
	}
	if !report.OK || report.MapID != "map.coverage" || report.Workflows.Catalog != 2 || report.Workflows.Mapped != 2 || report.Workflows.Missing != 0 {
		t.Fatalf("workflow coverage = %#v", report)
	}
	if report.Cases.Nodes != 3 || report.Cases.PathReferences != 4 || report.Cases.Reused != 1 || report.Materializations != 1 {
		t.Fatalf("case convergence = %#v", report)
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

func TestMapAtlasWritesInteractiveArtifact(t *testing.T) {
	ctx := context.Background()
	storePath := filepath.Join(t.TempDir(), "map-atlas.sqlite")
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
	outputPath := filepath.Join(t.TempDir(), "flow-map-atlas.html")
	out := runCLI(t, "map", "atlas", "--store", storeRef, "--map", "map.profile.flow", "--output", outputPath, "--json")
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
		t.Fatalf("decode map atlas json: %v\n%s", err, out)
	}
	if !report.OK || report.MapID != "map.profile.flow" || report.Output != outputPath || report.Counts.Nodes != 3 || report.Counts.Paths != 1 {
		t.Fatalf("map atlas report = %#v", report)
	}
	raw, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("read atlas: %v", err)
	}
	html := string(raw)
	for _, want := range []string{
		`id="map-atlas-data"`,
		`function selectNode`,
		`id="workflow-filter"`,
		`case.submit.field.required`,
		`Field required`,
		`template.submit`,
		`Test families`,
		`function interfaceReverseCases`,
		`function caseFamilySummaries`,
		`id="toggle-validation"`,
		`id="map-atlas-minimap"`,
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
			t.Fatalf("atlas missing %q\n%s", want, html)
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
			t.Fatalf("atlas should use JavaScript-string escaping for handler arguments %q:\n%s", unsafeHandler, html)
		}
	}
}

func TestMapAtlasOverlaysSavedPlanTasks(t *testing.T) {
	ctx := context.Background()
	storePath := filepath.Join(t.TempDir(), "map-atlas-plan.sqlite")
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
	runtime, err = openStore(ctx, storeRef)
	if err != nil {
		t.Fatalf("reopen store: %v", err)
	}
	now := time.Now().UTC()
	record := store.TestMapPlanRecord{
		Instance: store.TestMapPlanInstance{
			ID:             "plan.atlas.failed",
			MapID:          "map.profile.flow",
			ProfileID:      "profile.flow",
			Mode:           mapplanner.ModeRun,
			Status:         store.StatusFailed,
			Scope:          mapplanner.ScopeCases,
			TargetKind:     mapplanner.TargetMap,
			TargetID:       "map.profile.flow",
			PlannerVersion: mapplanner.PlannerVersion,
			StartedAt:      now,
			FinishedAt:     now,
		},
		Tasks: []store.TestMapPlanTask{{
			PlanID:       "plan.atlas.failed",
			ID:           "task.failed.case",
			Index:        1,
			Kind:         mapplanner.TaskRunCase,
			Operation:    mapplanner.TaskRunCase,
			NodeID:       "case.submit.field.required",
			CaseID:       "case.submit.field.required",
			Status:       store.StatusFailed,
			Reason:       "expected status mismatch",
			APICaseRunID: "run.atlas.failed.case",
			EvidenceRoot: ".runtime/evidence/run.atlas.failed",
			SummaryJSON:  `{"error":"expected status mismatch"}`,
			StartedAt:    now,
			FinishedAt:   now,
		}},
	}
	if err := runtime.SaveTestMapPlan(ctx, record); err != nil {
		t.Fatalf("save review plan: %v", err)
	}
	closeCLIStore(runtime)

	outputPath := filepath.Join(t.TempDir(), "flow-map-atlas-plan.html")
	out := runCLI(t, "map", "atlas", "--store", storeRef, "--map", "map.profile.flow", "--plan", "plan.atlas.failed", "--output", outputPath, "--json")
	var report struct {
		OK     bool   `json:"ok"`
		MapID  string `json:"mapId"`
		PlanID string `json:"planId"`
	}
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatalf("decode map atlas plan json: %v\n%s", err, out)
	}
	if !report.OK || report.MapID != "map.profile.flow" || report.PlanID != "plan.atlas.failed" {
		t.Fatalf("map atlas plan report = %#v", report)
	}
	raw, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("read atlas: %v", err)
	}
	html := string(raw)
	for _, want := range []string{
		`"planId":"plan.atlas.failed"`,
		`"status":"failed"`,
		`"apiCaseRunId":"run.atlas.failed.case"`,
		`"evidenceRoot":".runtime/evidence/run.atlas.failed"`,
		`Map run plan`,
		`Run tasks`,
	} {
		if !strings.Contains(html, want) {
			t.Fatalf("atlas with plan missing %q\n%s", want, html)
		}
	}
}

func TestMapAtlasCanFilterWorkflowPaths(t *testing.T) {
	ctx := context.Background()
	storePath := filepath.Join(t.TempDir(), "map-atlas-filter.sqlite")
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

	outputPath := filepath.Join(t.TempDir(), "cancel-map-atlas.html")
	out := runCLI(t, "map", "atlas", "--store", storeRef, "--map", "map.contract", "--filter", "cancel", "--output", outputPath, "--json")
	var report struct {
		OK     bool   `json:"ok"`
		Filter string `json:"filter"`
		Counts struct {
			Paths int `json:"paths"`
			Nodes int `json:"nodes"`
		} `json:"counts"`
	}
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatalf("decode filtered map atlas json: %v\n%s", err, out)
	}
	if !report.OK || report.Filter != "cancel" || report.Counts.Paths != 1 || report.Counts.Nodes != 2 {
		t.Fatalf("filtered review report = %#v", report)
	}
	raw, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("read filtered atlas: %v", err)
	}
	html := string(raw)
	if !strings.Contains(html, "workflow.cancel.success") {
		t.Fatalf("filtered atlas missing cancel workflow:\n%s", html)
	}
	if strings.Contains(html, "workflow.create.success") {
		t.Fatalf("filtered atlas should not include create workflow:\n%s", html)
	}
}

func TestMapAtlasHandlesWorkflowPathWithoutSteps(t *testing.T) {
	document := mapAtlasDocument{
		Version: "1.0",
		Map: store.TestPlanMap{
			ID:          "map.contract",
			ProfileID:   "profile.contract",
			DisplayName: "Contract Map",
			Status:      "active",
		},
		Counts: mapAtlasCountsReport{Paths: 1},
		Paths: []mapAtlasPath{{
			ID:          "workflow.empty",
			WorkflowID:  "workflow.empty",
			DisplayName: "Empty workflow",
			Status:      "active",
		}},
	}
	html, err := renderMapAtlasHTML(document)
	if err != nil {
		t.Fatalf("render atlas: %v", err)
	}
	if strings.Contains(html, `"steps":null`) {
		t.Fatalf("atlas should serialize empty path steps as an empty array:\n%s", html)
	}
	if strings.Contains(html, `p.steps.length`) {
		t.Fatalf("atlas should not assume workflow path steps are non-null:\n%s", html)
	}
}

func TestMapAtlasFilterIncludesReplayPathForFixtureTarget(t *testing.T) {
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

	filtered := filterMapAtlasGraph(graph, "field.required")
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
