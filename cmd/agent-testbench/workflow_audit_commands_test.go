package main

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"agent-testbench/internal/store"
)

type workflowAuditRunHistory struct {
	firstRunID  string
	secondRunID string
}

type workflowAuditCommandReport struct {
	OK         bool   `json:"ok"`
	WorkflowID string `json:"workflowId"`
	IssueCount int    `json:"issueCount"`
	Issues     []struct {
		Code      string `json:"code"`
		SubjectID string `json:"subjectId"`
	} `json:"issues"`
	Store *struct {
		LatestRun *struct {
			ID     string `json:"id"`
			Status string `json:"status"`
		} `json:"latestRun"`
		BindingCases []struct {
			StepID       string `json:"stepId"`
			CaseID       string `json:"caseId"`
			HasPassed    bool   `json:"hasPassed"`
			LatestStatus string `json:"latestStatus"`
			LatestRunID  string `json:"latestRunId"`
		} `json:"bindingCases"`
	} `json:"store"`
}

type workflowAuditCaseState struct {
	HasPassed    bool
	LatestStatus string
	LatestRunID  string
}

func TestWorkflowAuditCommandEmitsJSONWithScopedStoreState(t *testing.T) {
	storeRef := configureNamedPostgreSQLActiveStore(t, "daily-workflow-audit-json-pg")
	runWorkflowAuditCommandEmitsJSONWithScopedStoreState(t, storeRef, "PostgreSQL")
}

func TestWorkflowAuditCommandUsesNamedMySQLActiveStore(t *testing.T) {
	storeRef := configureNamedMySQLActiveStore(t, "daily-workflow-audit-json-mysql")
	runWorkflowAuditCommandEmitsJSONWithScopedStoreState(t, storeRef, "MySQL")
}

func runWorkflowAuditCommandEmitsJSONWithScopedStoreState(t *testing.T, storeRef string, label string) {
	t.Helper()
	ctx := context.Background()
	fixture := writeWorkflowAuditProfile(t)
	runCLI(t, "config", "publish", "--from", fixture.profileDir)
	s, err := openStore(ctx, storeRef)
	if err != nil {
		t.Fatalf("open %s store: %v", label, err)
	}
	history := recordWorkflowAuditRunHistory(t, ctx, s, label, fixture)
	if err := s.Close(); err != nil {
		t.Fatalf("close %s store: %v", label, err)
	}

	report := decodeWorkflowAuditCommandReport(t, label, runCLI(t, "workflow", "audit", "--workflow", fixture.workflowID, "--json"))
	requireWorkflowAuditCommandSummary(t, label, fixture, history, report)
	requireWorkflowAuditCommandCaseState(t, label, fixture, history, report)
}

func recordWorkflowAuditRunHistory(t *testing.T, ctx context.Context, s store.Store, label string, fixture workflowAuditFixture) workflowAuditRunHistory {
	t.Helper()
	started := time.Now().UTC().Add(-10 * time.Second)
	history := workflowAuditRunHistory{
		firstRunID:  uniqueTestID(t, "run.workflow.001"),
		secondRunID: uniqueTestID(t, "run.workflow.002"),
	}
	recordWorkflowAuditRun(t, ctx, s, label, fixture, history.firstRunID, store.StatusFailed, started, 2*time.Second)
	laterStarted := started.Add(10 * time.Second)
	recordWorkflowAuditRun(t, ctx, s, label, fixture, history.secondRunID, store.StatusPassed, laterStarted, 3*time.Second)
	return history
}

func recordWorkflowAuditRun(t *testing.T, ctx context.Context, s store.Store, label string, fixture workflowAuditFixture, runID string, status string, started time.Time, duration time.Duration) {
	t.Helper()
	finished := started.Add(duration)
	if _, err := s.CreateRun(ctx, store.Run{
		ID:         runID,
		ProfileID:  fixture.profileID,
		WorkflowID: fixture.workflowID,
		Status:     status,
		StartedAt:  started,
		FinishedAt: finished,
		CreatedAt:  started,
		UpdatedAt:  finished,
	}); err != nil {
		t.Fatalf("create %s workflow run %s: %v", label, runID, err)
	}
	if _, err := s.RecordAPICaseRun(ctx, store.APICaseRun{
		ID:         runID + ".case.alpha",
		RunID:      runID,
		CaseID:     fixture.alphaCaseID,
		Status:     status,
		StartedAt:  started,
		FinishedAt: finished,
		CreatedAt:  started,
	}); err != nil {
		t.Fatalf("record %s case run %s: %v", label, runID, err)
	}
}

func decodeWorkflowAuditCommandReport(t *testing.T, label string, raw string) workflowAuditCommandReport {
	t.Helper()
	var report workflowAuditCommandReport
	if err := json.Unmarshal([]byte(raw), &report); err != nil {
		t.Fatalf("decode %s workflow audit json: %v\n%s", label, err, raw)
	}
	return report
}

func requireWorkflowAuditCommandSummary(t *testing.T, label string, fixture workflowAuditFixture, history workflowAuditRunHistory, report workflowAuditCommandReport) {
	t.Helper()
	if report.OK || report.WorkflowID != fixture.workflowID || report.IssueCount != 2 {
		t.Fatalf("%s workflow audit summary = %#v", label, report)
	}
	if len(report.Issues) != 2 || report.Issues[0].Code != "api-case-node-missing" || report.Issues[1].Code != "case-dependency-fixture-missing" {
		t.Fatalf("%s workflow audit issues = %#v", label, report.Issues)
	}
	if report.Store == nil || report.Store.LatestRun == nil || report.Store.LatestRun.ID != history.secondRunID || report.Store.LatestRun.Status != store.StatusPassed {
		t.Fatalf("%s latest workflow run = %#v", label, report.Store)
	}
}

func requireWorkflowAuditCommandCaseState(t *testing.T, label string, fixture workflowAuditFixture, history workflowAuditRunHistory, report workflowAuditCommandReport) {
	t.Helper()
	caseState := workflowAuditCommandCaseState(report)
	if !caseState[fixture.alphaCaseID].HasPassed || caseState[fixture.alphaCaseID].LatestStatus != store.StatusPassed || caseState[fixture.alphaCaseID].LatestRunID != history.secondRunID {
		t.Fatalf("%s alpha workflow state = %#v", label, caseState[fixture.alphaCaseID])
	}
	if caseState[fixture.betaCaseID].HasPassed || caseState[fixture.betaCaseID].LatestStatus != "" || caseState[fixture.betaCaseID].LatestRunID != "" {
		t.Fatalf("%s beta workflow state = %#v", label, caseState[fixture.betaCaseID])
	}
}

func workflowAuditCommandCaseState(report workflowAuditCommandReport) map[string]workflowAuditCaseState {
	caseState := map[string]workflowAuditCaseState{}
	if report.Store == nil {
		return caseState
	}
	for _, item := range report.Store.BindingCases {
		caseState[item.CaseID] = workflowAuditCaseState{
			HasPassed:    item.HasPassed,
			LatestStatus: item.LatestStatus,
			LatestRunID:  item.LatestRunID,
		}
	}
	return caseState
}

type workflowAuditFixture struct {
	profileDir       string
	profileID        string
	workflowID       string
	nodeID           string
	missingNodeID    string
	alphaCaseID      string
	betaCaseID       string
	templateID       string
	dependencyID     string
	missingFixtureID string
}

func writeWorkflowAuditProfile(t *testing.T) workflowAuditFixture {
	t.Helper()
	fixture := workflowAuditFixture{
		profileDir:       filepath.Join(t.TempDir(), "profile"),
		profileID:        uniqueTestID(t, "profile.workflow-audit"),
		workflowID:       uniqueTestID(t, "workflow.audit"),
		nodeID:           uniqueTestID(t, "node.audit"),
		missingNodeID:    uniqueTestID(t, "node.missing"),
		alphaCaseID:      uniqueTestID(t, "case.audit.alpha"),
		betaCaseID:       uniqueTestID(t, "case.audit.beta"),
		templateID:       uniqueTestID(t, "template.audit"),
		dependencyID:     uniqueTestID(t, "dependency.audit"),
		missingFixtureID: uniqueTestID(t, "fixture.missing"),
	}
	writeFile(t, filepath.Join(fixture.profileDir, "profile.json"), fmt.Sprintf(`{
  "id": %q,
  "displayName": "Sample Profile",
  "services": [],
  "workflows": [{"id":%q,"displayName":"Workflow Alpha"}],
  "interfaceNodes": [{"id":%q,"displayName":"Node Alpha"}],
  "apiCases": [
    {"id":%q,"displayName":"Case Alpha","nodeId":%q},
    {"id":%q,"displayName":"Case Beta","nodeId":%q}
  ],
  "requestTemplates": [{"id":%q,"nodeId":%q,"method":"POST","path":"/v1/items"}],
  "caseDependencies": [{"id":%q,"caseId":%q,"fixtureId":%q}],
	"workflowBindings": [
    {"workflowId":%q,"stepId":"step.one","nodeId":%q,"caseId":%q,"required":true},
    {"workflowId":%q,"stepId":"step.two","nodeId":%q,"caseId":%q,"required":true}
  ],
  "fixtures": []
}`, fixture.profileID, fixture.workflowID, fixture.nodeID, fixture.alphaCaseID, fixture.nodeID, fixture.betaCaseID, fixture.missingNodeID, fixture.templateID, fixture.nodeID, fixture.dependencyID, fixture.betaCaseID, fixture.missingFixtureID, fixture.workflowID, fixture.nodeID, fixture.alphaCaseID, fixture.workflowID, fixture.nodeID, fixture.betaCaseID))
	return fixture
}

func TestWorkflowAuditCommandPrintsTextSummary(t *testing.T) {
	configureNamedPostgreSQLActiveStore(t, "daily-workflow-audit-text-pg")
	runWorkflowAuditCommandPrintsTextSummary(t, "PostgreSQL")
}

func TestWorkflowAuditCommandPrintsTextSummaryWithMySQLStore(t *testing.T) {
	configureNamedMySQLActiveStore(t, "daily-workflow-audit-text-mysql")
	runWorkflowAuditCommandPrintsTextSummary(t, "MySQL")
}

func runWorkflowAuditCommandPrintsTextSummary(t *testing.T, label string) {
	t.Helper()
	fixture := writeWorkflowAuditTextProfile(t)
	runCLI(t, "config", "publish", "--from", fixture.profileDir)

	out := runCLI(t, "workflow", "audit", "--workflow", fixture.workflowID)

	for _, want := range []string{
		"Workflow Audit: " + fixture.workflowID,
		"OK: true",
		"Issues: 0",
		"Bindings: 1",
		"Binding: step.one Node: " + fixture.nodeID + " Case: " + fixture.alphaCaseID + " Required: true",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("%s workflow audit output missing %q: %q", label, want, out)
		}
	}
}

func writeWorkflowAuditTextProfile(t *testing.T) workflowAuditFixture {
	t.Helper()
	fixture := workflowAuditFixture{
		profileDir:  filepath.Join(t.TempDir(), "profile"),
		profileID:   uniqueTestID(t, "profile.workflow-audit-text"),
		workflowID:  uniqueTestID(t, "workflow.audit-text"),
		nodeID:      uniqueTestID(t, "node.audit-text"),
		alphaCaseID: uniqueTestID(t, "case.audit-text"),
	}
	writeFile(t, filepath.Join(fixture.profileDir, "profile.json"), fmt.Sprintf(`{
  "id": %q,
  "displayName": "Sample Profile",
  "services": [],
  "workflows": [{"id":%q,"displayName":"Workflow Alpha"}],
  "interfaceNodes": [{"id":%q,"displayName":"Node Alpha"}],
  "apiCases": [{"id":%q,"displayName":"Case Alpha","nodeId":%q}],
  "requestTemplates": [],
  "caseDependencies": [],
  "workflowBindings": [{"workflowId":%q,"stepId":"step.one","nodeId":%q,"caseId":%q,"required":true}],
  "fixtures": []
}`, fixture.profileID, fixture.workflowID, fixture.nodeID, fixture.alphaCaseID, fixture.nodeID, fixture.workflowID, fixture.nodeID, fixture.alphaCaseID))
	return fixture
}
