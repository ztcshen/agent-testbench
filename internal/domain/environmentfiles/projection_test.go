package environmentfiles

import (
	"encoding/json"
	"strings"
	"testing"

	"agent-testbench/internal/store"
)

func TestProjectionReportExplainsStoreBackedFilesAndGaps(t *testing.T) {
	env := store.Environment{
		ComposeJSON: `{
			"composeFile":"compose.yml",
			"composeFiles":["compose.yml"],
			"envFiles":["runtime.env"],
			"env":{"APP_MODE":"test"},
			"generatedFiles":{"compose.yml":"services: {}"}
		}`,
		SummaryJSON: `{"startupFiles":{"files":[{"path":"runtime.env"}]}}`,
	}
	graph := store.EnvironmentComponentGraph{
		Assets: []store.ComponentConfigAsset{{
			OwnerComponentID: "app",
			AssetID:          "app.secret",
			AssetKind:        assetKindComposeSecret,
			TargetPath:       ".agent-testbench/restore/secrets/app.key",
			ContentInline:    "secret",
		}},
	}

	report := FromEnvironment(env, graph)
	if report.OK || report.Counts.Referenced != 3 || report.Counts.Missing != 1 {
		t.Fatalf("projection report = %#v", report)
	}
	if len(report.Missing) != 1 || report.Missing[0].Path != "runtime.env" || report.Missing[0].Source != "summary.startupFiles" {
		t.Fatalf("missing projection = %#v", report.Missing)
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
	env := store.Environment{
		ComposeJSON: `{
			"composeFile":"compose/docker-compose.yml",
			"envFiles":["compose/runtime.env"],
			"package":{"url":"git@example.com:team/env.git","checkout":"."}
		}`,
	}

	report := FromEnvironment(env, store.EnvironmentComponentGraph{})
	if !report.OK || report.Counts.Referenced != 2 || report.Counts.Missing != 0 {
		t.Fatalf("package projection report = %#v", report)
	}
	if !projectionContains(report, KindComposeFile, "compose/docker-compose.yml", "environment-package", true) {
		t.Fatalf("package compose projection missing: %#v", report.Files)
	}
}

func TestProjectionReportAcceptsComponentAssetForReferencedFile(t *testing.T) {
	env := store.Environment{
		ComposeJSON: `{
			"composeFile":"compose/docker-compose.yml",
			"generatedFiles":{"compose/docker-compose.yml":"services: {}"},
			"envFiles":["compose/app.env"]
		}`,
	}
	graph := store.EnvironmentComponentGraph{
		Assets: []store.ComponentConfigAsset{{
			OwnerComponentID: "app",
			AssetID:          "app.env",
			AssetKind:        "env-file",
			TargetPath:       "compose/app.env",
			ContentInline:    "APP_MODE=test\n",
		}},
	}

	report := FromEnvironment(env, graph)
	if !report.OK || report.Counts.Missing != 0 {
		t.Fatalf("component asset projection report = %#v", report)
	}
	if !projectionContains(report, KindEnvFile, "compose/app.env", "component_config_assets", true) {
		t.Fatalf("component asset should satisfy referenced env file: %#v", report.Files)
	}
}

func TestProjectionReportDiscoversComposeNativeFileReferences(t *testing.T) {
	env := store.Environment{
		ComposeJSON: `{
			"composeFile":"compose/docker-compose.yml",
			"generatedFiles":{
				"compose/docker-compose.yml":"services:\n  app:\n    image: alpine:3.20\n    env_file:\n      - ./app.env\n      - path: ./extra.env\n        required: true\n    configs:\n      - source: app_config\n        target: /etc/app/config.yml\n    secrets:\n      - db_password\nconfigs:\n  app_config:\n    file: ./config/app.yml\nsecrets:\n  db_password:\n    file: ./secrets/db.txt\n",
				"compose/config/app.yml":"mode: test\n"
			}
		}`,
	}
	graph := store.EnvironmentComponentGraph{
		Assets: []store.ComponentConfigAsset{{
			OwnerComponentID: "db",
			AssetID:          "db.password",
			AssetKind:        assetKindComposeSecret,
			TargetPath:       "compose/secrets/db.txt",
			ContentInline:    "secret\n",
		}},
	}

	report := FromEnvironment(env, graph)
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
	env := store.Environment{ComposeJSON: projectionTestComposeJSON(t, map[string]any{
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

	report := FromEnvironment(env, store.EnvironmentComponentGraph{})
	if report.OK || report.Counts.Referenced != 9 || report.Counts.Missing != 3 {
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
	for _, file := range report.Files {
		switch file.Path {
		case "compose/app.env", "compose/required: false", "compose/format: raw",
			"compose/env/${TARGET}.env", "compose/config/${PROFILE}.yml":
			t.Fatalf("optional metadata or interpolated path should not be collected: %#v", report.Files)
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
		if file.Kind == kind && file.Path == path && file.Source == source && file.OK == ok {
			return true
		}
	}
	return false
}
