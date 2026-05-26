package main

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"agent-testbench/internal/store"
)

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
	fixture := publishUniqueCaseSuiteCoverageProfile(t)

	oldDefaultRunID := uniqueTestID(t, "run.default.old")
	latestDefaultRunID := uniqueTestID(t, "run.default.latest")
	latestVariantRunID := uniqueTestID(t, "run.variant.latest")
	recordCaseSuiteCoverageRuns(t, storeRef, label,
		caseSuiteCoverageRun{runID: oldDefaultRunID, caseID: fixture.defaultCaseID, status: store.StatusFailed, offset: -2 * time.Minute},
		caseSuiteCoverageRun{runID: latestDefaultRunID, caseID: fixture.defaultCaseID, status: store.StatusPassed, offset: -time.Minute},
		caseSuiteCoverageRun{runID: latestVariantRunID, caseID: fixture.variantCaseID, status: store.StatusFailed, offset: 0},
	)

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
	fixture := publishCaseSuiteReadinessHistory(t, storeRef, label)

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
	fixture := publishCaseSuiteReadinessHistory(t, storeRef, label)

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
