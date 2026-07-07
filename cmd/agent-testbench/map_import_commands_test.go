package main

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"agent-testbench/internal/store"
)

func TestMapImportWorkflowsAndExplainUsesStoreCatalog(t *testing.T) {
	storePath := filepath.Join(t.TempDir(), "map.sqlite")
	storeRef := "sqlite://" + storePath
	seedMapCommandProfileCatalog(t, storeRef, mapCommandProfileCatalogFixture())

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
	storePath := filepath.Join(t.TempDir(), "map-filtered.sqlite")
	storeRef := "sqlite://" + storePath
	seedMapCommandProfileCatalog(t, storeRef, mapCommandProfileCatalogFixture())

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

func TestMapImportWorkflowsAppendPreservesExistingPaths(t *testing.T) {
	storePath := filepath.Join(t.TempDir(), "map-append.sqlite")
	storeRef := "sqlite://" + storePath
	seedMapCommandProfileCatalog(t, storeRef, mapCommandProfileCatalogFixture())

	runCLI(t, "map", "import-workflows", "--store", storeRef, "--json")
	seedMapCommandProfileCatalog(t, storeRef, mapCommandProfileCatalogWithAuditWorkflow())

	appendOut := runCLI(t, "map", "import-workflows", "--store", storeRef, "--map", "map.profile.flow", "--workflow", "workflow.flow.audit", "--append", "--json")
	var appendReport struct {
		OK     bool   `json:"ok"`
		Mode   string `json:"mode"`
		Before struct {
			Paths int `json:"paths"`
		} `json:"before"`
		Imported struct {
			Paths     int `json:"paths"`
			PathSteps int `json:"pathSteps"`
		} `json:"imported"`
		Counts struct {
			Paths     int `json:"paths"`
			PathSteps int `json:"pathSteps"`
		} `json:"counts"`
	}
	if err := json.Unmarshal([]byte(appendOut), &appendReport); err != nil {
		t.Fatalf("decode append import json: %v\n%s", err, appendOut)
	}
	if !appendReport.OK || appendReport.Mode != "append" || appendReport.Before.Paths != 1 || appendReport.Imported.Paths != 1 || appendReport.Counts.Paths != 2 || appendReport.Counts.PathSteps != 4 {
		t.Fatalf("append import report = %#v", appendReport)
	}

	workflowsOut := runCLI(t, "map", "workflows", "--store", storeRef, "--map", "map.profile.flow", "--json")
	var workflowsReport struct {
		OK        bool `json:"ok"`
		Count     int  `json:"count"`
		Workflows []struct {
			WorkflowID string `json:"workflowId"`
		} `json:"workflows"`
	}
	if err := json.Unmarshal([]byte(workflowsOut), &workflowsReport); err != nil {
		t.Fatalf("decode appended map workflows json: %v\n%s", err, workflowsOut)
	}
	if !workflowsReport.OK || workflowsReport.Count != 2 ||
		!mapCommandWorkflowReportHasWorkflow(workflowsReport.Workflows, "workflow.flow.create") ||
		!mapCommandWorkflowReportHasWorkflow(workflowsReport.Workflows, "workflow.flow.audit") {
		t.Fatalf("appended map workflows report = %#v", workflowsReport)
	}
}

func TestMapImportWorkflowsAppendRejectsMergedCycle(t *testing.T) {
	ctx := context.Background()
	storePath := filepath.Join(t.TempDir(), "map-append-cycle.sqlite")
	storeRef := "sqlite://" + storePath
	runtime, err := openStore(ctx, storeRef)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	if err := runtime.ReplaceTestPlanGraph(ctx, store.TestPlanGraph{
		Map: store.TestPlanMap{ID: "map.profile.flow", ProfileID: "profile.flow"},
		Nodes: []store.TestPlanNode{
			{MapID: "map.profile.flow", ID: "case.prepare", CaseID: "case.prepare"},
			{MapID: "map.profile.flow", ID: "case.submit.success", CaseID: "case.submit.success"},
		},
		Paths: []store.TestPlanPath{{
			MapID: "map.profile.flow", ID: "workflow.reverse", WorkflowID: "workflow.reverse",
		}},
		PathSteps: []store.TestPlanPathStep{
			{MapID: "map.profile.flow", PathID: "workflow.reverse", StepIndex: 1, NodeID: "case.submit.success", CaseID: "case.submit.success"},
			{MapID: "map.profile.flow", PathID: "workflow.reverse", StepIndex: 2, NodeID: "case.prepare", CaseID: "case.prepare"},
		},
		Edges: []store.TestPlanEdge{{
			MapID: "map.profile.flow", ID: "edge.reverse", FromNodeID: "case.submit.success", ToNodeID: "case.prepare", Kind: "control", PathID: "workflow.reverse",
		}},
	}); err != nil {
		t.Fatalf("seed reverse graph: %v", err)
	}
	closeCLIStore(runtime)
	seedMapCommandProfileCatalog(t, storeRef, mapCommandProfileCatalogFixture())

	out := runCLIFails(t, "map", "import-workflows", "--store", storeRef, "--map", "map.profile.flow", "--workflow", "workflow.flow.create", "--append", "--json")
	if !strings.Contains(out, "plan graph contains cycle") {
		t.Fatalf("append cycle should fail before storing graph:\n%s", out)
	}

	runtime, err = openStore(ctx, storeRef)
	if err != nil {
		t.Fatalf("reopen store: %v", err)
	}
	defer closeCLIStore(runtime)
	graph, err := runtime.GetTestPlanGraph(ctx, "map.profile.flow")
	if err != nil {
		t.Fatalf("get graph after failed append: %v", err)
	}
	if len(graph.Edges) != 1 || graph.Edges[0].ID != "edge.reverse" {
		t.Fatalf("failed append should not replace stored graph: %#v", graph.Edges)
	}
}

func TestMapImportWorkflowsAppendDropsStaleFixtureDataForRefreshedPath(t *testing.T) {
	ctx := context.Background()
	storePath := filepath.Join(t.TempDir(), "map-append-stale-fixture.sqlite")
	storeRef := "sqlite://" + storePath
	seedMapCommandProfileCatalog(t, storeRef, mapCommandProfileCatalogFixture())
	runCLI(t, "map", "import-workflows", "--store", storeRef, "--json")

	seedMapCommandProfileCatalog(t, storeRef, mapCommandProfileCatalogWithoutFixtures())
	runCLI(t, "map", "import-workflows", "--store", storeRef, "--map", "map.profile.flow", "--workflow", "workflow.flow.create", "--append", "--json")

	runtime, err := openStore(ctx, storeRef)
	if err != nil {
		t.Fatalf("reopen store: %v", err)
	}
	defer closeCLIStore(runtime)
	graph, err := runtime.GetTestPlanGraph(ctx, "map.profile.flow")
	if err != nil {
		t.Fatalf("get refreshed graph: %v", err)
	}
	if len(graph.Materializations) != 0 {
		t.Fatalf("refreshed path should drop stale materializations: %#v", graph.Materializations)
	}
	for _, edge := range graph.Edges {
		if edge.Kind == "fixture" || edge.MaterializationID != "" {
			t.Fatalf("refreshed path should drop stale fixture edges: %#v", graph.Edges)
		}
	}
}

func TestMapImportWorkflowsReplaceWarnsWhenFilteredImportShrinksExistingMap(t *testing.T) {
	storePath := filepath.Join(t.TempDir(), "map-replace-warning.sqlite")
	storeRef := "sqlite://" + storePath
	seedMapCommandProfileCatalog(t, storeRef, mapCommandProfileCatalogWithAuditWorkflow())

	runCLI(t, "map", "import-workflows", "--store", storeRef, "--json")
	replaceOut := runCLI(t, "map", "import-workflows", "--store", storeRef, "--map", "map.profile.flow", "--workflow", "workflow.flow.audit", "--json")
	var replaceReport struct {
		OK       bool     `json:"ok"`
		Mode     string   `json:"mode"`
		Warnings []string `json:"warnings"`
		Before   struct {
			Paths int `json:"paths"`
		} `json:"before"`
		Counts struct {
			Paths int `json:"paths"`
		} `json:"counts"`
	}
	if err := json.Unmarshal([]byte(replaceOut), &replaceReport); err != nil {
		t.Fatalf("decode replace import json: %v\n%s", err, replaceOut)
	}
	if !replaceReport.OK || replaceReport.Mode != "replace" || replaceReport.Before.Paths != 2 || replaceReport.Counts.Paths != 1 || len(replaceReport.Warnings) != 1 || !strings.Contains(replaceReport.Warnings[0], "--append") {
		t.Fatalf("replace warning report = %#v", replaceReport)
	}
}

func mapCommandProfileCatalogWithoutFixtures() store.ProfileCatalog {
	catalog := mapCommandProfileCatalogFixture()
	catalog.Fixtures = nil
	catalog.CaseDependencies = nil
	return catalog
}

func TestMapImportWorkflowsRejectsPositionalArgsBeforeOpeningStore(t *testing.T) {
	storePath := filepath.Join(t.TempDir(), "map.sqlite")
	out := runCLIFails(t, "map", "import-workflows", "typo", "--store", "sqlite://"+storePath, "--json")
	if !strings.Contains(out, "does not accept positional arguments") {
		t.Fatalf("unexpected import-workflows positional arg error:\n%s", out)
	}
}
