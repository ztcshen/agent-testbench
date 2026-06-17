package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"agent-testbench/internal/store"
)

func TestMapRunExecutesPlanTasksAndExplainReadsResult(t *testing.T) {
	ctx := context.Background()
	storeRef := seedExecutableMapCommandStore(t, ctx)

	runCLI(t, "map", "import-workflows", "--store", storeRef, "--json")
	report := decodeMapRunCommandReport(t, runCLI(t, "map", "run", "--store", storeRef, "--map", "map.profile.flow", "--scope", "all", "--environment", "env.local", "--json"))
	assertMapRunCommandReport(t, report)
	assertStoredMapRunPlan(t, ctx, storeRef, report.PlanID)
	assertMapRunExplainCommandReport(t, runCLI(t, "map", "run", "explain", "--store", storeRef, "--plan", report.PlanID, "--json"), report.PlanID)
}

type mapRunCommandReport struct {
	OK            bool   `json:"ok"`
	PlanID        string `json:"planId"`
	MapID         string `json:"mapId"`
	Scope         string `json:"scope"`
	Status        string `json:"status"`
	EnvironmentID string `json:"environmentId"`
	Summary       struct {
		TotalTasks   int `json:"totalTasks"`
		PassedTasks  int `json:"passedTasks"`
		SkippedTasks int `json:"skippedTasks"`
		FailedTasks  int `json:"failedTasks"`
	} `json:"summary"`
	Tasks []mapRunCommandTask `json:"tasks"`
}

type mapRunCommandTask struct {
	ID            string `json:"id"`
	Kind          string `json:"kind"`
	Status        string `json:"status"`
	WorkflowRunID string `json:"workflowRunId"`
	APICaseRunID  string `json:"apiCaseRunId"`
}

type mapRunExplainCommandReport struct {
	OK      bool   `json:"ok"`
	PlanID  string `json:"planId"`
	Status  string `json:"status"`
	Summary struct {
		TotalTasks   int `json:"totalTasks"`
		PassedTasks  int `json:"passedTasks"`
		SkippedTasks int `json:"skippedTasks"`
	} `json:"summary"`
	NextActions []string `json:"nextActions"`
}

func seedExecutableMapCommandStore(t *testing.T, ctx context.Context) string {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/prepare", "/submit":
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, `{"ok":true}`)
		case "/validate":
			w.WriteHeader(http.StatusBadRequest)
			_, _ = fmt.Fprint(w, `{"error":"field required"}`)
		default:
			t.Fatalf("unexpected request path: %s", r.URL.Path)
		}
	}))
	t.Cleanup(server.Close)

	storePath := filepath.Join(t.TempDir(), "map-run.sqlite")
	storeRef := "sqlite://" + storePath
	runtime, err := openStore(ctx, storeRef)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	if err := runtime.ReplaceProfileCatalog(ctx, mapCommandExecutableProfileCatalogFixture(server.URL)); err != nil {
		t.Fatalf("seed executable profile catalog: %v", err)
	}
	closeCLIStore(runtime)
	return storeRef
}

func decodeMapRunCommandReport(t *testing.T, out string) mapRunCommandReport {
	t.Helper()
	var report mapRunCommandReport
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatalf("decode map run json: %v\n%s", err, out)
	}
	return report
}

func assertMapRunCommandReport(t *testing.T, report mapRunCommandReport) {
	t.Helper()
	if !report.OK || report.PlanID == "" || report.MapID != "map.profile.flow" || report.Scope != "all" || report.EnvironmentID != "env.local" || report.Status != "passed" {
		t.Fatalf("map run report = %#v", report)
	}
	if report.Summary.TotalTasks != 3 || report.Summary.PassedTasks != 3 || report.Summary.SkippedTasks != 0 || report.Summary.FailedTasks != 0 {
		t.Fatalf("map run summary = %#v", report.Summary)
	}
	if len(report.Tasks) != 3 {
		t.Fatalf("map run tasks = %#v", report.Tasks)
	}
	if report.Tasks[0].Kind != "run_path" || report.Tasks[0].Status != "passed" || report.Tasks[0].WorkflowRunID == "" {
		t.Fatalf("workflow task = %#v", report.Tasks[0])
	}
	if report.Tasks[1].Kind != "run_path_prefix" || report.Tasks[1].Status != "passed" || report.Tasks[1].WorkflowRunID == "" {
		t.Fatalf("replay task = %#v", report.Tasks[1])
	}
	if report.Tasks[2].Kind != "run_case" || report.Tasks[2].Status != "passed" || report.Tasks[2].APICaseRunID == "" {
		t.Fatalf("case task = %#v", report.Tasks[2])
	}
}

func assertStoredMapRunPlan(t *testing.T, ctx context.Context, storeRef string, planID string) {
	t.Helper()
	runtime, err := openStore(ctx, storeRef)
	if err != nil {
		t.Fatalf("reopen store: %v", err)
	}
	defer closeCLIStore(runtime)
	saved, err := runtime.GetTestMapPlan(ctx, planID)
	if err != nil {
		t.Fatalf("get map run plan: %v", err)
	}
	if saved.Instance.Mode != "run" || saved.Instance.Status != "passed" || saved.Instance.StartedAt.IsZero() || saved.Instance.FinishedAt.IsZero() {
		t.Fatalf("saved run instance = %#v", saved.Instance)
	}
	caseTask := saved.Tasks[2]
	if caseTask.EvidenceRoot == "" {
		t.Fatalf("case task should keep evidence root: %#v", caseTask)
	}
	caseRuns, err := runtime.ListAPICaseRuns(ctx, strings.TrimSuffix(caseTask.APICaseRunID, ".case"))
	if err != nil {
		t.Fatalf("list map case runs: %v", err)
	}
	if len(caseRuns) != 1 || caseRuns[0].TestPlanNodeID != "case.submit.field.required" || caseRuns[0].TestPlanOperation != "run_case" || !strings.Contains(caseRuns[0].PlannerSummaryJSON, planID) {
		t.Fatalf("map case run planner metadata = %#v", caseRuns)
	}
}

func assertMapRunExplainCommandReport(t *testing.T, out string, planID string) {
	t.Helper()
	var explain mapRunExplainCommandReport
	if err := json.Unmarshal([]byte(out), &explain); err != nil {
		t.Fatalf("decode map run explain json: %v\n%s", err, out)
	}
	if !explain.OK || explain.PlanID != planID || explain.Status != "passed" || explain.Summary.TotalTasks != 3 || explain.Summary.PassedTasks != 3 || explain.Summary.SkippedTasks != 0 {
		t.Fatalf("map run explain = %#v", explain)
	}
	if len(explain.NextActions) == 0 || !strings.Contains(strings.Join(explain.NextActions, "\n"), "map run explain --plan "+planID) {
		t.Fatalf("map run explain next actions = %#v", explain.NextActions)
	}
}

func mapCommandExecutableProfileCatalogFixture(baseURL string) store.ProfileCatalog {
	catalog := mapCommandProfileCatalogFixture()
	for i := range catalog.APICases {
		catalog.APICases[i].BaseURL = baseURL
		if catalog.APICases[i].ID == "case.submit.field.required" {
			catalog.APICases[i].PatchJSON = `[{"op":"remove","path":"$.field"}]`
		}
	}
	catalog.TemplateConfigs = []store.CatalogTemplateConfig{
		{
			ID:         "cfg.case.prepare",
			ScopeType:  "api-case",
			ScopeID:    "case.prepare",
			ConfigJSON: `{"caseId":"case.prepare","caseExecution":{"method":"GET","nodeId":"node.prepare","path":"/prepare","expectedHttpCodes":[200]}}`,
			Status:     "active",
		},
		{
			ID:         "cfg.case.submit.success",
			ScopeType:  "api-case",
			ScopeID:    "case.submit.success",
			ConfigJSON: `{"caseId":"case.submit.success","caseExecution":{"method":"POST","nodeId":"node.submit","path":"/submit","body":{"field":"ok"},"expectedHttpCodes":[200]}}`,
			Status:     "active",
		},
		{
			ID:         "cfg.case.submit.field.required",
			ScopeType:  "api-case",
			ScopeID:    "case.submit.field.required",
			ConfigJSON: `{"caseId":"case.submit.field.required","caseExecution":{"method":"POST","nodeId":"node.submit","path":"/validate","body":{"field":"ok"},"expectedHttpCodes":[400],"expectedResponseContains":["field required"]}}`,
			Status:     "active",
		},
	}
	return catalog
}
