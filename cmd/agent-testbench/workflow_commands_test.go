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
	"time"

	"agent-testbench/internal/store"
	"agent-testbench/internal/store/sqlite"
)

func TestWorkflowPlanCommandPrintsBoundSteps(t *testing.T) {
	dir := t.TempDir()
	writeWorkflowProfile(t, dir)
	configureNamedPostgreSQLActiveStore(t, "daily-workflow-plan-pg")
	runWorkflowPlanCommandPrintsBoundSteps(t, dir, "PostgreSQL")
}

func TestWorkflowPlanCommandPrintsBoundStepsWithMySQLStore(t *testing.T) {
	dir := t.TempDir()
	writeWorkflowProfile(t, dir)
	configureNamedMySQLActiveStore(t, "daily-workflow-plan-mysql")
	runWorkflowPlanCommandPrintsBoundSteps(t, dir, "MySQL")
}

func runWorkflowPlanCommandPrintsBoundSteps(t *testing.T, dir string, label string) {
	t.Helper()
	runCLI(t, "config", "publish", "--from", dir)

	out := runCLI(t, "workflow", "plan", "--workflow", "workflow.alpha")

	for _, want := range []string{
		"Workflow: workflow.alpha",
		"Step: step.one",
		"Node: node.alpha",
		"Case: case.alpha",
		"Required: true",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("%s workflow plan output missing %q: %q", label, want, out)
		}
	}
}

func TestWorkflowPlanCommandCanEmitJSONFromStore(t *testing.T) {
	profileDir := t.TempDir()
	writeWorkflowProfile(t, profileDir)
	configureNamedPostgreSQLActiveStore(t, "daily-workflow-plan-json-pg")
	runWorkflowPlanCommandCanEmitJSONFromStore(t, profileDir, "PostgreSQL")
}

func TestWorkflowPlanCommandCanEmitJSONFromMySQLStore(t *testing.T) {
	profileDir := t.TempDir()
	writeWorkflowProfile(t, profileDir)
	configureNamedMySQLActiveStore(t, "daily-workflow-plan-json-mysql")
	runWorkflowPlanCommandCanEmitJSONFromStore(t, profileDir, "MySQL")
}

func runWorkflowPlanCommandCanEmitJSONFromStore(t *testing.T, profileDir string, label string) {
	t.Helper()
	runCLI(t, "config", "publish", "--from", profileDir)

	out := runCLI(t, "workflow", "plan", "--workflow", "workflow.alpha", "--json")

	var payload struct {
		OK         bool   `json:"ok"`
		ProfileID  string `json:"profileId"`
		WorkflowID string `json:"workflowId"`
		Counts     struct {
			Steps         int `json:"steps"`
			RequiredSteps int `json:"requiredSteps"`
		} `json:"counts"`
		Steps []struct {
			StepID string `json:"stepId"`
			NodeID string `json:"nodeId"`
			CaseID string `json:"caseId"`
		} `json:"steps"`
	}
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		t.Fatalf("decode %s workflow plan json: %v\n%s", label, err, out)
	}
	if !payload.OK || payload.ProfileID != "sample" || payload.WorkflowID != "workflow.alpha" || payload.Counts.Steps != 1 || payload.Counts.RequiredSteps != 1 {
		t.Fatalf("%s workflow plan json summary = %#v", label, payload)
	}
	if len(payload.Steps) != 1 || payload.Steps[0].StepID != "step.one" || payload.Steps[0].NodeID != "node.alpha" || payload.Steps[0].CaseID != "case.alpha" {
		t.Fatalf("%s workflow plan json steps = %#v", label, payload.Steps)
	}
}

func TestWorkflowPlanCommandRejectsMissingWorkflow(t *testing.T) {
	dir := t.TempDir()
	writeWorkflowProfile(t, dir)
	configureNamedPostgreSQLActiveStore(t, "daily-workflow-plan-missing-pg")
	runWorkflowPlanCommandRejectsMissingWorkflow(t, dir, "PostgreSQL")
}

func TestWorkflowPlanCommandRejectsMissingWorkflowWithMySQLStore(t *testing.T) {
	dir := t.TempDir()
	writeWorkflowProfile(t, dir)
	configureNamedMySQLActiveStore(t, "daily-workflow-plan-missing-mysql")
	runWorkflowPlanCommandRejectsMissingWorkflow(t, dir, "MySQL")
}

func runWorkflowPlanCommandRejectsMissingWorkflow(t *testing.T, dir string, label string) {
	t.Helper()
	runCLI(t, "config", "publish", "--from", dir)

	out := runCLIFails(t, "workflow", "plan", "--workflow", "workflow.missing")
	if !strings.Contains(out, "workflow not found") || !strings.Contains(out, "workflow.missing") {
		t.Fatalf("%s missing workflow output = %q", label, out)
	}
}

func TestWorkflowRunCommandsReadStoredRuns(t *testing.T) {
	storeRef := configureNamedPostgreSQLActiveStore(t, "daily-workflow-runs-pg")
	runWorkflowRunCommandsReadStoredRuns(t, storeRef, "PostgreSQL")
}

func TestWorkflowRunCommandsUseNamedMySQLActiveStore(t *testing.T) {
	storeRef := configureNamedMySQLActiveStore(t, "daily-workflow-runs-mysql")
	runWorkflowRunCommandsReadStoredRuns(t, storeRef, "MySQL")
}

func runWorkflowRunCommandsReadStoredRuns(t *testing.T, storeRef string, label string) {
	t.Helper()
	ctx := context.Background()
	s, err := openStore(ctx, storeRef)
	if err != nil {
		t.Fatalf("open %s store: %v", label, err)
	}
	defer s.Close()
	runID := uniqueTestID(t, "run.workflow")
	workflowID := uniqueTestID(t, "workflow.alpha")
	caseID := uniqueTestID(t, "case.alpha")
	started := time.Date(2026, 5, 18, 10, 0, 0, 0, time.UTC)
	if _, err := s.CreateRun(ctx, store.Run{
		ID:         runID,
		ProfileID:  uniqueTestID(t, "profile.workflow-runs"),
		WorkflowID: workflowID,
		Status:     store.StatusPassed,
		SummaryJSON: fmt.Sprintf(`{
			"summary":{"stepCount":1,"passed":1},
			"steps":[{"stepId":"step.one","caseId":%q,"status":"passed"}]
		}`, caseID),
		StartedAt:  started,
		FinishedAt: started.Add(time.Second),
		CreatedAt:  started,
		UpdatedAt:  started.Add(time.Second),
	}); err != nil {
		t.Fatalf("create %s run: %v", label, err)
	}

	listOut := runCLI(t, "workflow", "runs", "--json")
	var list struct {
		OK           bool `json:"ok"`
		WorkflowRuns []struct {
			ID         string `json:"id"`
			WorkflowID string `json:"workflowId"`
			Status     string `json:"status"`
			StepCount  int    `json:"stepCount"`
		} `json:"workflowRuns"`
	}
	if err := json.Unmarshal([]byte(listOut), &list); err != nil {
		t.Fatalf("decode %s workflow runs json: %v\n%s", label, err, listOut)
	}
	foundRun := false
	for _, item := range list.WorkflowRuns {
		if item.ID == runID && item.WorkflowID == workflowID && item.StepCount == 1 {
			foundRun = true
			break
		}
	}
	if !list.OK || !foundRun {
		t.Fatalf("%s workflow runs = %#v", label, list)
	}

	detailOut := runCLI(t, "workflow", "run", "--run", runID, "--json")
	var detail struct {
		OK      bool           `json:"ok"`
		Run     map[string]any `json:"run"`
		Summary struct {
			Steps []map[string]any `json:"steps"`
		} `json:"summary"`
	}
	if err := json.Unmarshal([]byte(detailOut), &detail); err != nil {
		t.Fatalf("decode %s workflow run json: %v\n%s", label, err, detailOut)
	}
	if !detail.OK || detail.Run["id"] != runID || len(detail.Summary.Steps) != 1 || detail.Summary.Steps[0]["stepId"] != "step.one" {
		t.Fatalf("%s workflow run detail = %#v", label, detail)
	}

	stepOut := runCLI(t, "workflow", "step", "--run", runID, "--step", "step.one", "--json")
	var stepDetail struct {
		OK      bool           `json:"ok"`
		Run     map[string]any `json:"run"`
		Summary struct {
			Steps []map[string]any `json:"steps"`
		} `json:"summary"`
	}
	if err := json.Unmarshal([]byte(stepOut), &stepDetail); err != nil {
		t.Fatalf("decode %s workflow step json: %v\n%s", label, err, stepOut)
	}
	if !stepDetail.OK || stepDetail.Run["id"] != runID || len(stepDetail.Summary.Steps) != 1 || stepDetail.Summary.Steps[0]["stepId"] != "step.one" {
		t.Fatalf("%s workflow step detail = %#v", label, stepDetail)
	}

	latestOut := runCLI(t, "workflow", "latest-step", "--workflow", workflowID, "--step", "step.one", "--json")
	var latestDetail struct {
		OK      bool           `json:"ok"`
		Run     map[string]any `json:"run"`
		Summary struct {
			Steps []map[string]any `json:"steps"`
		} `json:"summary"`
	}
	if err := json.Unmarshal([]byte(latestOut), &latestDetail); err != nil {
		t.Fatalf("decode latest %s workflow step json: %v\n%s", label, err, latestOut)
	}
	if !latestDetail.OK || latestDetail.Run["id"] != runID || len(latestDetail.Summary.Steps) != 1 || latestDetail.Summary.Steps[0]["caseId"] != caseID {
		t.Fatalf("latest %s workflow step detail = %#v", label, latestDetail)
	}

	if out := runCLI(t, "workflow", "runs", "--store", storeRef, "--json"); !strings.Contains(out, runID) {
		t.Fatalf("%s workflow runs --store output = %q", label, out)
	}
	if out := runCLI(t, "workflow", "run", "--store", storeRef, "--run", runID, "--json"); !strings.Contains(out, "step.one") {
		t.Fatalf("%s workflow run --store output = %q", label, out)
	}
	if out := runCLI(t, "workflow", "step", "--store", storeRef, "--run", runID, "--step", "step.one", "--json"); !strings.Contains(out, caseID) {
		t.Fatalf("%s workflow step --store output = %q", label, out)
	}
	if out := runCLI(t, "workflow", "latest-step", "--store", storeRef, "--workflow", workflowID, "--step", "step.one", "--json"); !strings.Contains(out, runID) {
		t.Fatalf("%s workflow latest-step --store output = %q", label, out)
	}
}

func TestWorkflowRunCommandsUseNamedSQLiteActiveStore(t *testing.T) {
	storeRef := configureNamedSQLiteActiveStore(t, "daily-workflow-runs-sqlite")
	runWorkflowRunCommandsReadStoredRuns(t, storeRef, "SQLite")
}

func TestWorkflowGateFailsWithFailedStepAndActionableReport(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	storePath := filepath.Join(dir, "store.sqlite")
	s, err := sqlite.Open(ctx, sqlite.Config{Path: storePath})
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	started := time.Date(2026, 5, 24, 10, 0, 0, 0, time.UTC)
	runID := "run.workflow-gate"
	workflowID := "workflow.checkout"
	if _, err := s.CreateRun(ctx, store.Run{
		ID:           runID,
		ProfileID:    "sample",
		WorkflowID:   workflowID,
		Status:       store.StatusFailed,
		EvidenceRoot: filepath.Join(dir, "evidence", runID),
		SummaryJSON: `{
			"summary":{"stepCount":2,"passed":1,"failed":1},
			"steps":[
				{"stepId":"step.prepare","caseId":"case.prepare","caseRunId":"run.workflow-gate.prepare","status":"passed"},
				{"stepId":"step.submit","caseId":"case.submit","caseRunId":"run.workflow-gate.submit","status":"failed"}
			]
		}`,
		StartedAt:  started,
		FinishedAt: started.Add(2 * time.Second),
		CreatedAt:  started,
		UpdatedAt:  started.Add(2 * time.Second),
	}); err != nil {
		t.Fatalf("create run: %v", err)
	}
	for _, item := range []struct {
		id     string
		caseID string
		status string
	}{
		{id: "run.workflow-gate.prepare", caseID: "case.prepare", status: store.StatusPassed},
		{id: "run.workflow-gate.submit", caseID: "case.submit", status: store.StatusFailed},
	} {
		if _, err := s.RecordAPICaseRun(ctx, store.APICaseRun{
			ID:                   item.id,
			RunID:                runID,
			CaseID:               item.caseID,
			Status:               item.status,
			RequestSummaryJSON:   `{"method":"POST","path":"/workflow"}`,
			AssertionSummaryJSON: `{"status":"` + item.status + `"}`,
			StartedAt:            started,
			FinishedAt:           started.Add(time.Second),
			CreatedAt:            started,
		}); err != nil {
			t.Fatalf("record case run %s: %v", item.id, err)
		}
		if _, err := s.RecordEvidence(ctx, store.EvidenceRecord{
			ID:        item.id + ".summary",
			RunID:     runID,
			CaseRunID: item.id,
			Kind:      "summary",
			URI:       filepath.Join(dir, "evidence", runID, item.id, "summary.json"),
			MediaType: "application/json",
			Summary:   `{"kind":"summary"}`,
			CreatedAt: started,
		}); err != nil {
			t.Fatalf("record evidence %s: %v", item.id, err)
		}
	}
	if err := s.Close(); err != nil {
		t.Fatalf("close store: %v", err)
	}

	out := runCLIFails(t, "workflow", "gate", "--store", "sqlite://"+storePath, "--run", runID, "--require-passed", "--require-steps", "--require-evidence", "--json")
	var report struct {
		OK         bool   `json:"ok"`
		RunID      string `json:"runId"`
		WorkflowID string `json:"workflowId"`
		Status     string `json:"status"`
		Counts     struct {
			Steps            int `json:"steps"`
			PassedSteps      int `json:"passedSteps"`
			FailedSteps      int `json:"failedSteps"`
			CaseRuns         int `json:"caseRuns"`
			EvidenceComplete int `json:"evidenceComplete"`
		} `json:"counts"`
		Gates struct {
			RunPassed        bool `json:"runPassed"`
			StepsPassed      bool `json:"stepsPassed"`
			EvidenceComplete bool `json:"evidenceComplete"`
		} `json:"gates"`
		FailedSteps []struct {
			StepID    string `json:"stepId"`
			CaseID    string `json:"caseId"`
			CaseRunID string `json:"caseRunId"`
			Status    string `json:"status"`
		} `json:"failedSteps"`
		NextActions []string `json:"nextActions"`
	}
	if err := json.Unmarshal([]byte(extractJSONObject(t, out)), &report); err != nil {
		t.Fatalf("decode workflow gate json: %v\n%s", err, out)
	}
	if report.OK || report.RunID != runID || report.WorkflowID != workflowID || report.Status != store.StatusFailed {
		t.Fatalf("workflow gate identity = %#v", report)
	}
	if report.Counts.Steps != 2 || report.Counts.PassedSteps != 1 || report.Counts.FailedSteps != 1 || report.Counts.CaseRuns != 2 || report.Counts.EvidenceComplete != 2 {
		t.Fatalf("workflow gate counts = %#v", report.Counts)
	}
	if report.Gates.RunPassed || report.Gates.StepsPassed || !report.Gates.EvidenceComplete {
		t.Fatalf("workflow gate booleans = %#v", report.Gates)
	}
	if len(report.FailedSteps) != 1 || report.FailedSteps[0].StepID != "step.submit" || report.FailedSteps[0].CaseRunID != "run.workflow-gate.submit" {
		t.Fatalf("workflow gate failed steps = %#v", report.FailedSteps)
	}
	next := strings.Join(report.NextActions, "\n")
	if !strings.Contains(next, "agent-testbench workflow step --run "+quoteCommandValue(runID)+" --step 'step.submit'") || !strings.Contains(next, "agent-testbench case diagnose --case-run 'run.workflow-gate.submit'") {
		t.Fatalf("workflow gate next actions = %#v", report.NextActions)
	}
}

func TestWorkflowGateCorrelatesCaseRunByStepIDWhenCaseIDRepeats(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	storePath := filepath.Join(dir, "store.sqlite")
	s, err := sqlite.Open(ctx, sqlite.Config{Path: storePath})
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	started := time.Date(2026, 5, 24, 10, 0, 0, 0, time.UTC)
	runID := "run.workflow-gate-duplicate-case"
	if _, err := s.CreateRun(ctx, store.Run{
		ID:           runID,
		ProfileID:    "sample",
		WorkflowID:   "workflow.duplicate-case",
		Status:       store.StatusFailed,
		EvidenceRoot: filepath.Join(dir, "evidence", runID),
		SummaryJSON: `{
			"steps":[
				{"stepId":"step.prepare","caseId":"case.shared","status":"passed"},
				{"stepId":"step.submit","caseId":"case.shared","status":"failed"}
			]
		}`,
		StartedAt:  started,
		FinishedAt: started.Add(2 * time.Second),
		CreatedAt:  started,
		UpdatedAt:  started.Add(2 * time.Second),
	}); err != nil {
		t.Fatalf("create run: %v", err)
	}
	for _, item := range []struct {
		id     string
		stepID string
		status string
	}{
		{id: "z.prepare.case", stepID: "step.prepare", status: store.StatusPassed},
		{id: "a.submit.case", stepID: "step.submit", status: store.StatusFailed},
	} {
		if _, err := s.RecordAPICaseRun(ctx, store.APICaseRun{
			ID:                   item.id,
			RunID:                runID,
			CaseID:               "case.shared",
			Status:               item.status,
			RequestSummaryJSON:   `{"method":"POST","path":"/workflow","stepId":"` + item.stepID + `"}`,
			AssertionSummaryJSON: `{"status":"` + item.status + `"}`,
			StartedAt:            started,
			FinishedAt:           started.Add(time.Second),
			CreatedAt:            started,
		}); err != nil {
			t.Fatalf("record case run %s: %v", item.id, err)
		}
	}
	if err := s.Close(); err != nil {
		t.Fatalf("close store: %v", err)
	}

	out := runCLIFails(t, "workflow", "gate", "--store", "sqlite://"+storePath, "--run", runID, "--require-passed", "--require-steps", "--json")
	var report struct {
		FailedSteps []struct {
			StepID    string `json:"stepId"`
			CaseRunID string `json:"caseRunId"`
		} `json:"failedSteps"`
		NextActions []string `json:"nextActions"`
	}
	if err := json.Unmarshal([]byte(extractJSONObject(t, out)), &report); err != nil {
		t.Fatalf("decode workflow gate json: %v\n%s", err, out)
	}
	if len(report.FailedSteps) != 1 || report.FailedSteps[0].StepID != "step.submit" || report.FailedSteps[0].CaseRunID != "a.submit.case" {
		t.Fatalf("workflow gate should select case run by step id, got %#v", report.FailedSteps)
	}
	next := strings.Join(report.NextActions, "\n")
	if !strings.Contains(next, "agent-testbench case diagnose --case-run 'a.submit.case' --json") {
		t.Fatalf("workflow gate next actions should use step-correlated case run: %#v", report.NextActions)
	}
}

func TestWorkflowGateDoesNotPickArbitraryCaseRunWhenCaseIDIsAmbiguous(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	storePath := filepath.Join(dir, "store.sqlite")
	s, err := sqlite.Open(ctx, sqlite.Config{Path: storePath})
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	started := time.Date(2026, 5, 24, 10, 0, 0, 0, time.UTC)
	runID := "run.workflow-gate-ambiguous-case"
	if _, err := s.CreateRun(ctx, store.Run{
		ID:         runID,
		ProfileID:  "sample",
		WorkflowID: "workflow.ambiguous-case",
		Status:     store.StatusFailed,
		SummaryJSON: `{
			"steps":[{"stepId":"step.submit","caseId":"case.shared","status":"failed"}]
		}`,
		StartedAt:  started,
		FinishedAt: started.Add(2 * time.Second),
		CreatedAt:  started,
		UpdatedAt:  started.Add(2 * time.Second),
	}); err != nil {
		t.Fatalf("create run: %v", err)
	}
	for _, id := range []string{"a.first.case", "z.second.case"} {
		if _, err := s.RecordAPICaseRun(ctx, store.APICaseRun{
			ID:                   id,
			RunID:                runID,
			CaseID:               "case.shared",
			Status:               store.StatusPassed,
			RequestSummaryJSON:   `{"method":"POST","path":"/workflow"}`,
			AssertionSummaryJSON: `{"status":"passed"}`,
			StartedAt:            started,
			FinishedAt:           started.Add(time.Second),
			CreatedAt:            started,
		}); err != nil {
			t.Fatalf("record case run %s: %v", id, err)
		}
	}
	if err := s.Close(); err != nil {
		t.Fatalf("close store: %v", err)
	}

	out := runCLIFails(t, "workflow", "gate", "--store", "sqlite://"+storePath, "--run", runID, "--require-passed", "--require-steps", "--json")
	var report struct {
		FailedSteps []struct {
			CaseRunID string `json:"caseRunId"`
			Status    string `json:"status"`
		} `json:"failedSteps"`
		NextActions []string `json:"nextActions"`
	}
	if err := json.Unmarshal([]byte(extractJSONObject(t, out)), &report); err != nil {
		t.Fatalf("decode workflow gate json: %v\n%s", err, out)
	}
	if len(report.FailedSteps) != 1 || report.FailedSteps[0].CaseRunID != "" || report.FailedSteps[0].Status != store.StatusFailed {
		t.Fatalf("workflow gate should leave ambiguous case run unresolved, got %#v", report.FailedSteps)
	}
	if strings.Contains(strings.Join(report.NextActions, "\n"), "--case-run") {
		t.Fatalf("workflow gate should not emit case-run actions for ambiguous case selection: %#v", report.NextActions)
	}
}

func TestWorkflowGateNextActionsQuoteDynamicIDs(t *testing.T) {
	report := workflowGateReport{
		RunID: "run $(touch nope)",
		Gates: workflowGateGates{
			StepsPresent: true,
		},
		FailedSteps: []workflowGateStep{
			{StepID: "step 'submit'", CaseRunID: "case-run $HOME"},
		},
		MissingEvidence: []workflowGateStep{
			{CaseRunID: "case-run `uname`"},
		},
	}

	actions := workflowGateNextActions(report, workflowGateOptions{RequireEvidence: true})
	joined := strings.Join(actions, "\n")
	for _, want := range []string{
		"--run 'run $(touch nope)' --step 'step '\\''submit'\\''' --json",
		"--case-run 'case-run $HOME' --json",
		"--case-run 'case-run `uname`' --json",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("workflow gate action missing %q:\n%s", want, joined)
		}
	}
}

func TestTraceTopologyCollectCommandPersistsTopology(t *testing.T) {
	ctx := context.Background()
	storePath := filepath.Join(t.TempDir(), "store.sqlite")
	s, err := sqlite.Open(ctx, sqlite.Config{Path: storePath})
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	startedAt := time.Date(2026, 5, 18, 10, 0, 0, 0, time.UTC)
	if _, err := s.CreateRun(ctx, store.Run{
		ID:         "run.trace",
		ProfileID:  "sample",
		WorkflowID: "workflow.alpha",
		Status:     store.StatusPassed,
		StartedAt:  startedAt,
		FinishedAt: startedAt.Add(3 * time.Second),
		CreatedAt:  startedAt,
		UpdatedAt:  startedAt.Add(3 * time.Second),
	}); err != nil {
		t.Fatalf("create run: %v", err)
	}
	if err := s.Close(); err != nil {
		t.Fatalf("close store: %v", err)
	}
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload struct {
			Query string `json:"query"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode provider request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.Contains(payload.Query, "queryBasicTraces"):
			_, _ = w.Write([]byte(`{"data":{"queryBasicTraces":{"traces":[{"endpointNames":["POST:/alpha"],"duration":120,"start":"2026-05-18 1000","isError":false,"traceIds":["trace.alpha"]}]}}}`))
		case strings.Contains(payload.Query, "queryTrace"):
			_, _ = w.Write([]byte(`{"data":{"queryTrace":{"spans":[{"traceId":"trace.alpha","segmentId":"segment.entry","spanId":0,"parentSpanId":-1,"refs":[],"serviceCode":"service.entry","endpointName":"/alpha","type":"Entry","component":"Tomcat"},{"traceId":"trace.alpha","segmentId":"segment.worker","spanId":0,"parentSpanId":-1,"refs":[{"traceId":"trace.alpha","parentSegmentId":"segment.entry","parentSpanId":0,"type":"CrossProcess"}],"serviceCode":"service.worker","endpointName":"POST:/alpha","type":"Entry","component":"Server"}]}}}`))
		default:
			t.Fatalf("unexpected provider query: %s", payload.Query)
		}
	}))
	defer provider.Close()

	out := runCLI(t, "trace", "topology", "collect",
		"--store", "sqlite://"+storePath,
		"--trace-graphql-url", provider.URL,
		"--run", "run.trace",
		"--step", "step.alpha",
		"--case", "case.alpha",
		"--request", "request.alpha",
		"--endpoint", "/alpha",
		"--started-at", startedAt.Format(time.RFC3339Nano),
		"--json",
	)

	var payload struct {
		OK            bool `json:"ok"`
		TraceTopology struct {
			WorkflowRunID string `json:"workflowRunId"`
			TraceID       string `json:"traceId"`
			Status        string `json:"status"`
		} `json:"traceTopology"`
		Topology struct {
			SpanCount      int `json:"spanCount"`
			ConfirmedEdges []struct {
				Source string `json:"source"`
				Target string `json:"target"`
			} `json:"confirmedEdges"`
		} `json:"topology"`
	}
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		t.Fatalf("decode trace topology collect json: %v\n%s", err, out)
	}
	if !payload.OK || payload.TraceTopology.WorkflowRunID != "run.trace" || payload.TraceTopology.TraceID != "trace.alpha" || payload.TraceTopology.Status != "complete" {
		t.Fatalf("trace topology collect payload = %#v", payload)
	}
	if payload.Topology.SpanCount != 2 || len(payload.Topology.ConfirmedEdges) != 1 || payload.Topology.ConfirmedEdges[0].Source != "service.entry" || payload.Topology.ConfirmedEdges[0].Target != "service.worker" {
		t.Fatalf("trace topology = %#v", payload.Topology)
	}
	s, err = sqlite.Open(ctx, sqlite.Config{Path: storePath})
	if err != nil {
		t.Fatalf("reopen store: %v", err)
	}
	defer s.Close()
	rows, err := s.ListTraceTopologies(ctx, "run.trace")
	if err != nil {
		t.Fatalf("list trace topologies: %v", err)
	}
	if len(rows) != 1 || rows[0].StepID != "step.alpha" || rows[0].CaseID != "case.alpha" || rows[0].RequestID != "request.alpha" {
		t.Fatalf("stored topologies = %#v", rows)
	}
}

func TestReplayEvidenceCommandEmitsShellPayload(t *testing.T) {
	out := runCLI(t, "replay", "evidence", "--trace-id", "TRACE-1", "--json")

	var payload struct {
		OK  bool `json:"ok"`
		Run struct {
			TraceID string `json:"traceId"`
		} `json:"run"`
		Evidence struct {
			TraceID string `json:"traceId"`
			Systems []any  `json:"systems"`
		} `json:"evidence"`
	}
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		t.Fatalf("decode replay evidence json: %v\n%s", err, out)
	}
	if !payload.OK || payload.Run.TraceID != "TRACE-1" || payload.Evidence.TraceID != "TRACE-1" || len(payload.Evidence.Systems) != 0 {
		t.Fatalf("replay evidence payload = %#v", payload)
	}
}

func TestWorkflowAuditCommandEmitsJSONWithScopedStoreState(t *testing.T) {
	storeRef := configureNamedPostgreSQLActiveStore(t, "daily-workflow-audit-json-pg")
	runWorkflowAuditCommandEmitsJSONWithScopedStoreState(t, storeRef, "PostgreSQL")
}

func TestWorkflowAuditCommandUsesNamedMySQLActiveStore(t *testing.T) {
	storeRef := configureNamedMySQLActiveStore(t, "daily-workflow-audit-json-mysql")
	runWorkflowAuditCommandEmitsJSONWithScopedStoreState(t, storeRef, "MySQL")
}

func runWorkflowAuditCommandEmitsJSONWithScopedStoreState(t *testing.T, storeRef string, label string) {
	t.Helper()
	ctx := context.Background()
	fixture := writeWorkflowAuditProfile(t)
	runCLI(t, "config", "publish", "--from", fixture.profileDir)
	s, err := openStore(ctx, storeRef)
	if err != nil {
		t.Fatalf("open %s store: %v", label, err)
	}
	firstRunID := uniqueTestID(t, "run.workflow.001")
	secondRunID := uniqueTestID(t, "run.workflow.002")
	started := time.Now().UTC().Add(-10 * time.Second)
	finished := started.Add(2 * time.Second)
	if _, err := s.CreateRun(ctx, store.Run{
		ID:         firstRunID,
		ProfileID:  fixture.profileID,
		WorkflowID: fixture.workflowID,
		Status:     store.StatusFailed,
		StartedAt:  started,
		FinishedAt: finished,
		CreatedAt:  started,
		UpdatedAt:  finished,
	}); err != nil {
		t.Fatalf("create first %s workflow run: %v", label, err)
	}
	if _, err := s.RecordAPICaseRun(ctx, store.APICaseRun{
		ID:         firstRunID + ".case.alpha",
		RunID:      firstRunID,
		CaseID:     fixture.alphaCaseID,
		Status:     store.StatusFailed,
		StartedAt:  started,
		FinishedAt: finished,
		CreatedAt:  started,
	}); err != nil {
		t.Fatalf("record first %s case run: %v", label, err)
	}
	laterStarted := started.Add(10 * time.Second)
	laterFinished := laterStarted.Add(3 * time.Second)
	if _, err := s.CreateRun(ctx, store.Run{
		ID:         secondRunID,
		ProfileID:  fixture.profileID,
		WorkflowID: fixture.workflowID,
		Status:     store.StatusPassed,
		StartedAt:  laterStarted,
		FinishedAt: laterFinished,
		CreatedAt:  laterStarted,
		UpdatedAt:  laterFinished,
	}); err != nil {
		t.Fatalf("create second %s workflow run: %v", label, err)
	}
	if _, err := s.RecordAPICaseRun(ctx, store.APICaseRun{
		ID:         secondRunID + ".case.alpha",
		RunID:      secondRunID,
		CaseID:     fixture.alphaCaseID,
		Status:     store.StatusPassed,
		StartedAt:  laterStarted,
		FinishedAt: laterFinished,
		CreatedAt:  laterStarted,
	}); err != nil {
		t.Fatalf("record second %s case run: %v", label, err)
	}
	if err := s.Close(); err != nil {
		t.Fatalf("close %s store: %v", label, err)
	}

	out := runCLI(t, "workflow", "audit", "--workflow", fixture.workflowID, "--json")

	var report struct {
		OK         bool   `json:"ok"`
		WorkflowID string `json:"workflowId"`
		IssueCount int    `json:"issueCount"`
		Issues     []struct {
			Code      string `json:"code"`
			SubjectID string `json:"subjectId"`
		} `json:"issues"`
		Store *struct {
			LatestRun *struct {
				ID     string `json:"id"`
				Status string `json:"status"`
			} `json:"latestRun"`
			BindingCases []struct {
				StepID       string `json:"stepId"`
				CaseID       string `json:"caseId"`
				HasPassed    bool   `json:"hasPassed"`
				LatestStatus string `json:"latestStatus"`
				LatestRunID  string `json:"latestRunId"`
			} `json:"bindingCases"`
		} `json:"store"`
	}
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatalf("decode %s workflow audit json: %v\n%s", label, err, out)
	}
	if report.OK || report.WorkflowID != fixture.workflowID || report.IssueCount != 2 {
		t.Fatalf("%s workflow audit summary = %#v", label, report)
	}
	if len(report.Issues) != 2 || report.Issues[0].Code != "api-case-node-missing" || report.Issues[1].Code != "case-dependency-fixture-missing" {
		t.Fatalf("%s workflow audit issues = %#v", label, report.Issues)
	}
	if report.Store == nil || report.Store.LatestRun == nil || report.Store.LatestRun.ID != secondRunID || report.Store.LatestRun.Status != store.StatusPassed {
		t.Fatalf("%s latest workflow run = %#v", label, report.Store)
	}
	caseState := map[string]struct {
		HasPassed    bool
		LatestStatus string
		LatestRunID  string
	}{}
	for _, item := range report.Store.BindingCases {
		caseState[item.CaseID] = struct {
			HasPassed    bool
			LatestStatus string
			LatestRunID  string
		}{HasPassed: item.HasPassed, LatestStatus: item.LatestStatus, LatestRunID: item.LatestRunID}
	}
	if !caseState[fixture.alphaCaseID].HasPassed || caseState[fixture.alphaCaseID].LatestStatus != store.StatusPassed || caseState[fixture.alphaCaseID].LatestRunID != secondRunID {
		t.Fatalf("%s alpha workflow state = %#v", label, caseState[fixture.alphaCaseID])
	}
	if caseState[fixture.betaCaseID].HasPassed || caseState[fixture.betaCaseID].LatestStatus != "" || caseState[fixture.betaCaseID].LatestRunID != "" {
		t.Fatalf("%s beta workflow state = %#v", label, caseState[fixture.betaCaseID])
	}
}

type workflowAuditFixture struct {
	profileDir       string
	profileID        string
	workflowID       string
	nodeID           string
	missingNodeID    string
	alphaCaseID      string
	betaCaseID       string
	templateID       string
	dependencyID     string
	missingFixtureID string
}

func writeWorkflowAuditProfile(t *testing.T) workflowAuditFixture {
	t.Helper()
	fixture := workflowAuditFixture{
		profileDir:       filepath.Join(t.TempDir(), "profile"),
		profileID:        uniqueTestID(t, "profile.workflow-audit"),
		workflowID:       uniqueTestID(t, "workflow.audit"),
		nodeID:           uniqueTestID(t, "node.audit"),
		missingNodeID:    uniqueTestID(t, "node.missing"),
		alphaCaseID:      uniqueTestID(t, "case.audit.alpha"),
		betaCaseID:       uniqueTestID(t, "case.audit.beta"),
		templateID:       uniqueTestID(t, "template.audit"),
		dependencyID:     uniqueTestID(t, "dependency.audit"),
		missingFixtureID: uniqueTestID(t, "fixture.missing"),
	}
	writeFile(t, filepath.Join(fixture.profileDir, "profile.json"), fmt.Sprintf(`{
  "id": %q,
  "displayName": "Sample Profile",
  "services": [],
  "workflows": [{"id":%q,"displayName":"Workflow Alpha"}],
  "interfaceNodes": [{"id":%q,"displayName":"Node Alpha"}],
  "apiCases": [
    {"id":%q,"displayName":"Case Alpha","nodeId":%q},
    {"id":%q,"displayName":"Case Beta","nodeId":%q}
  ],
  "requestTemplates": [{"id":%q,"nodeId":%q,"method":"POST","path":"/v1/items"}],
  "caseDependencies": [{"id":%q,"caseId":%q,"fixtureId":%q}],
	"workflowBindings": [
    {"workflowId":%q,"stepId":"step.one","nodeId":%q,"caseId":%q,"required":true},
    {"workflowId":%q,"stepId":"step.two","nodeId":%q,"caseId":%q,"required":true}
  ],
  "fixtures": []
}`, fixture.profileID, fixture.workflowID, fixture.nodeID, fixture.alphaCaseID, fixture.nodeID, fixture.betaCaseID, fixture.missingNodeID, fixture.templateID, fixture.nodeID, fixture.dependencyID, fixture.betaCaseID, fixture.missingFixtureID, fixture.workflowID, fixture.nodeID, fixture.alphaCaseID, fixture.workflowID, fixture.nodeID, fixture.betaCaseID))
	return fixture
}

func TestWorkflowAuditCommandPrintsTextSummary(t *testing.T) {
	configureNamedPostgreSQLActiveStore(t, "daily-workflow-audit-text-pg")
	runWorkflowAuditCommandPrintsTextSummary(t, "PostgreSQL")
}

func TestWorkflowAuditCommandPrintsTextSummaryWithMySQLStore(t *testing.T) {
	configureNamedMySQLActiveStore(t, "daily-workflow-audit-text-mysql")
	runWorkflowAuditCommandPrintsTextSummary(t, "MySQL")
}

func runWorkflowAuditCommandPrintsTextSummary(t *testing.T, label string) {
	t.Helper()
	fixture := writeWorkflowAuditTextProfile(t)
	runCLI(t, "config", "publish", "--from", fixture.profileDir)

	out := runCLI(t, "workflow", "audit", "--workflow", fixture.workflowID)

	for _, want := range []string{
		"Workflow Audit: " + fixture.workflowID,
		"OK: true",
		"Issues: 0",
		"Bindings: 1",
		"Binding: step.one Node: " + fixture.nodeID + " Case: " + fixture.alphaCaseID + " Required: true",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("%s workflow audit output missing %q: %q", label, want, out)
		}
	}
}

func writeWorkflowAuditTextProfile(t *testing.T) workflowAuditFixture {
	t.Helper()
	fixture := workflowAuditFixture{
		profileDir:  filepath.Join(t.TempDir(), "profile"),
		profileID:   uniqueTestID(t, "profile.workflow-audit-text"),
		workflowID:  uniqueTestID(t, "workflow.audit-text"),
		nodeID:      uniqueTestID(t, "node.audit-text"),
		alphaCaseID: uniqueTestID(t, "case.audit-text"),
	}
	writeFile(t, filepath.Join(fixture.profileDir, "profile.json"), fmt.Sprintf(`{
  "id": %q,
  "displayName": "Sample Profile",
  "services": [],
  "workflows": [{"id":%q,"displayName":"Workflow Alpha"}],
  "interfaceNodes": [{"id":%q,"displayName":"Node Alpha"}],
  "apiCases": [{"id":%q,"displayName":"Case Alpha","nodeId":%q}],
  "requestTemplates": [],
  "caseDependencies": [],
  "workflowBindings": [{"workflowId":%q,"stepId":"step.one","nodeId":%q,"caseId":%q,"required":true}],
  "fixtures": []
}`, fixture.profileID, fixture.workflowID, fixture.nodeID, fixture.alphaCaseID, fixture.nodeID, fixture.workflowID, fixture.nodeID, fixture.alphaCaseID))
	return fixture
}
