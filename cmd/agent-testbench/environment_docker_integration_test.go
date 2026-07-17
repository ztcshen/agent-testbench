package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const (
	dockerIntegrationOptInEnv       = "AGENT_TESTBENCH_DOCKER_INTEGRATION"
	dockerIntegrationDefaultImage   = "alpine:3.20@sha256:d9e853e87e55526f6b2917df91a2115c36dd7c696a35be12163d44e6e2a4b6bc"
	dockerIntegrationPullAttempts   = 3
	dockerIntegrationPullRetryDelay = time.Second
)

type dockerLifecycleIntegration struct {
	workspace     string
	storeRef      string
	project       string
	secret        string
	fixtureDir    string
	composeSource string
}

type dockerIntegrationRestoreReport struct {
	OK     bool `json:"ok"`
	Docker struct {
		OK           bool       `json:"ok"`
		Action       string     `json:"action"`
		Commands     [][]string `json:"commands"`
		HealthChecks []struct {
			OK bool `json:"ok"`
		} `json:"healthChecks"`
	} `json:"docker"`
}

type dockerIntegrationStatusReport struct {
	OK     bool `json:"ok"`
	Docker struct {
		OK       bool `json:"ok"`
		Services []struct {
			Service string `json:"service"`
			State   string `json:"state"`
			Health  string `json:"health"`
			OK      bool   `json:"ok"`
		} `json:"services"`
	} `json:"docker"`
}

type dockerIntegrationStopReport struct {
	OK     bool `json:"ok"`
	Docker struct {
		OK     bool   `json:"ok"`
		Action string `json:"action"`
	} `json:"docker"`
}

func TestEnvironmentDockerComposeLifecycleIntegration(t *testing.T) {
	image := requireDockerIntegrationPrerequisites(t)
	fixture := newDockerLifecycleIntegration(t, image)
	fixture.register(t)
	composeFile, composeEnvFile, containerID := fixture.restore(t)
	fixture.assertStatus(t)
	networkName, volumeName := fixture.assertResources(t, containerID)
	fixture.stopAndAssertCleanup(t, composeFile, composeEnvFile, networkName, volumeName)
}

func requireDockerIntegrationPrerequisites(t *testing.T) string {
	t.Helper()
	if os.Getenv(dockerIntegrationOptInEnv) != "1" {
		t.Skip("set " + dockerIntegrationOptInEnv + "=1 to run against a real Docker daemon")
	}
	requireDockerIntegrationCommand(t, 20*time.Second, "info", "--format", "{{.ServerVersion}}")
	requireDockerIntegrationCommand(t, 20*time.Second, "compose", "version")
	image := strings.TrimSpace(os.Getenv("AGENT_TESTBENCH_DOCKER_INTEGRATION_IMAGE"))
	if image == "" {
		image = dockerIntegrationDefaultImage
	}
	if strings.ContainsAny(image, "\r\n\"") {
		t.Fatalf("integration image must be a single safe Docker image reference: %q", image)
	}
	requireDockerIntegrationCommandWithRetry(t, dockerIntegrationPullAttempts, time.Minute, dockerIntegrationPullRetryDelay, "pull", image)
	return image
}

func newDockerLifecycleIntegration(t *testing.T, image string) dockerLifecycleIntegration {
	t.Helper()
	fixture := dockerLifecycleIntegration{
		workspace:  t.TempDir(),
		storeRef:   "sqlite://" + filepath.Join(t.TempDir(), "docker-integration.sqlite"),
		project:    fmt.Sprintf("atb-it-%d-%d", os.Getpid(), time.Now().UnixNano()),
		secret:     fmt.Sprintf("sentinel-compose-secret-%d-%d", os.Getpid(), time.Now().UnixNano()),
		fixtureDir: t.TempDir(),
	}
	dockerfileSource := filepath.Join(fixture.fixtureDir, "Dockerfile")
	fixture.composeSource = filepath.Join(fixture.fixtureDir, "compose.yml")
	writeFile(t, dockerfileSource, `ARG BASE_IMAGE
FROM ${BASE_IMAGE}
RUN mkdir -p /usr/local/share && printf 'built-by-agent-testbench\n' > /usr/local/share/atb-build-marker
`)
	writeFile(t, fixture.composeSource, fmt.Sprintf(`services:
  probe:
    build:
      context: .
      dockerfile: Dockerfile
      args:
        BASE_IMAGE: "%s"
    environment:
      ATB_API_TOKEN: "${ATB_API_TOKEN:?ATB_API_TOKEN is required}"
    command:
      - sh
      - -ec
      - |
        test -n "$$ATB_API_TOKEN"
        test -f /usr/local/share/atb-build-marker
        printf 'ready\n' > /data/ready
        trap 'exit 0' TERM INT
        while :; do sleep 1; done
    healthcheck:
      test: ["CMD-SHELL", "test -f /usr/local/share/atb-build-marker && test -s /data/ready && test -n \"$$ATB_API_TOKEN\""]
      interval: 1s
      timeout: 1s
      retries: 30
    volumes:
      - probe-data:/data
    networks:
      - lifecycle

volumes:
  probe-data:

networks:
  lifecycle:
`, image))
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()
		cmd := exec.CommandContext(ctx, "docker", "compose", "-f", fixture.composeSource, "-p", fixture.project, "down", "--volumes", "--remove-orphans", "--rmi", "local")
		cmd.Env = append(os.Environ(), "ATB_API_TOKEN="+fixture.secret)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Logf("Docker integration cleanup failed: %v\n%s", err, out)
		}
	})
	return fixture
}

func (fixture dockerLifecycleIntegration) register(t *testing.T) {
	t.Helper()
	dockerfileSource := filepath.Join(fixture.fixtureDir, "Dockerfile")
	runCLI(t, "environment", "register",
		"--store", fixture.storeRef,
		"--id", "env.docker.integration",
		"--service", "probe",
		"--compose-file", "compose.yml",
		"--compose-generated-file", "compose.yml="+fixture.composeSource,
		"--compose-generated-file", "Dockerfile="+dockerfileSource,
		"--compose-project-name", fixture.project,
		"--compose-service", "probe",
		"--compose-env", "ATB_API_TOKEN="+fixture.secret,
		"--compose-skip-pull",
		"--health-compose-service", "probe",
		"--verification-workflow", "workflow.docker-integration",
	)
	componentGraphSource := filepath.Join(fixture.fixtureDir, "component-graph.json")
	writeFile(t, componentGraphSource, `{
  "components": [
    {
      "componentId": "probe",
      "kind": "middleware",
      "role": "probe",
      "composeService": "probe",
      "required": true,
      "healthCheckJson": "{\"kind\":\"compose-service\",\"service\":\"probe\"}"
    }
  ],
  "dependencies": [],
  "assets": []
}
`)
	runCLI(t, "environment", "components", "replace",
		"--store", fixture.storeRef,
		"--file", componentGraphSource,
		"env.docker.integration",
	)
}

func (fixture dockerLifecycleIntegration) restore(t *testing.T) (string, string, string) {
	t.Helper()
	restoreOut := runCLI(t, "environment", "restore",
		"--store", fixture.storeRef,
		"--workspace", fixture.workspace,
		"--execute",
		"--health-timeout-seconds", "45",
		"--json",
		"env.docker.integration",
	)
	assertEnvironmentOutputHasNoSecrets(t, restoreOut, fixture.secret)
	var restore dockerIntegrationRestoreReport
	decodeDockerIntegrationJSON(t, restoreOut, &restore)
	if !restore.OK || !restore.Docker.OK || restore.Docker.Action != "run-docker-compose" || len(restore.Docker.HealthChecks) != 1 || !restore.Docker.HealthChecks[0].OK {
		t.Fatalf("real Docker restore report = %#v\n%s", restore, restoreOut)
	}
	if !dockerIntegrationCommandsContainSubcommand(restore.Docker.Commands, "build", "probe") {
		t.Fatalf("real Docker restore did not run an explicit Compose build: %#v", restore.Docker.Commands)
	}

	composeFile := filepath.Join(fixture.workspace, "compose.yml")
	composeEnvFile := environmentRestoreGeneratedEnvFilePath(fixture.workspace)
	imageID := strings.TrimSpace(requireDockerIntegrationCommand(t, 20*time.Second, "compose", "-f", composeFile, "--env-file", composeEnvFile, "-p", fixture.project, "images", "-q", "probe"))
	if imageID == "" {
		t.Fatal("real Docker restore did not leave a built probe image")
	}
	containerID := strings.TrimSpace(requireDockerIntegrationCommand(t, 20*time.Second, "compose", "-f", composeFile, "--env-file", composeEnvFile, "-p", fixture.project, "ps", "-q", "probe"))
	if containerID == "" {
		t.Fatal("real Docker restore did not start the probe container")
	}
	return composeFile, composeEnvFile, containerID
}

func (fixture dockerLifecycleIntegration) assertStatus(t *testing.T) {
	t.Helper()
	statusOut := runCLI(t, "environment", "status", "--store", fixture.storeRef, "--workspace", fixture.workspace, "--json", "env.docker.integration")
	assertEnvironmentOutputHasNoSecrets(t, statusOut, fixture.secret)
	var status dockerIntegrationStatusReport
	decodeDockerIntegrationJSON(t, statusOut, &status)
	if !status.OK || !status.Docker.OK || len(status.Docker.Services) != 1 || status.Docker.Services[0].Service != "probe" || status.Docker.Services[0].State != "running" || status.Docker.Services[0].Health != "healthy" || !status.Docker.Services[0].OK {
		t.Fatalf("real Docker status report = %#v\n%s", status, statusOut)
	}
}

func (fixture dockerLifecycleIntegration) assertResources(t *testing.T, containerID string) (string, string) {
	t.Helper()
	networkName := requireSingleDockerIntegrationResource(t, "network", "ls",
		"--filter", "label=com.docker.compose.project="+fixture.project,
		"--filter", "label=com.docker.compose.network=lifecycle",
		"--format", "{{.Name}}",
	)
	volumeName := requireSingleDockerIntegrationResource(t, "volume", "ls",
		"--filter", "label=com.docker.compose.project="+fixture.project,
		"--filter", "label=com.docker.compose.volume=probe-data",
		"--format", "{{.Name}}",
	)
	assertDockerIntegrationContainerResources(t, containerID, networkName, volumeName)
	return networkName, volumeName
}

func (fixture dockerLifecycleIntegration) stopAndAssertCleanup(t *testing.T, composeFile string, composeEnvFile string, networkName string, volumeName string) {
	t.Helper()
	stopOut := runCLI(t, "environment", "stop", "--store", fixture.storeRef, "--workspace", fixture.workspace, "--down", "--remove-orphans", "--json", "env.docker.integration")
	assertEnvironmentOutputHasNoSecrets(t, stopOut, fixture.secret)
	var stop dockerIntegrationStopReport
	decodeDockerIntegrationJSON(t, stopOut, &stop)
	if !stop.OK || !stop.Docker.OK || stop.Docker.Action != "compose-down" {
		t.Fatalf("real Docker stop report = %#v\n%s", stop, stopOut)
	}
	if got := strings.TrimSpace(requireDockerIntegrationCommand(t, 20*time.Second, "compose", "-f", composeFile, "--env-file", composeEnvFile, "-p", fixture.project, "ps", "-a", "-q", "probe")); got != "" {
		t.Fatalf("environment stop --down left probe container %q", got)
	}
	assertDockerIntegrationResourceMissing(t, "network", networkName)
	requireDockerIntegrationCommand(t, 20*time.Second, "volume", "inspect", volumeName)

	requireDockerIntegrationCommand(t, time.Minute, "compose", "-f", composeFile, "--env-file", composeEnvFile, "-p", fixture.project, "down", "--volumes", "--remove-orphans", "--rmi", "local")
	assertDockerIntegrationResourceMissing(t, "volume", volumeName)
}

func dockerIntegrationCommandsContainSubcommand(commands [][]string, subcommand string, service string) bool {
	for _, command := range commands {
		for index, arg := range command {
			if arg != subcommand {
				continue
			}
			for _, trailing := range command[index+1:] {
				if trailing == service {
					return true
				}
			}
		}
	}
	return false
}

func requireSingleDockerIntegrationResource(t *testing.T, resource string, args ...string) string {
	t.Helper()
	output := strings.Fields(requireDockerIntegrationCommand(t, 20*time.Second, append([]string{resource}, args...)...))
	if len(output) != 1 {
		t.Fatalf("Docker Compose project %s resources = %q, want exactly one", resource, output)
	}
	return output[0]
}

func assertDockerIntegrationContainerResources(t *testing.T, containerID string, networkName string, volumeName string) {
	t.Helper()
	var inspected []struct {
		NetworkSettings struct {
			Networks map[string]json.RawMessage `json:"Networks"`
		} `json:"NetworkSettings"`
		Mounts []struct {
			Type        string `json:"Type"`
			Name        string `json:"Name"`
			Destination string `json:"Destination"`
		} `json:"Mounts"`
	}
	decodeDockerIntegrationJSON(t, requireDockerIntegrationCommand(t, 20*time.Second, "inspect", containerID), &inspected)
	if len(inspected) != 1 {
		t.Fatalf("docker inspect returned %d containers, want 1", len(inspected))
	}
	if _, ok := inspected[0].NetworkSettings.Networks[networkName]; !ok {
		t.Fatalf("probe container is not attached to project network %q: %#v", networkName, inspected[0].NetworkSettings.Networks)
	}
	for _, mount := range inspected[0].Mounts {
		if mount.Type == "volume" && mount.Name == volumeName && mount.Destination == "/data" {
			return
		}
	}
	t.Fatalf("probe container is not using named volume %q at /data: %#v", volumeName, inspected[0].Mounts)
}

func assertDockerIntegrationResourceMissing(t *testing.T, resource string, name string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "docker", resource, "inspect", name).CombinedOutput()
	if err == nil {
		t.Fatalf("Docker %s %q still exists:\n%s", resource, name, out)
	}
	detail := strings.ToLower(string(out))
	if !strings.Contains(detail, "not found") && !strings.Contains(detail, "no such") {
		t.Fatalf("cannot verify Docker %s %q was removed: %v\n%s", resource, name, err, out)
	}
}

func requireDockerIntegrationCommand(t *testing.T, timeout time.Duration, args ...string) string {
	t.Helper()
	output, err := runDockerIntegrationCommand(timeout, args...)
	if err != nil {
		t.Fatalf("docker %s failed: %v\n%s", strings.Join(args, " "), err, output)
	}
	return output
}

func requireDockerIntegrationCommandWithRetry(t *testing.T, attempts int, timeout time.Duration, delay time.Duration, args ...string) string {
	t.Helper()
	if attempts < 1 {
		t.Fatal("Docker integration retry attempts must be positive")
	}
	var output string
	var err error
	for attempt := 1; attempt <= attempts; attempt++ {
		output, err = runDockerIntegrationCommand(timeout, args...)
		if err == nil {
			return output
		}
		if attempt < attempts {
			t.Logf("docker %s attempt %d/%d failed: %v; retrying", strings.Join(args, " "), attempt, attempts, err)
			time.Sleep(delay)
		}
	}
	t.Fatalf("docker %s failed after %d attempts: %v\n%s", strings.Join(args, " "), attempts, err, output)
	return ""
}

func runDockerIntegrationCommand(timeout time.Duration, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	out, err := exec.CommandContext(ctx, "docker", args...).CombinedOutput()
	return string(out), err
}

func decodeDockerIntegrationJSON(t *testing.T, raw string, target any) {
	t.Helper()
	if err := json.Unmarshal([]byte(raw), target); err != nil {
		t.Fatalf("decode Docker integration JSON: %v\n%s", err, raw)
	}
}
