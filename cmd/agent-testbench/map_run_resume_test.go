package main

import (
	"strings"
	"testing"
	"time"

	"agent-testbench/internal/domain/mapplanner"
	"agent-testbench/internal/store"
)

func TestMapRunNextActionsQuotePlanID(t *testing.T) {
	actions := mapRunNextActions("plan with space")
	if len(actions) != 1 || !strings.Contains(actions[0], "--plan 'plan with space'") {
		t.Fatalf("next actions should quote plan id = %#v", actions)
	}
}

func TestPrepareExistingMapRunRecordResumeKeepsPassedTasks(t *testing.T) {
	record := mapRunResumeFixture()

	prepared := prepareExistingMapRunRecord(record, mapRunOptions{resumeRun: true})

	if prepared.Tasks[0].Status != store.StatusPassed || prepared.Tasks[0].WorkflowRunID != "run.already.passed" || prepared.Tasks[0].EvidenceRoot != "evidence/already" {
		t.Fatalf("resume should keep passed task execution metadata = %#v", prepared.Tasks[0])
	}
	if prepared.Tasks[1].Status != mapplanner.TaskStatusPlanned || prepared.Tasks[1].APICaseRunID != "" || prepared.Tasks[1].Reason != "" || !prepared.Tasks[1].FinishedAt.IsZero() {
		t.Fatalf("resume should reset failed task for execution = %#v", prepared.Tasks[1])
	}
	if prepared.Tasks[2].Status != mapplanner.TaskStatusPlanned || !prepared.Tasks[2].StartedAt.IsZero() || !prepared.Tasks[2].FinishedAt.IsZero() {
		t.Fatalf("resume should leave planned task runnable without erasing started time = %#v", prepared.Tasks[2])
	}
}

func TestPrepareExistingMapRunRecordResetsInstanceStartedAt(t *testing.T) {
	record := mapRunResumeFixture()
	record.Instance.Mode = mapplanner.ModeExplain
	record.Instance.StartedAt = time.Now().UTC().Add(-2 * time.Hour)

	prepared := prepareExistingMapRunRecord(record, mapRunOptions{})

	if !prepared.Instance.StartedAt.After(record.Instance.StartedAt) || prepared.Instance.Mode != mapplanner.ModeRun {
		t.Fatalf("existing plan execution should reset instance start time = %#v", prepared.Instance)
	}
}

func TestPrepareExistingMapRunRecordRetryFailedOnlyResetsFailedTasks(t *testing.T) {
	record := mapRunResumeFixture()

	prepared := prepareExistingMapRunRecord(record, mapRunOptions{retryFailed: true})

	if prepared.Tasks[0].Status != store.StatusPassed || prepared.Tasks[0].WorkflowRunID != "run.already.passed" {
		t.Fatalf("retry failed should keep passed task = %#v", prepared.Tasks[0])
	}
	if prepared.Tasks[1].Status != store.StatusFailed || prepared.Tasks[1].APICaseRunID != "" || prepared.Tasks[1].Reason != "" {
		t.Fatalf("retry failed should reset failed task = %#v", prepared.Tasks[1])
	}
	if prepared.Tasks[2].Status != mapplanner.TaskStatusPlanned || prepared.Tasks[2].APICaseRunID != "" || !prepared.Tasks[2].StartedAt.IsZero() {
		t.Fatalf("retry failed should park unrelated planned task without executing it = %#v", prepared.Tasks[2])
	}
}

func TestPrepareExistingMapRunRecordRerunTaskOnlyResetsSelectedTasks(t *testing.T) {
	record := mapRunResumeFixture()

	prepared := prepareExistingMapRunRecord(record, mapRunOptions{rerunTaskIDs: []string{"task.path"}})

	if prepared.Tasks[0].Status != mapplanner.TaskStatusPlanned || prepared.Tasks[0].WorkflowRunID != "" || prepared.Tasks[0].EvidenceRoot != "" {
		t.Fatalf("rerun task should reset selected task = %#v", prepared.Tasks[0])
	}
	if prepared.Tasks[1].Status != store.StatusFailed || prepared.Tasks[1].APICaseRunID != "run.failed.case" || prepared.Tasks[1].Reason == "" {
		t.Fatalf("rerun task should keep unselected failed task metadata = %#v", prepared.Tasks[1])
	}
}

func mapRunResumeFixture() store.TestMapPlanRecord {
	started := time.Now().UTC().Add(-2 * time.Minute)
	finished := started.Add(time.Minute)
	return store.TestMapPlanRecord{
		Instance: store.TestMapPlanInstance{
			ID:        "plan.resume",
			MapID:     "map.resume",
			ProfileID: "profile.resume",
			Mode:      mapplanner.ModeRun,
			Status:    store.StatusFailed,
			StartedAt: started,
		},
		Tasks: []store.TestMapPlanTask{
			{
				ID:            "task.path",
				PlanID:        "plan.resume",
				Index:         0,
				Kind:          mapplanner.TaskRunPath,
				Status:        store.StatusPassed,
				PathID:        "path.empty",
				WorkflowRunID: "run.already.passed",
				EvidenceRoot:  "evidence/already",
				StartedAt:     started,
				FinishedAt:    finished,
			},
			{
				ID:           "task.case.failed",
				PlanID:       "plan.resume",
				Index:        1,
				Kind:         mapplanner.TaskRunCase,
				Status:       store.StatusFailed,
				CaseID:       "case.failed",
				APICaseRunID: "run.failed.case",
				EvidenceRoot: "evidence/failed",
				Reason:       "assertion failed",
				StartedAt:    started,
				FinishedAt:   finished,
			},
			{
				ID:     "task.case.planned",
				PlanID: "plan.resume",
				Index:  2,
				Kind:   mapplanner.TaskRunCase,
				Status: mapplanner.TaskStatusPlanned,
				CaseID: "case.planned",
			},
		},
	}
}
