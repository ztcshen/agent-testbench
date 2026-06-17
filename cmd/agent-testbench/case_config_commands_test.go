package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os/exec"
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

func TestCaseConfigUpsertUpdatesSelectedConfigAndRequestAuthFields(t *testing.T) {
	storePath := filepath.Join(t.TempDir(), "store.sqlite")
	seedCaseConfigUpsertCatalogWithBaseConfig(t, storePath)
	keyPath := writeCaseConfigSigningKey(t)
	authJSON, err := json.Marshal(map[string]string{
		"appId":   "app-001",
		"secret":  "secret-001",
		"keyPath": keyPath,
	})
	if err != nil {
		t.Fatalf("encode auth json: %v", err)
	}

	out := runCLI(t, "case", "config", "upsert",
		"--store", "sqlite://"+storePath,
		"--case", "case.generic.submit",
		"--method", "POST",
		"--path", "/generic/submit",
		"--body-json", `{"amount":"100.00"}`,
		"--header", "Content-Type=application/json",
		"--header", "X-Trace={{ override:trace_id }}",
		"--auth-json", string(authJSON),
		"--signed",
		"--trace-endpoint", "gateway.generic.submit",
		"--expected-status", "200",
		"--json",
	)
	var report struct {
		OK      bool `json:"ok"`
		Created bool `json:"created"`
		Updated bool `json:"updated"`
		Config  struct {
			ID string `json:"id"`
		} `json:"config"`
		SelectedByRunner bool `json:"selectedByRunner"`
	}
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatalf("decode case config upsert report: %v\n%s", err, out)
	}
	if !report.OK || report.Created || !report.Updated || report.Config.ID != "config.case.generic.submit" || !report.SelectedByRunner {
		t.Fatalf("case config upsert report = %#v", report)
	}

	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/generic/submit" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		if r.Header.Get("Content-Type") != "application/json" || r.Header.Get("X-Trace") != "trace-123" {
			t.Fatalf("headers were not rendered from upsert config: %#v", r.Header)
		}
		if strings.TrimSpace(r.Header.Get("Authorization")) == "" {
			t.Fatalf("signed request missing Authorization header")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"ok":true}`)
	}))
	defer target.Close()

	runOut := runCLI(t, "case", "run",
		"--store", "sqlite://"+storePath,
		"--case-id", "case.generic.submit",
		"--base-url", target.URL,
		"--run-id", "run.case-config-auth-upsert",
		"--override", "trace_id=trace-123",
		"--json",
	)
	if !strings.Contains(runOut, `"status": "passed"`) {
		t.Fatalf("store-backed auth/header config should pass run:\n%s", runOut)
	}
}

func writeCaseConfigSigningKey(t *testing.T) string {
	t.Helper()
	keyPath := filepath.Join(t.TempDir(), "request-signing-key.pem")
	cmd := exec.Command("openssl", "genpkey", "-algorithm", "RSA", "-pkeyopt", "rsa_keygen_bits:2048", "-out", keyPath)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("generate signing key: %v\n%s", err, out)
	}
	return keyPath
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

func seedCaseConfigUpsertCatalogWithBaseConfig(t *testing.T, storePath string) {
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
		TemplateConfigs: []store.CatalogTemplateConfig{{
			ID:         "config.case.generic.submit",
			ScopeType:  "case",
			ScopeID:    "case.generic.submit",
			Status:     "active",
			ConfigJSON: `{"caseId":"case.generic.submit","caseExecution":{"method":"POST","nodeId":"node.generic","path":"/generic/submit"}}`,
		}},
	}); err != nil {
		t.Fatalf("replace profile catalog: %v", err)
	}
}
