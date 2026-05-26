package main

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

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

	registerVerifiedDiscoveryEnvironment(t, storeRef)
	updateVerifiedDiscoveryRepo(t, storeRef)
	requireVerifiedDiscoveryCount(t, storeRef, false)
	requireVerifiedDiscoveryPublishDenied(t, storeRef)
	verifyEnvironmentForDiscovery(t, storeRef)
	requireVerifiedDiscoveryPublishNeedsArtifacts(t, storeRef)
	seedEnvironmentVerificationArtifacts(t, storeRef, "run.core-10")
	publishVerifiedDiscoveryEnvironment(t, storeRef)
	requireVerifiedDiscoveryCount(t, storeRef, true)
	requireVerifiedDiscoveryBootstrapPlan(t, storeRef)
}

func registerVerifiedDiscoveryEnvironment(t *testing.T, storeRef string) {
	t.Helper()
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
}

func updateVerifiedDiscoveryRepo(t *testing.T, storeRef string) {
	t.Helper()
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
}

func requireVerifiedDiscoveryCount(t *testing.T, storeRef string, published bool) {
	t.Helper()
	discoverOut := runCLI(t, "environment", "discover", "--store", storeRef, "--json")
	var discovered struct {
		Count int `json:"count"`
	}
	if err := json.Unmarshal([]byte(discoverOut), &discovered); err != nil {
		t.Fatalf("decode discover json: %v\n%s", err, discoverOut)
	}
	if !published {
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
		return
	}
	if discovered.Count != 1 {
		t.Fatalf("verified discovery count = %#v", discovered)
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
}

func requireVerifiedDiscoveryPublishDenied(t *testing.T, storeRef string) {
	t.Helper()
	publishDenied := runCLIFails(t, "environment", "publish-verified", "--store", storeRef, "env.team.verified")
	if !strings.Contains(publishDenied, "not publishable") {
		t.Fatalf("publish should require complete verification evidence: %q", publishDenied)
	}
}

func verifyEnvironmentForDiscovery(t *testing.T, storeRef string) {
	t.Helper()
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
}

func requireVerifiedDiscoveryPublishNeedsArtifacts(t *testing.T, storeRef string) {
	t.Helper()
	missingArtifacts := runCLIFails(t, "environment", "publish-verified", "--store", storeRef, "env.team.verified")
	if !strings.Contains(missingArtifacts, "was not found in Store") {
		t.Fatalf("publish should require indexed verification artifacts: %q", missingArtifacts)
	}
}

func publishVerifiedDiscoveryEnvironment(t *testing.T, storeRef string) {
	t.Helper()
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
}

func requireVerifiedDiscoveryBootstrapPlan(t *testing.T, storeRef string) {
	t.Helper()
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
