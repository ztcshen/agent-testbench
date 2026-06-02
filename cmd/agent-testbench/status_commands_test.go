package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
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

func TestStatusAndDoctorWarnWhenRuntimeBinaryPredatesHead(t *testing.T) {
	repo := createSetupRepo(t)
	oldRevision := strings.TrimSpace(runGit(t, repo, "rev-parse", "HEAD"))
	runtimePath := filepath.Join(repo, ".runtime", "bin", "agent-testbench")
	writeRuntimeVersionScript(t, runtimePath, oldRevision)
	if err := os.Chmod(runtimePath, 0o755); err != nil {
		t.Fatalf("chmod runtime: %v", err)
	}
	old := time.Now().Add(-24 * time.Hour)
	if err := os.Chtimes(runtimePath, old, old); err != nil {
		t.Fatalf("make runtime stale: %v", err)
	}
	writeFile(t, filepath.Join(repo, "README.md"), "# newer source\n")
	runGit(t, repo, "add", "README.md")
	runGit(t, repo, "-c", "user.name=Open Test", "-c", "user.email=open-test@example.com", "commit", "-m", "newer source")
	env := []string{"AGENT_TESTBENCH_CONFIG_HOME=" + t.TempDir(), "AGENT_TESTBENCH_REPO=" + repo}
	statusOut := runCLIWithEnv(t, env, "status", "--json")
	var statusReport struct {
		Runtime struct {
			Exists           bool   `json:"exists"`
			Executable       bool   `json:"executable"`
			Fresh            bool   `json:"fresh"`
			StaleReason      string `json:"staleReason"`
			BuildRevision    string `json:"buildRevision"`
			BinaryModifiedAt string `json:"binaryModifiedAt"`
			SourceCommitAt   string `json:"sourceCommitAt"`
			RepairCommand    string `json:"repairCommand"`
		} `json:"runtime"`
		Next []string `json:"next"`
	}
	if err := json.Unmarshal([]byte(statusOut), &statusReport); err != nil {
		t.Fatalf("decode stale runtime status: %v\n%s", err, statusOut)
	}
	if !statusReport.Runtime.Exists || !statusReport.Runtime.Executable || statusReport.Runtime.Fresh || !strings.Contains(statusReport.Runtime.StaleReason, "different git revision") || statusReport.Runtime.BuildRevision != oldRevision {
		t.Fatalf("status should report stale runtime: %#v", statusReport.Runtime)
	}
	if statusReport.Runtime.BinaryModifiedAt == "" || statusReport.Runtime.SourceCommitAt == "" {
		t.Fatalf("status should include runtime/source timestamps: %#v", statusReport.Runtime)
	}
	wantRepair := statusReport.Runtime.RepairCommand
	if !strings.Contains(wantRepair, "/bin/agent-testbench.sh' setup --repo ") || !strings.Contains(wantRepair, "--build-runtime --runtime-only") || !stringSliceContains(statusReport.Next, wantRepair) {
		t.Fatalf("status should suggest runtime rebuild: %#v", statusReport.Next)
	}

	doctorOut := runCLIWithEnv(t, env, "doctor", "--json")
	var doctorReport struct {
		Checks []struct {
			Code   string `json:"code"`
			OK     bool   `json:"ok"`
			Detail string `json:"detail"`
			Fix    string `json:"fix"`
		} `json:"checks"`
	}
	if err := json.Unmarshal([]byte(doctorOut), &doctorReport); err != nil {
		t.Fatalf("decode stale runtime doctor: %v\n%s", err, doctorOut)
	}
	for _, check := range doctorReport.Checks {
		if check.Code != "runtime.fresh" {
			continue
		}
		if check.OK || !strings.Contains(check.Detail, "different git revision") || check.Fix != wantRepair {
			t.Fatalf("runtime freshness check = %#v", check)
		}
		return
	}
	t.Fatalf("doctor report missing runtime.fresh check: %#v", doctorReport.Checks)
}

func TestStatusWarnsWhenRuntimeBuildRevisionDiffersFromHead(t *testing.T) {
	repo := createSetupRepo(t)
	head := strings.TrimSpace(runGit(t, repo, "rev-parse", "HEAD"))
	runtimePath := filepath.Join(repo, ".runtime", "bin", "agent-testbench")
	writeRuntimeVersionScript(t, runtimePath, "ffffffffffffffffffffffffffffffffffffffff")
	if err := os.Chmod(runtimePath, 0o755); err != nil {
		t.Fatalf("chmod runtime: %v", err)
	}
	future := time.Now().Add(24 * time.Hour)
	if err := os.Chtimes(runtimePath, future, future); err != nil {
		t.Fatalf("make runtime mtime newer than head: %v", err)
	}

	out := runCLIWithEnv(t, []string{"AGENT_TESTBENCH_CONFIG_HOME=" + t.TempDir(), "AGENT_TESTBENCH_REPO=" + repo}, "status", "--json")
	var report struct {
		Runtime struct {
			Fresh          bool   `json:"fresh"`
			BuildRevision  string `json:"buildRevision"`
			SourceRevision string `json:"sourceRevision"`
			StaleReason    string `json:"staleReason"`
		} `json:"runtime"`
	}
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatalf("decode different revision status: %v\n%s", err, out)
	}
	if report.Runtime.Fresh || report.Runtime.SourceRevision != head || !strings.Contains(report.Runtime.StaleReason, "different git revision") {
		t.Fatalf("status should reject mismatched runtime revision: %#v", report.Runtime)
	}
}

func TestStatusWarnsWhenRuntimeBuildRevisionUnavailable(t *testing.T) {
	repo := createSetupRepo(t)
	runtimePath := filepath.Join(repo, ".runtime", "bin", "agent-testbench")
	writeFile(t, runtimePath, "#!/usr/bin/env sh\nexit 0\n")
	if err := os.Chmod(runtimePath, 0o755); err != nil {
		t.Fatalf("chmod runtime: %v", err)
	}
	future := time.Now().Add(24 * time.Hour)
	if err := os.Chtimes(runtimePath, future, future); err != nil {
		t.Fatalf("make runtime mtime newer than head: %v", err)
	}

	out := runCLIWithEnv(t, []string{"AGENT_TESTBENCH_CONFIG_HOME=" + t.TempDir(), "AGENT_TESTBENCH_REPO=" + repo}, "status", "--json")
	var report struct {
		Runtime struct {
			Fresh       bool   `json:"fresh"`
			StaleReason string `json:"staleReason"`
		} `json:"runtime"`
	}
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatalf("decode missing revision status: %v\n%s", err, out)
	}
	if report.Runtime.Fresh || !strings.Contains(report.Runtime.StaleReason, "does not report a build revision") {
		t.Fatalf("status should reject runtime without build revision: %#v", report.Runtime)
	}
}

func writeRuntimeVersionScript(t *testing.T, path string, revision string) {
	t.Helper()
	writeFile(t, path, "#!/usr/bin/env sh\nif [ \"$1\" = \"version\" ] && [ \"$2\" = \"--json\" ]; then\n  printf '{\"version\":\"0.1.0\",\"buildRevision\":\""+revision+"\"}\\n'\n  exit 0\nfi\nexit 0\n")
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
