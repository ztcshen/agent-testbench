package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestWorkflowAcceptanceCLIStartsAndReadsAsyncReport(t *testing.T) {
	var startPayload map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/cases/batch-runs":
			if err := json.NewDecoder(r.Body).Decode(&startPayload); err != nil {
				t.Fatalf("decode start payload: %v", err)
			}
			writeTestJSON(t, w, http.StatusAccepted, map[string]any{
				"ok":         true,
				"batchRunId": "batch.acceptance.001",
				"requestId":  "acceptance-001",
				"workflowId": "workflow.core-10",
				"status":     "running",
				"total":      10,
				"reportUrl":  "/api/cases/batch-runs/batch.acceptance.001",
			})
		case r.Method == http.MethodGet && r.URL.Path == "/api/cases/batch-runs/batch.acceptance.001":
			writeTestJSON(t, w, http.StatusOK, map[string]any{
				"ok":         true,
				"batchRunId": "batch.acceptance.001",
				"workflowId": "workflow.core-10",
				"status":     "passed",
				"total":      10,
				"acceptance": map[string]any{
					"ok":               true,
					"templateId":       "environment.workflow.skywalking.v1",
					"workflowId":       "workflow.core-10",
					"expectedSteps":    10,
					"completedSteps":   10,
					"passedSteps":      10,
					"failedSteps":      0,
					"topologyProvider": "skywalking",
				},
			})
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	startOut := runCLI(t, "workflow", "acceptance", "start",
		"--server-url", server.URL,
		"--workflow", "workflow.core-10",
		"--request-id", "acceptance-001",
		"--base-url", "http://127.0.0.1:18080",
		"--timeout-seconds", "30",
		"--json",
	)
	var started struct {
		OK         bool   `json:"ok"`
		BatchRunID string `json:"batchRunId"`
		WorkflowID string `json:"workflowId"`
		Status     string `json:"status"`
	}
	if err := json.Unmarshal([]byte(startOut), &started); err != nil {
		t.Fatalf("decode workflow acceptance start: %v\n%s", err, startOut)
	}
	if !started.OK || started.BatchRunID != "batch.acceptance.001" || started.WorkflowID != "workflow.core-10" || started.Status != "running" {
		t.Fatalf("workflow acceptance start = %#v", started)
	}
	if startPayload["workflowId"] != "workflow.core-10" || startPayload["requestId"] != "acceptance-001" || startPayload["baseUrl"] != "http://127.0.0.1:18080" || startPayload["timeoutSeconds"] != float64(30) {
		t.Fatalf("workflow acceptance start payload = %#v", startPayload)
	}

	reportOut := runCLI(t, "workflow", "acceptance", "report",
		"--server-url", server.URL,
		"--run", "batch.acceptance.001",
		"--json",
	)
	var report struct {
		Acceptance struct {
			OK               bool   `json:"ok"`
			TemplateID       string `json:"templateId"`
			TopologyProvider string `json:"topologyProvider"`
		} `json:"acceptance"`
	}
	if err := json.Unmarshal([]byte(reportOut), &report); err != nil {
		t.Fatalf("decode workflow acceptance report: %v\n%s", err, reportOut)
	}
	if !report.Acceptance.OK || report.Acceptance.TemplateID != "environment.workflow.skywalking.v1" || report.Acceptance.TopologyProvider != "skywalking" {
		t.Fatalf("workflow acceptance report = %#v", report.Acceptance)
	}
}

func TestCaseBatchCLIStartsAndReadsAsyncReport(t *testing.T) {
	var startPayload map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/cases/batch-runs":
			if err := json.NewDecoder(r.Body).Decode(&startPayload); err != nil {
				t.Fatalf("decode start payload: %v", err)
			}
			writeTestJSON(t, w, http.StatusAccepted, map[string]any{
				"ok":         true,
				"batchRunId": "batch.case.001",
				"requestId":  "case-batch-001",
				"status":     "running",
				"total":      2,
				"reportUrl":  "/api/cases/batch-runs/batch.case.001",
			})
		case r.Method == http.MethodGet && r.URL.Path == "/api/cases/batch-runs/batch.case.001":
			writeTestJSON(t, w, http.StatusOK, map[string]any{
				"ok":         true,
				"batchRunId": "batch.case.001",
				"status":     "passed",
				"total":      2,
				"passed":     2,
				"failed":     0,
			})
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	startOut := runCLI(t, "case", "batch", "start",
		"--server-url", server.URL,
		"--case", "case.alpha",
		"--case", "case.beta",
		"--request-id", "case-batch-001",
		"--base-url", "http://127.0.0.1:18080",
		"--timeout-seconds", "30",
		"--json",
	)
	var started struct {
		OK         bool   `json:"ok"`
		BatchRunID string `json:"batchRunId"`
		Status     string `json:"status"`
		Total      int    `json:"total"`
	}
	if err := json.Unmarshal([]byte(startOut), &started); err != nil {
		t.Fatalf("decode case batch start: %v\n%s", err, startOut)
	}
	if !started.OK || started.BatchRunID != "batch.case.001" || started.Status != "running" || started.Total != 2 {
		t.Fatalf("case batch start = %#v", started)
	}
	caseIDs, _ := startPayload["caseIds"].([]any)
	if len(caseIDs) != 2 || caseIDs[0] != "case.alpha" || caseIDs[1] != "case.beta" || startPayload["requestId"] != "case-batch-001" || startPayload["baseUrl"] != "http://127.0.0.1:18080" || startPayload["timeoutSeconds"] != float64(30) {
		t.Fatalf("case batch start payload = %#v", startPayload)
	}

	reportOut := runCLI(t, "case", "batch", "report",
		"--server-url", server.URL,
		"--run", "batch.case.001",
		"--json",
	)
	var report struct {
		OK     bool   `json:"ok"`
		Status string `json:"status"`
		Total  int    `json:"total"`
		Passed int    `json:"passed"`
		Failed int    `json:"failed"`
	}
	if err := json.Unmarshal([]byte(reportOut), &report); err != nil {
		t.Fatalf("decode case batch report: %v\n%s", err, reportOut)
	}
	if !report.OK || report.Status != "passed" || report.Total != 2 || report.Passed != 2 || report.Failed != 0 {
		t.Fatalf("case batch report = %#v", report)
	}
}

func TestEnvironmentAcceptanceCLIStartsAndReadsAsyncReport(t *testing.T) {
	var startPayload map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/environments/env.team/acceptance-runs":
			if err := json.NewDecoder(r.Body).Decode(&startPayload); err != nil {
				t.Fatalf("decode environment start payload: %v", err)
			}
			writeTestJSON(t, w, http.StatusAccepted, map[string]any{
				"ok":            true,
				"environmentId": "env.team",
				"batchRunId":    "batch.env.acceptance.001",
				"requestId":     "env-acceptance-001",
				"workflowId":    "workflow.core-10",
				"status":        "running",
				"reportUrl":     "/api/environments/env.team/acceptance-runs/batch.env.acceptance.001",
			})
		case r.Method == http.MethodGet && r.URL.Path == "/api/environments/env.team/acceptance-runs/batch.env.acceptance.001":
			writeTestJSON(t, w, http.StatusOK, map[string]any{
				"ok":            true,
				"environmentId": "env.team",
				"batchRunId":    "batch.env.acceptance.001",
				"workflowId":    "workflow.core-10",
				"status":        "passed",
				"acceptance": map[string]any{
					"ok":               true,
					"templateId":       "environment.workflow.skywalking.v1",
					"workflowId":       "workflow.core-10",
					"topologyProvider": "skywalking",
					"healthSummary":    map[string]any{"total": 1, "passed": 1, "failed": 0},
				},
			})
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	startOut := runCLI(t, "environment", "acceptance", "start",
		"--server-url", server.URL,
		"--request-id", "env-acceptance-001",
		"--base-url", "http://127.0.0.1:18080",
		"--json",
		"env.team",
	)
	var started struct {
		OK            bool   `json:"ok"`
		EnvironmentID string `json:"environmentId"`
		BatchRunID    string `json:"batchRunId"`
		WorkflowID    string `json:"workflowId"`
	}
	if err := json.Unmarshal([]byte(startOut), &started); err != nil {
		t.Fatalf("decode environment acceptance start: %v\n%s", err, startOut)
	}
	if !started.OK || started.EnvironmentID != "env.team" || started.BatchRunID != "batch.env.acceptance.001" || started.WorkflowID != "workflow.core-10" {
		t.Fatalf("environment acceptance start = %#v", started)
	}
	if startPayload["requestId"] != "env-acceptance-001" || startPayload["baseUrl"] != "http://127.0.0.1:18080" {
		t.Fatalf("environment acceptance start payload = %#v", startPayload)
	}

	reportOut := runCLI(t, "environment", "acceptance", "report",
		"--server-url", server.URL,
		"--run", "batch.env.acceptance.001",
		"--json",
		"env.team",
	)
	var report struct {
		Acceptance struct {
			OK            bool `json:"ok"`
			HealthSummary struct {
				Total  int `json:"total"`
				Passed int `json:"passed"`
			} `json:"healthSummary"`
		} `json:"acceptance"`
	}
	if err := json.Unmarshal([]byte(reportOut), &report); err != nil {
		t.Fatalf("decode environment acceptance report: %v\n%s", err, reportOut)
	}
	if !report.Acceptance.OK || report.Acceptance.HealthSummary.Total != 1 || report.Acceptance.HealthSummary.Passed != 1 {
		t.Fatalf("environment acceptance report = %#v", report.Acceptance)
	}
}
