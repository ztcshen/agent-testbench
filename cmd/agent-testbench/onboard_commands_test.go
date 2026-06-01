package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestOnboardConfiguresStoreShellEntrypointAndSmoke(t *testing.T) {
	configHome := t.TempDir()
	repo := createSetupRepo(t)
	binDir := filepath.Join(t.TempDir(), "bin")
	storePath := filepath.Join(repo, ".runtime", "onboard.sqlite")
	env := []string{
		"AGENT_TESTBENCH_CONFIG_HOME=" + configHome,
		"AGENT_TESTBENCH_REPO=" + repo,
		"PATH=" + binDir + string(os.PathListSeparator) + os.Getenv("PATH"),
	}

	out := runCLIWithEnv(t, env,
		"onboard",
		"--repo", repo,
		"--store", "local",
		"--sqlite", storePath,
		"--build-runtime=false",
		"--install-shell",
		"--bin-dir", binDir,
		"--smoke", "commands",
		"--json",
	)

	var report struct {
		OK    bool `json:"ok"`
		Store struct {
			Name    string `json:"name"`
			Backend string `json:"backend"`
			Active  bool   `json:"active"`
		} `json:"store"`
		Runtime struct {
			Path  string `json:"path"`
			Built bool   `json:"built"`
		} `json:"runtime"`
		Shell struct {
			Installed bool   `json:"installed"`
			EntryPath string `json:"entryPath"`
			OnPath    bool   `json:"onPath"`
		} `json:"shell"`
		Checks []struct {
			Name   string `json:"name"`
			OK     bool   `json:"ok"`
			Detail string `json:"detail"`
		} `json:"checks"`
		Smoke struct {
			Mode string `json:"mode"`
			OK   bool   `json:"ok"`
		} `json:"smoke"`
		Next []string `json:"next"`
	}
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatalf("decode onboard report: %v\n%s", err, out)
	}
	if !report.OK || report.Store.Name != "local" || report.Store.Backend != "sqlite" || !report.Store.Active {
		t.Fatalf("onboard store report = %#v", report)
	}
	if report.Runtime.Built {
		t.Fatalf("onboard should honor --build-runtime=false: %#v", report.Runtime)
	}
	if !report.Shell.Installed || report.Shell.EntryPath != filepath.Join(binDir, "agent-testbench") || !report.Shell.OnPath {
		t.Fatalf("onboard shell report = %#v", report.Shell)
	}
	if report.Smoke.Mode != "commands" || !report.Smoke.OK {
		t.Fatalf("onboard smoke report = %#v", report.Smoke)
	}
	if len(report.Checks) == 0 || !strings.Contains(strings.Join(report.Next, "\n"), "agent-testbench task list") {
		t.Fatalf("onboard should include checks and task next actions: %#v", report)
	}
	linkTarget, err := os.Readlink(filepath.Join(binDir, "agent-testbench"))
	if err != nil {
		t.Fatalf("read onboard symlink: %v", err)
	}
	if linkTarget != report.Runtime.Path {
		t.Fatalf("onboard symlink target = %q want %q", linkTarget, report.Runtime.Path)
	}
	current := runCLIWithEnv(t, env, "store", "current", "--json")
	if !strings.Contains(current, `"name": "local"`) || !strings.Contains(current, `"backend": "sqlite"`) {
		t.Fatalf("onboard should activate store, current = %s", current)
	}
}

func TestOnboardRejectsUnknownSmokeMode(t *testing.T) {
	configHome := t.TempDir()
	repo := createSetupRepo(t)

	out := runCLIFailsWithEnv(t, []string{"AGENT_TESTBENCH_CONFIG_HOME=" + configHome}, "onboard", "--repo", repo, "--smoke", "kitchen-sink", "--json")

	if !strings.Contains(out, "unsupported onboard smoke mode") {
		t.Fatalf("onboard smoke error = %q", out)
	}
}

func TestOnboardDoesNotOverwriteExistingShellEntrypoint(t *testing.T) {
	configHome := t.TempDir()
	repo := createSetupRepo(t)
	binDir := filepath.Join(t.TempDir(), "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatalf("create bin dir: %v", err)
	}
	entry := filepath.Join(binDir, "agent-testbench")
	if err := os.WriteFile(entry, []byte("custom entrypoint"), 0o755); err != nil {
		t.Fatalf("write existing entrypoint: %v", err)
	}

	out := runCLIFailsWithEnv(t, []string{"AGENT_TESTBENCH_CONFIG_HOME=" + configHome},
		"onboard",
		"--repo", repo,
		"--build-runtime=false",
		"--install-shell",
		"--bin-dir", binDir,
		"--smoke", "none",
		"--json",
	)

	if !strings.Contains(out, "existing shell entrypoint") {
		t.Fatalf("onboard should reject existing entrypoint:\n%s", out)
	}
	raw, err := os.ReadFile(entry)
	if err != nil {
		t.Fatalf("read existing entrypoint: %v", err)
	}
	if string(raw) != "custom entrypoint" {
		t.Fatalf("existing entrypoint should not be overwritten, got %q", raw)
	}
}

func TestOnboardFailsWhenRequestedSmokeCheckFails(t *testing.T) {
	configHome := t.TempDir()
	repo := createSetupRepo(t)
	storePathIsDirectory := t.TempDir()

	out := runCLIFailsWithEnv(t, []string{"AGENT_TESTBENCH_CONFIG_HOME=" + configHome},
		"onboard",
		"--repo", repo,
		"--sqlite", storePathIsDirectory,
		"--build-runtime=false",
		"--smoke", "store",
		"--json",
	)

	if !strings.Contains(out, `"ok": false`) || !strings.Contains(out, `"mode": "store"`) {
		t.Fatalf("onboard should return a failing smoke report:\n%s", out)
	}
}
