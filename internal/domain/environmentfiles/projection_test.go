package environmentfiles

import (
	"encoding/json"
	"strings"
	"testing"
)

type projectionTestEnvironment struct {
	ComposeJSON string
	SummaryJSON string
}

type projectionTestGraph struct {
	Assets []ProjectionAsset
}

func projectionReport(env projectionTestEnvironment, graph projectionTestGraph) ProjectionReport {
	return FromJSON(env.ComposeJSON, env.SummaryJSON, graph.Assets)
}

func TestProjectionReportExplainsStoreBackedFilesAndGaps(t *testing.T) {
	env := projectionTestEnvironment{
		ComposeJSON: `{
			"composeFile":"compose.yml",
			"composeFiles":["compose.yml"],
			"envFiles":["runtime.env"],
			"env":{"APP_MODE":"test"},
			"generatedFiles":{"compose.yml":"services: {}"}
		}`,
		SummaryJSON: `{"startupFiles":{"files":[{"path":"runtime.env"}]}}`,
	}
	graph := projectionTestGraph{
		Assets: []ProjectionAsset{{
			OwnerComponentID: "app",
			AssetID:          "app.secret",
			AssetKind:        assetKindComposeSecret,
			TargetPath:       ".agent-testbench/restore/secrets/app.key",
			ContentInline:    "secret",
		}},
	}

	report := projectionReport(env, graph)
	if report.OK || report.Counts.Referenced != 3 || report.Counts.Missing != 1 || report.Counts.RepairItems != 1 {
		t.Fatalf("projection report = %#v", report)
	}
	if len(report.Missing) != 1 || report.Missing[0].Path != "runtime.env" || report.Missing[0].Source != "summary.startupFiles" {
		t.Fatalf("missing projection = %#v", report.Missing)
	}
	if len(report.RepairPlan) != 1 || report.RepairPlan[0].Name != "startup-file-content" || report.RepairPlan[0].Target != "environment_files" || !report.RepairPlan[0].BlocksRestore {
		t.Fatalf("startup repair plan = %#v", report.RepairPlan)
	}
	if !projectionContains(report, KindComposeFile, "compose.yml", "compose.generatedFiles", true) {
		t.Fatalf("compose file projection missing: %#v", report.Files)
	}
	if !projectionContains(report, KindGeneratedEnv, ".agent-testbench/restore.env", "compose.env", true) {
		t.Fatalf("generated compose env projection missing: %#v", report.Files)
	}
	if !projectionContains(report, KindAsset, ".agent-testbench/restore/secrets/app.key", "component_config_assets", true) {
		t.Fatalf("component asset projection missing: %#v", report.Files)
	}
}

func TestProjectionReportAcceptsEnvironmentPackageFiles(t *testing.T) {
	env := projectionTestEnvironment{
		ComposeJSON: `{
			"composeFile":"compose/docker-compose.yml",
			"envFiles":["compose/runtime.env"],
			"package":{"url":"git@example.com:team/env.git","checkout":"."}
		}`,
	}

	report := projectionReport(env, projectionTestGraph{})
	if !report.OK || report.Counts.Referenced != 2 || report.Counts.Missing != 0 {
		t.Fatalf("package projection report = %#v", report)
	}
	if !projectionContains(report, KindComposeFile, "compose/docker-compose.yml", "environment-package", true) {
		t.Fatalf("package compose projection missing: %#v", report.Files)
	}
}

func TestProjectionReportAcceptsComponentAssetForReferencedFile(t *testing.T) {
	env := projectionTestEnvironment{
		ComposeJSON: `{
			"composeFile":"compose/docker-compose.yml",
			"generatedFiles":{"compose/docker-compose.yml":"services: {}"},
			"envFiles":["compose/app.env"]
		}`,
	}
	graph := projectionTestGraph{
		Assets: []ProjectionAsset{{
			OwnerComponentID: "app",
			AssetID:          "app.env",
			AssetKind:        "env-file",
			TargetPath:       "compose/app.env",
			ContentInline:    "APP_MODE=test\n",
		}},
	}

	report := projectionReport(env, graph)
	if !report.OK || report.Counts.Missing != 0 {
		t.Fatalf("component asset projection report = %#v", report)
	}
	if !projectionContains(report, KindEnvFile, "compose/app.env", "component_config_assets", true) {
		t.Fatalf("component asset should satisfy referenced env file: %#v", report.Files)
	}
}

func TestProjectionReportDiscoversComposeNativeFileReferences(t *testing.T) {
	env := projectionTestEnvironment{
		ComposeJSON: `{
			"composeFile":"compose/docker-compose.yml",
			"generatedFiles":{
				"compose/docker-compose.yml":"services:\n  app:\n    image: alpine:3.20\n    env_file:\n      - ./app.env\n      - path: ./extra.env\n        required: true\n    configs:\n      - source: app_config\n        target: /etc/app/config.yml\n    secrets:\n      - db_password\nconfigs:\n  app_config:\n    file: ./config/app.yml\nsecrets:\n  db_password:\n    file: ./secrets/db.txt\n",
				"compose/config/app.yml":"mode: test\n"
			}
		}`,
	}
	graph := projectionTestGraph{
		Assets: []ProjectionAsset{{
			OwnerComponentID: "db",
			AssetID:          "db.password",
			AssetKind:        assetKindComposeSecret,
			TargetPath:       "compose/secrets/db.txt",
			ContentInline:    "secret\n",
		}},
	}

	report := projectionReport(env, graph)
	if report.OK || report.Counts.Referenced != 5 || report.Counts.Missing != 2 {
		t.Fatalf("compose-native projection report = %#v", report)
	}
	if !projectionContains(report, KindComposeConfigFile, "compose/config/app.yml", "compose.generatedFiles", true) {
		t.Fatalf("compose config file should be Store-backed by generatedFiles: %#v", report.Files)
	}
	if !projectionContains(report, KindComposeSecretFile, "compose/secrets/db.txt", "component_config_assets", true) {
		t.Fatalf("compose secret file should be Store-backed by component asset: %#v", report.Files)
	}
	if !projectionContains(report, KindEnvFile, "compose/app.env", "workspace-file", false) ||
		!projectionContains(report, KindEnvFile, "compose/extra.env", "workspace-file", false) {
		t.Fatalf("compose env_file gaps should be visible: %#v", report.Missing)
	}
}

func TestProjectionReportDiscoversComposeNativeReferenceVariants(t *testing.T) {
	composeContent := strings.Join([]string{
		"include:",
		"  - ./base.yml # shared base",
		"  - path:",
		"      - ./fragments/cache.yml",
		"    env_file: ./include.env",
		"services:",
		"  app:",
		"    image: alpine:3.20",
		"    extends:",
		"      file: ./common.yml # service defaults",
		"      service: app-base",
		"    env_file:",
		"      - path: ./app.env # app env",
		"        required: false",
		"        format: raw",
		"      - ./extra.env # extra env",
		"      - ./env/${TARGET}.env",
		"configs:",
		"    app_config:",
		"        file: ./config/app.yml # app config",
		"    dynamic_config:",
		"        file: ./config/${PROFILE}.yml",
		"secrets:",
		"    db_password:",
		"        file: ./secrets/db.txt # db secret",
	}, "\n") + "\n"
	env := projectionTestEnvironment{ComposeJSON: projectionTestComposeJSON(t, map[string]any{
		"composeFile": "compose/docker-compose.yml",
		"generatedFiles": map[string]string{
			"compose/docker-compose.yml":  composeContent,
			"compose/base.yml":            "services:\n  base:\n    image: alpine:3.20\n    env_file:\n      - ./base.env\n",
			"compose/fragments/cache.yml": "services: {}\n",
			"compose/common.yml":          "services: {}\n",
			"compose/config/app.yml":      "mode: test\n",
			"compose/secrets/db.txt":      "secret\n",
		},
	})}

	report := projectionReport(env, projectionTestGraph{})
	if report.OK || report.Counts.Referenced != 11 || report.Counts.Missing != 5 {
		t.Fatalf("compose variant projection report = %#v", report)
	}
	for _, path := range []string{"compose/base.yml", "compose/fragments/cache.yml", "compose/common.yml"} {
		if !projectionContains(report, KindComposeFile, path, "compose.generatedFiles", true) {
			t.Fatalf("compose file reference %s should be Store-backed: %#v", path, report.Files)
		}
	}
	if !projectionContains(report, KindComposeConfigFile, "compose/config/app.yml", "compose.generatedFiles", true) {
		t.Fatalf("indented config file should be Store-backed: %#v", report.Files)
	}
	if !projectionContains(report, KindComposeSecretFile, "compose/secrets/db.txt", "compose.generatedFiles", true) {
		t.Fatalf("indented secret file should be Store-backed: %#v", report.Files)
	}
	for _, path := range []string{"compose/include.env", "compose/extra.env", "compose/base.env"} {
		if !projectionContains(report, KindEnvFile, path, "workspace-file", false) {
			t.Fatalf("env file gap %s should be visible: %#v", path, report.Missing)
		}
	}
	for _, path := range []string{"compose/env/${TARGET}.env", "compose/config/${PROFILE}.yml"} {
		if !projectionContains(report, "", path, "compose.interpolation", false) {
			t.Fatalf("unresolved dynamic path %s should be visible: %#v", path, report.Missing)
		}
	}
	if !projectionRepairContains(report, "compose-env-variable", "compose.env", "compose-config-file:compose/config/${PROFILE}.yml") ||
		!projectionRepairContains(report, "compose-env-variable", "compose.env", "env-file:compose/env/${TARGET}.env") {
		t.Fatalf("compose interpolation repair plan missing: %#v", report.RepairPlan)
	}
	for _, file := range report.Files {
		switch file.Path {
		case "compose/app.env", "compose/required: false", "compose/format: raw":
			t.Fatalf("optional metadata or interpolated path should not be collected: %#v", report.Files)
		}
	}
}

func TestProjectionReportResolvesComposeNativeInterpolatedReferencesFromStoreEnv(t *testing.T) {
	env := projectionTestEnvironment{ComposeJSON: projectionTestComposeJSON(t, map[string]any{
		"composeFile": "compose/docker-compose.yml",
		"env": map[string]string{
			"PROFILE": "prod",
			"TARGET":  "blue",
		},
		"generatedFiles": map[string]string{
			"compose/docker-compose.yml": strings.Join([]string{
				"services:",
				"  app:",
				"    image: alpine:3.20",
				"    env_file: ./env/${TARGET}.env",
				"configs:",
				"  app_config:",
				"    file: ./config/${PROFILE}.yml",
			}, "\n") + "\n",
			"compose/env/blue.env":    "APP_MODE=test\n",
			"compose/config/prod.yml": "mode: prod\n",
		},
	})}

	report := projectionReport(env, projectionTestGraph{})
	if !report.OK || report.Counts.Missing != 0 {
		t.Fatalf("interpolated projection report = %#v", report)
	}
	if !projectionContains(report, KindEnvFile, "compose/env/blue.env", "compose.generatedFiles", true) {
		t.Fatalf("env interpolation should resolve through compose.env: %#v", report.Files)
	}
	if !projectionContains(report, KindComposeConfigFile, "compose/config/prod.yml", "compose.generatedFiles", true) {
		t.Fatalf("config interpolation should resolve through compose.env: %#v", report.Files)
	}
}

func TestProjectionReportResolvesNestedComposeNativeInterpolationFromStoreEnv(t *testing.T) {
	env := projectionTestEnvironment{ComposeJSON: projectionTestComposeJSON(t, map[string]any{
		"composeFile": "compose/docker-compose.yml",
		"env": map[string]string{
			"DEFAULT_PROFILE": "prod",
		},
		"generatedFiles": map[string]string{
			"compose/docker-compose.yml": strings.Join([]string{
				"services:",
				"  app:",
				"    image: alpine:3.20",
				"configs:",
				"  app_config:",
				"    file: ./config/${PROFILE:-${DEFAULT_PROFILE:-dev}}.yml",
			}, "\n") + "\n",
			"compose/config/prod.yml": "mode: prod\n",
		},
	})}

	report := projectionReport(env, projectionTestGraph{})
	if !report.OK || report.Counts.Missing != 0 {
		t.Fatalf("nested interpolated projection report = %#v", report)
	}
	if !projectionContains(report, KindComposeConfigFile, "compose/config/prod.yml", "compose.generatedFiles", true) {
		t.Fatalf("nested config interpolation should resolve through compose.env: %#v", report.Files)
	}
}

func TestProjectionReportResolvesComposeNativeInterpolationFromStoreEnvFiles(t *testing.T) {
	env := projectionTestEnvironment{ComposeJSON: projectionTestComposeJSON(t, map[string]any{
		"composeFile": "compose/docker-compose.yml",
		"envFiles": []string{
			"compose/runtime.env",
			"compose/override.env",
		},
		"generatedFiles": map[string]string{
			"compose/docker-compose.yml": strings.Join([]string{
				"services:",
				"  app:",
				"    image: alpine:3.20",
				"configs:",
				"  app_config:",
				"    file: ./config/${PROFILE}.yml",
			}, "\n") + "\n",
			"compose/runtime.env":     "PROFILE=dev\n",
			"compose/override.env":    "export PROFILE=prod # selected profile\n",
			"compose/config/prod.yml": "mode: prod\n",
		},
	})}

	report := projectionReport(env, projectionTestGraph{})
	if !report.OK || report.Counts.Missing != 0 {
		t.Fatalf("env-file interpolated projection report = %#v", report)
	}
	if !projectionContains(report, KindComposeConfigFile, "compose/config/prod.yml", "compose.generatedFiles", true) {
		t.Fatalf("config interpolation should resolve through Store-backed compose env files: %#v", report.Files)
	}
}

func TestProjectionReportResolvesComposeNativeInterpolationFromEnvFileAsset(t *testing.T) {
	env := projectionTestEnvironment{ComposeJSON: projectionTestComposeJSON(t, map[string]any{
		"composeFile": "compose/docker-compose.yml",
		"envFiles": []string{
			"compose/runtime.env",
		},
		"generatedFiles": map[string]string{
			"compose/docker-compose.yml": strings.Join([]string{
				"services:",
				"  app:",
				"    image: alpine:3.20",
				"    env_file: ./env/${TARGET}.env",
			}, "\n") + "\n",
			"compose/env/blue.env": "APP_MODE=test\n",
		},
	})}
	graph := projectionTestGraph{
		Assets: []ProjectionAsset{{
			OwnerComponentID: "app",
			AssetID:          "app.runtime-env",
			AssetKind:        "env-file",
			TargetPath:       "compose/runtime.env",
			ContentInline:    "TARGET=blue\n",
		}},
	}

	report := projectionReport(env, graph)
	if !report.OK || report.Counts.Missing != 0 {
		t.Fatalf("env-file asset interpolated projection report = %#v", report)
	}
	if !projectionContains(report, KindEnvFile, "compose/env/blue.env", "compose.generatedFiles", true) {
		t.Fatalf("env interpolation should resolve through Store-backed env-file asset: %#v", report.Files)
	}
}

func TestProjectionReportRejectsAbsoluteComposeNativeReferences(t *testing.T) {
	env := projectionTestEnvironment{ComposeJSON: projectionTestComposeJSON(t, map[string]any{
		"composeFile": "compose/docker-compose.yml",
		"generatedFiles": map[string]string{
			"compose/docker-compose.yml": strings.Join([]string{
				"services:",
				"  app:",
				"    image: alpine:3.20",
				"    env_file: /etc/app.env",
			}, "\n") + "\n",
			"/etc/app.env": "APP_MODE=host-local\n",
		},
	})}

	report := projectionReport(env, projectionTestGraph{})
	if report.OK || report.Counts.Missing != 1 {
		t.Fatalf("absolute env_file should fail projection readiness: %#v", report)
	}
	if !projectionContains(report, KindEnvFile, "/etc/app.env", "compose.path", false) {
		t.Fatalf("absolute env_file should be reported as a compose path gap: %#v", report.Files)
	}
	if !projectionRepairContains(report, "compose-file-projection", "fileProjection.missing", "env-file:/etc/app.env") {
		t.Fatalf("absolute env_file repair plan should require Store projection: %#v", report.RepairPlan)
	}
}

func TestProjectionReportRejectsInterpolatedAbsoluteComposeNativeReferences(t *testing.T) {
	env := projectionTestEnvironment{ComposeJSON: projectionTestComposeJSON(t, map[string]any{
		"composeFile": "compose/docker-compose.yml",
		"env": map[string]string{
			"APP_ENV_FILE": "/etc/app.env",
		},
		"package": map[string]string{
			"url": "git@example.com:team/env.git",
		},
		"generatedFiles": map[string]string{
			"compose/docker-compose.yml": strings.Join([]string{
				"services:",
				"  app:",
				"    image: alpine:3.20",
				"    env_file: ${APP_ENV_FILE}",
			}, "\n") + "\n",
		},
	})}

	report := projectionReport(env, projectionTestGraph{})
	if report.OK || report.Counts.Missing != 1 {
		t.Fatalf("interpolated absolute env_file should fail projection readiness: %#v", report)
	}
	if !projectionContains(report, KindEnvFile, "/etc/app.env", "compose.path", false) {
		t.Fatalf("interpolated absolute env_file should not be accepted by package projection: %#v", report.Files)
	}
}

func TestProjectionReportRejectsEmptyRemoteAssetRef(t *testing.T) {
	env := projectionTestEnvironment{ComposeJSON: projectionTestComposeJSON(t, map[string]any{
		"composeFile": "compose/docker-compose.yml",
		"generatedFiles": map[string]string{
			"compose/docker-compose.yml": strings.Join([]string{
				"services:",
				"  app:",
				"    image: alpine:3.20",
				"    secrets:",
				"      - db_password",
				"secrets:",
				"  db_password:",
				"    file: ./secrets/db.txt",
			}, "\n") + "\n",
		},
	})}
	graph := projectionTestGraph{
		Assets: []ProjectionAsset{{
			OwnerComponentID: "db",
			AssetID:          "db.password",
			AssetKind:        assetKindComposeSecret,
			TargetPath:       "compose/secrets/db.txt",
			RemoteRefJSON:    "{}",
		}},
	}

	report := projectionReport(env, graph)
	if report.OK || report.Counts.Missing != 2 {
		t.Fatalf("empty remote ref should fail projection readiness: %#v", report)
	}
	if !projectionContains(report, KindComposeSecretFile, "compose/secrets/db.txt", "component_config_assets", false) {
		t.Fatalf("empty remote ref asset should be reported as not materializable: %#v", report.Files)
	}
}

func TestProjectionReportScansComponentBackedComposeFileReferences(t *testing.T) {
	env := projectionTestEnvironment{ComposeJSON: projectionTestComposeJSON(t, map[string]any{
		"composeFile": "compose/docker-compose.yml",
	})}
	graph := projectionTestGraph{
		Assets: []ProjectionAsset{
			{
				OwnerComponentID: "compose",
				AssetID:          "compose.main",
				AssetKind:        KindComposeFile,
				TargetPath:       "compose/docker-compose.yml",
				ContentInline: strings.Join([]string{
					"services:",
					"  app:",
					"    image: alpine:3.20",
					"    env_file: ./app.env",
					"configs:",
					"  app_config:",
					"    file: ./config/app.yml",
				}, "\n") + "\n",
			},
			{
				OwnerComponentID: "app",
				AssetID:          "app.config",
				AssetKind:        assetKindComposeConfig,
				TargetPath:       "compose/config/app.yml",
				ContentInline:    "mode: test\n",
			},
		},
	}

	report := projectionReport(env, graph)
	if report.OK || report.Counts.Missing != 1 {
		t.Fatalf("component-backed compose projection report = %#v", report)
	}
	if !projectionContains(report, KindComposeFile, "compose/docker-compose.yml", "component_config_assets", true) {
		t.Fatalf("component-backed compose file should satisfy composeFiles: %#v", report.Files)
	}
	if !projectionContains(report, KindComposeConfigFile, "compose/config/app.yml", "component_config_assets", true) {
		t.Fatalf("nested config file should be discovered from component-backed compose content: %#v", report.Files)
	}
	if !projectionContains(report, KindEnvFile, "compose/app.env", "workspace-file", false) {
		t.Fatalf("nested env_file gap should be discovered from component-backed compose content: %#v", report.Missing)
	}
}

func TestProjectionReportLimitsExtendsScanToReferencedService(t *testing.T) {
	env := projectionTestEnvironment{ComposeJSON: projectionTestComposeJSON(t, map[string]any{
		"composeFile": "compose/docker-compose.yml",
		"generatedFiles": map[string]string{
			"compose/docker-compose.yml": strings.Join([]string{
				"services:",
				"  app:",
				"    image: alpine:3.20",
				"    extends:",
				"      file: ./common.yml",
				"      service: app-base",
			}, "\n") + "\n",
			"compose/common.yml": strings.Join([]string{
				"services:",
				"  app-base:",
				"    image: alpine:3.20",
				"    env_file: ./base.env",
				"    configs:",
				"      - source: app_config",
				"        target: /etc/app.yml",
				"  unused:",
				"    image: alpine:3.20",
				"    env_file: ./unused.env",
				"    configs:",
				"      - source: unused_config",
				"        target: /etc/unused.yml",
				"configs:",
				"  app_config:",
				"    file: ./config/app.yml",
				"  unused_config:",
				"    file: ./config/unused.yml",
			}, "\n") + "\n",
			"compose/config/app.yml": "mode: app\n",
		},
	})}

	report := projectionReport(env, projectionTestGraph{})
	if report.OK || report.Counts.Missing != 1 {
		t.Fatalf("extends projection report = %#v", report)
	}
	if !projectionContains(report, KindEnvFile, "compose/base.env", "workspace-file", false) {
		t.Fatalf("referenced extends service env_file should be visible: %#v", report.Files)
	}
	if !projectionContains(report, KindComposeConfigFile, "compose/config/app.yml", "compose.generatedFiles", true) {
		t.Fatalf("referenced extends service config should be collected: %#v", report.Files)
	}
	for _, path := range []string{"compose/unused.env", "compose/config/unused.yml"} {
		if projectionPathContains(report, path) {
			t.Fatalf("unreferenced extends service file %s should not block projection: %#v", path, report.Files)
		}
	}
}

func projectionTestComposeJSON(t *testing.T, value map[string]any) string {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal compose JSON: %v", err)
	}
	return string(data)
}

func projectionContains(report ProjectionReport, kind string, path string, source string, ok bool) bool {
	for _, file := range report.Files {
		if (kind == "" || file.Kind == kind) && file.Path == path && file.Source == source && file.OK == ok {
			return true
		}
	}
	return false
}

func projectionPathContains(report ProjectionReport, path string) bool {
	for _, file := range report.Files {
		if file.Path == path {
			return true
		}
	}
	return false
}

func projectionRepairContains(report ProjectionReport, name string, target string, missing string) bool {
	for _, item := range report.RepairPlan {
		if item.Name != name || item.Target != target {
			continue
		}
		for _, value := range item.Missing {
			if value == missing {
				return true
			}
		}
	}
	return false
}
