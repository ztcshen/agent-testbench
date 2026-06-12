package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestEnvironmentStatusReportsComposeStateWithoutHeavyRestore(t *testing.T) {
	fixture := newEnvironmentRestoreDockerCLIFixture(t)
	composeSource := filepath.Join(t.TempDir(), "compose.yml")
	writeFile(t, composeSource, "services:\n  web:\n    image: alpine:3.20\n    container_name: env-status-web\n  worker:\n    image: alpine:3.20\n")
	runCLI(t, "environment", "register",
		"--store", fixture.StoreDSN,
		"--id", "env.status",
		"--compose-file", "compose.yml",
		"--compose-generated-file", "compose.yml="+composeSource,
		"--compose-service", "web",
		"--compose-service", "worker",
		"--compose-skip-pull",
		"--compose-skip-build",
		"--health-compose-service", "web",
		"--verification-workflow", "workflow.core-10",
	)

	out := runCLIWithEnv(t, fixture.DockerEnv, "environment", "status", "--store", fixture.StoreDSN, "--workspace", fixture.Workspace, "--json", "env.status")
	var report struct {
		OK          bool `json:"ok"`
		Environment struct {
			ID      string         `json:"id"`
			Summary map[string]any `json:"summary"`
		} `json:"environment"`
		Docker struct {
			Action  string `json:"action"`
			Summary struct {
				Total   int `json:"total"`
				Running int `json:"running"`
				Healthy int `json:"healthy"`
				Ready   int `json:"ready"`
				Failed  int `json:"failed"`
			} `json:"summary"`
			Services []struct {
				Service   string `json:"service"`
				Container string `json:"container"`
				State     string `json:"state"`
				Health    string `json:"health"`
				OK        bool   `json:"ok"`
			} `json:"services"`
		} `json:"docker"`
		VerificationWorkflow string `json:"verificationWorkflow"`
	}
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatalf("decode environment status report: %v\n%s", err, out)
	}
	if !report.OK || report.Environment.ID != "env.status" || report.VerificationWorkflow != "workflow.core-10" {
		t.Fatalf("environment status report = %#v", report)
	}
	if report.Docker.Action != "inspect-compose-services" ||
		len(report.Docker.Services) != 2 ||
		report.Docker.Services[0].Service != "web" ||
		report.Docker.Services[1].Service != "worker" ||
		report.Docker.Services[0].State != "running" ||
		report.Docker.Services[0].Health != "healthy" ||
		!report.Docker.Services[0].OK ||
		!report.Docker.Services[1].OK {
		t.Fatalf("environment status docker state = %#v", report.Docker)
	}
	if report.Docker.Summary.Total != 2 || report.Docker.Summary.Running != 2 || report.Docker.Summary.Healthy != 2 || report.Docker.Summary.Ready != 2 || report.Docker.Summary.Failed != 0 {
		t.Fatalf("environment status health summary = %#v", report.Docker.Summary)
	}
	if _, ok := report.Environment.Summary["lastRestore"]; ok {
		t.Fatalf("environment status should not create lastRestore: %#v", report.Environment.Summary)
	}
	dockerCalls, err := os.ReadFile(fixture.DockerCallsPath)
	if err != nil {
		t.Fatalf("read fake docker calls: %v", err)
	}
	calls := string(dockerCalls)
	if strings.Contains(calls, " up -d") || strings.Contains(calls, " pull") || strings.Contains(calls, " build") || strings.Contains(calls, " down") || strings.Contains(calls, " stop") {
		t.Fatalf("environment status should not run heavy compose commands:\n%s", calls)
	}
	wantBatch := "compose -f " + filepath.Join(fixture.Workspace, "compose.yml") + " ps -a --format json web worker"
	if !strings.Contains(calls, wantBatch) || strings.Count(calls, " ps -a --format json ") != 1 {
		t.Fatalf("environment status should inspect compose services in one batch, want %q:\n%s", wantBatch, calls)
	}
}

func TestEnvironmentStatusExposesLastRestoreSummary(t *testing.T) {
	fixture := newEnvironmentRestoreDockerCLIFixture(t)
	composeSource := filepath.Join(t.TempDir(), "compose.yml")
	writeFile(t, composeSource, "services:\n  web:\n    image: alpine:3.20\n")
	runCLI(t, "environment", "register",
		"--store", fixture.StoreDSN,
		"--id", "env.status.restore-summary",
		"--compose-file", "compose.yml",
		"--compose-generated-file", "compose.yml="+composeSource,
		"--compose-service", "web",
		"--verification-workflow", "workflow.core-10",
	)
	runtime, err := openStore(context.Background(), fixture.StoreDSN)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer runtime.Close()
	env, err := runtime.GetEnvironment(context.Background(), "env.status.restore-summary")
	if err != nil {
		t.Fatalf("get environment: %v", err)
	}
	env.SummaryJSON = `{"lastRestore":{"id":"restore.latest","ok":true,"docker":{"action":"run-docker-compose"}}}`
	if _, err := runtime.UpsertEnvironment(context.Background(), env); err != nil {
		t.Fatalf("upsert environment summary: %v", err)
	}

	out := runCLIWithEnv(t, fixture.DockerEnv, "environment", "status", "--store", fixture.StoreDSN, "--workspace", fixture.Workspace, "--json", "env.status.restore-summary")
	var report struct {
		OK          bool `json:"ok"`
		Environment struct {
			Summary struct {
				LastRestore struct {
					ID     string `json:"id"`
					OK     bool   `json:"ok"`
					Docker struct {
						Action string `json:"action"`
					} `json:"docker"`
				} `json:"lastRestore"`
			} `json:"summary"`
		} `json:"environment"`
	}
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatalf("decode environment status report: %v\n%s", err, out)
	}
	if !report.OK || report.Environment.Summary.LastRestore.ID != "restore.latest" || !report.Environment.Summary.LastRestore.OK || report.Environment.Summary.LastRestore.Docker.Action != "run-docker-compose" {
		t.Fatalf("environment status should expose lastRestore summary: %#v", report.Environment.Summary.LastRestore)
	}
}

func TestEnvironmentStatusPreservesOneShotComposeExpectations(t *testing.T) {
	fixture := newEnvironmentRestoreDockerCLIFixture(t)
	fixture.writeDockerTool(t, `#!/usr/bin/env bash
printf '%s\n' "$*" >> "$DOCKER_CALLS_FILE"
if [ "$1" = compose ] && [ "$2" = version ]; then
  printf 'Docker Compose version v2.0.0\n'
  exit 0
fi
if [ "$1" = compose ] && [[ "$*" == *" ps -a --format json"* ]]; then
  printf '{"Name":"demo-seed","Service":"seed","State":"exited","ExitCode":0}\n'
  exit 0
fi
exit 0
`)
	composeSource := filepath.Join(t.TempDir(), "compose.yml")
	writeFile(t, composeSource, "services:\n  seed:\n    image: alpine:3.20\n")
	runCLI(t, "environment", "register",
		"--store", fixture.StoreDSN,
		"--id", "env.status.oneshot",
		"--compose-file", "compose.yml",
		"--compose-generated-file", "compose.yml="+composeSource,
		"--compose-service", "seed",
		"--verification-workflow", "workflow.core-10",
	)
	runtime, err := openStore(context.Background(), fixture.StoreDSN)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer runtime.Close()
	env, err := runtime.GetEnvironment(context.Background(), "env.status.oneshot")
	if err != nil {
		t.Fatalf("get environment: %v", err)
	}
	env.HealthChecksJSON = `[{"kind":"compose-service","service":"seed","expect":"service_completed_successfully"}]`
	if _, err := runtime.UpsertEnvironment(context.Background(), env); err != nil {
		t.Fatalf("upsert one-shot health check: %v", err)
	}

	out := runCLIWithEnv(t, fixture.DockerEnv, "environment", "status", "--store", fixture.StoreDSN, "--workspace", fixture.Workspace, "--json", "env.status.oneshot")
	var report struct {
		OK     bool `json:"ok"`
		Docker struct {
			Summary struct {
				Ready  int `json:"ready"`
				Failed int `json:"failed"`
			} `json:"summary"`
			Services []struct {
				Service  string `json:"service"`
				State    string `json:"state"`
				ExitCode int    `json:"exitCode"`
				OK       bool   `json:"ok"`
			} `json:"services"`
		} `json:"docker"`
	}
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatalf("decode one-shot status report: %v\n%s", err, out)
	}
	if !report.OK || report.Docker.Summary.Ready != 1 || report.Docker.Summary.Failed != 0 || len(report.Docker.Services) != 1 || !report.Docker.Services[0].OK || report.Docker.Services[0].State != "exited" || report.Docker.Services[0].ExitCode != 0 {
		t.Fatalf("one-shot status report = %#v", report)
	}
}

func TestEnvironmentStatusRequiresObservedExitCodeForOneShot(t *testing.T) {
	fixture := newEnvironmentRestoreDockerCLIFixture(t)
	fixture.writeDockerTool(t, `#!/usr/bin/env bash
printf '%s\n' "$*" >> "$DOCKER_CALLS_FILE"
if [ "$1" = compose ] && [ "$2" = version ]; then
  printf 'Docker Compose version v2.0.0\n'
  exit 0
fi
if [ "$1" = compose ] && [[ "$*" == *" ps -a --format json"* ]]; then
  printf '{"Name":"demo-seed","Service":"seed","State":"exited"}\n'
  exit 0
fi
exit 0
`)
	composeSource := filepath.Join(t.TempDir(), "compose.yml")
	writeFile(t, composeSource, "services:\n  seed:\n    image: alpine:3.20\n")
	runCLI(t, "environment", "register",
		"--store", fixture.StoreDSN,
		"--id", "env.status.oneshot.no-exit",
		"--compose-file", "compose.yml",
		"--compose-generated-file", "compose.yml="+composeSource,
		"--compose-service", "seed",
		"--verification-workflow", "workflow.core-10",
	)
	runtime, err := openStore(context.Background(), fixture.StoreDSN)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer runtime.Close()
	env, err := runtime.GetEnvironment(context.Background(), "env.status.oneshot.no-exit")
	if err != nil {
		t.Fatalf("get environment: %v", err)
	}
	env.HealthChecksJSON = `[{"kind":"compose-service","service":"seed","expect":"service_completed_successfully"}]`
	if _, err := runtime.UpsertEnvironment(context.Background(), env); err != nil {
		t.Fatalf("upsert one-shot health check: %v", err)
	}

	out := runCLIFailsWithEnv(t, fixture.DockerEnv, "environment", "status", "--store", fixture.StoreDSN, "--workspace", fixture.Workspace, "--json", "env.status.oneshot.no-exit")
	var report struct {
		OK     bool `json:"ok"`
		Docker struct {
			Summary struct {
				Ready  int `json:"ready"`
				Failed int `json:"failed"`
			} `json:"summary"`
			Services []struct {
				Service string `json:"service"`
				State   string `json:"state"`
				OK      bool   `json:"ok"`
				Error   string `json:"error"`
			} `json:"services"`
		} `json:"docker"`
	}
	if err := json.Unmarshal([]byte(extractJSONObject(t, out)), &report); err != nil {
		t.Fatalf("decode missing-exit one-shot status report: %v\n%s", err, out)
	}
	if report.OK || report.Docker.Summary.Ready != 0 || report.Docker.Summary.Failed != 1 || len(report.Docker.Services) != 1 || report.Docker.Services[0].OK || !strings.Contains(report.Docker.Services[0].Error, "not ready") {
		t.Fatalf("missing-exit one-shot status report = %#v", report)
	}
}

func TestEnvironmentStatusFailsWhenNoComposeServicesCanBeInspected(t *testing.T) {
	fixture := newEnvironmentRestoreDockerCLIFixture(t)
	composeSource := filepath.Join(t.TempDir(), "compose.yml")
	writeFile(t, composeSource, "name: empty-status\n")
	runCLI(t, "environment", "register",
		"--store", fixture.StoreDSN,
		"--id", "env.status.empty",
		"--compose-file", "compose.yml",
		"--compose-generated-file", "compose.yml="+composeSource,
		"--verification-workflow", "workflow.core-10",
	)

	out := runCLIFailsWithEnv(t, fixture.DockerEnv, "environment", "status", "--store", fixture.StoreDSN, "--workspace", fixture.Workspace, "--json", "env.status.empty")
	var report struct {
		OK     bool `json:"ok"`
		Docker struct {
			OK       bool `json:"ok"`
			Services []struct {
				Service string `json:"service"`
			} `json:"services"`
			Error string `json:"error"`
		} `json:"docker"`
	}
	if err := json.Unmarshal([]byte(extractJSONObject(t, out)), &report); err != nil {
		t.Fatalf("decode empty service status report: %v\n%s", err, out)
	}
	if report.OK || report.Docker.OK || len(report.Docker.Services) != 0 || !strings.Contains(report.Docker.Error, "no compose services") {
		t.Fatalf("empty service status report = %#v", report)
	}
}

func TestEnvironmentStatusPreservesComposePSErrorWithoutServiceHints(t *testing.T) {
	fixture := newEnvironmentRestoreDockerCLIFixture(t)
	fixture.writeDockerTool(t, `#!/usr/bin/env bash
printf '%s\n' "$*" >> "$DOCKER_CALLS_FILE"
if [ "$1" = compose ] && [ "$2" = version ]; then
  printf 'Docker Compose version v2.0.0\n'
  exit 0
fi
if [ "$1" = compose ] && [[ "$*" == *" ps -a --format json"* ]]; then
  printf 'missing required env file\n' >&2
  exit 1
fi
exit 0
`)
	composeSource := filepath.Join(t.TempDir(), "compose.yml")
	writeFile(t, composeSource, "name: ps-error\n")
	runCLI(t, "environment", "register",
		"--store", fixture.StoreDSN,
		"--id", "env.status.ps-error",
		"--compose-file", "compose.yml",
		"--compose-generated-file", "compose.yml="+composeSource,
		"--verification-workflow", "workflow.core-10",
	)

	out := runCLIFailsWithEnv(t, fixture.DockerEnv, "environment", "status", "--store", fixture.StoreDSN, "--workspace", fixture.Workspace, "--json", "env.status.ps-error")
	var report struct {
		OK     bool `json:"ok"`
		Docker struct {
			OK       bool `json:"ok"`
			Services []struct {
				Service string `json:"service"`
				OK      bool   `json:"ok"`
				Error   string `json:"error"`
			} `json:"services"`
			Error string `json:"error"`
		} `json:"docker"`
	}
	if err := json.Unmarshal([]byte(extractJSONObject(t, out)), &report); err != nil {
		t.Fatalf("decode compose ps error status report: %v\n%s", err, out)
	}
	if report.OK || report.Docker.OK || len(report.Docker.Services) != 1 || report.Docker.Services[0].Service != "docker compose ps" || report.Docker.Services[0].OK || !strings.Contains(report.Docker.Services[0].Error, "missing required env file") || !strings.Contains(report.Docker.Error, "missing required env file") {
		t.Fatalf("compose ps error status report = %#v", report)
	}
}

func TestEnvironmentStopDefaultsToComposeStopAndPersistsLastStop(t *testing.T) {
	fixture := newEnvironmentRestoreDockerCLIFixture(t)
	composeSource := filepath.Join(t.TempDir(), "compose.yml")
	writeFile(t, composeSource, "services:\n  web:\n    image: alpine:3.20\n")
	runCLI(t, "environment", "register",
		"--store", fixture.StoreDSN,
		"--id", "env.stop",
		"--compose-file", "compose.yml",
		"--compose-generated-file", "compose.yml="+composeSource,
		"--compose-service", "web",
		"--compose-skip-pull",
		"--compose-skip-build",
		"--verification-workflow", "workflow.core-10",
	)

	out := runCLIWithEnv(t, fixture.DockerEnv, "environment", "stop", "--store", fixture.StoreDSN, "--workspace", fixture.Workspace, "--json", "env.stop")
	var report struct {
		OK          bool `json:"ok"`
		Environment struct {
			Summary struct {
				LastStop struct {
					OK      bool     `json:"ok"`
					Action  string   `json:"action"`
					Command []string `json:"command"`
				} `json:"lastStop"`
			} `json:"summary"`
		} `json:"environment"`
		Docker struct {
			Action  string   `json:"action"`
			Command []string `json:"command"`
		} `json:"docker"`
	}
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatalf("decode environment stop report: %v\n%s", err, out)
	}
	if !report.OK || report.Docker.Action != "compose-stop" || !reflect.DeepEqual(report.Docker.Command, []string{"docker", "compose", "-f", filepath.Join(fixture.Workspace, "compose.yml"), "stop", "web"}) {
		t.Fatalf("environment stop report = %#v", report)
	}
	if !report.Environment.Summary.LastStop.OK || report.Environment.Summary.LastStop.Action != "compose-stop" {
		t.Fatalf("environment lastStop summary = %#v", report.Environment.Summary.LastStop)
	}
	dockerCalls, err := os.ReadFile(fixture.DockerCallsPath)
	if err != nil {
		t.Fatalf("read fake docker calls: %v", err)
	}
	calls := string(dockerCalls)
	if !strings.Contains(calls, "compose -f "+filepath.Join(fixture.Workspace, "compose.yml")+" stop web") {
		t.Fatalf("environment stop should run compose stop:\n%s", calls)
	}
	if strings.Contains(calls, " down") || strings.Contains(calls, "--rmi") || strings.Contains(calls, " -v") || strings.Contains(calls, "rm ") {
		t.Fatalf("environment stop default must not remove containers, images, or volumes:\n%s", calls)
	}
}

func TestEnvironmentStopDownRemoveOrphansRequiresExplicitFlags(t *testing.T) {
	fixture := newEnvironmentRestoreDockerCLIFixture(t)
	composeSource := filepath.Join(t.TempDir(), "compose.yml")
	writeFile(t, composeSource, "services:\n  web:\n    image: alpine:3.20\n")
	runCLI(t, "environment", "register",
		"--store", fixture.StoreDSN,
		"--id", "env.stop.down",
		"--compose-file", "compose.yml",
		"--compose-generated-file", "compose.yml="+composeSource,
		"--compose-service", "web",
		"--verification-workflow", "workflow.core-10",
	)

	out := runCLIFailsWithEnv(t, fixture.DockerEnv, "environment", "stop", "--store", fixture.StoreDSN, "--workspace", fixture.Workspace, "--remove-orphans", "--json", "env.stop.down")
	if !strings.Contains(out, "--remove-orphans requires --down") {
		t.Fatalf("remove-orphans without down should fail clearly: %q", out)
	}

	out = runCLIWithEnv(t, fixture.DockerEnv, "environment", "stop", "--store", fixture.StoreDSN, "--workspace", fixture.Workspace, "--down", "--remove-orphans", "--json", "env.stop.down")
	var report struct {
		OK     bool `json:"ok"`
		Docker struct {
			Action  string   `json:"action"`
			Command []string `json:"command"`
		} `json:"docker"`
	}
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatalf("decode environment stop down report: %v\n%s", err, out)
	}
	want := []string{"docker", "compose", "-f", filepath.Join(fixture.Workspace, "compose.yml"), "down", "--remove-orphans"}
	if !report.OK || report.Docker.Action != "compose-down" || !reflect.DeepEqual(report.Docker.Command, want) {
		t.Fatalf("environment stop down report = %#v want command %#v", report, want)
	}
}

func TestEnvironmentStopDefaultRequiresInspectableComposeServices(t *testing.T) {
	fixture := newEnvironmentRestoreDockerCLIFixture(t)
	composeSource := filepath.Join(t.TempDir(), "compose.yml")
	writeFile(t, composeSource, "name: empty-stop\n")
	runCLI(t, "environment", "register",
		"--store", fixture.StoreDSN,
		"--id", "env.stop.empty",
		"--compose-file", "compose.yml",
		"--compose-generated-file", "compose.yml="+composeSource,
		"--verification-workflow", "workflow.core-10",
	)

	out := runCLIFailsWithEnv(t, fixture.DockerEnv, "environment", "stop", "--store", fixture.StoreDSN, "--workspace", fixture.Workspace, "--json", "env.stop.empty")
	if !strings.Contains(out, "found no compose services") {
		t.Fatalf("empty default stop should fail clearly: %q", out)
	}
	dockerCalls, err := os.ReadFile(fixture.DockerCallsPath)
	if err != nil && !os.IsNotExist(err) {
		t.Fatalf("read fake docker calls: %v", err)
	}
	if strings.Contains(string(dockerCalls), " stop") {
		t.Fatalf("empty default stop must not run broad docker compose stop:\n%s", dockerCalls)
	}

	out = runCLIWithEnv(t, fixture.DockerEnv, "environment", "stop", "--store", fixture.StoreDSN, "--workspace", fixture.Workspace, "--down", "--json", "env.stop.empty")
	if !strings.Contains(out, `"action": "compose-down"`) {
		t.Fatalf("explicit down should remain available: %q", out)
	}
}
