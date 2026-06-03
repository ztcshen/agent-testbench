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

func TestEnvironmentRestoreAcceptsComposeDependencyCompletedOneShotService(t *testing.T) {
	fixture := newEnvironmentRestoreDockerCLIFixture(t)
	for _, entry := range fixture.DockerEnv {
		key, value, ok := strings.Cut(entry, "=")
		if !ok {
			t.Fatalf("invalid fake docker env entry %q", entry)
		}
		t.Setenv(key, value)
	}
	fixture.writeDockerTool(t, `#!/usr/bin/env bash
printf '%s\n' "$*" >> "$DOCKER_CALLS_FILE"
if [ "$1" = "compose" ] && [ "$2" = "version" ]; then
  printf 'Docker Compose version v2.0.0\n'
  exit 0
fi
if [ "$1" = "manifest" ] && [ "$2" = "inspect" ]; then
  exit 0
fi
if [ "$1" = "compose" ] && [[ "$*" == *" ps -a --format json "* ]]; then
  service="${@: -1}"
  if [ "$service" = "s3-seed" ]; then
    printf '[{"Name":"demo-s3-seed","Service":"s3-seed","State":"exited","ExitCode":0}]\n'
  else
    printf '{"Name":"demo-%s","Service":"%s","State":"running","Health":"healthy"}\n' "$service" "$service"
  fi
  exit 0
fi
exit 0
`)

	report, err := buildEnvironmentRestoreReport(context.Background(), store.Environment{
		ID: "env.oneshot.seed",
		ComposeJSON: `{
			"composeFile":"compose.yml",
			"services":["app","s3-seed"],
			"generatedFiles":{
				"compose.yml":"services:\n  app:\n    image: alpine:3.20\n    depends_on:\n      s3-seed:\n        condition: service_completed_successfully\n  s3-seed:\n    image: minio/mc:RELEASE.2024-05-09T17-04-24Z\n"
			}
		}`,
		HealthChecksJSON:       `[{"kind":"compose-service","service":"s3-seed"}]`,
		VerificationWorkflowID: "workflow.core-10",
	}, fixture.Workspace, true, false, false, time.Second, environmentRestoreWorkflowOptions{}, environmentRestoreDockerCleanupOptions{})
	if err != nil {
		t.Fatalf("build restore report: %v", err)
	}
	if !report.OK || !report.Docker.OK {
		t.Fatalf("one-shot compose dependency restore should pass: %#v", report.Docker)
	}

	var seedCheck environmentRestoreHealthCheckReport
	for _, check := range report.Docker.HealthChecks {
		if check.Service == "s3-seed" {
			seedCheck = check
			break
		}
	}
	if !seedCheck.OK || seedCheck.State != environmentRestoreDockerStateExited || seedCheck.ExitCode != 0 || seedCheck.Expect != "service_completed_successfully" {
		encoded, _ := json.MarshalIndent(report.Docker.HealthChecks, "", "  ")
		t.Fatalf("s3-seed health should be accepted as completed one-shot: %s", encoded)
	}

	rawDockerCalls, err := os.ReadFile(fixture.DockerCallsPath)
	if err != nil {
		t.Fatalf("read fake docker calls: %v", err)
	}
	dockerCalls := string(rawDockerCalls)
	if !strings.Contains(dockerCalls, "ps -a --format json s3-seed") {
		t.Fatalf("restore should inspect one-shot service with docker compose ps -a:\n%s", dockerCalls)
	}
}

func TestEnvironmentRestoreWaitsForComposeDependencyCompletedOneShotExit(t *testing.T) {
	fixture := newEnvironmentRestoreDockerCLIFixture(t)
	for _, entry := range fixture.DockerEnv {
		key, value, ok := strings.Cut(entry, "=")
		if !ok {
			t.Fatalf("invalid fake docker env entry %q", entry)
		}
		t.Setenv(key, value)
	}
	fixture.writeDockerTool(t, `#!/usr/bin/env bash
printf '%s\n' "$*" >> "$DOCKER_CALLS_FILE"
if [ "$1" = "compose" ] && [ "$2" = "version" ]; then
  printf 'Docker Compose version v2.0.0\n'
  exit 0
fi
if [ "$1" = "manifest" ] && [ "$2" = "inspect" ]; then
  exit 0
fi
if [ "$1" = "compose" ] && [[ "$*" == *" ps -a --format json "* ]]; then
  service="${@: -1}"
  if [ "$service" = "s3-seed" ]; then
    attempts_file="$DOCKER_CALLS_FILE.s3-seed-attempts"
    attempts=0
    if [ -f "$attempts_file" ]; then attempts="$(cat "$attempts_file")"; fi
    attempts=$((attempts + 1))
    printf '%s\n' "$attempts" > "$attempts_file"
    if [ "$attempts" -eq 1 ]; then
      printf '[{"Name":"demo-s3-seed","Service":"s3-seed","State":"running","Health":"healthy"}]\n'
    else
      printf '[{"Name":"demo-s3-seed","Service":"s3-seed","State":"exited","ExitCode":0}]\n'
    fi
  else
    printf '{"Name":"demo-%s","Service":"%s","State":"running","Health":"healthy"}\n' "$service" "$service"
  fi
  exit 0
fi
exit 0
`)

	report, err := buildEnvironmentRestoreReport(context.Background(), store.Environment{
		ID: "env.oneshot.wait",
		ComposeJSON: `{
			"composeFile":"compose.yml",
			"services":["app","s3-seed"],
			"generatedFiles":{
				"compose.yml":"services:\n  app:\n    image: alpine:3.20\n    depends_on:\n      s3-seed:\n        condition: service_completed_successfully\n  s3-seed:\n    image: minio/mc:RELEASE.2024-05-09T17-04-24Z\n"
			}
		}`,
		HealthChecksJSON:       `[{"kind":"compose-service","service":"s3-seed"}]`,
		VerificationWorkflowID: "workflow.core-10",
	}, fixture.Workspace, true, false, false, 2*time.Second, environmentRestoreWorkflowOptions{}, environmentRestoreDockerCleanupOptions{})
	if err != nil {
		t.Fatalf("build restore report: %v", err)
	}
	if !report.OK || !report.Docker.OK {
		t.Fatalf("one-shot compose dependency restore should wait and pass: %#v", report.Docker)
	}
	rawDockerCalls, err := os.ReadFile(fixture.DockerCallsPath)
	if err != nil {
		t.Fatalf("read fake docker calls: %v", err)
	}
	if got := strings.Count(string(rawDockerCalls), "ps -a --format json s3-seed"); got < 2 {
		t.Fatalf("restore should keep polling until s3-seed exits, got %d calls:\n%s", got, rawDockerCalls)
	}
}

func TestParseComposeCompletedDependencyServicesStripsQuotedKeys(t *testing.T) {
	services := parseComposeCompletedDependencyServices(`services:
  app:
    image: alpine:3.20
    depends_on:
      "seed-job":
        condition: service_completed_successfully
  "seed-job":
    image: alpine:3.20
`)
	if !services["seed-job"] {
		t.Fatalf("quoted completed dependency key should be unquoted: %#v", services)
	}
	if services[`"seed-job"`] {
		t.Fatalf("quoted completed dependency key should not retain quote characters: %#v", services)
	}
}

func TestParseComposeCompletedDependencyServicesHandlesFlowStyleCondition(t *testing.T) {
	services := parseComposeCompletedDependencyServices(`services:
  app:
    image: alpine:3.20
    depends_on:
      seed-job: {condition: service_completed_successfully}
  seed-job:
    image: alpine:3.20
`)
	if !services["seed-job"] {
		t.Fatalf("flow-style completed dependency should be detected: %#v", services)
	}
}

func TestEnvironmentRestoreRefreshesCompletedExpectationAfterComposeSourceExists(t *testing.T) {
	workspace := t.TempDir()
	writeFile(t, filepath.Join(workspace, "compose.yml"), `services:
  app:
    image: alpine:3.20
    depends_on:
      seed-job:
        condition: service_completed_successfully
  seed-job:
    image: alpine:3.20
`)
	checks := []any{map[string]any{"kind": "compose-service", "service": "seed-job"}}
	compose := map[string]any{"composeFile": "compose.yml"}
	early := environmentRestoreRefreshCompletedExpectations(checks, compose, filepath.Join(workspace, "missing"))
	if strings.TrimSpace(valueString(early[0].(map[string]any)["expect"])) != "" {
		t.Fatalf("missing compose source should not infer completed expectation: %#v", early)
	}
	refreshed := environmentRestoreRefreshCompletedExpectations(checks, compose, workspace)
	if valueString(refreshed[0].(map[string]any)["expect"]) != "service_completed_successfully" {
		t.Fatalf("materialized compose source should infer completed expectation: %#v", refreshed)
	}
}
