package controlplane_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"agent-testbench/internal/domain/profile"
	"agent-testbench/internal/runner/apicase"
	"agent-testbench/internal/server/controlplane"
	"agent-testbench/internal/store"
)

func TestServerTestKitRunExecutesStoreCatalogFileCase(t *testing.T) {
	ctx := context.Background()
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/items" {
			t.Fatalf("target request = %s %s", r.Method, r.URL.Path)
		}
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"status":"created"}`))
	}))
	defer target.Close()

	casePath := filepath.Join(t.TempDir(), "case.json")
	caseJSON := `{
		"id":"case.file",
		"request":{"method":"POST","path":"/items","body":{"id":"default"}},
		"assertions":{"expectedStatusCodes":[201],"responseContains":["created"]}
	}`
	if err := os.WriteFile(casePath, []byte(caseJSON), 0o644); err != nil {
		t.Fatalf("write case file: %v", err)
	}
	evidenceDir := filepath.Join(t.TempDir(), "evidence")
	runtime := openTestKitSQLiteStore(t, ctx, "file-case.sqlite")
	if err := runtime.ReplaceProfileCatalog(ctx, store.ProfileCatalog{
		ProfileID:      "sample",
		IndexedAt:      time.Now().UTC(),
		InterfaceNodes: []store.CatalogInterfaceNode{{ID: "node.file", Status: "active"}},
		APICases: []store.CatalogAPICase{{
			ID: "case.file", DisplayName: "File Case", NodeID: "node.file", Status: "active", CasePath: casePath,
			BaseURL: target.URL, EvidenceDir: evidenceDir, DefaultOverridesJSON: `{"id":"catalog"}`,
		}},
	}); err != nil {
		t.Fatalf("replace profile catalog: %v", err)
	}
	server := httptest.NewServer(controlplane.NewWithStore(profile.Bundle{ID: "bootstrap"}, runtime))
	defer server.Close()

	results := make([]map[string]any, 2)
	for index := range results {
		postJSONInto(t, server.URL+"/api/test-kit/run", `{"caseId":"case.file"}`, http.StatusOK, &results[index])
		if results[index]["ok"] != true || results[index]["status"] != store.StatusPassed || results[index]["caseId"] != "case.file" {
			t.Fatalf("file case result %d = %#v", index, results[index])
		}
	}
	firstRunID := strings.TrimSpace(fmt.Sprint(results[0]["runId"]))
	secondRunID := strings.TrimSpace(fmt.Sprint(results[1]["runId"]))
	if firstRunID == "" || secondRunID == "" || firstRunID == secondRunID {
		t.Fatalf("rapid file case run ids must be unique: %q %q", firstRunID, secondRunID)
	}
	summary := results[0]["summary"].(map[string]any)
	if summary["httpCode"] != float64(http.StatusCreated) {
		t.Fatalf("file case summary = %#v", summary)
	}
	runs, err := runtime.ListRuns(ctx)
	if err != nil || len(runs) != 2 || runs[0].Status != store.StatusPassed || runs[1].Status != store.StatusPassed {
		t.Fatalf("stored file case runs = %#v err=%v", runs, err)
	}
	for _, run := range runs {
		caseRuns, err := runtime.ListAPICaseRuns(ctx, run.ID)
		if err != nil || len(caseRuns) != 1 || caseRuns[0].CaseID != "case.file" {
			t.Fatalf("stored file case rows for %s = %#v err=%v", run.ID, caseRuns, err)
		}
	}
}

func TestServerTestKitRunRejectsUnsupportedExternalCaseExplicitly(t *testing.T) {
	ctx := context.Background()
	runtime := openTestKitSQLiteStore(t, ctx, "external-case.sqlite")
	if err := runtime.ReplaceProfileCatalog(ctx, store.ProfileCatalog{
		ProfileID:      "sample",
		IndexedAt:      time.Now().UTC(),
		InterfaceNodes: []store.CatalogInterfaceNode{{ID: "node.karate", Status: "active"}},
		APICases: []store.CatalogAPICase{{
			ID: "case.karate", NodeID: "node.karate", Status: "active", SourceKind: "karate",
			SourcePath: "tests/api.feature", ExecutorID: "executor.karate",
		}},
	}); err != nil {
		t.Fatalf("replace profile catalog: %v", err)
	}
	handler := controlplane.NewWithStore(profile.Bundle{ID: "bootstrap"}, runtime)
	capabilities := httptest.NewRecorder()
	handler.ServeHTTP(capabilities, httptest.NewRequest(http.MethodGet, "/api/cases/capabilities", nil))
	if capabilities.Code != http.StatusOK {
		t.Fatalf("external capabilities status = %d body=%s", capabilities.Code, capabilities.Body.String())
	}
	var capabilityPayload map[string]any
	if err := json.Unmarshal(capabilities.Body.Bytes(), &capabilityPayload); err != nil {
		t.Fatalf("decode external capabilities: %v", err)
	}
	capability := capabilityPayload["cases"].([]any)[0].(map[string]any)
	if capability["executionReady"] != false {
		t.Fatalf("external-only case must remain planning-only: %#v", capability)
	}
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/test-kit/run", strings.NewReader(`{"caseId":"case.karate"}`))
	request.Header.Set("Content-Type", "application/json")
	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusNotImplemented {
		t.Fatalf("external case status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	var result map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &result); err != nil {
		t.Fatalf("decode external result: %v", err)
	}
	errorText := fmt.Sprint(result["error"])
	if !strings.Contains(errorText, "karate") || !strings.Contains(errorText, "executor.karate") {
		t.Fatalf("external case error must identify unsupported contract: %#v", result)
	}
	runs, err := runtime.ListRuns(ctx)
	if err != nil || len(runs) != 0 {
		t.Fatalf("unsupported external case must not be recorded as an ordinary failed run: %#v err=%v", runs, err)
	}
}

func TestServerTestKitRunExecutesLocalConfigWhenExternalMetadataIsPresent(t *testing.T) {
	ctx := context.Background()
	var calls int
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.URL.Path != "/local" {
			t.Fatalf("local execution path = %q", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"local":true}`))
	}))
	defer target.Close()
	runtime := openTestKitSQLiteStore(t, ctx, "external-with-local.sqlite")
	if err := runtime.ReplaceProfileCatalog(ctx, store.ProfileCatalog{
		ProfileID:      "sample",
		IndexedAt:      time.Now().UTC(),
		InterfaceNodes: []store.CatalogInterfaceNode{{ID: "node.external-local", Status: "active"}},
		APICases: []store.CatalogAPICase{{
			ID: "case.external-local", NodeID: "node.external-local", Status: "active", BaseURL: target.URL,
			SourceKind: "karate", SourcePath: "tests/local.feature", ExecutorID: "executor.karate",
		}},
		TemplateConfigs: []store.CatalogTemplateConfig{{
			ID: "config.external-local", ScopeType: "case", ScopeID: "case.external-local", Status: "active",
			ConfigJSON: `{"caseExecution":{"method":"GET","nodeId":"node.external-local","path":"/local","expectedHttpCodes":[200]}}`,
		}},
	}); err != nil {
		t.Fatalf("replace external-local catalog: %v", err)
	}
	handler := controlplane.NewWithStore(profile.Bundle{ID: "bootstrap"}, runtime)
	capabilities := httptest.NewRecorder()
	handler.ServeHTTP(capabilities, httptest.NewRequest(http.MethodGet, "/api/cases/capabilities", nil))
	if capabilities.Code != http.StatusOK {
		t.Fatalf("external-local capabilities status = %d body=%s", capabilities.Code, capabilities.Body.String())
	}
	var capabilityPayload map[string]any
	if err := json.Unmarshal(capabilities.Body.Bytes(), &capabilityPayload); err != nil {
		t.Fatalf("decode external-local capabilities: %v", err)
	}
	capability := capabilityPayload["cases"].([]any)[0].(map[string]any)
	if capability["executionReady"] != true {
		t.Fatalf("external metadata with local execution must be ready: %#v", capability)
	}

	server := httptest.NewServer(handler)
	defer server.Close()
	var result map[string]any
	postJSONInto(t, server.URL+"/api/test-kit/run", `{"caseId":"case.external-local"}`, http.StatusOK, &result)
	if result["ok"] != true || calls != 1 {
		t.Fatalf("external-local execution = %#v calls=%d", result, calls)
	}
}

func TestServerTestKitRunUsesCaseScopedConfigWithoutEmbeddedCaseID(t *testing.T) {
	ctx := context.Background()
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/scoped" {
			t.Fatalf("target path = %q", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer target.Close()
	runtime := openTestKitSQLiteStore(t, ctx, "case-scope.sqlite")
	if err := runtime.ReplaceProfileCatalog(ctx, store.ProfileCatalog{
		ProfileID: "sample",
		IndexedAt: time.Now().UTC(),
		APICases:  []store.CatalogAPICase{{ID: "case.scoped", NodeID: "node.scoped", Status: "active", BaseURL: target.URL}},
		TemplateConfigs: []store.CatalogTemplateConfig{{
			ID: "config.scoped", ScopeType: "case", ScopeID: "case.scoped", Status: "active",
			ConfigJSON: `{"caseExecution":{"method":"GET","nodeId":"node.scoped","path":"/scoped","expectedHttpCodes":[200]}}`,
		}},
	}); err != nil {
		t.Fatalf("replace profile catalog: %v", err)
	}
	server := httptest.NewServer(controlplane.NewWithStore(profile.Bundle{ID: "bootstrap"}, runtime))
	defer server.Close()

	var result map[string]any
	postJSONInto(t, server.URL+"/api/test-kit/run", `{"caseId":"case.scoped"}`, http.StatusOK, &result)
	if result["ok"] != true {
		t.Fatalf("case-scoped config result = %#v", result)
	}
}

func TestServerTestKitRunDoesNotUseStepConfigTargetingAnotherCase(t *testing.T) {
	ctx := context.Background()
	targetCalled := false
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		targetCalled = true
		w.WriteHeader(http.StatusOK)
	}))
	defer target.Close()
	runtime := openTestKitSQLiteStore(t, ctx, "wrong-step-scope.sqlite")
	if err := runtime.ReplaceProfileCatalog(ctx, store.ProfileCatalog{
		ProfileID: "sample",
		IndexedAt: time.Now().UTC(),
		APICases:  []store.CatalogAPICase{{ID: "case.requested", Status: "active", BaseURL: target.URL}},
		TemplateConfigs: []store.CatalogTemplateConfig{{
			ID: "config.other", WorkflowID: "workflow.alpha", ScopeType: "step", ScopeID: "step.alpha", Status: "active",
			ConfigJSON: `{"caseId":"case.other","caseExecution":{"method":"GET","path":"/wrong"}}`,
		}},
	}); err != nil {
		t.Fatalf("replace profile catalog: %v", err)
	}
	result, err := controlplane.RunTrustedTestKitCase(ctx, profile.Bundle{ID: "bootstrap"}, runtime, controlplane.TrustedTestKitRunRequest{
		CaseID: "case.requested", WorkflowID: "workflow.alpha", StepID: "step.alpha",
	})
	if err != nil {
		t.Fatalf("run trusted wrong-case step config: %v", err)
	}
	if targetCalled {
		t.Fatal("step-scoped config for another case must not execute")
	}
	if result["ok"] != false || !strings.Contains(fmt.Sprint(result["error"]), "execution adapter") {
		t.Fatalf("wrong-case step config result = %#v", result)
	}
}

func TestServerTestKitFileCasePreservesAssertionFailureEvidence(t *testing.T) {
	ctx := context.Background()
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"state":"actual"}`))
	}))
	defer target.Close()
	runtime, serverURL := newCatalogFileCaseTestServer(t, ctx, "file-assertion.sqlite", target.URL, `{
		"id":"case.file.failure",
		"request":{"method":"GET","path":"/assertion"},
		"assertions":{"expectedStatusCodes":[200],"responseContains":["expected"]}
	}`)

	var result map[string]any
	postJSONInto(t, serverURL+"/api/test-kit/run", `{"caseId":"case.file.failure"}`, http.StatusOK, &result)
	failure := fmt.Sprint(result["error"])
	if result["ok"] != false || !strings.Contains(failure, "expected") {
		t.Fatalf("file assertion result = %#v", result)
	}
	evidenceRoot := strings.TrimSpace(fmt.Sprint(result["evidenceRoot"]))
	assertions := readTestKitEvidenceObject(t, filepath.Join(evidenceRoot, "assertions.json"))
	errorsValue, _ := assertions["errors"].([]any)
	if assertions["status"] != "failed" || len(errorsValue) != 1 || fmt.Sprint(errorsValue[0]) != failure {
		t.Fatalf("runner assertions were overwritten: %#v", assertions)
	}
	summary := readTestKitEvidenceObject(t, filepath.Join(evidenceRoot, "summary.json"))
	if summary["status"] != "failed" || summary["error"] != failure {
		t.Fatalf("runner summary was overwritten: %#v", summary)
	}
	requireTestKitEvidenceKinds(t, ctx, runtime, fmt.Sprint(result["runId"]), "case", "request", "response", "assertions", "summary")
}

func TestServerTestKitFileCasePreservesTransportFailureEvidence(t *testing.T) {
	ctx := context.Background()
	target := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	targetURL := target.URL
	target.Close()
	runtime, serverURL := newCatalogFileCaseTestServer(t, ctx, "file-transport.sqlite", targetURL, `{
		"id":"case.file.failure",
		"request":{"method":"GET","path":"/transport"},
		"assertions":{"expectedStatusCodes":[200]}
	}`)

	var result map[string]any
	postJSONInto(t, serverURL+"/api/test-kit/run", `{"caseId":"case.file.failure"}`, http.StatusOK, &result)
	if result["ok"] != false || strings.TrimSpace(fmt.Sprint(result["error"])) == "" {
		t.Fatalf("file transport result = %#v", result)
	}
	evidenceRoot := strings.TrimSpace(fmt.Sprint(result["evidenceRoot"]))
	assertions := readTestKitEvidenceObject(t, filepath.Join(evidenceRoot, "assertions.json"))
	errorEvidence := readTestKitEvidenceObject(t, filepath.Join(evidenceRoot, "error.json"))
	summary := readTestKitEvidenceObject(t, filepath.Join(evidenceRoot, "summary.json"))
	failure := fmt.Sprint(result["error"])
	assertionErrors, _ := assertions["errors"].([]any)
	if len(assertionErrors) != 1 || fmt.Sprint(assertionErrors[0]) != failure || errorEvidence["message"] != failure || summary["error"] != failure {
		t.Fatalf("runner failure Evidence mismatch: assertions=%#v error=%#v summary=%#v", assertions, errorEvidence, summary)
	}
	requireTestKitEvidenceKinds(t, ctx, runtime, fmt.Sprint(result["runId"]), "case", "request", "assertions", "error", "summary")
}

func TestServerTestKitInlineFailureWritesCanonicalEvidence(t *testing.T) {
	ctx := context.Background()
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"available":false}`))
	}))
	defer target.Close()
	runtime := openTestKitSQLiteStore(t, ctx, "inline-failure.sqlite")
	if err := runtime.ReplaceProfileCatalog(ctx, store.ProfileCatalog{
		ProfileID:      "sample",
		IndexedAt:      time.Now().UTC(),
		InterfaceNodes: []store.CatalogInterfaceNode{{ID: "node.inline", Status: "active"}},
		APICases:       []store.CatalogAPICase{{ID: "case.inline", NodeID: "node.inline", Status: "active", BaseURL: target.URL}},
		TemplateConfigs: []store.CatalogTemplateConfig{{
			ID: "config.inline", ScopeType: "case", ScopeID: "case.inline", Status: "active",
			ConfigJSON: `{"caseExecution":{"method":"GET","nodeId":"node.inline","path":"/health","expectedHttpCodes":[200]}}`,
		}},
	}); err != nil {
		t.Fatalf("replace inline catalog: %v", err)
	}
	server := httptest.NewServer(controlplane.NewWithStore(profile.Bundle{ID: "bootstrap"}, runtime))
	defer server.Close()

	var result map[string]any
	postJSONInto(t, server.URL+"/api/test-kit/run", `{"caseId":"case.inline"}`, http.StatusOK, &result)
	failure := fmt.Sprint(result["error"])
	if result["ok"] != false || !strings.Contains(failure, "503") {
		t.Fatalf("inline failure result = %#v", result)
	}
	evidenceRoot := strings.TrimSpace(fmt.Sprint(result["evidenceRoot"]))
	assertions := readTestKitEvidenceObject(t, filepath.Join(evidenceRoot, "assertions.json"))
	errorEvidence := readTestKitEvidenceObject(t, filepath.Join(evidenceRoot, "error.json"))
	summary := readTestKitEvidenceObject(t, filepath.Join(evidenceRoot, "summary.json"))
	assertionErrors, _ := assertions["errors"].([]any)
	if len(assertionErrors) != 1 || fmt.Sprint(assertionErrors[0]) != failure || errorEvidence["message"] != failure || summary["error"] != failure {
		t.Fatalf("inline failure Evidence mismatch: assertions=%#v error=%#v summary=%#v", assertions, errorEvidence, summary)
	}
	requireTestKitEvidenceKinds(t, ctx, runtime, fmt.Sprint(result["runId"]), "case", "request", "response", "assertions", "error", "summary")
}

func TestServerTestKitInlineCaseDoesNotFollowRedirect(t *testing.T) {
	ctx := context.Background()
	var redirected bool
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/redirected" {
			redirected = true
			w.WriteHeader(http.StatusOK)
			return
		}
		http.Redirect(w, r, "/redirected", http.StatusFound)
	}))
	defer target.Close()
	runtime := openTestKitSQLiteStore(t, ctx, "inline-redirect.sqlite")
	if err := runtime.ReplaceProfileCatalog(ctx, store.ProfileCatalog{
		ProfileID:      "sample",
		IndexedAt:      time.Now().UTC(),
		InterfaceNodes: []store.CatalogInterfaceNode{{ID: "node.redirect", Status: "active"}},
		APICases:       []store.CatalogAPICase{{ID: "case.redirect", NodeID: "node.redirect", Status: "active", BaseURL: target.URL}},
		TemplateConfigs: []store.CatalogTemplateConfig{{
			ID: "config.redirect", ScopeType: "case", ScopeID: "case.redirect", Status: "active",
			ConfigJSON: `{"caseExecution":{"method":"GET","nodeId":"node.redirect","path":"/start","expectedHttpCodes":[302]}}`,
		}},
	}); err != nil {
		t.Fatalf("replace redirect catalog: %v", err)
	}
	server := httptest.NewServer(controlplane.NewWithStore(profile.Bundle{ID: "bootstrap"}, runtime))
	defer server.Close()

	var result map[string]any
	postJSONInto(t, server.URL+"/api/test-kit/run", `{"caseId":"case.redirect"}`, http.StatusOK, &result)
	if result["ok"] != true || redirected {
		t.Fatalf("redirect execution = %#v redirected=%t", result, redirected)
	}
	response := result["result"].(map[string]any)["response"].(map[string]any)
	if response["statusCode"] != float64(http.StatusFound) {
		t.Fatalf("redirect response = %#v", response)
	}
}

func TestServerTestKitRejectsUnsafeTimeoutBeforeExecution(t *testing.T) {
	ctx := context.Background()
	var calls int
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		w.WriteHeader(http.StatusOK)
	}))
	defer target.Close()
	runtime := openTestKitSQLiteStore(t, ctx, "timeout-boundary.sqlite")
	if err := runtime.ReplaceProfileCatalog(ctx, store.ProfileCatalog{
		ProfileID:      "sample",
		IndexedAt:      time.Now().UTC(),
		InterfaceNodes: []store.CatalogInterfaceNode{{ID: "node.timeout", Status: "active"}},
		APICases:       []store.CatalogAPICase{{ID: "case.timeout", NodeID: "node.timeout", Status: "active", BaseURL: target.URL}},
		TemplateConfigs: []store.CatalogTemplateConfig{{
			ID: "config.timeout", ScopeType: "case", ScopeID: "case.timeout", Status: "active",
			ConfigJSON: `{"caseExecution":{"method":"GET","nodeId":"node.timeout","path":"/health","expectedHttpCodes":[200]}}`,
		}},
	}); err != nil {
		t.Fatalf("replace timeout catalog: %v", err)
	}
	server := httptest.NewServer(controlplane.NewWithStore(profile.Bundle{ID: "bootstrap"}, runtime))
	defer server.Close()

	for _, timeout := range []string{"-1", "601", "9223372036854775808", "1e100"} {
		var result map[string]any
		postJSONInto(t, server.URL+"/api/test-kit/run", `{"caseId":"case.timeout","timeoutSeconds":`+timeout+`}`, http.StatusBadRequest, &result)
		if result["ok"] != false || !strings.Contains(fmt.Sprint(result["error"]), "timeoutSeconds") {
			t.Fatalf("unsafe timeout %s result = %#v", timeout, result)
		}
	}
	var batchResult map[string]any
	postJSONInto(t, server.URL+"/api/test-kit/run-batch", `{"caseIds":["case.timeout"],"timeoutSeconds":601}`, http.StatusBadRequest, &batchResult)
	if batchResult["ok"] != false || !strings.Contains(fmt.Sprint(batchResult["error"]), "timeoutSeconds") {
		t.Fatalf("unsafe batch timeout result = %#v", batchResult)
	}
	if calls != 0 {
		t.Fatalf("unsafe timeout reached target %d times", calls)
	}
	if runs, err := runtime.ListRuns(ctx); err != nil || len(runs) != 0 {
		t.Fatalf("unsafe timeout persisted runs = %#v err=%v", runs, err)
	}

	var accepted map[string]any
	postJSONInto(t, server.URL+"/api/test-kit/run", `{"caseId":"case.timeout","timeoutSeconds":0}`, http.StatusOK, &accepted)
	if accepted["ok"] != true || calls != 1 {
		t.Fatalf("zero timeout default result = %#v calls=%d", accepted, calls)
	}
}

func TestServerTestKitInlineCaseRejectsOversizedResponse(t *testing.T) {
	ctx := context.Background()
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		chunk := bytes.Repeat([]byte("x"), 32<<10)
		for written := int64(0); written <= apicase.MaxResponseBodyBytes; written += int64(len(chunk)) {
			if _, err := w.Write(chunk); err != nil {
				return
			}
		}
	}))
	defer target.Close()
	runtime := openTestKitSQLiteStore(t, ctx, "inline-large-response.sqlite")
	if err := runtime.ReplaceProfileCatalog(ctx, store.ProfileCatalog{
		ProfileID:      "sample",
		IndexedAt:      time.Now().UTC(),
		InterfaceNodes: []store.CatalogInterfaceNode{{ID: "node.large-response", Status: "active"}},
		APICases:       []store.CatalogAPICase{{ID: "case.large-response", NodeID: "node.large-response", Status: "active", BaseURL: target.URL}},
		TemplateConfigs: []store.CatalogTemplateConfig{{
			ID: "config.large-response", ScopeType: "case", ScopeID: "case.large-response", Status: "active",
			ConfigJSON: `{"caseExecution":{"method":"GET","nodeId":"node.large-response","path":"/stream","expectedHttpCodes":[200]}}`,
		}},
	}); err != nil {
		t.Fatalf("replace large response catalog: %v", err)
	}
	server := httptest.NewServer(controlplane.NewWithStore(profile.Bundle{ID: "bootstrap"}, runtime))
	defer server.Close()

	var result map[string]any
	postJSONInto(t, server.URL+"/api/test-kit/run", `{"caseId":"case.large-response"}`, http.StatusOK, &result)
	if result["ok"] != false || !strings.Contains(fmt.Sprint(result["error"]), "exceeds") {
		t.Fatalf("inline oversized response result = %#v", result)
	}
	evidenceRoot := strings.TrimSpace(fmt.Sprint(result["evidenceRoot"]))
	errorEvidence := readTestKitEvidenceObject(t, filepath.Join(evidenceRoot, "error.json"))
	summary := readTestKitEvidenceObject(t, filepath.Join(evidenceRoot, "summary.json"))
	if errorEvidence["message"] != result["error"] || summary["error"] != result["error"] {
		t.Fatalf("inline oversized response Evidence: error=%#v summary=%#v", errorEvidence, summary)
	}
}

func newCatalogFileCaseTestServer(t *testing.T, ctx context.Context, storeName string, targetURL string, caseJSON string) (store.Store, string) {
	t.Helper()
	casePath := filepath.Join(t.TempDir(), "case.json")
	if err := os.WriteFile(casePath, []byte(caseJSON), 0o644); err != nil {
		t.Fatalf("write file case: %v", err)
	}
	runtime := openTestKitSQLiteStore(t, ctx, storeName)
	if err := runtime.ReplaceProfileCatalog(ctx, store.ProfileCatalog{
		ProfileID:      "sample",
		IndexedAt:      time.Now().UTC(),
		InterfaceNodes: []store.CatalogInterfaceNode{{ID: "node.file.failure", Status: "active"}},
		APICases: []store.CatalogAPICase{{
			ID: "case.file.failure", NodeID: "node.file.failure", Status: "active", CasePath: casePath,
			BaseURL: targetURL, EvidenceDir: filepath.Join(t.TempDir(), "evidence"),
		}},
	}); err != nil {
		t.Fatalf("replace file case catalog: %v", err)
	}
	server := httptest.NewServer(controlplane.NewWithStore(profile.Bundle{ID: "bootstrap"}, runtime))
	t.Cleanup(server.Close)
	return runtime, server.URL
}

func readTestKitEvidenceObject(t *testing.T, path string) map[string]any {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read test-kit Evidence %s: %v", filepath.Base(path), err)
	}
	var value map[string]any
	if err := json.Unmarshal(raw, &value); err != nil {
		t.Fatalf("decode test-kit Evidence %s: %v", filepath.Base(path), err)
	}
	return value
}

func requireTestKitEvidenceKinds(t *testing.T, ctx context.Context, runtime store.Store, runID string, expected ...string) {
	t.Helper()
	records, err := runtime.ListEvidence(ctx, runID)
	if err != nil {
		t.Fatalf("list test-kit Evidence: %v", err)
	}
	kinds := make(map[string]bool, len(records))
	for _, record := range records {
		kinds[record.Kind] = true
	}
	if len(kinds) != len(expected) {
		t.Fatalf("test-kit Evidence kinds = %v, want %v", kinds, expected)
	}
	for _, kind := range expected {
		if !kinds[kind] {
			t.Fatalf("test-kit Evidence kinds = %v, missing %s", kinds, kind)
		}
	}
}
