package main

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"agent-testbench/internal/domain/commandline"
	"agent-testbench/internal/store"
	"agent-testbench/internal/store/sqlite"
)

type workflowGateFailureFixture struct {
	storePath  string
	runID      string
	workflowID string
}

type workflowGateFullReport struct {
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

func TestWorkflowGateFailsWithFailedStepAndActionableReport(t *testing.T) {
	fixture := writeWorkflowGateFailureStore(t)
	out := runCLIFails(t, "workflow", "gate", "--store", "sqlite://"+fixture.storePath, "--run", fixture.runID, "--require-passed", "--require-steps", "--require-evidence", "--json")
	report := decodeWorkflowGateFullReport(t, out)
	requireWorkflowGateFailureReport(t, fixture, report)
}

func TestWorkflowGateCountsEvidenceStoredUnderCaseRunID(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	storePath := filepath.Join(dir, "store.sqlite")
	s, err := sqlite.Open(ctx, sqlite.Config{Path: storePath})
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	started := time.Date(2026, 6, 2, 6, 0, 0, 0, time.UTC)
	runID := "run.workflow.case-evidence"
	caseRunID := runID + ".case"
	if _, err := s.CreateRun(ctx, store.Run{
		ID:           runID,
		ProfileID:    "sample",
		WorkflowID:   "workflow.case-evidence",
		Status:       store.StatusPassed,
		EvidenceRoot: filepath.Join(dir, "evidence", runID),
		SummaryJSON:  `{"steps":[{"stepId":"step.submit","caseId":"case.submit","caseRunId":"` + caseRunID + `","status":"passed"}]}`,
		StartedAt:    started,
		FinishedAt:   started.Add(time.Second),
		CreatedAt:    started,
		UpdatedAt:    started.Add(time.Second),
	}); err != nil {
		t.Fatalf("create run: %v", err)
	}
	if _, err := s.RecordAPICaseRun(ctx, store.APICaseRun{
		ID:                   caseRunID,
		RunID:                runID,
		CaseID:               "case.submit",
		Status:               store.StatusPassed,
		RequestSummaryJSON:   `{"method":"POST","path":"/workflow","stepId":"step.submit"}`,
		AssertionSummaryJSON: `{"status":"passed"}`,
		StartedAt:            started,
		FinishedAt:           started.Add(time.Second),
		CreatedAt:            started,
	}); err != nil {
		t.Fatalf("record case run: %v", err)
	}
	if _, err := s.CreateRun(ctx, store.Run{
		ID:           caseRunID,
		ProfileID:    "sample",
		WorkflowID:   "workflow.case-evidence",
		Status:       store.StatusPassed,
		EvidenceRoot: filepath.Join(dir, "evidence", caseRunID),
		StartedAt:    started,
		FinishedAt:   started.Add(time.Second),
		CreatedAt:    started,
		UpdatedAt:    started.Add(time.Second),
	}); err != nil {
		t.Fatalf("create case-run evidence run: %v", err)
	}
	if _, err := s.RecordEvidence(ctx, store.EvidenceRecord{
		ID:        caseRunID + ".response",
		RunID:     caseRunID,
		CaseRunID: caseRunID,
		StepID:    "step.submit",
		Kind:      "http-response",
		URI:       filepath.Join(dir, "evidence", caseRunID, "response.json"),
		MediaType: "application/json",
		Summary:   `{"http_code":200}`,
		CreatedAt: started,
	}); err != nil {
		t.Fatalf("record case-run evidence: %v", err)
	}
	if err := s.Close(); err != nil {
		t.Fatalf("close store: %v", err)
	}

	out := runCLI(t, "workflow", "gate", "--store", "sqlite://"+storePath, "--run", runID, "--require-passed", "--require-steps", "--require-evidence", "--json")
	report := decodeWorkflowGateFullReport(t, out)
	if !report.OK || report.Counts.EvidenceComplete != 1 || !report.Gates.EvidenceComplete {
		t.Fatalf("workflow gate should count case-run scoped evidence = %#v", report)
	}
}

func TestWorkflowGateCountsEvidenceForCaseRunOnlyReferencedBySummary(t *testing.T) {
	dir := t.TempDir()
	storePath := filepath.Join(dir, "store.sqlite")
	ctx := context.Background()
	s, err := sqlite.Open(ctx, sqlite.Config{Path: storePath})
	if err != nil {
		t.Fatalf("open sqlite store: %v", err)
	}
	defer s.Close()
	runID := "run.workflow.summary-only"
	caseRunID := "run.workflow.summary-only.case"
	started := time.Date(2026, 6, 15, 9, 0, 0, 0, time.UTC)
	if _, err := s.CreateRun(ctx, store.Run{
		ID:          runID,
		ProfileID:   "sample",
		WorkflowID:  "workflow.summary-only",
		Status:      store.StatusPassed,
		SummaryJSON: `{"steps":[{"stepId":"trial","caseId":"case.trial","caseRunId":"` + caseRunID + `","status":"passed"}]}`,
		StartedAt:   started,
		FinishedAt:  started.Add(time.Second),
		CreatedAt:   started,
		UpdatedAt:   started.Add(time.Second),
	}); err != nil {
		t.Fatalf("create workflow run: %v", err)
	}
	if _, err := s.RecordEvidence(ctx, store.EvidenceRecord{
		ID:        "evidence.summary-only.response",
		RunID:     caseRunID,
		CaseRunID: caseRunID,
		StepID:    "trial",
		Kind:      "response",
		URI:       filepath.Join(dir, "response.json"),
		CreatedAt: started,
	}); err != nil {
		t.Fatalf("record summary-only case evidence: %v", err)
	}

	out := runCLI(t, "workflow", "gate", "--store", "sqlite://"+storePath, "--run", runID, "--require-passed", "--require-steps", "--require-evidence", "--json")
	report := decodeWorkflowGateFullReport(t, out)
	if !report.OK || report.Counts.EvidenceComplete != 1 || !report.Gates.EvidenceComplete {
		t.Fatalf("workflow gate should count summary caseRunId evidence = %#v", report)
	}
}

func TestWorkflowGateDoesNotUseStepEvidenceForAPIWorkflowSteps(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	storePath := filepath.Join(dir, "store.sqlite")
	s, err := sqlite.Open(ctx, sqlite.Config{Path: storePath})
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	started := time.Date(2026, 6, 3, 8, 0, 0, 0, time.UTC)
	runID := "run.workflow.step-evidence-api"
	if _, err := s.CreateRun(ctx, store.Run{
		ID:          runID,
		WorkflowID:  "workflow.api-step-evidence",
		Status:      store.StatusPassed,
		SummaryJSON: `{"steps":[{"stepId":"step.submit","caseId":"case.submit","status":"passed"}]}`,
		StartedAt:   started,
		FinishedAt:  started.Add(time.Second),
		CreatedAt:   started,
		UpdatedAt:   started.Add(time.Second),
	}); err != nil {
		t.Fatalf("create run: %v", err)
	}
	if _, err := s.RecordEvidence(ctx, store.EvidenceRecord{
		ID:        runID + ".step.submit",
		RunID:     runID,
		StepID:    "step.submit",
		Kind:      "workflow-step-note",
		MediaType: "application/json",
		Summary:   `{"note":"not case-run scoped"}`,
		CreatedAt: started,
	}); err != nil {
		t.Fatalf("record workflow-run step evidence: %v", err)
	}
	if err := s.Close(); err != nil {
		t.Fatalf("close store: %v", err)
	}

	out := runCLIFails(t, "workflow", "gate", "--store", "sqlite://"+storePath, "--run", runID, "--require-passed", "--require-steps", "--require-evidence", "--json")
	report := decodeWorkflowGateFullReport(t, out)
	if report.Gates.EvidenceComplete || report.Counts.EvidenceComplete != 0 {
		t.Fatalf("API workflow step should not use workflow-run step evidence as case evidence: %#v", report)
	}
}

func writeWorkflowGateFailureStore(t *testing.T) workflowGateFailureFixture {
	t.Helper()
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
	return workflowGateFailureFixture{storePath: storePath, runID: runID, workflowID: workflowID}
}

func decodeWorkflowGateFullReport(t *testing.T, raw string) workflowGateFullReport {
	t.Helper()
	var report workflowGateFullReport
	if err := json.Unmarshal([]byte(extractJSONObject(t, raw)), &report); err != nil {
		t.Fatalf("decode workflow gate json: %v\n%s", err, raw)
	}
	return report
}

func requireWorkflowGateFailureReport(t *testing.T, fixture workflowGateFailureFixture, report workflowGateFullReport) {
	t.Helper()
	if report.OK || report.RunID != fixture.runID || report.WorkflowID != fixture.workflowID || report.Status != store.StatusFailed {
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
	if !strings.Contains(next, "agent-testbench workflow step --run "+commandline.ShellQuote(fixture.runID)+" --step 'step.submit'") || !strings.Contains(next, "agent-testbench case diagnose --case-run 'run.workflow-gate.submit'") {
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

func TestWorkflowGateNextActionsInspectTaskStepMissingEvidence(t *testing.T) {
	report := workflowGateReport{
		RunID: "workflow-run-1",
		Gates: workflowGateGates{
			StepsPresent: true,
		},
		MissingEvidence: []workflowGateStep{
			{Kind: cliCommandTask, StepID: "postcondition", TaskRunID: "task-run-1"},
		},
	}

	actions := workflowGateNextActions(report, workflowGateOptions{RequireEvidence: true})
	joined := strings.Join(actions, "\n")
	if !strings.Contains(joined, "agent-testbench workflow step --run 'workflow-run-1' --step 'postcondition' --json") {
		t.Fatalf("task missing evidence should suggest inspecting workflow step: %#v", actions)
	}
	if strings.Contains(joined, "Workflow gate passed") {
		t.Fatalf("missing task evidence should not report no action needed: %#v", actions)
	}
}
