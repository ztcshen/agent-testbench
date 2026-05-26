package main

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"agent-testbench/internal/store"
)

func TestEnvironmentRestorePreflightReportsMissingDockerComposePlugin(t *testing.T) {
	workspace := filepath.Join(t.TempDir(), "workspace")
	fakeBin := t.TempDir()
	writeFile(t, filepath.Join(fakeBin, "git"), "#!/bin/sh\nexit 0\n")
	writeFile(t, filepath.Join(fakeBin, "docker"), "#!/bin/sh\nif [ \"$1\" = compose ] && [ \"$2\" = version ]; then exit 17; fi\nexit 0\n")
	if err := os.Chmod(filepath.Join(fakeBin, "git"), 0o755); err != nil {
		t.Fatalf("chmod fake git: %v", err)
	}
	if err := os.Chmod(filepath.Join(fakeBin, "docker"), 0o755); err != nil {
		t.Fatalf("chmod fake docker: %v", err)
	}
	t.Setenv("PATH", fakeBin)
	report, err := buildEnvironmentRestoreReport(context.Background(), store.Environment{
		ID:                     "env.preflight.compose",
		ReposJSON:              `{}`,
		ComposeJSON:            `{"composeFile":"docker-compose.yml"}`,
		HealthChecksJSON:       `[]`,
		VerificationWorkflowID: "workflow.core-10",
	}, workspace, false, false, false, time.Second, environmentRestoreWorkflowOptions{}, environmentRestoreDockerCleanupOptions{})
	if err != nil {
		t.Fatalf("build restore preflight report: %v", err)
	}
	if report.OK || report.Preflight.OK || !restoreTypedPreflightHasTool(report.Preflight.Tools, "docker", true) || !restoreTypedPreflightHasTool(report.Preflight.Tools, "docker compose", false) {
		t.Fatalf("missing docker compose preflight report = %#v", report.Preflight)
	}
}

func restoreTypedReadinessHasItem(items []environmentRestoreReadinessItem, name string, ok bool, detailContains string) bool {
	for _, item := range items {
		if item.Name != name || item.OK != ok {
			continue
		}
		if detailContains == "" || strings.Contains(item.Detail, detailContains) {
			return true
		}
	}
	return false
}

func restorePreflightHasTool(tools []struct {
	Name     string `json:"name"`
	Required bool   `json:"required"`
	OK       bool   `json:"ok"`
}, name string, ok bool) bool {
	for _, tool := range tools {
		if tool.Name == name && tool.Required && tool.OK == ok {
			return true
		}
	}
	return false
}

func restoreTypedPreflightHasTool(tools []environmentRestorePreflightTool, name string, ok bool) bool {
	for _, tool := range tools {
		if tool.Name == name && tool.Required && tool.OK == ok {
			return true
		}
	}
	return false
}

func restoreReadinessHasItem(items []struct {
	Name   string `json:"name"`
	OK     bool   `json:"ok"`
	Detail string `json:"detail"`
}, name string, ok bool, detailContains string) bool {
	for _, item := range items {
		if item.Name != name || item.OK != ok {
			continue
		}
		if detailContains == "" || strings.Contains(item.Detail, detailContains) {
			return true
		}
	}
	return false
}

func commandSlicesContain(commands [][]string, part string) bool {
	for _, command := range commands {
		for _, item := range command {
			if item == part {
				return true
			}
		}
	}
	return false
}

func TestEnvironmentRestoreExecutesDockerComposeWithoutRepository(t *testing.T) {
	storePath := filepath.Join(t.TempDir(), "store.sqlite")
	workspace := filepath.Join(t.TempDir(), "workspace")
	fakeDockerEnv, _ := fakeDockerCommand(t)
	healthServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer healthServer.Close()
	writeFile(t, filepath.Join(workspace, "compose.yml"), "services: {}\n")

	runCLI(t, "environment", "register",
		"--store", "sqlite://"+storePath,
		"--id", "env.docker.only",
		"--compose-file", "compose.yml",
		"--health-url", healthServer.URL+"/ready",
		"--verification-workflow", "workflow.core-10",
	)

	out := runCLIWithEnv(t, fakeDockerEnv, "environment", "restore", "--store", "sqlite://"+storePath, "--workspace", workspace, "--execute", "--json", "env.docker.only")
	var report struct {
		OK     bool  `json:"ok"`
		Repos  []any `json:"repos"`
		Docker struct {
			OK           bool   `json:"ok"`
			Action       string `json:"action"`
			HealthChecks []struct {
				OK bool `json:"ok"`
			} `json:"healthChecks"`
		} `json:"docker"`
	}
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatalf("decode docker-only restore json: %v\n%s", err, out)
	}
	if !report.OK || len(report.Repos) != 0 || !report.Docker.OK || report.Docker.Action != "run-docker-compose" || len(report.Docker.HealthChecks) != 1 || !report.Docker.HealthChecks[0].OK {
		t.Fatalf("docker-only restore report = %#v", report)
	}
}

func TestEnvironmentRestoreAppliesAssetsBoundToDependencyEdges(t *testing.T) {
	workspace := filepath.Join(t.TempDir(), "workspace")
	fakeDockerEnv, dockerCallsPath := fakeDockerCommand(t)
	healthServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer healthServer.Close()
	for _, kv := range fakeDockerEnv {
		parts := strings.SplitN(kv, "=", 2)
		t.Setenv(parts[0], parts[1])
	}
	report, err := buildEnvironmentRestoreReport(context.Background(), store.Environment{
		ID: "env.edge.assets",
		ComposeJSON: `{
			"composeFile":"compose.yml",
			"generatedFiles":{
				"compose.yml":"services:\n  mysql:\n    image: mysql:8\n  apollo:\n    image: wiremock/wiremock\n  app:\n    image: alpine:3.20\n",
				"compose/platform/apollo/mappings/app.json":"{\"request\":{\"url\":\"/configs/app\"},\"response\":{\"status\":200}}\n"
			},
			"services":["mysql","apollo","app"],
			"skipPull":true,
			"skipBuild":true
		}`,
		HealthChecksJSON:       `[]`,
		VerificationWorkflowID: "workflow.edge-assets",
	}, workspace, true, false, false, time.Second, environmentRestoreWorkflowOptions{}, environmentRestoreDockerCleanupOptions{}, store.EnvironmentComponentGraph{
		Components: []store.EnvironmentComponent{
			{ComponentID: "mysql", Kind: "middleware", Role: "database", ComposeService: "mysql", Required: true, HealthCheckJSON: `{"type":"compose-service","service":"mysql"}`},
			{ComponentID: "apollo", Kind: "middleware", Role: "config", ComposeService: "apollo", Required: true, HealthCheckJSON: `{"type":"compose-service","service":"apollo"}`},
			{ComponentID: "app", Kind: "app", Role: "business-service", ComposeService: "app", Required: true, HealthCheckJSON: `{"type":"url","url":"` + healthServer.URL + `/health"}`},
		},
		Dependencies: []store.ComponentDependency{
			{ConsumerComponentID: "app", ProviderComponentID: "mysql", Phase: "startup", Capability: "sql", Required: true, ProfileJSON: `{"assetIds":["app.mysql.schema"]}`},
			{ConsumerComponentID: "app", ProviderComponentID: "apollo", Phase: "startup", Capability: "config", Required: true, ProfileJSON: `{"assetIds":["app.apollo.config"]}`},
		},
		Assets: []store.ComponentConfigAsset{
			{OwnerComponentID: "app", AssetID: "app.mysql.schema", AssetKind: "mysql-ddl", TargetComponentID: "mysql", TargetPath: "compose/mysql/init/app.sql", ContentInline: "create database if not exists app;\n", SizeBytes: int64(len("create database if not exists app;\n")), ApplyOrder: 10, SummaryJSON: `{}`},
			{OwnerComponentID: "app", AssetID: "app.apollo.config", AssetKind: "apollo-config", TargetComponentID: "apollo", TargetPath: "compose/platform/apollo/mappings/app.json", ContentInline: "{\"request\":{\"url\":\"/configs/app\"},\"response\":{\"status\":200}}\n", ApplyOrder: 20, SummaryJSON: `{}`},
		},
	})
	if err != nil {
		t.Fatalf("build edge asset restore report: %v", err)
	}
	if !report.OK || !report.Docker.OK || len(report.Docker.AppliedAssets) != 2 {
		t.Fatalf("edge asset restore report = %#v", report.Docker)
	}
	actions := map[string]string{}
	for _, asset := range report.Docker.AppliedAssets {
		actions[asset.AssetID] = asset.Action
	}
	if actions["app.mysql.schema"] != "apply-mysql-sql" || actions["app.apollo.config"] != "verify-generated-file" {
		t.Fatalf("edge asset actions = %#v assets=%#v", actions, report.Docker.AppliedAssets)
	}
	dockerCalls, err := os.ReadFile(dockerCallsPath)
	if err != nil {
		t.Fatalf("read fake docker calls: %v", err)
	}
	if !strings.Contains(string(dockerCalls), "compose -f "+filepath.Join(workspace, "compose.yml")+" up -d mysql apollo app") ||
		!strings.Contains(string(dockerCalls), "compose -f "+filepath.Join(workspace, "compose.yml")+" exec -T mysql sh -lc") ||
		strings.Contains(string(dockerCalls), "-proot") {
		t.Fatalf("edge asset docker calls:\n%s", dockerCalls)
	}
}

func TestEnvironmentRestoreEdgeAssetsAvoidNonSQLMySQLAndDuplicateApply(t *testing.T) {
	workspace := filepath.Join(t.TempDir(), "workspace")
	writeFile(t, filepath.Join(workspace, "compose", "mysql", "config.cnf"), "[mysqld]\n")
	graph := store.EnvironmentComponentGraph{
		Components: []store.EnvironmentComponent{
			{ComponentID: "mysql", Kind: "middleware", Role: "database", ComposeService: "mysql"},
			{ComponentID: "app", Kind: "app", Role: "business-service", ComposeService: "app"},
			{ComponentID: "worker", Kind: "app", Role: "worker", ComposeService: "worker"},
		},
		Dependencies: []store.ComponentDependency{
			{ConsumerComponentID: "app", ProviderComponentID: "mysql", Capability: "config", ProfileJSON: `{"assetIds":["mysql.config"]}`},
			{ConsumerComponentID: "app", ProviderComponentID: "mysql", Capability: "sql", ProfileJSON: `{"assetIds":["shared.schema"]}`},
			{ConsumerComponentID: "worker", ProviderComponentID: "mysql", Capability: "sql", ProfileJSON: `{"assetIds":["shared.schema"]}`},
		},
		Assets: []store.ComponentConfigAsset{
			{OwnerComponentID: "mysql", AssetID: "mysql.config", AssetKind: "mysql-config", TargetComponentID: "mysql", TargetPath: "compose/mysql/config.cnf"},
			{OwnerComponentID: "app", AssetID: "shared.schema", AssetKind: "mysql-ddl", TargetComponentID: "mysql", TargetPath: "compose/mysql/init/shared.sql", ContentInline: "create database if not exists app;\n"},
		},
	}
	items := environmentRestoreApplyEdgeAssets(context.Background(), graph, map[string]any{
		"generatedFiles": map[string]any{
			"compose/mysql/config.cnf": "[mysqld]\n",
		},
	}, workspace, false, []string{"-f", "compose.yml"})
	if len(items) != 2 {
		t.Fatalf("edge assets should dedupe repeated asset ids, got %#v", items)
	}
	actions := map[string]string{}
	commands := map[string]string{}
	for _, item := range items {
		actions[item.AssetID] = item.Action
		commands[item.AssetID] = strings.Join(item.Command, " ")
	}
	if actions["mysql.config"] != "project-generated-file" || commands["mysql.config"] != "" {
		t.Fatalf("non-SQL MySQL asset should not run through mysql client: actions=%#v commands=%#v", actions, commands)
	}
	if actions["shared.schema"] != "plan-apply-mysql-sql" || strings.Contains(commands["shared.schema"], "-proot") || !strings.Contains(commands["shared.schema"], "MYSQL_ROOT_PASSWORD") {
		t.Fatalf("SQL MySQL asset command should use container env credentials: actions=%#v commands=%#v", actions, commands)
	}
	if strings.Contains(commands["shared.schema"], "MYSQL_DATABASE") || !strings.Contains(commands["shared.schema"], "AGENT_TESTBENCH_MYSQL_APPLY_DATABASE") {
		t.Fatalf("SQL MySQL asset command should not force MYSQL_DATABASE by default: %#v", commands)
	}
}

func TestEnvironmentRestoreEdgeAssetsRequireMySQLProviderSignal(t *testing.T) {
	workspace := t.TempDir()
	graph := store.EnvironmentComponentGraph{
		Components: []store.EnvironmentComponent{
			{ComponentID: "postgres", Kind: "middleware", Role: "database", ComposeService: "postgres"},
			{ComponentID: "mysql.primary", Kind: "middleware", Role: "database", ComposeService: "mysql"},
			{ComponentID: "app", Kind: "app", Role: "business-service", ComposeService: "app"},
			{ComponentID: "worker", Kind: "app", Role: "worker", ComposeService: "worker"},
		},
		Dependencies: []store.ComponentDependency{
			{ConsumerComponentID: "app", ProviderComponentID: "postgres", Capability: "sql", ProfileJSON: `{"assetIds":["postgres.schema"]}`},
			{ConsumerComponentID: "app", ProviderComponentID: "mysql.primary", Capability: "sql", ProfileJSON: `{"assetIds":["shared.schema"]}`},
			{ConsumerComponentID: "worker", ProviderComponentID: "postgres", Capability: "sql", ProfileJSON: `{"assetIds":["shared.schema"]}`},
		},
		Assets: []store.ComponentConfigAsset{
			{OwnerComponentID: "app", AssetID: "postgres.schema", AssetKind: "postgres-ddl", TargetComponentID: "postgres", TargetPath: "postgres.sql", ContentInline: "create schema app;\n"},
			{OwnerComponentID: "app", AssetID: "shared.schema", AssetKind: "schema", TargetPath: "shared.sql", ContentInline: "create database if not exists shared;\n"},
		},
	}
	items := environmentRestoreApplyEdgeAssets(context.Background(), graph, nil, workspace, false, []string{"-f", "compose.yml"})
	if len(items) != 3 {
		t.Fatalf("shared asset should be applied once per effective target, got %#v", items)
	}
	actionsByTarget := map[string]string{}
	for _, item := range items {
		actionsByTarget[item.AssetID+"@"+item.TargetComponentID] = item.Action
	}
	if actionsByTarget["postgres.schema@postgres"] == "plan-apply-mysql-sql" {
		t.Fatalf("postgres SQL asset should not use MySQL apply: %#v", actionsByTarget)
	}
	if actionsByTarget["shared.schema@mysql.primary"] != "plan-apply-mysql-sql" {
		t.Fatalf("shared schema should use MySQL apply for MySQL target: %#v", actionsByTarget)
	}
	if actionsByTarget["shared.schema@postgres"] == "plan-apply-mysql-sql" {
		t.Fatalf("shared schema should not use MySQL apply for PostgreSQL target: %#v", actionsByTarget)
	}
}

func TestEnvironmentRestoreEdgeAssetContentRejectsParentPath(t *testing.T) {
	item := environmentRestoreApplyEdgeAsset(context.Background(),
		store.ComponentDependency{ConsumerComponentID: "app", ProviderComponentID: "mysql", Capability: "sql", ProfileJSON: `{"assetIds":["bad.schema"]}`},
		store.ComponentConfigAsset{OwnerComponentID: "app", AssetID: "bad.schema", AssetKind: "mysql-ddl", TargetComponentID: "mysql", TargetPath: ".."},
		map[string]store.EnvironmentComponent{"mysql": {ComponentID: "mysql", ComposeService: "mysql"}},
		nil,
		t.TempDir(),
		false,
		[]string{"-f", "compose.yml"},
	)
	if item.OK || !strings.Contains(item.Error, "target path is required") {
		t.Fatalf("parent path edge asset should be rejected: %#v", item)
	}
}

func TestEnvironmentRestoreRetriesMySQLAssetUntilServiceReady(t *testing.T) {
	workspace := t.TempDir()
	command, callsPath := fakeMySQLApplyCommandWithFirstFailure(t)
	attempts, errText := runRestoreMySQLCommandWithInputRetry(context.Background(), workspace, command, "create database if not exists app;\n")
	if errText != "" {
		t.Fatalf("mysql retry command failed: %s", errText)
	}
	if attempts != 2 {
		t.Fatalf("mysql asset attempts = %d, want 2", attempts)
	}
	calls, err := os.ReadFile(callsPath)
	if err != nil {
		t.Fatalf("read mysql retry calls: %v", err)
	}
	if got := strings.Count(string(calls), "apply"); got != 2 {
		t.Fatalf("mysql command calls = %d, want 2\n%s", got, calls)
	}
}

func TestEnvironmentRestoreRunsMixedHealthProbes(t *testing.T) {
	storePath := filepath.Join(t.TempDir(), "store.sqlite")
	workspace := filepath.Join(t.TempDir(), "workspace")
	fakeDockerEnv, _ := fakeDockerCommand(t)
	healthServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer healthServer.Close()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen tcp health: %v", err)
	}
	defer func() { _ = listener.Close() }()
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			_ = conn.Close()
		}
	}()
	writeFile(t, filepath.Join(workspace, "compose.yml"), "services: {}\n")

	runCLI(t, "environment", "register",
		"--store", "sqlite://"+storePath,
		"--id", "env.health.mixed",
		"--compose-file", "compose.yml",
		"--health-url", healthServer.URL+"/ready",
		"--health-tcp", listener.Addr().String(),
		"--health-command", "test -f compose.yml",
		"--health-compose-service", "web",
		"--verification-workflow", "workflow.core-10",
	)

	out := runCLIWithEnv(t, fakeDockerEnv, "environment", "restore", "--store", "sqlite://"+storePath, "--workspace", workspace, "--execute", "--json", "env.health.mixed")
	var report struct {
		OK     bool `json:"ok"`
		Docker struct {
			HealthChecks []struct {
				Kind    string `json:"kind"`
				OK      bool   `json:"ok"`
				State   string `json:"state"`
				Health  string `json:"health"`
				Service string `json:"service"`
			} `json:"healthChecks"`
		} `json:"docker"`
	}
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatalf("decode mixed health restore json: %v\n%s", err, out)
	}
	if !report.OK || len(report.Docker.HealthChecks) != 4 {
		t.Fatalf("mixed health report = %#v", report)
	}
	seen := map[string]bool{}
	for _, check := range report.Docker.HealthChecks {
		if !check.OK {
			t.Fatalf("mixed health check failed: %#v", check)
		}
		seen[check.Kind] = true
		if check.Kind == "compose-service" && (check.Service != "web" || check.State != "running" || check.Health != "healthy") {
			t.Fatalf("compose service health = %#v", check)
		}
	}
	for _, kind := range []string{"url", "tcp", "command", "compose-service"} {
		if !seen[kind] {
			t.Fatalf("missing health kind %s in %#v", kind, report.Docker.HealthChecks)
		}
	}
}

func TestEnvironmentRestoreFailsWhenHealthProbeFails(t *testing.T) {
	storePath := filepath.Join(t.TempDir(), "store.sqlite")
	workspace := filepath.Join(t.TempDir(), "workspace")
	fakeDockerEnv, _ := fakeDockerCommand(t)
	writeFile(t, filepath.Join(workspace, "compose.yml"), "services: {}\n")
	runCLI(t, "environment", "register",
		"--store", "sqlite://"+storePath,
		"--id", "env.health.fail",
		"--compose-file", "compose.yml",
		"--health-command", "echo nope && exit 7",
		"--verification-workflow", "workflow.core-10",
	)

	out := runCLIFailsWithEnv(t, fakeDockerEnv, "environment", "restore", "--store", "sqlite://"+storePath, "--workspace", workspace, "--execute", "--health-timeout-seconds", "1", "--json", "env.health.fail")
	if !strings.Contains(out, `"kind": "command"`) || !strings.Contains(out, "exit status 7") {
		t.Fatalf("health failure output = %q", out)
	}
	inspectOut := runCLI(t, "environment", "inspect", "--store", "sqlite://"+storePath, "--json", "env.health.fail")
	if !strings.Contains(inspectOut, `"phase": "health-check"`) {
		t.Fatalf("health failure should persist health-check phase: %s", inspectOut)
	}
}

func TestEnvironmentRestoreHonorsComposeOptionsFromStore(t *testing.T) {
	storePath := filepath.Join(t.TempDir(), "store.sqlite")
	workspace := filepath.Join(t.TempDir(), "workspace")
	fakeDockerEnv, dockerCallsPath := fakeDockerCommand(t)
	writeFile(t, filepath.Join(workspace, "compose.yml"), "services: {}\n")
	writeFile(t, filepath.Join(workspace, ".env.local"), "MODE=local\n")
	runCLI(t, "environment", "register",
		"--store", "sqlite://"+storePath,
		"--id", "env.compose.options",
		"--compose-file", "compose.yml",
		"--compose-project-name", "demo",
		"--compose-env-file", ".env.local",
		"--compose-profile", "api",
		"--compose-service", "web",
		"--compose-skip-pull",
		"--compose-skip-build",
		"--health-url", newHealthyTestURL(t),
		"--verification-workflow", "workflow.core-10",
	)

	out := runCLIWithEnv(t, fakeDockerEnv, "environment", "restore", "--store", "sqlite://"+storePath, "--workspace", workspace, "--execute", "--json", "env.compose.options")
	var report struct {
		OK      bool `json:"ok"`
		Compose struct {
			ProjectName string   `json:"projectName"`
			EnvFiles    []string `json:"envFiles"`
			Profiles    []string `json:"profiles"`
			Services    []string `json:"services"`
			SkipPull    bool     `json:"skipPull"`
			SkipBuild   bool     `json:"skipBuild"`
		} `json:"compose"`
		Docker struct {
			Commands     [][]string `json:"commands"`
			HealthChecks []struct {
				Kind    string `json:"kind"`
				Service string `json:"service"`
				State   string `json:"state"`
				OK      bool   `json:"ok"`
			} `json:"healthChecks"`
		} `json:"docker"`
	}
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatalf("decode compose options restore json: %v\n%s", err, out)
	}
	if !report.OK || report.Compose.ProjectName != "demo" || len(report.Compose.EnvFiles) != 1 || len(report.Compose.Profiles) != 1 || len(report.Compose.Services) != 1 || !report.Compose.SkipPull || !report.Compose.SkipBuild {
		t.Fatalf("compose options report = %#v", report)
	}
	if len(report.Docker.Commands) != 1 {
		t.Fatalf("compose options should only run up command, got %#v", report.Docker.Commands)
	}
	foundComposeServiceHealth := false
	for _, check := range report.Docker.HealthChecks {
		if check.Kind == "compose-service" && check.Service == "web" && check.State == "running" && check.OK {
			foundComposeServiceHealth = true
		}
	}
	if !foundComposeServiceHealth {
		t.Fatalf("compose service readiness should be generated for requested service: %#v", report.Docker.HealthChecks)
	}
	want := "compose -f " + filepath.Join(workspace, "compose.yml") + " -p demo --env-file " + filepath.Join(workspace, ".env.local") + " --profile api up -d web"
	dockerCalls, err := os.ReadFile(dockerCallsPath)
	if err != nil {
		t.Fatalf("read fake docker calls: %v", err)
	}
	if strings.Contains(string(dockerCalls), " pull") || strings.Contains(string(dockerCalls), " build") || !strings.Contains(string(dockerCalls), want) {
		t.Fatalf("compose option docker calls want %q:\n%s", want, dockerCalls)
	}
	if !strings.Contains(string(dockerCalls), "compose -f "+filepath.Join(workspace, "compose.yml")+" -p demo --env-file "+filepath.Join(workspace, ".env.local")+" --profile api ps --format json web") {
		t.Fatalf("compose option docker calls should include service readiness check:\n%s", dockerCalls)
	}
}

func TestEnvironmentRestoreSupportsMultipleComposeFiles(t *testing.T) {
	storePath := filepath.Join(t.TempDir(), "store.sqlite")
	workspace := filepath.Join(t.TempDir(), "workspace")
	fakeDockerEnv, dockerCallsPath := fakeDockerCommand(t)
	writeFile(t, filepath.Join(workspace, "compose.base.yml"), "services: {}\n")
	writeFile(t, filepath.Join(workspace, "compose.apps.yml"), "services: {}\n")
	runCLI(t, "environment", "register",
		"--store", "sqlite://"+storePath,
		"--id", "env.compose.multi",
		"--compose-file", "compose.base.yml",
		"--compose-file", "compose.apps.yml",
		"--compose-env", "SANDBOX_ROOT=$AGENT_TESTBENCH_WORKSPACE",
		"--compose-skip-pull",
		"--compose-skip-build",
		"--health-compose-service", "web",
		"--verification-workflow", "workflow.core-10",
	)

	out := runCLIWithEnv(t, fakeDockerEnv, "environment", "restore", "--store", "sqlite://"+storePath, "--workspace", workspace, "--execute", "--json", "env.compose.multi")
	var report struct {
		OK      bool `json:"ok"`
		Compose struct {
			ComposeFile  string   `json:"composeFile"`
			ComposeFiles []string `json:"composeFiles"`
		} `json:"compose"`
		Docker struct {
			ComposeFile string     `json:"composeFile"`
			Commands    [][]string `json:"commands"`
		} `json:"docker"`
	}
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatalf("decode multi compose restore json: %v\n%s", err, out)
	}
	if !report.OK || report.Compose.ComposeFile != "compose.base.yml" || len(report.Compose.ComposeFiles) != 2 || !strings.Contains(report.Docker.ComposeFile, "compose.base.yml") || !strings.Contains(report.Docker.ComposeFile, "compose.apps.yml") {
		t.Fatalf("multi compose report = %#v", report)
	}
	dockerCalls, err := os.ReadFile(dockerCallsPath)
	if err != nil {
		t.Fatalf("read fake docker calls: %v", err)
	}
	want := "compose -f " + filepath.Join(workspace, "compose.base.yml") + " -f " + filepath.Join(workspace, "compose.apps.yml") + " up -d"
	want = strings.Replace(want, " up -d", " --env-file "+filepath.Join(workspace, ".agent-testbench", "restore.env")+" up -d", 1)
	if !strings.Contains(string(dockerCalls), want) {
		t.Fatalf("multi compose docker calls missing %q:\n%s", want, dockerCalls)
	}
	envFile, err := os.ReadFile(filepath.Join(workspace, ".agent-testbench", "restore.env"))
	if err != nil || !strings.Contains(string(envFile), "SANDBOX_ROOT="+workspace) {
		t.Fatalf("generated compose env file = %q err=%v", envFile, err)
	}
}

func TestEnvironmentRestoreDoesNotPullComposeBuildServices(t *testing.T) {
	storePath := filepath.Join(t.TempDir(), "store.sqlite")
	workspace := filepath.Join(t.TempDir(), "workspace")
	composeSource := filepath.Join(t.TempDir(), "compose.yml")
	fakeDockerEnv, dockerCallsPath := fakeDockerCommand(t)
	writeFile(t, composeSource, `services:
  web:
    image: nginx:alpine
  llt:
    build:
      context: ${DOCKER_LLT_SIMULATOR_REPO}
    image: agent-testbench/llt-simulator:local
`)
	runCLI(t, "environment", "register",
		"--store", "sqlite://"+storePath,
		"--id", "env.compose.build-filter",
		"--compose-file", "compose/docker-compose.yml",
		"--compose-generated-file", "compose/docker-compose.yml="+composeSource,
		"--compose-env", "DOCKER_LLT_SIMULATOR_REPO=$AGENT_TESTBENCH_WORKSPACE/agent-testbench-llt-simulator",
		"--compose-service", "web",
		"--compose-service", "llt",
		"--verification-workflow", "workflow.core-10",
	)

	out := runCLIWithEnv(t, fakeDockerEnv, "environment", "restore", "--store", "sqlite://"+storePath, "--workspace", workspace, "--execute", "--json", "env.compose.build-filter")
	var report struct {
		OK bool `json:"ok"`
	}
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatalf("decode build service restore json: %v\n%s", err, out)
	}
	if !report.OK {
		t.Fatalf("build service restore report = %#v\n%s", report, out)
	}
	dockerCalls, err := os.ReadFile(dockerCallsPath)
	if err != nil {
		t.Fatalf("read fake docker calls: %v", err)
	}
	calls := string(dockerCalls)
	if !strings.Contains(calls, " pull web\n") || strings.Contains(calls, " pull web llt") || strings.Contains(calls, " pull llt") {
		t.Fatalf("pull should include image services only:\n%s", calls)
	}
	if !strings.Contains(calls, " build llt\n") || strings.Contains(calls, " build web") {
		t.Fatalf("build should include build services only:\n%s", calls)
	}
	if !strings.Contains(calls, " up -d web llt") {
		t.Fatalf("up should still include all requested services:\n%s", calls)
	}
}

func TestEnvironmentRestoreCanPrepareRepositoriesBeforeDocker(t *testing.T) {
	storePath := filepath.Join(t.TempDir(), "store.sqlite")
	remoteRepo := createBareGitRepo(t, "main")
	workspace := filepath.Join(t.TempDir(), "workspace")
	fakeDockerEnv, dockerCallsPath := fakeDockerCommand(t)
	healthServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer healthServer.Close()
	writeFile(t, filepath.Join(workspace, "compose.yml"), "services: {}\n")
	runCLI(t, "environment", "register",
		"--store", "sqlite://"+storePath,
		"--id", "env.prepare.repos",
		"--service", "entry-gateway",
		"--repo", "entry-gateway="+remoteRepo,
		"--checkout", "entry-gateway=entry-gateway",
		"--compose-file", "compose.yml",
		"--health-url", healthServer.URL+"/ready",
		"--verification-workflow", "workflow.core-10",
	)

	out := runCLIWithEnv(t, fakeDockerEnv, "environment", "restore", "--store", "sqlite://"+storePath, "--workspace", workspace, "--execute", "--prepare-repos-only", "--json", "env.prepare.repos")
	var report struct {
		OK       bool `json:"ok"`
		Executed bool `json:"executed"`
		Repos    []struct {
			ServiceID string `json:"serviceId"`
			Action    string `json:"action"`
			OK        bool   `json:"ok"`
		} `json:"repos"`
		Docker struct {
			OK     bool   `json:"ok"`
			Action string `json:"action"`
		} `json:"docker"`
		Readiness struct {
			OK bool `json:"ok"`
		} `json:"readiness"`
	}
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatalf("decode prepare repos restore json: %v\n%s", err, out)
	}
	if !report.OK || !report.Executed || len(report.Repos) != 1 || report.Repos[0].Action != "clone" || !report.Repos[0].OK || !report.Docker.OK || report.Docker.Action != "skipped-after-repository-preparation" || !report.Readiness.OK {
		t.Fatalf("prepare repos report = %#v", report)
	}
	if _, err := os.Stat(filepath.Join(workspace, "entry-gateway", ".git")); err != nil {
		t.Fatalf("repository was not cloned before Docker: %v", err)
	}
	dockerCalls, err := os.ReadFile(dockerCallsPath)
	if err != nil {
		t.Fatalf("read fake docker calls: %v", err)
	}
	if strings.Contains(string(dockerCalls), " compose ") {
		t.Fatalf("prepare repos should not invoke Docker Compose:\n%s", dockerCalls)
	}
}

func TestEnvironmentRestoreCanPreparePackageRepositoryBeforeDocker(t *testing.T) {
	storePath := filepath.Join(t.TempDir(), "store.sqlite")
	packageRepo := createBareGitRepoWithFiles(t, "main", map[string]string{
		"compose/docker-compose.yml": "services: {}\n",
		"README.md":                  "# environment package\n",
	})
	workspace := filepath.Join(t.TempDir(), "workspace")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatalf("create empty workspace: %v", err)
	}
	fakeDockerEnv, dockerCallsPath := fakeDockerCommand(t)
	healthServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer healthServer.Close()
	runCLI(t, "environment", "register",
		"--store", "sqlite://"+storePath,
		"--id", "env.package.prepare",
		"--package-repo", packageRepo,
		"--package-branch", "main",
		"--compose-file", "compose/docker-compose.yml",
		"--health-url", healthServer.URL+"/ready",
		"--verification-workflow", "workflow.core-10",
	)

	out := runCLIWithEnv(t, fakeDockerEnv, "environment", "restore", "--store", "sqlite://"+storePath, "--workspace", workspace, "--execute", "--prepare-repos-only", "--json", "env.package.prepare")
	var report struct {
		OK      bool `json:"ok"`
		Package struct {
			Configured bool   `json:"configured"`
			Action     string `json:"action"`
			OK         bool   `json:"ok"`
			Checkout   string `json:"checkout"`
		} `json:"package"`
		Repos  []any `json:"repos"`
		Docker struct {
			OK     bool   `json:"ok"`
			Action string `json:"action"`
		} `json:"docker"`
		Readiness struct {
			OK    bool `json:"ok"`
			Items []struct {
				Name   string `json:"name"`
				OK     bool   `json:"ok"`
				Detail string `json:"detail"`
			} `json:"items"`
		} `json:"readiness"`
	}
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatalf("decode package prepare restore json: %v\n%s", err, out)
	}
	if !report.OK || !report.Package.Configured || report.Package.Action != "clone" || !report.Package.OK || report.Package.Checkout != workspace || len(report.Repos) != 0 || !report.Docker.OK || report.Docker.Action != "skipped-after-repository-preparation" || !report.Readiness.OK {
		t.Fatalf("package prepare report = %#v", report)
	}
	if !restoreReadinessHasItem(report.Readiness.Items, "environment-package", true, "environment package") {
		t.Fatalf("readiness should include package gate: %#v", report.Readiness.Items)
	}
	if raw, err := os.ReadFile(filepath.Join(workspace, "compose", "docker-compose.yml")); err != nil || !strings.Contains(string(raw), "services") {
		t.Fatalf("package compose file missing raw=%q err=%v", raw, err)
	}
	dockerCalls, err := os.ReadFile(dockerCallsPath)
	if err != nil {
		t.Fatalf("read fake docker calls: %v", err)
	}
	if strings.Contains(string(dockerCalls), " compose ") {
		t.Fatalf("prepare package should not invoke Docker Compose:\n%s", dockerCalls)
	}
}

func TestEnvironmentRestoreWritesStoreGeneratedComposeFileBeforeDocker(t *testing.T) {
	storePath := filepath.Join(t.TempDir(), "store.sqlite")
	workspace := filepath.Join(t.TempDir(), "workspace")
	sourceCompose := filepath.Join(t.TempDir(), "source-compose.yml")
	writeFile(t, sourceCompose, "services:\n  generated-service:\n    image: alpine:3.20\n")
	fakeDockerEnv, dockerCallsPath := fakeDockerCommand(t)
	healthServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer healthServer.Close()

	runCLI(t, "environment", "register",
		"--store", "sqlite://"+storePath,
		"--id", "env.generated.compose",
		"--compose-file", "compose/docker-compose.yml",
		"--compose-generated-file", "compose/docker-compose.yml="+sourceCompose,
		"--compose-skip-pull",
		"--compose-skip-build",
		"--health-url", healthServer.URL+"/ready",
		"--verification-workflow", "workflow.core-10",
	)

	dryRunOut := runCLIWithEnv(t, fakeDockerEnv, "environment", "restore", "--store", "sqlite://"+storePath, "--workspace", workspace, "--json", "env.generated.compose")
	var dryRun struct {
		OK     bool `json:"ok"`
		Docker struct {
			Generated []struct {
				Path   string `json:"path"`
				Action string `json:"action"`
				OK     bool   `json:"ok"`
			} `json:"generatedFiles"`
		} `json:"docker"`
	}
	if err := json.Unmarshal([]byte(dryRunOut), &dryRun); err != nil {
		t.Fatalf("decode generated compose dry-run json: %v\n%s", err, dryRunOut)
	}
	generatedPath := filepath.Join(workspace, "compose", "docker-compose.yml")
	if !dryRun.OK || len(dryRun.Docker.Generated) != 1 || dryRun.Docker.Generated[0].Action != "plan-write" || dryRun.Docker.Generated[0].Path != generatedPath || !dryRun.Docker.Generated[0].OK {
		t.Fatalf("generated compose dry-run = %#v", dryRun)
	}
	if _, err := os.Stat(generatedPath); !os.IsNotExist(err) {
		t.Fatalf("dry-run should not write generated compose file, stat err=%v", err)
	}

	executeOut := runCLIWithEnv(t, fakeDockerEnv, "environment", "restore", "--store", "sqlite://"+storePath, "--workspace", workspace, "--execute", "--json", "env.generated.compose")
	var executed struct {
		OK     bool `json:"ok"`
		Docker struct {
			Action    string `json:"action"`
			Generated []struct {
				Path   string `json:"path"`
				Action string `json:"action"`
				OK     bool   `json:"ok"`
			} `json:"generatedFiles"`
		} `json:"docker"`
	}
	if err := json.Unmarshal([]byte(executeOut), &executed); err != nil {
		t.Fatalf("decode generated compose execute json: %v\n%s", err, executeOut)
	}
	if !executed.OK || executed.Docker.Action != "run-docker-compose" || len(executed.Docker.Generated) != 1 || executed.Docker.Generated[0].Action != "write" || !executed.Docker.Generated[0].OK {
		t.Fatalf("generated compose execute = %#v", executed)
	}
	if raw, err := os.ReadFile(generatedPath); err != nil || !strings.Contains(string(raw), "generated-service") {
		t.Fatalf("generated compose file raw=%q err=%v", raw, err)
	}
	dockerCalls, err := os.ReadFile(dockerCallsPath)
	if err != nil {
		t.Fatalf("read fake docker calls: %v", err)
	}
	if !strings.Contains(string(dockerCalls), "compose -f "+generatedPath+" up -d") {
		t.Fatalf("fake docker calls should use generated compose file:\n%s", dockerCalls)
	}
}

func TestEnvironmentRestorePrepareReposOnlyWritesStoreGeneratedComposeFile(t *testing.T) {
	storePath := filepath.Join(t.TempDir(), "store.sqlite")
	workspace := filepath.Join(t.TempDir(), "workspace")
	sourceCompose := filepath.Join(t.TempDir(), "source-compose.yml")
	writeFile(t, sourceCompose, "services:\n  generated-service:\n    image: alpine:3.20\n")
	fakeDockerEnv, dockerCallsPath := fakeDockerCommand(t)
	healthServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer healthServer.Close()
	runCLI(t, "environment", "register",
		"--store", "sqlite://"+storePath,
		"--id", "env.generated.prepare",
		"--compose-file", "compose/docker-compose.yml",
		"--compose-generated-file", "compose/docker-compose.yml="+sourceCompose,
		"--health-url", healthServer.URL+"/ready",
		"--verification-workflow", "workflow.core-10",
	)

	out := runCLIWithEnv(t, fakeDockerEnv, "environment", "restore", "--store", "sqlite://"+storePath, "--workspace", workspace, "--execute", "--prepare-repos-only", "--json", "env.generated.prepare")
	var report struct {
		OK     bool `json:"ok"`
		Docker struct {
			Action    string `json:"action"`
			Generated []struct {
				Path   string `json:"path"`
				Action string `json:"action"`
				OK     bool   `json:"ok"`
			} `json:"generatedFiles"`
		} `json:"docker"`
	}
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatalf("decode generated prepare-only restore json: %v\n%s", err, out)
	}
	generatedPath := filepath.Join(workspace, "compose", "docker-compose.yml")
	if !report.OK || report.Docker.Action != "skipped-after-repository-preparation" || len(report.Docker.Generated) != 1 || report.Docker.Generated[0].Action != "write" || report.Docker.Generated[0].Path != generatedPath || !report.Docker.Generated[0].OK {
		t.Fatalf("generated prepare-only report = %#v", report)
	}
	if raw, err := os.ReadFile(generatedPath); err != nil || !strings.Contains(string(raw), "generated-service") {
		t.Fatalf("generated compose file raw=%q err=%v", raw, err)
	}
	dockerCalls, err := os.ReadFile(dockerCallsPath)
	if err != nil {
		t.Fatalf("read fake docker calls: %v", err)
	}
	if strings.Contains(string(dockerCalls), " compose ") {
		t.Fatalf("prepare-only should not invoke Docker Compose:\n%s", dockerCalls)
	}
}

func TestEnvironmentRestoreBlocksDockerWhenContainerNamesAlreadyExist(t *testing.T) {
	workspace := filepath.Join(t.TempDir(), "workspace")
	fakeBin := t.TempDir()
	writeFile(t, filepath.Join(fakeBin, "git"), "#!/bin/sh\nexit 0\n")
	writeFile(t, filepath.Join(fakeBin, "docker"), "#!/bin/sh\nif [ \"$1\" = compose ] && [ \"$2\" = version ]; then exit 0; fi\nif [ \"$1\" = ps ]; then printf 'sandbox-mysql\\n'; exit 0; fi\nexit 0\n")
	if err := os.Chmod(filepath.Join(fakeBin, "git"), 0o755); err != nil {
		t.Fatalf("chmod fake git: %v", err)
	}
	if err := os.Chmod(filepath.Join(fakeBin, "docker"), 0o755); err != nil {
		t.Fatalf("chmod fake docker: %v", err)
	}
	t.Setenv("PATH", fakeBin)

	report, err := buildEnvironmentRestoreReport(context.Background(), store.Environment{
		ID:                     "env.container.conflict",
		ComposeJSON:            `{"composeFile":"compose.yml","generatedFiles":{"compose.yml":"services:\n  mysql:\n    image: mysql:8\n    container_name: sandbox-mysql\n"}}`,
		HealthChecksJSON:       `[{"kind":"url","url":"http://127.0.0.1:18080/health"}]`,
		VerificationWorkflowID: "workflow.core-10",
	}, workspace, false, false, false, time.Second, environmentRestoreWorkflowOptions{}, environmentRestoreDockerCleanupOptions{})
	if err != nil {
		t.Fatalf("build restore container conflict report: %v", err)
	}
	if report.OK || report.Preflight.OK || len(report.Preflight.ContainerConflicts) != 1 || report.Preflight.ContainerConflicts[0] != "sandbox-mysql" {
		t.Fatalf("container conflict report = %#v", report)
	}
	if !restoreTypedReadinessHasItem(report.Readiness.Items, "docker-container-conflicts", false, "sandbox-mysql") {
		t.Fatalf("readiness should include container conflict: %#v", report.Readiness.Items)
	}
}

func TestEnvironmentRestoreAssumeCleanDockerIgnoresLocalContainerConflicts(t *testing.T) {
	workspace := filepath.Join(t.TempDir(), "workspace")
	fakeBin := t.TempDir()
	writeFile(t, filepath.Join(fakeBin, "git"), "#!/bin/sh\nexit 0\n")
	writeFile(t, filepath.Join(fakeBin, "docker"), "#!/bin/sh\nif [ \"$1\" = compose ] && [ \"$2\" = version ]; then exit 0; fi\nif [ \"$1\" = ps ]; then printf 'sandbox-mysql\\n'; exit 0; fi\nexit 0\n")
	if err := os.Chmod(filepath.Join(fakeBin, "git"), 0o755); err != nil {
		t.Fatalf("chmod fake git: %v", err)
	}
	if err := os.Chmod(filepath.Join(fakeBin, "docker"), 0o755); err != nil {
		t.Fatalf("chmod fake docker: %v", err)
	}
	t.Setenv("PATH", fakeBin)

	report, err := buildEnvironmentRestoreReport(context.Background(), store.Environment{
		ID:                     "env.clean-machine",
		ComposeJSON:            `{"composeFile":"compose.yml","generatedFiles":{"compose.yml":"services:\n  mysql:\n    image: mysql:8\n    container_name: sandbox-mysql\n"}}`,
		HealthChecksJSON:       `[{"kind":"url","url":"http://127.0.0.1:18080/health"}]`,
		VerificationWorkflowID: "workflow.core-10",
	}, workspace, false, false, false, time.Second, environmentRestoreWorkflowOptions{}, environmentRestoreDockerCleanupOptions{AssumeCleanDocker: true}, store.EnvironmentComponentGraph{
		Components: []store.EnvironmentComponent{
			{ComponentID: "mysql", Kind: "middleware", Role: "database", ComposeService: "mysql", Required: true, HealthCheckJSON: `{"kind":"compose-service","service":"mysql"}`},
			{ComponentID: "gateway", Kind: "app", Role: "business-service", ComposeService: "gateway", Required: true, HealthCheckJSON: `{"kind":"url","url":"http://127.0.0.1:18080/health"}`},
		},
		Dependencies: []store.ComponentDependency{
			{ConsumerComponentID: "gateway", ProviderComponentID: "mysql", Required: true},
		},
		Assets: []store.ComponentConfigAsset{
			{OwnerComponentID: "mysql", AssetID: "mysql.schema", AssetKind: "mysql-ddl", TargetPath: "mysql/init/schema.sql", ContentInline: "create table demo(id bigint);"},
		},
	})
	if err != nil {
		t.Fatalf("build clean-machine restore report: %v", err)
	}
	if !report.OK || !report.Preflight.OK || !report.Preflight.AssumeCleanDocker || len(report.Preflight.ContainerConflicts) != 0 || report.Docker.Action != "plan-docker-compose" {
		t.Fatalf("clean-machine report should not be blocked by local containers: %#v", report)
	}
	if !restoreTypedReadinessHasItem(report.Readiness.Items, "docker-container-conflicts", true, "clean-machine dry-run") {
		t.Fatalf("readiness should document clean-machine assumption: %#v", report.Readiness.Items)
	}
	if report.Readiness.Action != "ready-for-clean-machine-execute" || !strings.Contains(report.Readiness.NextStep, "--execute") {
		t.Fatalf("clean-machine readiness should point to execute: %#v", report.Readiness)
	}
	if len(report.NextActions) == 0 || !strings.Contains(report.NextActions[0], "colleague machine") {
		t.Fatalf("clean-machine next actions should point to colleague machine: %#v", report.NextActions)
	}
	if !report.CleanMachine.Ready || strings.Join(report.CleanMachine.ExecuteCommand, " ") != "agent-testbench environment restore env.clean-machine --store STORE_NAME_OR_SQL_DSN --workspace "+workspace+" --execute --json" {
		t.Fatalf("clean-machine execute command = %#v", report.CleanMachine)
	}
	if strings.Join(report.CleanMachine.PrepareCommand, " ") != "agent-testbench environment restore env.clean-machine --store STORE_NAME_OR_SQL_DSN --workspace "+workspace+" --execute --prepare-repos-only --json" {
		t.Fatalf("clean-machine prepare command = %#v", report.CleanMachine)
	}
	if !restoreCleanMachinePrereqOK(report.CleanMachine.Prerequisites, "tool:docker") || !restoreCleanMachinePrereqOK(report.CleanMachine.Prerequisites, "docker-start-plan") {
		t.Fatalf("clean-machine prerequisites = %#v", report.CleanMachine.Prerequisites)
	}
	if report.CleanMachine.Summary.Components != 2 || report.CleanMachine.Summary.StartupBatches != 2 || report.CleanMachine.Summary.HealthGates != 2 {
		t.Fatalf("clean-machine component summary = %#v", report.CleanMachine.Summary)
	}
	if report.CleanMachine.Summary.InlineAssetBytes == 0 || report.CleanMachine.Summary.GraphMetadataLimitBytes != store.ComponentGraphMaxBytes || report.CleanMachine.Summary.DockerImagesStored || report.CleanMachine.Summary.LargeBinariesStored {
		t.Fatalf("clean-machine storage summary = %#v", report.CleanMachine.Summary)
	}
}

func restoreCleanMachinePrereqOK(items []environmentRestoreCleanMachinePrerequisite, name string) bool {
	for _, item := range items {
		if item.Name == name && item.OK {
			return true
		}
	}
	return false
}

func TestEnvironmentRestoreEffectiveHealthChecksUseStartedComposeServices(t *testing.T) {
	checks := []any{
		map[string]any{"id": "llt-url", "kind": "url", "url": "http://127.0.0.1:28080/health"},
	}
	compose := map[string]any{"services": []any{"app", "db"}}
	graph := store.EnvironmentComponentGraph{
		Components: []store.EnvironmentComponent{
			{ComponentID: "app", ComposeService: "app", HealthCheckJSON: `{"type":"compose-service","service":"app"}`},
			{ComponentID: "demo", ComposeService: "demo", HealthCheckJSON: `{"type":"compose-service","service":"demo"}`},
			{ComponentID: "db", ComposeService: "db", HealthCheckJSON: `{"type":"compose-service","service":"db"}`},
		},
	}
	effective := environmentRestoreEffectiveHealthChecks(checks, compose, graph)
	if !restoreHealthChecksContain(effective, "url", "", "http://127.0.0.1:28080/health") {
		t.Fatalf("explicit URL health check missing: %#v", effective)
	}
	if !restoreHealthChecksContain(effective, "compose-service", "app", "") || !restoreHealthChecksContain(effective, "compose-service", "db", "") {
		t.Fatalf("started service health checks missing: %#v", effective)
	}
	if restoreHealthChecksContain(effective, "compose-service", "demo", "") {
		t.Fatalf("unstarted component health check should be excluded: %#v", effective)
	}
}

func TestEnvironmentRestoreEffectiveHealthChecksCoverBusinessURLService(t *testing.T) {
	compose := map[string]any{"services": []any{"app"}}
	graph := store.EnvironmentComponentGraph{
		Components: []store.EnvironmentComponent{
			{
				ComponentID:     "app",
				Kind:            "app",
				Role:            "business-service",
				ComposeService:  "app",
				HealthCheckJSON: `{"type":"url","url":"http://127.0.0.1:18080/actuator/health"}`,
			},
		},
	}
	effective := environmentRestoreEffectiveHealthChecks(nil, compose, graph)
	if !restoreHealthChecksContain(effective, "url", "app", "http://127.0.0.1:18080/actuator/health") {
		t.Fatalf("business URL health check missing service binding: %#v", effective)
	}
	if restoreHealthChecksContain(effective, "compose-service", "app", "") {
		t.Fatalf("business service with URL health should not add compose-only health: %#v", effective)
	}
}

func restoreHealthChecksContain(items []any, kind string, service string, url string) bool {
	for _, raw := range items {
		item, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		if strings.TrimSpace(valueString(item["kind"])) != kind {
			continue
		}
		if service != "" && strings.TrimSpace(valueString(item["service"])) != service {
			continue
		}
		if url != "" && strings.TrimSpace(valueString(item["url"])) != url {
			continue
		}
		return true
	}
	return false
}

func TestEnvironmentRestoreCanAdoptExistingContainers(t *testing.T) {
	workspace := filepath.Join(t.TempDir(), "workspace")
	fakeBin := t.TempDir()
	writeFile(t, filepath.Join(fakeBin, "git"), "#!/bin/sh\nexit 0\n")
	writeFile(t, filepath.Join(fakeBin, "docker"), "#!/bin/sh\nif [ \"$1\" = compose ] && [ \"$2\" = version ]; then exit 0; fi\nif [ \"$1\" = ps ]; then printf 'sandbox-mysql\\n'; exit 0; fi\nif [ \"$1\" = inspect ]; then printf 'running healthy\\n'; exit 0; fi\nexit 0\n")
	if err := os.Chmod(filepath.Join(fakeBin, "git"), 0o755); err != nil {
		t.Fatalf("chmod fake git: %v", err)
	}
	if err := os.Chmod(filepath.Join(fakeBin, "docker"), 0o755); err != nil {
		t.Fatalf("chmod fake docker: %v", err)
	}
	t.Setenv("PATH", fakeBin)

	report, err := buildEnvironmentRestoreReport(context.Background(), store.Environment{
		ID:                     "env.adopt.container",
		ComposeJSON:            `{"composeFile":"compose.yml","services":["mysql"],"generatedFiles":{"compose.yml":"services:\n  mysql:\n    image: mysql:8\n    container_name: sandbox-mysql\n"}}`,
		HealthChecksJSON:       `[]`,
		VerificationWorkflowID: "workflow.core-10",
	}, workspace, true, false, false, time.Second, environmentRestoreWorkflowOptions{}, environmentRestoreDockerCleanupOptions{
		UseExistingContainers: true,
	})
	if err != nil {
		t.Fatalf("build restore adopt existing container report: %v", err)
	}
	if !report.OK || !report.Preflight.OK || report.Docker.Action != "use-existing-containers" || len(report.Docker.Commands) != 0 || len(report.Docker.HealthChecks) != 1 || !report.Docker.HealthChecks[0].OK || report.Docker.HealthChecks[0].Container != "sandbox-mysql" {
		t.Fatalf("adopt existing container report = %#v", report)
	}
	if !restoreTypedReadinessHasItem(report.Readiness.Items, "docker-container-conflicts", true, "explicitly adopted") {
		t.Fatalf("readiness should acknowledge explicit adoption: %#v", report.Readiness.Items)
	}
	if _, err := os.Stat(filepath.Join(workspace, "compose.yml")); err != nil {
		t.Fatalf("adopt existing containers should write Store startup file: %v", err)
	}
}
