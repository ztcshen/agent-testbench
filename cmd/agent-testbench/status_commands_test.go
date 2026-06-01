package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStatusReportsRepoRuntimeAndStoreSummary(t *testing.T) {
	configHome := t.TempDir()
	repo := createSetupRepo(t)
	env := []string{"AGENT_TESTBENCH_CONFIG_HOME=" + configHome, "AGENT_TESTBENCH_REPO=" + repo}
	out := runCLIWithEnv(t, env, "status", "--json")

	var report struct {
		OK      bool   `json:"ok"`
		Version string `json:"version"`
		Repo    struct {
			Path     string `json:"path"`
			Branch   string `json:"branch"`
			Revision string `json:"revision"`
		} `json:"repo"`
		Runtime struct {
			Path   string `json:"path"`
			Exists bool   `json:"exists"`
		} `json:"runtime"`
		Store struct {
			Configured bool `json:"configured"`
		} `json:"store"`
		Next []string `json:"next"`
	}
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatalf("decode status report: %v\n%s", err, out)
	}
	if !report.OK || report.Version == "" || report.Repo.Path == "" || report.Repo.Revision == "" {
		t.Fatalf("status report missing repo basics: %#v", report)
	}
	if report.Runtime.Path == "" || report.Runtime.Exists {
		t.Fatalf("status runtime should report the default path without requiring it to exist: %#v", report.Runtime)
	}
	if report.Store.Configured {
		t.Fatalf("status should report no active store with isolated config home: %#v", report.Store)
	}
	if !stringSliceContains(report.Next, "agent-testbench store config set NAME --url sqlite://PATH") {
		t.Fatalf("status should include first-time store setup next action: %#v", report.Next)
	}

	textOut := runCLIWithEnv(t, env, "status")
	if !strings.Contains(textOut, "AgentTestBench Status") || !strings.Contains(textOut, "Next") {
		t.Fatalf("status text output should be readable:\n%s", textOut)
	}
}

func TestDoctorReportsMissingActiveStoreWithoutFailing(t *testing.T) {
	configHome := t.TempDir()
	out := runCLIWithEnv(t, []string{"AGENT_TESTBENCH_CONFIG_HOME=" + configHome}, "doctor", "--json")

	var report struct {
		OK     bool `json:"ok"`
		Checks []struct {
			Name   string `json:"name"`
			OK     bool   `json:"ok"`
			Detail string `json:"detail"`
		} `json:"checks"`
	}
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatalf("decode doctor report: %v\n%s", err, out)
	}
	foundActiveStore := false
	for _, check := range report.Checks {
		if check.Name == "active-store" {
			foundActiveStore = true
			if check.OK || !strings.Contains(check.Detail, "store config set") {
				t.Fatalf("active-store doctor check = %#v", check)
			}
		}
	}
	if !foundActiveStore {
		t.Fatalf("doctor report missing active-store check: %#v", report.Checks)
	}

	textOut := runCLIWithEnv(t, []string{"AGENT_TESTBENCH_CONFIG_HOME=" + configHome}, "doctor")
	if !strings.Contains(textOut, "AgentTestBench Doctor") || !strings.Contains(textOut, "active-store") {
		t.Fatalf("doctor text output should include checks:\n%s", textOut)
	}
}

func TestDoctorFixCreatesLocalStoreAndRuntimeDirectory(t *testing.T) {
	configHome := t.TempDir()
	repo := t.TempDir()
	out := runCLIWithEnv(t, []string{"AGENT_TESTBENCH_CONFIG_HOME=" + configHome, "AGENT_TESTBENCH_REPO=" + repo}, "doctor", "--fix", "--json")

	var report struct {
		OK     bool `json:"ok"`
		Checks []struct {
			Name  string `json:"name"`
			Code  string `json:"code"`
			OK    bool   `json:"ok"`
			Fixed bool   `json:"fixed"`
		} `json:"checks"`
	}
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatalf("decode doctor fix report: %v\n%s", err, out)
	}
	foundStore := false
	foundRuntime := false
	for _, check := range report.Checks {
		if check.Name == "active-store" {
			foundStore = true
			if !check.Fixed || check.Code == "" {
				t.Fatalf("active-store should be fixed with stable code: %#v", check)
			}
		}
		if check.Name == "runtime-directory" {
			foundRuntime = true
			if !check.OK || !check.Fixed || check.Code == "" {
				t.Fatalf("runtime-directory should be fixed: %#v", check)
			}
		}
	}
	if !foundStore || !foundRuntime {
		t.Fatalf("doctor fix missing store/runtime checks: %#v", report.Checks)
	}
	statusOut := runCLIWithEnv(t, []string{"AGENT_TESTBENCH_CONFIG_HOME=" + configHome, "AGENT_TESTBENCH_REPO=" + repo}, "status", "--json")
	if !strings.Contains(statusOut, `"configured": true`) || !strings.Contains(statusOut, `"backend": "sqlite"`) {
		t.Fatalf("doctor --fix should configure local sqlite store:\n%s", statusOut)
	}
}

func TestDoctorWarnsWhenShellEntrypointIsStale(t *testing.T) {
	remoteRepo := createBareGitRepoWithFiles(t, "main", map[string]string{
		"cmd/agent-testbench/main.go": "package main\nfunc main() {}\n",
		"go.mod":                      "module status-fixture\n",
	})
	checkout := cloneUpdateFixture(t, remoteRepo)
	runtimePath := filepath.Join(checkout, ".runtime", "bin", "agent-testbench")
	writeFile(t, runtimePath, "#!/usr/bin/env sh\nexit 0\n")
	if err := os.Chmod(runtimePath, 0o755); err != nil {
		t.Fatalf("chmod runtime: %v", err)
	}
	staleDir := t.TempDir()
	stalePath := filepath.Join(staleDir, "agent-testbench")
	writeFile(t, stalePath, "#!/usr/bin/env sh\nexit 0\n")
	if err := os.Chmod(stalePath, 0o755); err != nil {
		t.Fatalf("chmod stale binary: %v", err)
	}

	out := runCLIWithEnv(t, []string{
		"AGENT_TESTBENCH_CONFIG_HOME=" + t.TempDir(),
		"AGENT_TESTBENCH_REPO=" + checkout,
		"PATH=" + staleDir + string(os.PathListSeparator) + os.Getenv("PATH"),
	}, "doctor", "--json")
	var report struct {
		Checks []struct {
			Name     string `json:"name"`
			Code     string `json:"code"`
			OK       bool   `json:"ok"`
			Optional bool   `json:"optional"`
			Detail   string `json:"detail"`
			Fix      string `json:"fix"`
		} `json:"checks"`
	}
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatalf("decode doctor report: %v\n%s", err, out)
	}
	for _, check := range report.Checks {
		if check.Code != "runtime.shell-entrypoint" {
			continue
		}
		if check.OK || !check.Optional || !strings.Contains(check.Detail, stalePath) || !strings.Contains(check.Fix, ".runtime/bin") {
			t.Fatalf("shell entrypoint check = %#v", check)
		}
		return
	}
	t.Fatalf("doctor report missing runtime.shell-entrypoint check: %#v", report.Checks)
}

func TestStatusDeepIncludesStoreSchema(t *testing.T) {
	configHome := t.TempDir()
	storePath := t.TempDir() + "/status.sqlite"
	runCLIWithEnv(t, []string{"AGENT_TESTBENCH_CONFIG_HOME=" + configHome}, "store", "config", "set", "local", "--url", "sqlite://"+storePath)
	runCLIWithEnv(t, []string{"AGENT_TESTBENCH_CONFIG_HOME=" + configHome}, "store", "use", "local")
	runCLIWithEnv(t, []string{"AGENT_TESTBENCH_CONFIG_HOME=" + configHome}, "store", "upgrade")

	out := runCLIWithEnv(t, []string{"AGENT_TESTBENCH_CONFIG_HOME=" + configHome}, "status", "--deep", "--json")
	var report struct {
		Store struct {
			Schema struct {
				OK            bool `json:"ok"`
				TargetVersion int  `json:"targetVersion"`
				Pending       int  `json:"pending"`
			} `json:"schema"`
		} `json:"store"`
	}
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatalf("decode deep status report: %v\n%s", err, out)
	}
	if !report.Store.Schema.OK || report.Store.Schema.TargetVersion == 0 || report.Store.Schema.Pending != 0 {
		t.Fatalf("deep status should include sqlite schema status: %#v", report.Store.Schema)
	}
}

func TestStatusDeepDoesNotCreateMissingSQLiteStore(t *testing.T) {
	configHome := t.TempDir()
	storePath := filepath.Join(t.TempDir(), "missing", "status.sqlite")
	runCLIWithEnv(t, []string{"AGENT_TESTBENCH_CONFIG_HOME=" + configHome}, "store", "config", "set", "local", "--url", "sqlite://"+storePath)
	runCLIWithEnv(t, []string{"AGENT_TESTBENCH_CONFIG_HOME=" + configHome}, "store", "use", "local")

	out := runCLIWithEnv(t, []string{"AGENT_TESTBENCH_CONFIG_HOME=" + configHome}, "status", "--deep", "--json")
	var report struct {
		Store struct {
			Schema struct {
				OK    bool   `json:"ok"`
				Path  string `json:"path"`
				Error string `json:"error"`
			} `json:"schema"`
		} `json:"store"`
	}
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatalf("decode deep status report: %v\n%s", err, out)
	}
	if report.Store.Schema.OK || report.Store.Schema.Path != storePath || !strings.Contains(report.Store.Schema.Error, "does not exist") {
		t.Fatalf("deep status should report missing sqlite store without opening it: %#v", report.Store.Schema)
	}
	if _, err := os.Stat(storePath); !os.IsNotExist(err) {
		t.Fatalf("status --deep should not create sqlite store file, stat err = %v", err)
	}
	if _, err := os.Stat(filepath.Dir(storePath)); !os.IsNotExist(err) {
		t.Fatalf("status --deep should not create sqlite store directory, stat err = %v", err)
	}
}

func stringSliceContains(items []string, want string) bool {
	for _, item := range items {
		if item == want {
			return true
		}
	}
	return false
}
