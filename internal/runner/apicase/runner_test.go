package apicase_test

import (
	"bytes"
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

	"agent-testbench/internal/runner/apicase"
)

func TestRunWritesEvidenceBundle(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"status":"created"}`)
	}))
	defer server.Close()
	casePath := filepath.Join(t.TempDir(), "case.json")
	writeCaseFile(t, casePath)
	evidenceDir := filepath.Join(t.TempDir(), "evidence")

	result, err := apicase.Run(context.Background(), apicase.RunOptions{
		CasePath:    casePath,
		EvidenceDir: evidenceDir,
		RunID:       "run-001",
		BaseURL:     server.URL,
	})
	if err != nil {
		t.Fatalf("run api case: %v", err)
	}
	if result.Status != "passed" || result.RunID != "run-001" || result.EvidencePath == "" {
		t.Fatalf("result = %#v", result)
	}
	if result.StartedAt == "" || result.FinishedAt == "" || result.ElapsedMs < 0 {
		t.Fatalf("result timing was not recorded: %#v", result)
	}

	for _, name := range []string{"case.json", "request.json", "response.json", "assertions.json", "summary.json"} {
		if _, err := os.Stat(filepath.Join(result.EvidencePath, name)); err != nil {
			t.Fatalf("expected evidence file %s: %v", name, err)
		}
	}

	var request struct {
		Method string         `json:"method"`
		Path   string         `json:"path"`
		Body   map[string]any `json:"body"`
	}
	readJSONFile(t, filepath.Join(result.EvidencePath, "request.json"), &request)
	if request.Method != "POST" || request.Path != "/v1/items" || request.Body["id"] != "item-001" {
		t.Fatalf("request = %#v", request)
	}
}

func TestRunExecutesHTTPCaseAndWritesResponseEvidence(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/items" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = fmt.Fprint(w, `{"status":"created"}`)
	}))
	defer server.Close()

	casePath := filepath.Join(t.TempDir(), "case.json")
	writeCaseFile(t, casePath)
	evidenceDir := filepath.Join(t.TempDir(), "evidence")

	result, err := apicase.Run(context.Background(), apicase.RunOptions{
		CasePath:    casePath,
		EvidenceDir: evidenceDir,
		RunID:       "run-002",
		BaseURL:     server.URL,
	})
	if err != nil {
		t.Fatalf("run api case: %v", err)
	}
	if result.Status != "passed" {
		t.Fatalf("result = %#v", result)
	}
	for _, name := range []string{"response.json", "assertions.json"} {
		if _, err := os.Stat(filepath.Join(result.EvidencePath, name)); err != nil {
			t.Fatalf("expected evidence file %s: %v", name, err)
		}
	}
	var assertions struct {
		Status string `json:"status"`
	}
	readJSONFile(t, filepath.Join(result.EvidencePath, "assertions.json"), &assertions)
	if assertions.Status != "passed" {
		t.Fatalf("assertions = %#v", assertions)
	}
}

func TestRunFailsWhenResponseContainsForbiddenFragment(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"status":"created","trial_available":true}`)
	}))
	defer server.Close()

	casePath := filepath.Join(t.TempDir(), "case.json")
	raw := []byte(`{
  "id": "case.no-forbidden-field",
  "request": {"method": "GET", "path": "/v1/items"},
  "assertions": {
    "expectedStatusCodes": [200],
    "responseNotContains": ["\"trial_available\""]
  }
}`)
	if err := os.WriteFile(casePath, raw, 0o644); err != nil {
		t.Fatalf("write case file: %v", err)
	}

	result, err := apicase.Run(context.Background(), apicase.RunOptions{
		CasePath:    casePath,
		EvidenceDir: filepath.Join(t.TempDir(), "evidence"),
		RunID:       "run-forbidden",
		BaseURL:     server.URL,
	})
	if err != nil {
		t.Fatalf("run api case: %v", err)
	}
	if result.Status != "failed" {
		t.Fatalf("forbidden response field should fail assertions: %#v", result)
	}
	var assertions struct {
		Status string   `json:"status"`
		Errors []string `json:"errors"`
	}
	readJSONFile(t, filepath.Join(result.EvidencePath, "assertions.json"), &assertions)
	if assertions.Status != "failed" || len(assertions.Errors) != 1 || !strings.Contains(assertions.Errors[0], "must not contain") {
		t.Fatalf("assertions = %#v", assertions)
	}
}

func TestRunPreservesTimeoutFailureEvidence(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-time.After(100 * time.Millisecond):
		}
	}))
	defer server.Close()

	casePath := filepath.Join(t.TempDir(), "case.json")
	writeCaseFile(t, casePath)
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Millisecond)
	defer cancel()

	result, err := apicase.Run(ctx, apicase.RunOptions{
		CasePath:    casePath,
		EvidenceDir: filepath.Join(t.TempDir(), "evidence"),
		RunID:       "run-timeout",
		BaseURL:     server.URL,
	})
	if err != nil {
		t.Fatalf("timeout should be represented by a failed run result: %v", err)
	}
	if result.OK || result.Status != "failed" || result.FailurePhase != "request-send" || result.FailureCategory != "timeout" || !strings.Contains(result.Error, "deadline exceeded") {
		t.Fatalf("timeout result = %#v", result)
	}
	for _, name := range []string{"case.json", "request.json", "assertions.json", "error.json", "summary.json"} {
		if _, err := os.Stat(filepath.Join(result.EvidencePath, name)); err != nil {
			t.Fatalf("expected timeout evidence %s: %v", name, err)
		}
	}
	var evidence apicase.ErrorEvidence
	readJSONFile(t, filepath.Join(result.EvidencePath, "error.json"), &evidence)
	if evidence.Status != "failed" || evidence.Phase != "request-send" || evidence.Category != "timeout" || evidence.Message == "" {
		t.Fatalf("timeout error evidence = %#v", evidence)
	}
}

func TestRunReturnsPartialResultWhenEvidenceDirectoryCannotBeCreated(t *testing.T) {
	casePath := filepath.Join(t.TempDir(), "case.json")
	writeCaseFile(t, casePath)
	occupied := filepath.Join(t.TempDir(), "occupied")
	if err := os.WriteFile(occupied, []byte("not a directory"), 0o644); err != nil {
		t.Fatalf("write occupied Evidence root: %v", err)
	}

	result, err := apicase.Run(context.Background(), apicase.RunOptions{
		CasePath:    casePath,
		EvidenceDir: occupied,
		RunID:       "run-evidence-write-failure",
		BaseURL:     "http://127.0.0.1",
	})
	if err == nil || !strings.Contains(err.Error(), "create evidence directory") {
		t.Fatalf("Evidence directory error = %v", err)
	}
	if result.RunID != "run-evidence-write-failure" || result.CaseID == "" || result.EvidencePath == "" {
		t.Fatalf("partial failed result handles = %#v", result)
	}
	if result.OK || result.Status != "failed" || result.FailurePhase != apicase.FailurePhasePersistence || result.FailureCategory != apicase.FailureCategoryEvidenceWrite {
		t.Fatalf("partial failed result classification = %#v", result)
	}
	if result.StartedAt == "" || result.FinishedAt == "" || result.Error == "" {
		t.Fatalf("partial failed result timing/error = %#v", result)
	}
}

func TestRunRejectsUnsafeExplicitRunIDBeforeWritingEvidence(t *testing.T) {
	casePath := filepath.Join(t.TempDir(), "case.json")
	writeCaseFile(t, casePath)
	for _, runID := range []string{".", "..", "nested/run", `nested\\run`, "run:escape", filepath.Join(string(filepath.Separator), "absolute-run")} {
		t.Run(strings.ReplaceAll(runID, string(filepath.Separator), "_"), func(t *testing.T) {
			evidenceDir := filepath.Join(t.TempDir(), "evidence")
			result, err := apicase.Run(context.Background(), apicase.RunOptions{
				CasePath:    casePath,
				EvidenceDir: evidenceDir,
				RunID:       runID,
				BaseURL:     "http://127.0.0.1",
			})
			if err == nil || !strings.Contains(err.Error(), "single path segment") {
				t.Fatalf("unsafe run id %q error = %v", runID, err)
			}
			if result.RunID != "" || result.EvidencePath != "" {
				t.Fatalf("unsafe run id %q result = %#v", runID, result)
			}
			if _, statErr := os.Stat(evidenceDir); !errors.Is(statErr, os.ErrNotExist) {
				t.Fatalf("unsafe run id %q created Evidence root: %v", runID, statErr)
			}
		})
	}
}

func TestRunDoesNotFollowHTTPRedirects(t *testing.T) {
	var redirected bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/redirected" {
			redirected = true
			w.WriteHeader(http.StatusOK)
			return
		}
		http.Redirect(w, r, "/redirected", http.StatusFound)
	}))
	defer server.Close()
	casePath := filepath.Join(t.TempDir(), "redirect-case.json")
	if err := os.WriteFile(casePath, []byte(`{
		"id":"case.redirect",
		"request":{"method":"GET","path":"/start"},
		"assertions":{"expectedStatusCodes":[302]}
	}`), 0o644); err != nil {
		t.Fatalf("write redirect case: %v", err)
	}
	result, err := apicase.Run(context.Background(), apicase.RunOptions{
		CasePath: casePath, EvidenceDir: t.TempDir(), RunID: "run-redirect", BaseURL: server.URL,
	})
	if err != nil || !result.OK {
		t.Fatalf("redirect run = %#v err=%v", result, err)
	}
	if redirected {
		t.Fatal("api case runner followed redirect")
	}
	var response apicase.ResponseEvidence
	readJSONFile(t, filepath.Join(result.EvidencePath, "response.json"), &response)
	if response.StatusCode != http.StatusFound {
		t.Fatalf("redirect response = %#v", response)
	}
}

func TestRunRejectsRequestPathsThatOverrideConfiguredOrigin(t *testing.T) {
	var configuredCalls int
	configured := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		configuredCalls++
	}))
	defer configured.Close()
	var overrideCalls int
	override := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		overrideCalls++
	}))
	defer override.Close()
	paths := []string{
		override.URL + "/collect",
		strings.TrimPrefix(override.URL, "http:") + "/collect",
		"http:attacker.invalid/collect",
	}
	for index, requestPath := range paths {
		t.Run(fmt.Sprintf("path-%d", index), func(t *testing.T) {
			casePath := filepath.Join(t.TempDir(), "origin-override.json")
			caseJSON := fmt.Sprintf(`{
				"id":"case.origin-override",
				"request":{"method":"GET","path":%q},
				"assertions":{"expectedStatusCodes":[200]}
			}`, requestPath)
			if err := os.WriteFile(casePath, []byte(caseJSON), 0o644); err != nil {
				t.Fatalf("write origin override case: %v", err)
			}
			result, err := apicase.Run(context.Background(), apicase.RunOptions{
				CasePath: casePath, EvidenceDir: t.TempDir(), RunID: fmt.Sprintf("run-origin-%d", index), BaseURL: configured.URL,
			})
			if err != nil {
				t.Fatalf("origin override should be represented by failed Evidence: %v", err)
			}
			if result.OK || result.Status != "failed" || result.FailurePhase != "request-materialization" || !strings.Contains(result.Error, "must not override") {
				t.Fatalf("origin override result = %#v", result)
			}
		})
	}
	if configuredCalls != 0 || overrideCalls != 0 {
		t.Fatalf("origin override reached HTTP server: configured=%d override=%d", configuredCalls, overrideCalls)
	}
}

func TestRunFailsWhenStreamingResponseExceedsLimit(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		chunk := bytes.Repeat([]byte("x"), 32<<10)
		for written := int64(0); written <= apicase.MaxResponseBodyBytes; written += int64(len(chunk)) {
			if _, err := w.Write(chunk); err != nil {
				return
			}
		}
	}))
	defer server.Close()
	casePath := filepath.Join(t.TempDir(), "large-response.json")
	if err := os.WriteFile(casePath, []byte(`{
		"id":"case.large-response",
		"request":{"method":"GET","path":"/stream"},
		"assertions":{"expectedStatusCodes":[200]}
	}`), 0o644); err != nil {
		t.Fatalf("write large response case: %v", err)
	}
	result, err := apicase.Run(context.Background(), apicase.RunOptions{
		CasePath: casePath, EvidenceDir: t.TempDir(), RunID: "run-large-response", BaseURL: server.URL,
	})
	if err != nil {
		t.Fatalf("large response must be represented by failed Evidence: %v", err)
	}
	if result.OK || result.Status != "failed" || result.FailurePhase != "response-read" || result.FailureCategory != "response-too-large" || !strings.Contains(result.Error, "exceeds") {
		t.Fatalf("large response result = %#v", result)
	}
	for _, name := range []string{"case.json", "request.json", "assertions.json", "error.json", "summary.json"} {
		if _, err := os.Stat(filepath.Join(result.EvidencePath, name)); err != nil {
			t.Fatalf("large response Evidence %s: %v", name, err)
		}
	}
	if _, err := os.Stat(filepath.Join(result.EvidencePath, "response.json")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("oversized response must not be stored as truncated Evidence: %v", err)
	}
	var errorEvidence apicase.ErrorEvidence
	readJSONFile(t, filepath.Join(result.EvidencePath, "error.json"), &errorEvidence)
	if errorEvidence.Phase != "response-read" || errorEvidence.Category != "response-too-large" || errorEvidence.Message != result.Error {
		t.Fatalf("large response error Evidence = %#v", errorEvidence)
	}
}

func TestRunChecksLeaseBeforeEveryEvidenceWrite(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()
	casePath := filepath.Join(t.TempDir(), "case.json")
	writeCaseFile(t, casePath)
	var checks int
	leaseLost := errors.New("case Evidence owner lease lost")
	result, err := apicase.Run(context.Background(), apicase.RunOptions{
		CasePath:    casePath,
		EvidenceDir: t.TempDir(),
		RunID:       "run-evidence-fence",
		BaseURL:     server.URL,
		BeforeEvidenceWrite: func(context.Context) error {
			checks++
			if checks == 4 {
				return leaseLost
			}
			return nil
		},
	})
	if !errors.Is(err, leaseLost) {
		t.Fatalf("Evidence fence error = %v, want lease lost", err)
	}
	if result.FailurePhase != apicase.FailurePhasePersistence || result.FailureCategory != apicase.FailureCategoryEvidenceWrite {
		t.Fatalf("Evidence fence result = %#v", result)
	}
	for _, name := range []string{"case.json", "request.json"} {
		if _, err := os.Stat(filepath.Join(result.EvidencePath, name)); err != nil {
			t.Fatalf("expected pre-takeover Evidence %s: %v", name, err)
		}
	}
	for _, name := range []string{"response.json", "assertions.json", "summary.json"} {
		if _, err := os.Stat(filepath.Join(result.EvidencePath, name)); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("post-takeover Evidence %s error = %v, want not exist", name, err)
		}
	}
}

func TestRunAppliesRequestBodyOverrides(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"status":"created"}`)
	}))
	defer server.Close()
	casePath := filepath.Join(t.TempDir(), "case.json")
	writeCaseFile(t, casePath)
	evidenceDir := filepath.Join(t.TempDir(), "evidence")

	result, err := apicase.Run(context.Background(), apicase.RunOptions{
		CasePath:    casePath,
		EvidenceDir: evidenceDir,
		RunID:       "run-override",
		BaseURL:     server.URL,
		Overrides: map[string]any{
			"id":       "item-override",
			"priority": "high",
		},
	})
	if err != nil {
		t.Fatalf("run api case with overrides: %v", err)
	}

	var request struct {
		Body map[string]any `json:"body"`
	}
	readJSONFile(t, filepath.Join(result.EvidencePath, "request.json"), &request)
	if request.Body["id"] != "item-override" || request.Body["priority"] != "high" {
		t.Fatalf("request body overrides = %#v", request.Body)
	}
}

func writeCaseFile(t *testing.T, path string) {
	t.Helper()
	raw := []byte(`{
  "id": "case.alpha",
  "title": "Create Item",
  "request": {
    "method": "POST",
    "path": "/v1/items",
    "headers": {"Content-Type": "application/json"},
    "body": {"id": "item-001"}
  },
  "assertions": {
    "expectedStatusCodes": [200, 201],
    "responseContains": ["created"]
  }
}`)
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatalf("write case file: %v", err)
	}
}

func readJSONFile(t *testing.T, path string, target any) {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	if err := json.Unmarshal(raw, target); err != nil {
		t.Fatalf("decode %s: %v", path, err)
	}
}
