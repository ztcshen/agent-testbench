package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"agent-testbench/internal/store"
)

const environmentRestoreReadinessSQLHealth = `[{"kind":"url","url":"http://127.0.0.1:18080/health"}]`

func TestEnvironmentRestoreRejectsComponentRemoteAssetWithoutRemoteURL(t *testing.T) {
	report := buildEnvironmentRestoreComponentReadinessReport(t, "env.component.remote-asset", environmentRestoreReadinessSQLHealth, store.EnvironmentComponentGraph{
		Components: []store.EnvironmentComponent{
			environmentRestoreReadinessAppComponent("app", "", "http://127.0.0.1:18080/app/health"),
		},
		Assets: []store.ComponentConfigAsset{
			environmentRestoreReadinessRemoteAsset("app", "app.large-ddl", "mysql-ddl", "compose/mysql/init/app.sql", `{"path":"compose/mysql/init/app.sql"}`, 0),
		},
	})
	if report.ComponentGraph.OK || report.ComponentGraph.RemoteAssets != 1 || report.ComponentGraph.MissingRemoteAssetRefs != 1 {
		t.Fatalf("component graph remote asset report = %#v", report.ComponentGraph)
	}
	if !restoreTypedReadinessHasItem(report.Readiness.Items, "component-graph", false, "remote Git URL/path") {
		t.Fatalf("readiness should reject incomplete remote asset refs: %#v", report.Readiness.Items)
	}
}

func TestEnvironmentRestoreMaterializesRemoteComponentAsset(t *testing.T) {
	workspace := filepath.Join(t.TempDir(), "workspace")
	sourceCheckout := createEnvironmentRestoreReadinessAssetSourceRepo(t, "compose/mysql/init/app.sql", "create table app_remote (id bigint primary key);\n")
	report := buildEnvironmentRestoreReadinessReportWithMode(t, newEnvironmentRestoreReadinessEnv(
		"env.component.remote-materialize",
		`{"startCommand":"true"}`,
		environmentRestoreReadinessSQLHealth,
	), workspace, true, false, true, environmentRestoreWorkflowOptions{}, store.EnvironmentComponentGraph{
		Components: []store.EnvironmentComponent{
			environmentRestoreReadinessAppComponent("app", "", "http://127.0.0.1:18080/app/health"),
		},
		Assets: []store.ComponentConfigAsset{
			environmentRestoreReadinessRemoteAsset("app", "app.remote-ddl", "mysql-ddl", "compose/mysql/init/app.sql", `{"url":"git@example.com:team/assets.git","checkout":"`+filepath.ToSlash(sourceCheckout)+`","path":"compose/mysql/init/app.sql"}`, 0),
		},
	})
	if !report.OK || len(report.ComponentAssets) != 1 || !report.ComponentAssets[0].OK || report.ComponentAssets[0].Action != "materialize" {
		t.Fatalf("remote component asset report = %#v", report)
	}
	raw, err := os.ReadFile(filepath.Join(workspace, "compose/mysql/init/app.sql"))
	if err != nil || !strings.Contains(string(raw), "app_remote") {
		t.Fatalf("remote component asset was not written raw=%q err=%v", raw, err)
	}
}

func TestEnvironmentRestoreSQLStoreUsesStoreGeneratedStartupFiles(t *testing.T) {
	for _, backend := range environmentRestoreReadinessProductStoreBackends() {
		t.Run(backend.name, func(t *testing.T) {
			report := buildEnvironmentRestoreSQLReadinessReport(t, backend, environmentRestoreSQLStoreStartupEnv("generated", "llt", environmentRestoreReadinessStoreGeneratedCompose(), `[{"kind":"url","url":"http://127.0.0.1:28080/health"}]`))
			if !report.SourcePolicy.OK || !report.SourcePolicy.RemoteOnly || report.Package.Action != "ignored-for-sql-store-restore" || report.Docker.Action != "plan-docker-compose" {
				t.Fatalf("%s generated startup report = %#v", backend.name, report)
			}
			if len(report.Docker.Generated) != 1 || report.Docker.Generated[0].Action != environmentRestoreGeneratedFileActionPlanWrite || !report.Docker.Generated[0].OK {
				t.Fatalf("%s generated startup file report = %#v", backend.name, report.Docker.Generated)
			}
			if !restoreTypedReadinessHasItem(report.Readiness.Items, "store-startup-files", true, "generated from Store metadata") {
				t.Fatalf("%s readiness should accept Store generated startup files: %#v", backend.name, report.Readiness.Items)
			}
			if !restoreTypedReadinessHasItem(report.Readiness.Items, "file-projection", true, "Store-backed projection") {
				t.Fatalf("%s readiness should expose Store-backed file projection: %#v", backend.name, report.Readiness.Items)
			}
		})
	}
}

func TestEnvironmentRestoreStoreStartupFilesAcceptWrittenWorkspaceCompose(t *testing.T) {
	workspace := filepath.Join(t.TempDir(), "workspace")
	composePath := filepath.Join(workspace, "compose", "docker-compose.yml")
	writeFile(t, composePath, "services: {}\n")
	report := environmentRestoreReport{
		Workspace: workspace,
		Compose: map[string]any{
			"composeFile": "compose/docker-compose.yml",
			"generatedFiles": map[string]any{
				"generated/docker-compose.yml": "services: {}\n",
			},
		},
		Docker: environmentRestoreDockerReport{
			Generated: []environmentRestoreGeneratedFile{{
				Path:   composePath,
				Action: environmentRestoreGeneratedFileActionWrite,
				OK:     true,
			}},
		},
	}
	ok, detail := environmentRestoreStoreStartupFilesReady(report)
	if !ok || !strings.Contains(detail, "generated from Store metadata") {
		t.Fatalf("written workspace compose should satisfy Store startup readiness: ok=%t detail=%q", ok, detail)
	}
}

func TestEnvironmentRestoreStoreStartupFilesRejectLeftoverWorkspaceCompose(t *testing.T) {
	workspace := filepath.Join(t.TempDir(), "workspace")
	composePath := filepath.Join(workspace, "compose", "docker-compose.yml")
	writeFile(t, composePath, "services: {}\n")
	report := environmentRestoreReport{
		Workspace: workspace,
		Compose: map[string]any{
			"composeFile": "compose/docker-compose.yml",
		},
	}
	ok, detail := environmentRestoreStoreStartupFilesReady(report)
	if ok || !strings.Contains(detail, "missing environment_files") {
		t.Fatalf("leftover workspace compose should not satisfy Store startup readiness: ok=%t detail=%q", ok, detail)
	}
}

func TestEnvironmentRestoreSQLStoreRejectsLocalStartupFilesWithoutStoreGeneratedContent(t *testing.T) {
	for _, backend := range environmentRestoreReadinessProductStoreBackends() {
		t.Run(backend.name, func(t *testing.T) {
			report := buildEnvironmentRestoreSQLReadinessReport(t, backend, environmentRestoreSQLStoreStartupEnv("local.compose", "llt", environmentRestoreReadinessLocalComposePackage(), `[{"kind":"url","url":"http://127.0.0.1:28080/health"}]`))
			if !report.SourcePolicy.OK || report.Package.Action != "ignored-for-sql-store-restore" {
				t.Fatalf("%s local startup pre-readiness report = %#v", backend.name, report)
			}
			if !restoreTypedReadinessHasItem(report.Readiness.Items, "store-startup-files", false, "missing environment_files") {
				t.Fatalf("%s readiness should reject local startup files without Store content: %#v", backend.name, report.Readiness.Items)
			}
			if !restoreTypedReadinessHasItem(report.Readiness.Items, "file-projection", false, "compose-file:compose/docker-compose.yml") {
				t.Fatalf("%s readiness should expose missing file projection: %#v", backend.name, report.Readiness.Items)
			}
		})
	}
}

func TestEnvironmentRestoreSQLStoreRejectsMissingComposeStartupAssets(t *testing.T) {
	for _, backend := range environmentRestoreReadinessProductStoreBackends() {
		t.Run(backend.name, func(t *testing.T) {
			report := buildEnvironmentRestoreSQLReadinessReport(t, backend, environmentRestoreSQLStoreStartupEnv("missing.assets", "app", environmentRestoreReadinessGeneratedComposeMissingAssets(), environmentRestoreReadinessSQLHealth))
			if report.OK || report.Preflight.OK || len(report.Preflight.StartupAssets) != 2 {
				t.Fatalf("%s missing startup assets report = %#v", backend.name, report.Preflight.StartupAssets)
			}
			if !restoreTypedReadinessHasItem(report.Readiness.Items, "startup-assets", false, "compose/mysql/init") {
				t.Fatalf("%s readiness should include missing startup assets: %#v", backend.name, report.Readiness.Items)
			}
		})
	}
}

func TestEnvironmentRestoreSQLStoreRejectsComposeNativeFileProjectionGaps(t *testing.T) {
	for _, backend := range environmentRestoreReadinessProductStoreBackends() {
		t.Run(backend.name, func(t *testing.T) {
			report := buildEnvironmentRestoreSQLReadinessReport(t, backend, environmentRestoreSQLStoreStartupEnv("native.file.gaps", "app", environmentRestoreReadinessGeneratedComposeWithNativeFileGap(), environmentRestoreReadinessSQLHealth))
			if report.Readiness.OK {
				t.Fatalf("%s readiness should block missing native Compose file projections: %#v", backend.name, report.Readiness)
			}
			if !restoreTypedReadinessHasItem(report.Readiness.Items, "file-projection", false, "env-file:compose/app.env") {
				t.Fatalf("%s readiness should expose missing env_file projection: %#v", backend.name, report.Readiness.Items)
			}
			if report.FileProjection.OK || len(report.FileProjection.Missing) != 1 {
				t.Fatalf("%s fileProjection should show one native file gap: %#v", backend.name, report.FileProjection)
			}
			if report.OK || report.Docker.Action != environmentRestoreDockerActionSkippedFileProjection || report.Docker.OK {
				t.Fatalf("%s Docker startup should be blocked before Compose execution: %#v", backend.name, report.Docker)
			}
		})
	}
}

func TestEnvironmentRestoreSQLStoreAcceptsStoreGeneratedComposeStartupAssets(t *testing.T) {
	for _, backend := range environmentRestoreReadinessProductStoreBackends() {
		t.Run(backend.name, func(t *testing.T) {
			report := buildEnvironmentRestoreSQLReadinessReport(t, backend, environmentRestoreSQLStoreStartupEnv("assets", "app", environmentRestoreReadinessGeneratedComposeWithAssets(), environmentRestoreReadinessSQLHealth))
			if !report.Preflight.OK || len(report.Preflight.StartupAssets) != 2 {
				t.Fatalf("%s startup assets report = %#v readiness=%#v docker=%#v", backend.name, report.Preflight.StartupAssets, report.Readiness, report.Docker)
			}
			if !restoreTypedReadinessHasItem(report.Readiness.Items, "startup-assets", true, "2 Compose startup asset") {
				t.Fatalf("%s readiness should accept Store generated startup assets: %#v", backend.name, report.Readiness.Items)
			}
		})
	}
}

func TestEnvironmentRestoreStartupAssetsTreatRepoCheckoutAsCovered(t *testing.T) {
	workspace := filepath.Join(t.TempDir(), "workspace")
	compose := map[string]any{
		"composeFiles": []any{"compose/docker-compose.yml"},
		"generatedFiles": map[string]any{
			"compose/docker-compose.yml": strings.Join([]string{
				"services:",
				"  app:",
				"    image: alpine:3.20",
				"    volumes:",
				"      - ${DOCKER_APP_REPO}:/workspace/app",
			}, "\n") + "\n",
		},
		"env": map[string]any{
			"DOCKER_APP_REPO": "$AGENT_TESTBENCH_WORKSPACE/app",
		},
	}
	assets := environmentRestoreStartupAssets(compose, []environmentRestoreRepoSpec{{
		Checkout: filepath.Join(workspace, "app"),
	}}, workspace)
	if len(assets) != 0 {
		t.Fatalf("repo checkout bind mount should be covered, got %#v", assets)
	}
}

func TestEnvironmentRestoreStartupAssetsParsesShortVolumeDefaultInterpolation(t *testing.T) {
	workspace := filepath.Join(t.TempDir(), "workspace")
	compose := map[string]any{
		"composeFiles": []any{"compose/docker-compose.yml"},
		"generatedFiles": map[string]any{
			"compose/docker-compose.yml": strings.Join([]string{
				"services:",
				"  app:",
				"    image: alpine:3.20",
				"    volumes:",
				"      - ${CONFIG_DIR:-./config}:/etc/config",
			}, "\n") + "\n",
		},
	}
	assets := environmentRestoreStartupAssets(compose, nil, workspace)
	if len(assets) != 1 || assets[0].Path != filepath.Clean("compose/config") || assets[0].OK {
		t.Fatalf("default interpolation bind mount should be a missing startup asset, got %#v", assets)
	}
}

func TestParseComposeShortVolumeHandlesInterpolationAndAccessMode(t *testing.T) {
	source, target, ok := parseComposeShortVolume("${CONFIG_DIR:-./config}:/etc/config:ro")
	if !ok || source != "${CONFIG_DIR:-./config}" || target != "/etc/config" {
		t.Fatalf("short volume with interpolation and mode = source %q target %q ok %t", source, target, ok)
	}
}

func TestParseComposeShortVolumeAllowsSourceWordInPath(t *testing.T) {
	source, target, ok := parseComposeShortVolume("/tmp/missing-source:/workspace/app")
	if !ok || source != "/tmp/missing-source" || target != "/workspace/app" {
		t.Fatalf("short volume with source word in path = source %q target %q ok %t", source, target, ok)
	}
}

func TestEnvironmentRestoreMaterializesComponentAssetsAsStartupFiles(t *testing.T) {
	for _, backend := range environmentRestoreReadinessProductStoreBackends() {
		t.Run(backend.name, func(t *testing.T) {
			report := buildEnvironmentRestoreSQLReadinessReport(t, backend, environmentRestoreSQLStoreStartupEnv("component.assets", "app", environmentRestoreReadinessGeneratedComposeMissingAssets(), environmentRestoreReadinessSQLHealth), store.EnvironmentComponentGraph{
				Components: []store.EnvironmentComponent{
					environmentRestoreReadinessComponent("mysql", "middleware", "database", "", environmentRestoreReadinessComposeHealth("mysql")),
					environmentRestoreReadinessAppComponent("app", "", "http://127.0.0.1:18080/health"),
				},
				Assets: []store.ComponentConfigAsset{
					environmentRestoreReadinessInlineAsset("app", "app.mysql.schema", "mysql-ddl", "mysql", "compose/mysql/init/schema.sql", "create database app;\n", 0),
					environmentRestoreReadinessInlineAsset("app", "app.run-script", "container-start-script", "app", "compose/scripts/run-app.sh", "#!/bin/sh\nexit 0\n", 0),
				},
			})
			if len(report.Preflight.StartupAssets) != 2 {
				t.Fatalf("%s component asset startup report = %#v readiness=%#v", backend.name, report.Preflight.StartupAssets, report.Readiness)
			}
			if !restoreTypedReadinessHasItem(report.Readiness.Items, "startup-assets", true, "2 Compose startup asset") {
				t.Fatalf("%s readiness should accept component asset startup files: %#v", backend.name, report.Readiness.Items)
			}
			if _, ok := generatedFileContentMapFromAny(report.Compose["generatedFiles"])["compose/mysql/init/schema.sql"]; !ok {
				t.Fatalf("%s component schema asset was not projected into generatedFiles: %#v", backend.name, report.Compose["generatedFiles"])
			}
		})
	}
}

func TestEnvironmentRestoreOrdersComponentAssetsByBlockingDependencyOrder(t *testing.T) {
	workspace := filepath.Join(t.TempDir(), "workspace")
	report := buildEnvironmentRestoreReadinessReportWithMode(t, newEnvironmentRestoreReadinessEnv("env.component.asset-order", `{"startCommand":"true"}`, `[]`), workspace, false, false, true, environmentRestoreWorkflowOptions{}, store.EnvironmentComponentGraph{
		Components: []store.EnvironmentComponent{
			environmentRestoreReadinessComponent("worker", "app", "worker", "worker", environmentRestoreReadinessURLHealth("http://127.0.0.1:18081/health")),
			environmentRestoreReadinessComponent("db", "middleware", "database", "db", environmentRestoreReadinessComposeHealth("")),
			environmentRestoreReadinessAppComponent("app", "app", "http://127.0.0.1:18080/health"),
		},
		Dependencies: []store.ComponentDependency{
			{ConsumerComponentID: "app", ProviderComponentID: "db", Phase: "startup", Capability: "sql", Required: true, ProfileJSON: `{}`},
			{ConsumerComponentID: "worker", ProviderComponentID: "app", Phase: "startup", Capability: "http", Required: true, ProfileJSON: `{}`},
		},
		Assets: []store.ComponentConfigAsset{
			environmentRestoreReadinessRemoteAsset("worker", "worker.remote", "script", "b-worker-remote.sh", `{"url":"git@example.com:team/assets.git","path":"b-worker-remote.sh"}`, 1),
			environmentRestoreReadinessInlineAsset("app", "app.late", "config", "", "a-app-late.txt", "app late\n", 20),
			environmentRestoreReadinessInlineAsset("db", "db.schema", "mysql-ddl", "", "z-db-schema.sql", "create database app;\n", 10),
			environmentRestoreReadinessRemoteAsset("app", "app.remote", "script", "c-app-remote.sh", `{"url":"git@example.com:team/assets.git","path":"c-app-remote.sh"}`, 5),
			environmentRestoreReadinessInlineAsset("app", "app.early", "config", "", "d-app-early.txt", "app early\n", 1),
		},
	})
	if !report.OK {
		t.Fatalf("component asset order report should be OK: %#v", report)
	}
	if got := strings.Join(report.ComponentGraph.BlockingOrder, ","); got != "db,app,worker" {
		t.Fatalf("blocking order = %s", got)
	}
	var generatedPaths []string
	for _, item := range report.Docker.Generated {
		generatedPaths = append(generatedPaths, strings.TrimPrefix(item.Path, workspace+string(os.PathSeparator)))
	}
	if got := strings.Join(generatedPaths, ","); got != "z-db-schema.sql,d-app-early.txt,a-app-late.txt" {
		t.Fatalf("generated file order = %s reports=%#v", got, report.Docker.Generated)
	}
	var remoteAssetIDs []string
	for _, item := range report.ComponentAssets {
		remoteAssetIDs = append(remoteAssetIDs, item.AssetID)
	}
	if got := strings.Join(remoteAssetIDs, ","); got != "app.remote,worker.remote" {
		t.Fatalf("remote asset order = %s reports=%#v", got, report.ComponentAssets)
	}
}

func createEnvironmentRestoreReadinessAssetSourceRepo(t *testing.T, targetPath string, content string) string {
	t.Helper()
	sourceCheckout := filepath.Join(t.TempDir(), "asset-source")
	runGit(t, "", "init", "-b", "main", sourceCheckout)
	writeFile(t, filepath.Join(sourceCheckout, targetPath), content)
	runGit(t, sourceCheckout, "add", ".")
	runGit(t, sourceCheckout, "-c", "user.name=Open Test", "-c", "user.email=open-test@example.com", "commit", "-m", "asset source")
	runGit(t, sourceCheckout, "remote", "add", "origin", "git@example.com:team/assets.git")
	return sourceCheckout
}

func environmentRestoreSQLStoreStartupEnv(suffix string, repoID string, composeJSON string, healthChecksJSON string) store.Environment {
	env := newEnvironmentRestoreReadinessEnv("env.sql."+suffix, composeJSON, healthChecksJSON)
	env.ReposJSON = `{"` + repoID + `":{"url":"git@example.com:team/` + repoID + `.git","checkout":"` + repoID + `"}}`
	return env
}

func environmentRestoreReadinessStoreGeneratedCompose() string {
	return `{"composeFile":"compose/docker-compose.yml","composeFiles":["compose/docker-compose.yml"],"generatedFiles":{"compose/docker-compose.yml":"services:\n  llt:\n    image: alpine:3.20\n"},"package":{"url":"/Users/zlh/codes/agent-testbench-validation","checkout":"."}}`
}

func environmentRestoreReadinessLocalComposePackage() string {
	return `{"composeFile":"compose/docker-compose.yml","composeFiles":["compose/docker-compose.yml"],"package":{"url":"/Users/zlh/codes/agent-testbench-validation","checkout":"."}}`
}

func environmentRestoreReadinessGeneratedComposeMissingAssets() string {
	return `{"composeFile":"compose/docker-compose.yml","composeFiles":["compose/docker-compose.yml"],"generatedFiles":{"compose/docker-compose.yml":"services:\n  mysql:\n    image: mysql:8\n    volumes:\n      - ./mysql/init:/docker-entrypoint-initdb.d\n  app:\n    image: alpine:3.20\n    command: [\"/bin/sh\", \"/sandbox/compose/scripts/run-app.sh\"]\n    volumes:\n      - ${DOCKER_APP_REPO:-/tmp/app}:/workspace/app\n      - ${SANDBOX_ROOT:-/tmp/sandbox}:/sandbox\n"},"env":{"DOCKER_APP_REPO":"$AGENT_TESTBENCH_WORKSPACE/app","SANDBOX_ROOT":"$AGENT_TESTBENCH_WORKSPACE"}}`
}

func environmentRestoreReadinessGeneratedComposeWithAssets() string {
	return `{"composeFile":"compose/docker-compose.yml","composeFiles":["compose/docker-compose.yml"],"generatedFiles":{"compose/docker-compose.yml":"services:\n  mysql:\n    image: mysql:8\n    volumes:\n      - ./mysql/init:/docker-entrypoint-initdb.d\n  app:\n    image: alpine:3.20\n    command: [\"/bin/sh\", \"/sandbox/compose/scripts/run-app.sh\"]\n    volumes:\n      - ${DOCKER_APP_REPO:-/tmp/app}:/workspace/app\n      - ${SANDBOX_ROOT:-/tmp/sandbox}:/sandbox\n","compose/mysql/init/schema.sql":"create database app;\n","compose/scripts/run-app.sh":"#!/bin/sh\nexit 0\n"},"env":{"DOCKER_APP_REPO":"$AGENT_TESTBENCH_WORKSPACE/app","SANDBOX_ROOT":"$AGENT_TESTBENCH_WORKSPACE"}}`
}

func environmentRestoreReadinessGeneratedComposeWithNativeFileGap() string {
	return `{"composeFile":"compose/docker-compose.yml","composeFiles":["compose/docker-compose.yml"],"generatedFiles":{"compose/docker-compose.yml":"services:\n  app:\n    image: alpine:3.20\n    env_file:\n      - ./app.env\n    configs:\n      - source: app_config\n        target: /etc/app/config.yml\nconfigs:\n  app_config:\n    file: ./config/app.yml\n","compose/config/app.yml":"mode: test\n"}}`
}
