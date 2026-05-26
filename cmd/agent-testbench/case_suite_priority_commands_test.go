package main

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"agent-testbench/internal/store"
)

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
	fixture := publishUniqueCaseSuiteCoverageProfile(t)

	variantRun1ID := uniqueTestID(t, "run.variant.1")
	variantRun2ID := uniqueTestID(t, "run.variant.2")
	variantRun3ID := uniqueTestID(t, "run.variant.3")
	recordCaseSuiteCoverageRuns(t, storeRef, label,
		caseSuiteCoverageRun{runID: variantRun1ID, caseID: fixture.variantCaseID, status: store.StatusPassed, offset: -3 * time.Minute},
		caseSuiteCoverageRun{runID: variantRun2ID, caseID: fixture.variantCaseID, status: store.StatusFailed, offset: -2 * time.Minute},
		caseSuiteCoverageRun{runID: variantRun3ID, caseID: fixture.variantCaseID, status: store.StatusPassed, offset: -time.Minute},
		caseSuiteCoverageRun{runID: uniqueTestID(t, "run.default.1"), caseID: fixture.defaultCaseID, status: store.StatusPassed, offset: 0},
	)

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
	fixture := publishCaseSuitePriorityHistory(t, storeRef, label)

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
	fixture := publishCaseSuitePriorityHistory(t, storeRef, label)

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
