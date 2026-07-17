package controlplane

import (
	"context"
	"errors"
	"os"
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
	parentEvidenceRoot := filepath.Join(t.TempDir(), "parent-evidence")
	childEvidenceRootAbsolute := filepath.Join(t.TempDir(), "child-evidence")
	workingDirectory, err := os.Getwd()
	if err != nil {
		t.Fatalf("get working directory: %v", err)
	}
	childEvidenceRoot, err := filepath.Rel(workingDirectory, childEvidenceRootAbsolute)
	if err != nil {
		t.Fatalf("make child Evidence root relative: %v", err)
	}
	if err := os.MkdirAll(childEvidenceRootAbsolute, 0o755); err != nil {
		t.Fatalf("create child Evidence root: %v", err)
	}
	requestPath := filepath.Join(childEvidenceRootAbsolute, "request.json")
	requestURI := filepath.Join(childEvidenceRoot, "request.json")
	if err := os.WriteFile(requestPath, []byte(`{"method":"POST","path":"/copied-evidence"}`), 0o644); err != nil {
		t.Fatalf("write child request Evidence: %v", err)
	}
	for _, run := range []store.Run{
		{ID: parentRunID, ProfileID: "sample", WorkflowID: "workflow.parent", Status: store.StatusRunning, EvidenceRoot: parentEvidenceRoot, SummaryJSON: `{}`, CreatedAt: now, UpdatedAt: now},
		{ID: childRunID, ProfileID: "sample", WorkflowID: "workflow.parent", Status: store.StatusPassed, EvidenceRoot: childEvidenceRoot, SummaryJSON: `{}`, CreatedAt: now, UpdatedAt: now},
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
		URI: requestURI, MediaType: "application/json", LabelsJSON: `{"caseId":"case-one","stepId":"step-one"}`, CreatedAt: now,
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
	if err != nil || len(evidence) != 1 || evidence[0].RunID != parentRunID || evidence[0].StepID != "step-one" || evidence[0].URI != requestPath {
		t.Fatalf("parent Evidence = %#v err=%v", evidence, err)
	}
	payload, ok, err := CaseEvidencePayloadForRunID(ctx, runtime, parentRunID, "case-one", "step-one")
	if err != nil || !ok {
		t.Fatalf("parent case Evidence payload ok=%t err=%v", ok, err)
	}
	request := mapFromAny(mapFromAny(payload[apiFieldEvidence])["request"])
	if request["path"] != "/copied-evidence" {
		t.Fatalf("parent request Evidence = %#v", request)
	}
	lifecycle := mapFromAny(mapFromAny(request["attachment"])["lifecycle"])
	if lifecycle[evidenceLifecycleAvailable] != true || lifecycle["path"] != requestPath {
		t.Fatalf("parent request Evidence lifecycle = %#v", lifecycle)
	}
	tasks, err := runtime.ListPostProcessTasks(ctx, parentRunID)
	if err != nil || len(tasks) != 1 || tasks[0].RunID != parentRunID || tasks[0].StepID != "step-one" || tasks[0].Status != store.StatusPassed {
		t.Fatalf("parent post-process tasks = %#v err=%v", tasks, err)
	}
}

func TestStableCopiedWorkflowEvidenceURI(t *testing.T) {
	sourceRootAbsolute := filepath.Join(t.TempDir(), "source-evidence")
	workingDirectory, err := os.Getwd()
	if err != nil {
		t.Fatalf("get working directory: %v", err)
	}
	sourceRoot, err := filepath.Rel(workingDirectory, sourceRootAbsolute)
	if err != nil {
		t.Fatalf("make source Evidence root relative: %v", err)
	}
	wantPath := filepath.Join(sourceRootAbsolute, "request.json")
	tests := []struct {
		name string
		uri  string
		root string
		want string
	}{
		{name: "bare relative path", uri: "request.json", root: sourceRoot, want: wantPath},
		{name: "source-root-prefixed relative path", uri: filepath.Join(sourceRoot, "request.json"), root: sourceRoot, want: wantPath},
		{name: "absolute path", uri: wantPath, root: sourceRoot, want: wantPath},
		{name: "absolute file URI", uri: "file://" + wantPath, root: sourceRoot, want: "file://" + wantPath},
		{name: "remote URI", uri: "https://evidence.example.test/request.json", root: sourceRoot, want: "https://evidence.example.test/request.json"},
		{name: "relative file URI", uri: "file://request.json", root: sourceRoot, want: "file://" + wantPath},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := stableCopiedWorkflowEvidenceURI(test.uri, test.root)
			if err != nil {
				t.Fatalf("stable copied Evidence URI: %v", err)
			}
			if got != test.want {
				t.Fatalf("stable copied Evidence URI = %q, want %q", got, test.want)
			}
		})
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
