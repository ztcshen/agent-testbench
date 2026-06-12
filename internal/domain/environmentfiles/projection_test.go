package environmentfiles

import (
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
			AssetKind:        "compose-secret",
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

func projectionContains(report ProjectionReport, kind string, path string, source string, ok bool) bool {
	for _, file := range report.Files {
		if file.Kind == kind && file.Path == path && file.Source == source && file.OK == ok {
			return true
		}
	}
	return false
}
