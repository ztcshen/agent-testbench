package controlplane_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"agent-testbench/internal/domain/profile"
	"agent-testbench/internal/server/controlplane"
	"agent-testbench/internal/store"
)

func TestServerRunsAPICaseAndIndexesStoreRecords(t *testing.T) {
	ctx, s := openAPICaseBatchSQLiteStore(t)

	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/items" {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		var request map[string]any
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatalf("decode target request: %v", err)
		}
		if request["id"] != "item-override" {
			t.Fatalf("target request overrides = %#v", request)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"created"}`))
	}))
	defer target.Close()

	dir := t.TempDir()
	casePath := filepath.Join(dir, "case.json")
	if err := os.WriteFile(casePath, []byte(`{
  "id": "case.alpha",
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
}`), 0o644); err != nil {
		t.Fatalf("write api case: %v", err)
	}

	server := httptest.NewServer(controlplane.NewWithStore(profile.Bundle{ID: "sample", DisplayName: "Sample Profile"}, s))
	defer server.Close()

	body := `{"casePath":` + strconv.Quote(casePath) + `,"baseUrl":` + strconv.Quote(target.URL) + `,"evidenceDir":` + strconv.Quote(filepath.Join(dir, "evidence")) + `,"overrides":{"id":"item-override"}}`
	var payload struct {
		OK        bool   `json:"ok"`
		ViewerURL string `json:"viewerUrl"`
		Report    struct {
			RunID          string `json:"run_id"`
			CaseID         string `json:"case_id"`
			Status         string `json:"status"`
			Operation      string `json:"operation"`
			ActualHTTPCode int    `json:"actual_http_code"`
			ElapsedMs      int64  `json:"elapsed_ms"`
		} `json:"report"`
	}
	postJSONInto(t, server.URL+"/api/cases/run", body, http.StatusOK, &payload)
	if !payload.OK || payload.Report.CaseID != "case.alpha" || payload.Report.Status != store.StatusPassed || payload.ViewerURL == "" {
		t.Fatalf("api case run payload = %#v", payload)
	}
	if payload.Report.RunID == "" || payload.Report.ElapsedMs < 0 {
		t.Fatalf("api case run timing = %#v", payload.Report)
	}
	if payload.Report.Operation != "POST /v1/items" || payload.Report.ActualHTTPCode != 200 {
		t.Fatalf("api case run report details = %#v", payload.Report)
	}

	runs, err := s.ListRuns(ctx)
	if err != nil {
		t.Fatalf("list runs: %v", err)
	}
	if len(runs) != 1 || runs[0].ID != payload.Report.RunID || runs[0].Status != store.StatusPassed {
		t.Fatalf("stored runs = %#v", runs)
	}
	caseRuns, err := s.ListAPICaseRuns(ctx, payload.Report.RunID)
	if err != nil {
		t.Fatalf("list api case runs: %v", err)
	}
	if len(caseRuns) != 1 || caseRuns[0].CaseID != "case.alpha" || !caseRuns[0].FinishedAt.After(caseRuns[0].StartedAt) {
		t.Fatalf("stored api case runs = %#v", caseRuns)
	}
	evidence, err := s.ListEvidence(ctx, payload.Report.RunID)
	if err != nil {
		t.Fatalf("list evidence: %v", err)
	}
	if len(evidence) < 4 {
		t.Fatalf("stored evidence = %#v", evidence)
	}
}
