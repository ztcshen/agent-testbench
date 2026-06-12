package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEnvironmentRestorePlansDockerCleanupWithoutExecuting(t *testing.T) {
	storePath := filepath.Join(t.TempDir(), "store.sqlite")
	workspace := filepath.Join(t.TempDir(), "workspace")
	fakeDockerEnv, _ := fakeDockerCommand(t)
	writeFile(t, filepath.Join(workspace, "compose.yml"), "services: {}\n")
	runCLI(t, "environment", "register",
		"--store", "sqlite://"+storePath,
		"--id", "env.cleanup.plan",
		"--compose-file", "compose.yml",
		"--compose-project-name", "demo",
		"--compose-service", "web",
		"--verification-workflow", "workflow.core-10",
	)

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
			} `json:"cleanup"`
		} `json:"docker"`
	}
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatalf("decode cleanup dry-run json: %v\n%s", err, out)
	}
	cleanup := report.Docker.Cleanup
	if !report.OK || !cleanup.Requested || cleanup.Allowed || !cleanup.IncludeImages || cleanup.Action != "plan-cleanup" || len(cleanup.BackupCommands) != 3 || len(cleanup.Commands) != 1 {
		t.Fatalf("cleanup dry-run report = %#v", report.Docker.Cleanup)
	}
	command := strings.Join(cleanup.Commands[0], " ")
	if !strings.Contains(command, "compose -f "+filepath.Join(workspace, "compose.yml")+" -p demo down --remove-orphans --rmi all") {
		t.Fatalf("cleanup command = %#v", cleanup.Commands[0])
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
	writeFile(t, filepath.Join(workspace, "compose.yml"), "services: {}\n")
	runCLI(t, "environment", "register",
		"--store", "sqlite://"+storePath,
		"--id", "env.cleanup.block",
		"--compose-file", "compose.yml",
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

func TestEnvironmentRestoreRunsAllowedDockerCleanupBeforeStartup(t *testing.T) {
	storePath := filepath.Join(t.TempDir(), "store.sqlite")
	workspace := filepath.Join(t.TempDir(), "workspace")
	fakeDockerEnv, dockerCallsPath := fakeDockerCommand(t)
	writeFile(t, filepath.Join(workspace, "compose.yml"), "services: {}\n")
	runCLI(t, "environment", "register",
		"--store", "sqlite://"+storePath,
		"--id", "env.cleanup.execute",
		"--compose-file", "compose.yml",
		"--compose-skip-pull",
		"--compose-skip-build",
		"--health-url", newHealthyTestURL(t),
		"--verification-workflow", "workflow.core-10",
	)

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
	for _, want := range []string{"compose -f " + filepath.Join(workspace, "compose.yml") + " ps", "compose -f " + filepath.Join(workspace, "compose.yml") + " images", "compose -f " + filepath.Join(workspace, "compose.yml") + " config", "compose -f " + filepath.Join(workspace, "compose.yml") + " down --remove-orphans --rmi all", "compose -f " + filepath.Join(workspace, "compose.yml") + " up -d"} {
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
		"--compose-service", "kafka",
		"--compose-skip-pull",
		"--compose-skip-build",
		"--health-compose-service", "kafka",
		"--verification-workflow", "workflow.core-10",
	)

	out := runCLIWithEnv(t, fixture.DockerEnv, "environment", "restore", "--store", fixture.StoreDSN, "--workspace", fixture.Workspace, "--execute", "--clean-docker-state", "--allow-destructive-docker-cleanup", "--json", "env.cleanup.fixed-container")
	if !strings.Contains(out, `"ok": true`) || !strings.Contains(out, "fixedContainerNames") {
		t.Fatalf("cleanup with fixed container report = %s", out)
	}
	raw, err := os.ReadFile(fixture.DockerCallsPath)
	if err != nil {
		t.Fatalf("read fake docker calls: %v", err)
	}
	joined := string(raw)
	for _, want := range []string{
		"compose -f " + filepath.Join(fixture.Workspace, "compose.yml") + " down --remove-orphans",
		"rm -f sandbox-kafka",
		"compose -f " + filepath.Join(fixture.Workspace, "compose.yml") + " up -d kafka",
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
		"--compose-service", "kafka",
		"--compose-skip-pull",
		"--compose-skip-build",
		"--health-compose-service", "kafka",
		"--verification-workflow", "workflow.core-10",
	)

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
