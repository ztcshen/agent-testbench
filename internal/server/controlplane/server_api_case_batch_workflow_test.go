package controlplane_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"agent-testbench/internal/domain/profile"
	"agent-testbench/internal/server/controlplane"
	"agent-testbench/internal/store"
	"agent-testbench/internal/store/sqlite"
)

type apiCaseBatchWorkflowSummaryForTest struct {
	Summary struct {
		ExpectedStepCount int `json:"expectedStepCount"`
		StepCount         int `json:"stepCount"`
		Passed            int `json:"passed"`
		Failed            int `json:"failed"`
	} `json:"summary"`
	Steps []struct {
		StepID string `json:"stepId"`
		CaseID string `json:"caseId"`
		Status string `json:"status"`
	} `json:"steps"`
	Acceptance struct {
		OK               bool   `json:"ok"`
		TemplateID       string `json:"templateId"`
		TopologyProvider string `json:"topologyProvider"`
	} `json:"acceptance"`
}

func TestServerStartsAsyncAPICaseBatchRunForWorkflow(t *testing.T) {
	ctx, s := openAPICaseBatchSQLiteStore(t)
	bundle := newAPICaseBatchWorkflowBundle(t)
	server := httptest.NewServer(controlplane.NewWithStore(bundle, s))
	defer server.Close()

	created := postAPICaseBatchWorkflowRun(t, server.URL)
	report := waitAPICaseBatchReport(t, server.URL+created.ReportURL)
	requireAPICaseBatchWorkflowReport(t, report)
	requireAPICaseBatchWorkflowStoreRecords(t, ctx, s, report)
}

func TestServerRejectsGenericBatchRunForEnvironmentVerificationWorkflow(t *testing.T) {
	ctx, s := openAPICaseBatchSQLiteStore(t)
	if _, err := s.UpsertEnvironment(ctx, store.Environment{
		ID:                     "env.workflow",
		DisplayName:            "Workflow Environment",
		Status:                 "draft",
		VerificationWorkflowID: "workflow.ten",
	}); err != nil {
		t.Fatalf("upsert environment: %v", err)
	}
	server := httptest.NewServer(controlplane.NewWithStore(newAPICaseBatchWorkflowBundle(t), s))
	defer server.Close()

	payload := postJSONResponse(t, server.URL+"/api/cases/batch-runs", `{"requestId":"workflow-env-001","workflowId":"workflow.ten"}`, http.StatusConflict)
	errorText, _ := payload["error"].(string)
	if !strings.Contains(errorText, "bound to environment env.workflow") || !strings.Contains(errorText, "/api/environments/env.workflow/acceptance-runs") {
		t.Fatalf("generic workflow gate error = %#v", payload)
	}
}

func TestServerRejectsAsyncAPICaseBatchWithoutNodes(t *testing.T) {
	server := httptest.NewServer(controlplane.NewWithStore(profile.Bundle{ID: "sample"}, nil))
	defer server.Close()

	var payload map[string]any
	postJSONInto(t, server.URL+"/api/cases/batch-runs", `{"requestId":"change-001"}`, http.StatusBadRequest, &payload)
}

func newAPICaseBatchWorkflowBundle(t *testing.T) profile.Bundle {
	t.Helper()

	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.HasPrefix(r.URL.Path, "/v1/workflow-steps/") {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))
	t.Cleanup(target.Close)

	dir := t.TempDir()
	bundle := profile.Bundle{
		ID: "sample",
		Workflows: []profile.Workflow{
			{ID: "workflow.ten", DisplayName: "Ten Step Workflow"},
		},
	}
	for i := 1; i <= 10; i++ {
		appendAPICaseBatchWorkflowStep(t, &bundle, dir, target.URL, i)
	}
	return bundle
}

func appendAPICaseBatchWorkflowStep(t *testing.T, bundle *profile.Bundle, dir string, targetURL string, order int) {
	t.Helper()

	stepID := fmt.Sprintf("step-%02d", order)
	nodeID := fmt.Sprintf("node.step.%02d", order)
	caseID := fmt.Sprintf("case.step.%02d", order)
	bundle.InterfaceNodes = append(bundle.InterfaceNodes, profile.InterfaceNode{ID: nodeID, DisplayName: nodeID})
	bundle.APICases = append(bundle.APICases, profile.APICase{
		ID:          caseID,
		DisplayName: caseID,
		NodeID:      nodeID,
		CasePath:    writeAPICaseBatchGETCase(t, dir, caseID, fmt.Sprintf("/v1/workflow-steps/%02d", order)),
		BaseURL:     targetURL,
		EvidenceDir: filepath.Join(dir, "evidence"),
	})
	bundle.WorkflowBindings = append(bundle.WorkflowBindings, profile.WorkflowBinding{
		WorkflowID: "workflow.ten",
		StepID:     stepID,
		NodeID:     nodeID,
		CaseID:     caseID,
		Required:   true,
		SortOrder:  order,
	})
}

func postAPICaseBatchWorkflowRun(t *testing.T, serverURL string) apiCaseBatchRunCreatedForTest {
	t.Helper()

	var created apiCaseBatchRunCreatedForTest
	postJSONInto(t, serverURL+"/api/cases/batch-runs", `{"requestId":"workflow-001","workflowId":"workflow.ten"}`, http.StatusAccepted, &created)
	if created.WorkflowID != "workflow.ten" || created.Total != 10 {
		t.Fatalf("workflow batch response = %#v", created)
	}
	return created
}

func requireAPICaseBatchWorkflowReport(t *testing.T, report apiCaseBatchReportForTest) {
	t.Helper()

	if !report.OK || report.Status != store.StatusPassed || report.WorkflowID != "workflow.ten" || report.Completed != 10 || report.Passed != 10 || len(report.Cases) != 10 {
		t.Fatalf("workflow batch report = %#v", report)
	}
	if report.Acceptance.TemplateID != "environment.workflow.skywalking.v1" || report.Acceptance.WorkflowID != "workflow.ten" || report.Acceptance.OK || report.Acceptance.ExpectedSteps != 10 || report.Acceptance.CompletedSteps != 10 || report.Acceptance.PassedSteps != 10 || report.Acceptance.TopologyProvider != "skywalking" {
		t.Fatalf("workflow acceptance report should require SkyWalking topology: %#v", report.Acceptance)
	}
	if len(report.Acceptance.Steps) != 10 || !report.Acceptance.Steps[0].EvidenceComplete || report.Acceptance.Steps[0].TopologyComplete {
		t.Fatalf("workflow acceptance steps = %#v", report.Acceptance.Steps)
	}
	if report.Cases[0].StepID != "step-01" || report.Cases[9].StepID != "step-10" {
		t.Fatalf("workflow step order = %#v", report.Cases)
	}
}

func requireAPICaseBatchWorkflowStoreRecords(t *testing.T, ctx context.Context, s *sqlite.Store, report apiCaseBatchReportForTest) {
	t.Helper()

	runs, err := s.ListRuns(ctx)
	if err != nil {
		t.Fatalf("list runs: %v", err)
	}
	if len(runs) != 11 {
		t.Fatalf("stored runs = %#v", runs)
	}
	batchRun, err := s.GetRun(ctx, report.BatchRunID)
	if err != nil {
		t.Fatalf("get stored workflow batch run: %v", err)
	}
	if batchRun.Status != store.StatusPassed || batchRun.WorkflowID != "workflow.ten" {
		t.Fatalf("stored workflow batch run = %#v", batchRun)
	}
	requireAPICaseBatchWorkflowSummary(t, batchRun)
}

func requireAPICaseBatchWorkflowSummary(t *testing.T, batchRun store.Run) {
	t.Helper()

	var storedSummary apiCaseBatchWorkflowSummaryForTest
	if err := json.Unmarshal([]byte(batchRun.SummaryJSON), &storedSummary); err != nil {
		t.Fatalf("decode stored workflow batch summary: %v", err)
	}
	if storedSummary.Summary.ExpectedStepCount != 10 || storedSummary.Summary.StepCount != 10 || storedSummary.Summary.Passed != 10 || storedSummary.Summary.Failed != 0 || len(storedSummary.Steps) != 10 {
		t.Fatalf("stored workflow run summary counts = %#v", storedSummary)
	}
	if storedSummary.Steps[0].StepID != "step-01" || storedSummary.Steps[0].CaseID == "" || storedSummary.Steps[0].Status != store.StatusPassed {
		t.Fatalf("stored workflow run steps = %#v", storedSummary.Steps)
	}
	if storedSummary.Acceptance.OK || storedSummary.Acceptance.TemplateID != "environment.workflow.skywalking.v1" || storedSummary.Acceptance.TopologyProvider != "skywalking" {
		t.Fatalf("stored workflow acceptance summary = %#v", storedSummary.Acceptance)
	}
}
