package main

import (
	"bufio"
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestEnvironmentRestoreExecutesDockerComposeWithoutRepository(t *testing.T) {
	fixture := newEnvironmentRestoreDockerCLIFixture(t)
	fixture.writeWorkspaceFile(t, "compose.yml", "services: {}\n")

	runCLI(t, "environment", "register",
		"--store", fixture.StoreDSN,
		"--id", "env.docker.only",
		"--compose-file", "compose.yml",
		"--health-url", newHealthyTestURL(t),
		"--verification-workflow", "workflow.core-10",
	)

	out := runCLIWithEnv(t, fixture.DockerEnv, "environment", "restore", "--store", fixture.StoreDSN, "--workspace", fixture.Workspace, "--execute", "--json", "env.docker.only")
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

func TestEnvironmentRestoreStreamJSONEmitsAgentEvents(t *testing.T) {
	fixture := newEnvironmentRestoreDockerCLIFixture(t)
	fixture.writeWorkspaceFile(t, "compose.yml", "services: {}\n")

	runCLI(t, "environment", "register",
		"--store", fixture.StoreDSN,
		"--id", "env.stream.events",
		"--compose-file", "compose.yml",
		"--health-url", newHealthyTestURL(t),
		"--verification-workflow", "workflow.core-10",
	)

	out := runCLIWithEnv(t, fixture.DockerEnv, "environment", "restore", "--store", fixture.StoreDSN, "--workspace", fixture.Workspace, "--execute", "--output-format", "stream-json", "env.stream.events")
	events := decodeAgentStreamEvents(t, out)
	if len(events) < 6 {
		t.Fatalf("expected multiple stream events, got %d: %s", len(events), out)
	}
	if valueString(events[0]["type"]) != "run_started" || valueString(events[0]["status"]) != "running" {
		t.Fatalf("first stream event = %#v", events[0])
	}
	for _, phase := range []string{
		"docker.prepare",
		"docker.compose.validate",
		"docker.native-assets",
		"docker.compose.execute",
		"docker.edge-assets",
		"docker.health",
	} {
		if !agentStreamHasEvent(events, "step_started", phase, "running", "") {
			t.Fatalf("stream missing %s start: %#v", phase, events)
		}
		if !agentStreamHasEvent(events, "step_completed", phase, "passed", "") {
			t.Fatalf("stream missing %s completion: %#v", phase, events)
		}
	}
	if !agentStreamHasEvent(events, "tool_call_started", "command", "started", "docker compose up") {
		t.Fatalf("stream missing docker compose command start: %#v", events)
	}
	if !agentStreamHasEvent(events, "tool_call_completed", "command", "completed", "docker compose up") {
		t.Fatalf("stream missing docker compose command completion: %#v", events)
	}
	last := events[len(events)-1]
	report := mapFromReportAny(last["report"])
	if valueString(last["type"]) != "run_completed" || valueString(last["status"]) != "passed" || !boolFromReportAny(report["ok"]) {
		t.Fatalf("last stream event = %#v", last)
	}
}

func TestEnvironmentRestoreStreamJSONEmitsRunStartedBeforePreflightDocker(t *testing.T) {
	fixture := newEnvironmentRestoreDockerCLIFixture(t)
	fixture.writeDockerTool(t, `#!/usr/bin/env bash
printf '%s\n' "$*" >> "$DOCKER_CALLS_FILE"
if [ "$1" = "compose" ] && [ "$2" = "version" ]; then
  sleep 2
  printf 'Docker Compose version v2.0.0\n'
  exit 0
fi
`)
	fixture.writeWorkspaceFile(t, "compose.yml", "services: {}\n")
	runCLI(t, "environment", "register",
		"--store", fixture.StoreDSN,
		"--id", "env.stream.preflight",
		"--compose-file", "compose.yml",
		"--verification-workflow", "workflow.core-10",
	)

	cmd := exec.Command(os.Args[0], "environment", "restore",
		"--store", fixture.StoreDSN,
		"--workspace", fixture.Workspace,
		"--execute",
		"--output-format", "stream-json",
		"env.stream.preflight",
	)
	cmd.Env = append(append(os.Environ(), fixture.DockerEnv...), "AGENT_TESTBENCH_TEST_CLI=1")
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatalf("stdout pipe: %v", err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatalf("start restore: %v", err)
	}
	defer func() {
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
		_ = cmd.Wait()
	}()

	lines := make(chan string, 1)
	go func() {
		scanner := bufio.NewScanner(stdout)
		if scanner.Scan() {
			lines <- scanner.Text()
			return
		}
		lines <- ""
	}()
	select {
	case line := <-lines:
		if !strings.Contains(line, `"type":"run_started"`) || !strings.Contains(line, `"phase":"environment.restore"`) {
			t.Fatalf("first stream line = %q", line)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("restore should emit run_started before blocking Docker preflight")
	}
}

func TestEnvironmentRestoreStreamJSONEmitsComposeExecuteWaitingObservation(t *testing.T) {
	fixture := newEnvironmentRestoreDockerCLIFixture(t)
	fixture.writeWorkspaceFile(t, "compose.yml", "services: {}\n")

	runCLI(t, "environment", "register",
		"--store", fixture.StoreDSN,
		"--id", "env.stream.compose-wait",
		"--compose-file", "compose.yml",
		"--health-url", newHealthyTestURL(t),
		"--verification-workflow", "workflow.core-10",
	)

	env := append([]string{}, fixture.DockerEnv...)
	env = append(env,
		"AGENT_TESTBENCH_FAKE_DOCKER_COMPOSE_UP_SLEEP=0.05",
		"AGENT_TESTBENCH_COMPOSE_EXECUTE_PROGRESS_INTERVAL_MS=1",
	)
	out := runCLIWithEnv(t, env, "environment", "restore", "--store", fixture.StoreDSN, "--workspace", fixture.Workspace, "--execute", "--output-format", "stream-json", "env.stream.compose-wait")
	events := decodeAgentStreamEvents(t, out)
	if !agentStreamHasEvent(events, "tool_observation", "docker.compose.execute", "waiting", "docker compose up") {
		t.Fatalf("stream missing docker compose execute waiting observation: %#v", events)
	}
}

func TestEnvironmentRestoreStreamJSONSkipsWorkflowWhenDockerIsNotReady(t *testing.T) {
	fixture := newEnvironmentRestoreDockerCLIFixture(t)
	fixture.writeWorkspaceFile(t, "compose.yml", "services: {}\n")

	runCLI(t, "environment", "register",
		"--store", fixture.StoreDSN,
		"--id", "env.stream.workflow-gate",
		"--compose-file", "compose.yml",
		"--health-command", "echo nope && exit 7",
		"--verification-workflow", "workflow.core-10",
	)

	env := append([]string{}, fixture.DockerEnv...)
	env = append(env, "AGENT_TESTBENCH_HEALTH_PROGRESS_INTERVAL_MS=1")
	out := runCLIFailsWithEnv(t, env, "environment", "restore",
		"--store", fixture.StoreDSN,
		"--workspace", fixture.Workspace,
		"--execute",
		"--health-timeout-seconds", "1",
		"--run-workflow",
		"--server-url", "http://127.0.0.1:1",
		"--output-format", "stream-json",
		"env.stream.workflow-gate",
	)
	events := decodeAgentStreamEvents(t, agentStreamJSONEventLines(out))
	if !agentStreamHasEvent(events, "step_completed", "docker.health", "failed", "") {
		t.Fatalf("stream missing failed docker health completion: %#v", events)
	}
	if !agentStreamHasEvent(events, "tool_observation", "docker.health", "waiting", "command health-01") {
		t.Fatalf("stream missing docker health waiting observation: %#v", events)
	}
	if agentStreamHasEvent(events, "tool_observation", "health.wait", "waiting", "") {
		t.Fatalf("stream should report health waiting under docker.health: %#v", events)
	}
	if !agentStreamHasEvent(events, "step_completed", "workflow.acceptance", "skipped", "workflow.core-10") {
		t.Fatalf("stream missing skipped workflow gate: %#v", events)
	}
	if agentStreamHasEvent(events, "step_started", "workflow.acceptance", "running", "workflow.core-10") {
		t.Fatalf("workflow should not start when Docker health is not ready: %#v", events)
	}
}

func TestEnvironmentRestoreRunsMixedHealthProbes(t *testing.T) {
	fixture := newEnvironmentRestoreDockerCLIFixture(t)
	fixture.writeWorkspaceFile(t, "compose.yml", "services: {}\n")
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

	runCLI(t, "environment", "register",
		"--store", fixture.StoreDSN,
		"--id", "env.health.mixed",
		"--compose-file", "compose.yml",
		"--health-url", newHealthyTestURL(t),
		"--health-tcp", listener.Addr().String(),
		"--health-command", "test -f compose.yml",
		"--health-compose-service", "web",
		"--verification-workflow", "workflow.core-10",
	)

	out := runCLIWithEnv(t, fixture.DockerEnv, "environment", "restore", "--store", fixture.StoreDSN, "--workspace", fixture.Workspace, "--execute", "--json", "env.health.mixed")
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
	fixture := newEnvironmentRestoreDockerCLIFixture(t)
	fixture.writeWorkspaceFile(t, "compose.yml", "services: {}\n")
	runCLI(t, "environment", "register",
		"--store", fixture.StoreDSN,
		"--id", "env.health.fail",
		"--compose-file", "compose.yml",
		"--health-command", "echo nope && exit 7",
		"--verification-workflow", "workflow.core-10",
	)

	out := runCLIFailsWithEnv(t, fixture.DockerEnv, "environment", "restore", "--store", fixture.StoreDSN, "--workspace", fixture.Workspace, "--execute", "--health-timeout-seconds", "1", "--json", "env.health.fail")
	if !strings.Contains(out, `"kind": "command"`) || !strings.Contains(out, "exit status 7") {
		t.Fatalf("health failure output = %q", out)
	}
	inspectOut := runCLI(t, "environment", "inspect", "--store", fixture.StoreDSN, "--json", "env.health.fail")
	if !strings.Contains(inspectOut, `"phase": "health-check"`) {
		t.Fatalf("health failure should persist health-check phase: %s", inspectOut)
	}
}

func TestEnvironmentRestoreHealthWaitReportsProgress(t *testing.T) {
	var progress strings.Builder
	ctx := contextWithEnvironmentRestoreProgress(context.Background(), &progress)
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer target.Close()

	check := waitEnvironmentRestoreURLHealthCheck(ctx, environmentRestoreHealthCheckReport{
		ID:   "app-ready",
		Kind: "url",
		URL:  target.URL + "/ready",
	}, 20*time.Millisecond)
	if check.OK {
		t.Fatalf("health check should fail")
	}
	logs := progress.String()
	if !strings.Contains(logs, "restore health checking") || !strings.Contains(logs, target.URL+"/ready") || !strings.Contains(logs, "HTTP 503") {
		t.Fatalf("progress logs = %q", logs)
	}
}

func TestEnvironmentRestoreCommandHealthTimeoutBoundsSlowProbe(t *testing.T) {
	started := time.Now()
	check := waitEnvironmentRestoreCommandHealthCheck(context.Background(), environmentRestoreHealthCheckReport{
		ID:      "slow-probe",
		Kind:    "command",
		Command: "sleep 2",
	}, 20*time.Millisecond, "")

	if check.OK {
		t.Fatalf("slow probe should fail")
	}
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("slow probe ignored health timeout: elapsed=%s check=%#v", elapsed, check)
	}
	if !strings.Contains(check.Error, "deadline") && !strings.Contains(check.Error, "killed") {
		t.Fatalf("slow probe error should explain timeout, got %#v", check)
	}
}

func TestEnvironmentRestoreAcceptsExplicitCompletedOneShotComposeServiceHealth(t *testing.T) {
	fakeBin := t.TempDir()
	callsPath := filepath.Join(fakeBin, "docker-calls.txt")
	t.Setenv("PATH", fakeBin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("DOCKER_CALLS_FILE", callsPath)
	writeFile(t, filepath.Join(fakeBin, "docker"), `#!/usr/bin/env bash
printf '%s\n' "$*" >> "$DOCKER_CALLS_FILE"
if [ "$1" = "compose" ] && [ "$2" = "version" ]; then
  printf 'Docker Compose version v2.0.0\n'
  exit 0
fi
if [ "$1" = "compose" ]; then
  service=""
  saw_ps=0
  for arg in "$@"; do
    if [ "$arg" = "ps" ]; then
      saw_ps=1
    fi
    service="$arg"
  done
  if [ "$saw_ps" = "1" ]; then
    printf '{"Name":"%s","Service":"%s","State":"exited","ExitCode":0}\n' "$service" "$service"
    exit 0
  fi
fi
exit 0
`)
	if err := os.Chmod(filepath.Join(fakeBin, "docker"), 0o755); err != nil {
		t.Fatalf("chmod fake docker: %v", err)
	}
	workspace := t.TempDir()
	check := waitEnvironmentRestoreComposeServiceHealthCheck(context.Background(), environmentRestoreHealthCheckReport{
		Kind:    "compose-service",
		Service: "s3-seed",
		Expect:  "completed",
	}, 3*time.Second, workspace, []string{"-f", filepath.Join(workspace, "compose.yml")})
	if !check.OK || check.State != environmentRestoreDockerStateExited || check.ExitCode != 0 {
		t.Fatalf("explicit one-shot compose service should pass via exit code: %#v", check)
	}
	normal := waitEnvironmentRestoreComposeServiceHealthCheck(context.Background(), environmentRestoreHealthCheckReport{
		Kind:    "compose-service",
		Service: "app",
	}, 500*time.Millisecond, workspace, []string{"-f", filepath.Join(workspace, "compose.yml")})
	if normal.OK || normal.State != environmentRestoreDockerStateExited || normal.ExitCode != 0 {
		t.Fatalf("non-one-shot exited service should not pass: %#v", normal)
	}
}

func TestEnvironmentRestoreAcceptsExplicitCompletedOneShotContainerHealth(t *testing.T) {
	fakeBin := t.TempDir()
	t.Setenv("PATH", fakeBin+string(os.PathListSeparator)+os.Getenv("PATH"))
	writeFile(t, filepath.Join(fakeBin, "docker"), `#!/usr/bin/env bash
if [ "$1" = "inspect" ]; then
  if [ "${@: -1}" = "seed-running" ]; then
    printf 'running healthy 0\n'
    exit 0
  fi
  printf 'exited  0\n'
  exit 0
fi
exit 0
`)
	if err := os.Chmod(filepath.Join(fakeBin, "docker"), 0o755); err != nil {
		t.Fatalf("chmod fake docker: %v", err)
	}
	check := waitEnvironmentRestoreContainerHealthCheck(context.Background(), environmentRestoreHealthCheckReport{
		Kind:      "container",
		Container: "seed-job",
		OneShot:   true,
	}, 2*time.Second)
	if !check.OK || check.State != environmentRestoreDockerStateExited || check.ExitCode != 0 {
		t.Fatalf("explicit one-shot container should pass via exit code: %#v", check)
	}
	running := waitEnvironmentRestoreContainerHealthCheck(context.Background(), environmentRestoreHealthCheckReport{
		Kind:      "container",
		Container: "seed-running",
		Expect:    "service_completed_successfully",
	}, 500*time.Millisecond)
	if running.OK || running.State != "running" {
		t.Fatalf("completed one-shot container should not pass while still running: %#v", running)
	}
	normal := waitEnvironmentRestoreContainerHealthCheck(context.Background(), environmentRestoreHealthCheckReport{
		Kind:      "container",
		Container: "app",
	}, 500*time.Millisecond)
	if normal.OK || normal.State != environmentRestoreDockerStateExited || normal.ExitCode != 0 {
		t.Fatalf("non-one-shot exited container should not pass: %#v", normal)
	}
}
