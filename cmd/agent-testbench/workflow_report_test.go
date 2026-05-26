package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWorkflowReportWritesReportWhenStepFails(t *testing.T) {
	storeRef := configureNamedPostgreSQLActiveStore(t, "daily-workflow-report-fail-pg")
	runWorkflowReportWritesReportWhenStepFails(t, storeRef, "PostgreSQL")
}

func TestWorkflowReportUsesNamedMySQLActiveStoreWhenStepFails(t *testing.T) {
	storeRef := configureNamedMySQLActiveStore(t, "daily-workflow-report-fail-mysql")
	runWorkflowReportWritesReportWhenStepFails(t, storeRef, "MySQL")
}

func runWorkflowReportWritesReportWhenStepFails(t *testing.T, _ string, label string) {
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
	defer server.Close()
	fixture := writeUniqueWorkflowBatchReportProfile(t)
	runCLI(t, "config", "publish", "--from", fixture.profileDir)
	listOut := runCLI(t, "workflow", "discover", "--filter", fixture.workflowID, "--json")
	var listReport struct {
		Items []struct {
			ID          string `json:"id"`
			DisplayName string `json:"displayName"`
		} `json:"items"`
	}
	if err := json.Unmarshal([]byte(listOut), &listReport); err != nil {
		t.Fatalf("decode %s workflow discover json: %v\n%s", label, err, listOut)
	}
	if len(listReport.Items) != 1 || listReport.Items[0].ID != fixture.workflowID {
		t.Fatalf("%s workflow discover = %#v", label, listReport.Items)
	}

	outputDir := filepath.Join(t.TempDir(), "workflow-report")
	out := runCLI(t,
		"workflow", "report",
		"--workflow", listReport.Items[0].ID,
		"--base-url", server.URL,
		"--output-dir", outputDir,
		"--json",
	)

	var report struct {
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
		} `json:"steps"`
	}
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatalf("decode %s workflow report json: %v\n%s", label, err, out)
	}
	if report.OK || report.RunID == "" || report.Counts.Total != 2 || report.Counts.Passed != 1 || report.Counts.Failed != 1 {
		t.Fatalf("%s workflow report = %#v", label, report)
	}
	if len(report.Steps) != 2 || report.Steps[1].RunID == "" || report.Steps[1].CaseRunID != report.Steps[1].RunID+".case" || report.Steps[1].DetailURL == "" {
		t.Fatalf("%s workflow report evidence handles = %#v", label, report.Steps)
	}
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
