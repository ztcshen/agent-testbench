package main

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"agent-testbench/internal/store"
)

func TestCaseRunsCommandListsStoredCaseRuns(t *testing.T) {
	storeRef := configureNamedPostgreSQLActiveStore(t, "daily-case-runs-pg")
	runCaseRunsCommandListsStoredCaseRuns(t, storeRef, "PostgreSQL")
}

func TestCaseRunsCommandUsesNamedMySQLActiveStore(t *testing.T) {
	storeRef := configureNamedMySQLActiveStore(t, "daily-case-runs-mysql")
	runCaseRunsCommandListsStoredCaseRuns(t, storeRef, "MySQL")
}

func runCaseRunsCommandListsStoredCaseRuns(t *testing.T, storeRef string, label string) {
	t.Helper()
	fixture := seedCaseRunsCommandFixture(t, storeRef, label)
	out := runCLI(t, "case", "runs", "--run", fixture.runID, "--json")
	assertCaseRunsReport(t, label, out, fixture)
}

func TestCaseEvidenceCommandReadsCaseRunEvidence(t *testing.T) {
	storeRef := configureNamedPostgreSQLActiveStore(t, "daily-case-evidence-pg")
	runCaseEvidenceCommandReadsCaseRunEvidence(t, storeRef, "PostgreSQL")
}

func TestCaseEvidenceCommandUsesNamedMySQLActiveStore(t *testing.T) {
	storeRef := configureNamedMySQLActiveStore(t, "daily-case-evidence-mysql")
	runCaseEvidenceCommandReadsCaseRunEvidence(t, storeRef, "MySQL")
}

func runCaseEvidenceCommandReadsCaseRunEvidence(t *testing.T, storeRef string, label string) {
	t.Helper()
	fixture := seedCaseEvidenceCommandFixture(t, storeRef, label)
	out := runCLI(t, "case", "evidence", "--case-run", fixture.caseRunID, "--json")
	assertCaseEvidencePayload(t, label, out, fixture)
}

func TestCaseTimingCommandSummarizesStoredCaseRuns(t *testing.T) {
	storeRef := configureNamedPostgreSQLActiveStore(t, "daily-case-timing-pg")
	runCaseTimingCommandSummarizesStoredCaseRuns(t, storeRef, "PostgreSQL")
}

func TestCaseTimingCommandUsesNamedMySQLActiveStore(t *testing.T) {
	storeRef := configureNamedMySQLActiveStore(t, "daily-case-timing-mysql")
	runCaseTimingCommandSummarizesStoredCaseRuns(t, storeRef, "MySQL")
}

func runCaseTimingCommandSummarizesStoredCaseRuns(t *testing.T, storeRef string, label string) {
	t.Helper()
	fixture := seedCaseTimingCommandFixture(t, storeRef, label)
	out := runCLI(t, "case", "timing", "--kind", "case", "--max-age-minutes", "1", "--json")
	assertCaseTimingPayload(t, label, out, fixture)
}

func TestCaseQueryCommandsAcceptStoreFlag(t *testing.T) {
	storeRef := createCaseQueryStoreFlagStore(t)
	assertCaseQueryStoreFlagCommands(t, storeRef)
}

func TestCaseReadCommandsUseNamedSQLiteActiveStore(t *testing.T) {
	configureNamedSQLiteActiveStore(t, "daily-case-read-sqlite")
	runID := uniqueTestID(t, "case-run-sqlite")
	createStoredCaseRun(t, runID, "SQLite")

	if out := runCLI(t, "case", "runs", "--json"); !strings.Contains(out, runID) {
		t.Fatalf("SQLite case runs output = %q", out)
	}
	if out := runCLI(t, "case", "evidence", "--case-run", runID+".case", "--json"); !strings.Contains(out, runID) || !strings.Contains(out, "/v1/items") {
		t.Fatalf("SQLite case evidence output = %q", out)
	}
	if out := runCLI(t, "case", "timing", "--kind", "case", "--json"); !strings.Contains(out, `"caseRunCount": 1`) {
		t.Fatalf("SQLite case timing output = %q", out)
	}
}

func TestCaseRunsCommandPrefersParentWorkflowCaseRunsOverBatchChildren(t *testing.T) {
	storeRef := configureNamedSQLiteActiveStore(t, "case-runs-parent-workflow-sqlite")
	ctx, s := openCaseQueryStore(t, storeRef, "SQLite")
	fixture := seedCaseRunsBatchSummaryFixture(t, ctx, s, "SQLite", true)

	out := runCLI(t, "case", "runs", "--run", fixture.parentRunID, "--json")
	var report caseRunsCommandReport
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatalf("decode parent workflow case runs: %v\n%s", err, out)
	}
	if len(report.CaseRuns) != 3 || len(report.Warnings) != 0 {
		t.Fatalf("parent workflow case runs should not include child run duplicates: %#v", report)
	}
	for _, item := range report.CaseRuns {
		if item.RunID != fixture.parentRunID {
			t.Fatalf("parent workflow case run should stay on parent run: %#v", report)
		}
	}
}

func TestCaseRunsCommandReadsBatchChildrenWhenParentHasNoCaseRuns(t *testing.T) {
	storeRef := configureNamedSQLiteActiveStore(t, "case-runs-batch-children-sqlite")
	ctx, s := openCaseQueryStore(t, storeRef, "SQLite")
	fixture := seedCaseRunsBatchSummaryFixture(t, ctx, s, "SQLite", false)

	out := runCLI(t, "case", "runs", "--run", fixture.parentRunID, "--json")
	var report caseRunsCommandReport
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatalf("decode batch child case runs: %v\n%s", err, out)
	}
	if len(report.CaseRuns) != 3 || len(report.Warnings) != 1 {
		t.Fatalf("batch child case runs should be resolved from summary: %#v", report)
	}
	for _, item := range report.CaseRuns {
		if item.RunID == fixture.parentRunID {
			t.Fatalf("batch child case run should come from child runs: %#v", report)
		}
	}
}

type caseRunsBatchSummaryFixture struct {
	parentRunID string
}

func seedCaseRunsBatchSummaryFixture(t *testing.T, ctx context.Context, s store.Store, label string, withParentCaseRuns bool) caseRunsBatchSummaryFixture {
	t.Helper()
	parentRunID := uniqueTestID(t, "run.workflow-parent")
	started := time.Date(2026, 6, 19, 6, 0, 0, 0, time.UTC)
	steps := make([]map[string]string, 0, 3)
	parentCaseRuns := make([]store.APICaseRun, 0, 3)
	for index := 1; index <= 3; index++ {
		caseID := uniqueTestID(t, "case.workflow-step")
		childRunID := uniqueTestID(t, "run.workflow-child")
		stepID := uniqueTestID(t, "step.workflow")
		steps = append(steps, map[string]string{
			"runId":  childRunID,
			"caseId": caseID,
			"stepId": stepID,
		})
		recordCaseQueryRun(t, ctx, s, label, store.Run{
			ID:         childRunID,
			ProfileID:  "profile.workflow",
			WorkflowID: "workflow.alpha",
			Status:     store.StatusPassed,
			StartedAt:  started,
			FinishedAt: started.Add(time.Second),
		})
		recordCaseQueryAPICaseRun(t, ctx, s, label, store.APICaseRun{
			ID:                 childRunID + ".case",
			RunID:              childRunID,
			CaseID:             caseID,
			Status:             store.StatusPassed,
			RequestSummaryJSON: `{"method":"GET","path":"/child"}`,
			StartedAt:          started,
			FinishedAt:         started.Add(100 * time.Millisecond),
		})
		if withParentCaseRuns {
			parentCaseRuns = append(parentCaseRuns, store.APICaseRun{
				ID:                 parentRunID + "." + stepID,
				RunID:              parentRunID,
				CaseID:             caseID,
				Status:             store.StatusPassed,
				RequestSummaryJSON: `{"method":"GET","path":"/parent"}`,
				StartedAt:          started,
				FinishedAt:         started.Add(100 * time.Millisecond),
			})
		}
	}
	summary, err := json.Marshal(map[string]any{"steps": steps})
	if err != nil {
		t.Fatalf("marshal batch summary: %v", err)
	}
	recordCaseQueryRun(t, ctx, s, label, store.Run{
		ID:          parentRunID,
		ProfileID:   "profile.workflow",
		WorkflowID:  "workflow.alpha",
		Status:      store.StatusPassed,
		SummaryJSON: string(summary),
		StartedAt:   started,
		FinishedAt:  started.Add(time.Second),
	})
	for _, caseRun := range parentCaseRuns {
		recordCaseQueryAPICaseRun(t, ctx, s, label, caseRun)
	}
	return caseRunsBatchSummaryFixture{parentRunID: parentRunID}
}
