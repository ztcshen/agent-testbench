package main

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

type environmentNamedStoreFixture struct {
	envID string
	runID string
	label string
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
	fixture := environmentNamedStoreFixture{
		envID: envID,
		runID: "run.core-10." + time.Now().UTC().Format("20060102150405.000000000"),
		label: label,
	}
	registerNamedStoreEnvironment(t, fixture)
	requireNamedStoreUnverifiedDiscoveryHidden(t, fixture)
	requireNamedStorePublishDenied(t, fixture)
	verifyNamedStoreEnvironment(t, fixture)
	requireNamedStorePublishNeedsArtifacts(t, fixture)
	seedEnvironmentVerificationArtifacts(t, storeRef, fixture.runID)
	publishNamedStoreEnvironment(t, fixture)
	requireNamedStoreVerifiedDiscovery(t, fixture)
	requireNamedStoreBootstrapPlan(t, fixture)
}

func registerNamedStoreEnvironment(t *testing.T, fixture environmentNamedStoreFixture) {
	t.Helper()
	registerOut := runCLI(t, "environment", "register",
		"--id", fixture.envID,
		"--display-name", "Team "+fixture.label+" Environment",
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
	if !registered.OK || registered.Environment.ID != fixture.envID || registered.Environment.Status != "draft" || registered.Environment.Verified {
		t.Fatalf("registered %s environment = %#v", fixture.label, registered.Environment)
	}
	if registered.Environment.VerificationWorkflowID != "workflow.core-10" || registered.Environment.Repos["entry-gateway"] == nil {
		t.Fatalf("registered %s environment catalog fields = %#v", fixture.label, registered.Environment)
	}
}

func requireNamedStoreUnverifiedDiscoveryHidden(t *testing.T, fixture environmentNamedStoreFixture) {
	t.Helper()
	discoverOut := runCLI(t, "environment", "discover", "--json")
	var discovered struct {
		Count int `json:"count"`
	}
	if err := json.Unmarshal([]byte(discoverOut), &discovered); err != nil {
		t.Fatalf("decode discover json: %v\n%s", err, discoverOut)
	}
	if discovered.Count != 0 {
		t.Fatalf("unverified %s environment should stay out of default discovery: %#v", fixture.label, discovered)
	}
}

func requireNamedStorePublishDenied(t *testing.T, fixture environmentNamedStoreFixture) {
	t.Helper()
	publishDenied := runCLIFails(t, "environment", "publish-verified", fixture.envID)
	if !strings.Contains(publishDenied, "not publishable") {
		t.Fatalf("publish should require complete verification evidence: %q", publishDenied)
	}
}

func verifyNamedStoreEnvironment(t *testing.T, fixture environmentNamedStoreFixture) {
	t.Helper()
	verifyOut := runCLI(t, "environment", "verify",
		fixture.envID,
		"--run", fixture.runID,
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
	if verified.Environment.Status != "verified-ready" || verified.Environment.LastVerificationRunID != fixture.runID || verified.Environment.LastVerificationStatus != "passed" || !verified.Environment.EvidenceComplete || !verified.Environment.TopologyComplete {
		t.Fatalf("verified %s environment = %#v", fixture.label, verified.Environment)
	}
}

func requireNamedStorePublishNeedsArtifacts(t *testing.T, fixture environmentNamedStoreFixture) {
	t.Helper()
	missingArtifacts := runCLIFails(t, "environment", "publish-verified", fixture.envID)
	if !strings.Contains(missingArtifacts, "was not found in Store") {
		t.Fatalf("publish should require indexed %s verification artifacts: %q", fixture.label, missingArtifacts)
	}
}

func publishNamedStoreEnvironment(t *testing.T, fixture environmentNamedStoreFixture) {
	t.Helper()
	publishOut := runCLI(t, "environment", "publish-verified", fixture.envID, "--json")
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
		t.Fatalf("published %s environment = %#v", fixture.label, published.Environment)
	}
}

func requireNamedStoreVerifiedDiscovery(t *testing.T, fixture environmentNamedStoreFixture) {
	t.Helper()
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
	if discoveredVerified.Count != 1 || discoveredVerified.Items[0].ID != fixture.envID || !discoveredVerified.Items[0].Verified {
		t.Fatalf("verified %s discovery = %#v", fixture.label, discoveredVerified)
	}
}

func requireNamedStoreBootstrapPlan(t *testing.T, fixture environmentNamedStoreFixture) {
	t.Helper()
	bootstrapOut := runCLI(t, "environment", "bootstrap", "--json", fixture.envID)
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
		t.Fatalf("%s bootstrap plan = %#v", fixture.label, bootstrap.Plan)
	}
}
