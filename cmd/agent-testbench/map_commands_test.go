package main

import (
	"context"
	"encoding/json"
	"path/filepath"
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
	if !importReport.OK || importReport.Map.ID != "map.profile.withdraw" || importReport.Counts.Nodes != 3 || importReport.Counts.Paths != 1 || importReport.Counts.Materializations != 1 {
		t.Fatalf("map import report = %#v", importReport)
	}

	explainOut := runCLI(t, "map", "explain", "--store", storeRef, "--map", "map.profile.withdraw", "--case", "case.apply.days.required", "--json")
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
	if !explainReport.OK || explainReport.TargetCaseID != "case.apply.days.required" || len(explainReport.Operations) != 2 {
		t.Fatalf("map explain report = %#v", explainReport)
	}
	if explainReport.Operations[0].Kind != "run_path_prefix" || explainReport.Operations[0].PathID != "workflow.withdraw.success" || explainReport.Operations[0].UntilNodeID != "case.quote" {
		t.Fatalf("prefix operation = %#v", explainReport.Operations[0])
	}
	if explainReport.Operations[1].Kind != "run_case" || explainReport.Operations[1].CaseID != "case.apply.days.required" {
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
	if report.Count != 3 {
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
			{MapID: "map.contract", ID: "case.quote", CaseID: "case.quote", Role: "primary", StateEffect: "advance", SummaryJSON: `{}`, SortOrder: 1},
			{MapID: "map.contract", ID: "case.apply", CaseID: "case.apply", Role: "primary", StateEffect: "advance", SummaryJSON: `{}`, SortOrder: 2},
			{MapID: "map.contract", ID: "case.cancel", CaseID: "case.cancel", Role: "primary", StateEffect: "advance", SummaryJSON: `{}`, SortOrder: 3},
		},
		Paths: []store.TestPlanPath{
			{MapID: "map.contract", ID: "workflow.apply.success", WorkflowID: "workflow.apply.success", DisplayName: "Apply Success", Status: "active", SummaryJSON: `{}`, SortOrder: 1},
			{MapID: "map.contract", ID: "workflow.cancel.success", WorkflowID: "workflow.cancel.success", DisplayName: "Cancel Success", Status: "active", SummaryJSON: `{}`, SortOrder: 2},
		},
		PathSteps: []store.TestPlanPathStep{
			{MapID: "map.contract", PathID: "workflow.apply.success", StepIndex: 1, StepID: "quote", NodeID: "case.quote", CaseID: "case.quote", Required: true, SummaryJSON: `{}`},
			{MapID: "map.contract", PathID: "workflow.apply.success", StepIndex: 2, StepID: "apply", NodeID: "case.apply", CaseID: "case.apply", Required: true, SummaryJSON: `{}`},
			{MapID: "map.contract", PathID: "workflow.cancel.success", StepIndex: 1, StepID: "apply", NodeID: "case.apply", CaseID: "case.apply", Required: true, SummaryJSON: `{}`},
			{MapID: "map.contract", PathID: "workflow.cancel.success", StepIndex: 2, StepID: "cancel", NodeID: "case.cancel", CaseID: "case.cancel", Required: true, SummaryJSON: `{}`},
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
	if workflow.WorkflowID != "workflow.cancel.success" || workflow.StepCount != 2 || workflow.FirstNodeID != "case.apply" || workflow.LastNodeID != "case.cancel" {
		t.Fatalf("workflow row = %#v", workflow)
	}
}

func mapCommandProfileCatalogFixture() store.ProfileCatalog {
	return store.ProfileCatalog{
		ProfileID: "profile.withdraw",
		Workflows: []store.CatalogWorkflow{{ID: "workflow.withdraw.success", DisplayName: "Withdraw Success"}},
		APICases: []store.CatalogAPICase{
			{ID: "case.quote", DisplayName: "Quote", NodeID: "node.quote", RequestTemplateID: "template.quote", Status: "active", SortOrder: 1},
			{ID: "case.apply.success", DisplayName: "Apply success", NodeID: "node.apply", RequestTemplateID: "template.apply", Status: "active", SortOrder: 2},
			{ID: "case.apply.days.required", DisplayName: "Days required", NodeID: "node.apply", CaseType: "negative", RequestTemplateID: "template.apply", RenderMode: "template_patch", PatchJSON: `[{"op":"remove","path":"$.body.days"}]`, ExpectedJSON: `{"status":400}`, Status: "active", SortOrder: 3},
		},
		WorkflowBindings: []store.CatalogWorkflowBinding{
			{WorkflowID: "workflow.withdraw.success", StepID: "step.quote", NodeID: "node.quote", CaseID: "case.quote", Required: true, SortOrder: 1},
			{WorkflowID: "workflow.withdraw.success", StepID: "step.apply", NodeID: "node.apply", CaseID: "case.apply.success", Required: true, SortOrder: 2},
		},
		Fixtures: []store.CatalogFixture{{
			ID: "fixture.before.apply", DisplayName: "Before apply", Kind: "workflow_prefix", SourceWorkflowID: "workflow.withdraw.success", SourceUntilStep: "step.quote", Status: "active",
		}},
		CaseDependencies: []store.CatalogCaseDependency{{
			ID: "dependency.days.required", CaseID: "case.apply.days.required", FixtureID: "fixture.before.apply", Required: true, MappingsJSON: `[]`,
		}},
	}
}
