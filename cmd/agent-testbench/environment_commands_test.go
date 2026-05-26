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
)

func TestEnvironmentCommandsAcceptActiveSQLiteStore(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("AGENT_TESTBENCH_CONFIG_HOME", filepath.Join(dir, "config"))
	if err := saveStoreConfig(storeConfigFile{
		Active: "local-sqlite",
		Stores: map[string]storeConfigEntry{
			"local-sqlite": {Name: "local-sqlite", URL: "sqlite://" + filepath.Join(dir, "store.sqlite"), Backend: "sqlite"},
		},
	}); err != nil {
		t.Fatalf("save store config: %v", err)
	}

	if err := runEnvironment(context.Background(), []string{"register", "--id", "env.sqlite", "--verification-workflow", "workflow.sqlite"}); err != nil {
		t.Fatalf("register with active SQLite Store: %v", err)
	}
	if err := runEnvironment(context.Background(), []string{"discover", "--json"}); err != nil {
		t.Fatalf("discover with active SQLite Store: %v", err)
	}
}

func TestEnvironmentRegisterRequiresVerificationWorkflow(t *testing.T) {
	storePath := filepath.Join(t.TempDir(), "store.sqlite")
	out := runCLIFails(t, "environment", "register",
		"--store", "sqlite://"+storePath,
		"--id", "env.no-workflow",
		"--repo", "entry-gateway=https://example.com/team/entry-gateway.git",
	)
	if !strings.Contains(out, "--verification-workflow") {
		t.Fatalf("register without verification workflow output = %q", out)
	}
}

func TestEnvironmentRegisterRejectsOversizedDefinitionMetadata(t *testing.T) {
	storePath := filepath.Join(t.TempDir(), "store.sqlite")
	large := strings.Repeat("x", store.EnvironmentDefinitionMaxBytes)
	err := runEnvironment(context.Background(), []string{"register",
		"--store", "sqlite://" + storePath,
		"--id", "env.too-large",
		"--description", large,
		"--verification-workflow", "workflow.core-10",
	})
	if err == nil {
		t.Fatal("expected oversized environment metadata to be rejected")
	}
	got := err.Error()
	if !strings.Contains(got, "write blocked") || !strings.Contains(got, fmt.Sprintf("1 MB safety boundary is %d bytes", store.EnvironmentDefinitionMaxBytes)) || !strings.Contains(got, "Reason:") || !strings.Contains(got, "largest contributor") {
		t.Fatalf("oversized environment metadata error = %q", got)
	}
}

func TestEnvironmentCommandsGateVerifiedDiscovery(t *testing.T) {
	storePath := filepath.Join(t.TempDir(), "store.sqlite")
	storeRef := "sqlite://" + storePath

	registerOut := runCLI(t, "environment", "register",
		"--store", storeRef,
		"--id", "env.team.verified",
		"--display-name", "Team Verified Environment",
		"--description", "Accepted local Docker environment",
		"--service", "entry-gateway",
		"--repo", "entry-gateway=../entry-gateway",
		"--branch", "entry-gateway=main",
		"--repo-ref", "entry-gateway=v1.2.3",
		"--checkout", "entry-gateway=/tmp/entry-gateway",
		"--compose-file", "docker-compose.yml",
		"--start-command", "docker compose up -d",
		"--health-url", newHealthyTestURL(t),
		"--verification-workflow", "workflow.core-10",
		"--json",
	)
	var registered struct {
		OK          bool `json:"ok"`
		Environment struct {
			ID                     string           `json:"id"`
			Status                 string           `json:"status"`
			Verified               bool             `json:"verified"`
			VerificationWorkflowID string           `json:"verificationWorkflowId"`
			Services               []map[string]any `json:"services"`
			Repos                  map[string]any   `json:"repos"`
		} `json:"environment"`
	}
	if err := json.Unmarshal([]byte(registerOut), &registered); err != nil {
		t.Fatalf("decode environment register json: %v\n%s", err, registerOut)
	}
	if !registered.OK || registered.Environment.ID != "env.team.verified" || registered.Environment.Status != "draft" || registered.Environment.Verified {
		t.Fatalf("registered environment = %#v", registered.Environment)
	}
	if registered.Environment.VerificationWorkflowID != "workflow.core-10" || len(registered.Environment.Services) != 1 || registered.Environment.Repos["entry-gateway"] == nil {
		t.Fatalf("registered environment catalog fields = %#v", registered.Environment)
	}

	repoSetOut := runCLI(t, "environment", "repo", "set",
		"--store", storeRef,
		"--repo-ref", "entry-gateway=v1.2.4",
		"--checkout", "entry-gateway=entry-gateway",
		"--json",
		"env.team.verified",
	)
	var repoSet struct {
		OK          bool `json:"ok"`
		Environment struct {
			VerificationWorkflowID string           `json:"verificationWorkflowId"`
			Services               []map[string]any `json:"services"`
			Repos                  map[string]any   `json:"repos"`
		} `json:"environment"`
	}
	if err := json.Unmarshal([]byte(repoSetOut), &repoSet); err != nil {
		t.Fatalf("decode environment repo set json: %v\n%s", err, repoSetOut)
	}
	entryRepo, _ := repoSet.Environment.Repos["entry-gateway"].(map[string]any)
	if !repoSet.OK || repoSet.Environment.VerificationWorkflowID != "workflow.core-10" || entryRepo["ref"] != "v1.2.4" || entryRepo["checkout"] != "entry-gateway" {
		t.Fatalf("repo set environment = %#v", repoSet.Environment)
	}
	if len(repoSet.Environment.Services) != 1 || repoSet.Environment.Services[0]["ref"] != "v1.2.4" || repoSet.Environment.Services[0]["checkout"] != "entry-gateway" {
		t.Fatalf("repo set services = %#v", repoSet.Environment.Services)
	}

	discoverOut := runCLI(t, "environment", "discover", "--store", storeRef, "--json")
	var discovered struct {
		Count int `json:"count"`
	}
	if err := json.Unmarshal([]byte(discoverOut), &discovered); err != nil {
		t.Fatalf("decode discover json: %v\n%s", err, discoverOut)
	}
	if discovered.Count != 0 {
		t.Fatalf("unverified environment should stay out of default discovery: %#v", discovered)
	}

	discoverAllOut := runCLI(t, "environment", "discover", "--store", storeRef, "--all", "--json")
	var discoveredAll struct {
		Count int `json:"count"`
		Items []struct {
			ID string `json:"id"`
		} `json:"items"`
	}
	if err := json.Unmarshal([]byte(discoverAllOut), &discoveredAll); err != nil {
		t.Fatalf("decode discover all json: %v\n%s", err, discoverAllOut)
	}
	if discoveredAll.Count != 1 || discoveredAll.Items[0].ID != "env.team.verified" {
		t.Fatalf("discover all = %#v", discoveredAll)
	}

	publishDenied := runCLIFails(t, "environment", "publish-verified", "--store", storeRef, "env.team.verified")
	if !strings.Contains(publishDenied, "not publishable") {
		t.Fatalf("publish should require complete verification evidence: %q", publishDenied)
	}

	verifyOut := runCLI(t, "environment", "verify",
		"env.team.verified",
		"--store", storeRef,
		"--run", "run.core-10",
		"--status", "passed",
		"--evidence-complete",
		"--topology-complete",
		"--json",
	)
	var verified struct {
		Environment struct {
			Status                 string `json:"status"`
			LastVerificationRunID  string `json:"lastVerificationRunId"`
			LastVerificationStatus string `json:"lastVerificationStatus"`
			EvidenceComplete       bool   `json:"evidenceComplete"`
			TopologyComplete       bool   `json:"topologyComplete"`
		} `json:"environment"`
	}
	if err := json.Unmarshal([]byte(verifyOut), &verified); err != nil {
		t.Fatalf("decode verify json: %v\n%s", err, verifyOut)
	}
	if verified.Environment.Status != "verified-ready" || verified.Environment.LastVerificationRunID != "run.core-10" || verified.Environment.LastVerificationStatus != "passed" || !verified.Environment.EvidenceComplete || !verified.Environment.TopologyComplete {
		t.Fatalf("verified environment = %#v", verified.Environment)
	}

	missingArtifacts := runCLIFails(t, "environment", "publish-verified", "--store", storeRef, "env.team.verified")
	if !strings.Contains(missingArtifacts, "was not found in Store") {
		t.Fatalf("publish should require indexed verification artifacts: %q", missingArtifacts)
	}
	seedEnvironmentVerificationArtifacts(t, storeRef, "run.core-10")

	publishOut := runCLI(t, "environment", "publish-verified", "env.team.verified", "--store", storeRef, "--json")
	var published struct {
		Environment struct {
			Status   string `json:"status"`
			Verified bool   `json:"verified"`
		} `json:"environment"`
	}
	if err := json.Unmarshal([]byte(publishOut), &published); err != nil {
		t.Fatalf("decode publish json: %v\n%s", err, publishOut)
	}
	if published.Environment.Status != "verified" || !published.Environment.Verified {
		t.Fatalf("published environment = %#v", published.Environment)
	}

	discoverVerifiedOut := runCLI(t, "environment", "discover", "--store", storeRef, "--json")
	var discoveredVerified struct {
		Count int `json:"count"`
		Items []struct {
			ID       string `json:"id"`
			Verified bool   `json:"verified"`
		} `json:"items"`
	}
	if err := json.Unmarshal([]byte(discoverVerifiedOut), &discoveredVerified); err != nil {
		t.Fatalf("decode verified discover json: %v\n%s", err, discoverVerifiedOut)
	}
	if discoveredVerified.Count != 1 || discoveredVerified.Items[0].ID != "env.team.verified" || !discoveredVerified.Items[0].Verified {
		t.Fatalf("verified discovery = %#v", discoveredVerified)
	}

	bootstrapOut := runCLI(t, "environment", "bootstrap", "--store", storeRef, "--json", "env.team.verified")
	var bootstrap struct {
		Plan struct {
			VerificationWorkflow string         `json:"verificationWorkflow"`
			Repos                map[string]any `json:"repos"`
			HealthChecks         []any          `json:"healthChecks"`
			Restore              struct {
				PauseBeforeHeavyValidation bool `json:"pauseBeforeHeavyValidation"`
				Docker                     struct {
					Action   string     `json:"action"`
					Commands [][]string `json:"commands"`
				} `json:"docker"`
			} `json:"restore"`
			Steps []struct {
				Kind string `json:"kind"`
			} `json:"steps"`
		} `json:"plan"`
	}
	if err := json.Unmarshal([]byte(bootstrapOut), &bootstrap); err != nil {
		t.Fatalf("decode bootstrap json: %v\n%s", err, bootstrapOut)
	}
	if bootstrap.Plan.VerificationWorkflow != "workflow.core-10" || bootstrap.Plan.Repos["entry-gateway"] == nil || len(bootstrap.Plan.HealthChecks) != 1 {
		t.Fatalf("bootstrap plan = %#v", bootstrap.Plan)
	}
	if repo, ok := bootstrap.Plan.Repos["entry-gateway"].(map[string]any); !ok || repo["ref"] != "v1.2.4" {
		t.Fatalf("bootstrap repo ref = %#v", bootstrap.Plan.Repos["entry-gateway"])
	}
	if !bootstrap.Plan.Restore.PauseBeforeHeavyValidation || bootstrap.Plan.Restore.Docker.Action != "docker-compose" || len(bootstrap.Plan.Restore.Docker.Commands) != 3 {
		t.Fatalf("bootstrap restore plan = %#v", bootstrap.Plan.Restore)
	}
	if len(bootstrap.Plan.Steps) != 4 || bootstrap.Plan.Steps[0].Kind != "repository" || bootstrap.Plan.Steps[1].Kind != "docker" {
		t.Fatalf("bootstrap executable steps = %#v", bootstrap.Plan.Steps)
	}
}

func TestWorkflowAcceptanceCLIStartsAndReadsAsyncReport(t *testing.T) {
	var startPayload map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/cases/batch-runs":
			if err := json.NewDecoder(r.Body).Decode(&startPayload); err != nil {
				t.Fatalf("decode start payload: %v", err)
			}
			writeTestJSON(t, w, http.StatusAccepted, map[string]any{
				"ok":         true,
				"batchRunId": "batch.acceptance.001",
				"requestId":  "acceptance-001",
				"workflowId": "workflow.core-10",
				"status":     "running",
				"total":      10,
				"reportUrl":  "/api/cases/batch-runs/batch.acceptance.001",
			})
		case r.Method == http.MethodGet && r.URL.Path == "/api/cases/batch-runs/batch.acceptance.001":
			writeTestJSON(t, w, http.StatusOK, map[string]any{
				"ok":         true,
				"batchRunId": "batch.acceptance.001",
				"workflowId": "workflow.core-10",
				"status":     "passed",
				"total":      10,
				"acceptance": map[string]any{
					"ok":               true,
					"templateId":       "environment.workflow.skywalking.v1",
					"workflowId":       "workflow.core-10",
					"expectedSteps":    10,
					"completedSteps":   10,
					"passedSteps":      10,
					"failedSteps":      0,
					"topologyProvider": "skywalking",
				},
			})
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	startOut := runCLI(t, "workflow", "acceptance", "start",
		"--server-url", server.URL,
		"--workflow", "workflow.core-10",
		"--request-id", "acceptance-001",
		"--base-url", "http://127.0.0.1:18080",
		"--timeout-seconds", "30",
		"--json",
	)
	var started struct {
		OK         bool   `json:"ok"`
		BatchRunID string `json:"batchRunId"`
		WorkflowID string `json:"workflowId"`
		Status     string `json:"status"`
	}
	if err := json.Unmarshal([]byte(startOut), &started); err != nil {
		t.Fatalf("decode workflow acceptance start: %v\n%s", err, startOut)
	}
	if !started.OK || started.BatchRunID != "batch.acceptance.001" || started.WorkflowID != "workflow.core-10" || started.Status != "running" {
		t.Fatalf("workflow acceptance start = %#v", started)
	}
	if startPayload["workflowId"] != "workflow.core-10" || startPayload["requestId"] != "acceptance-001" || startPayload["baseUrl"] != "http://127.0.0.1:18080" || startPayload["timeoutSeconds"] != float64(30) {
		t.Fatalf("workflow acceptance start payload = %#v", startPayload)
	}

	reportOut := runCLI(t, "workflow", "acceptance", "report",
		"--server-url", server.URL,
		"--run", "batch.acceptance.001",
		"--json",
	)
	var report struct {
		Acceptance struct {
			OK               bool   `json:"ok"`
			TemplateID       string `json:"templateId"`
			TopologyProvider string `json:"topologyProvider"`
		} `json:"acceptance"`
	}
	if err := json.Unmarshal([]byte(reportOut), &report); err != nil {
		t.Fatalf("decode workflow acceptance report: %v\n%s", err, reportOut)
	}
	if !report.Acceptance.OK || report.Acceptance.TemplateID != "environment.workflow.skywalking.v1" || report.Acceptance.TopologyProvider != "skywalking" {
		t.Fatalf("workflow acceptance report = %#v", report.Acceptance)
	}
}

func TestCaseBatchCLIStartsAndReadsAsyncReport(t *testing.T) {
	var startPayload map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/cases/batch-runs":
			if err := json.NewDecoder(r.Body).Decode(&startPayload); err != nil {
				t.Fatalf("decode start payload: %v", err)
			}
			writeTestJSON(t, w, http.StatusAccepted, map[string]any{
				"ok":         true,
				"batchRunId": "batch.case.001",
				"requestId":  "case-batch-001",
				"status":     "running",
				"total":      2,
				"reportUrl":  "/api/cases/batch-runs/batch.case.001",
			})
		case r.Method == http.MethodGet && r.URL.Path == "/api/cases/batch-runs/batch.case.001":
			writeTestJSON(t, w, http.StatusOK, map[string]any{
				"ok":         true,
				"batchRunId": "batch.case.001",
				"status":     "passed",
				"total":      2,
				"passed":     2,
				"failed":     0,
			})
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	startOut := runCLI(t, "case", "batch", "start",
		"--server-url", server.URL,
		"--case", "case.alpha",
		"--case", "case.beta",
		"--request-id", "case-batch-001",
		"--base-url", "http://127.0.0.1:18080",
		"--timeout-seconds", "30",
		"--json",
	)
	var started struct {
		OK         bool   `json:"ok"`
		BatchRunID string `json:"batchRunId"`
		Status     string `json:"status"`
		Total      int    `json:"total"`
	}
	if err := json.Unmarshal([]byte(startOut), &started); err != nil {
		t.Fatalf("decode case batch start: %v\n%s", err, startOut)
	}
	if !started.OK || started.BatchRunID != "batch.case.001" || started.Status != "running" || started.Total != 2 {
		t.Fatalf("case batch start = %#v", started)
	}
	caseIDs, _ := startPayload["caseIds"].([]any)
	if len(caseIDs) != 2 || caseIDs[0] != "case.alpha" || caseIDs[1] != "case.beta" || startPayload["requestId"] != "case-batch-001" || startPayload["baseUrl"] != "http://127.0.0.1:18080" || startPayload["timeoutSeconds"] != float64(30) {
		t.Fatalf("case batch start payload = %#v", startPayload)
	}

	reportOut := runCLI(t, "case", "batch", "report",
		"--server-url", server.URL,
		"--run", "batch.case.001",
		"--json",
	)
	var report struct {
		OK     bool   `json:"ok"`
		Status string `json:"status"`
		Total  int    `json:"total"`
		Passed int    `json:"passed"`
		Failed int    `json:"failed"`
	}
	if err := json.Unmarshal([]byte(reportOut), &report); err != nil {
		t.Fatalf("decode case batch report: %v\n%s", err, reportOut)
	}
	if !report.OK || report.Status != "passed" || report.Total != 2 || report.Passed != 2 || report.Failed != 0 {
		t.Fatalf("case batch report = %#v", report)
	}
}

func TestEnvironmentAcceptanceCLIStartsAndReadsAsyncReport(t *testing.T) {
	var startPayload map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/environments/env.team/acceptance-runs":
			if err := json.NewDecoder(r.Body).Decode(&startPayload); err != nil {
				t.Fatalf("decode environment start payload: %v", err)
			}
			writeTestJSON(t, w, http.StatusAccepted, map[string]any{
				"ok":            true,
				"environmentId": "env.team",
				"batchRunId":    "batch.env.acceptance.001",
				"requestId":     "env-acceptance-001",
				"workflowId":    "workflow.core-10",
				"status":        "running",
				"reportUrl":     "/api/environments/env.team/acceptance-runs/batch.env.acceptance.001",
			})
		case r.Method == http.MethodGet && r.URL.Path == "/api/environments/env.team/acceptance-runs/batch.env.acceptance.001":
			writeTestJSON(t, w, http.StatusOK, map[string]any{
				"ok":            true,
				"environmentId": "env.team",
				"batchRunId":    "batch.env.acceptance.001",
				"workflowId":    "workflow.core-10",
				"status":        "passed",
				"acceptance": map[string]any{
					"ok":               true,
					"templateId":       "environment.workflow.skywalking.v1",
					"workflowId":       "workflow.core-10",
					"topologyProvider": "skywalking",
					"healthSummary":    map[string]any{"total": 1, "passed": 1, "failed": 0},
				},
			})
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	startOut := runCLI(t, "environment", "acceptance", "start",
		"--server-url", server.URL,
		"--request-id", "env-acceptance-001",
		"--base-url", "http://127.0.0.1:18080",
		"--json",
		"env.team",
	)
	var started struct {
		OK            bool   `json:"ok"`
		EnvironmentID string `json:"environmentId"`
		BatchRunID    string `json:"batchRunId"`
		WorkflowID    string `json:"workflowId"`
	}
	if err := json.Unmarshal([]byte(startOut), &started); err != nil {
		t.Fatalf("decode environment acceptance start: %v\n%s", err, startOut)
	}
	if !started.OK || started.EnvironmentID != "env.team" || started.BatchRunID != "batch.env.acceptance.001" || started.WorkflowID != "workflow.core-10" {
		t.Fatalf("environment acceptance start = %#v", started)
	}
	if startPayload["requestId"] != "env-acceptance-001" || startPayload["baseUrl"] != "http://127.0.0.1:18080" {
		t.Fatalf("environment acceptance start payload = %#v", startPayload)
	}

	reportOut := runCLI(t, "environment", "acceptance", "report",
		"--server-url", server.URL,
		"--run", "batch.env.acceptance.001",
		"--json",
		"env.team",
	)
	var report struct {
		Acceptance struct {
			OK            bool `json:"ok"`
			HealthSummary struct {
				Total  int `json:"total"`
				Passed int `json:"passed"`
			} `json:"healthSummary"`
		} `json:"acceptance"`
	}
	if err := json.Unmarshal([]byte(reportOut), &report); err != nil {
		t.Fatalf("decode environment acceptance report: %v\n%s", err, reportOut)
	}
	if !report.Acceptance.OK || report.Acceptance.HealthSummary.Total != 1 || report.Acceptance.HealthSummary.Passed != 1 {
		t.Fatalf("environment acceptance report = %#v", report.Acceptance)
	}
}

func TestEnvironmentCommandsUseNamedPostgreSQLActiveStore(t *testing.T) {
	storeRef := configureNamedPostgreSQLActiveStore(t, "daily-environment-pg")
	runEnvironmentCommandsUseNamedActiveStore(t, storeRef, "env.team.pg", "PostgreSQL")
}

func TestEnvironmentCommandsUseNamedMySQLActiveStore(t *testing.T) {
	storeRef := configureNamedMySQLActiveStore(t, "daily-environment-mysql")
	runEnvironmentCommandsUseNamedActiveStore(t, storeRef, "env.team.mysql", "MySQL")
}

func runEnvironmentCommandsUseNamedActiveStore(t *testing.T, storeRef string, envID string, label string) {
	t.Helper()
	runID := "run.core-10." + time.Now().UTC().Format("20060102150405.000000000")

	registerOut := runCLI(t, "environment", "register",
		"--id", envID,
		"--display-name", "Team "+label+" Environment",
		"--description", "Accepted local Docker environment",
		"--service", "entry-gateway",
		"--repo", "entry-gateway=../entry-gateway",
		"--branch", "entry-gateway=main",
		"--checkout", "entry-gateway=/tmp/entry-gateway",
		"--compose-file", "docker-compose.yml",
		"--start-command", "docker compose up -d",
		"--health-url", newHealthyTestURL(t),
		"--verification-workflow", "workflow.core-10",
		"--json",
	)
	var registered struct {
		OK          bool `json:"ok"`
		Environment struct {
			ID                     string         `json:"id"`
			Status                 string         `json:"status"`
			Verified               bool           `json:"verified"`
			VerificationWorkflowID string         `json:"verificationWorkflowId"`
			Repos                  map[string]any `json:"repos"`
		} `json:"environment"`
	}
	if err := json.Unmarshal([]byte(registerOut), &registered); err != nil {
		t.Fatalf("decode environment register json: %v\n%s", err, registerOut)
	}
	if !registered.OK || registered.Environment.ID != envID || registered.Environment.Status != "draft" || registered.Environment.Verified {
		t.Fatalf("registered %s environment = %#v", label, registered.Environment)
	}
	if registered.Environment.VerificationWorkflowID != "workflow.core-10" || registered.Environment.Repos["entry-gateway"] == nil {
		t.Fatalf("registered %s environment catalog fields = %#v", label, registered.Environment)
	}

	discoverOut := runCLI(t, "environment", "discover", "--json")
	var discovered struct {
		Count int `json:"count"`
	}
	if err := json.Unmarshal([]byte(discoverOut), &discovered); err != nil {
		t.Fatalf("decode discover json: %v\n%s", err, discoverOut)
	}
	if discovered.Count != 0 {
		t.Fatalf("unverified %s environment should stay out of default discovery: %#v", label, discovered)
	}

	publishDenied := runCLIFails(t, "environment", "publish-verified", envID)
	if !strings.Contains(publishDenied, "not publishable") {
		t.Fatalf("publish should require complete verification evidence: %q", publishDenied)
	}

	verifyOut := runCLI(t, "environment", "verify",
		envID,
		"--run", runID,
		"--status", "passed",
		"--evidence-complete",
		"--topology-complete",
		"--json",
	)
	var verified struct {
		Environment struct {
			Status                 string `json:"status"`
			LastVerificationRunID  string `json:"lastVerificationRunId"`
			LastVerificationStatus string `json:"lastVerificationStatus"`
			EvidenceComplete       bool   `json:"evidenceComplete"`
			TopologyComplete       bool   `json:"topologyComplete"`
		} `json:"environment"`
	}
	if err := json.Unmarshal([]byte(verifyOut), &verified); err != nil {
		t.Fatalf("decode verify json: %v\n%s", err, verifyOut)
	}
	if verified.Environment.Status != "verified-ready" || verified.Environment.LastVerificationRunID != runID || verified.Environment.LastVerificationStatus != "passed" || !verified.Environment.EvidenceComplete || !verified.Environment.TopologyComplete {
		t.Fatalf("verified %s environment = %#v", label, verified.Environment)
	}

	missingArtifacts := runCLIFails(t, "environment", "publish-verified", envID)
	if !strings.Contains(missingArtifacts, "was not found in Store") {
		t.Fatalf("publish should require indexed %s verification artifacts: %q", label, missingArtifacts)
	}
	seedEnvironmentVerificationArtifacts(t, storeRef, runID)

	publishOut := runCLI(t, "environment", "publish-verified", envID, "--json")
	var published struct {
		Environment struct {
			Status   string `json:"status"`
			Verified bool   `json:"verified"`
		} `json:"environment"`
	}
	if err := json.Unmarshal([]byte(publishOut), &published); err != nil {
		t.Fatalf("decode publish json: %v\n%s", err, publishOut)
	}
	if published.Environment.Status != "verified" || !published.Environment.Verified {
		t.Fatalf("published %s environment = %#v", label, published.Environment)
	}

	discoverVerifiedOut := runCLI(t, "environment", "discover", "--json")
	var discoveredVerified struct {
		Count int `json:"count"`
		Items []struct {
			ID       string `json:"id"`
			Verified bool   `json:"verified"`
		} `json:"items"`
	}
	if err := json.Unmarshal([]byte(discoverVerifiedOut), &discoveredVerified); err != nil {
		t.Fatalf("decode verified discover json: %v\n%s", err, discoverVerifiedOut)
	}
	if discoveredVerified.Count != 1 || discoveredVerified.Items[0].ID != envID || !discoveredVerified.Items[0].Verified {
		t.Fatalf("verified %s discovery = %#v", label, discoveredVerified)
	}

	bootstrapOut := runCLI(t, "environment", "bootstrap", "--json", envID)
	var bootstrap struct {
		Plan struct {
			VerificationWorkflow string         `json:"verificationWorkflow"`
			Repos                map[string]any `json:"repos"`
			HealthChecks         []any          `json:"healthChecks"`
		} `json:"plan"`
	}
	if err := json.Unmarshal([]byte(bootstrapOut), &bootstrap); err != nil {
		t.Fatalf("decode bootstrap json: %v\n%s", err, bootstrapOut)
	}
	if bootstrap.Plan.VerificationWorkflow != "workflow.core-10" || bootstrap.Plan.Repos["entry-gateway"] == nil || len(bootstrap.Plan.HealthChecks) != 1 {
		t.Fatalf("%s bootstrap plan = %#v", label, bootstrap.Plan)
	}
}

func TestEnvironmentComponentsReplaceRejectsBlockingDependencyCycle(t *testing.T) {
	storePath := filepath.Join(t.TempDir(), "store.sqlite")
	graphPath := filepath.Join(t.TempDir(), "graph.json")
	runCLI(t, "environment", "register",
		"--store", "sqlite://"+storePath,
		"--id", "env.component.replace-cycle",
		"--start-command", "true",
		"--verification-workflow", "workflow.core-10",
	)
	writeFile(t, graphPath, mustJSON(t, store.EnvironmentComponentGraph{
		Components: []store.EnvironmentComponent{
			{ComponentID: "app.a", Kind: "app", Role: "business-service", ComposeService: "app-a", Required: true, HealthCheckJSON: `{"type":"url","url":"http://127.0.0.1:18081/health"}`, RuntimeJSON: `{}`, SummaryJSON: `{}`},
			{ComponentID: "app.b", Kind: "app", Role: "business-service", ComposeService: "app-b", Required: true, HealthCheckJSON: `{"type":"url","url":"http://127.0.0.1:18082/health"}`, RuntimeJSON: `{}`, SummaryJSON: `{}`},
		},
		Dependencies: []store.ComponentDependency{
			{ConsumerComponentID: "app.a", ProviderComponentID: "app.b", Phase: "startup", Capability: "http", Required: true, ProfileJSON: `{}`},
			{ConsumerComponentID: "app.b", ProviderComponentID: "app.a", Phase: "startup", Capability: "http", Required: true, ProfileJSON: `{}`},
		},
	}))
	out := runCLIFails(t, "environment", "components", "replace", "--store", "sqlite://"+storePath, "--file", graphPath, "env.component.replace-cycle")
	if !strings.Contains(out, "component graph restore readiness failed") || !strings.Contains(out, "cycle") || !strings.Contains(out, "app.a") || !strings.Contains(out, "app.b") {
		t.Fatalf("replace cycle failure output = %q", out)
	}
}

func TestEnvironmentComponentsReplaceRejectsInvalidComponentHealthCheck(t *testing.T) {
	storePath := filepath.Join(t.TempDir(), "store.sqlite")
	graphPath := filepath.Join(t.TempDir(), "graph.json")
	runCLI(t, "environment", "register",
		"--store", "sqlite://"+storePath,
		"--id", "env.component.replace-health",
		"--start-command", "true",
		"--verification-workflow", "workflow.core-10",
	)
	writeFile(t, graphPath, mustJSON(t, store.EnvironmentComponentGraph{
		Components: []store.EnvironmentComponent{
			{ComponentID: "app", Kind: "app", Role: "business-service", Required: true, HealthCheckJSON: `{"kind":"url"}`, RuntimeJSON: `{}`, SummaryJSON: `{}`},
		},
	}))
	out := runCLIFails(t, "environment", "components", "replace", "--store", "sqlite://"+storePath, "--file", graphPath, "env.component.replace-health")
	if !strings.Contains(out, "component graph restore readiness failed") || !strings.Contains(out, "url health check requires url") {
		t.Fatalf("replace invalid health failure output = %q", out)
	}
}

func TestEnvironmentComponentsReplaceRejectsRemoteComponentAssetWithoutURLPath(t *testing.T) {
	storePath := filepath.Join(t.TempDir(), "store.sqlite")
	graphPath := filepath.Join(t.TempDir(), "graph.json")
	runCLI(t, "environment", "register",
		"--store", "sqlite://"+storePath,
		"--id", "env.component.replace-remote-asset",
		"--start-command", "true",
		"--verification-workflow", "workflow.core-10",
	)
	writeFile(t, graphPath, mustJSON(t, store.EnvironmentComponentGraph{
		Components: []store.EnvironmentComponent{
			{ComponentID: "app", Kind: "app", Role: "business-service", ComposeService: "app", Required: true, HealthCheckJSON: `{"type":"url","url":"http://127.0.0.1:18080/health"}`, RuntimeJSON: `{}`, SummaryJSON: `{}`},
		},
		Assets: []store.ComponentConfigAsset{
			{OwnerComponentID: "app", AssetID: "app.remote-ddl", AssetKind: "mysql-ddl", TargetPath: "compose/mysql/init/app.sql", RemoteRefJSON: `{"path":"compose/mysql/init/app.sql"}`, SizeBytes: 48 * 1024, SummaryJSON: `{}`},
		},
	}))
	out := runCLIFails(t, "environment", "components", "replace", "--store", "sqlite://"+storePath, "--file", graphPath, "env.component.replace-remote-asset")
	if !strings.Contains(out, "component graph restore readiness failed") || !strings.Contains(out, "remote Git URL/path") {
		t.Fatalf("replace invalid remote asset output = %q", out)
	}
}

func TestEnvironmentComponentsInspectReportsRestoreReadiness(t *testing.T) {
	storePath := filepath.Join(t.TempDir(), "store.sqlite")
	graphPath := filepath.Join(t.TempDir(), "graph.json")
	runCLI(t, "environment", "register",
		"--store", "sqlite://"+storePath,
		"--id", "env.component.inspect-readiness",
		"--start-command", "true",
		"--verification-workflow", "workflow.core-10",
	)
	writeFile(t, graphPath, mustJSON(t, store.EnvironmentComponentGraph{
		Components: []store.EnvironmentComponent{
			{ComponentID: "db", Kind: "middleware", Role: "database", ComposeService: "db", Required: true, HealthCheckJSON: `{"type":"compose-service"}`, RuntimeJSON: `{}`, SummaryJSON: `{}`},
			{ComponentID: "app", Kind: "app", Role: "business-service", ComposeService: "app", Required: true, HealthCheckJSON: `{"type":"url","url":"http://127.0.0.1:18080/health"}`, RuntimeJSON: `{}`, SummaryJSON: `{}`},
		},
		Dependencies: []store.ComponentDependency{
			{ConsumerComponentID: "app", ProviderComponentID: "db", Phase: "startup", Capability: "sql", Required: true, ProfileJSON: `{}`},
		},
		Assets: []store.ComponentConfigAsset{
			{OwnerComponentID: "app", AssetID: "app.schema", AssetKind: "mysql-ddl", TargetComponentID: "db", TargetPath: "compose/mysql/init/app.sql", ContentInline: "create database app;\n", ApplyOrder: 10, SummaryJSON: `{}`},
		},
	}))
	replaceOut := runCLI(t, "environment", "components", "replace", "--store", "sqlite://"+storePath, "--file", graphPath, "--json", "env.component.inspect-readiness")
	inspectOut := runCLI(t, "environment", "components", "inspect", "--store", "sqlite://"+storePath, "--json", "env.component.inspect-readiness")
	documentedReplaceOut := runCLI(t, "environment", "components", "replace", "env.component.inspect-readiness", "--store", "sqlite://"+storePath, "--file", graphPath, "--json")
	documentedInspectOut := runCLI(t, "environment", "components", "inspect", "env.component.inspect-readiness", "--store", "sqlite://"+storePath, "--json")
	for _, out := range []string{replaceOut, inspectOut, documentedReplaceOut, documentedInspectOut} {
		var payload struct {
			ComponentGraph struct {
				RestoreReadiness struct {
					OK                   bool     `json:"ok"`
					BlockingDependencies int      `json:"blockingDependencies"`
					Assets               int      `json:"assets"`
					BlockingOrder        []string `json:"blockingOrder"`
				} `json:"restoreReadiness"`
			} `json:"componentGraph"`
		}
		if err := json.Unmarshal([]byte(out), &payload); err != nil {
			t.Fatalf("decode components readiness json: %v\n%s", err, out)
		}
		readiness := payload.ComponentGraph.RestoreReadiness
		if !readiness.OK || readiness.BlockingDependencies != 1 || readiness.Assets != 1 || strings.Join(readiness.BlockingOrder, ",") != "db,app" {
			t.Fatalf("components readiness payload = %#v\n%s", readiness, out)
		}
	}
}

func TestEnvironmentInspectReportsComponentGraphReadiness(t *testing.T) {
	storePath := filepath.Join(t.TempDir(), "store.sqlite")
	graphPath := filepath.Join(t.TempDir(), "graph.json")
	runCLI(t, "environment", "register",
		"--store", "sqlite://"+storePath,
		"--id", "env.inspect.component-readiness",
		"--start-command", "true",
		"--verification-workflow", "workflow.core-10",
	)
	writeFile(t, graphPath, mustJSON(t, store.EnvironmentComponentGraph{
		Components: []store.EnvironmentComponent{
			{ComponentID: "db", Kind: "middleware", Role: "database", ComposeService: "db", Required: true, HealthCheckJSON: `{"type":"compose-service"}`, RuntimeJSON: `{}`, SummaryJSON: `{}`},
			{ComponentID: "app", Kind: "app", Role: "business-service", ComposeService: "app", Required: true, HealthCheckJSON: `{"type":"url","url":"http://127.0.0.1:18080/health"}`, RuntimeJSON: `{}`, SummaryJSON: `{}`},
		},
		Dependencies: []store.ComponentDependency{
			{ConsumerComponentID: "app", ProviderComponentID: "db", Phase: "startup", Capability: "sql", Required: true, ProfileJSON: `{}`},
		},
	}))
	runCLI(t, "environment", "components", "replace", "--store", "sqlite://"+storePath, "--file", graphPath, "env.inspect.component-readiness")
	out := runCLI(t, "environment", "inspect", "--store", "sqlite://"+storePath, "--json", "env.inspect.component-readiness")
	var payload struct {
		ComponentGraph struct {
			OK                   bool     `json:"ok"`
			Components           int      `json:"components"`
			BlockingDependencies int      `json:"blockingDependencies"`
			BlockingOrder        []string `json:"blockingOrder"`
		} `json:"componentGraph"`
	}
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		t.Fatalf("decode environment inspect component readiness json: %v\n%s", err, out)
	}
	if !payload.ComponentGraph.OK || payload.ComponentGraph.Components != 2 || payload.ComponentGraph.BlockingDependencies != 1 || strings.Join(payload.ComponentGraph.BlockingOrder, ",") != "db,app" {
		t.Fatalf("environment inspect component readiness = %#v", payload.ComponentGraph)
	}
}

func TestEnvironmentBootstrapReportsComponentGraphReadiness(t *testing.T) {
	storePath := filepath.Join(t.TempDir(), "store.sqlite")
	graphPath := filepath.Join(t.TempDir(), "graph.json")
	runCLI(t, "environment", "register",
		"--store", "sqlite://"+storePath,
		"--id", "env.component.bootstrap-readiness",
		"--start-command", "true",
		"--verification-workflow", "workflow.core-10",
	)
	writeFile(t, graphPath, mustJSON(t, store.EnvironmentComponentGraph{
		Components: []store.EnvironmentComponent{
			{ComponentID: "db", Kind: "middleware", Role: "database", ComposeService: "db", Required: true, HealthCheckJSON: `{"type":"compose-service"}`, RuntimeJSON: `{}`, SummaryJSON: `{}`},
			{ComponentID: "app", Kind: "app", Role: "business-service", ComposeService: "app", Required: true, HealthCheckJSON: `{"type":"url","url":"http://127.0.0.1:18080/health"}`, RuntimeJSON: `{}`, SummaryJSON: `{}`},
		},
		Dependencies: []store.ComponentDependency{
			{ConsumerComponentID: "app", ProviderComponentID: "db", Phase: "startup", Capability: "sql", Required: true, ProfileJSON: `{}`},
		},
	}))
	runCLI(t, "environment", "components", "replace", "--store", "sqlite://"+storePath, "--file", graphPath, "env.component.bootstrap-readiness")
	out := runCLI(t, "environment", "bootstrap", "--store", "sqlite://"+storePath, "--json", "env.component.bootstrap-readiness")
	var payload struct {
		Plan struct {
			ComponentGraph struct {
				OK                   bool     `json:"ok"`
				BlockingDependencies int      `json:"blockingDependencies"`
				BlockingOrder        []string `json:"blockingOrder"`
			} `json:"componentGraph"`
			ComponentStartupPlan struct {
				OK      bool `json:"ok"`
				Batches []struct {
					Components []struct {
						ComponentID string `json:"componentId"`
					} `json:"components"`
				} `json:"batches"`
				HealthGates []struct {
					ComponentID string `json:"componentId"`
				} `json:"healthGates"`
			} `json:"componentStartupPlan"`
			Restore struct {
				ComponentGraph struct {
					OK            bool     `json:"ok"`
					BlockingOrder []string `json:"blockingOrder"`
				} `json:"componentGraph"`
				ComponentStartupPlan struct {
					OK bool `json:"ok"`
				} `json:"componentStartupPlan"`
			} `json:"restore"`
		} `json:"plan"`
	}
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		t.Fatalf("decode bootstrap component readiness json: %v\n%s", err, out)
	}
	if !payload.Plan.ComponentGraph.OK || payload.Plan.ComponentGraph.BlockingDependencies != 1 || strings.Join(payload.Plan.ComponentGraph.BlockingOrder, ",") != "db,app" {
		t.Fatalf("bootstrap component graph readiness = %#v", payload.Plan.ComponentGraph)
	}
	if !payload.Plan.Restore.ComponentGraph.OK || strings.Join(payload.Plan.Restore.ComponentGraph.BlockingOrder, ",") != "db,app" {
		t.Fatalf("bootstrap restore component graph readiness = %#v", payload.Plan.Restore.ComponentGraph)
	}
	if !payload.Plan.ComponentStartupPlan.OK || len(payload.Plan.ComponentStartupPlan.Batches) != 2 || payload.Plan.ComponentStartupPlan.Batches[0].Components[0].ComponentID != "db" || payload.Plan.ComponentStartupPlan.Batches[1].Components[0].ComponentID != "app" || len(payload.Plan.ComponentStartupPlan.HealthGates) != 2 {
		t.Fatalf("bootstrap component startup plan = %#v", payload.Plan.ComponentStartupPlan)
	}
	if !payload.Plan.Restore.ComponentStartupPlan.OK {
		t.Fatalf("bootstrap restore component startup plan = %#v", payload.Plan.Restore.ComponentStartupPlan)
	}
}

func TestEnvironmentStartupFilePutMergesGeneratedFilesWithoutReRegistering(t *testing.T) {
	storePath := filepath.Join(t.TempDir(), "store.sqlite")
	sourceCompose := filepath.Join(t.TempDir(), "source-compose.yml")
	writeFile(t, sourceCompose, "services:\n  generated-service:\n    image: alpine:3.20\n")
	runCLI(t, "environment", "register",
		"--store", "sqlite://"+storePath,
		"--id", "env.startup.files",
		"--repo", "entry-gateway=https://example.com/team/entry-gateway.git",
		"--checkout", "entry-gateway=services/entry-gateway",
		"--compose-file", "compose/docker-compose.yml",
		"--health-url", newHealthyTestURL(t),
		"--verification-workflow", "workflow.core-10",
	)

	out := runCLI(t, "environment", "startup-file", "put",
		"--store", "sqlite://"+storePath,
		"--file", "compose/docker-compose.yml="+sourceCompose,
		"--json",
		"env.startup.files",
	)
	var payload struct {
		GeneratedFiles []struct {
			Path  string `json:"path"`
			Bytes int    `json:"bytes"`
		} `json:"generatedFiles"`
		Environment struct {
			Repos   map[string]any `json:"repos"`
			Compose struct {
				GeneratedFiles map[string]string `json:"generatedFiles"`
			} `json:"compose"`
			Summary struct {
				StartupFiles struct {
					Files []struct {
						Path string `json:"path"`
					} `json:"files"`
				} `json:"startupFiles"`
			} `json:"summary"`
		} `json:"environment"`
	}
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		t.Fatalf("decode startup-file put json: %v\n%s", err, out)
	}
	if len(payload.GeneratedFiles) != 1 || payload.GeneratedFiles[0].Path != "compose/docker-compose.yml" || payload.GeneratedFiles[0].Bytes == 0 {
		t.Fatalf("startup-file payload = %#v", payload.GeneratedFiles)
	}
	if payload.Environment.Repos["entry-gateway"] == nil {
		t.Fatalf("startup-file put should preserve existing repositories: %#v", payload.Environment.Repos)
	}
	if !strings.Contains(payload.Environment.Compose.GeneratedFiles["compose/docker-compose.yml"], "generated-service") {
		t.Fatalf("generated file was not stored in compose metadata: %#v", payload.Environment.Compose.GeneratedFiles)
	}
	if len(payload.Environment.Summary.StartupFiles.Files) != 1 || payload.Environment.Summary.StartupFiles.Files[0].Path != "compose/docker-compose.yml" {
		t.Fatalf("startup-file summary = %#v", payload.Environment.Summary.StartupFiles)
	}
}
