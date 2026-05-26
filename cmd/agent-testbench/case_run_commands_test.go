package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"agent-testbench/internal/store"
	"agent-testbench/internal/store/sqlite"
)

func TestCaseRunCommandWritesEvidence(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprint(w, `{"status":"created"}`)
	}))
	defer server.Close()
	dir := t.TempDir()
	casePath := filepath.Join(dir, "case.json")
	writeAPICaseFile(t, casePath)
	evidenceDir := filepath.Join(dir, "evidence")
	storePath := filepath.Join(dir, "store.sqlite")

	out := runCLI(t, "case", "run", "--case", casePath, "--base-url", server.URL, "--run-id", "case-run-001", "--evidence-dir", evidenceDir, "--store", "sqlite://"+storePath)
	if !strings.Contains(out, "Case Run: case-run-001") || !strings.Contains(out, "Status: passed") {
		t.Fatalf("case run output = %q", out)
	}
	if _, err := os.Stat(filepath.Join(evidenceDir, "case-run-001", "summary.json")); err != nil {
		t.Fatalf("summary evidence missing: %v", err)
	}
}

func TestCaseRunCommandRequiresActiveStoreBeforeFileExecution(t *testing.T) {
	called := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprint(w, `{"status":"created"}`)
	}))
	defer server.Close()
	dir := t.TempDir()
	casePath := filepath.Join(dir, "case.json")
	writeAPICaseFile(t, casePath)
	configHome := filepath.Join(dir, "config")

	out := runCLIFailsWithEnv(t, []string{"AGENT_TESTBENCH_CONFIG_HOME=" + configHome}, "case", "run", "--case", casePath, "--base-url", server.URL, "--run-id", "case-run-no-store")
	if !strings.Contains(out, errNoActiveStoreConfigured.Error()) {
		t.Fatalf("case run without store output = %q", out)
	}
	if called {
		t.Fatal("case run executed request before resolving active Store")
	}
}

func TestCaseRunDryRunPreviewsFileCaseWithoutStoreOrHTTP(t *testing.T) {
	called := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprint(w, `{"status":"created"}`)
	}))
	defer server.Close()

	dir := t.TempDir()
	casePath := filepath.Join(dir, "case.json")
	writeAPICaseFile(t, casePath)
	evidenceDir := filepath.Join(dir, "evidence")
	configHome := filepath.Join(dir, "config")

	out := runCLIWithEnv(t, []string{"AGENT_TESTBENCH_CONFIG_HOME=" + configHome},
		"case", "run",
		"--case", casePath,
		"--base-url", server.URL,
		"--run-id", "case-run-dry",
		"--evidence-dir", evidenceDir,
		"--override", "id=item-override",
		"--dry-run",
		"--json",
	)
	if called {
		t.Fatal("dry-run executed the HTTP request")
	}
	if _, err := os.Stat(filepath.Join(evidenceDir, "case-run-dry")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("dry-run should not write evidence, stat err = %v", err)
	}

	var plan struct {
		OK      bool   `json:"ok"`
		DryRun  bool   `json:"dryRun"`
		RunID   string `json:"runId"`
		CaseID  string `json:"caseId"`
		Request struct {
			Method   string   `json:"method"`
			Path     string   `json:"path"`
			URL      string   `json:"url"`
			BodyKeys []string `json:"bodyKeys"`
		} `json:"request"`
		Assertions struct {
			ExpectedStatusCodes []int `json:"expectedStatusCodes"`
		} `json:"assertions"`
		Effects struct {
			HTTPRequest         bool   `json:"httpRequest"`
			WritesEvidence      bool   `json:"writesEvidence"`
			WritesStore         bool   `json:"writesStore"`
			PlannedEvidencePath string `json:"plannedEvidencePath"`
		} `json:"effects"`
	}
	if err := json.Unmarshal([]byte(out), &plan); err != nil {
		t.Fatalf("decode dry-run json: %v\n%s", err, out)
	}
	if !plan.OK || !plan.DryRun || plan.RunID != "case-run-dry" || plan.CaseID != "case.alpha" {
		t.Fatalf("dry-run plan identity = %#v", plan)
	}
	if plan.Request.Method != "POST" || plan.Request.Path != "/v1/items" || plan.Request.URL != server.URL+"/v1/items" {
		t.Fatalf("dry-run request plan = %#v", plan.Request)
	}
	if len(plan.Request.BodyKeys) != 1 || plan.Request.BodyKeys[0] != "id" {
		t.Fatalf("dry-run body keys = %#v", plan.Request.BodyKeys)
	}
	if len(plan.Assertions.ExpectedStatusCodes) != 1 || plan.Assertions.ExpectedStatusCodes[0] != 200 {
		t.Fatalf("dry-run assertions = %#v", plan.Assertions)
	}
	if plan.Effects.HTTPRequest || plan.Effects.WritesEvidence || plan.Effects.WritesStore || plan.Effects.PlannedEvidencePath != filepath.Join(evidenceDir, "case-run-dry") {
		t.Fatalf("dry-run effects = %#v", plan.Effects)
	}
}

func TestCaseRunCommandUsesActiveSQLiteStore(t *testing.T) {
	called := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprint(w, `{"status":"created"}`)
	}))
	defer server.Close()
	dir := t.TempDir()
	casePath := filepath.Join(dir, "case.json")
	writeAPICaseFile(t, casePath)
	t.Setenv("AGENT_TESTBENCH_CONFIG_HOME", filepath.Join(dir, "config"))
	storePath := filepath.Join(dir, "store.sqlite")
	if err := saveStoreConfig(storeConfigFile{
		Active: "legacy-local",
		Stores: map[string]storeConfigEntry{
			"legacy-local": {Name: "legacy-local", URL: "sqlite://" + storePath, Backend: "sqlite"},
		},
	}); err != nil {
		t.Fatalf("save store config: %v", err)
	}

	out := runCLI(t, "case", "run", "--case", casePath, "--base-url", server.URL, "--run-id", "case-run-active-sqlite")
	if !strings.Contains(out, "case-run-active-sqlite") {
		t.Fatalf("case run with active SQLite store output = %q", out)
	}
	if !called {
		t.Fatal("case run did not execute request with active SQLite Store")
	}
}

func TestCaseRunCommandExecutesHTTPCase(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request map[string]any
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if request["id"] != "item-override" {
			t.Fatalf("request overrides = %#v", request)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprint(w, `{"status":"created"}`)
	}))
	defer server.Close()

	dir := t.TempDir()
	casePath := filepath.Join(dir, "case.json")
	writeAPICaseFile(t, casePath)
	evidenceDir := filepath.Join(dir, "evidence")
	storePath := filepath.Join(dir, "store.sqlite")

	out := runCLI(t, "case", "run", "--case", casePath, "--base-url", server.URL, "--run-id", "case-run-002", "--evidence-dir", evidenceDir, "--override", "id=item-override", "--store", "sqlite://"+storePath)
	if !strings.Contains(out, "Case Run: case-run-002") || !strings.Contains(out, "Status: passed") {
		t.Fatalf("case run output = %q", out)
	}
	if _, err := os.Stat(filepath.Join(evidenceDir, "case-run-002", "response.json")); err != nil {
		t.Fatalf("response evidence missing: %v", err)
	}
}

func TestCaseRunCommandIndexesStoreRecords(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprint(w, `{"status":"created"}`)
	}))
	defer server.Close()

	dir := t.TempDir()
	casePath := filepath.Join(dir, "case.json")
	writeAPICaseFile(t, casePath)
	storePath := filepath.Join(dir, "store.sqlite")
	evidenceDir := filepath.Join(dir, "evidence")

	runCLI(t, "case", "run", "--case", casePath, "--base-url", server.URL, "--run-id", "case-run-003", "--evidence-dir", evidenceDir, "--store", "sqlite://"+storePath, "--profile", "sample")

	s, err := sqlite.Open(context.Background(), sqlite.Config{Path: storePath})
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer s.Close()
	run, err := s.GetRun(context.Background(), "case-run-003")
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if run.ProfileID != "sample" || run.Status != "passed" {
		t.Fatalf("run = %#v", run)
	}
	if !run.FinishedAt.After(run.StartedAt) {
		t.Fatalf("run timing was not indexed: %#v", run)
	}
	var runSummary struct {
		RunID  string `json:"runId"`
		CaseID string `json:"caseId"`
		Status string `json:"status"`
	}
	if err := json.Unmarshal([]byte(run.SummaryJSON), &runSummary); err != nil {
		t.Fatalf("decode run summary: %v", err)
	}
	if runSummary.RunID != "case-run-003" || runSummary.CaseID != "case.alpha" || runSummary.Status != "passed" {
		t.Fatalf("run summary = %#v", runSummary)
	}
	caseRuns, err := s.ListAPICaseRuns(context.Background(), "case-run-003")
	if err != nil {
		t.Fatalf("list api case runs: %v", err)
	}
	if len(caseRuns) != 1 || caseRuns[0].CaseID != "case.alpha" {
		t.Fatalf("case runs = %#v", caseRuns)
	}
	if !caseRuns[0].FinishedAt.After(caseRuns[0].StartedAt) {
		t.Fatalf("case run timing was not indexed: %#v", caseRuns[0])
	}
	var requestSummary struct {
		Method  string `json:"method"`
		Path    string `json:"path"`
		HasBody bool   `json:"hasBody"`
	}
	if err := json.Unmarshal([]byte(caseRuns[0].RequestSummaryJSON), &requestSummary); err != nil {
		t.Fatalf("decode request summary: %v", err)
	}
	if requestSummary.Method != "POST" || requestSummary.Path != "/v1/items" || !requestSummary.HasBody {
		t.Fatalf("request summary = %#v", requestSummary)
	}
	var assertionSummary struct {
		Status     string `json:"status"`
		ErrorCount int    `json:"errorCount"`
	}
	if err := json.Unmarshal([]byte(caseRuns[0].AssertionSummaryJSON), &assertionSummary); err != nil {
		t.Fatalf("decode assertion summary: %v", err)
	}
	if assertionSummary.Status != "passed" || assertionSummary.ErrorCount != 0 {
		t.Fatalf("assertion summary = %#v", assertionSummary)
	}
	records, err := s.ListEvidence(context.Background(), "case-run-003")
	if err != nil {
		t.Fatalf("list evidence: %v", err)
	}
	if len(records) != 5 {
		t.Fatalf("evidence records = %#v", records)
	}
	var responseSummary string
	for _, record := range records {
		if record.Kind == "response" {
			responseSummary = record.Summary
		}
	}
	var response struct {
		StatusCode int `json:"statusCode"`
		BodyBytes  int `json:"bodyBytes"`
	}
	if err := json.Unmarshal([]byte(responseSummary), &response); err != nil {
		t.Fatalf("decode response evidence summary: %v", err)
	}
	if response.StatusCode != http.StatusOK || response.BodyBytes == 0 {
		t.Fatalf("response evidence summary = %#v", response)
	}
}

func TestCaseRunCommandIndexesStoreRecordsWithStoreFlag(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprint(w, `{"status":"created"}`)
	}))
	defer server.Close()

	dir := t.TempDir()
	casePath := filepath.Join(dir, "case.json")
	writeAPICaseFile(t, casePath)
	storePath := filepath.Join(dir, "store.sqlite")
	evidenceDir := filepath.Join(dir, "evidence")

	runCLI(t, "case", "run", "--case", casePath, "--base-url", server.URL, "--run-id", "case-run-store-flag", "--evidence-dir", evidenceDir, "--store", "sqlite://"+storePath, "--profile", "sample")

	s, err := sqlite.Open(context.Background(), sqlite.Config{Path: storePath})
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer s.Close()
	run, err := s.GetRun(context.Background(), "case-run-store-flag")
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if run.ProfileID != "sample" || run.Status != "passed" {
		t.Fatalf("run = %#v", run)
	}
}

func TestCaseDiagnoseCommandSummarizesFailedCaseRunEvidence(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = fmt.Fprint(w, `{"status":"rejected"}`)
	}))
	defer server.Close()

	dir := t.TempDir()
	casePath := filepath.Join(dir, "case.json")
	writeAPICaseFile(t, casePath)
	storePath := filepath.Join(dir, "store.sqlite")
	evidenceDir := filepath.Join(dir, "evidence")

	runCLI(t, "case", "run", "--case", casePath, "--base-url", server.URL, "--run-id", "case-run-diagnose", "--evidence-dir", evidenceDir, "--store", "sqlite://"+storePath, "--profile", "sample")
	out := runCLI(t, "case", "diagnose", "--case-run", "case-run-diagnose.case", "--store", "sqlite://"+storePath, "--json")

	var report struct {
		OK              bool     `json:"ok"`
		CaseRunID       string   `json:"caseRunId"`
		Status          string   `json:"status"`
		Category        string   `json:"category"`
		PrimaryFinding  string   `json:"primaryFinding"`
		AssertionErrors []string `json:"assertionErrors"`
		Signals         []struct {
			Name  string `json:"name"`
			Value string `json:"value"`
		} `json:"signals"`
		NextActions []string `json:"nextActions"`
	}
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatalf("decode diagnosis json: %v\n%s", err, out)
	}
	if report.OK || report.CaseRunID != "case-run-diagnose.case" || report.Status != "failed" || report.Category != "assertion-mismatch" {
		t.Fatalf("diagnosis identity = %#v", report)
	}
	if !strings.Contains(report.PrimaryFinding, "status code 400 was not expected") {
		t.Fatalf("primary finding = %q", report.PrimaryFinding)
	}
	if len(report.AssertionErrors) != 2 || !strings.Contains(report.AssertionErrors[0], "status code 400") {
		t.Fatalf("assertion errors = %#v", report.AssertionErrors)
	}
	foundHTTPStatus := false
	for _, signal := range report.Signals {
		if signal.Name == "http.status" && signal.Value == "400" {
			foundHTTPStatus = true
		}
	}
	if !foundHTTPStatus {
		t.Fatalf("diagnosis signals missing http.status=400: %#v", report.Signals)
	}
	joinedActions := strings.Join(report.NextActions, "\n")
	if !strings.Contains(joinedActions, "agent-testbench case evidence --case-run case-run-diagnose.case") {
		t.Fatalf("next actions = %#v", report.NextActions)
	}
}

func TestCaseGateFailsWithActionableReportForFailedCaseRuns(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	storePath := filepath.Join(dir, "store.sqlite")
	s, err := sqlite.Open(ctx, sqlite.Config{Path: storePath})
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	started := time.Date(2026, 5, 24, 9, 0, 0, 0, time.UTC)
	runID := "run.case-gate"
	if _, err := s.CreateRun(ctx, store.Run{
		ID:           runID,
		ProfileID:    "sample",
		Status:       store.StatusFailed,
		EvidenceRoot: filepath.Join(dir, "evidence", runID),
		StartedAt:    started,
		FinishedAt:   started.Add(time.Second),
		CreatedAt:    started,
		UpdatedAt:    started.Add(time.Second),
	}); err != nil {
		t.Fatalf("create run: %v", err)
	}
	for _, item := range []struct {
		id     string
		caseID string
		status string
	}{
		{id: runID + ".passed", caseID: "case.passed", status: store.StatusPassed},
		{id: runID + ".failed", caseID: "case.failed", status: store.StatusFailed},
	} {
		if _, err := s.RecordAPICaseRun(ctx, store.APICaseRun{
			ID:                   item.id,
			RunID:                runID,
			CaseID:               item.caseID,
			Status:               item.status,
			RequestSummaryJSON:   `{"method":"GET","path":"/gate"}`,
			AssertionSummaryJSON: `{"status":"` + item.status + `"}`,
			StartedAt:            started,
			FinishedAt:           started.Add(time.Second),
			CreatedAt:            started,
		}); err != nil {
			t.Fatalf("record case run %s: %v", item.id, err)
		}
		if _, err := s.RecordEvidence(ctx, store.EvidenceRecord{
			ID:        item.id + ".summary",
			RunID:     runID,
			CaseRunID: item.id,
			Kind:      "summary",
			URI:       filepath.Join(dir, "evidence", runID, item.id, "summary.json"),
			MediaType: "application/json",
			Summary:   `{"kind":"summary"}`,
			CreatedAt: started,
		}); err != nil {
			t.Fatalf("record evidence %s: %v", item.id, err)
		}
	}
	if err := s.Close(); err != nil {
		t.Fatalf("close store: %v", err)
	}

	out := runCLIFails(t, "case", "gate", "--store", "sqlite://"+storePath, "--run", runID, "--require-no-failures", "--require-evidence", "--json")
	var report struct {
		OK     bool   `json:"ok"`
		RunID  string `json:"runId"`
		Counts struct {
			Total            int `json:"total"`
			Passed           int `json:"passed"`
			Failed           int `json:"failed"`
			EvidenceComplete int `json:"evidenceComplete"`
		} `json:"counts"`
		Gates struct {
			HasCaseRuns      bool `json:"hasCaseRuns"`
			NoFailures       bool `json:"noFailures"`
			EvidenceComplete bool `json:"evidenceComplete"`
		} `json:"gates"`
		FailedCaseRuns []struct {
			ID     string `json:"id"`
			CaseID string `json:"caseId"`
			Status string `json:"status"`
		} `json:"failedCaseRuns"`
		NextActions []string `json:"nextActions"`
	}
	if err := json.Unmarshal([]byte(extractJSONObject(t, out)), &report); err != nil {
		t.Fatalf("decode gate json: %v\n%s", err, out)
	}
	if report.OK || report.RunID != runID || report.Counts.Total != 2 || report.Counts.Passed != 1 || report.Counts.Failed != 1 || report.Counts.EvidenceComplete != 2 {
		t.Fatalf("gate report counts = %#v", report)
	}
	if !report.Gates.HasCaseRuns || report.Gates.NoFailures || !report.Gates.EvidenceComplete {
		t.Fatalf("gate booleans = %#v", report.Gates)
	}
	if len(report.FailedCaseRuns) != 1 || report.FailedCaseRuns[0].ID != runID+".failed" || report.FailedCaseRuns[0].CaseID != "case.failed" {
		t.Fatalf("failed case runs = %#v", report.FailedCaseRuns)
	}
	if !strings.Contains(strings.Join(report.NextActions, "\n"), "agent-testbench case diagnose --case-run "+runID+".failed") {
		t.Fatalf("gate next actions = %#v", report.NextActions)
	}
}

func TestCaseRunCommandExecutesStoreCatalogCaseID(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/catalog" {
			t.Fatalf("request path = %s", r.URL.Path)
		}
		var request map[string]any
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if request["id"] != "item-override" {
			t.Fatalf("request overrides = %#v", request)
		}
		w.WriteHeader(http.StatusCreated)
		_, _ = fmt.Fprint(w, `{"status":"created"}`)
	}))
	defer server.Close()

	dir := t.TempDir()
	storePath := filepath.Join(dir, "store.sqlite")
	evidenceDir := filepath.Join(dir, "evidence")
	s, err := sqlite.Open(context.Background(), sqlite.Config{Path: storePath})
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	if err := s.ReplaceProfileCatalog(context.Background(), store.ProfileCatalog{
		ProfileID: "sample",
		APICases: []store.CatalogAPICase{{
			ID:          "case.catalog",
			DisplayName: "Catalog Case",
			NodeID:      "node.alpha",
		}},
		TemplateConfigs: []store.CatalogTemplateConfig{{
			ID:         "cfg.case.catalog",
			ScopeType:  "api-case",
			ScopeID:    "case.catalog",
			ConfigJSON: `{"caseId":"case.catalog","caseExecution":{"method":"POST","nodeId":"node.alpha","path":"/v1/catalog","body":{"id":"{{ override:id }}"},"expectedHttpCodes":[201]}}`,
			Status:     "active",
		}},
	}); err != nil {
		t.Fatalf("replace catalog: %v", err)
	}
	s.Close()

	out := runCLI(t, "case", "run", "--case-id", "case.catalog", "--base-url", server.URL, "--run-id", "catalog-run-001", "--evidence-dir", evidenceDir, "--store", "sqlite://"+storePath, "--profile", "sample", "--override", "id=item-override", "--json")
	var payload map[string]any
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		t.Fatalf("decode case-id run json: %v\n%s", err, out)
	}
	if payload["runId"] != "catalog-run-001" || payload["caseRunId"] != "catalog-run-001.case" || payload["caseId"] != "case.catalog" || payload["status"] != "passed" {
		t.Fatalf("case-id run payload = %#v", payload)
	}

	s, err = sqlite.Open(context.Background(), sqlite.Config{Path: storePath})
	if err != nil {
		t.Fatalf("reopen store: %v", err)
	}
	defer s.Close()
	run, err := s.GetRun(context.Background(), "catalog-run-001")
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if run.ProfileID != "sample" || run.Status != "passed" || run.EvidenceRoot != filepath.Join(evidenceDir, "catalog-run-001") {
		t.Fatalf("run = %#v", run)
	}
	caseRuns, err := s.ListAPICaseRuns(context.Background(), "catalog-run-001")
	if err != nil {
		t.Fatalf("list api case runs: %v", err)
	}
	if len(caseRuns) != 1 || caseRuns[0].CaseID != "case.catalog" || caseRuns[0].Status != "passed" {
		t.Fatalf("case runs = %#v", caseRuns)
	}
	records, err := s.ListEvidence(context.Background(), "catalog-run-001")
	if err != nil {
		t.Fatalf("list evidence: %v", err)
	}
	if len(records) != 3 {
		t.Fatalf("evidence records = %#v", records)
	}
	if _, err := os.Stat(filepath.Join(evidenceDir, "catalog-run-001", "request.json")); err != nil {
		t.Fatalf("request evidence missing: %v", err)
	}
}
