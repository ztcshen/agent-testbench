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

func TestMapGatePassesCompletedPlanWithEvidence(t *testing.T) {
	ctx := context.Background()
	storeRef := seedMapGateStore(t, ctx, mapGatePlanFixture(store.StatusPassed), true)

	out := runCLI(t, "map", "gate", "--store", storeRef, "--plan", "plan.gate", "--require-passed", "--require-tasks", "--require-evidence", "--json")
	report := decodeMapGateReport(t, out)

	if !report.OK || report.PlanID != "plan.gate" || report.Status != store.StatusPassed {
		t.Fatalf("map gate should pass = %#v", report)
	}
	if report.Counts.TotalTasks != 2 || report.Counts.PassedTasks != 2 || report.Counts.EvidenceComplete != 2 {
		t.Fatalf("map gate counts = %#v", report.Counts)
	}
	if !report.Gates.PlanPassed || !report.Gates.TasksPassed || !report.Gates.EvidenceComplete {
		t.Fatalf("map gate booleans = %#v", report.Gates)
	}
}

func TestMapGateCountsEvidenceStoredUnderCaseRunID(t *testing.T) {
	ctx := context.Background()
	record := mapGatePlanFixture(store.StatusPassed)
	storeRef := "sqlite://" + filepath.Join(t.TempDir(), "map-gate-case-run-evidence.sqlite")
	runtime, err := openStore(ctx, storeRef)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { closeCLIStore(runtime) })
	if err := runtime.SaveTestMapPlan(ctx, record); err != nil {
		t.Fatalf("save map plan: %v", err)
	}
	recordMapGateEvidence(t, ctx, runtime, "run.workflow.step.case", "run.workflow.step.case")
	recordMapGateEvidence(t, ctx, runtime, "run.case.case", "run.case.case")

	out := runCLI(t, "map", "gate", "--store", storeRef, "--plan", "plan.gate", "--require-passed", "--require-tasks", "--require-evidence", "--json")
	report := decodeMapGateReport(t, out)

	if !report.OK || report.Counts.EvidenceComplete != 2 || !report.Gates.EvidenceComplete {
		t.Fatalf("map gate should count case-run-scoped evidence = %#v", report)
	}
}

func TestMapGateExemptsPassedMaterializationTaskFromEvidence(t *testing.T) {
	ctx := context.Background()
	record := mapGatePlanFixture(store.StatusPassed)
	record.Tasks = append(record.Tasks[:1], append([]store.TestMapPlanTask{{
		ID:                "task.materialization",
		PlanID:            "plan.gate",
		Index:             1,
		Kind:              mapplanner.TaskReuseMaterialized,
		Status:            store.StatusPassed,
		MaterializationID: "fixture.before.case",
		SummaryJSON:       `{"fixtureId":"fixture.before.case"}`,
		StartedAt:         record.Tasks[0].StartedAt,
		FinishedAt:        record.Tasks[0].FinishedAt,
	}}, record.Tasks[1:]...)...)
	storeRef := seedMapGateStore(t, ctx, record, true)

	out := runCLI(t, "map", "gate", "--store", storeRef, "--plan", "plan.gate", "--require-passed", "--require-tasks", "--require-evidence", "--json")
	report := decodeMapGateReport(t, out)

	if !report.OK || !report.Gates.EvidenceComplete || report.Counts.EvidenceComplete != 3 {
		t.Fatalf("passed materialization should be evidence-complete by provenance = %#v", report)
	}
	if len(report.MissingEvidence) != 0 {
		t.Fatalf("materialization task should not be missing evidence = %#v", report.MissingEvidence)
	}
}

func TestMapGateFailsForFailedTaskAndMissingEvidence(t *testing.T) {
	ctx := context.Background()
	storeRef := seedMapGateStore(t, ctx, mapGatePlanFixture(store.StatusFailed), false)

	out := runCLIFails(t, "map", "gate", "--store", storeRef, "--plan", "plan.gate", "--require-passed", "--require-tasks", "--require-evidence", "--json")
	report := decodeMapGateReport(t, out)

	if report.OK || report.Gates.PlanPassed || report.Gates.TasksPassed || report.Gates.EvidenceComplete {
		t.Fatalf("map gate should fail = %#v", report)
	}
	if len(report.FailedTasks) != 1 || report.FailedTasks[0].ID != "task.case" {
		t.Fatalf("map gate failed tasks = %#v", report.FailedTasks)
	}
	if len(report.MissingEvidence) != 2 {
		t.Fatalf("map gate missing evidence = %#v", report.MissingEvidence)
	}
	if !strings.Contains(strings.Join(report.NextActions, "\n"), "map run --plan 'plan.gate' --retry-failed") {
		t.Fatalf("map gate next actions = %#v", report.NextActions)
	}
}

type mapGateCommandReport struct {
	OK     bool   `json:"ok"`
	PlanID string `json:"planId"`
	Status string `json:"status"`
	Counts struct {
		TotalTasks       int `json:"totalTasks"`
		PassedTasks      int `json:"passedTasks"`
		FailedTasks      int `json:"failedTasks"`
		BlockedTasks     int `json:"blockedTasks"`
		OtherTasks       int `json:"otherTasks"`
		EvidenceComplete int `json:"evidenceComplete"`
	} `json:"counts"`
	Gates struct {
		PlanPassed       bool `json:"planPassed"`
		TasksPresent     bool `json:"tasksPresent"`
		TasksPassed      bool `json:"tasksPassed"`
		EvidenceComplete bool `json:"evidenceComplete"`
	} `json:"gates"`
	FailedTasks     []mapGateCommandTask `json:"failedTasks"`
	MissingEvidence []mapGateCommandTask `json:"missingEvidence"`
	NextActions     []string             `json:"nextActions"`
}

type mapGateCommandTask struct {
	ID            string `json:"id"`
	Status        string `json:"status"`
	WorkflowRunID string `json:"workflowRunId"`
	APICaseRunID  string `json:"apiCaseRunId"`
	EvidenceCount int    `json:"evidenceCount"`
}

func decodeMapGateReport(t *testing.T, out string) mapGateCommandReport {
	t.Helper()
	var report mapGateCommandReport
	if err := json.Unmarshal([]byte(extractJSONObject(t, out)), &report); err != nil {
		t.Fatalf("decode map gate json: %v\n%s", err, out)
	}
	return report
}

func seedMapGateStore(t *testing.T, ctx context.Context, record store.TestMapPlanRecord, withEvidence bool) string {
	t.Helper()
	storeRef := "sqlite://" + filepath.Join(t.TempDir(), "map-gate.sqlite")
	runtime, err := openStore(ctx, storeRef)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { closeCLIStore(runtime) })
	if err := runtime.SaveTestMapPlan(ctx, record); err != nil {
		t.Fatalf("save map plan: %v", err)
	}
	if withEvidence {
		recordMapGateEvidence(t, ctx, runtime, "run.workflow.step", "run.workflow.step.case")
		recordMapGateEvidence(t, ctx, runtime, "run.case", "run.case.case")
	}
	return storeRef
}

func recordMapGateEvidence(t *testing.T, ctx context.Context, runtime store.Store, runID string, caseRunID string) {
	t.Helper()
	if _, err := runtime.RecordEvidence(ctx, store.EvidenceRecord{
		ID:        "evidence." + runID,
		RunID:     runID,
		CaseRunID: caseRunID,
		Kind:      "summary",
		URI:       "evidence/" + runID + "/summary.json",
		MediaType: "application/json",
		CreatedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("record evidence: %v", err)
	}
}

func mapGatePlanFixture(status string) store.TestMapPlanRecord {
	taskStatus := store.StatusPassed
	if status == store.StatusFailed {
		taskStatus = store.StatusFailed
	}
	now := time.Now().UTC().Add(-time.Minute)
	return store.TestMapPlanRecord{
		Instance: store.TestMapPlanInstance{
			ID:        "plan.gate",
			MapID:     "map.gate",
			ProfileID: "profile.gate",
			Mode:      mapplanner.ModeRun,
			Status:    status,
			StartedAt: now,
		},
		Tasks: []store.TestMapPlanTask{
			{
				ID:            "task.workflow",
				PlanID:        "plan.gate",
				Index:         0,
				Kind:          mapplanner.TaskRunPath,
				Status:        store.StatusPassed,
				PathID:        "path.gate",
				WorkflowRunID: "run.workflow",
				SummaryJSON:   `{"steps":[{"apiCaseRunId":"run.workflow.step.case","runId":"run.workflow.step","status":"passed"}]}`,
				StartedAt:     now,
				FinishedAt:    now.Add(time.Second),
			},
			{
				ID:           "task.case",
				PlanID:       "plan.gate",
				Index:        1,
				Kind:         mapplanner.TaskRunCase,
				Status:       taskStatus,
				CaseID:       "case.gate",
				APICaseRunID: "run.case.case",
				StartedAt:    now,
				FinishedAt:   now.Add(time.Second),
			},
		},
	}
}
