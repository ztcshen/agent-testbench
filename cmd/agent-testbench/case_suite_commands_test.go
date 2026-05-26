package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"agent-testbench/internal/store"
)

func TestCaseSuiteReportRunsCasesByMaintenanceFilters(t *testing.T) {
	storeRef := configureNamedPostgreSQLActiveStore(t, "daily-case-suite-report-pg")
	runCaseSuiteReportRunsCasesByMaintenanceFilters(t, storeRef, "PostgreSQL")
}

func TestCaseSuiteReportUsesNamedMySQLActiveStore(t *testing.T) {
	storeRef := configureNamedMySQLActiveStore(t, "daily-case-suite-report-mysql")
	runCaseSuiteReportRunsCasesByMaintenanceFilters(t, storeRef, "MySQL")
}

func runCaseSuiteReportRunsCasesByMaintenanceFilters(t *testing.T, _ string, label string) {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Query().Get("mode") {
		case "bad":
			w.WriteHeader(http.StatusBadRequest)
			_, _ = fmt.Fprint(w, `{"status":"rejected"}`)
		default:
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, `{"status":"accepted"}`)
		}
	}))
	defer server.Close()
	fixture := writeUniqueInterfaceNodeBatchReportProfile(t)
	runCLI(t, "config", "publish", "--from", fixture.profileDir)

	outputDir := filepath.Join(t.TempDir(), "suite-report")
	out := runCLI(t,
		"case", "suite", "report",
		"--tag", "smoke",
		"--owner", "team-a",
		"--base-url", server.URL,
		"--output-dir", outputDir,
		"--json",
	)

	var report struct {
		OK             bool   `json:"ok"`
		JUnitReportURL string `json:"junitReportUrl"`
		Filters        struct {
			Tags  []string `json:"tags"`
			Owner string   `json:"owner"`
		} `json:"filters"`
		Counts struct {
			Total  int `json:"total"`
			Passed int `json:"passed"`
			Failed int `json:"failed"`
		} `json:"counts"`
		Results []struct {
			CaseID    string   `json:"caseId"`
			Title     string   `json:"title"`
			NodeID    string   `json:"nodeId"`
			Tags      []string `json:"tags"`
			Priority  string   `json:"priority"`
			Owner     string   `json:"owner"`
			Status    string   `json:"status"`
			CaseRunID string   `json:"caseRunId"`
			DetailURL string   `json:"detailUrl"`
		} `json:"results"`
	}
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatalf("decode %s suite report json: %v\n%s", label, err, out)
	}
	if !report.OK || report.Counts.Total != 1 || report.Counts.Passed != 1 || report.Counts.Failed != 0 {
		t.Fatalf("%s suite report = %#v", label, report)
	}
	if strings.Join(report.Filters.Tags, ",") != "smoke" || report.Filters.Owner != "team-a" {
		t.Fatalf("%s suite filters = %#v", label, report.Filters)
	}
	if len(report.Results) != 1 {
		t.Fatalf("%s suite results = %#v", label, report.Results)
	}
	item := report.Results[0]
	if item.CaseID != fixture.defaultCaseID || item.NodeID != fixture.nodeAlphaID || item.Priority != "p0" || item.Owner != "team-a" || item.CaseRunID == "" || item.DetailURL == "" {
		t.Fatalf("%s suite result item = %#v", label, item)
	}
	if strings.Join(item.Tags, ",") != "smoke,regression" {
		t.Fatalf("%s suite result tags = %#v", label, item.Tags)
	}
	html, err := os.ReadFile(filepath.Join(outputDir, "report.html"))
	if err != nil {
		t.Fatalf("%s suite html report missing: %v", label, err)
	}
	for _, want := range []string{"Case Suite Report", "Case Alpha Default", "team-a", "smoke", "p0", "caseRunId"} {
		if !strings.Contains(string(html), want) {
			t.Fatalf("%s suite html missing %q:\n%s", label, want, html)
		}
	}
	if strings.Contains(string(html), "Case Alpha Variant") {
		t.Fatalf("%s suite html should not include unselected case:\n%s", label, html)
	}
	junitPath := filepath.Join(outputDir, "report.junit.xml")
	junitRaw, err := os.ReadFile(junitPath)
	if err != nil {
		t.Fatalf("%s suite junit report missing: %v", label, err)
	}
	if report.JUnitReportURL != junitPath {
		t.Fatalf("%s junit report url = %q want %q", label, report.JUnitReportURL, junitPath)
	}
	for _, want := range []string{`<testsuite name="Case Suite Report" tests="1" failures="0"`, `name="` + fixture.defaultCaseID + `"`, `classname="` + fixture.nodeAlphaID + `"`} {
		if !strings.Contains(string(junitRaw), want) {
			t.Fatalf("%s suite junit missing %q:\n%s", label, want, junitRaw)
		}
	}

	variantOut := runCLI(t,
		"case", "suite", "report",
		"--tag", "negative",
		"--base-url", server.URL,
		"--output-dir", filepath.Join(t.TempDir(), "variant-suite-report"),
		"--json",
	)
	var variantReport struct {
		OK     bool `json:"ok"`
		Counts struct {
			Total          int `json:"total"`
			Passed         int `json:"passed"`
			DerivedConfigs int `json:"derivedConfigs"`
		} `json:"counts"`
		Results []struct {
			CaseID   string `json:"caseId"`
			Priority string `json:"priority"`
			Owner    string `json:"owner"`
			HTTPCode int    `json:"httpCode"`
		} `json:"results"`
	}
	if err := json.Unmarshal([]byte(variantOut), &variantReport); err != nil {
		t.Fatalf("decode %s variant suite report json: %v\n%s", label, err, variantOut)
	}
	if !variantReport.OK || variantReport.Counts.Total != 1 || variantReport.Counts.Passed != 1 || variantReport.Counts.DerivedConfigs != 1 {
		t.Fatalf("%s variant suite report = %#v", label, variantReport)
	}
	if len(variantReport.Results) != 1 || variantReport.Results[0].CaseID != fixture.variantCaseID || variantReport.Results[0].HTTPCode != http.StatusBadRequest {
		t.Fatalf("%s variant suite result = %#v", label, variantReport.Results)
	}
}

func TestCaseSuiteCommandsUseNamedPostgreSQLActiveStore(t *testing.T) {
	configureNamedPostgreSQLActiveStore(t, "daily-suite-pg")
	runCaseSuiteCommandsUseNamedActiveStore(t, "pg", "PostgreSQL")
}

func TestCaseSuiteCommandsUseNamedMySQLActiveStore(t *testing.T) {
	configureNamedMySQLActiveStore(t, "daily-suite-mysql")
	runCaseSuiteCommandsUseNamedActiveStore(t, "mysql", "MySQL")
}

func runCaseSuiteCommandsUseNamedActiveStore(t *testing.T, runLabel string, label string) {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Query().Get("mode") {
		case "bad":
			w.WriteHeader(http.StatusBadRequest)
			_, _ = fmt.Fprint(w, `{"status":"rejected"}`)
		default:
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, `{"status":"accepted"}`)
		}
	}))
	defer server.Close()
	profileDir := writeInterfaceNodeBatchReportProfile(t)
	runCLI(t, "config", "publish", "--from", profileDir)

	reportOut := runCLI(t,
		"case", "suite", "report",
		"--tag", "smoke",
		"--owner", "team-a",
		"--base-url", server.URL,
		"--output-dir", filepath.Join(t.TempDir(), runLabel+"-suite-report"),
		"--json",
	)
	var suiteReport struct {
		OK     bool `json:"ok"`
		Counts struct {
			Total  int `json:"total"`
			Passed int `json:"passed"`
			Failed int `json:"failed"`
		} `json:"counts"`
		Results []struct {
			CaseID    string `json:"caseId"`
			CaseRunID string `json:"caseRunId"`
			DetailURL string `json:"detailUrl"`
		} `json:"results"`
	}
	if err := json.Unmarshal([]byte(reportOut), &suiteReport); err != nil {
		t.Fatalf("decode %s suite report json: %v\n%s", label, err, reportOut)
	}
	if !suiteReport.OK || suiteReport.Counts.Total != 1 || suiteReport.Counts.Passed != 1 || suiteReport.Counts.Failed != 0 || len(suiteReport.Results) != 1 {
		t.Fatalf("%s suite report = %#v", label, suiteReport)
	}
	if suiteReport.Results[0].CaseID != "case.alpha.default" || suiteReport.Results[0].CaseRunID == "" || suiteReport.Results[0].DetailURL == "" {
		t.Fatalf("%s suite report result = %#v", label, suiteReport.Results[0])
	}

	variantOut := runCLI(t,
		"case", "suite", "report",
		"--tag", "negative",
		"--base-url", server.URL,
		"--output-dir", filepath.Join(t.TempDir(), runLabel+"-variant-suite-report"),
		"--json",
	)
	var variantReport struct {
		OK     bool `json:"ok"`
		Counts struct {
			Total          int `json:"total"`
			Passed         int `json:"passed"`
			DerivedConfigs int `json:"derivedConfigs"`
		} `json:"counts"`
		Results []struct {
			CaseID   string `json:"caseId"`
			HTTPCode int    `json:"httpCode"`
		} `json:"results"`
	}
	if err := json.Unmarshal([]byte(variantOut), &variantReport); err != nil {
		t.Fatalf("decode %s variant suite report json: %v\n%s", label, err, variantOut)
	}
	if !variantReport.OK || variantReport.Counts.Total != 1 || variantReport.Counts.Passed != 1 || variantReport.Counts.DerivedConfigs != 1 {
		t.Fatalf("%s variant suite report = %#v", label, variantReport)
	}
	if len(variantReport.Results) != 1 || variantReport.Results[0].CaseID != "case.alpha.variant" || variantReport.Results[0].HTTPCode != http.StatusBadRequest {
		t.Fatalf("%s variant suite result = %#v", label, variantReport.Results)
	}

	coverageOut := runCLI(t, "case", "suite", "coverage", "--status", "active", "--json")
	var coverage struct {
		OK     bool `json:"ok"`
		Counts struct {
			Total  int `json:"total"`
			Passed int `json:"passed"`
			Failed int `json:"failed"`
			NotRun int `json:"notRun"`
		} `json:"counts"`
	}
	if err := json.Unmarshal([]byte(coverageOut), &coverage); err != nil {
		t.Fatalf("decode %s suite coverage json: %v\n%s", label, err, coverageOut)
	}
	if !coverage.OK || coverage.Counts.Total != 2 || coverage.Counts.Passed != 2 || coverage.Counts.Failed != 0 || coverage.Counts.NotRun != 0 {
		t.Fatalf("%s suite coverage = %#v", label, coverage)
	}

	priorityOut := runCLI(t,
		"case", "suite", "priority",
		"--signal", "Alpha",
		"--limit", "2",
		"--request-id", runLabel+"-change-001",
		"--base-url", server.URL,
		"--json",
	)
	var priority struct {
		OK      bool     `json:"ok"`
		CaseIDs []string `json:"caseIds"`
		Counts  struct {
			Selected int `json:"selected"`
			Blocked  int `json:"blocked"`
		} `json:"counts"`
		BatchRequest struct {
			RequestID string   `json:"requestId"`
			CaseIDs   []string `json:"caseIds"`
			BaseURL   string   `json:"baseUrl"`
		} `json:"batchRequest"`
	}
	if err := json.Unmarshal([]byte(priorityOut), &priority); err != nil {
		t.Fatalf("decode %s suite priority json: %v\n%s", label, err, priorityOut)
	}
	if !priority.OK || priority.Counts.Selected != 2 || priority.Counts.Blocked != 0 || priority.BatchRequest.RequestID != runLabel+"-change-001" || priority.BatchRequest.BaseURL != server.URL {
		t.Fatalf("%s suite priority = %#v", label, priority)
	}
	if strings.Join(priority.BatchRequest.CaseIDs, ",") != strings.Join(priority.CaseIDs, ",") || len(priority.CaseIDs) != 2 {
		t.Fatalf("%s suite priority case ids = %#v batch=%#v", label, priority.CaseIDs, priority.BatchRequest.CaseIDs)
	}

	briefOut := runCLI(t, "case", "suite", "brief", "--signal", "Alpha", "--limit", "2", "--base-url", server.URL, "--json")
	var brief struct {
		OK     bool `json:"ok"`
		Counts struct {
			Ready            int `json:"ready"`
			Blocked          int `json:"blocked"`
			PrioritySelected int `json:"prioritySelected"`
		} `json:"counts"`
		Recommended []struct {
			CaseID string `json:"caseId"`
		} `json:"recommended"`
	}
	if err := json.Unmarshal([]byte(briefOut), &brief); err != nil {
		t.Fatalf("decode %s suite brief json: %v\n%s", label, err, briefOut)
	}
	if !brief.OK || brief.Counts.Ready != 2 || brief.Counts.Blocked != 0 || brief.Counts.PrioritySelected != 2 || len(brief.Recommended) != 2 {
		t.Fatalf("%s suite brief = %#v", label, brief)
	}
}

func TestCaseSuiteCoverageReportsLatestRunStatusByMaintenanceFilters(t *testing.T) {
	storeRef := configureNamedPostgreSQLActiveStore(t, "daily-case-suite-coverage-pg")
	runCaseSuiteCoverageReportsLatestRunStatusByMaintenanceFilters(t, storeRef, "PostgreSQL")
}

func TestCaseSuiteCoverageUsesNamedMySQLActiveStore(t *testing.T) {
	storeRef := configureNamedMySQLActiveStore(t, "daily-case-suite-coverage-mysql")
	runCaseSuiteCoverageReportsLatestRunStatusByMaintenanceFilters(t, storeRef, "MySQL")
}

func runCaseSuiteCoverageReportsLatestRunStatusByMaintenanceFilters(t *testing.T, storeRef string, label string) {
	t.Helper()
	ctx := context.Background()
	fixture := writeUniqueCaseSuiteCoverageProfile(t)
	runCLI(t, "config", "publish", "--from", fixture.profileDir)

	s, err := openStore(ctx, storeRef)
	if err != nil {
		t.Fatalf("open %s store: %v", label, err)
	}
	base := time.Now().UTC()
	oldDefaultRunID := uniqueTestID(t, "run.default.old")
	latestDefaultRunID := uniqueTestID(t, "run.default.latest")
	latestVariantRunID := uniqueTestID(t, "run.variant.latest")
	recordCaseRunForCoverage(t, ctx, s, oldDefaultRunID, fixture.defaultCaseID, store.StatusFailed, base.Add(-2*time.Minute))
	recordCaseRunForCoverage(t, ctx, s, latestDefaultRunID, fixture.defaultCaseID, store.StatusPassed, base.Add(-time.Minute))
	recordCaseRunForCoverage(t, ctx, s, latestVariantRunID, fixture.variantCaseID, store.StatusFailed, base)
	if err := s.Close(); err != nil {
		t.Fatalf("close %s store: %v", label, err)
	}

	out := runCLI(t,
		"case", "suite", "coverage",
		"--profile", fixture.profileDir,
		"--tag", "regression",
		"--status", "active",
		"--json",
	)

	var report struct {
		OK     bool `json:"ok"`
		Counts struct {
			Total  int `json:"total"`
			Passed int `json:"passed"`
			Failed int `json:"failed"`
			NotRun int `json:"notRun"`
		} `json:"counts"`
		Items []struct {
			CaseID       string `json:"caseId"`
			Title        string `json:"title"`
			NodeID       string `json:"nodeId"`
			LatestStatus string `json:"latestStatus"`
			LatestRunID  string `json:"latestRunId"`
			CaseRunID    string `json:"caseRunId"`
			DetailURL    string `json:"detailUrl"`
			HasPassed    bool   `json:"hasPassed"`
			Reason       string `json:"reason"`
		} `json:"items"`
	}
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatalf("decode %s suite coverage json: %v\n%s", label, err, out)
	}
	if report.OK || report.Counts.Total != 3 || report.Counts.Passed != 1 || report.Counts.Failed != 1 || report.Counts.NotRun != 1 {
		t.Fatalf("%s suite coverage report = %#v", label, report)
	}
	byCase := map[string]struct {
		LatestStatus string
		LatestRunID  string
		CaseRunID    string
		DetailURL    string
		HasPassed    bool
		Reason       string
	}{}
	for _, item := range report.Items {
		byCase[item.CaseID] = struct {
			LatestStatus string
			LatestRunID  string
			CaseRunID    string
			DetailURL    string
			HasPassed    bool
			Reason       string
		}{item.LatestStatus, item.LatestRunID, item.CaseRunID, item.DetailURL, item.HasPassed, item.Reason}
	}
	if byCase[fixture.defaultCaseID].LatestStatus != store.StatusPassed || byCase[fixture.defaultCaseID].LatestRunID != latestDefaultRunID || !byCase[fixture.defaultCaseID].HasPassed {
		t.Fatalf("%s default coverage = %#v", label, byCase[fixture.defaultCaseID])
	}
	if byCase[fixture.variantCaseID].LatestStatus != store.StatusFailed || byCase[fixture.variantCaseID].CaseRunID != latestVariantRunID+".case" || byCase[fixture.variantCaseID].DetailURL == "" || byCase[fixture.variantCaseID].HasPassed {
		t.Fatalf("%s variant coverage = %#v", label, byCase[fixture.variantCaseID])
	}
	if byCase[fixture.unrunCaseID].LatestStatus != "not-run" || byCase[fixture.unrunCaseID].Reason != "no run recorded in Store" {
		t.Fatalf("%s unrun coverage = %#v", label, byCase[fixture.unrunCaseID])
	}

	textOut := runCLI(t, "case", "suite", "coverage", "--profile", fixture.profileDir, "--tag", "regression")
	for _, want := range []string{"Case Suite Coverage", "Total: 3 Passed: 1 Failed: 1 Not Run: 1", fixture.variantCaseID, latestVariantRunID + ".case"} {
		if !strings.Contains(textOut, want) {
			t.Fatalf("%s coverage text missing %q:\n%s", label, want, textOut)
		}
	}
}

type caseSuiteCoverageFixture struct {
	profileDir    string
	profileID     string
	nodeID        string
	defaultCaseID string
	variantCaseID string
	unrunCaseID   string
	configID      string
}

func writeUniqueCaseSuiteCoverageProfile(t *testing.T) caseSuiteCoverageFixture {
	t.Helper()
	fixture := caseSuiteCoverageFixture{
		profileDir:    t.TempDir(),
		profileID:     uniqueTestID(t, "profile.case-suite-coverage"),
		nodeID:        uniqueTestID(t, "node.case-suite-coverage"),
		defaultCaseID: uniqueTestID(t, "case.default"),
		variantCaseID: uniqueTestID(t, "case.variant"),
		unrunCaseID:   uniqueTestID(t, "case.unrun"),
		configID:      uniqueTestID(t, "config.case.variant"),
	}
	writeFile(t, filepath.Join(fixture.profileDir, "profile.json"), fmt.Sprintf(`{
  "id": %q,
  "displayName": "Sample Profile",
  "services": [{"id":"service.alpha","displayName":"Service Alpha"}],
  "workflows": [],
  "interfaceNodes": [{"id":%q,"displayName":"Node Alpha","serviceId":"service.alpha","operation":"Alpha","method":"GET","path":"/alpha"}],
  "apiCases": [
    {"id":%q,"displayName":"Default Case","nodeId":%q,"sortOrder":1,"tags":["regression","smoke"],"priority":"p0","owner":"team-a","description":"Default maintained case.","casePath":"cases/default.json"},
    {"id":%q,"displayName":"Variant Case","nodeId":%q,"sortOrder":2,"tags":["regression"],"priority":"p1","owner":"team-a","description":"Variant maintained case."},
    {"id":%q,"displayName":"Unrun Case","nodeId":%q,"sortOrder":3,"tags":["regression"],"priority":"p2","owner":"team-b","description":"Unrun maintained case."}
  ],
  "requestTemplates": [],
  "templateConfigs": [
    {
      "id": %q,
      "scopeType": "case",
      "scopeId": %q,
      "status": "active",
      "configJson": %q
    }
  ],
  "caseDependencies": [],
  "workflowBindings": [],
  "fixtures": []
}`, fixture.profileID, fixture.nodeID, fixture.defaultCaseID, fixture.nodeID, fixture.variantCaseID, fixture.nodeID, fixture.unrunCaseID, fixture.nodeID, fixture.configID, fixture.variantCaseID, fmt.Sprintf(`{"caseId":%q,"caseExecution":{"method":"GET","nodeId":%q,"path":"/alpha","expectedHttpCodes":[200]}}`, fixture.variantCaseID, fixture.nodeID)))
	return fixture
}

func TestCaseSuiteInspectReportsReadinessByMaintenanceFilters(t *testing.T) {
	storeRef := configureNamedPostgreSQLActiveStore(t, "daily-case-suite-inspect-pg")
	runCaseSuiteInspectReportsReadinessByMaintenanceFilters(t, storeRef, "PostgreSQL")
}

func TestCaseSuiteInspectUsesNamedMySQLActiveStore(t *testing.T) {
	storeRef := configureNamedMySQLActiveStore(t, "daily-case-suite-inspect-mysql")
	runCaseSuiteInspectReportsReadinessByMaintenanceFilters(t, storeRef, "MySQL")
}

func runCaseSuiteInspectReportsReadinessByMaintenanceFilters(t *testing.T, storeRef string, label string) {
	t.Helper()
	ctx := context.Background()
	fixture := writeUniqueCaseSuiteCoverageProfile(t)
	runCLI(t, "config", "publish", "--from", fixture.profileDir)

	s, err := openStore(ctx, storeRef)
	if err != nil {
		t.Fatalf("open %s store: %v", label, err)
	}
	base := time.Now().UTC()
	recordCaseRunForCoverage(t, ctx, s, uniqueTestID(t, "run.default.latest"), fixture.defaultCaseID, store.StatusPassed, base.Add(-time.Minute))
	recordCaseRunForCoverage(t, ctx, s, uniqueTestID(t, "run.variant.latest"), fixture.variantCaseID, store.StatusFailed, base)
	if err := s.Close(); err != nil {
		t.Fatalf("close %s store: %v", label, err)
	}

	out := runCLI(t,
		"case", "suite", "inspect",
		"--profile", fixture.profileDir,
		"--tag", "regression",
		"--status", "active",
		"--json",
	)

	var report struct {
		OK     bool `json:"ok"`
		Counts struct {
			Total   int `json:"total"`
			Ready   int `json:"ready"`
			Blocked int `json:"blocked"`
			Failed  int `json:"failed"`
			NotRun  int `json:"notRun"`
		} `json:"counts"`
		Items []struct {
			CaseID             string   `json:"caseId"`
			Ready              bool     `json:"ready"`
			HasRunnableFile    bool     `json:"hasRunnableFile"`
			HasExecutionConfig bool     `json:"hasExecutionConfig"`
			LatestStatus       string   `json:"latestStatus"`
			Issues             []string `json:"issues"`
			SuggestedAction    string   `json:"suggestedAction"`
		} `json:"items"`
	}
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatalf("decode %s suite inspection json: %v\n%s", label, err, out)
	}
	if report.OK || report.Counts.Total != 3 || report.Counts.Ready != 2 || report.Counts.Blocked != 1 || report.Counts.Failed != 1 || report.Counts.NotRun != 1 {
		t.Fatalf("%s suite inspection report = %#v", label, report)
	}
	byCase := map[string]struct {
		Ready              bool
		HasRunnableFile    bool
		HasExecutionConfig bool
		LatestStatus       string
		Issues             []string
		SuggestedAction    string
	}{}
	for _, item := range report.Items {
		byCase[item.CaseID] = struct {
			Ready              bool
			HasRunnableFile    bool
			HasExecutionConfig bool
			LatestStatus       string
			Issues             []string
			SuggestedAction    string
		}{item.Ready, item.HasRunnableFile, item.HasExecutionConfig, item.LatestStatus, item.Issues, item.SuggestedAction}
	}
	if !byCase[fixture.defaultCaseID].Ready || !byCase[fixture.defaultCaseID].HasRunnableFile || byCase[fixture.defaultCaseID].LatestStatus != store.StatusPassed {
		t.Fatalf("%s default inspection = %#v", label, byCase[fixture.defaultCaseID])
	}
	if !byCase[fixture.variantCaseID].Ready || !byCase[fixture.variantCaseID].HasExecutionConfig || byCase[fixture.variantCaseID].SuggestedAction != "rerun" {
		t.Fatalf("%s variant inspection = %#v", label, byCase[fixture.variantCaseID])
	}
	if byCase[fixture.unrunCaseID].Ready || byCase[fixture.unrunCaseID].SuggestedAction != "add-runnable-source" || len(byCase[fixture.unrunCaseID].Issues) == 0 {
		t.Fatalf("%s unrun inspection = %#v", label, byCase[fixture.unrunCaseID])
	}

	textOut := runCLI(t, "case", "suite", "inspect", "--profile", fixture.profileDir, "--tag", "regression")
	for _, want := range []string{"Case Suite Inspection", "Total: 3 Ready: 2 Blocked: 1", fixture.unrunCaseID, "add-runnable-source"} {
		if !strings.Contains(textOut, want) {
			t.Fatalf("%s inspection text missing %q:\n%s", label, want, textOut)
		}
	}
}

func TestCaseSuitePlanBuildsExecutableBatchRequest(t *testing.T) {
	storeRef := configureNamedPostgreSQLActiveStore(t, "daily-case-suite-plan-pg")
	runCaseSuitePlanBuildsExecutableBatchRequest(t, storeRef, "pg", "PostgreSQL")
}

func TestCaseSuitePlanUsesNamedMySQLActiveStore(t *testing.T) {
	storeRef := configureNamedMySQLActiveStore(t, "daily-case-suite-plan-mysql")
	runCaseSuitePlanBuildsExecutableBatchRequest(t, storeRef, "mysql", "MySQL")
}

func runCaseSuitePlanBuildsExecutableBatchRequest(t *testing.T, storeRef string, runLabel string, label string) {
	t.Helper()
	ctx := context.Background()
	fixture := writeUniqueCaseSuiteCoverageProfile(t)
	runCLI(t, "config", "publish", "--from", fixture.profileDir)

	s, err := openStore(ctx, storeRef)
	if err != nil {
		t.Fatalf("open %s store: %v", label, err)
	}
	base := time.Now().UTC()
	recordCaseRunForCoverage(t, ctx, s, uniqueTestID(t, "run.default.latest"), fixture.defaultCaseID, store.StatusPassed, base.Add(-time.Minute))
	recordCaseRunForCoverage(t, ctx, s, uniqueTestID(t, "run.variant.latest"), fixture.variantCaseID, store.StatusFailed, base)
	if err := s.Close(); err != nil {
		t.Fatalf("close %s store: %v", label, err)
	}

	out := runCLI(t,
		"case", "suite", "plan",
		"--profile", fixture.profileDir,
		"--tag", "regression",
		"--status", "active",
		"--action", "run",
		"--action", "rerun",
		"--request-id", runLabel+"-change-001",
		"--base-url", "http://127.0.0.1:8080",
		"--evidence-dir", ".runtime/evidence",
		"--timeout-seconds", "7",
		"--json",
	)

	var report struct {
		OK      bool     `json:"ok"`
		CaseIDs []string `json:"caseIds"`
		Counts  struct {
			Total    int `json:"total"`
			Ready    int `json:"ready"`
			Blocked  int `json:"blocked"`
			Selected int `json:"selected"`
			Skipped  int `json:"skipped"`
		} `json:"counts"`
		BatchRequest struct {
			RequestID      string   `json:"requestId"`
			CaseIDs        []string `json:"caseIds"`
			BaseURL        string   `json:"baseUrl"`
			EvidenceDir    string   `json:"evidenceDir"`
			TimeoutSeconds int      `json:"timeoutSeconds"`
		} `json:"batchRequest"`
	}
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatalf("decode %s suite plan json: %v\n%s", label, err, out)
	}
	if !report.OK || strings.Join(report.CaseIDs, ",") != fixture.variantCaseID || report.Counts.Total != 3 || report.Counts.Ready != 2 || report.Counts.Blocked != 1 || report.Counts.Selected != 1 || report.Counts.Skipped != 1 {
		t.Fatalf("%s suite plan report = %#v", label, report)
	}
	if report.BatchRequest.RequestID != runLabel+"-change-001" || strings.Join(report.BatchRequest.CaseIDs, ",") != fixture.variantCaseID || report.BatchRequest.BaseURL != "http://127.0.0.1:8080" || report.BatchRequest.EvidenceDir != ".runtime/evidence" || report.BatchRequest.TimeoutSeconds != 7 {
		t.Fatalf("%s batch request = %#v", label, report.BatchRequest)
	}

	textOut := runCLI(t, "case", "suite", "plan", "--profile", fixture.profileDir, "--tag", "regression", "--action", "rerun")
	for _, want := range []string{"Case Suite Plan", "Selected: 1", fixture.variantCaseID} {
		if !strings.Contains(textOut, want) {
			t.Fatalf("%s plan text missing %q:\n%s", label, want, textOut)
		}
	}
}

func TestCaseSuiteStabilityReportsTransitions(t *testing.T) {
	storeRef := configureNamedPostgreSQLActiveStore(t, "daily-case-suite-stability-pg")
	runCaseSuiteStabilityReportsTransitions(t, storeRef, "PostgreSQL")
}

func TestCaseSuiteStabilityUsesNamedMySQLActiveStore(t *testing.T) {
	storeRef := configureNamedMySQLActiveStore(t, "daily-case-suite-stability-mysql")
	runCaseSuiteStabilityReportsTransitions(t, storeRef, "MySQL")
}

func runCaseSuiteStabilityReportsTransitions(t *testing.T, storeRef string, label string) {
	t.Helper()
	ctx := context.Background()
	fixture := writeUniqueCaseSuiteCoverageProfile(t)
	runCLI(t, "config", "publish", "--from", fixture.profileDir)

	s, err := openStore(ctx, storeRef)
	if err != nil {
		t.Fatalf("open %s store: %v", label, err)
	}
	base := time.Now().UTC()
	variantRun1ID := uniqueTestID(t, "run.variant.1")
	variantRun2ID := uniqueTestID(t, "run.variant.2")
	variantRun3ID := uniqueTestID(t, "run.variant.3")
	recordCaseRunForCoverage(t, ctx, s, variantRun1ID, fixture.variantCaseID, store.StatusPassed, base.Add(-3*time.Minute))
	recordCaseRunForCoverage(t, ctx, s, variantRun2ID, fixture.variantCaseID, store.StatusFailed, base.Add(-2*time.Minute))
	recordCaseRunForCoverage(t, ctx, s, variantRun3ID, fixture.variantCaseID, store.StatusPassed, base.Add(-time.Minute))
	recordCaseRunForCoverage(t, ctx, s, uniqueTestID(t, "run.default.1"), fixture.defaultCaseID, store.StatusPassed, base)
	if err := s.Close(); err != nil {
		t.Fatalf("close %s store: %v", label, err)
	}

	out := runCLI(t,
		"case", "suite", "stability",
		"--profile", fixture.profileDir,
		"--tag", "regression",
		"--status", "active",
		"--limit", "3",
		"--json",
	)
	var report struct {
		OK     bool `json:"ok"`
		Counts struct {
			Total    int `json:"total"`
			Stable   int `json:"stable"`
			Unstable int `json:"unstable"`
			NotRun   int `json:"notRun"`
		} `json:"counts"`
		Items []struct {
			CaseID       string `json:"caseId"`
			LatestStatus string `json:"latestStatus"`
			Transitions  int    `json:"transitions"`
			Unstable     bool   `json:"unstable"`
			Recent       []struct {
				RunID string `json:"runId"`
			} `json:"recent"`
		} `json:"items"`
	}
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatalf("decode %s suite stability json: %v\n%s", label, err, out)
	}
	if report.OK || report.Counts.Total != 3 || report.Counts.Unstable != 1 || report.Counts.Stable != 1 || report.Counts.NotRun != 1 {
		t.Fatalf("%s suite stability report = %#v", label, report)
	}
	byCase := map[string]struct {
		LatestStatus string
		Transitions  int
		Unstable     bool
		Recent       []struct {
			RunID string `json:"runId"`
		}
	}{}
	for _, item := range report.Items {
		byCase[item.CaseID] = struct {
			LatestStatus string
			Transitions  int
			Unstable     bool
			Recent       []struct {
				RunID string `json:"runId"`
			}
		}{item.LatestStatus, item.Transitions, item.Unstable, item.Recent}
	}
	if !byCase[fixture.variantCaseID].Unstable || byCase[fixture.variantCaseID].Transitions != 2 || byCase[fixture.variantCaseID].LatestStatus != store.StatusPassed || byCase[fixture.variantCaseID].Recent[0].RunID != variantRun3ID {
		t.Fatalf("%s variant stability = %#v", label, byCase[fixture.variantCaseID])
	}

	textOut := runCLI(t, "case", "suite", "stability", "--profile", fixture.profileDir, "--tag", "regression", "--limit", "3")
	for _, want := range []string{"Case Suite Stability", "Unstable: 1", fixture.variantCaseID} {
		if !strings.Contains(textOut, want) {
			t.Fatalf("%s stability text missing %q:\n%s", label, want, textOut)
		}
	}
}

func TestCaseSuitePriorityBuildsRankedBatchRequest(t *testing.T) {
	storeRef := configureNamedPostgreSQLActiveStore(t, "daily-case-suite-priority-pg")
	runCaseSuitePriorityBuildsRankedBatchRequest(t, storeRef, "pg", "PostgreSQL")
}

func TestCaseSuitePriorityUsesNamedMySQLActiveStore(t *testing.T) {
	storeRef := configureNamedMySQLActiveStore(t, "daily-case-suite-priority-mysql")
	runCaseSuitePriorityBuildsRankedBatchRequest(t, storeRef, "mysql", "MySQL")
}

func runCaseSuitePriorityBuildsRankedBatchRequest(t *testing.T, storeRef string, runLabel string, label string) {
	t.Helper()
	ctx := context.Background()
	fixture := writeUniqueCaseSuiteCoverageProfile(t)
	runCLI(t, "config", "publish", "--from", fixture.profileDir)

	s, err := openStore(ctx, storeRef)
	if err != nil {
		t.Fatalf("open %s store: %v", label, err)
	}
	base := time.Now().UTC()
	recordCaseRunForCoverage(t, ctx, s, uniqueTestID(t, "run.default.1"), fixture.defaultCaseID, store.StatusPassed, base.Add(-2*time.Minute))
	recordCaseRunForCoverage(t, ctx, s, uniqueTestID(t, "run.variant.1"), fixture.variantCaseID, store.StatusPassed, base.Add(-time.Minute))
	recordCaseRunForCoverage(t, ctx, s, uniqueTestID(t, "run.variant.2"), fixture.variantCaseID, store.StatusFailed, base)
	if err := s.Close(); err != nil {
		t.Fatalf("close %s store: %v", label, err)
	}

	out := runCLI(t,
		"case", "suite", "priority",
		"--profile", fixture.profileDir,
		"--tag", "regression",
		"--status", "active",
		"--signal", "Variant",
		"--limit", "2",
		"--request-id", runLabel+"-change-011",
		"--base-url", "http://127.0.0.1:8080",
		"--json",
	)
	var report struct {
		OK      bool     `json:"ok"`
		CaseIDs []string `json:"caseIds"`
		Counts  struct {
			Total    int `json:"total"`
			Selected int `json:"selected"`
			Skipped  int `json:"skipped"`
			Blocked  int `json:"blocked"`
		} `json:"counts"`
		Selected []struct {
			CaseID  string   `json:"caseId"`
			Score   int      `json:"score"`
			Reasons []string `json:"reasons"`
		} `json:"selected"`
		BatchRequest struct {
			RequestID string   `json:"requestId"`
			CaseIDs   []string `json:"caseIds"`
			BaseURL   string   `json:"baseUrl"`
		} `json:"batchRequest"`
	}
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatalf("decode %s suite priority json: %v\n%s", label, err, out)
	}
	if !report.OK || report.Counts.Total != 3 || report.Counts.Selected != 2 || report.Counts.Blocked != 1 || strings.Join(report.CaseIDs, ",") != fixture.variantCaseID+","+fixture.defaultCaseID {
		t.Fatalf("%s suite priority report = %#v", label, report)
	}
	if report.Selected[0].CaseID != fixture.variantCaseID || report.Selected[0].Score <= report.Selected[1].Score || len(report.Selected[0].Reasons) == 0 {
		t.Fatalf("%s suite priority selected = %#v", label, report.Selected)
	}
	if report.BatchRequest.RequestID != runLabel+"-change-011" || strings.Join(report.BatchRequest.CaseIDs, ",") != fixture.variantCaseID+","+fixture.defaultCaseID || report.BatchRequest.BaseURL != "http://127.0.0.1:8080" {
		t.Fatalf("%s suite priority batch = %#v", label, report.BatchRequest)
	}

	textOut := runCLI(t, "case", "suite", "priority", "--profile", fixture.profileDir, "--tag", "regression", "--signal", "Variant", "--limit", "1")
	for _, want := range []string{"Case Suite Priority", "Selected: 1", fixture.variantCaseID} {
		if !strings.Contains(textOut, want) {
			t.Fatalf("%s priority text missing %q:\n%s", label, want, textOut)
		}
	}
}

func TestCaseSuiteBriefSummarizesMaintainedSuiteForAgents(t *testing.T) {
	storeRef := configureNamedPostgreSQLActiveStore(t, "daily-case-suite-brief-pg")
	runCaseSuiteBriefSummarizesMaintainedSuiteForAgents(t, storeRef, "pg", "PostgreSQL")
}

func TestCaseSuiteBriefUsesNamedMySQLActiveStore(t *testing.T) {
	storeRef := configureNamedMySQLActiveStore(t, "daily-case-suite-brief-mysql")
	runCaseSuiteBriefSummarizesMaintainedSuiteForAgents(t, storeRef, "mysql", "MySQL")
}

func runCaseSuiteBriefSummarizesMaintainedSuiteForAgents(t *testing.T, storeRef string, runLabel string, label string) {
	t.Helper()
	ctx := context.Background()
	fixture := writeUniqueCaseSuiteCoverageProfile(t)
	runCLI(t, "config", "publish", "--from", fixture.profileDir)

	s, err := openStore(ctx, storeRef)
	if err != nil {
		t.Fatalf("open %s store: %v", label, err)
	}
	base := time.Now().UTC()
	recordCaseRunForCoverage(t, ctx, s, uniqueTestID(t, "run.default.1"), fixture.defaultCaseID, store.StatusPassed, base.Add(-2*time.Minute))
	recordCaseRunForCoverage(t, ctx, s, uniqueTestID(t, "run.variant.1"), fixture.variantCaseID, store.StatusPassed, base.Add(-time.Minute))
	recordCaseRunForCoverage(t, ctx, s, uniqueTestID(t, "run.variant.2"), fixture.variantCaseID, store.StatusFailed, base)
	if err := s.Close(); err != nil {
		t.Fatalf("close %s store: %v", label, err)
	}

	out := runCLI(t,
		"case", "suite", "brief",
		"--profile", fixture.profileDir,
		"--tag", "regression",
		"--status", "active",
		"--signal", "Variant",
		"--limit", "2",
		"--request-id", runLabel+"-change-012",
		"--base-url", "http://127.0.0.1:8080",
		"--json",
	)
	var report struct {
		OK     bool `json:"ok"`
		Counts struct {
			Total            int `json:"total"`
			Ready            int `json:"ready"`
			Blocked          int `json:"blocked"`
			Failed           int `json:"failed"`
			PrioritySelected int `json:"prioritySelected"`
		} `json:"counts"`
		Recommended []struct {
			CaseID string `json:"caseId"`
			Score  int    `json:"score"`
		} `json:"recommended"`
		BatchRequest struct {
			RequestID string   `json:"requestId"`
			CaseIDs   []string `json:"caseIds"`
			BaseURL   string   `json:"baseUrl"`
		} `json:"batchRequest"`
	}
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatalf("decode %s suite brief json: %v\n%s", label, err, out)
	}
	if !report.OK || report.Counts.Total != 3 || report.Counts.Ready != 2 || report.Counts.Blocked != 1 || report.Counts.Failed != 1 || report.Counts.PrioritySelected != 2 {
		t.Fatalf("%s suite brief report = %#v", label, report)
	}
	if len(report.Recommended) != 2 || report.Recommended[0].CaseID != fixture.variantCaseID || report.Recommended[0].Score <= report.Recommended[1].Score {
		t.Fatalf("%s suite brief recommended = %#v", label, report.Recommended)
	}
	if report.BatchRequest.RequestID != runLabel+"-change-012" || strings.Join(report.BatchRequest.CaseIDs, ",") != fixture.variantCaseID+","+fixture.defaultCaseID || report.BatchRequest.BaseURL != "http://127.0.0.1:8080" {
		t.Fatalf("%s suite brief batch = %#v", label, report.BatchRequest)
	}

	textOut := runCLI(t, "case", "suite", "brief", "--profile", fixture.profileDir, "--tag", "regression", "--signal", "Variant")
	for _, want := range []string{"Case Suite Brief", "Ready: 2", "Recommended: 2", fixture.variantCaseID} {
		if !strings.Contains(textOut, want) {
			t.Fatalf("%s brief text missing %q:\n%s", label, want, textOut)
		}
	}
}

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
	fixture := writeUniqueCaseSuiteQualityProfile(t)
	runCLI(t, "config", "publish", "--from", fixture.profileDir)

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
	fixture := writeUniqueCaseSuiteQualityProfile(t)
	runCLI(t, "config", "publish", "--from", fixture.profileDir)

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
	fixture := writeUniqueCaseSuiteQualityProfile(t)
	outputDir := filepath.Join(t.TempDir(), "quality-report")
	runCLI(t, "config", "publish", "--from", fixture.profileDir)

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
	ctx := context.Background()
	fixture := writeUniqueCaseSuiteCoverageProfile(t)
	runCLI(t, "config", "publish", "--from", fixture.profileDir)

	s, err := openStore(ctx, storeRef)
	if err != nil {
		t.Fatalf("open %s store: %v", label, err)
	}
	base := time.Now().UTC()
	recordCaseRunForCoverage(t, ctx, s, uniqueTestID(t, "run.default.latest"), fixture.defaultCaseID, store.StatusPassed, base.Add(-time.Minute))
	recordCaseRunForCoverage(t, ctx, s, uniqueTestID(t, "run.variant.latest"), fixture.variantCaseID, store.StatusFailed, base)
	if err := s.Close(); err != nil {
		t.Fatalf("close %s store: %v", label, err)
	}

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
