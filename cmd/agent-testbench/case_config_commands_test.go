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
	"time"

	"agent-testbench/internal/store"
	"agent-testbench/internal/store/sqlite"
)

func TestCaseConfigUpsertMaintainsStoreBackedExecutionConfig(t *testing.T) {
	storePath := filepath.Join(t.TempDir(), "store.sqlite")
	seedCaseConfigUpsertCatalog(t, storePath)

	out := runCLI(t, "case", "config", "upsert",
		"--store", "sqlite://"+storePath,
		"--case", "case.generic.submit",
		"--method", "POST",
		"--path", "/generic/submit",
		"--body-json", `{"amount":"100.00"}`,
		"--expected-status", "200",
		"--response-not-contains", `"trial_available"`,
		"--json",
	)
	var report struct {
		OK      bool   `json:"ok"`
		CaseID  string `json:"caseId"`
		Created bool   `json:"created"`
		Config  struct {
			ID string `json:"id"`
		} `json:"config"`
	}
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatalf("decode case config upsert report: %v\n%s", err, out)
	}
	if !report.OK || report.CaseID != "case.generic.submit" || !report.Created || report.Config.ID == "" {
		t.Fatalf("case config upsert report = %#v", report)
	}

	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/generic/submit" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"result_status":"S","trial_available":true}`)
	}))
	defer target.Close()

	runOut := runCLI(t, "case", "run",
		"--store", "sqlite://"+storePath,
		"--case-id", "case.generic.submit",
		"--base-url", target.URL,
		"--run-id", "run.case-config-upsert",
		"--json",
	)
	if !strings.Contains(runOut, `"status": "failed"`) {
		t.Fatalf("store-backed forbidden assertion should fail run:\n%s", runOut)
	}
}

func seedCaseConfigUpsertCatalog(t *testing.T, storePath string) {
	t.Helper()
	ctx := context.Background()
	s, err := sqlite.Open(ctx, sqlite.Config{Path: storePath})
	if err != nil {
		t.Fatalf("open sqlite store: %v", err)
	}
	defer s.Close()
	if err := s.ReplaceProfileCatalog(ctx, store.ProfileCatalog{
		ProfileID: "default",
		IndexedAt: time.Now().UTC(),
		InterfaceNodes: []store.CatalogInterfaceNode{{
			ID:        "node.generic",
			ServiceID: "service.generic",
			Method:    "POST",
			Path:      "/generic/submit",
			Status:    "active",
		}},
		APICases: []store.CatalogAPICase{{
			ID:          "case.generic.submit",
			DisplayName: "Generic Submit",
			NodeID:      "node.generic",
			Status:      "active",
		}},
	}); err != nil {
		t.Fatalf("replace profile catalog: %v", err)
	}
}
