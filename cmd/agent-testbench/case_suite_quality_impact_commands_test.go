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

	"agent-testbench/internal/store"
)

func TestCaseSuiteQualityAuditsMaintainedCaseMetadata(t *testing.T) {
	storeRef := configureNamedPostgreSQLActiveStore(t, "daily-case-suite-quality-pg")
	runCaseSuiteQualityAuditsMaintainedCaseMetadata(t, storeRef, "PostgreSQL")
}

func TestCaseSuiteQualityUsesNamedMySQLActiveStore(t *testing.T) {
	storeRef := configureNamedMySQLActiveStore(t, "daily-case-suite-quality-mysql")
	runCaseSuiteQualityAuditsMaintainedCaseMetadata(t, storeRef, "MySQL")
}

func runCaseSuiteQualityAuditsMaintainedCaseMetadata(t *testing.T, _ string, label string) {
	t.Helper()
	fixture := publishUniqueCaseSuiteQualityProfile(t)

	out := runCLI(t,
		"case", "suite", "quality",
		"--profile", fixture.profileDir,
		"--status", "active",
		"--json",
	)
	var report struct {
		OK     bool `json:"ok"`
		Counts struct {
			Nodes             int `json:"nodes"`
			NodesWithoutCases int `json:"nodesWithoutCases"`
			Cases             int `json:"cases"`
			CompleteCases     int `json:"completeCases"`
			IncompleteCases   int `json:"incompleteCases"`
			MissingOwner      int `json:"missingOwner"`
			MissingRunnable   int `json:"missingRunnable"`
			MissingExecution  int `json:"missingExecution"`
		} `json:"counts"`
		Cases []struct {
			CaseID   string   `json:"caseId"`
			Complete bool     `json:"complete"`
			Issues   []string `json:"issues"`
		} `json:"cases"`
		Nodes []struct {
			NodeID string   `json:"nodeId"`
			Issues []string `json:"issues"`
		} `json:"nodes"`
	}
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatalf("decode %s suite quality json: %v\n%s", label, err, out)
	}
	if report.OK || report.Counts.Nodes != 2 || report.Counts.NodesWithoutCases != 1 || report.Counts.Cases != 2 || report.Counts.CompleteCases != 1 || report.Counts.IncompleteCases != 1 {
		t.Fatalf("%s suite quality report = %#v", label, report)
	}
	if report.Counts.MissingOwner != 1 || report.Counts.MissingRunnable != 1 || report.Counts.MissingExecution != 1 {
		t.Fatalf("%s suite quality gaps = %#v", label, report.Counts)
	}
	if len(report.Nodes) != 1 || report.Nodes[0].NodeID != fixture.nodeEmptyID {
		t.Fatalf("%s suite quality nodes = %#v", label, report.Nodes)
	}
	textOut := runCLI(t, "case", "suite", "quality", "--profile", fixture.profileDir, "--status", "active")
	for _, want := range []string{"Case Suite Quality", "Incomplete: 1", fixture.nodeEmptyID, fixture.gapsCaseID} {
		if !strings.Contains(textOut, want) {
			t.Fatalf("%s quality text missing %q:\n%s", label, want, textOut)
		}
	}
}

func TestCaseSuiteQualityPlanSuggestsAuthoringActions(t *testing.T) {
	storeRef := configureNamedPostgreSQLActiveStore(t, "daily-case-suite-quality-plan-pg")
	runCaseSuiteQualityPlanSuggestsAuthoringActions(t, storeRef, "PostgreSQL")
}

func TestCaseSuiteQualityPlanUsesNamedMySQLActiveStore(t *testing.T) {
	storeRef := configureNamedMySQLActiveStore(t, "daily-case-suite-quality-plan-mysql")
	runCaseSuiteQualityPlanSuggestsAuthoringActions(t, storeRef, "MySQL")
}

func runCaseSuiteQualityPlanSuggestsAuthoringActions(t *testing.T, _ string, label string) {
	t.Helper()
	fixture := publishUniqueCaseSuiteQualityProfile(t)

	out := runCLI(t,
		"case", "suite", "quality-plan",
		"--profile", fixture.profileDir,
		"--status", "active",
		"--json",
	)
	var report struct {
		OK     bool `json:"ok"`
		Counts struct {
			Total            int `json:"total"`
			DraftCase        int `json:"draftCase"`
			CompleteMetadata int `json:"completeMetadata"`
			AddRunnable      int `json:"addRunnable"`
			AddExecution     int `json:"addExecution"`
		} `json:"counts"`
		Actions []struct {
			Type            string   `json:"type"`
			NodeID          string   `json:"nodeId"`
			CaseID          string   `json:"caseId"`
			SuggestedCaseID string   `json:"suggestedCaseId"`
			Fields          []string `json:"fields"`
			Command         []string `json:"command"`
		} `json:"actions"`
	}
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatalf("decode %s suite quality plan json: %v\n%s", label, err, out)
	}
	if !report.OK || report.Counts.Total != 4 || report.Counts.DraftCase != 1 || report.Counts.CompleteMetadata != 1 || report.Counts.AddRunnable != 1 || report.Counts.AddExecution != 1 {
		t.Fatalf("%s suite quality plan report = %#v", label, report)
	}
	if len(report.Actions) != 4 || report.Actions[0].Type != "draft-case" || report.Actions[0].NodeID != fixture.nodeEmptyID || report.Actions[0].SuggestedCaseID != fixture.suggestedEmptyCaseID {
		t.Fatalf("%s suite quality plan actions = %#v", label, report.Actions)
	}
	textOut := runCLI(t, "case", "suite", "quality-plan", "--profile", fixture.profileDir, "--status", "active")
	for _, want := range []string{"Case Suite Quality Plan", "Draft Case: 1", fixture.suggestedEmptyCaseID, fixture.gapsCaseID} {
		if !strings.Contains(textOut, want) {
			t.Fatalf("%s quality plan text missing %q:\n%s", label, want, textOut)
		}
	}
}

func TestCaseSuiteQualityReportWritesJSONAndHTML(t *testing.T) {
	storeRef := configureNamedPostgreSQLActiveStore(t, "daily-case-suite-quality-report-pg")
	runCaseSuiteQualityReportWritesJSONAndHTML(t, storeRef, "PostgreSQL")
}

func TestCaseSuiteQualityReportUsesNamedMySQLActiveStore(t *testing.T) {
	storeRef := configureNamedMySQLActiveStore(t, "daily-case-suite-quality-report-mysql")
	runCaseSuiteQualityReportWritesJSONAndHTML(t, storeRef, "MySQL")
}

func runCaseSuiteQualityReportWritesJSONAndHTML(t *testing.T, _ string, label string) {
	t.Helper()
	fixture := publishUniqueCaseSuiteQualityProfile(t)
	outputDir := filepath.Join(t.TempDir(), "quality-report")

	out := runCLI(t,
		"case", "suite", "quality-report",
		"--profile", fixture.profileDir,
		"--status", "active",
		"--output-dir", outputDir,
		"--json",
	)
	var report struct {
		OK            bool   `json:"ok"`
		ProfileID     string `json:"profileId"`
		ReportURL     string `json:"reportUrl"`
		JSONReportURL string `json:"jsonReportUrl"`
		QualityPlan   struct {
			Counts struct {
				Total            int `json:"total"`
				DraftCase        int `json:"draftCase"`
				CompleteMetadata int `json:"completeMetadata"`
				AddRunnable      int `json:"addRunnable"`
				AddExecution     int `json:"addExecution"`
			} `json:"counts"`
			Actions []struct {
				Type            string `json:"type"`
				CaseID          string `json:"caseId"`
				SuggestedCaseID string `json:"suggestedCaseId"`
			} `json:"actions"`
		} `json:"qualityPlan"`
	}
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatalf("decode %s suite quality report json: %v\n%s", label, err, out)
	}
	if !report.OK || report.ProfileID != fixture.profileID || report.QualityPlan.Counts.Total != 4 || report.QualityPlan.Counts.DraftCase != 1 || report.QualityPlan.Counts.CompleteMetadata != 1 || report.QualityPlan.Counts.AddRunnable != 1 || report.QualityPlan.Counts.AddExecution != 1 {
		t.Fatalf("%s suite quality report = %#v", label, report)
	}
	if report.ReportURL != filepath.Join(outputDir, "report.html") || report.JSONReportURL != filepath.Join(outputDir, "report.json") {
		t.Fatalf("%s suite quality report paths = %#v", label, report)
	}
	jsonReportRaw, err := os.ReadFile(filepath.Join(outputDir, "report.json"))
	if err != nil {
		t.Fatalf("read %s quality json report: %v", label, err)
	}
	htmlReportRaw, err := os.ReadFile(filepath.Join(outputDir, "report.html"))
	if err != nil {
		t.Fatalf("read %s quality html report: %v", label, err)
	}
	jsonReport := string(jsonReportRaw)
	htmlReport := string(htmlReportRaw)
	for _, want := range []string{"Case Suite Quality Report", fixture.suggestedEmptyCaseID, fixture.gapsCaseID, "complete-case-metadata", "add-execution-config"} {
		if !strings.Contains(htmlReport, want) {
			t.Fatalf("%s quality html missing %q:\n%s", label, want, htmlReport)
		}
	}
	if !strings.Contains(jsonReport, `"qualityPlan"`) || !strings.Contains(jsonReport, fixture.suggestedEmptyCaseID) {
		t.Fatalf("%s quality json report missing expected content:\n%s", label, jsonReport)
	}

	textOut := runCLI(t, "case", "suite", "quality-report", "--profile", fixture.profileDir, "--status", "active", "--output-dir", filepath.Join(t.TempDir(), "text-quality-report"))
	for _, want := range []string{"Case Suite Quality Report", "Total Actions: 4", "Report:"} {
		if !strings.Contains(textOut, want) {
			t.Fatalf("%s quality report text missing %q:\n%s", label, want, textOut)
		}
	}
}

func TestCaseSuiteImpactBuildsExecutableBatchRequest(t *testing.T) {
	storeRef := configureNamedPostgreSQLActiveStore(t, "daily-case-suite-impact-pg")
	runCaseSuiteImpactBuildsExecutableBatchRequest(t, storeRef, "pg", "PostgreSQL")
}

func TestCaseSuiteImpactUsesNamedMySQLActiveStore(t *testing.T) {
	storeRef := configureNamedMySQLActiveStore(t, "daily-case-suite-impact-mysql")
	runCaseSuiteImpactBuildsExecutableBatchRequest(t, storeRef, "mysql", "MySQL")
}

func runCaseSuiteImpactBuildsExecutableBatchRequest(t *testing.T, storeRef string, runLabel string, label string) {
	t.Helper()
	fixture := publishCaseSuiteReadinessHistory(t, storeRef, label)

	out := runCLI(t,
		"case", "suite", "impact",
		"--profile", fixture.profileDir,
		"--signal", "/alpha",
		"--status", "active",
		"--action", "run",
		"--action", "rerun",
		"--request-id", runLabel+"-change-002",
		"--base-url", "http://127.0.0.1:8080",
		"--json",
	)

	var report struct {
		OK     bool `json:"ok"`
		Counts struct {
			Signals  int `json:"signals"`
			Nodes    int `json:"nodes"`
			Cases    int `json:"cases"`
			Selected int `json:"selected"`
			Blocked  int `json:"blocked"`
		} `json:"counts"`
		BatchRequest struct {
			RequestID string   `json:"requestId"`
			CaseIDs   []string `json:"caseIds"`
			BaseURL   string   `json:"baseUrl"`
		} `json:"batchRequest"`
		Cases []struct {
			CaseID  string   `json:"caseId"`
			Reasons []string `json:"reasons"`
		} `json:"cases"`
	}
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatalf("decode %s suite impact json: %v\n%s", label, err, out)
	}
	if !report.OK || report.Counts.Signals != 1 || report.Counts.Nodes != 1 || report.Counts.Cases != 3 || report.Counts.Selected != 1 || report.Counts.Blocked != 1 {
		t.Fatalf("%s suite impact report = %#v", label, report)
	}
	if report.BatchRequest.RequestID != runLabel+"-change-002" || strings.Join(report.BatchRequest.CaseIDs, ",") != fixture.variantCaseID || report.BatchRequest.BaseURL != "http://127.0.0.1:8080" {
		t.Fatalf("%s impact batch request = %#v", label, report.BatchRequest)
	}
	if len(report.Cases) != 3 || len(report.Cases[0].Reasons) == 0 {
		t.Fatalf("%s impact cases = %#v", label, report.Cases)
	}

	textOut := runCLI(t, "case", "suite", "impact", "--profile", fixture.profileDir, "--signal", "/alpha", "--action", "rerun")
	for _, want := range []string{"Case Suite Impact", "Selected: 1", fixture.variantCaseID} {
		if !strings.Contains(textOut, want) {
			t.Fatalf("%s impact text missing %q:\n%s", label, want, textOut)
		}
	}
}

func TestCaseSuiteImpactReportRunsImpactedCases(t *testing.T) {
	storeRef := configureNamedPostgreSQLActiveStore(t, "daily-case-suite-impact-report-pg")
	runCaseSuiteImpactReportRunsImpactedCases(t, storeRef, "pg", "PostgreSQL")
}

func TestCaseSuiteImpactReportUsesNamedMySQLActiveStore(t *testing.T) {
	storeRef := configureNamedMySQLActiveStore(t, "daily-case-suite-impact-report-mysql")
	runCaseSuiteImpactReportRunsImpactedCases(t, storeRef, "mysql", "MySQL")
}

func runCaseSuiteImpactReportRunsImpactedCases(t *testing.T, _ string, runLabel string, label string) {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/lookup" || r.URL.Query().Get("mode") != "ok" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprint(w, `{"status":"accepted"}`)
	}))
	defer server.Close()
	fixture := writeUniqueInterfaceNodeBatchReportProfile(t)
	runCLI(t, "config", "publish", "--from", fixture.profileDir)

	outputDir := filepath.Join(t.TempDir(), "impact-report")
	out := runCLI(t,
		"case", "suite", "impact-report",
		"--profile", fixture.profileDir,
		"--signal", "/lookup",
		"--tag", "smoke",
		"--status", "active",
		"--action", "run",
		"--request-id", runLabel+"-change-003",
		"--base-url", server.URL,
		"--output-dir", outputDir,
		"--json",
	)

	var report struct {
		OK     bool `json:"ok"`
		Impact struct {
			BatchRequest struct {
				RequestID string   `json:"requestId"`
				CaseIDs   []string `json:"caseIds"`
			} `json:"batchRequest"`
		} `json:"impact"`
		Report struct {
			OK        bool   `json:"ok"`
			ReportURL string `json:"reportUrl"`
			Counts    struct {
				Total  int `json:"total"`
				Passed int `json:"passed"`
				Failed int `json:"failed"`
			} `json:"counts"`
			Results []struct {
				CaseID    string `json:"caseId"`
				CaseRunID string `json:"caseRunId"`
				Status    string `json:"status"`
			} `json:"results"`
		} `json:"report"`
	}
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatalf("decode %s impact report json: %v\n%s", label, err, out)
	}
	if !report.OK || report.Impact.BatchRequest.RequestID != runLabel+"-change-003" || strings.Join(report.Impact.BatchRequest.CaseIDs, ",") != fixture.defaultCaseID {
		t.Fatalf("%s impact report selection = %#v", label, report)
	}
	if !report.Report.OK || report.Report.Counts.Total != 1 || report.Report.Counts.Passed != 1 || report.Report.Counts.Failed != 0 || len(report.Report.Results) != 1 {
		t.Fatalf("%s impact execution report = %#v", label, report.Report)
	}
	if report.Report.Results[0].CaseID != fixture.defaultCaseID || report.Report.Results[0].CaseRunID == "" || report.Report.Results[0].Status != store.StatusPassed {
		t.Fatalf("%s impact execution item = %#v", label, report.Report.Results[0])
	}
	if _, err := os.Stat(filepath.Join(outputDir, "report.html")); err != nil {
		t.Fatalf("%s impact report html missing: %v", label, err)
	}
}
