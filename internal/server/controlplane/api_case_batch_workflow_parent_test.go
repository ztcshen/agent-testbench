package controlplane

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"agent-testbench/internal/store"
	"agent-testbench/internal/store/sqlite"
)

type workflowParentReadFailureStore struct {
	store.Store
	failEvidence bool
	failTasks    bool
}

func (s workflowParentReadFailureStore) ListEvidence(ctx context.Context, runID string) ([]store.EvidenceRecord, error) {
	if s.failEvidence {
		return nil, errors.New("injected Evidence read failure")
	}
	return s.Store.ListEvidence(ctx, runID)
}

func (s workflowParentReadFailureStore) ListPostProcessTasks(ctx context.Context, runID string) ([]store.PostProcessTask, error) {
	if s.failTasks {
		return nil, errors.New("injected task read failure")
	}
	return s.Store.ListPostProcessTasks(ctx, runID)
}

func TestMaterializeAPICaseBatchWorkflowParentCopiesStepEvidenceAndTasks(t *testing.T) {
	ctx := context.Background()
	runtime, err := sqlite.Open(ctx, sqlite.Config{Path: filepath.Join(t.TempDir(), "store.sqlite")})
	if err != nil {
		t.Fatalf("open Store: %v", err)
	}
	defer runtime.Close()

	now := time.Now().UTC()
	parentRunID := "batch.workflow-parent.001"
	childRunID := parentRunID + ".step-one.case-one"
	childCaseRunID := childRunID + ".case"
	for _, run := range []store.Run{
		{ID: parentRunID, ProfileID: "sample", WorkflowID: "workflow.parent", Status: store.StatusRunning, SummaryJSON: `{}`, CreatedAt: now, UpdatedAt: now},
		{ID: childRunID, ProfileID: "sample", WorkflowID: "workflow.parent", Status: store.StatusPassed, SummaryJSON: `{}`, CreatedAt: now, UpdatedAt: now},
	} {
		if _, err := runtime.CreateRun(ctx, run); err != nil {
			t.Fatalf("create run %s: %v", run.ID, err)
		}
	}
	if _, err := runtime.RecordAPICaseRun(ctx, store.APICaseRun{
		ID: childCaseRunID, RunID: childRunID, CaseID: "case-one", Status: store.StatusPassed,
		RequestSummaryJSON: `{}`, AssertionSummaryJSON: `{"status":"passed"}`, StartedAt: now, FinishedAt: now, CreatedAt: now,
	}); err != nil {
		t.Fatalf("record child case run: %v", err)
	}
	if _, err := runtime.RecordEvidence(ctx, store.EvidenceRecord{
		ID: childRunID + ".request", RunID: childRunID, CaseRunID: childCaseRunID, StepID: "step-one", Kind: "request",
		URI: "file:///tmp/request.json", MediaType: "application/json", LabelsJSON: `{"caseId":"case-one","stepId":"step-one"}`, CreatedAt: now,
	}); err != nil {
		t.Fatalf("record child Evidence: %v", err)
	}
	if _, err := runtime.RecordPostProcessTask(ctx, store.PostProcessTask{
		ID: childRunID + ".step-one.trace", RunID: childRunID, WorkflowID: "workflow.parent", StepID: "step-one", CaseID: "case-one",
		Kind: postProcessKindTraceTopology, Status: store.StatusPassed, StartedAt: now, FinishedAt: now, CreatedAt: now,
	}); err != nil {
		t.Fatalf("record child post-process task: %v", err)
	}

	report := apiCaseBatchRunReport{
		BatchRunID: parentRunID,
		WorkflowID: "workflow.parent",
		Status:     store.StatusPassed,
		Total:      1,
		Completed:  1,
		Passed:     1,
		FinishedAt: now.Format(time.RFC3339Nano),
		Cases: []apiCaseBatchCaseReport{{
			CaseID: "case-one", StepID: "step-one", RunID: childRunID, CaseRunID: childCaseRunID, Status: store.StatusPassed,
		}},
	}
	if err := materializeAPICaseBatchWorkflowParent(ctx, runtime, report); err != nil {
		t.Fatalf("materialize parent workflow run: %v", err)
	}

	caseRuns, err := runtime.ListAPICaseRuns(ctx, parentRunID)
	if err != nil || len(caseRuns) != 1 || caseRuns[0].CaseID != "case-one" || caseRuns[0].Status != store.StatusPassed {
		t.Fatalf("parent case runs = %#v err=%v", caseRuns, err)
	}
	evidence, err := runtime.ListEvidence(ctx, parentRunID)
	if err != nil || len(evidence) != 1 || evidence[0].RunID != parentRunID || evidence[0].StepID != "step-one" {
		t.Fatalf("parent Evidence = %#v err=%v", evidence, err)
	}
	tasks, err := runtime.ListPostProcessTasks(ctx, parentRunID)
	if err != nil || len(tasks) != 1 || tasks[0].RunID != parentRunID || tasks[0].StepID != "step-one" || tasks[0].Status != store.StatusPassed {
		t.Fatalf("parent post-process tasks = %#v err=%v", tasks, err)
	}
}

func TestMaterializeAPICaseBatchWorkflowParentFailsClosedOnSourceReadErrors(t *testing.T) {
	tests := []struct {
		name         string
		failEvidence bool
		failTasks    bool
		want         string
	}{
		{name: "Evidence", failEvidence: true, want: "list Evidence for source run"},
		{name: "post-process tasks", failTasks: true, want: "list post-process tasks for source run"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx := context.Background()
			runtime, err := sqlite.Open(ctx, sqlite.Config{Path: filepath.Join(t.TempDir(), "store.sqlite")})
			if err != nil {
				t.Fatalf("open Store: %v", err)
			}
			defer runtime.Close()

			now := time.Now().UTC()
			parentRunID := "batch.workflow-parent.read-failure"
			childRunID := parentRunID + ".step-one.case-one"
			for _, run := range []store.Run{
				{ID: parentRunID, ProfileID: "sample", WorkflowID: "workflow.parent", Status: store.StatusRunning, SummaryJSON: `{}`, CreatedAt: now, UpdatedAt: now},
				{ID: childRunID, ProfileID: "sample", WorkflowID: "workflow.parent", Status: store.StatusPassed, SummaryJSON: `{}`, CreatedAt: now, UpdatedAt: now},
			} {
				if _, err := runtime.CreateRun(ctx, run); err != nil {
					t.Fatalf("create run %s: %v", run.ID, err)
				}
			}

			report := apiCaseBatchRunReport{
				BatchRunID: parentRunID,
				WorkflowID: "workflow.parent",
				Status:     store.StatusPassed,
				Total:      1,
				Completed:  1,
				Passed:     1,
				FinishedAt: now.Format(time.RFC3339Nano),
				Cases: []apiCaseBatchCaseReport{{
					CaseID: "case-one", StepID: "step-one", RunID: childRunID, Status: store.StatusPassed,
				}},
			}
			failingStore := workflowParentReadFailureStore{
				Store:        runtime,
				failEvidence: test.failEvidence,
				failTasks:    test.failTasks,
			}
			err = materializeAPICaseBatchWorkflowParent(ctx, failingStore, report)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("materialize error = %v, want %q", err, test.want)
			}
		})
	}
}
