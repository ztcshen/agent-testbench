package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
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
	report := decodeCaseConfigUpsertSelectionReport(t, out)
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

func TestCaseConfigUpsertReusesAPICaseScopedExecutionConfig(t *testing.T) {
	storePath := filepath.Join(t.TempDir(), "store.sqlite")
	seedCaseConfigUpsertCatalogWithAPICaseScopedConfig(t, storePath)

	out := runCLI(t, "case", "config", "upsert",
		"--store", "sqlite://"+storePath,
		"--case", "case.generic.submit",
		"--method", "POST",
		"--path", "/generic/submit",
		"--header", "X-Trace={{ override:trace_id }}",
		"--expected-status", "200",
		"--json",
	)
	report := decodeCaseConfigUpsertSelectionReport(t, out)
	if !report.OK || report.Created || !report.Updated || report.Config.ID != "config.api-case.generic.submit" || !report.SelectedByRunner {
		t.Fatalf("api-case scoped config should be updated in place: %#v", report)
	}
}

func TestCaseConfigUpsertPersistsDefaultOverridesAndWorkflowIO(t *testing.T) {
	storePath := filepath.Join(t.TempDir(), "store.sqlite")
	seedCaseConfigUpsertWorkflowCatalog(t, storePath)

	out := runCLI(t, "case", "config", "upsert",
		"--store", "sqlite://"+storePath,
		"--case", "case.generic.prepare",
		"--method", "POST",
		"--path", "/generic/prepare",
		"--body-json", `{"executorParam":"{{ override:executorParam }}"}`,
		"--default-override", "executorParam=sample-runner",
		"--exports-json", `[{"name":"transaction_id","from":"responseBody","path":"transaction_id"}]`,
		"--expected-status", "200",
		"--json",
	)
	var report struct {
		OK               bool           `json:"ok"`
		DefaultOverrides map[string]any `json:"defaultOverrides"`
	}
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatalf("decode case config upsert report: %v\n%s", err, out)
	}
	if !report.OK || report.DefaultOverrides["executorParam"] != "sample-runner" {
		t.Fatalf("case config upsert default overrides = %#v", report)
	}

	runCLI(t, "case", "config", "upsert",
		"--store", "sqlite://"+storePath,
		"--case", "case.generic.callback",
		"--method", "POST",
		"--path", "/generic/callback",
		"--body-json", `{"transactionId":"{{ override:transaction_id }}"}`,
		"--inputs-json", `[{"name":"transaction_id","source":"previous","required":true}]`,
		"--expected-status", "200",
		"--json",
	)

	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read request body: %v", err)
		}
		if r.Method != http.MethodPost || r.URL.Path != "/generic/prepare" || !strings.Contains(string(body), `"executorParam":"sample-runner"`) {
			t.Fatalf("unexpected rendered request: %s %s body=%s", r.Method, r.URL.Path, body)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"transaction_id":"tx-001"}`)
	}))
	defer target.Close()

	runOut := runCLI(t, "case", "run",
		"--store", "sqlite://"+storePath,
		"--case-id", "case.generic.prepare",
		"--base-url", target.URL,
		"--run-id", "run.case-config-default-override",
		"--json",
	)
	if !strings.Contains(runOut, `"status": "passed"`) {
		t.Fatalf("store-backed default override should pass run:\n%s", runOut)
	}

	auditOut := runCLI(t, "workflow", "audit", "--store", "sqlite://"+storePath, "--workflow", "workflow.generic", "--json")
	var audit struct {
		OK         bool `json:"ok"`
		IssueCount int  `json:"issueCount"`
	}
	if err := json.Unmarshal([]byte(auditOut), &audit); err != nil {
		t.Fatalf("decode workflow audit json: %v\n%s", err, auditOut)
	}
	if !audit.OK || audit.IssueCount != 0 {
		t.Fatalf("workflow audit should see persisted exports/inputs: %#v\n%s", audit, auditOut)
	}

	ctx := context.Background()
	s, err := sqlite.Open(ctx, sqlite.Config{Path: storePath})
	if err != nil {
		t.Fatalf("open sqlite store: %v", err)
	}
	defer s.Close()
	catalog, err := s.GetProfileCatalog(ctx)
	if err != nil {
		t.Fatalf("get profile catalog: %v", err)
	}
	apiCase, ok := findCatalogAPICase(catalog.APICases, "case.generic.prepare")
	if !ok || apiCase.DefaultOverridesJSON != `{"executorParam":"sample-runner"}` {
		t.Fatalf("persisted api case default overrides = %#v", apiCase)
	}
}

func TestCaseConfigUpsertUpdatesWorkflowStepExecutionConfigForAudit(t *testing.T) {
	storePath := filepath.Join(t.TempDir(), "step-config.sqlite")
	seedCaseConfigUpsertWorkflowStepCatalog(t, storePath)
	storeRef := "sqlite://" + storePath

	auditOut := runCLI(t, "workflow", "audit", "--store", storeRef, "--workflow", "workflow.generic", "--json")
	var before struct {
		OK         bool `json:"ok"`
		IssueCount int  `json:"issueCount"`
	}
	if err := json.Unmarshal([]byte(auditOut), &before); err != nil {
		t.Fatalf("decode workflow audit before upsert: %v\n%s", err, auditOut)
	}
	if before.OK || before.IssueCount != 1 {
		t.Fatalf("workflow audit should start with missing step export: %#v\n%s", before, auditOut)
	}

	out := runCLI(t, "case", "config", "upsert",
		"--store", storeRef,
		"--case", "case.generic.prepare",
		"--workflow", "workflow.generic",
		"--step", "prepare",
		"--exports-json", `[{"name":"transaction_id","from":"responseBody","path":"transaction_id"}]`,
		"--json",
	)
	var report struct {
		OK      bool `json:"ok"`
		Updated bool `json:"updated"`
		Config  struct {
			ID         string `json:"id"`
			ScopeType  string `json:"scopeType"`
			WorkflowID string `json:"workflowId"`
			StepID     string `json:"stepId"`
		} `json:"config"`
		SelectedByRunner bool `json:"selectedByRunner"`
	}
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatalf("decode step config upsert report: %v\n%s", err, out)
	}
	if !report.OK || !report.Updated || report.Config.ID != "cfg.workflow-step.workflow.generic.prepare" ||
		report.Config.ScopeType != "step" || report.Config.WorkflowID != "workflow.generic" || report.Config.StepID != "prepare" || !report.SelectedByRunner {
		t.Fatalf("step config upsert report = %#v", report)
	}

	auditOut = runCLI(t, "workflow", "audit", "--store", storeRef, "--workflow", "workflow.generic", "--json")
	var after struct {
		OK         bool `json:"ok"`
		IssueCount int  `json:"issueCount"`
	}
	if err := json.Unmarshal([]byte(auditOut), &after); err != nil {
		t.Fatalf("decode workflow audit after upsert: %v\n%s", err, auditOut)
	}
	if !after.OK || after.IssueCount != 0 {
		t.Fatalf("workflow audit should see persisted step exports: %#v\n%s", after, auditOut)
	}
}

func TestCaseConfigUpsertCreatesStepConfigWithoutRescopingGenericConfig(t *testing.T) {
	ctx := context.Background()
	storePath := filepath.Join(t.TempDir(), "step-new-config.sqlite")
	storeRef := "sqlite://" + storePath
	genericConfigID := "config.case.generic.prepare"
	replaceCaseConfigUpsertCatalog(t, storePath, caseConfigWorkflowCatalog("/generic",
		[]store.CatalogTemplateConfig{{
			ID:         genericConfigID,
			ScopeType:  "case",
			ScopeID:    "case.generic.prepare",
			Status:     "active",
			ConfigJSON: `{"caseId":"case.generic.prepare","caseExecution":{"method":"POST","nodeId":"node.generic","path":"/generic/prepare"}}`,
		}},
	))

	out := runCLI(t, "case", "config", "upsert",
		"--store", storeRef,
		"--case", "case.generic.prepare",
		"--workflow", "workflow.generic",
		"--step", "prepare",
		"--exports-json", `[{"name":"transaction_id","from":"responseBody","path":"transaction_id"}]`,
		"--json",
	)
	var report struct {
		OK      bool `json:"ok"`
		Created bool `json:"created"`
		Config  struct {
			ID        string `json:"id"`
			ScopeType string `json:"scopeType"`
			StepID    string `json:"stepId"`
		} `json:"config"`
		SelectedByRunner bool `json:"selectedByRunner"`
	}
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatalf("decode step config create report: %v\n%s", err, out)
	}
	if !report.OK || !report.Created || report.Config.ID == genericConfigID || report.Config.ScopeType != "step" || report.Config.StepID != "prepare" || !report.SelectedByRunner {
		t.Fatalf("step config create report = %#v", report)
	}

	s, err := sqlite.Open(ctx, sqlite.Config{Path: storePath})
	if err != nil {
		t.Fatalf("open sqlite store: %v", err)
	}
	defer s.Close()
	catalog, err := s.GetProfileCatalog(ctx)
	if err != nil {
		t.Fatalf("get catalog: %v", err)
	}
	genericConfig, ok := findCatalogTemplateConfig(catalog.TemplateConfigs, genericConfigID)
	if !ok || genericConfig.ScopeType != "case" || genericConfig.ScopeID != "case.generic.prepare" {
		t.Fatalf("generic config should remain case scoped: %#v", genericConfig)
	}
	stepConfig, ok := findCatalogTemplateConfig(catalog.TemplateConfigs, report.Config.ID)
	if !ok || stepConfig.ScopeType != "step" || stepConfig.ScopeID != "prepare" || stepConfig.WorkflowID != "workflow.generic" {
		t.Fatalf("step config should be separate: %#v", stepConfig)
	}
}

func TestCaseConfigUpsertRejectsMismatchedWorkflowStepCase(t *testing.T) {
	storePath := filepath.Join(t.TempDir(), "step-mismatch.sqlite")
	seedCaseConfigUpsertWorkflowStepCatalog(t, storePath)
	out := runCLIFails(t, "case", "config", "upsert",
		"--store", "sqlite://"+storePath,
		"--case", "case.generic.prepare",
		"--workflow", "workflow.generic",
		"--step", "callback",
		"--exports-json", `[{"name":"transaction_id","from":"responseBody","path":"transaction_id"}]`,
		"--json",
	)
	if !strings.Contains(out, "bound to case case.generic.callback, not case.generic.prepare") {
		t.Fatalf("unexpected mismatched step error:\n%s", out)
	}
}

func TestCaseConfigStepDefaultIDsAvoidTupleCollisions(t *testing.T) {
	left := defaultCaseExecutionTemplateConfigID("case.left", "workflow.flow", "create.submit")
	right := defaultCaseExecutionTemplateConfigID("case.right", "workflow.flow.create", "submit")
	if left == right {
		t.Fatalf("step config default IDs should be collision-free: %q", left)
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

type caseConfigUpsertSelectionReport struct {
	OK      bool `json:"ok"`
	Created bool `json:"created"`
	Updated bool `json:"updated"`
	Config  struct {
		ID string `json:"id"`
	} `json:"config"`
	SelectedByRunner bool `json:"selectedByRunner"`
}

func decodeCaseConfigUpsertSelectionReport(t *testing.T, out string) caseConfigUpsertSelectionReport {
	t.Helper()
	var report caseConfigUpsertSelectionReport
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatalf("decode case config upsert report: %v\n%s", err, out)
	}
	return report
}

func replaceCaseConfigUpsertCatalog(t *testing.T, storePath string, catalog store.ProfileCatalog) {
	t.Helper()
	ctx := context.Background()
	s, err := sqlite.Open(ctx, sqlite.Config{Path: storePath})
	if err != nil {
		t.Fatalf("open sqlite store: %v", err)
	}
	defer s.Close()
	if err := s.ReplaceProfileCatalog(ctx, catalog); err != nil {
		t.Fatalf("replace profile catalog: %v", err)
	}
}

func seedCaseConfigUpsertWorkflowCatalog(t *testing.T, storePath string) {
	t.Helper()
	replaceCaseConfigUpsertCatalog(t, storePath, caseConfigWorkflowCatalog("/generic/prepare", nil))
}

func seedCaseConfigUpsertWorkflowStepCatalog(t *testing.T, storePath string) {
	t.Helper()
	replaceCaseConfigUpsertCatalog(t, storePath, caseConfigWorkflowCatalog("/generic",
		[]store.CatalogTemplateConfig{
			{
				ID:         "cfg.workflow-step.workflow.generic.prepare",
				WorkflowID: "workflow.generic",
				ScopeType:  "step",
				ScopeID:    "prepare",
				Status:     "active",
				ConfigJSON: `{"caseId":"case.generic.prepare","caseExecution":{"method":"POST","nodeId":"node.generic","path":"/generic/prepare"}}`,
			},
			{
				ID:         "cfg.workflow-step.workflow.generic.callback",
				WorkflowID: "workflow.generic",
				ScopeType:  "step",
				ScopeID:    "callback",
				Status:     "active",
				ConfigJSON: `{"caseId":"case.generic.callback","caseExecution":{"method":"POST","nodeId":"node.generic","path":"/generic/callback"},"inputs":[{"name":"transaction_id","source":"previous","required":true}]}`,
			},
		},
	))
}

func seedCaseConfigUpsertCatalog(t *testing.T, storePath string) {
	t.Helper()
	replaceCaseConfigUpsertCatalog(t, storePath, caseConfigSubmitCatalog(nil))
}

func seedCaseConfigUpsertCatalogWithAPICaseScopedConfig(t *testing.T, storePath string) {
	t.Helper()
	replaceCaseConfigUpsertCatalog(t, storePath, caseConfigSubmitCatalog([]store.CatalogTemplateConfig{{
		ID:         "config.api-case.generic.submit",
		ScopeType:  "api-case",
		ScopeID:    "case.generic.submit",
		Status:     "active",
		ConfigJSON: `{"caseId":"case.generic.submit","caseExecution":{"method":"POST","nodeId":"node.generic","path":"/generic/submit"}}`,
	}}))
}

func seedCaseConfigUpsertCatalogWithBaseConfig(t *testing.T, storePath string) {
	t.Helper()
	replaceCaseConfigUpsertCatalog(t, storePath, caseConfigSubmitCatalog([]store.CatalogTemplateConfig{{
		ID:         "config.case.generic.submit",
		ScopeType:  "case",
		ScopeID:    "case.generic.submit",
		Status:     "active",
		ConfigJSON: `{"caseId":"case.generic.submit","caseExecution":{"method":"POST","nodeId":"node.generic","path":"/generic/submit"}}`,
	}}))
}

func caseConfigSubmitCatalog(templateConfigs []store.CatalogTemplateConfig) store.ProfileCatalog {
	return store.ProfileCatalog{
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
		TemplateConfigs: templateConfigs,
	}
}

func caseConfigWorkflowCatalog(interfacePath string, templateConfigs []store.CatalogTemplateConfig) store.ProfileCatalog {
	return store.ProfileCatalog{
		ProfileID: "default",
		IndexedAt: time.Now().UTC(),
		InterfaceNodes: []store.CatalogInterfaceNode{{
			ID:        "node.generic",
			ServiceID: "service.generic",
			Method:    "POST",
			Path:      interfacePath,
			Status:    "active",
		}},
		Workflows: []store.CatalogWorkflow{{
			ID:          "workflow.generic",
			DisplayName: "Generic Flow",
		}},
		WorkflowBindings: []store.CatalogWorkflowBinding{
			{WorkflowID: "workflow.generic", StepID: "prepare", NodeID: "node.generic", CaseID: "case.generic.prepare", Required: true, SortOrder: 1},
			{WorkflowID: "workflow.generic", StepID: "callback", NodeID: "node.generic", CaseID: "case.generic.callback", Required: true, SortOrder: 2},
		},
		APICases: []store.CatalogAPICase{
			{ID: "case.generic.prepare", DisplayName: "Generic Prepare", NodeID: "node.generic", Status: "active", SortOrder: 1},
			{ID: "case.generic.callback", DisplayName: "Generic Callback", NodeID: "node.generic", Status: "active", SortOrder: 2},
		},
		TemplateConfigs: templateConfigs,
	}
}
