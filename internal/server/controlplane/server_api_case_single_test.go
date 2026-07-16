package controlplane_test

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"sync/atomic"
	"testing"

	"agent-testbench/internal/domain/profile"
	"agent-testbench/internal/server/controlplane"
)

func TestServerRejectsPublicFilePathCaseExecution(t *testing.T) {
	ctx, s := openAPICaseBatchSQLiteStore(t)

	var targetCalls atomic.Int32
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		targetCalls.Add(1)
		w.WriteHeader(http.StatusOK)
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

	evidenceDir := filepath.Join(dir, "evidence")
	body := `{"casePath":` + strconv.Quote(casePath) + `,"baseUrl":` + strconv.Quote(target.URL) + `,"evidenceDir":` + strconv.Quote(evidenceDir) + `,"runId":"forged","environmentId":"forged"}`
	var payload struct {
		OK   bool   `json:"ok"`
		Code string `json:"code"`
	}
	postJSONInto(t, server.URL+"/api/cases/run", body, http.StatusGone, &payload)
	if payload.OK || payload.Code != "file_path_execution_disabled" {
		t.Fatalf("api case run payload = %#v", payload)
	}
	if got := targetCalls.Load(); got != 0 {
		t.Fatalf("target calls = %d, want 0", got)
	}
	if _, err := os.Stat(evidenceDir); !os.IsNotExist(err) {
		t.Fatalf("evidence directory was created: %v", err)
	}

	runs, err := s.ListRuns(ctx)
	if err != nil {
		t.Fatalf("list runs: %v", err)
	}
	if len(runs) != 0 {
		t.Fatalf("stored runs = %#v", runs)
	}
}
