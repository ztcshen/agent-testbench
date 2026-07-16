package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestEnvironmentRestoreGeneratedEnvFileUsesOwnerOnlyPermissions(t *testing.T) {
	workspace := t.TempDir()
	compose := map[string]any{"env": map[string]any{"API_TOKEN": "sentinel-mode-secret"}}
	path, err := writeEnvironmentRestoreGeneratedEnvFile(workspace, compose)
	if err != nil {
		t.Fatalf("write generated env file: %v", err)
	}
	assertFileMode(t, path, 0o600)

	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatalf("relax generated env file mode: %v", err)
	}
	if _, err := writeEnvironmentRestoreGeneratedEnvFile(workspace, compose); err != nil {
		t.Fatalf("rewrite generated env file: %v", err)
	}
	assertFileMode(t, path, 0o600)
}

func TestEnvironmentRestoreGeneratedEnvFileRejectsSymlinkTargets(t *testing.T) {
	compose := map[string]any{"env": map[string]any{"API_TOKEN": "sentinel-symlink-secret"}}
	t.Run("file", func(t *testing.T) {
		workspace := t.TempDir()
		outside := filepath.Join(t.TempDir(), "outside.env")
		writeFile(t, outside, "outside-must-survive\n")
		if err := os.Mkdir(filepath.Join(workspace, ".agent-testbench"), 0o755); err != nil {
			t.Fatalf("create projection directory: %v", err)
		}
		if err := os.Symlink(outside, environmentRestoreGeneratedEnvFilePath(workspace)); err != nil {
			t.Fatalf("create file symlink: %v", err)
		}
		if _, err := writeEnvironmentRestoreGeneratedEnvFile(workspace, compose); err == nil {
			t.Fatal("generated environment writer followed a file symlink")
		}
		assertFileContent(t, outside, "outside-must-survive\n")
	})

	t.Run("parent-directory", func(t *testing.T) {
		workspace := t.TempDir()
		outside := t.TempDir()
		if err := os.Symlink(outside, filepath.Join(workspace, ".agent-testbench")); err != nil {
			t.Fatalf("create parent symlink: %v", err)
		}
		if _, err := writeEnvironmentRestoreGeneratedEnvFile(workspace, compose); err == nil {
			t.Fatal("generated environment writer followed a parent directory symlink")
		}
		if _, err := os.Lstat(filepath.Join(outside, "restore.env")); !os.IsNotExist(err) {
			t.Fatalf("writer escaped through parent symlink: %v", err)
		}
	})
}

func TestEnvironmentLifecycleProjectionReaderRejectsSymlinkEscapes(t *testing.T) {
	t.Run("file", func(t *testing.T) {
		workspace := t.TempDir()
		outside := filepath.Join(t.TempDir(), "outside.yml")
		writeFile(t, outside, "outside-file-secret\n")
		if err := os.Symlink(outside, filepath.Join(workspace, "compose.yml")); err != nil {
			t.Fatalf("create projection file symlink: %v", err)
		}
		content, err := readEnvironmentLifecycleProjectionFile(workspace, "compose.yml", 0o644)
		if err == nil || strings.Contains(content, "outside-file-secret") {
			t.Fatalf("projection reader followed file symlink: content=%q err=%v", content, err)
		}
	})

	t.Run("parent-directory", func(t *testing.T) {
		workspace := t.TempDir()
		outside := t.TempDir()
		writeFile(t, filepath.Join(outside, "compose.yml"), "outside-parent-secret\n")
		if err := os.Symlink(outside, filepath.Join(workspace, "projection")); err != nil {
			t.Fatalf("create projection parent symlink: %v", err)
		}
		content, err := readEnvironmentLifecycleProjectionFile(workspace, "projection/compose.yml", 0o644)
		if err == nil || strings.Contains(content, "outside-parent-secret") {
			t.Fatalf("projection reader followed parent symlink: content=%q err=%v", content, err)
		}
	})

	t.Run("mode", func(t *testing.T) {
		workspace := t.TempDir()
		path := filepath.Join(workspace, "compose.yml")
		writeFile(t, path, "services: {}\n")
		if err := os.Chmod(path, 0o600); err != nil {
			t.Fatalf("chmod projection: %v", err)
		}
		if _, err := readEnvironmentLifecycleProjectionFile(workspace, "compose.yml", 0o644); err == nil || !strings.Contains(err.Error(), "mode") {
			t.Fatalf("projection reader did not preserve mode validation: %v", err)
		}
	})
}

func assertFileContent(t *testing.T, path string, want string) {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	if string(raw) != want {
		t.Fatalf("content for %s = %q, want %q", path, raw, want)
	}
}

func TestEnvironmentStatusIsReadOnlyAndRedactsEnvironmentSecrets(t *testing.T) {
	fixture := newEnvironmentRestoreDockerCLIFixture(t)
	secret := "sentinel-status-env-secret"
	registerLifecycleProjectionFixture(t, fixture, "env.status.read-only", secret)
	composePath, envPath, oldTime := prepareLifecycleProjectionFixture(t, fixture.Workspace, secret)

	out := runCLIWithEnv(t, fixture.DockerEnv, "environment", "status",
		"--store", fixture.StoreDSN,
		"--workspace", fixture.Workspace,
		"--json",
		"env.status.read-only",
	)
	assertEnvironmentOutputHasNoSecrets(t, out, secret)
	assertFileUnchangedSince(t, composePath, oldTime)
	assertFileUnchangedSince(t, envPath, oldTime)
}

func TestEnvironmentStatusDoesNotMaterializeMissingProjection(t *testing.T) {
	fixture := newEnvironmentRestoreDockerCLIFixture(t)
	registerLifecycleProjectionFixture(t, fixture, "env.status.missing-projection", "sentinel-missing-secret")

	out := runCLIFailsWithEnv(t, fixture.DockerEnv, "environment", "status",
		"--store", fixture.StoreDSN,
		"--workspace", fixture.Workspace,
		"--json",
		"env.status.missing-projection",
	)
	if !strings.Contains(out, "read-only") || !strings.Contains(out, "--prepare-repos-only") {
		t.Fatalf("missing projection should explain the explicit materialization step:\n%s", out)
	}
	for _, path := range []string{
		filepath.Join(fixture.Workspace, "compose.yml"),
		environmentRestoreGeneratedEnvFilePath(fixture.Workspace),
	} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("read-only status unexpectedly materialized %s: %v", path, err)
		}
	}
}

func TestEnvironmentStopIsReadOnlyAndDoesNotPersistSecrets(t *testing.T) {
	fixture := newEnvironmentRestoreDockerCLIFixture(t)
	secret := "sentinel-stop-env-secret"
	fixture.writeDockerTool(t, `#!/usr/bin/env bash
if [ "$1" = compose ] && [[ "$*" == *" stop "* ]]; then
  printf '%s\n' 'sentinel-stop-env-secret' >&2
  exit 9
fi
exit 0
`)
	registerLifecycleProjectionFixture(t, fixture, "env.stop.read-only", secret)
	composePath, envPath, oldTime := prepareLifecycleProjectionFixture(t, fixture.Workspace, secret)

	out := runCLIFailsWithEnv(t, fixture.DockerEnv, "environment", "stop",
		"--store", fixture.StoreDSN,
		"--workspace", fixture.Workspace,
		"--json",
		"env.stop.read-only",
	)
	assertEnvironmentOutputHasNoSecrets(t, out, secret)
	assertFileUnchangedSince(t, composePath, oldTime)
	assertFileUnchangedSince(t, envPath, oldTime)

	runtime, err := openStore(context.Background(), fixture.StoreDSN)
	if err != nil {
		t.Fatalf("open Store: %v", err)
	}
	defer closeCLIStore(runtime)
	env, err := runtime.GetEnvironment(context.Background(), "env.stop.read-only")
	if err != nil {
		t.Fatalf("get environment: %v", err)
	}
	assertEnvironmentOutputHasNoSecrets(t, env.SummaryJSON, secret)
}

func registerLifecycleProjectionFixture(t *testing.T, fixture environmentRestoreDockerCLIFixture, id string, secret string) {
	t.Helper()
	composeSource := filepath.Join(t.TempDir(), "compose.yml")
	writeFile(t, composeSource, "services:\n  web:\n    image: alpine:3.20\n")
	runCLI(t, "environment", "register",
		"--store", fixture.StoreDSN,
		"--id", id,
		"--compose-file", "compose.yml",
		"--compose-generated-file", "compose.yml="+composeSource,
		"--compose-env", "API_TOKEN="+secret,
		"--compose-service", "web",
		"--health-compose-service", "web",
		"--verification-workflow", "workflow.lifecycle-projection",
	)
}

func prepareLifecycleProjectionFixture(t *testing.T, workspace string, secret string) (string, string, time.Time) {
	t.Helper()
	composePath := filepath.Join(workspace, "compose.yml")
	writeFile(t, composePath, "services:\n  web:\n    image: alpine:3.20\n")
	envPath, err := writeEnvironmentRestoreGeneratedEnvFile(workspace, map[string]any{
		"env": map[string]any{"API_TOKEN": secret},
	})
	if err != nil {
		t.Fatalf("prepare generated env file: %v", err)
	}
	if err := os.Chmod(envPath, 0o600); err != nil {
		t.Fatalf("chmod generated env file: %v", err)
	}
	oldTime := time.Unix(946684800, 0)
	for _, path := range []string{composePath, envPath} {
		if err := os.Chtimes(path, oldTime, oldTime); err != nil {
			t.Fatalf("set fixture timestamp for %s: %v", path, err)
		}
	}
	return composePath, envPath, oldTime
}

func assertFileMode(t *testing.T, path string, want os.FileMode) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat %s: %v", path, err)
	}
	if info.Mode().Perm() != want {
		t.Fatalf("mode for %s = %o, want %o", path, info.Mode().Perm(), want)
	}
}

func assertFileUnchangedSince(t *testing.T, path string, wantTime time.Time) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat %s: %v", path, err)
	}
	if !info.ModTime().Equal(wantTime) {
		t.Fatalf("read-only lifecycle command rewrote %s: mtime=%s want=%s", path, info.ModTime(), wantTime)
	}
}
