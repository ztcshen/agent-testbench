package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"agent-testbench/internal/store"
)

type workflowReportCommandReport struct {
	OK        bool   `json:"ok"`
	RunID     string `json:"runId"`
	ReportURL string `json:"reportUrl"`
	Counts    struct {
		Total  int `json:"total"`
		Passed int `json:"passed"`
		Failed int `json:"failed"`
	} `json:"counts"`
	Steps []struct {
		RunID     string `json:"runId"`
		CaseRunID string `json:"caseRunId"`
		DetailURL string `json:"detailUrl"`
		Status    string `json:"status"`
		Error     string `json:"error"`
	} `json:"steps"`
}

func TestWorkflowReportWritesReportWhenStepFails(t *testing.T) {
	storeRef := configureNamedPostgreSQLActiveStore(t, "daily-workflow-report-fail-pg")
	runWorkflowReportWritesReportWhenStepFails(t, storeRef, "PostgreSQL")
}

func TestWorkflowReportUsesNamedMySQLActiveStoreWhenStepFails(t *testing.T) {
	storeRef := configureNamedMySQLActiveStore(t, "daily-workflow-report-fail-mysql")
	runWorkflowReportWritesReportWhenStepFails(t, storeRef, "MySQL")
}

func runWorkflowReportWritesReportWhenStepFails(t *testing.T, storeRef string, label string) {
	t.Helper()
	serverURL := newFailingWorkflowReportServer(t)
	fixture := writeUniqueWorkflowBatchReportProfile(t)
	runCLI(t, "config", "publish", "--from", fixture.profileDir, "--store", storeRef)
	workflowID := discoverWorkflowReportID(t, label, storeRef, fixture.workflowID)

	outputDir := filepath.Join(t.TempDir(), "workflow-report")
	report := runWorkflowReportJSON(t, label, storeRef, workflowID, serverURL, outputDir)
	requireFailedWorkflowReport(t, label, report)
	requireWorkflowReportHTML(t, label, report, fixture, outputDir)
}

func TestWorkflowReportFailsAdmissionWhenStepInputIsMissing(t *testing.T) {
	storeRef := configureNamedSQLiteActiveStore(t, "workflow-report-missing-input")
	fixture := writeUniqueWorkflowBatchReportProfile(t)
	runCLI(t, "config", "publish", "--from", fixture.profileDir, "--store", storeRef)
	workflowID := discoverWorkflowReportID(t, "missing input", storeRef, fixture.workflowID)

	var secondCalled int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/first":
			w.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprint(w, `{}`)
		case "/second":
			atomic.StoreInt32(&secondCalled, 1)
			w.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprint(w, `{"status":"unexpected"}`)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	outputDir := filepath.Join(t.TempDir(), "workflow-report")
	report := runWorkflowReportJSON(t, "missing input", storeRef, workflowID, server.URL, outputDir)
	if report.OK || report.Counts.Passed != 1 || report.Counts.Failed != 1 || len(report.Steps) != 2 {
		t.Fatalf("missing-input workflow report = %#v", report)
	}
	if atomic.LoadInt32(&secondCalled) != 0 {
		t.Fatalf("workflow report should fail admission before calling the second step")
	}
	if report.Steps[1].Status != store.StatusFailed ||
		!strings.Contains(report.Steps[1].Error, "missing workflow input") ||
		!strings.Contains(report.Steps[1].Error, "item_id") {
		t.Fatalf("missing-input step report = %#v", report.Steps[1])
	}
}

func newFailingWorkflowReportServer(t *testing.T) string {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/first":
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, `{"item_id":"item-001"}`)
		case "/second":
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = fmt.Fprint(w, `{"status":"failed"}`)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(server.Close)
	return server.URL
}

func discoverWorkflowReportID(t *testing.T, label string, storeRef string, workflowID string) string {
	t.Helper()

	listOut := runCLI(t, "workflow", "discover", "--store", storeRef, "--filter", workflowID, "--json")
	var listReport struct {
		Items []struct {
			ID          string `json:"id"`
			DisplayName string `json:"displayName"`
		} `json:"items"`
	}
	if err := json.Unmarshal([]byte(listOut), &listReport); err != nil {
		t.Fatalf("decode %s workflow discover json: %v\n%s", label, err, listOut)
	}
	if len(listReport.Items) != 1 || listReport.Items[0].ID != workflowID {
		t.Fatalf("%s workflow discover = %#v", label, listReport.Items)
	}
	return listReport.Items[0].ID
}

func runWorkflowReportJSON(t *testing.T, label string, storeRef string, workflowID string, serverURL string, outputDir string) workflowReportCommandReport {
	t.Helper()

	out := runCLI(t,
		"workflow", "report",
		"--store", storeRef,
		"--workflow", workflowID,
		"--base-url", serverURL,
		"--output-dir", outputDir,
		"--json",
	)

	var report workflowReportCommandReport
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatalf("decode %s workflow report json: %v\n%s", label, err, out)
	}
	return report
}

func requireFailedWorkflowReport(t *testing.T, label string, report workflowReportCommandReport) {
	t.Helper()

	if report.OK || report.RunID == "" || report.Counts.Total != 2 || report.Counts.Passed != 1 || report.Counts.Failed != 1 {
		t.Fatalf("%s workflow report = %#v", label, report)
	}
	if len(report.Steps) != 2 || report.Steps[1].RunID == "" || report.Steps[1].CaseRunID != report.Steps[1].RunID+".case" || report.Steps[1].DetailURL == "" {
		t.Fatalf("%s workflow report evidence handles = %#v", label, report.Steps)
	}
}

func requireWorkflowReportHTML(t *testing.T, label string, report workflowReportCommandReport, fixture workflowBatchReportFixture, outputDir string) {
	t.Helper()

	htmlPath := filepath.Join(outputDir, "report.html")
	html, err := os.ReadFile(htmlPath)
	if err != nil {
		t.Fatalf("%s html report missing: %v", label, err)
	}
	for _, want := range []string{fixture.workflowName, "First Step", "Second Step", "failed", "caseRunId"} {
		if !strings.Contains(string(html), want) {
			t.Fatalf("%s workflow html missing %q:\n%s", label, want, html)
		}
	}
	if report.ReportURL != htmlPath {
		t.Fatalf("%s report url = %q want %q", label, report.ReportURL, htmlPath)
	}
}
