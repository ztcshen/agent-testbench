package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"agent-testbench/internal/store"
	"agent-testbench/internal/store/sqlite"
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

func TestEnvironmentRegisterStoresDockerFilesAsStructuredRows(t *testing.T) {
	storePath := filepath.Join(t.TempDir(), "store.sqlite")
	composeSource := filepath.Join(t.TempDir(), "compose.yml")
	envSource := filepath.Join(t.TempDir(), "runtime.env")
	writeFile(t, composeSource, "services:\n  app:\n    image: alpine:3.20\n")
	writeFile(t, envSource, "APP_MODE=test\n")

	registerOut := runCLI(t, "environment", "register",
		"--store", "sqlite://"+storePath,
		"--id", "env.structured.files",
		"--compose-file", "compose/docker-compose.yml",
		"--compose-env-file", "compose/runtime.env",
		"--compose-generated-file", "compose/docker-compose.yml="+composeSource,
		"--compose-generated-file", "compose/runtime.env="+envSource,
		"--service", "app",
		"--repo", "app=https://example.com/team/app.git",
		"--branch", "app=main",
		"--repo-ref", "app=v1.0.0",
		"--checkout", "app=app",
		"--health-url", "http://127.0.0.1:18080/health",
		"--verification-workflow", "workflow.core-10",
		"--json",
	)
	var registered struct {
		Environment struct {
			Compose      map[string]any   `json:"compose"`
			Services     []map[string]any `json:"services"`
			Repos        map[string]any   `json:"repos"`
			HealthChecks []map[string]any `json:"healthChecks"`
		} `json:"environment"`
	}
	if err := json.Unmarshal([]byte(registerOut), &registered); err != nil {
		t.Fatalf("decode register output: %v\n%s", err, registerOut)
	}
	requireStructuredRegisterOutput(t, registered.Environment.Compose, registered.Environment.Services, registered.Environment.Repos, registered.Environment.HealthChecks)
	requireStructuredEnvironmentRows(t, storePath)
	requireRawEnvironmentCompatibilityColumns(t, storePath)
	requireStructuredEnvironmentRuntimeProjection(t, storePath)
	requireStructuredEnvironmentInspectProjection(t, storePath)
}

func TestEnvironmentRegisterDoesNotPersistPartialRowsWhenStructuredFilesFail(t *testing.T) {
	storePath := filepath.Join(t.TempDir(), "store.sqlite")
	composeSource := filepath.Join(t.TempDir(), "compose.yml")
	writeFile(t, composeSource, strings.Repeat("x", store.EnvironmentFileInlineMaxBytes+1))

	out := runCLIFails(t, "environment", "register",
		"--store", "sqlite://"+storePath,
		"--id", "env.partial.blocked",
		"--compose-file", "compose/docker-compose.yml",
		"--compose-generated-file", "compose/docker-compose.yml="+composeSource,
		"--verification-workflow", "workflow.core-10",
	)
	if !strings.Contains(out, "write blocked") {
		t.Fatalf("oversized structured file error = %q", out)
	}

	ctx := context.Background()
	s, err := sqlite.Open(ctx, sqlite.Config{Path: storePath})
	if err != nil {
		t.Fatalf("open sqlite store: %v", err)
	}
	t.Cleanup(func() {
		_ = s.Close()
	})
	if _, err := s.GetEnvironment(ctx, "env.partial.blocked"); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("failed environment registration should not leave a partial environment row, got err=%v", err)
	}
}

func TestEnvironmentRepoSetPreservesMixedLegacyServices(t *testing.T) {
	storePath := filepath.Join(t.TempDir(), "store.sqlite")
	storeRef := "sqlite://" + storePath
	runCLI(t, "environment", "register",
		"--store", storeRef,
		"--id", "env.mixed.repo-set",
		"--service", "app",
		"--repo", "app=https://example.invalid/app.git",
		"--checkout", "app=app",
		"--verification-workflow", "workflow.core-10",
		"--json",
	)
	seedLegacyEnvironmentRepositoryMetadata(t, storePath)

	repoSetOut := runCLI(t, "environment", "repo", "set",
		"--store", storeRef,
		"--repo-ref", "app=v2.0.0",
		"--checkout", "app=app-v2",
		"--json",
		"env.mixed.repo-set",
	)
	var repoSet struct {
		Environment struct {
			Services []map[string]any `json:"services"`
			Repos    map[string]any   `json:"repos"`
		} `json:"environment"`
	}
	if err := json.Unmarshal([]byte(repoSetOut), &repoSet); err != nil {
		t.Fatalf("decode environment repo set json: %v\n%s", err, repoSetOut)
	}
	requireMixedRepoSetProjection(t, repoSet.Environment.Services, repoSet.Environment.Repos)
	requireMixedRepoSetStructuredRows(t, storePath)
	if rawServicesJSON := sqliteScalar(t, storePath, `select services_json from environments where id = 'env.mixed.repo-set';`); rawServicesJSON != "[]" {
		t.Fatalf("raw services_json should be migrated into structured services after repo set: %s", rawServicesJSON)
	}
	if rawReposJSON := sqliteScalar(t, storePath, `select repos_json from environments where id = 'env.mixed.repo-set';`); rawReposJSON != "{}" {
		t.Fatalf("raw repos_json should be migrated into structured services after repo set: %s", rawReposJSON)
	}
}

func TestEnvironmentConfigureReposUpdatesStructuredRepository(t *testing.T) {
	storePath := filepath.Join(t.TempDir(), "store.sqlite")
	storeRef := "sqlite://" + storePath
	runCLI(t, "environment", "register",
		"--store", storeRef,
		"--id", "env.configure.repos",
		"--service", "app",
		"--repo", "app=https://example.invalid/app.git",
		"--checkout", "app=app",
		"--verification-workflow", "workflow.core-10",
		"--json",
	)

	out := runCLI(t, "environment", "configure",
		"--view", "repos",
		"--store", storeRef,
		"--repo-ref", "app=v2.0.0",
		"--checkout", "app=app-v2",
		"--json",
		"env.configure.repos",
	)
	var payload struct {
		OK          bool `json:"ok"`
		Environment struct {
			Services []map[string]any `json:"services"`
			Repos    map[string]any   `json:"repos"`
		} `json:"environment"`
	}
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		t.Fatalf("decode environment configure repos json: %v\n%s", err, out)
	}
	appRepo, _ := payload.Environment.Repos["app"].(map[string]any)
	if !payload.OK || appRepo["ref"] != "v2.0.0" || appRepo["checkout"] != "app-v2" {
		t.Fatalf("configure repos should update structured repository metadata: %#v", payload)
	}
	if len(payload.Environment.Services) != 1 || payload.Environment.Services[0]["ref"] != "v2.0.0" || payload.Environment.Services[0]["checkout"] != "app-v2" {
		t.Fatalf("configure repos should update structured service projection: %#v", payload.Environment.Services)
	}
}

func seedLegacyEnvironmentRepositoryMetadata(t *testing.T, storePath string) {
	t.Helper()
	ctx := context.Background()
	runtime, err := sqlite.Open(ctx, sqlite.Config{Path: storePath})
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer runtime.Close()
	env, err := runtime.GetEnvironment(ctx, "env.mixed.repo-set")
	if err != nil {
		t.Fatalf("get environment: %v", err)
	}
	env.ServicesJSON = `[{"id":"legacy-service","repo":"https://example.invalid/legacy.git","checkout":"legacy-service"}]`
	env.ReposJSON = `{"legacy-service":{"url":"https://example.invalid/legacy.git","checkout":"legacy-service"}}`
	if _, err := runtime.UpsertEnvironment(ctx, env); err != nil {
		t.Fatalf("seed legacy environment metadata: %v", err)
	}
}

func requireMixedRepoSetProjection(t *testing.T, services []map[string]any, repos map[string]any) {
	t.Helper()
	serviceIDs := map[string]bool{}
	for _, service := range services {
		serviceIDs[fmt.Sprint(service["id"])] = true
	}
	appRepo, _ := repos["app"].(map[string]any)
	legacyRepo, _ := repos["legacy-service"].(map[string]any)
	if len(services) != 2 || !serviceIDs["app"] || !serviceIDs["legacy-service"] {
		t.Fatalf("repo set should preserve structured and legacy services: %#v", services)
	}
	if appRepo["ref"] != "v2.0.0" || appRepo["checkout"] != "app-v2" {
		t.Fatalf("repo set should update structured app repo: %#v", appRepo)
	}
	if legacyRepo["url"] != "https://example.invalid/legacy.git" || legacyRepo["checkout"] != "legacy-service" {
		t.Fatalf("repo set should preserve legacy repo metadata: %#v", legacyRepo)
	}
}

func requireMixedRepoSetStructuredRows(t *testing.T, storePath string) {
	t.Helper()
	ctx := context.Background()
	runtime, err := sqlite.Open(ctx, sqlite.Config{Path: storePath})
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer runtime.Close()
	services, err := runtime.ListEnvironmentServices(ctx, "env.mixed.repo-set")
	if err != nil {
		t.Fatalf("list environment services: %v", err)
	}
	if len(services) != 2 {
		t.Fatalf("repo set should migrate mixed services into structured rows: %#v", services)
	}
}

func requireStructuredRegisterOutput(t *testing.T, compose map[string]any, services []map[string]any, repos map[string]any, healthChecks []map[string]any) {
	t.Helper()
	if _, ok := compose["generatedFiles"]; !ok {
		t.Fatalf("runtime register output should merge structured generated files: %#v", compose)
	}
	if len(services) != 1 || services[0]["repo"] != "https://example.com/team/app.git" || repos["app"] == nil || len(healthChecks) != 1 {
		t.Fatalf("runtime register output should merge structured metadata: services=%#v repos=%#v health=%#v", services, repos, healthChecks)
	}
}

func requireStructuredEnvironmentRows(t *testing.T, storePath string) {
	t.Helper()
	ctx := context.Background()
	runtime, err := sqlite.Open(ctx, sqlite.Config{Path: storePath})
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer runtime.Close()
	files, err := runtime.ListEnvironmentFiles(ctx, "env.structured.files")
	if err != nil {
		t.Fatalf("list environment files: %v", err)
	}
	if len(files) != 2 || files[0].Kind != store.EnvironmentFileKindComposeFile || files[1].Kind != store.EnvironmentFileKindComposeEnvFile {
		t.Fatalf("structured environment files = %#v", files)
	}
	services, err := runtime.ListEnvironmentServices(ctx, "env.structured.files")
	if err != nil {
		t.Fatalf("list environment services: %v", err)
	}
	if len(services) != 1 || services[0].ServiceID != "app" || services[0].RepoURL != "https://example.com/team/app.git" || services[0].Ref != "v1.0.0" {
		t.Fatalf("structured environment services = %#v", services)
	}
	checks, err := runtime.ListEnvironmentHealthChecks(ctx, "env.structured.files")
	if err != nil {
		t.Fatalf("list environment health checks: %v", err)
	}
	if len(checks) != 1 || checks[0].Kind != "url" || checks[0].URL != "http://127.0.0.1:18080/health" {
		t.Fatalf("structured environment health checks = %#v", checks)
	}
}

func requireRawEnvironmentCompatibilityColumns(t *testing.T, storePath string) {
	t.Helper()
	rawComposeJSON := sqliteScalar(t, storePath, `select compose_json from environments where id = 'env.structured.files';`)
	if strings.Contains(rawComposeJSON, "generatedFiles") {
		t.Fatalf("raw compose_json should not carry generated file content: %s", rawComposeJSON)
	}
	if rawServicesJSON := sqliteScalar(t, storePath, `select services_json from environments where id = 'env.structured.files';`); rawServicesJSON != "[]" {
		t.Fatalf("raw services_json should not carry structured services: %s", rawServicesJSON)
	}
	if rawReposJSON := sqliteScalar(t, storePath, `select repos_json from environments where id = 'env.structured.files';`); rawReposJSON != "{}" {
		t.Fatalf("raw repos_json should not carry structured repos: %s", rawReposJSON)
	}
	if rawHealthJSON := sqliteScalar(t, storePath, `select health_checks_json from environments where id = 'env.structured.files';`); rawHealthJSON != "[]" {
		t.Fatalf("raw health_checks_json should not carry structured health checks: %s", rawHealthJSON)
	}
}

func requireStructuredEnvironmentRuntimeProjection(t *testing.T, storePath string) {
	t.Helper()
	ctx := context.Background()
	runtime, err := sqlite.Open(ctx, sqlite.Config{Path: storePath})
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer runtime.Close()
	loaded, err := runtime.GetEnvironment(ctx, "env.structured.files")
	if err != nil {
		t.Fatalf("get environment: %v", err)
	}
	if !strings.Contains(loaded.ComposeJSON, "generatedFiles") || !strings.Contains(loaded.ComposeJSON, "APP_MODE=test") {
		t.Fatalf("structured files should merge into runtime compose json: %s", loaded.ComposeJSON)
	}
}

func requireStructuredEnvironmentInspectProjection(t *testing.T, storePath string) {
	t.Helper()
	inspectOut := runCLI(t, "environment", "inspect",
		"--store", "sqlite://"+storePath,
		"env.structured.files",
		"--json",
	)
	var inspected struct {
		FileProjection struct {
			Files []struct {
				Path   string `json:"path"`
				Kind   string `json:"kind"`
				Source string `json:"source"`
			} `json:"files"`
		} `json:"fileProjection"`
	}
	if err := json.Unmarshal([]byte(inspectOut), &inspected); err != nil {
		t.Fatalf("decode inspect output: %v\n%s", err, inspectOut)
	}
	projectionSources := map[string]string{}
	for _, file := range inspected.FileProjection.Files {
		projectionSources[file.Kind+":"+file.Path] = file.Source
	}
	if projectionSources["compose-file:compose/docker-compose.yml"] != "environment_files" ||
		projectionSources["env-file:compose/runtime.env"] != "environment_files" {
		t.Fatalf("inspect fileProjection should expose structured sources: %#v", projectionSources)
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
