package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEnvironmentRegisterStoresCommandLifecycleContract(t *testing.T) {
	storePath := filepath.Join(t.TempDir(), "store.sqlite")
	storeRef := "sqlite://" + storePath
	runCLI(t, "environment", "register",
		"--store", storeRef,
		"--id", "env.command.contract",
		"--start-command", "touch running.marker",
		"--status-command", "test -f running.marker",
		"--stop-command", "rm -f running.marker",
		"--health-command", "test -f running.marker",
		"--verification-workflow", "workflow.command-contract",
	)

	runtime, err := openStore(context.Background(), storeRef)
	if err != nil {
		t.Fatalf("open Store: %v", err)
	}
	defer closeCLIStore(runtime)
	env, err := runtime.GetEnvironment(context.Background(), "env.command.contract")
	if err != nil {
		t.Fatalf("get environment: %v", err)
	}
	compose := jsonObjectString(env.ComposeJSON)
	if compose["startCommand"] != "touch running.marker" ||
		compose["statusCommand"] != "test -f running.marker" ||
		compose["stopCommand"] != "rm -f running.marker" {
		t.Fatalf("command lifecycle contract was not stored: %#v", compose)
	}
}

func TestEnvironmentRegisterRejectsLifecycleCommandsWithoutStartCommand(t *testing.T) {
	out := runCLIFails(t, "environment", "register",
		"--store", "sqlite://"+filepath.Join(t.TempDir(), "store.sqlite"),
		"--id", "env.command.invalid",
		"--status-command", "true",
		"--stop-command", "true",
		"--verification-workflow", "workflow.command-contract",
	)
	if !strings.Contains(out, "require --start-command") {
		t.Fatalf("invalid lifecycle contract error = %q", out)
	}
}

func TestEnvironmentCommandLifecycleUsesExternalCommandsAndRedactsOutput(t *testing.T) {
	storePath := filepath.Join(t.TempDir(), "store.sqlite")
	storeRef := "sqlite://" + storePath
	workspace := filepath.Join(t.TempDir(), "runtime")
	secret := "sentinel-command-lifecycle-secret"
	runCLI(t, "environment", "register",
		"--store", storeRef,
		"--id", "env.command.lifecycle",
		"--start-command", "touch running.marker",
		"--status-command", "printf '%s\\n' '"+secret+"'; test -f running.marker",
		"--stop-command", "printf '%s\\n' '"+secret+"'; rm -f running.marker",
		"--health-command", "test -f running.marker",
		"--verification-workflow", "workflow.command-lifecycle",
	)

	restoreOut := runCLI(t, "environment", "restore",
		"--store", storeRef,
		"--workspace", workspace,
		"--execute",
		"--health-timeout-seconds", "1",
		"--json",
		"env.command.lifecycle",
	)
	assertEnvironmentOutputHasNoSecrets(t, restoreOut, secret)
	if _, err := os.Stat(filepath.Join(workspace, "running.marker")); err != nil {
		t.Fatalf("startCommand did not create runtime marker: %v", err)
	}

	statusOut := runCLI(t, "environment", "status",
		"--store", storeRef,
		"--workspace", workspace,
		"--json",
		"env.command.lifecycle",
	)
	assertEnvironmentOutputHasNoSecrets(t, statusOut, secret)
	status := decodeCommandLifecycleStatus(t, statusOut)
	if !status.OK || status.Docker.Action != "inspect-status-command" ||
		status.Docker.Summary.Ready != 1 || status.Docker.Summary.Failed != 0 ||
		len(status.Docker.HealthChecks) != 1 || !status.Docker.HealthChecks[0].OK {
		t.Fatalf("command environment status = %#v", status)
	}

	stopOut := runCLI(t, "environment", "stop",
		"--store", storeRef,
		"--workspace", workspace,
		"--json",
		"env.command.lifecycle",
	)
	assertEnvironmentOutputHasNoSecrets(t, stopOut, secret)
	var stopped struct {
		OK     bool `json:"ok"`
		Docker struct {
			Action   string   `json:"action"`
			Command  []string `json:"command"`
			Output   string   `json:"output"`
			ExitCode int      `json:"exitCode"`
		} `json:"docker"`
	}
	if err := json.Unmarshal([]byte(stopOut), &stopped); err != nil {
		t.Fatalf("decode command stop report: %v\n%s", err, stopOut)
	}
	if !stopped.OK || stopped.Docker.Action != environmentStopActionCommand ||
		len(stopped.Docker.Command) != 0 || stopped.Docker.Output != "" || stopped.Docker.ExitCode != 0 {
		t.Fatalf("command environment stop = %#v", stopped)
	}
	if _, err := os.Stat(filepath.Join(workspace, "running.marker")); !os.IsNotExist(err) {
		t.Fatalf("stopCommand did not remove runtime marker: %v", err)
	}

	failedStatusOut := runCLIFails(t, "environment", "status",
		"--store", storeRef,
		"--workspace", workspace,
		"--json",
		"env.command.lifecycle",
	)
	assertEnvironmentOutputHasNoSecrets(t, failedStatusOut, secret)
	failedStatus := decodeCommandLifecycleStatus(t, extractJSONObject(t, failedStatusOut))
	if failedStatus.OK || failedStatus.Docker.Action != "inspect-status-command" ||
		failedStatus.Docker.Summary.Failed != 1 || !strings.Contains(failedStatus.Docker.Error, "exited with code") {
		t.Fatalf("stopped command environment status = %#v", failedStatus)
	}

	runtime, err := openStore(context.Background(), storeRef)
	if err != nil {
		t.Fatalf("open Store: %v", err)
	}
	defer closeCLIStore(runtime)
	env, err := runtime.GetEnvironment(context.Background(), "env.command.lifecycle")
	if err != nil {
		t.Fatalf("get environment: %v", err)
	}
	assertEnvironmentOutputHasNoSecrets(t, env.SummaryJSON, secret)
}

func TestEnvironmentCommandStatusFallsBackToRecordedHealthProbe(t *testing.T) {
	storePath := filepath.Join(t.TempDir(), "store.sqlite")
	storeRef := "sqlite://" + storePath
	workspace := filepath.Join(t.TempDir(), "runtime")
	runCLI(t, "environment", "register",
		"--store", storeRef,
		"--id", "env.command.health",
		"--start-command", "touch running.marker",
		"--stop-command", "rm -f running.marker",
		"--health-command", "test -f running.marker",
		"--verification-workflow", "workflow.command-health",
	)
	runCLI(t, "environment", "restore",
		"--store", storeRef,
		"--workspace", workspace,
		"--execute",
		"--health-timeout-seconds", "1",
		"--json",
		"env.command.health",
	)

	statusOut := runCLI(t, "environment", "status",
		"--store", storeRef,
		"--workspace", workspace,
		"--health-timeout-seconds", "1",
		"--json",
		"env.command.health",
	)
	status := decodeCommandLifecycleStatus(t, statusOut)
	if !status.OK || status.Docker.Action != "inspect-health-probes" ||
		status.Docker.Summary.Ready != 1 || len(status.Docker.HealthChecks) != 1 ||
		status.Docker.HealthChecks[0].Kind != "command" || status.Docker.HealthChecks[0].Command != "" {
		t.Fatalf("command health probe status = %#v", status)
	}
}

func TestEnvironmentCommandStopRequiresRecordedStopCommand(t *testing.T) {
	storeRef := "sqlite://" + filepath.Join(t.TempDir(), "store.sqlite")
	runCLI(t, "environment", "register",
		"--store", storeRef,
		"--id", "env.command.no-stop",
		"--start-command", "true",
		"--health-command", "true",
		"--verification-workflow", "workflow.command-no-stop",
	)
	out := runCLIFails(t, "environment", "stop",
		"--store", storeRef,
		"--workspace", t.TempDir(),
		"--json",
		"env.command.no-stop",
	)
	if !strings.Contains(out, "missing-stop-command") || !strings.Contains(out, "requires a recorded stopCommand") {
		t.Fatalf("missing stopCommand error = %q", out)
	}
}

type commandLifecycleStatusReport struct {
	OK     bool `json:"ok"`
	Docker struct {
		Action       string                         `json:"action"`
		Error        string                         `json:"error"`
		Summary      environmentStatusHealthSummary `json:"summary"`
		HealthChecks []struct {
			Kind    string `json:"kind"`
			Command string `json:"command"`
			OK      bool   `json:"ok"`
		} `json:"healthChecks"`
	} `json:"docker"`
}

func decodeCommandLifecycleStatus(t *testing.T, output string) commandLifecycleStatusReport {
	t.Helper()
	var report commandLifecycleStatusReport
	if err := json.Unmarshal([]byte(output), &report); err != nil {
		t.Fatalf("decode command lifecycle status: %v\n%s", err, output)
	}
	return report
}
