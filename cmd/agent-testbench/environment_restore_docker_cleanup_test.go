package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func seedCleanupLinkedGraph(t *testing.T, storePath string, envID string, service string) {
	t.Helper()
	graphPath := filepath.Join(t.TempDir(), "component-graph.json")
	graph := strings.ReplaceAll(`{
		"components": [
			{"componentId":"SERVICE","kind":"app","role":"business-service","composeService":"SERVICE","required":true,"healthCheckJson":"{\"kind\":\"url\",\"url\":\"HEALTH_URL\"}"}
		]
	}`, "SERVICE", service)
	graph = strings.ReplaceAll(graph, "HEALTH_URL", newHealthyTestURL(t))
	writeFile(t, graphPath, graph)
	runCLI(t, "environment", "components", "replace",
		"--store", "sqlite://"+storePath,
		"--file", graphPath,
		envID,
	)
}

func TestEnvironmentRestorePlansDockerCleanupWithoutExecuting(t *testing.T) {
	storePath := filepath.Join(t.TempDir(), "store.sqlite")
	workspace := filepath.Join(t.TempDir(), "workspace")
	fakeDockerEnv, _ := fakeDockerCommand(t)
	composeSource := filepath.Join(t.TempDir(), "compose.yml")
	writeFile(t, composeSource, "services:\n  web:\n    image: alpine:3.20\n")
	runCLI(t, "environment", "register",
		"--store", "sqlite://"+storePath,
		"--id", "env.cleanup.plan",
		"--compose-file", "compose.yml",
		"--compose-generated-file", "compose.yml="+composeSource,
		"--compose-project-name", "demo",
		"--compose-service", "web",
		"--compose-env", "APP_MODE=test",
		"--verification-workflow", "workflow.core-10",
	)
	seedCleanupLinkedGraph(t, storePath, "env.cleanup.plan", "web")

	out := runCLIWithEnv(t, fakeDockerEnv, "environment", "restore", "--store", "sqlite://"+storePath, "--workspace", workspace, "--clean-docker-state", "--clean-docker-images", "--json", "env.cleanup.plan")
	var report struct {
		OK     bool `json:"ok"`
		Docker struct {
			Cleanup struct {
				Requested      bool       `json:"requested"`
				Allowed        bool       `json:"allowed"`
				IncludeImages  bool       `json:"includeImages"`
				Action         string     `json:"action"`
				BackupCommands [][]string `json:"backupCommands"`
				Commands       [][]string `json:"commands"`
				Warning        string     `json:"warning"`
				Linkage        struct {
					OK           bool `json:"ok"`
					EnvInjection struct {
						GeneratedEnvFile string   `json:"generatedEnvFile"`
						StoreEnvKeys     []string `json:"storeEnvKeys"`
					} `json:"envInjection"`
				} `json:"linkage"`
			} `json:"cleanup"`
		} `json:"docker"`
	}
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatalf("decode cleanup dry-run json: %v\n%s", err, out)
	}
	cleanup := report.Docker.Cleanup
	if !report.OK || !cleanup.Requested || cleanup.Allowed || !cleanup.IncludeImages || cleanup.Action != "plan-cleanup" || !cleanup.Linkage.OK || len(cleanup.BackupCommands) != 3 || len(cleanup.Commands) != 1 {
		t.Fatalf("cleanup dry-run report = %#v", report.Docker.Cleanup)
	}
	command := strings.Join(cleanup.Commands[0], " ")
	if !strings.Contains(command, "compose -f "+filepath.Join(workspace, "compose.yml")) ||
		!strings.Contains(command, "-p demo down --remove-orphans --rmi all") {
		t.Fatalf("cleanup command = %#v", cleanup.Commands[0])
	}
	if !strings.Contains(command, "--env-file "+filepath.Join(workspace, ".agent-testbench", "restore.env")) {
		t.Fatalf("cleanup command should use generated compose env file: %#v", cleanup.Commands[0])
	}
	if cleanup.Linkage.EnvInjection.GeneratedEnvFile != filepath.Join(workspace, ".agent-testbench", "restore.env") || !stringSliceContains(cleanup.Linkage.EnvInjection.StoreEnvKeys, "APP_MODE") {
		t.Fatalf("cleanup linkage env injection = %#v", cleanup.Linkage.EnvInjection)
	}
	allCommands := strings.Join(append(cleanup.BackupCommands[0], cleanup.Commands[0]...), " ")
	if strings.Contains(allCommands, "--volumes") || strings.Contains(allCommands, "system prune") {
		t.Fatalf("cleanup should stay scoped to compose project: %q", allCommands)
	}
	if !strings.Contains(cleanup.Warning, "SQL Store") {
		t.Fatalf("cleanup warning should mention Store boundary: %q", cleanup.Warning)
	}
}

func TestEnvironmentRestoreBlocksDockerCleanupWithoutExplicitAllow(t *testing.T) {
	storePath := filepath.Join(t.TempDir(), "store.sqlite")
	workspace := filepath.Join(t.TempDir(), "workspace")
	fakeDockerEnv, dockerCallsPath := fakeDockerCommand(t)
	composeSource := filepath.Join(t.TempDir(), "compose.yml")
	writeFile(t, composeSource, "services: {}\n")
	runCLI(t, "environment", "register",
		"--store", "sqlite://"+storePath,
		"--id", "env.cleanup.block",
		"--compose-file", "compose.yml",
		"--compose-generated-file", "compose.yml="+composeSource,
		"--verification-workflow", "workflow.core-10",
	)

	out := runCLIFailsWithEnv(t, fakeDockerEnv, "environment", "restore", "--store", "sqlite://"+storePath, "--workspace", workspace, "--execute", "--clean-docker-state", "--json", "env.cleanup.block")
	if !strings.Contains(out, "cleanup-blocked") || !strings.Contains(out, "--allow-destructive-docker-cleanup") {
		t.Fatalf("cleanup block output = %q", out)
	}
	if raw, err := os.ReadFile(dockerCallsPath); err == nil {
		calls := string(raw)
		for _, forbidden := range []string{" down ", " pull", " build", " up -d"} {
			if strings.Contains(calls, forbidden) {
				t.Fatalf("blocked cleanup should not run docker command %q:\n%s", forbidden, calls)
			}
		}
	}
	inspectOut := runCLI(t, "environment", "inspect", "--store", "sqlite://"+storePath, "--json", "env.cleanup.block")
	var inspected struct {
		Environment struct {
			Summary struct {
				LastRestore struct {
					OK     bool   `json:"ok"`
					Phase  string `json:"phase"`
					Docker struct {
						Action  string `json:"action"`
						OK      bool   `json:"ok"`
						Cleanup struct {
							Requested bool   `json:"requested"`
							Action    string `json:"action"`
							Error     string `json:"error"`
						} `json:"cleanup"`
					} `json:"docker"`
				} `json:"lastRestore"`
				RestoreAttempts []struct {
					Phase string `json:"phase"`
				} `json:"restoreAttempts"`
			} `json:"summary"`
		} `json:"environment"`
	}
	if err := json.Unmarshal([]byte(inspectOut), &inspected); err != nil {
		t.Fatalf("decode cleanup block inspect json: %v\n%s", err, inspectOut)
	}
	lastRestore := inspected.Environment.Summary.LastRestore
	if lastRestore.OK || lastRestore.Phase != "docker" || lastRestore.Docker.OK || lastRestore.Docker.Action != "plan-docker-compose" || !lastRestore.Docker.Cleanup.Requested || lastRestore.Docker.Cleanup.Action != "cleanup-blocked" || !strings.Contains(lastRestore.Docker.Cleanup.Error, "--allow-destructive-docker-cleanup") {
		t.Fatalf("persisted blocked cleanup summary = %#v", lastRestore)
	}
	if len(inspected.Environment.Summary.RestoreAttempts) != 1 || inspected.Environment.Summary.RestoreAttempts[0].Phase != "docker" {
		t.Fatalf("persisted blocked cleanup attempts = %#v", inspected.Environment.Summary.RestoreAttempts)
	}
}

func TestEnvironmentRestoreBlocksAllowedDockerCleanupWithoutCompleteLinkage(t *testing.T) {
	storePath := filepath.Join(t.TempDir(), "store.sqlite")
	workspace := filepath.Join(t.TempDir(), "workspace")
	fakeDockerEnv, dockerCallsPath := fakeDockerCommand(t)
	composeSource := filepath.Join(t.TempDir(), "compose.yml")
	writeFile(t, composeSource, "services:\n  web:\n    image: alpine:3.20\n")
	runCLI(t, "environment", "register",
		"--store", "sqlite://"+storePath,
		"--id", "env.cleanup.linkage.block",
		"--compose-file", "compose.yml",
		"--compose-generated-file", "compose.yml="+composeSource,
		"--compose-service", "web",
		"--verification-workflow", "workflow.core-10",
	)

	out := runCLIFailsWithEnv(t, fakeDockerEnv, "environment", "restore", "--store", "sqlite://"+storePath, "--workspace", workspace, "--execute", "--clean-docker-state", "--allow-destructive-docker-cleanup", "--json", "env.cleanup.linkage.block")
	if !strings.Contains(out, "cleanup-linkage-blocked") || !strings.Contains(out, "Store-to-Compose environment linkage") || !strings.Contains(out, "projectName") {
		t.Fatalf("cleanup linkage block output = %q", out)
	}
	var report struct {
		Docker struct {
			Cleanup struct {
				Linkage struct {
					RepairPlan []struct {
						Name          string `json:"name"`
						Target        string `json:"target"`
						CommandHint   string `json:"commandHint"`
						StoreBacked   bool   `json:"storeBacked"`
						BlocksCleanup bool   `json:"blocksCleanup"`
					} `json:"repairPlan"`
				} `json:"linkage"`
			} `json:"cleanup"`
		} `json:"docker"`
	}
	if err := json.Unmarshal([]byte(extractJSONObject(t, out)), &report); err != nil {
		t.Fatalf("decode cleanup linkage block json: %v\n%s", err, out)
	}
	repairNames := map[string]bool{}
	for _, item := range report.Docker.Cleanup.Linkage.RepairPlan {
		repairNames[item.Name] = true
		if !item.StoreBacked || !item.BlocksCleanup {
			t.Fatalf("cleanup repair item should be Store-backed and blocking: %#v", item)
		}
	}
	for _, want := range []string{"compose-project-name", "component-graph"} {
		if !repairNames[want] {
			t.Fatalf("cleanup linkage repair plan missing %q: %#v", want, report.Docker.Cleanup.Linkage.RepairPlan)
		}
	}
	if raw, err := os.ReadFile(dockerCallsPath); err == nil {
		calls := string(raw)
		if strings.Contains(calls, " down ") || strings.Contains(calls, " up -d") {
			t.Fatalf("linkage-blocked cleanup should not run down/up:\n%s", calls)
		}
	}
}

func TestEnvironmentRestoreBlocksDockerCleanupWhenComposeNativeProjectionMissing(t *testing.T) {
	storePath := filepath.Join(t.TempDir(), "store.sqlite")
	workspace := filepath.Join(t.TempDir(), "workspace")
	fakeDockerEnv, dockerCallsPath := fakeDockerCommand(t)
	composeSource := filepath.Join(t.TempDir(), "compose.yml")
	writeFile(t, composeSource, "services:\n  web:\n    image: alpine:3.20\n    env_file:\n      - ./app.env\n")
	runCLI(t, "environment", "register",
		"--store", "sqlite://"+storePath,
		"--id", "env.cleanup.native-file-gap",
		"--compose-file", "compose.yml",
		"--compose-generated-file", "compose.yml="+composeSource,
		"--compose-project-name", "demo",
		"--compose-service", "web",
		"--verification-workflow", "workflow.core-10",
	)
	seedCleanupLinkedGraph(t, storePath, "env.cleanup.native-file-gap", "web")

	out := runCLIFailsWithEnv(t, fakeDockerEnv, "environment", "restore", "--store", "sqlite://"+storePath, "--workspace", workspace, "--execute", "--clean-docker-state", "--allow-destructive-docker-cleanup", "--json", "env.cleanup.native-file-gap")
	if !strings.Contains(out, environmentRestoreDockerActionSkippedFileProjection) || !strings.Contains(out, "env-file:app.env") {
		t.Fatalf("cleanup should block missing Compose-native projection: %q", out)
	}
	var report struct {
		Docker struct {
			Action string `json:"action"`
		} `json:"docker"`
		FileProjection struct {
			RepairPlan []struct {
				Name    string   `json:"name"`
				Target  string   `json:"target"`
				Missing []string `json:"missing"`
			} `json:"repairPlan"`
		} `json:"fileProjection"`
	}
	if err := json.Unmarshal([]byte(extractJSONObject(t, out)), &report); err != nil {
		t.Fatalf("decode native projection cleanup json: %v\n%s", err, out)
	}
	if report.Docker.Action != environmentRestoreDockerActionSkippedFileProjection {
		t.Fatalf("native projection should block before Docker cleanup: %#v", report.Docker)
	}
	if len(report.FileProjection.RepairPlan) != 1 || report.FileProjection.RepairPlan[0].Name != "compose-file-projection" || report.FileProjection.RepairPlan[0].Target != "fileProjection.missing" || !stringSliceContains(report.FileProjection.RepairPlan[0].Missing, "env-file:app.env") {
		t.Fatalf("native projection repair plan = %#v", report.FileProjection.RepairPlan)
	}
	if raw, err := os.ReadFile(dockerCallsPath); err == nil {
		calls := string(raw)
		if strings.Contains(calls, " down ") || strings.Contains(calls, " up -d") {
			t.Fatalf("native projection gap should not run down/up:\n%s", calls)
		}
	}
}

func TestEnvironmentRestoreRunsAllowedDockerCleanupBeforeStartup(t *testing.T) {
	storePath := filepath.Join(t.TempDir(), "store.sqlite")
	workspace := filepath.Join(t.TempDir(), "workspace")
	fakeDockerEnv, dockerCallsPath := fakeDockerCommand(t)
	composeSource := filepath.Join(t.TempDir(), "compose.yml")
	writeFile(t, composeSource, "services:\n  web:\n    image: alpine:3.20\n")
	runCLI(t, "environment", "register",
		"--store", "sqlite://"+storePath,
		"--id", "env.cleanup.execute",
		"--compose-file", "compose.yml",
		"--compose-generated-file", "compose.yml="+composeSource,
		"--compose-project-name", "demo",
		"--compose-service", "web",
		"--compose-skip-pull",
		"--compose-skip-build",
		"--health-url", newHealthyTestURL(t),
		"--verification-workflow", "workflow.core-10",
	)
	seedCleanupLinkedGraph(t, storePath, "env.cleanup.execute", "web")

	out := runCLIWithEnv(t, fakeDockerEnv, "environment", "restore", "--store", "sqlite://"+storePath, "--workspace", workspace, "--execute", "--clean-docker-state", "--clean-docker-images", "--allow-destructive-docker-cleanup", "--json", "env.cleanup.execute")
	var report struct {
		OK     bool `json:"ok"`
		Docker struct {
			Cleanup struct {
				Action string `json:"action"`
			} `json:"cleanup"`
		} `json:"docker"`
	}
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatalf("decode cleanup execute json: %v\n%s", err, out)
	}
	if !report.OK || report.Docker.Cleanup.Action != "run-cleanup" {
		t.Fatalf("cleanup execute report = %#v", report)
	}
	raw, err := os.ReadFile(dockerCallsPath)
	if err != nil {
		t.Fatalf("read fake docker calls: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(raw)), "\n")
	joined := strings.Join(lines, "\n")
	base := "compose -f " + filepath.Join(workspace, "compose.yml") + " --env-file " + environmentRestoreGeneratedEnvFilePath(workspace) + " -p demo"
	for _, want := range []string{base + " ps", base + " images", base + " config", base + " down --remove-orphans --rmi all", base + " up -d web"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("cleanup docker calls missing %q:\n%s", want, joined)
		}
	}
	if strings.Contains(joined, "--volumes") || strings.Contains(joined, "system prune") {
		t.Fatalf("cleanup should not remove volumes or run global prune:\n%s", joined)
	}
	order := []string{" ps", " images", " config", " down --remove-orphans --rmi all", " up -d"}
	last := -1
	for _, marker := range order {
		index := strings.Index(joined, marker)
		if index <= last {
			t.Fatalf("cleanup order marker %q out of order in:\n%s", marker, joined)
		}
		last = index
	}
}

func TestEnvironmentRestoreStreamJSONEmitsCleanupCommandProgress(t *testing.T) {
	fixture := newEnvironmentRestoreDockerCLIFixture(t)
	fixture.writeDockerTool(t, `#!/usr/bin/env bash
printf '%s\n' "$*" >> "$DOCKER_CALLS_FILE"
if [ "$1" = "compose" ] && [ "$2" = "version" ]; then
  printf 'Docker Compose version v2.0.0\n'
  exit 0
fi
if [ "$1" = "compose" ]; then
  saw_down=0
  collect_services=0
  prev=""
  for arg in "$@"; do
    if [ "$arg" = "down" ]; then
      saw_down=1
    fi
    if [ "$prev" = "--format" ] && [ "$arg" = "json" ]; then
      collect_services=1
      prev="$arg"
      continue
    fi
    if [ "$collect_services" = "1" ] && [ "${arg#-}" = "$arg" ]; then
      printf '{"Name":"%s","Service":"%s","State":"running","Health":"healthy"}\n' "$arg" "$arg"
    fi
    prev="$arg"
  done
  if [ "$saw_down" = "1" ]; then
    sleep 0.05
  fi
fi
`)
	composeSource := filepath.Join(t.TempDir(), "compose.yml")
	writeFile(t, composeSource, "services:\n  web:\n    image: alpine:3.20\n")
	runCLI(t, "environment", "register",
		"--store", fixture.StoreDSN,
		"--id", "env.cleanup.stream",
		"--compose-file", "compose.yml",
		"--compose-generated-file", "compose.yml="+composeSource,
		"--compose-project-name", "demo-stream",
		"--compose-service", "web",
		"--compose-skip-pull",
		"--compose-skip-build",
		"--health-url", newHealthyTestURL(t),
		"--verification-workflow", "workflow.core-10",
	)
	seedCleanupLinkedGraph(t, fixture.StorePath, "env.cleanup.stream", "web")

	env := append([]string{}, fixture.DockerEnv...)
	env = append(env, "AGENT_TESTBENCH_COMPOSE_EXECUTE_PROGRESS_INTERVAL_MS=1")
	out := runCLIWithEnv(t, env, "environment", "restore",
		"--store", fixture.StoreDSN,
		"--workspace", fixture.Workspace,
		"--execute",
		"--clean-docker-state",
		"--allow-destructive-docker-cleanup",
		"--output-format", "stream-json",
		"env.cleanup.stream",
	)
	events := decodeAgentStreamEvents(t, out)
	if !agentStreamHasEvent(events, "tool_observation", "docker.cleanup", "waiting", "docker compose down") {
		t.Fatalf("stream missing docker cleanup waiting observation: %#v", events)
	}
	if !agentStreamHasEvent(events, "tool_call_started", "command", "started", "docker compose down") {
		t.Fatalf("stream missing docker cleanup command start: %#v", events)
	}
}

func TestEnvironmentRestoreCleanupRemovesConflictingFixedContainerNames(t *testing.T) {
	fixture := newEnvironmentRestoreDockerCLIFixture(t)
	fixture.writeDockerTool(t, `#!/usr/bin/env bash
printf '%s\n' "$*" >> "$DOCKER_CALLS_FILE"
if [ "$1" = "compose" ] && [ "$2" = "version" ]; then
  printf 'Docker Compose version v2.0.0\n'
  exit 0
fi
if [ "$1" = "ps" ] && [ "$2" = "-a" ]; then
  printf 'sandbox-kafka\n'
  exit 0
fi
if [ "$1" = "compose" ] && [[ "$*" == *" ps -a --format json "* ]]; then
  service="${@: -1}"
  printf '{"Name":"%s","Service":"%s","State":"running","Health":"healthy"}\n' "$service" "$service"
fi
`)
	composeSource := filepath.Join(t.TempDir(), "compose.yml")
	writeFile(t, composeSource, "services:\n  kafka:\n    image: apache/kafka:3.7.0\n    container_name: sandbox-kafka\n")
	runCLI(t, "environment", "register",
		"--store", fixture.StoreDSN,
		"--id", "env.cleanup.fixed-container",
		"--compose-file", "compose.yml",
		"--compose-generated-file", "compose.yml="+composeSource,
		"--compose-project-name", "demo-fixed",
		"--compose-service", "kafka",
		"--compose-skip-pull",
		"--compose-skip-build",
		"--health-compose-service", "kafka",
		"--verification-workflow", "workflow.core-10",
	)
	seedCleanupLinkedGraph(t, strings.TrimPrefix(fixture.StoreDSN, "sqlite://"), "env.cleanup.fixed-container", "kafka")

	out := runCLIWithEnv(t, fixture.DockerEnv, "environment", "restore", "--store", fixture.StoreDSN, "--workspace", fixture.Workspace, "--execute", "--clean-docker-state", "--allow-destructive-docker-cleanup", "--json", "env.cleanup.fixed-container")
	if !strings.Contains(out, `"ok": true`) || !strings.Contains(out, "fixedContainerNames") {
		t.Fatalf("cleanup with fixed container report = %s", out)
	}
	raw, err := os.ReadFile(fixture.DockerCallsPath)
	if err != nil {
		t.Fatalf("read fake docker calls: %v", err)
	}
	joined := string(raw)
	base := "compose -f " + filepath.Join(fixture.Workspace, "compose.yml") + " --env-file " + environmentRestoreGeneratedEnvFilePath(fixture.Workspace) + " -p demo-fixed"
	for _, want := range []string{
		base + " down --remove-orphans",
		"rm -f sandbox-kafka",
		base + " up -d kafka",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("cleanup docker calls missing %q:\n%s", want, joined)
		}
	}
}

func TestEnvironmentRestoreCleanupIgnoresMissingFixedContainerAfterComposeDown(t *testing.T) {
	fixture := newEnvironmentRestoreDockerCLIFixture(t)
	fixture.writeDockerTool(t, `#!/usr/bin/env bash
printf '%s\n' "$*" >> "$DOCKER_CALLS_FILE"
if [ "$1" = "compose" ] && [ "$2" = "version" ]; then
  printf 'Docker Compose version v2.0.0\n'
  exit 0
fi
if [ "$1" = "ps" ] && [ "$2" = "-a" ]; then
  printf 'sandbox-kafka\n'
  exit 0
fi
if [ "$1" = "rm" ] && [ "$2" = "-f" ]; then
  printf 'Error: No such container: %s\n' "$3" >&2
  exit 1
fi
if [ "$1" = "compose" ] && [[ "$*" == *" ps -a --format json "* ]]; then
  service="${@: -1}"
  printf '{"Name":"%s","Service":"%s","State":"running","Health":"healthy"}\n' "$service" "$service"
fi
`)
	composeSource := filepath.Join(t.TempDir(), "compose.yml")
	writeFile(t, composeSource, "services:\n  kafka:\n    image: apache/kafka:3.7.0\n    container_name: sandbox-kafka\n")
	runCLI(t, "environment", "register",
		"--store", fixture.StoreDSN,
		"--id", "env.cleanup.fixed-container-missing",
		"--compose-file", "compose.yml",
		"--compose-generated-file", "compose.yml="+composeSource,
		"--compose-project-name", "demo-fixed-missing",
		"--compose-service", "kafka",
		"--compose-skip-pull",
		"--compose-skip-build",
		"--health-compose-service", "kafka",
		"--verification-workflow", "workflow.core-10",
	)
	seedCleanupLinkedGraph(t, strings.TrimPrefix(fixture.StoreDSN, "sqlite://"), "env.cleanup.fixed-container-missing", "kafka")

	out := runCLIWithEnv(t, fixture.DockerEnv, "environment", "restore", "--store", fixture.StoreDSN, "--workspace", fixture.Workspace, "--execute", "--clean-docker-state", "--allow-destructive-docker-cleanup", "--json", "env.cleanup.fixed-container-missing")
	if !strings.Contains(out, `"ok": true`) {
		t.Fatalf("missing fixed container after compose down should not fail cleanup: %s", out)
	}
}

func TestEnvironmentRestoreFailsBeforeDockerWhenComposeFileIsMissing(t *testing.T) {
	storePath := filepath.Join(t.TempDir(), "store.sqlite")
	workspace := filepath.Join(t.TempDir(), "workspace")
	runCLI(t, "environment", "register",
		"--store", "sqlite://"+storePath,
		"--id", "env.missing.compose",
		"--compose-file", "missing-compose.yml",
		"--verification-workflow", "workflow.core-10",
	)

	out := runCLIFails(t, "environment", "restore", "env.missing.compose", "--store", "sqlite://"+storePath, "--workspace", workspace, "--execute", "--json")
	if !strings.Contains(out, "missing-compose-file") || !strings.Contains(out, "missing-compose.yml") {
		t.Fatalf("missing compose restore output = %q", out)
	}
}
