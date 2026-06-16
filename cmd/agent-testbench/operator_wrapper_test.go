package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestOperatorWrapperStatusPrefersRepoRuntime(t *testing.T) {
	repo := t.TempDir()
	writeExecutable(t, filepath.Join(repo, ".runtime", "bin", "agent-testbench"), "#!/usr/bin/env sh\necho runtime \"$@\"\n")
	writeExecutable(t, filepath.Join(repo, "bin", "agent-testbench.sh"), "#!/usr/bin/env sh\necho wrapper \"$@\"\n")

	cmd := exec.Command("./skills/agent-testbench-operator/scripts/atb.sh", "status")
	cmd.Dir = repoRootForTest(t)
	cmd.Env = append(os.Environ(), "ATB_REPO_DIR="+repo, "ATB_BIN=")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("run atb wrapper: %v\n%s", err, out)
	}
	if strings.TrimSpace(string(out)) != "runtime status" {
		t.Fatalf("status should use repo runtime, got %q", out)
	}
}

func TestRepoWrapperStatusPrefersRepoRuntime(t *testing.T) {
	repo := t.TempDir()
	writeExecutable(t, filepath.Join(repo, ".runtime", "bin", "agent-testbench"), "#!/usr/bin/env sh\necho runtime \"$@\"\n")
	copyFile(t, filepath.Join(repoRootForTest(t), "bin", "agent-testbench.sh"), filepath.Join(repo, "bin", "agent-testbench.sh"), 0o755)

	cmd := exec.Command(filepath.Join(repo, "bin", "agent-testbench.sh"), "status", "--json")
	cmd.Env = append(os.Environ(), "ATB_BIN=")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("run repo wrapper: %v\n%s", err, out)
	}
	if strings.TrimSpace(string(out)) != "runtime status --json" {
		t.Fatalf("repo wrapper status should use repo runtime, got %q", out)
	}
}

func writeExecutable(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
		t.Fatalf("write executable %s: %v", path, err)
	}
}

func copyFile(t *testing.T, src string, dst string, mode os.FileMode) {
	t.Helper()
	raw, err := os.ReadFile(src)
	if err != nil {
		t.Fatalf("read %s: %v", src, err)
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(dst), err)
	}
	if err := os.WriteFile(dst, raw, mode); err != nil {
		t.Fatalf("write %s: %v", dst, err)
	}
}

func repoRootForTest(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	return filepath.Clean(filepath.Join(wd, "..", ".."))
}
