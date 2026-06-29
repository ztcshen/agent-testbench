package main

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"agent-testbench/internal/domain/mapplanner"
	"agent-testbench/internal/store"
)

func TestMapValidationPromoteCommandIsDiscoverable(t *testing.T) {
	out := runCLI(t, "commands", "--all", "--filter", "map validation promote", "--json")
	if !strings.Contains(out, `"command": "map validation promote"`) {
		t.Fatalf("map validation promote command not discoverable:\n%s", out)
	}
}

func TestMapValidationPromoteTurnsStandaloneSmokeIntoExecutablePrimaryNode(t *testing.T) {
	ctx := context.Background()
	storePath := filepath.Join(t.TempDir(), "map-validation-promote.sqlite")
	storeRef := "sqlite://" + storePath
	runtime, err := openStore(ctx, storeRef)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	if err := runtime.ReplaceProfileCatalog(ctx, store.ProfileCatalog{
		ProfileID: "profile.promote",
		APICases: []store.CatalogAPICase{{
			ID: "case.smoke.patch", DisplayName: "Patch smoke", NodeID: "node.smoke", RequestTemplateID: "template.smoke",
			RenderMode: "template_patch", PatchJSON: `[{"op":"add","path":"$.trace","value":"smoke"}]`,
			Status: "active", SortOrder: 1,
		}},
	}); err != nil {
		t.Fatalf("seed catalog: %v", err)
	}
	graph := store.TestPlanGraph{
		Map: store.TestPlanMap{ID: "map.promote", ProfileID: "profile.promote", DisplayName: "Promote Map", Status: "draft", SummaryJSON: `{}`},
		Nodes: []store.TestPlanNode{{
			MapID: "map.promote", ID: "case.smoke.patch", CaseID: "case.smoke.patch", InterfaceNodeID: "node.smoke",
			RequestTemplateID: "template.smoke", Role: "validation", StateEffect: "unchanged", SummaryJSON: `{}`,
		}},
	}
	if err := runtime.ReplaceTestPlanGraph(ctx, graph); err != nil {
		t.Fatalf("seed validation graph: %v", err)
	}
	closeCLIStore(runtime)

	doctorOut := runCLIFails(t, "map", "doctor", "--store", storeRef, "--map", "map.promote", "--json")
	if !strings.Contains(doctorOut, "validation.anchor") {
		t.Fatalf("precondition should have validation anchor issue:\n%s", doctorOut)
	}

	out := runCLI(t, "map", "validation", "promote", "--store", storeRef, "--map", "map.promote", "--case", "case.smoke.patch", "--json")
	var report struct {
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
			Primary    int `json:"primary"`
			Validation int `json:"validation"`
		} `json:"counts"`
	}
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatalf("decode validation promote json: %v\n%s", err, out)
	}
	if !report.OK || report.Node.ID != "case.smoke.patch" || report.Node.Role != "primary" || report.Node.StateEffect != "advance" || report.Node.AnchorNodeID != "" || report.Node.BaseCaseID != "" {
		t.Fatalf("validation promote report = %#v", report)
	}
	if report.Counts.Primary != 1 || report.Counts.Validation != 0 {
		t.Fatalf("validation promote counts = %#v", report.Counts)
	}

	doctorClean := runCLI(t, "map", "doctor", "--store", storeRef, "--map", "map.promote", "--json")
	var doctor struct {
		OK         bool `json:"ok"`
		IssueCount int  `json:"issueCount"`
	}
	if err := json.Unmarshal([]byte(doctorClean), &doctor); err != nil {
		t.Fatalf("decode map doctor json: %v\n%s", err, doctorClean)
	}
	if !doctor.OK || doctor.IssueCount != 0 {
		t.Fatalf("promoted primary node should be doctor-clean: %#v", doctor)
	}

	explainOut := runCLI(t, "map", "explain", "--store", storeRef, "--map", "map.promote", "--case", "case.smoke.patch", "--json")
	var explain struct {
		OK            bool   `json:"ok"`
		TargetCaseID  string `json:"targetCaseId"`
		PhysicalTasks []struct {
			Kind   string `json:"kind"`
			PathID string `json:"pathId"`
			CaseID string `json:"caseId"`
		} `json:"physicalTasks"`
	}
	if err := json.Unmarshal([]byte(explainOut), &explain); err != nil {
		t.Fatalf("decode map explain json: %v\n%s", err, explainOut)
	}
	if !explain.OK || explain.TargetCaseID != "case.smoke.patch" || len(explain.PhysicalTasks) != 1 {
		t.Fatalf("promoted primary explain plan = %#v", explain)
	}
	task := explain.PhysicalTasks[0]
	if task.Kind != mapplanner.TaskRunPathPrefix || task.PathID != "path.primary.case.smoke.patch" {
		t.Fatalf("promoted primary task = %#v", task)
	}
}
