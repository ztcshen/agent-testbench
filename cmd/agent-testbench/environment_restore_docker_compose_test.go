package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"agent-testbench/internal/store"
)

func TestEnvironmentRestorePreflightReportsMissingDockerComposePlugin(t *testing.T) {
	workspace := filepath.Join(t.TempDir(), "workspace")
	installEnvironmentRestoreDockerTool(t, "#!/bin/sh\nif [ \"$1\" = compose ] && [ \"$2\" = version ]; then exit 17; fi\nexit 0\n")
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

func TestEnvironmentRestoreHonorsComposeOptionsFromStore(t *testing.T) {
	fixture := newEnvironmentRestoreDockerCLIFixture(t)
	fixture.writeWorkspaceFile(t, "compose.yml", "services: {}\n")
	fixture.writeWorkspaceFile(t, ".env.local", "MODE=local\n")
	runCLI(t, "environment", "register",
		"--store", fixture.StoreDSN,
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

	out := runCLIWithEnv(t, fixture.DockerEnv, "environment", "restore", "--store", fixture.StoreDSN, "--workspace", fixture.Workspace, "--execute", "--json", "env.compose.options")
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
	base := "compose -f " + filepath.Join(fixture.Workspace, "compose.yml") + " --env-file " + environmentRestoreGeneratedEnvFilePath(fixture.Workspace) + " -p demo --env-file " + filepath.Join(fixture.Workspace, ".env.local") + " --profile api"
	want := base + " up -d web"
	dockerCalls, err := os.ReadFile(fixture.DockerCallsPath)
	if err != nil {
		t.Fatalf("read fake docker calls: %v", err)
	}
	if strings.Contains(string(dockerCalls), " pull") || strings.Contains(string(dockerCalls), " build") || !strings.Contains(string(dockerCalls), want) {
		t.Fatalf("compose option docker calls want %q:\n%s", want, dockerCalls)
	}
	if !strings.Contains(string(dockerCalls), base+" ps -a --format json web") {
		t.Fatalf("compose option docker calls should include service readiness check:\n%s", dockerCalls)
	}
}

func TestEnvironmentRestoreSupportsMultipleComposeFiles(t *testing.T) {
	fixture := newEnvironmentRestoreDockerCLIFixture(t)
	fixture.writeWorkspaceFile(t, "compose.base.yml", "services: {}\n")
	fixture.writeWorkspaceFile(t, "compose.apps.yml", "services: {}\n")
	runCLI(t, "environment", "register",
		"--store", fixture.StoreDSN,
		"--id", "env.compose.multi",
		"--compose-file", "compose.base.yml",
		"--compose-file", "compose.apps.yml",
		"--compose-env", "SANDBOX_ROOT=$AGENT_TESTBENCH_WORKSPACE",
		"--compose-skip-pull",
		"--compose-skip-build",
		"--health-compose-service", "web",
		"--verification-workflow", "workflow.core-10",
	)

	out := runCLIWithEnv(t, fixture.DockerEnv, "environment", "restore", "--store", fixture.StoreDSN, "--workspace", fixture.Workspace, "--execute", "--json", "env.compose.multi")
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
	dockerCalls, err := os.ReadFile(fixture.DockerCallsPath)
	if err != nil {
		t.Fatalf("read fake docker calls: %v", err)
	}
	want := "compose -f " + filepath.Join(fixture.Workspace, "compose.base.yml") + " -f " + filepath.Join(fixture.Workspace, "compose.apps.yml") + " up -d"
	want = strings.Replace(want, " up -d", " --env-file "+filepath.Join(fixture.Workspace, ".agent-testbench", "restore.env")+" up -d", 1)
	if !strings.Contains(string(dockerCalls), want) {
		t.Fatalf("multi compose docker calls missing %q:\n%s", want, dockerCalls)
	}
	envFile, err := os.ReadFile(filepath.Join(fixture.Workspace, ".agent-testbench", "restore.env"))
	if err != nil || !strings.Contains(string(envFile), "SANDBOX_ROOT="+fixture.Workspace) {
		t.Fatalf("generated compose env file = %q err=%v", envFile, err)
	}
}

func TestEnvironmentRestoreRejectsMissingHostBindMountBeforeComposeUp(t *testing.T) {
	fixture := newEnvironmentRestoreDockerCLIFixture(t)
	missingHostPath := filepath.Join(t.TempDir(), "missing-source")
	composeSource := filepath.Join(t.TempDir(), "compose.yml")
	writeFile(t, composeSource, "services:\n  app:\n    image: alpine:3.20\n    volumes:\n      - "+missingHostPath+":/workspace/app\n")
	runCLI(t, "environment", "register",
		"--store", fixture.StoreDSN,
		"--id", "env.compose.bind-missing",
		"--compose-file", "compose.yml",
		"--compose-generated-file", "compose.yml="+composeSource,
		"--compose-service", "app",
		"--compose-skip-pull",
		"--compose-skip-build",
		"--health-compose-service", "app",
		"--verification-workflow", "workflow.core-10",
	)

	out := runCLIFailsWithEnv(t, fixture.DockerEnv, "environment", "restore", "--store", fixture.StoreDSN, "--workspace", fixture.Workspace, "--execute", "--json", "env.compose.bind-missing")
	if !strings.Contains(out, "missing host bind mount source") || !strings.Contains(out, missingHostPath) {
		t.Fatalf("missing host bind mount should fail before compose up, got:\n%s", out)
	}
	dockerCalls, err := os.ReadFile(fixture.DockerCallsPath)
	if err != nil {
		t.Fatalf("read fake docker calls: %v", err)
	}
	if strings.Contains(string(dockerCalls), " up -d") {
		t.Fatalf("restore should not run compose up after bind preflight failure:\n%s", dockerCalls)
	}
}

func TestEnvironmentRestoreRewritesGeneratedHostBindMountsToRegisteredCheckouts(t *testing.T) {
	fixture := newEnvironmentRestoreDockerCLIFixture(t)
	checkout := filepath.Join(fixture.Workspace, "entry-service")
	if err := os.MkdirAll(checkout, 0o755); err != nil {
		t.Fatalf("create checkout: %v", err)
	}
	legacyHostPath := "/Users/example/Workspaces/private/entry-service"
	composeSource := filepath.Join(t.TempDir(), "compose.yml")
	writeFile(t, composeSource, "services:\n  entry-service:\n    image: alpine:3.20\n    volumes:\n      - "+legacyHostPath+":/workspace/entry-service\n")
	runCLI(t, "environment", "register",
		"--store", fixture.StoreDSN,
		"--id", "env.compose.bind-rewrite",
		"--repo", "entry-service=https://example.com/team/entry-service.git",
		"--checkout", "entry-service=entry-service",
		"--compose-file", "compose.yml",
		"--compose-generated-file", "compose.yml="+composeSource,
		"--compose-service", "entry-service",
		"--compose-skip-pull",
		"--compose-skip-build",
		"--verification-workflow", "workflow.core-10",
	)

	out := runCLIWithEnv(t, fixture.DockerEnv, "environment", "restore", "--store", fixture.StoreDSN, "--workspace", fixture.Workspace, "--json", "env.compose.bind-rewrite")
	var report struct {
		OK        bool `json:"ok"`
		Preflight struct {
			OK            bool     `json:"ok"`
			ComposeIssues []string `json:"composeIssues"`
		} `json:"preflight"`
		Compose struct {
			GeneratedFiles map[string]string `json:"generatedFiles"`
		} `json:"compose"`
	}
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatalf("decode bind rewrite restore json: %v\n%s", err, out)
	}
	if !report.OK || !report.Preflight.OK || len(report.Preflight.ComposeIssues) != 0 {
		t.Fatalf("rewritten bind restore should pass preflight: %#v\n%s", report.Preflight, out)
	}
	content := report.Compose.GeneratedFiles["compose.yml"]
	if !strings.Contains(content, checkout+":/workspace/entry-service") || strings.Contains(content, legacyHostPath) {
		t.Fatalf("generated compose bind source was not rewritten:\n%s", content)
	}
}

func TestEnvironmentRestoreRewritesBindMountsWithoutPrefixBleed(t *testing.T) {
	content := "services:\n  combined:\n    image: alpine:3.20\n    volumes:\n      - /old/app:/workspace/app\n      - /old/app-api:/workspace/app-api\n"
	rewritten, ok := environmentRestoreRewriteComposeHostBindSources(content, map[string]string{
		"app":     "/current/service-alpha",
		"app-api": "/current/service-beta",
	})
	if !ok {
		t.Fatalf("expected bind sources to be rewritten")
	}
	if !strings.Contains(rewritten, "/current/service-alpha:/workspace/app") || !strings.Contains(rewritten, "/current/service-beta:/workspace/app-api") {
		t.Fatalf("bind sources were not rewritten exactly:\n%s", rewritten)
	}
	if strings.Contains(rewritten, "/current/service-alpha-api") {
		t.Fatalf("shorter bind source leaked into longer source:\n%s", rewritten)
	}
}

func TestEnvironmentRestoreReportsUnavailableComposeImageBeforePull(t *testing.T) {
	fixture := newEnvironmentRestoreDockerCLIFixture(t)
	fixture.writeDockerTool(t, `#!/usr/bin/env bash
printf '%s\n' "$*" >> "$DOCKER_CALLS_FILE"
if [ "$1" = "compose" ] && [ "$2" = "version" ]; then
  printf 'Docker Compose version v2.0.0\n'
  exit 0
fi
if [ "$1" = "manifest" ] && [ "$2" = "inspect" ]; then
  printf 'manifest for %s not found\n' "$3" >&2
  exit 1
fi
if [ "$1" = "image" ] && [ "$2" = "inspect" ]; then
  printf 'image %s not found\n' "$3" >&2
  exit 1
fi
if [ "$1" = "compose" ] && [[ "$*" == *" ps -a --format json "* ]]; then
  service="${@: -1}"
  printf '{"Name":"%s","Service":"%s","State":"running","Health":"healthy"}\n' "$service" "$service"
fi
`)
	composeSource := filepath.Join(t.TempDir(), "compose.yml")
	writeFile(t, composeSource, "services:\n  kafka:\n    image: bitnami/kafka:3.9.0\n")
	runCLI(t, "environment", "register",
		"--store", fixture.StoreDSN,
		"--id", "env.compose.image-missing",
		"--compose-file", "compose.yml",
		"--compose-generated-file", "compose.yml="+composeSource,
		"--compose-service", "kafka",
		"--compose-skip-build",
		"--health-compose-service", "kafka",
		"--verification-workflow", "workflow.core-10",
	)

	out := runCLIFailsWithEnv(t, fixture.DockerEnv, "environment", "restore", "--store", fixture.StoreDSN, "--workspace", fixture.Workspace, "--execute", "--pull", "--json", "env.compose.image-missing")
	if !strings.Contains(out, "unavailable compose image") || !strings.Contains(out, "bitnami/kafka:3.9.0") {
		t.Fatalf("unavailable image should fail before pull, got:\n%s", out)
	}
	dockerCalls, err := os.ReadFile(fixture.DockerCallsPath)
	if err != nil {
		t.Fatalf("read fake docker calls: %v", err)
	}
	if strings.Contains(string(dockerCalls), " pull kafka") {
		t.Fatalf("restore should not run compose pull after image preflight failure:\n%s", dockerCalls)
	}
}

func TestParseComposeImageReferencesIgnoresNestedImageKeys(t *testing.T) {
	got := parseComposeImageReferences(strings.Join([]string{
		"services:",
		"  app:",
		"    image: alpine:3.20",
		"    environment:",
		"      image: nested/value:latest",
		"  worker:",
		"    image: busybox:1.36",
	}, "\n") + "\n")
	if got["app"] != "alpine:3.20" || got["worker"] != "busybox:1.36" {
		t.Fatalf("compose image refs = %#v", got)
	}
}

func TestParseComposeImageReferencesInvalidYAMLIgnoresNestedImageKeys(t *testing.T) {
	got := parseComposeImageReferences(strings.Join([]string{
		"x-invalid: [",
		"services:",
		"  app:",
		"    image: alpine:3.20",
		"    environment:",
		"      image: nested/value:latest",
		"  worker:",
		"    profiles:",
		"      - worker",
		"    image: busybox:1.36",
	}, "\n") + "\n")
	if got["app"] != "alpine:3.20" || got["worker"] != "busybox:1.36" {
		t.Fatalf("invalid-yaml compose image refs = %#v", got)
	}
}

func TestEnvironmentRestoreAcceptsLocalComposeImageWhenRegistryProbeFails(t *testing.T) {
	fixture := newEnvironmentRestoreDockerCLIFixture(t)
	fixture.writeDockerTool(t, `#!/usr/bin/env bash
printf '%s\n' "$*" >> "$DOCKER_CALLS_FILE"
if [ "$1" = "compose" ] && [ "$2" = "version" ]; then
  printf 'Docker Compose version v2.0.0\n'
  exit 0
fi
if [ "$1" = "manifest" ] && [ "$2" = "inspect" ]; then
  printf 'registry timeout for %s\n' "$3" >&2
  exit 1
fi
if [ "$1" = "image" ] && [ "$2" = "inspect" ]; then
  printf '[{"Id":"sha256:local"}]\n'
  exit 0
fi
if [ "$1" = "compose" ] && [[ "$*" == *" ps -a --format json "* ]]; then
  service="${@: -1}"
  printf '{"Name":"%s","Service":"%s","State":"running","Health":"healthy"}\n' "$service" "$service"
fi
`)
	composeSource := filepath.Join(t.TempDir(), "compose.yml")
	writeFile(t, composeSource, "services:\n  kafka:\n    image: apache/kafka:3.7.0\n")
	runCLI(t, "environment", "register",
		"--store", fixture.StoreDSN,
		"--id", "env.compose.image-local",
		"--compose-file", "compose.yml",
		"--compose-generated-file", "compose.yml="+composeSource,
		"--compose-service", "kafka",
		"--compose-skip-build",
		"--health-compose-service", "kafka",
		"--verification-workflow", "workflow.core-10",
	)

	out := runCLIWithEnv(t, fixture.DockerEnv, "environment", "restore", "--store", fixture.StoreDSN, "--workspace", fixture.Workspace, "--execute", "--json", "env.compose.image-local")
	if !strings.Contains(out, `"ok": true`) || !strings.Contains(out, "local Docker image is available") {
		t.Fatalf("local image reuse should pass with note, got:\n%s", out)
	}
	dockerCalls, err := os.ReadFile(fixture.DockerCallsPath)
	if err != nil {
		t.Fatalf("read fake docker calls: %v", err)
	}
	wantComposeUp := "compose -f " + filepath.Join(fixture.Workspace, "compose.yml") + " --env-file " + environmentRestoreGeneratedEnvFilePath(fixture.Workspace) + " up -d kafka"
	if !strings.Contains(string(dockerCalls), "image inspect apache/kafka:3.7.0") || !strings.Contains(string(dockerCalls), wantComposeUp) {
		t.Fatalf("restore should inspect local image and still run compose up:\n%s", dockerCalls)
	}
	if strings.Contains(string(dockerCalls), " pull kafka") {
		t.Fatalf("restore should skip compose pull for image accepted from local cache:\n%s", dockerCalls)
	}
}

func TestEnvironmentRestoreSkipsRegistryProbeWhenPullIsNotRequested(t *testing.T) {
	fixture := newEnvironmentRestoreDockerCLIFixture(t)
	fixture.writeDockerTool(t, `#!/usr/bin/env bash
printf '%s\n' "$*" >> "$DOCKER_CALLS_FILE"
if [ "$1" = "compose" ] && [ "$2" = "version" ]; then
  printf 'Docker Compose version v2.0.0\n'
  exit 0
fi
if [ "$1" = "manifest" ] && [ "$2" = "inspect" ]; then
  printf 'registry probe should not run without --pull\n' >&2
  exit 1
fi
if [ "$1" = "image" ] && [ "$2" = "inspect" ]; then
  printf '[{"Id":"sha256:local"}]\n'
  exit 0
fi
if [ "$1" = "compose" ] && [[ "$*" == *" ps -a --format json "* ]]; then
  service="${@: -1}"
  printf '{"Name":"%s","Service":"%s","State":"running","Health":"healthy"}\n' "$service" "$service"
fi
`)
	composeSource := filepath.Join(t.TempDir(), "compose.yml")
	writeFile(t, composeSource, "services:\n  kafka:\n    image: apache/kafka:3.7.0\n")
	runCLI(t, "environment", "register",
		"--store", fixture.StoreDSN,
		"--id", "env.compose.no-pull-local",
		"--compose-file", "compose.yml",
		"--compose-generated-file", "compose.yml="+composeSource,
		"--compose-service", "kafka",
		"--compose-skip-build",
		"--health-compose-service", "kafka",
		"--verification-workflow", "workflow.core-10",
	)

	out := runCLIWithEnv(t, fixture.DockerEnv, "environment", "restore", "--store", fixture.StoreDSN, "--workspace", fixture.Workspace, "--execute", "--json", "env.compose.no-pull-local")
	if !strings.Contains(out, `"ok": true`) {
		t.Fatalf("local image reuse without --pull should pass, got:\n%s", out)
	}
	dockerCalls, err := os.ReadFile(fixture.DockerCallsPath)
	if err != nil {
		t.Fatalf("read fake docker calls: %v", err)
	}
	if strings.Contains(string(dockerCalls), "manifest inspect") {
		t.Fatalf("restore without --pull should not run remote manifest probes:\n%s", dockerCalls)
	}
	if !strings.Contains(string(dockerCalls), "image inspect apache/kafka:3.7.0") {
		t.Fatalf("restore should still check the local image cache:\n%s", dockerCalls)
	}
}

func TestEnvironmentRestoreDoesNotPullComposeBuildServices(t *testing.T) {
	fixture := newEnvironmentRestoreDockerCLIFixture(t)
	fixture.writeDockerTool(t, `#!/usr/bin/env bash
printf '%s\n' "$*" >> "$DOCKER_CALLS_FILE"
if [ "$1" = "compose" ] && [ "$2" = "version" ]; then
  printf 'Docker Compose version v2.0.0\n'
  exit 0
fi
if [ "$1" = "image" ] && [ "$2" = "inspect" ]; then
  exit 1
fi
if [ "$1" = "compose" ] && [[ "$*" == *" ps -a --format json "* ]]; then
  service="${@: -1}"
  printf '{"Name":"%s","Service":"%s","State":"running","Health":"healthy"}\n' "$service" "$service"
fi
`)
	composeSource := filepath.Join(t.TempDir(), "compose.yml")
	writeFile(t, composeSource, `services:
  web:
    image: nginx:alpine
  llt:
    build:
      context: ${DOCKER_LLT_SIMULATOR_REPO}
    image: agent-testbench/llt-simulator:local
`)
	runCLI(t, "environment", "register",
		"--store", fixture.StoreDSN,
		"--id", "env.compose.build-filter",
		"--compose-file", "compose/docker-compose.yml",
		"--compose-generated-file", "compose/docker-compose.yml="+composeSource,
		"--compose-env", "DOCKER_LLT_SIMULATOR_REPO=$AGENT_TESTBENCH_WORKSPACE/agent-testbench-llt-simulator",
		"--compose-service", "web",
		"--compose-service", "llt",
		"--verification-workflow", "workflow.core-10",
	)

	out := runCLIWithEnv(t, fixture.DockerEnv, "environment", "restore", "--store", fixture.StoreDSN, "--workspace", fixture.Workspace, "--execute", "--json", "env.compose.build-filter")
	var report struct {
		OK bool `json:"ok"`
	}
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatalf("decode build service restore json: %v\n%s", err, out)
	}
	if !report.OK {
		t.Fatalf("build service restore report = %#v\n%s", report, out)
	}
	dockerCalls, err := os.ReadFile(fixture.DockerCallsPath)
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
