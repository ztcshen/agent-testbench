package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"agent-testbench/internal/store"
	"agent-testbench/internal/store/sqlite"
)

func TestCaseIncompleteBatchesCommandReportsNotRunCases(t *testing.T) {
	storeRef := configureNamedPostgreSQLActiveStore(t, "daily-incomplete-batches-pg")
	runCaseIncompleteBatchesCommandReportsNotRunCases(t, storeRef, "PostgreSQL")
}

func TestCaseIncompleteBatchesUsesNamedMySQLActiveStore(t *testing.T) {
	storeRef := configureNamedMySQLActiveStore(t, "daily-incomplete-batches-mysql")
	runCaseIncompleteBatchesCommandReportsNotRunCases(t, storeRef, "MySQL")
}

func runCaseIncompleteBatchesCommandReportsNotRunCases(t *testing.T, _ string, label string) {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprint(w, `{"status":"created"}`)
	}))
	defer server.Close()
	dir := t.TempDir()
	profileDir := filepath.Join(dir, "profile")
	alphaPath := filepath.Join(dir, "case.alpha.json")
	betaPath := filepath.Join(dir, "case.beta.json")
	alphaCaseID := uniqueTestID(t, "case.alpha")
	betaCaseID := uniqueTestID(t, "case.beta")
	runID := uniqueTestID(t, "run-alpha")
	writeFile(t, alphaPath, fmt.Sprintf(`{
  "id": %q,
  "title": "Create Item",
  "request": {
    "method": "POST",
    "path": "/v1/items",
    "headers": {"Content-Type": "application/json"},
    "body": {"id": "item-001"}
  },
  "assertions": {
    "expectedStatusCodes": [200],
    "responseContains": ["created"]
  }
}`, alphaCaseID))
	writeFile(t, betaPath, fmt.Sprintf(`{
  "id": %q,
  "title": "Read Item",
  "request": {"method": "GET", "path": "/v1/items/item-001"},
  "assertions": {"expectedStatusCodes": [200]}
}`, betaCaseID))
	writeFile(t, filepath.Join(profileDir, "profile.json"), fmt.Sprintf(`{
  "id": "sample",
  "displayName": "Sample Profile",
  "services": [],
  "workflows": [],
  "interfaceNodes": [],
  "apiCases": [
    {"id":%q,"displayName":"Case Alpha","casePath":%q},
    {"id":%q,"displayName":"Case Beta","casePath":%q}
  ],
  "requestTemplates": [],
  "caseDependencies": [],
  "workflowBindings": [],
  "fixtures": []
}`, alphaCaseID, alphaPath, betaCaseID, betaPath))

	runCLI(t, "case", "run", "--case", alphaPath, "--base-url", server.URL, "--run-id", runID, "--profile", "sample")

	out := runCLI(t, "case", "incomplete-batches", "--profile", profileDir)
	for _, want := range []string{"Incomplete API Cases: 1", betaCaseID, "not-run", betaPath} {
		if !strings.Contains(out, want) {
			t.Fatalf("%s incomplete case output missing %q: %q", label, want, out)
		}
	}

	jsonOut := runCLI(t, "case", "incomplete-batches", "--profile", profileDir, "--json")
	var report struct {
		OK    bool `json:"ok"`
		Count int  `json:"count"`
		Items []struct {
			ID      string `json:"id"`
			Reason  string `json:"reason"`
			Command string `json:"suggestedCommand"`
		} `json:"items"`
	}
	if err := json.Unmarshal([]byte(jsonOut), &report); err != nil {
		t.Fatalf("decode %s incomplete cases report: %v\n%s", label, err, jsonOut)
	}
	if !report.OK || report.Count != 1 || len(report.Items) != 1 {
		t.Fatalf("%s incomplete cases report = %#v", label, report)
	}
	if report.Items[0].ID != betaCaseID || report.Items[0].Reason != "not-run" {
		t.Fatalf("%s incomplete case item = %#v", label, report.Items[0])
	}
	if !strings.Contains(report.Items[0].Command, betaPath) {
		t.Fatalf("%s suggested command = %q", label, report.Items[0].Command)
	}

	ctx := context.Background()
	storeOnlyPath := filepath.Join(dir, "store-only.sqlite")
	storeOnly, err := sqlite.Open(ctx, sqlite.Config{Path: storeOnlyPath})
	if err != nil {
		t.Fatalf("open %s store-only catalog: %v", label, err)
	}
	if err := storeOnly.ReplaceProfileCatalog(ctx, store.ProfileCatalog{
		ProfileID: "current",
		APICases: []store.CatalogAPICase{
			{ID: "case.store.passed", DisplayName: "Passed Store Case", Status: "active"},
			{ID: "case.store.pending", DisplayName: "Pending Store Case", CasePath: betaPath, Status: "active"},
		},
	}); err != nil {
		t.Fatalf("seed %s store-only catalog: %v", label, err)
	}
	if _, err := storeOnly.CreateRun(ctx, store.Run{ID: "run.store.passed", ProfileID: "current", WorkflowID: "case.store.passed", Status: store.StatusPassed}); err != nil {
		t.Fatalf("create %s store-only run: %v", label, err)
	}
	if _, err := storeOnly.RecordAPICaseRun(ctx, store.APICaseRun{ID: "run.store.passed.case", RunID: "run.store.passed", CaseID: "case.store.passed", Status: store.StatusPassed}); err != nil {
		t.Fatalf("record %s store-only case run: %v", label, err)
	}
	if err := storeOnly.Close(); err != nil {
		t.Fatalf("close %s store-only catalog: %v", label, err)
	}

	storeOnlyOut := runCLI(t, "case", "incomplete-batches", "--store", "sqlite://"+storeOnlyPath, "--json")
	var storeOnlyReport struct {
		OK    bool `json:"ok"`
		Count int  `json:"count"`
		Items []struct {
			ID      string `json:"id"`
			Reason  string `json:"reason"`
			Source  string `json:"source"`
			Command string `json:"suggestedCommand"`
		} `json:"items"`
	}
	if err := json.Unmarshal([]byte(storeOnlyOut), &storeOnlyReport); err != nil {
		t.Fatalf("decode %s store-only incomplete cases report: %v\n%s", label, err, storeOnlyOut)
	}
	if !storeOnlyReport.OK || storeOnlyReport.Count != 1 || len(storeOnlyReport.Items) != 1 {
		t.Fatalf("%s store-only incomplete report = %#v", label, storeOnlyReport)
	}
	if storeOnlyReport.Items[0].ID != "case.store.pending" || storeOnlyReport.Items[0].Reason != "not-run" || storeOnlyReport.Items[0].Source != "profile:current" {
		t.Fatalf("%s store-only incomplete item = %#v", label, storeOnlyReport.Items[0])
	}
	if !strings.Contains(storeOnlyReport.Items[0].Command, betaPath) {
		t.Fatalf("%s store-only suggested command = %q", label, storeOnlyReport.Items[0].Command)
	}
}
