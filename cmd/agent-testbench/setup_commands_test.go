package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSetupConfiguresLocalStoreAndCanBuildRuntime(t *testing.T) {
	configHome := t.TempDir()
	repo := createSetupRepo(t)
	fakeGoEnv, callsPath := fakeUpdateGoCommand(t)
	env := append(fakeGoEnv, "AGENT_TESTBENCH_CONFIG_HOME="+configHome)

	out := runCLIWithEnv(t, env, "setup", "--repo", repo, "--store", "local", "--sqlite", filepath.Join(repo, ".runtime", "local.sqlite"), "--build-runtime", "--json")
	var report struct {
		OK    bool `json:"ok"`
		Store struct {
			Name    string `json:"name"`
			Backend string `json:"backend"`
			Active  bool   `json:"active"`
		} `json:"store"`
		Runtime struct {
			Built bool   `json:"built"`
			Path  string `json:"path"`
		} `json:"runtime"`
		Next []string `json:"next"`
	}
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatalf("decode setup report: %v\n%s", err, out)
	}
	if !report.OK || report.Store.Name != "local" || report.Store.Backend != "sqlite" || !report.Store.Active || !report.Runtime.Built {
		t.Fatalf("setup report = %#v", report)
	}
	calls := readUpdateCalls(t, callsPath)
	if !strings.Contains(calls, "build -ldflags -X main.buildRevision=") || !strings.Contains(calls, "-o "+report.Runtime.Path+" ./cmd/agent-testbench") {
		t.Fatalf("setup did not build runtime: %s", readUpdateCalls(t, callsPath))
	}
	if len(report.Next) == 0 || !strings.Contains(strings.Join(report.Next, "\n"), "agent-testbench status") {
		t.Fatalf("setup next actions = %#v", report.Next)
	}
}

func TestSetupRejectsNonGitRepoWithoutCreatingConfigOrRuntime(t *testing.T) {
	configHome := t.TempDir()
	repo := filepath.Join(t.TempDir(), "not-a-checkout")
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatalf("create non-git repo dir: %v", err)
	}

	out := runCLIFailsWithEnv(t, []string{"AGENT_TESTBENCH_CONFIG_HOME=" + configHome}, "setup", "--repo", repo, "--json")

	if !strings.Contains(out, "git checkout") {
		t.Fatalf("setup should reject non-git repo paths with checkout guidance:\n%s", out)
	}
	if _, err := os.Stat(filepath.Join(repo, ".runtime")); !os.IsNotExist(err) {
		t.Fatalf("setup should not create runtime dir for invalid repo, stat err = %v", err)
	}
	if _, err := os.Stat(filepath.Join(configHome, "store-config.json")); !os.IsNotExist(err) {
		t.Fatalf("setup should not write config for invalid repo, stat err = %v", err)
	}
}

func TestSetupRuntimeOnlyPreservesActiveStore(t *testing.T) {
	configHome := t.TempDir()
	repo := createSetupRepo(t)
	fakeGoEnv, callsPath := fakeUpdateGoCommand(t)
	env := append(fakeGoEnv, "AGENT_TESTBENCH_CONFIG_HOME="+configHome)
	runCLIWithEnv(t, env, "store", "config", "set", "team", "--url", "postgres://user:secret@example.com:5432/team?sslmode=disable")
	runCLIWithEnv(t, env, "store", "use", "team")

	out := runCLIWithEnv(t, env, "setup", "--repo", repo, "--build-runtime", "--runtime-only", "--json")
	var report struct {
		OK      bool `json:"ok"`
		Store   any  `json:"store"`
		Runtime struct {
			Built bool   `json:"built"`
			Path  string `json:"path"`
		} `json:"runtime"`
	}
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatalf("decode runtime-only setup report: %v\n%s", err, out)
	}
	if !report.OK || !report.Runtime.Built || report.Runtime.Path == "" {
		t.Fatalf("runtime-only setup report = %#v", report)
	}
	current := runCLIWithEnv(t, env, "store", "current", "--json")
	if !strings.Contains(current, `"name": "team"`) || !strings.Contains(current, `"backend": "postgres"`) {
		t.Fatalf("runtime-only setup should preserve active Store, current = %s", current)
	}
	if !strings.Contains(readUpdateCalls(t, callsPath), "-o "+report.Runtime.Path+" ./cmd/agent-testbench") {
		t.Fatalf("runtime-only setup did not build runtime: %s", readUpdateCalls(t, callsPath))
	}
}

func createSetupRepo(t *testing.T) string {
	t.Helper()
	repo := t.TempDir()
	writeFile(t, filepath.Join(repo, "cmd/agent-testbench/main.go"), "package main\nfunc main() {}\n")
	writeFile(t, filepath.Join(repo, "go.mod"), "module setup-fixture\n")
	runGit(t, "", "init", "-b", "main", repo)
	runGit(t, repo, "add", ".")
	runGit(t, repo, "-c", "user.name=Open Test", "-c", "user.email=open-test@example.com", "commit", "-m", "setup fixture")
	if err := os.MkdirAll(filepath.Join(repo, ".runtime"), 0o755); err != nil {
		t.Fatalf("create runtime dir: %v", err)
	}
	return repo
}
