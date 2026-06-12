package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"agent-testbench/internal/store"
)

func TestEnvironmentRestoreProjectsDockerNativeStoreAssetsBeforeComposeUp(t *testing.T) {
	workspace := filepath.Join(t.TempDir(), "workspace")
	fakeBin := t.TempDir()
	callsPath := filepath.Join(fakeBin, "docker-calls.txt")
	installProjectionFakeDocker(t, fakeBin)
	t.Setenv("PATH", fakeBin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("DOCKER_CALLS_FILE", callsPath)
	t.Setenv("RESTORE_WORKSPACE", workspace)
	healthURL := newHealthyTestURL(t)

	report, err := buildEnvironmentRestoreReport(context.Background(), store.Environment{
		ID: "env.native.projection",
		ComposeJSON: `{
			"composeFile":"compose.yml",
			"generatedFiles":{
				"compose.yml":"services:\n  app:\n    image: alpine:3.20\n"
			},
			"services":["app"],
			"skipPull":true,
			"skipBuild":true
		}`,
		HealthChecksJSON:       `[{"kind":"compose-service","service":"app"}]`,
		VerificationWorkflowID: "workflow.native-projection",
	}, workspace, true, false, false, time.Second, environmentRestoreWorkflowOptions{}, environmentRestoreDockerCleanupOptions{}, store.EnvironmentComponentGraph{
		Components: []store.EnvironmentComponent{
			{ComponentID: "config", Kind: "config", Role: "configuration", Required: false},
			{ComponentID: "app", Kind: "app", Role: "business-service", ComposeService: "app", Required: true, HealthCheckJSON: `{"type":"url","url":"` + healthURL + `"}`},
		},
		Dependencies: []store.ComponentDependency{
			{ConsumerComponentID: "app", ProviderComponentID: "config", Phase: "startup", Capability: "config", Required: true, ProfileJSON: `{"assetIds":["app.config","app.secret","app.env"]}`},
		},
		Assets: []store.ComponentConfigAsset{
			{OwnerComponentID: "app", AssetID: "app.config", AssetKind: "compose-config", TargetComponentID: "app", TargetPath: ".agent-testbench/restore/config/app.yml", ContentInline: "mode: test\n", ApplyOrder: 10},
			{OwnerComponentID: "app", AssetID: "app.secret", AssetKind: "compose-secret", TargetComponentID: "app", TargetPath: ".agent-testbench/restore/secrets/app.key", ContentInline: "secret-value\n", Sensitive: true, ApplyOrder: 20},
			{OwnerComponentID: "app", AssetID: "app.env", AssetKind: "env-file", TargetComponentID: "app", TargetPath: ".agent-testbench/restore/env/app.env", ContentInline: "APP_MODE=test\n", ApplyOrder: 30, SummaryJSON: `{"dockerNative":{"fileMode":"0600"}}`},
		},
	})
	if err != nil {
		t.Fatalf("build restore report: %v", err)
	}
	if !report.OK || !report.Docker.OK {
		t.Fatalf("native projection restore report docker=%#v componentGraph=%#v preflight=%#v sourcePolicy=%#v componentAssets=%#v", report.Docker, report.ComponentGraph, report.Preflight, report.SourcePolicy, report.ComponentAssets)
	}
	actions := map[string]string{}
	for _, asset := range report.Docker.AppliedAssets {
		actions[asset.AssetID] = asset.Action
	}
	if actions["app.config"] != "project-compose-config" || actions["app.secret"] != "project-compose-secret" || actions["app.env"] != "project-env-file" {
		t.Fatalf("native projection asset actions = %#v assets=%#v", actions, report.Docker.AppliedAssets)
	}
	assertProjectedFile(t, workspace, ".agent-testbench/restore/config/app.yml", "mode: test", 0o644)
	assertProjectedFile(t, workspace, ".agent-testbench/restore/secrets/app.key", "secret-value", 0o600)
	assertProjectedFile(t, workspace, ".agent-testbench/restore/env/app.env", "APP_MODE=test", 0o600)
	calls, err := os.ReadFile(callsPath)
	if err != nil {
		t.Fatalf("read docker calls: %v", err)
	}
	if !strings.Contains(string(calls), " up -d app") {
		t.Fatalf("docker calls missing compose up:\n%s", calls)
	}
}

func installProjectionFakeDocker(t *testing.T, fakeBin string) {
	t.Helper()
	writeFile(t, filepath.Join(fakeBin, "docker"), `#!/usr/bin/env bash
set -euo pipefail
printf '%s\n' "$*" >> "$DOCKER_CALLS_FILE"
if [ "$1" = "compose" ] && [ "$2" = "version" ]; then
  printf 'Docker Compose version v2.0.0\n'
  exit 0
fi
if [ "$1" = "compose" ] && [[ "$*" == *" up -d "* ]]; then
  test -f "$RESTORE_WORKSPACE/.agent-testbench/restore/config/app.yml"
  test -f "$RESTORE_WORKSPACE/.agent-testbench/restore/secrets/app.key"
  test -f "$RESTORE_WORKSPACE/.agent-testbench/restore/env/app.env"
  exit 0
fi
if [ "$1" = "compose" ] && [[ "$*" == *" ps -a --format json "* ]]; then
  service="${@: -1}"
  printf '{"Name":"demo-%s","Service":"%s","State":"running","Health":"healthy"}\n' "$service" "$service"
  exit 0
fi
exit 0
`)
	if err := os.Chmod(filepath.Join(fakeBin, "docker"), 0o755); err != nil {
		t.Fatalf("chmod fake docker: %v", err)
	}
}

func assertProjectedFile(t *testing.T, workspace string, path string, wantContent string, wantMode os.FileMode) {
	t.Helper()
	target := filepath.Join(workspace, filepath.FromSlash(path))
	content, err := os.ReadFile(target)
	if err != nil || strings.TrimSpace(string(content)) != wantContent {
		t.Fatalf("projected %s = %q err=%v", path, content, err)
	}
	info, err := os.Stat(target)
	if err != nil || info.Mode().Perm() != wantMode {
		t.Fatalf("projected %s mode = %v err=%v, want %v", path, info.Mode().Perm(), err, wantMode)
	}
}

func TestEnvironmentRestoreMaterializesRemoteDockerNativeStoreAsset(t *testing.T) {
	workspace := filepath.Join(t.TempDir(), "workspace")
	sourceCheckout := createEnvironmentRestoreReadinessAssetSourceRepo(t, "config/app.yml", "mode: remote\n")
	report, err := buildEnvironmentRestoreReport(context.Background(), store.Environment{
		ID:                     "env.native.remote-projection",
		ComposeJSON:            `{"startCommand":"true"}`,
		HealthChecksJSON:       `[]`,
		VerificationWorkflowID: "workflow.native-remote-projection",
	}, workspace, true, false, true, time.Second, environmentRestoreWorkflowOptions{}, environmentRestoreDockerCleanupOptions{}, store.EnvironmentComponentGraph{
		Components: []store.EnvironmentComponent{
			{ComponentID: "app", Kind: "app", Role: "business-service", Required: true},
		},
		Assets: []store.ComponentConfigAsset{
			{
				OwnerComponentID:  "app",
				AssetID:           "app.remote.config",
				AssetKind:         "compose-secret",
				TargetComponentID: "app",
				TargetPath:        ".agent-testbench/restore/config/app.yml",
				RemoteRefJSON:     `{"url":"git@example.com:team/assets.git","checkout":"` + filepath.ToSlash(sourceCheckout) + `","path":"config/app.yml"}`,
				ApplyOrder:        10,
				Sensitive:         true,
			},
		},
	})
	if err != nil {
		t.Fatalf("build restore report: %v", err)
	}
	if len(report.ComponentAssets) != 1 || !report.ComponentAssets[0].OK || report.ComponentAssets[0].Action != "materialize" {
		t.Fatalf("remote native projection report componentAssets=%#v docker=%#v", report.ComponentAssets, report.Docker)
	}
	projected, err := os.ReadFile(filepath.Join(workspace, ".agent-testbench", "restore", "config", "app.yml"))
	if err != nil || strings.TrimSpace(string(projected)) != "mode: remote" {
		t.Fatalf("remote projected config = %q err=%v", projected, err)
	}
	info, err := os.Stat(filepath.Join(workspace, ".agent-testbench", "restore", "config", "app.yml"))
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("remote projected secret mode = %v err=%v, want 0600", info.Mode().Perm(), err)
	}
}
