package main

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"agent-testbench/internal/store"
	"agent-testbench/internal/store/sqlite"
)

func TestCaseRunsCommandListsStoredCaseRuns(t *testing.T) {
	storeRef := configureNamedPostgreSQLActiveStore(t, "daily-case-runs-pg")
	runCaseRunsCommandListsStoredCaseRuns(t, storeRef, "PostgreSQL")
}

func TestCaseRunsCommandUsesNamedMySQLActiveStore(t *testing.T) {
	storeRef := configureNamedMySQLActiveStore(t, "daily-case-runs-mysql")
	runCaseRunsCommandListsStoredCaseRuns(t, storeRef, "MySQL")
}

func runCaseRunsCommandListsStoredCaseRuns(t *testing.T, storeRef string, label string) {
	t.Helper()
	ctx := context.Background()
	s, err := openStore(ctx, storeRef)
	if err != nil {
		t.Fatalf("open %s store: %v", label, err)
	}
	defer s.Close()
	runID := uniqueTestID(t, "run.case-runs")
	caseRunID := runID + ".case"
	caseID := uniqueTestID(t, "case.alpha")
	started := time.Date(2026, 5, 18, 10, 0, 0, 0, time.UTC)
	if _, err := s.CreateRun(ctx, store.Run{
		ID:           runID,
		ProfileID:    uniqueTestID(t, "profile.case-runs"),
		WorkflowID:   uniqueTestID(t, "workflow.case-runs"),
		Status:       store.StatusPassed,
		EvidenceRoot: "/tmp/evidence/" + runID,
		StartedAt:    started,
		FinishedAt:   started.Add(time.Second),
	}); err != nil {
		t.Fatalf("create %s run: %v", label, err)
	}
	if _, err := s.RecordAPICaseRun(ctx, store.APICaseRun{
		ID:                   caseRunID,
		RunID:                runID,
		CaseID:               caseID,
		Status:               store.StatusPassed,
		RequestSummaryJSON:   `{"method":"POST","path":"/alpha"}`,
		AssertionSummaryJSON: `{"status":"passed"}`,
		StartedAt:            started,
		FinishedAt:           started.Add(250 * time.Millisecond),
	}); err != nil {
		t.Fatalf("record %s case run: %v", label, err)
	}
	if _, err := s.RecordEvidence(ctx, store.EvidenceRecord{
		ID:        runID + ".evidence",
		RunID:     runID,
		CaseRunID: caseRunID,
		Kind:      "http-response",
		URI:       "/tmp/evidence/" + runID + "/response.json",
	}); err != nil {
		t.Fatalf("record %s evidence: %v", label, err)
	}

	out := runCLI(t, "case", "runs", "--run", runID, "--json")

	var report struct {
		OK       bool `json:"ok"`
		CaseRuns []struct {
			ID            string `json:"id"`
			RunID         string `json:"runId"`
			CaseID        string `json:"caseId"`
			Status        string `json:"status"`
			Operation     string `json:"operation"`
			EvidenceCount int    `json:"evidenceCount"`
			EvidencePath  string `json:"evidencePath"`
		} `json:"caseRuns"`
	}
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatalf("decode %s case runs json: %v\n%s", label, err, out)
	}
	if !report.OK || len(report.CaseRuns) != 1 {
		t.Fatalf("%s case runs report = %#v", label, report)
	}
	item := report.CaseRuns[0]
	if item.ID != caseRunID || item.RunID != runID || item.CaseID != caseID || item.Status != store.StatusPassed || item.Operation != "POST /alpha" || item.EvidenceCount != 1 || item.EvidencePath != "/tmp/evidence/"+runID {
		t.Fatalf("%s case run item = %#v", label, item)
	}
}

func TestCaseEvidenceCommandReadsCaseRunEvidence(t *testing.T) {
	storeRef := configureNamedPostgreSQLActiveStore(t, "daily-case-evidence-pg")
	runCaseEvidenceCommandReadsCaseRunEvidence(t, storeRef, "PostgreSQL")
}

func TestCaseEvidenceCommandUsesNamedMySQLActiveStore(t *testing.T) {
	storeRef := configureNamedMySQLActiveStore(t, "daily-case-evidence-mysql")
	runCaseEvidenceCommandReadsCaseRunEvidence(t, storeRef, "MySQL")
}

func runCaseEvidenceCommandReadsCaseRunEvidence(t *testing.T, storeRef string, label string) {
	t.Helper()
	ctx := context.Background()
	s, err := openStore(ctx, storeRef)
	if err != nil {
		t.Fatalf("open %s store: %v", label, err)
	}
	defer s.Close()
	runID := uniqueTestID(t, "run.case-evidence")
	caseRunID := runID + ".case"
	if _, err := s.CreateRun(ctx, store.Run{
		ID:           runID,
		ProfileID:    uniqueTestID(t, "profile.case-evidence"),
		WorkflowID:   uniqueTestID(t, "workflow.case-evidence"),
		Status:       store.StatusPassed,
		EvidenceRoot: "/tmp/evidence/" + runID,
		SummaryJSON:  "{}",
	}); err != nil {
		t.Fatalf("create %s run: %v", label, err)
	}
	if _, err := s.RecordAPICaseRun(ctx, store.APICaseRun{
		ID:                   caseRunID,
		RunID:                runID,
		CaseID:               uniqueTestID(t, "case.alpha"),
		Status:               store.StatusPassed,
		RequestSummaryJSON:   `{"method":"GET","path":"/alpha"}`,
		AssertionSummaryJSON: `{"status":"passed"}`,
	}); err != nil {
		t.Fatalf("record %s case run: %v", label, err)
	}
	if _, err := s.RecordEvidence(ctx, store.EvidenceRecord{
		ID:        runID + ".response",
		RunID:     runID,
		CaseRunID: caseRunID,
		Kind:      "response",
		URI:       "/tmp/evidence/" + runID + "/response.json",
		MediaType: "application/json",
		Summary:   `{"statusCode":200}`,
	}); err != nil {
		t.Fatalf("record %s evidence: %v", label, err)
	}

	out := runCLI(t, "case", "evidence", "--case-run", caseRunID, "--json")

	var payload struct {
		OK       bool `json:"ok"`
		Evidence struct {
			Summary  map[string]any `json:"summary"`
			Request  map[string]any `json:"request"`
			Response map[string]any `json:"response"`
		} `json:"evidence"`
	}
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		t.Fatalf("decode %s case evidence json: %v\n%s", label, err, out)
	}
	if !payload.OK || payload.Evidence.Summary["case_run_id"] != caseRunID || payload.Evidence.Summary["operation"] != "GET /alpha" {
		t.Fatalf("%s case evidence summary = %#v", label, payload.Evidence.Summary)
	}
	if payload.Evidence.Response["http_code"] != float64(200) || payload.Evidence.Response["evidence_uri"] != "/tmp/evidence/"+runID+"/response.json" {
		t.Fatalf("%s case evidence response = %#v", label, payload.Evidence.Response)
	}
}

func TestCaseTimingCommandSummarizesStoredCaseRuns(t *testing.T) {
	storeRef := configureNamedPostgreSQLActiveStore(t, "daily-case-timing-pg")
	runCaseTimingCommandSummarizesStoredCaseRuns(t, storeRef, "PostgreSQL")
}

func TestCaseTimingCommandUsesNamedMySQLActiveStore(t *testing.T) {
	storeRef := configureNamedMySQLActiveStore(t, "daily-case-timing-mysql")
	runCaseTimingCommandSummarizesStoredCaseRuns(t, storeRef, "MySQL")
}

func runCaseTimingCommandSummarizesStoredCaseRuns(t *testing.T, storeRef string, label string) {
	t.Helper()
	ctx := context.Background()
	s, err := openStore(ctx, storeRef)
	if err != nil {
		t.Fatalf("open %s store: %v", label, err)
	}
	defer s.Close()
	fastRunID := uniqueTestID(t, "run.fast")
	slowRunID := uniqueTestID(t, "run.slow")
	fastCaseID := uniqueTestID(t, "case.fast")
	slowCaseID := uniqueTestID(t, "case.slow")
	base := time.Now().UTC()
	for _, item := range []struct {
		runID    string
		caseID   string
		duration time.Duration
	}{
		{runID: fastRunID, caseID: fastCaseID, duration: 200 * time.Millisecond},
		{runID: slowRunID, caseID: slowCaseID, duration: 36 * time.Hour},
	} {
		started := base
		if _, err := s.CreateRun(ctx, store.Run{
			ID:         item.runID,
			ProfileID:  "sample",
			Status:     store.StatusPassed,
			StartedAt:  started,
			FinishedAt: started.Add(item.duration),
			CreatedAt:  started,
			UpdatedAt:  started.Add(item.duration),
		}); err != nil {
			t.Fatalf("create %s run %s: %v", label, item.runID, err)
		}
		if _, err := s.RecordAPICaseRun(ctx, store.APICaseRun{
			ID:         item.runID + ".case",
			RunID:      item.runID,
			CaseID:     item.caseID,
			Status:     store.StatusPassed,
			StartedAt:  started,
			FinishedAt: started.Add(item.duration),
			CreatedAt:  started,
		}); err != nil {
			t.Fatalf("record %s case run %s: %v", label, item.runID, err)
		}
	}

	out := runCLI(t, "case", "timing", "--kind", "case", "--max-age-minutes", "1", "--json")

	var payload struct {
		OK      bool `json:"ok"`
		Summary struct {
			CaseRunCount          int            `json:"caseRunCount"`
			DurationMeasuredCount int            `json:"durationMeasuredCount"`
			MaxDurationMs         int            `json:"maxDurationMs"`
			SlowestRows           map[string]any `json:"slowestRows"`
		} `json:"summary"`
	}
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		t.Fatalf("decode %s case timing json: %v\n%s", label, err, out)
	}
	if !payload.OK || payload.Summary.CaseRunCount < 2 || payload.Summary.DurationMeasuredCount < 2 || payload.Summary.MaxDurationMs < int((36*time.Hour).Milliseconds()) {
		t.Fatalf("%s case timing summary = %#v", label, payload.Summary)
	}
	slowest := payload.Summary.SlowestRows["caseRun"].(map[string]any)
	if slowest["id"] != slowRunID+".case" || slowest["caseId"] != slowCaseID || slowest["durationMs"] != float64((36*time.Hour).Milliseconds()) {
		t.Fatalf("%s case timing slowest = %#v", label, slowest)
	}
}

func TestCaseQueryCommandsAcceptStoreFlag(t *testing.T) {
	ctx := context.Background()
	storePath := filepath.Join(t.TempDir(), "store.sqlite")
	s, err := sqlite.Open(ctx, sqlite.Config{Path: storePath})
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	started := time.Date(2026, 5, 18, 10, 0, 0, 0, time.UTC)
	if _, err := s.CreateRun(ctx, store.Run{
		ID:           "run-store-flag",
		ProfileID:    "sample",
		Status:       store.StatusPassed,
		EvidenceRoot: "/tmp/evidence/run-store-flag",
		StartedAt:    started,
		FinishedAt:   started.Add(time.Second),
		CreatedAt:    started,
		UpdatedAt:    started.Add(time.Second),
	}); err != nil {
		t.Fatalf("create run: %v", err)
	}
	if _, err := s.RecordAPICaseRun(ctx, store.APICaseRun{
		ID:                   "case-run-store-flag",
		RunID:                "run-store-flag",
		CaseID:               "case.alpha",
		Status:               store.StatusPassed,
		RequestSummaryJSON:   `{"method":"GET","path":"/alpha"}`,
		AssertionSummaryJSON: `{"status":"passed"}`,
		StartedAt:            started,
		FinishedAt:           started.Add(500 * time.Millisecond),
		CreatedAt:            started,
	}); err != nil {
		t.Fatalf("record case run: %v", err)
	}
	if _, err := s.RecordEvidence(ctx, store.EvidenceRecord{
		ID:        "response-store-flag",
		RunID:     "run-store-flag",
		CaseRunID: "case-run-store-flag",
		Kind:      "response",
		URI:       "/tmp/evidence/run-store-flag/response.json",
		MediaType: "application/json",
		Summary:   `{"statusCode":200}`,
	}); err != nil {
		t.Fatalf("record evidence: %v", err)
	}
	if err := s.Close(); err != nil {
		t.Fatalf("close store: %v", err)
	}

	storeRef := "sqlite://" + storePath
	runsOut := runCLI(t, "case", "runs", "--store", storeRef, "--json")
	if !strings.Contains(runsOut, "case-run-store-flag") {
		t.Fatalf("case runs output = %q", runsOut)
	}
	evidenceOut := runCLI(t, "case", "evidence", "--store", storeRef, "--case-run", "case-run-store-flag", "--json")
	if !strings.Contains(evidenceOut, "case-run-store-flag") || !strings.Contains(evidenceOut, "/alpha") {
		t.Fatalf("case evidence output = %q", evidenceOut)
	}
	timingOut := runCLI(t, "case", "timing", "--store", storeRef, "--kind", "case", "--json")
	if !strings.Contains(timingOut, `"maxDurationMs": 500`) {
		t.Fatalf("case timing output = %q", timingOut)
	}
}

func TestCaseReadCommandsUseNamedSQLiteActiveStore(t *testing.T) {
	configureNamedSQLiteActiveStore(t, "daily-case-read-sqlite")
	runID := uniqueTestID(t, "case-run-sqlite")
	createStoredCaseRun(t, runID, "SQLite")

	if out := runCLI(t, "case", "runs", "--json"); !strings.Contains(out, runID) {
		t.Fatalf("SQLite case runs output = %q", out)
	}
	if out := runCLI(t, "case", "evidence", "--case-run", runID+".case", "--json"); !strings.Contains(out, runID) || !strings.Contains(out, "/v1/items") {
		t.Fatalf("SQLite case evidence output = %q", out)
	}
	if out := runCLI(t, "case", "timing", "--kind", "case", "--json"); !strings.Contains(out, `"caseRunCount": 1`) {
		t.Fatalf("SQLite case timing output = %q", out)
	}
}
