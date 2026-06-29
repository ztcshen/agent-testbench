package main

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"agent-testbench/internal/domain/mapplanner"
	"agent-testbench/internal/store"
)

func TestMapValidationPromoteCommandIsDiscoverable(t *testing.T) {
	out := runCLI(t, "commands", "--all", "--filter", commandCatalogMapValidationPromote, "--json")
	if !strings.Contains(out, `"command": "`+commandCatalogMapValidationPromote+`"`) {
		t.Fatalf("map validation promote command not discoverable:\n%s", out)
	}
}

func TestMapValidationPromoteRejectsAmbiguousTargetFlags(t *testing.T) {
	out := runCLIFails(t, "map", "validation", "promote",
		"--map", "map.promote",
		"--case", "case.smoke.patch",
		"--node", "node.smoke",
		"--json",
	)
	if !strings.Contains(out, "--case and --node cannot both be set") {
		t.Fatalf("ambiguous promote target error = %s", out)
	}
}

func TestMapValidationPromoteTurnsStandaloneSmokeIntoExecutablePrimaryNode(t *testing.T) {
	storeRef, seedUpdatedAt := seedStandalonePromoteMap(t)
	assertStandalonePromotePrecondition(t, storeRef)
	out := runCLI(t, "map", "validation", "promote", "--store", storeRef, "--map", "map.promote", "--case", "case.smoke.patch", "--json")
	assertPromoteReportConvertedNode(t, out)
	assertPromoteRefreshedMapTimestamp(t, storeRef, seedUpdatedAt)
	assertPromotedMapDoctorClean(t, storeRef)
	assertPromotedMapExplainExecutable(t, storeRef)
}

type mapValidationPromoteTestReport struct {
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

type mapValidationDoctorTestReport struct {
	OK         bool `json:"ok"`
	IssueCount int  `json:"issueCount"`
}

type mapValidationExplainTestReport struct {
	OK            bool   `json:"ok"`
	TargetCaseID  string `json:"targetCaseId"`
	PhysicalTasks []struct {
		Kind   string `json:"kind"`
		PathID string `json:"pathId"`
		CaseID string `json:"caseId"`
	} `json:"physicalTasks"`
}

func seedStandalonePromoteMap(t *testing.T) (string, time.Time) {
	t.Helper()
	ctx := context.Background()
	storePath := filepath.Join(t.TempDir(), "map-validation-promote.sqlite")
	storeRef := "sqlite://" + storePath
	runtime, err := openStore(ctx, storeRef)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer closeCLIStore(runtime)
	if err := runtime.ReplaceProfileCatalog(ctx, standalonePromoteCatalog()); err != nil {
		t.Fatalf("seed catalog: %v", err)
	}
	seedUpdatedAt := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	if err := runtime.ReplaceTestPlanGraph(ctx, standalonePromoteGraph(seedUpdatedAt)); err != nil {
		t.Fatalf("seed validation graph: %v", err)
	}
	return storeRef, seedUpdatedAt
}

func standalonePromoteCatalog() store.ProfileCatalog {
	return store.ProfileCatalog{
		ProfileID: "profile.promote",
		APICases: []store.CatalogAPICase{{
			ID: "case.smoke.patch", DisplayName: "Patch smoke", NodeID: "node.smoke", RequestTemplateID: "template.smoke",
			RenderMode: "template_patch", PatchJSON: `[{"op":"add","path":"$.trace","value":"smoke"}]`,
			Status: "active", SortOrder: 1,
		}},
	}
}

func standalonePromoteGraph(updatedAt time.Time) store.TestPlanGraph {
	return store.TestPlanGraph{
		Map: store.TestPlanMap{
			ID: "map.promote", ProfileID: "profile.promote", DisplayName: "Promote Map", Status: "draft", SummaryJSON: `{}`,
			CreatedAt: updatedAt.Add(-time.Hour), UpdatedAt: updatedAt,
		},
		Nodes: []store.TestPlanNode{{
			MapID: "map.promote", ID: "case.smoke.patch", CaseID: "case.smoke.patch", InterfaceNodeID: "node.smoke",
			RequestTemplateID: "template.smoke", Role: "validation", StateEffect: "unchanged", SummaryJSON: `{}`,
		}},
	}
}

func assertStandalonePromotePrecondition(t *testing.T, storeRef string) {
	t.Helper()
	doctorOut := runCLIFails(t, "map", "doctor", "--store", storeRef, "--map", "map.promote", "--json")
	if !strings.Contains(doctorOut, "validation.anchor") {
		t.Fatalf("precondition should have validation anchor issue:\n%s", doctorOut)
	}
}

func assertPromoteReportConvertedNode(t *testing.T, out string) {
	t.Helper()
	var report mapValidationPromoteTestReport
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatalf("decode validation promote json: %v\n%s", err, out)
	}
	if !report.OK || report.Node.ID != "case.smoke.patch" || report.Node.Role != "primary" || report.Node.StateEffect != "advance" || report.Node.AnchorNodeID != "" || report.Node.BaseCaseID != "" {
		t.Fatalf("validation promote report = %#v", report)
	}
	if report.Counts.Primary != 1 || report.Counts.Validation != 0 {
		t.Fatalf("validation promote counts = %#v", report.Counts)
	}
}

func assertPromoteRefreshedMapTimestamp(t *testing.T, storeRef string, seedUpdatedAt time.Time) {
	t.Helper()
	ctx := context.Background()
	runtime, err := openStore(ctx, storeRef)
	if err != nil {
		t.Fatalf("reopen promoted store: %v", err)
	}
	loaded, err := runtime.GetTestPlanGraph(ctx, "map.promote")
	closeCLIStore(runtime)
	if err != nil {
		t.Fatalf("load promoted graph: %v", err)
	}
	if !loaded.Map.UpdatedAt.After(seedUpdatedAt) {
		t.Fatalf("promote should refresh map updatedAt, got %s want after %s", loaded.Map.UpdatedAt.Format(time.RFC3339Nano), seedUpdatedAt.Format(time.RFC3339Nano))
	}
}

func assertPromotedMapDoctorClean(t *testing.T, storeRef string) {
	t.Helper()
	doctorClean := runCLI(t, "map", "doctor", "--store", storeRef, "--map", "map.promote", "--json")
	var doctor mapValidationDoctorTestReport
	if err := json.Unmarshal([]byte(doctorClean), &doctor); err != nil {
		t.Fatalf("decode map doctor json: %v\n%s", err, doctorClean)
	}
	if !doctor.OK || doctor.IssueCount != 0 {
		t.Fatalf("promoted primary node should be doctor-clean: %#v", doctor)
	}
}

func assertPromotedMapExplainExecutable(t *testing.T, storeRef string) {
	t.Helper()
	explainOut := runCLI(t, "map", "explain", "--store", storeRef, "--map", "map.promote", "--case", "case.smoke.patch", "--json")
	var explain mapValidationExplainTestReport
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
