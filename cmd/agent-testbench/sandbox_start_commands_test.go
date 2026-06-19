package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"agent-testbench/internal/store"
	"agent-testbench/internal/store/sqlite"
)

type sandboxStartCommandReport struct {
	OK         bool                         `json:"ok"`
	DryRun     bool                         `json:"dryRun"`
	WorkflowID string                       `json:"workflowId"`
	Runtime    statusRuntimeReport          `json:"runtime"`
	Services   []sandboxStartCommandService `json:"services"`
	Error      string                       `json:"error"`
	Counts     struct {
		Planned int `json:"planned"`
		Failed  int `json:"failed"`
	} `json:"counts"`
}

type sandboxStartCommandService struct {
	ID              string `json:"id"`
	Command         string `json:"command"`
	RecoveryCommand string `json:"recoveryCommand"`
	Readiness       string `json:"readiness"`
	ExitCode        int    `json:"exitCode"`
	Skipped         bool   `json:"skipped"`
	Planned         bool   `json:"planned"`
	SkipReason      string `json:"skipReason"`
	Warning         string `json:"warning"`
	Error           string `json:"error"`
}

type sandboxStartFixture struct {
	storePath           string
	startedPath         string
	platformStartedPath string
}

func TestSandboxStartCommandRunsStartupCommandsFromStore(t *testing.T) {
	fixture := writeSandboxStartStoreFixture(t)
	report := runSandboxStartJSON(t, "sqlite://"+fixture.storePath, "sandbox start")
	requireSandboxStartServices(t, report)
	requireSandboxStartupSideEffects(t, fixture)
}

func TestSandboxStartRecreatesMissingComposeBackedContainer(t *testing.T) {
	storePath := writeSandboxComposeServiceFixture(t, "docker start sandbox-worker-service", "worker-service")
	fakeEnv, callsPath := fakeDockerCommand(t)
	installSandboxDockerTool(t, callsPath, `#!/usr/bin/env bash
printf '%s\n' "$*" >> "$DOCKER_CALLS_FILE"
if [ "$1" = "start" ]; then
  printf 'Error response from daemon: No such container: %s\n' "$2" >&2
  exit 1
fi
if [ "$1" = "compose" ] && [ "$2" = "up" ]; then
  exit 0
fi
if [ "$1" = "compose" ] && [[ "$*" == *" ps -a --format json "* ]]; then
  service="${@: -1}"
  printf '{"Name":"sandbox-%s","Service":"%s","State":"running","Health":"healthy"}\n' "$service" "$service"
  exit 0
fi
exit 0
`)

	out := runCLIWithEnv(t, fakeEnv, "sandbox", "start", "--store", "sqlite://"+storePath, "--service", "worker-service", "--json")
	var report sandboxStartCommandReport
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatalf("decode missing-container compose recovery report: %v\n%s", err, out)
	}
	if !report.OK || report.Counts.Failed != 0 || len(report.Services) != 1 {
		t.Fatalf("missing-container compose recovery report = %#v", report)
	}
	service := report.Services[0]
	if service.RecoveryCommand != "docker compose up -d worker-service" || service.Readiness != "compose-service-running" || !strings.Contains(service.Warning, "no healthUrl") {
		t.Fatalf("missing-container compose recovery service = %#v", service)
	}
	calls, err := os.ReadFile(callsPath)
	if err != nil {
		t.Fatalf("read docker calls: %v", err)
	}
	joined := string(calls)
	if !strings.Contains(joined, "start sandbox-worker-service") || !strings.Contains(joined, "compose up -d worker-service") || !strings.Contains(joined, "compose ps -a --format json worker-service") {
		t.Fatalf("missing-container compose recovery docker calls:\n%s", joined)
	}
}

func TestSandboxStartFailsWhenComposeServiceExitsAfterStartup(t *testing.T) {
	storePath := writeSandboxComposeServiceFixture(t, "docker compose up -d worker-service", "worker-service")
	fakeEnv, callsPath := fakeDockerCommand(t)
	installSandboxDockerTool(t, callsPath, `#!/usr/bin/env bash
printf '%s\n' "$*" >> "$DOCKER_CALLS_FILE"
if [ "$1" = "compose" ] && [ "$2" = "up" ]; then
  exit 0
fi
if [ "$1" = "compose" ] && [[ "$*" == *" ps -a --format json "* ]]; then
  service="${@: -1}"
  printf '{"Name":"sandbox-%s","Service":"%s","State":"exited","ExitCode":1}\n' "$service" "$service"
  exit 0
fi
exit 0
`)

	out := runCLIFailsWithEnv(t, fakeEnv, "sandbox", "start", "--store", "sqlite://"+storePath, "--service", "worker-service", "--json")
	var report sandboxStartCommandReport
	if err := json.Unmarshal([]byte(extractJSONObject(t, out)), &report); err != nil {
		t.Fatalf("decode compose-exited report: %v\n%s", err, out)
	}
	if report.OK || report.Counts.Failed != 1 || len(report.Services) != 1 {
		t.Fatalf("compose-exited report = %#v", report)
	}
	service := report.Services[0]
	if service.ExitCode == 0 || !strings.Contains(service.Error, "not running after startup") || service.Readiness != "compose-service-not-running" {
		t.Fatalf("compose-exited service = %#v", service)
	}
	calls, err := os.ReadFile(callsPath)
	if err != nil {
		t.Fatalf("read docker calls: %v", err)
	}
	if !strings.Contains(string(calls), "compose ps -a --format json worker-service") {
		t.Fatalf("compose-exited should inspect service state:\n%s", calls)
	}
}

func TestSandboxStartFailsFastWhenHealthCheckedComposeServiceExits(t *testing.T) {
	storePath := writeSandboxComposeHealthServiceFixture(t, "docker compose up -d worker-service", "worker-service", "http://127.0.0.1:1/health")
	fakeEnv, callsPath := fakeDockerCommand(t)
	installSandboxDockerTool(t, callsPath, `#!/usr/bin/env bash
printf '%s\n' "$*" >> "$DOCKER_CALLS_FILE"
if [ "$1" = "compose" ] && [ "$2" = "up" ]; then
  exit 0
fi
if [ "$1" = "compose" ] && [[ "$*" == *" ps -a --format json "* ]]; then
  service="${@: -1}"
  printf '{"Name":"sandbox-%s","Service":"%s","State":"exited","ExitCode":1}\n' "$service" "$service"
  exit 0
fi
exit 0
`)

	started := time.Now()
	out := runCLIFailsWithEnv(t, fakeEnv, "sandbox", "start", "--store", "sqlite://"+storePath, "--service", "worker-service", "--timeout-seconds", "5", "--json")
	elapsed := time.Since(started)
	var report sandboxStartCommandReport
	if err := json.Unmarshal([]byte(extractJSONObject(t, out)), &report); err != nil {
		t.Fatalf("decode health-compose-exited report: %v\n%s", err, out)
	}
	if elapsed > 2*time.Second {
		t.Fatalf("health-checked compose exit should fail fast, elapsed=%s report=%#v", elapsed, report)
	}
	if report.OK || report.Counts.Failed != 1 || len(report.Services) != 1 {
		t.Fatalf("health-compose-exited report = %#v", report)
	}
	service := report.Services[0]
	if service.Readiness != sandboxComposeServiceStoppedReadiness ||
		!strings.Contains(service.Error, "compose service is not running after startup") ||
		!strings.Contains(service.Error, "exitCode=1") {
		t.Fatalf("health-compose-exited service = %#v", service)
	}
	calls, err := os.ReadFile(callsPath)
	if err != nil {
		t.Fatalf("read docker calls: %v", err)
	}
	if !strings.Contains(string(calls), "compose ps -a --format json worker-service") {
		t.Fatalf("health-compose-exited should inspect service state while waiting:\n%s", calls)
	}
}

func TestSandboxStartPreflightsDockerDaemonBeforeStartup(t *testing.T) {
	storePath := writeSandboxComposeServiceFixture(t, "docker compose up -d worker-service", "worker-service")
	fakeEnv, callsPath := fakeDockerCommand(t)
	installSandboxDockerTool(t, callsPath, `#!/usr/bin/env bash
printf '%s\n' "$*" >> "$DOCKER_CALLS_FILE"
if [ "$1" = "info" ]; then
  printf 'Cannot connect to the Docker daemon at unix:///tmp/docker.sock. Is the docker daemon running?\n' >&2
  exit 1
fi
exit 0
`)

	out := runCLIFailsWithEnv(t, fakeEnv, "sandbox", "start", "--store", "sqlite://"+storePath, "--service", "worker-service", "--json")
	var report sandboxStartCommandReport
	if err := json.Unmarshal([]byte(extractJSONObject(t, out)), &report); err != nil {
		t.Fatalf("decode docker daemon preflight report: %v\n%s", err, out)
	}
	if report.OK || report.Counts.Failed != 1 || len(report.Services) != 1 {
		t.Fatalf("docker daemon preflight report = %#v", report)
	}
	service := report.Services[0]
	if service.Readiness != "docker-daemon-unavailable" ||
		!strings.Contains(service.Error, "environment-not-ready") ||
		!strings.Contains(service.Error, "Docker daemon unavailable") {
		t.Fatalf("docker daemon preflight service = %#v", service)
	}
	calls, err := os.ReadFile(callsPath)
	if err != nil {
		t.Fatalf("read docker calls: %v", err)
	}
	if strings.Contains(string(calls), "compose up") {
		t.Fatalf("docker daemon preflight must not run startup command:\n%s", calls)
	}
}

func TestSandboxStartPreflightsDockerThroughConfiguredWrapper(t *testing.T) {
	fakeEnv, callsPath := fakeDockerCommand(t)
	dir := filepath.Dir(callsPath)
	wrapperCallsPath := filepath.Join(dir, "docker-wrapper-calls.txt")
	wrapperPath := filepath.Join(dir, "docker-wrapper")
	installSandboxDockerTool(t, callsPath, `#!/usr/bin/env bash
printf '%s\n' "$*" >> "$DOCKER_CALLS_FILE"
if [ "$1" = "info" ]; then
  printf 'bare docker cannot access daemon\n' >&2
  exit 1
fi
if [ "$1" = "compose" ] && [ "$2" = "ps" ]; then
  printf 'bare docker compose cannot access daemon\n' >&2
  exit 1
fi
exit 0
`)
	writeFile(t, wrapperPath, `#!/usr/bin/env bash
printf '%s\n' "$*" >> "$DOCKER_WRAPPER_CALLS_FILE"
if [ "$1" = "info" ]; then
  printf 'wrapper daemon ok\n'
  exit 0
fi
if [ "$1" = "compose" ] && [ "$2" = "ps" ]; then
  printf '{"Name":"worker-service","Service":"worker-service","State":"running","Health":"healthy"}\n'
  exit 0
fi
exit 0
`)
	if err := os.Chmod(wrapperPath, 0o755); err != nil {
		t.Fatalf("chmod docker wrapper: %v", err)
	}
	storePath := writeSandboxComposeServiceFixture(t, wrapperPath+" compose up -d worker-service", "worker-service")
	env := append(fakeEnv, "DOCKER_WRAPPER_CALLS_FILE="+wrapperCallsPath)

	out := runCLIWithEnv(t, env, "sandbox", "start", "--store", "sqlite://"+storePath, "--service", "worker-service", "--json")
	var report sandboxStartCommandReport
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatalf("decode wrapper docker preflight report: %v\n%s", err, out)
	}
	if !report.OK || len(report.Services) != 1 || report.Services[0].ExitCode != 0 || report.Services[0].Readiness != "compose-service-running" {
		t.Fatalf("wrapper docker preflight should allow startup = %#v", report)
	}
	bareCalls, err := os.ReadFile(callsPath)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("read bare docker calls: %v", err)
	}
	if strings.Contains(string(bareCalls), "info") {
		t.Fatalf("preflight should not use bare docker when startup uses wrapper:\n%s", bareCalls)
	}
	if strings.Contains(string(bareCalls), "compose ps") {
		t.Fatalf("readiness should not use bare docker when startup uses wrapper:\n%s", bareCalls)
	}
	wrapperCalls, err := os.ReadFile(wrapperCallsPath)
	if err != nil {
		t.Fatalf("read wrapper docker calls: %v", err)
	}
	if !strings.Contains(string(wrapperCalls), "info") ||
		!strings.Contains(string(wrapperCalls), "compose up -d worker-service") ||
		!strings.Contains(string(wrapperCalls), "compose ps -a --format json worker-service") {
		t.Fatalf("configured wrapper should handle preflight, startup, and readiness:\n%s", wrapperCalls)
	}
}

func TestSandboxDockerPreflightPreservesDockerGlobalOptions(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want []string
	}{
		{
			name: "context",
			in:   "docker --context remote compose up -d worker-service",
			want: []string{"docker", sandboxDockerContextOption, "remote", "info"},
		},
		{
			name: "host",
			in:   "docker -H ssh://host compose up -d worker-service",
			want: []string{"docker", "-H", "ssh://host", "info"},
		},
		{
			name: "sudo context",
			in:   "sudo docker --context remote compose up -d worker-service",
			want: []string{sandboxSudoCommandToken, "docker", sandboxDockerContextOption, "remote", "info"},
		},
		{
			name: "sudo non interactive",
			in:   "sudo -n docker compose up -d worker-service",
			want: []string{sandboxSudoCommandToken, "-n", "docker", "info"},
		},
		{
			name: "sudo preserve env context",
			in:   "sudo -E docker --context remote compose up -d worker-service",
			want: []string{sandboxSudoCommandToken, "-E", "docker", sandboxDockerContextOption, "remote", "info"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := sandboxDockerPreflightCommand(tt.in); !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("preflight command = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestSandboxStartComposeServiceIgnoresNonDockerComposeText(t *testing.T) {
	service := store.CatalogService{StartupCommand: "printf compose up worker-service"}
	if got := sandboxStartComposeService(service, service.StartupCommand); got != "" {
		t.Fatalf("non-Docker startup text should not infer compose service, got %q", got)
	}
}

func TestSandboxStartComposeServiceIgnoresDockerRunArgsWithComposeText(t *testing.T) {
	service := store.CatalogService{StartupCommand: "docker run alpine compose up worker-service"}
	if got := sandboxStartComposeService(service, service.StartupCommand); got != "" {
		t.Fatalf("docker run command args should not infer compose service, got %q", got)
	}
}

func TestSandboxStartJSONIncludesRuntimeConsistencyEvidence(t *testing.T) {
	fixture := writeSandboxStartStoreFixture(t)
	report := runSandboxStartJSON(t, "sqlite://"+fixture.storePath, "sandbox start runtime evidence")
	if strings.TrimSpace(report.Runtime.Path) == "" || strings.TrimSpace(report.Runtime.ActivePath) == "" {
		t.Fatalf("sandbox start should include runtime evidence: %#v", report.Runtime)
	}
}

func TestSandboxStartStreamJSONEmitsAgentEvents(t *testing.T) {
	fixture := writeSandboxStartStoreFixture(t)
	out := runCLI(t, "sandbox", "start", "--store", "sqlite://"+fixture.storePath, "--output-format", "stream-json")
	events := decodeAgentStreamEvents(t, out)
	if len(events) < 8 {
		t.Fatalf("expected sandbox start stream events, got %d: %s", len(events), out)
	}
	if valueString(events[0]["type"]) != "run_started" || valueString(events[0]["phase"]) != "sandbox.start" {
		t.Fatalf("first sandbox stream event = %#v", events[0])
	}
	if !agentStreamHasEvent(events, "step_started", "sandbox.service", "running", "entry-service") {
		t.Fatalf("stream missing entry-service step start: %#v", events)
	}
	if !agentStreamHasEvent(events, "tool_call_started", "command", "started", "/bin/sh") {
		t.Fatalf("stream missing startup command start: %#v", events)
	}
	if !agentStreamHasEvent(events, "step_completed", "sandbox.service", "skipped", "documented-service") {
		t.Fatalf("stream missing documented-service skipped step: %#v", events)
	}
	last := events[len(events)-1]
	report := mapFromReportAny(last["report"])
	if valueString(last["type"]) != "run_completed" || valueString(last["status"]) != "passed" || !boolFromReportAny(report["ok"]) {
		t.Fatalf("last sandbox stream event = %#v", last)
	}
	requireSandboxStartupSideEffects(t, fixture)
}

func TestSandboxStartOutputFormatRejectsJSONConflict(t *testing.T) {
	fixture := writeSandboxStartStoreFixture(t)
	out := runCLIFails(t, "sandbox", "start", "--store", "sqlite://"+fixture.storePath, "--json", "--output-format", "stream-json")
	if !strings.Contains(out, "--json cannot be combined with --output-format stream-json") {
		t.Fatalf("sandbox start output-format conflict error = %q", out)
	}
	requireSandboxNoStartupSideEffects(t, fixture)
}

func TestSandboxStartMissingServiceExplainsRegistryBoundary(t *testing.T) {
	fixture := writeSandboxStartStoreFixture(t)

	out := runCLIFails(t, "sandbox", "start", "--store", "sqlite://"+fixture.storePath, "--service", "mysql")
	if !strings.Contains(out, "profile service registry") || !strings.Contains(out, "environment restore") {
		t.Fatalf("missing service error should explain registry boundary, got %q", out)
	}
}

func TestSandboxStartRejectsEnvironmentBoundWorkflow(t *testing.T) {
	fixture := writeSandboxStartStoreFixture(t)
	ctx := context.Background()
	s, err := openStore(ctx, "sqlite://"+fixture.storePath)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer s.Close()
	if _, err := s.UpsertEnvironment(ctx, store.Environment{
		ID:                     "env.fixture",
		DisplayName:            "Fixture Environment",
		VerificationWorkflowID: "workflow.fixture",
	}); err != nil {
		t.Fatalf("upsert environment: %v", err)
	}

	out := runCLIFails(t, "sandbox", "start", "--store", "sqlite://"+fixture.storePath, "--workflow", "workflow.fixture", "--dry-run", "--json")
	if !strings.Contains(out, "environment restore env.fixture") ||
		!strings.Contains(out, "--store STORE_NAME_OR_DSN") ||
		!strings.Contains(out, "--workspace WORKSPACE") ||
		!strings.Contains(out, "--run-workflow") {
		t.Fatalf("environment-bound workflow should direct operator to environment restore, got %q", out)
	}
	var report sandboxStartCommandReport
	if err := json.NewDecoder(strings.NewReader(out)).Decode(&report); err != nil {
		t.Fatalf("environment-bound workflow should emit JSON failure report: %v\n%s", err, out)
	}
	if report.OK || !strings.Contains(report.Error, "environment restore env.fixture") {
		t.Fatalf("environment-bound workflow JSON failure report = %#v", report)
	}
	requireSandboxNoStartupSideEffects(t, fixture)
}

func TestSandboxStartStreamJSONCompletesMissingServiceFailure(t *testing.T) {
	fixture := writeSandboxStartStoreFixture(t)
	out := runCLIFails(t, "sandbox", "start", "--store", "sqlite://"+fixture.storePath, "--service", "mysql", "--output-format", "stream-json")
	if !strings.Contains(out, "profile service registry") {
		t.Fatalf("missing service error should still be printed, got %q", out)
	}
	events := decodeAgentStreamEvents(t, agentStreamJSONEventLines(out))
	if !agentStreamHasEvent(events, "run_started", "sandbox.start", "running", "profile-service-registry") {
		t.Fatalf("stream missing sandbox run start: %#v", events)
	}
	if !agentStreamHasEvent(events, "run_completed", "sandbox.start", "failed", "profile-service-registry") {
		t.Fatalf("stream missing failed sandbox run completion: %#v", events)
	}
}

func TestSandboxStartStreamJSONCompletesStoreOpenFailure(t *testing.T) {
	storePath := filepath.Join(t.TempDir(), "missing", "store.sqlite")
	out := runCLIFails(t, "sandbox", "start", "--store", "sqlite://"+storePath, "--output-format", "stream-json")
	events := decodeAgentStreamEvents(t, agentStreamJSONEventLines(out))
	if !agentStreamHasEvent(events, "run_started", "sandbox.start", "running", "profile-service-registry") {
		t.Fatalf("stream missing sandbox run start: %#v", events)
	}
	if !agentStreamHasEvent(events, "run_completed", "sandbox.start", "failed", "profile-service-registry") {
		t.Fatalf("stream missing failed sandbox run completion: %#v", events)
	}
}

func agentStreamJSONEventLines(output string) string {
	lines := []string{}
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "{") {
			lines = append(lines, line)
		}
	}
	return strings.Join(lines, "\n")
}

func writeSandboxStartStoreFixture(t *testing.T) sandboxStartFixture {
	t.Helper()

	dir := t.TempDir()
	storePath := filepath.Join(dir, "store.sqlite")
	startedPath := filepath.Join(dir, "started.txt")
	platformStartedPath := filepath.Join(dir, "platform-started.txt")
	ctx := context.Background()
	s, err := sqlite.Open(ctx, sqlite.Config{Path: storePath})
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	if err := s.ReplaceProfileCatalog(ctx, store.ProfileCatalog{
		ProfileID: "sandbox",
		IndexedAt: time.Now().UTC(),
		Services: []store.CatalogService{
			{
				ID:             "entry-service",
				DisplayName:    "Entry Service",
				Kind:           "app",
				StartupCommand: fmt.Sprintf("printf entry-service > %q", startedPath),
				Status:         "active",
			},
			{
				ID:             "platform-service",
				DisplayName:    "Platform Service",
				Kind:           "platform",
				StartupCommand: fmt.Sprintf("printf platform-service > %q", platformStartedPath),
				Status:         "active",
			},
			{
				ID:          "documented-service",
				DisplayName: "Documented Service",
				Kind:        "external",
				Status:      "active",
			},
		},
		Workflows: []store.CatalogWorkflow{
			{ID: "workflow.fixture", DisplayName: "Fixture Workflow"},
		},
		InterfaceNodes: []store.CatalogInterfaceNode{
			{ID: "node.fixture.entry", ServiceID: "entry-service", Status: "active"},
		},
		WorkflowBindings: []store.CatalogWorkflowBinding{
			{WorkflowID: "workflow.fixture", StepID: "entry", NodeID: "node.fixture.entry", Required: true, SortOrder: 1},
		},
	}); err != nil {
		t.Fatalf("replace catalog: %v", err)
	}
	if err := s.Close(); err != nil {
		t.Fatalf("close store: %v", err)
	}

	return sandboxStartFixture{
		storePath:           storePath,
		startedPath:         startedPath,
		platformStartedPath: platformStartedPath,
	}
}

func writeSandboxComposeServiceFixture(t *testing.T, startupCommand string, dockerService string) string {
	t.Helper()
	return writeSandboxComposeServiceFixtureWithHealth(t, startupCommand, dockerService, "")
}

func writeSandboxComposeHealthServiceFixture(t *testing.T, startupCommand string, dockerService string, healthURL string) string {
	t.Helper()
	return writeSandboxComposeServiceFixtureWithHealth(t, startupCommand, dockerService, healthURL)
}

func writeSandboxComposeServiceFixtureWithHealth(t *testing.T, startupCommand string, dockerService string, healthURL string) string {
	t.Helper()
	storePath := filepath.Join(t.TempDir(), "store.sqlite")
	ctx := context.Background()
	s, err := sqlite.Open(ctx, sqlite.Config{Path: storePath})
	if err != nil {
		t.Fatalf("open sandbox compose service store: %v", err)
	}
	defer s.Close()
	if err := s.ReplaceProfileCatalog(ctx, store.ProfileCatalog{
		ProfileID: "sandbox-compose",
		IndexedAt: time.Now().UTC(),
		Services: []store.CatalogService{{
			ID:             "worker-service",
			DisplayName:    "Worker Service",
			Kind:           "app",
			ContainerName:  "sandbox-worker-service",
			DockerService:  dockerService,
			HealthURL:      healthURL,
			StartupCommand: startupCommand,
			Status:         "active",
		}},
	}); err != nil {
		t.Fatalf("replace sandbox compose service catalog: %v", err)
	}
	return storePath
}

func installSandboxDockerTool(t *testing.T, callsPath string, script string) {
	t.Helper()
	dockerPath := filepath.Join(filepath.Dir(callsPath), "docker")
	writeFile(t, dockerPath, script)
	if err := os.Chmod(dockerPath, 0o755); err != nil {
		t.Fatalf("chmod fake sandbox docker: %v", err)
	}
}

func runSandboxStartJSON(t *testing.T, storeRef string, label string, args ...string) sandboxStartCommandReport {
	t.Helper()

	cliArgs := append([]string{"sandbox", "start", "--json"}, args...)
	if storeRef != "" {
		cliArgs = append([]string{"sandbox", "start", "--store", storeRef, "--json"}, args...)
	}
	out := runCLI(t, cliArgs...)
	var report sandboxStartCommandReport
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatalf("decode %s sandbox start report: %v\n%s", label, err, out)
	}
	return report
}

func requireSandboxStartServices(t *testing.T, report sandboxStartCommandReport) {
	t.Helper()

	if !report.OK || len(report.Services) != 3 {
		t.Fatalf("sandbox start report = %#v", report)
	}
	byID := map[string]int{}
	skippedByID := map[string]bool{}
	for _, service := range report.Services {
		byID[service.ID] = service.ExitCode
		skippedByID[service.ID] = service.Skipped
	}
	if byID["entry-service"] != 0 || skippedByID["entry-service"] {
		t.Fatalf("entry-service result exit=%d skipped=%t", byID["entry-service"], skippedByID["entry-service"])
	}
	if byID["platform-service"] != 0 || skippedByID["platform-service"] {
		t.Fatalf("platform-service result exit=%d skipped=%t", byID["platform-service"], skippedByID["platform-service"])
	}
	if !skippedByID["documented-service"] {
		t.Fatalf("documented-service should be skipped without a startup command")
	}
}

func requireSandboxStartupSideEffects(t *testing.T, fixture sandboxStartFixture) {
	t.Helper()

	started, err := os.ReadFile(fixture.startedPath)
	if err != nil {
		t.Fatalf("read startup side effect: %v", err)
	}
	if string(started) != "entry-service" {
		t.Fatalf("startup command wrote %q", started)
	}
	platformStarted, err := os.ReadFile(fixture.platformStartedPath)
	if err != nil {
		t.Fatalf("read platform startup side effect: %v", err)
	}
	if string(platformStarted) != "platform-service" {
		t.Fatalf("platform startup command wrote %q", platformStarted)
	}
}

func requireSandboxNoStartupSideEffects(t *testing.T, fixture sandboxStartFixture) {
	t.Helper()
	for _, path := range []string{fixture.startedPath, fixture.platformStartedPath} {
		if _, err := os.Stat(path); err == nil {
			t.Fatalf("startup side effect should not exist: %s", path)
		} else if !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("stat startup side effect %s: %v", path, err)
		}
	}
}

func TestSandboxStartUsesNamedPostgreSQLActiveStore(t *testing.T) {
	storeRef := configureNamedPostgreSQLActiveStore(t, "daily-sandbox-start-pg")
	runSandboxStartUsesNamedActiveStore(t, storeRef, "pg", "PostgreSQL")
}

func TestSandboxStartUsesNamedMySQLActiveStore(t *testing.T) {
	storeRef := configureNamedMySQLActiveStore(t, "daily-sandbox-start-mysql")
	runSandboxStartUsesNamedActiveStore(t, storeRef, "mysql", "MySQL")
}

func runSandboxStartUsesNamedActiveStore(t *testing.T, storeRef string, suffixLabel string, label string) {
	t.Helper()
	startedPath, serviceID := seedNamedSandboxStartCatalog(t, storeRef, suffixLabel, label)
	report := runSandboxStartJSON(t, "", label, "--service", serviceID)
	requireNamedSandboxStartReport(t, label, report, serviceID)
	requireNamedSandboxStartupSideEffect(t, label, startedPath, serviceID)
}

func seedNamedSandboxStartCatalog(t *testing.T, storeRef string, suffixLabel string, label string) (string, string) {
	t.Helper()

	dir := t.TempDir()
	startedPath := filepath.Join(dir, "started-"+suffixLabel+".txt")
	suffix := time.Now().UTC().Format("20060102150405.000000000")
	serviceID := "entry-service-" + suffixLabel + "-" + suffix

	ctx := context.Background()
	s, err := openStore(ctx, storeRef)
	if err != nil {
		t.Fatalf("open %s active SQL Store: %v", label, err)
	}
	if err := s.ReplaceProfileCatalog(ctx, store.ProfileCatalog{
		ProfileID: "sandbox-" + suffixLabel + "-" + suffix,
		IndexedAt: time.Now().UTC(),
		Services: []store.CatalogService{
			{
				ID:             serviceID,
				DisplayName:    "Entry Service " + label,
				Kind:           "app",
				StartupCommand: fmt.Sprintf("printf %s > %q", serviceID, startedPath),
				Status:         "active",
			},
		},
	}); err != nil {
		_ = s.Close()
		t.Fatalf("replace %s catalog: %v", label, err)
	}
	if err := s.Close(); err != nil {
		t.Fatalf("close %s SQL Store: %v", label, err)
	}
	return startedPath, serviceID
}

func requireNamedSandboxStartReport(t *testing.T, label string, report sandboxStartCommandReport, serviceID string) {
	t.Helper()

	if !report.OK || len(report.Services) != 1 || report.Services[0].ID != serviceID || report.Services[0].ExitCode != 0 || report.Services[0].Skipped {
		t.Fatalf("%s sandbox start report = %#v", label, report)
	}
}

func requireNamedSandboxStartupSideEffect(t *testing.T, label string, startedPath string, serviceID string) {
	t.Helper()

	started, err := os.ReadFile(startedPath)
	if err != nil {
		t.Fatalf("read %s startup side effect: %v", label, err)
	}
	if string(started) != serviceID {
		t.Fatalf("%s startup command wrote %q want %q", label, started, serviceID)
	}
}
