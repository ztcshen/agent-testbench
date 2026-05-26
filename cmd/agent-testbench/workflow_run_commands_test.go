package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"agent-testbench/internal/store"
)

type workflowRunCommandFixture struct {
	runID      string
	workflowID string
	caseID     string
}

type workflowRunCommandList struct {
	OK           bool `json:"ok"`
	WorkflowRuns []struct {
		ID         string `json:"id"`
		WorkflowID string `json:"workflowId"`
		Status     string `json:"status"`
		StepCount  int    `json:"stepCount"`
	} `json:"workflowRuns"`
}

type workflowRunCommandDetail struct {
	OK      bool           `json:"ok"`
	Run     map[string]any `json:"run"`
	Summary struct {
		Steps []map[string]any `json:"steps"`
	} `json:"summary"`
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
	fixture := recordWorkflowRunCommandFixture(t, ctx, s, label)

	requireWorkflowRunCommandList(t, label, fixture)
	requireWorkflowRunCommandDetail(t, label, "workflow run", fixture.runID, "step.one", runCLI(t, "workflow", "run", "--run", fixture.runID, "--json"))
	requireWorkflowRunCommandDetail(t, label, "workflow step", fixture.runID, "step.one", runCLI(t, "workflow", "step", "--run", fixture.runID, "--step", "step.one", "--json"))
	requireWorkflowRunCommandLatestStep(t, label, fixture)
	requireWorkflowRunCommandStoreOverrides(t, storeRef, label, fixture)
}

func recordWorkflowRunCommandFixture(t *testing.T, ctx context.Context, s store.Store, label string) workflowRunCommandFixture {
	t.Helper()
	fixture := workflowRunCommandFixture{
		runID:      uniqueTestID(t, "run.workflow"),
		workflowID: uniqueTestID(t, "workflow.alpha"),
		caseID:     uniqueTestID(t, "case.alpha"),
	}
	started := time.Date(2026, 5, 18, 10, 0, 0, 0, time.UTC)
	if _, err := s.CreateRun(ctx, store.Run{
		ID:         fixture.runID,
		ProfileID:  uniqueTestID(t, "profile.workflow-runs"),
		WorkflowID: fixture.workflowID,
		Status:     store.StatusPassed,
		SummaryJSON: fmt.Sprintf(`{
			"summary":{"stepCount":1,"passed":1},
			"steps":[{"stepId":"step.one","caseId":%q,"status":"passed"}]
		}`, fixture.caseID),
		StartedAt:  started,
		FinishedAt: started.Add(time.Second),
		CreatedAt:  started,
		UpdatedAt:  started.Add(time.Second),
	}); err != nil {
		t.Fatalf("create %s run: %v", label, err)
	}
	return fixture
}

func requireWorkflowRunCommandList(t *testing.T, label string, fixture workflowRunCommandFixture) {
	t.Helper()
	listOut := runCLI(t, "workflow", "runs", "--json")
	var list workflowRunCommandList
	if err := json.Unmarshal([]byte(listOut), &list); err != nil {
		t.Fatalf("decode %s workflow runs json: %v\n%s", label, err, listOut)
	}
	foundRun := false
	for _, item := range list.WorkflowRuns {
		if item.ID == fixture.runID && item.WorkflowID == fixture.workflowID && item.StepCount == 1 {
			foundRun = true
			break
		}
	}
	if !list.OK || !foundRun {
		t.Fatalf("%s workflow runs = %#v", label, list)
	}
}

func requireWorkflowRunCommandDetail(t *testing.T, label string, name string, runID string, stepID string, raw string) workflowRunCommandDetail {
	t.Helper()
	var detail workflowRunCommandDetail
	if err := json.Unmarshal([]byte(raw), &detail); err != nil {
		t.Fatalf("decode %s %s json: %v\n%s", label, name, err, raw)
	}
	if !detail.OK || detail.Run["id"] != runID || len(detail.Summary.Steps) != 1 || detail.Summary.Steps[0]["stepId"] != stepID {
		t.Fatalf("%s %s detail = %#v", label, name, detail)
	}
	return detail
}

func requireWorkflowRunCommandLatestStep(t *testing.T, label string, fixture workflowRunCommandFixture) {
	t.Helper()
	latestOut := runCLI(t, "workflow", "latest-step", "--workflow", fixture.workflowID, "--step", "step.one", "--json")
	latestDetail := requireWorkflowRunCommandDetail(t, label, "latest workflow step", fixture.runID, "step.one", latestOut)
	if latestDetail.Summary.Steps[0]["caseId"] != fixture.caseID {
		t.Fatalf("latest %s workflow step detail = %#v", label, latestDetail)
	}
}

func requireWorkflowRunCommandStoreOverrides(t *testing.T, storeRef string, label string, fixture workflowRunCommandFixture) {
	t.Helper()
	if out := runCLI(t, "workflow", "runs", "--store", storeRef, "--json"); !strings.Contains(out, fixture.runID) {
		t.Fatalf("%s workflow runs --store output = %q", label, out)
	}
	if out := runCLI(t, "workflow", "run", "--store", storeRef, "--run", fixture.runID, "--json"); !strings.Contains(out, "step.one") {
		t.Fatalf("%s workflow run --store output = %q", label, out)
	}
	if out := runCLI(t, "workflow", "step", "--store", storeRef, "--run", fixture.runID, "--step", "step.one", "--json"); !strings.Contains(out, fixture.caseID) {
		t.Fatalf("%s workflow step --store output = %q", label, out)
	}
	if out := runCLI(t, "workflow", "latest-step", "--store", storeRef, "--workflow", fixture.workflowID, "--step", "step.one", "--json"); !strings.Contains(out, fixture.runID) {
		t.Fatalf("%s workflow latest-step --store output = %q", label, out)
	}
}

func TestWorkflowRunCommandsUseNamedSQLiteActiveStore(t *testing.T) {
	storeRef := configureNamedSQLiteActiveStore(t, "daily-workflow-runs-sqlite")
	runWorkflowRunCommandsReadStoredRuns(t, storeRef, "SQLite")
}
