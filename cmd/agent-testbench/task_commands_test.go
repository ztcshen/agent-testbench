package main

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestTaskRunRecordsHistoryAndNotification(t *testing.T) {
	storePath := filepath.Join(t.TempDir(), "tasks.sqlite")
	notifyPath := filepath.Join(t.TempDir(), "notify.jsonl")
	storeRef := "sqlite://" + storePath

	out := runCLI(t,
		"task", "run", "catalog-smoke",
		"--store", storeRef,
		"--command", "commands --filter task --json",
		"--notify-file", notifyPath,
		"--json",
	)

	var report struct {
		OK   bool `json:"ok"`
		Task struct {
			ID      string `json:"id"`
			Name    string `json:"name"`
			Status  string `json:"status"`
			Command string `json:"command"`
		} `json:"task"`
		Run struct {
			ID       string `json:"id"`
			TaskID   string `json:"taskId"`
			Status   string `json:"status"`
			ExitCode int    `json:"exitCode"`
			Output   string `json:"output"`
		} `json:"run"`
		Notify []struct {
			OK      bool   `json:"ok"`
			Channel string `json:"channel"`
			Target  string `json:"target"`
		} `json:"notify"`
	}
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatalf("decode task run report: %v\n%s", err, out)
	}
	if !report.OK || report.Task.Name != "catalog-smoke" || report.Task.Status != "active" || report.Task.Command == "" {
		t.Fatalf("task run report task = %#v", report)
	}
	if report.Run.TaskID != report.Task.ID || report.Run.Status != "passed" || report.Run.ExitCode != 0 || !strings.Contains(report.Run.Output, `"command": "task run"`) {
		t.Fatalf("task run report run = %#v", report.Run)
	}
	if len(report.Notify) != 1 || !report.Notify[0].OK || report.Notify[0].Target != notifyPath {
		t.Fatalf("task run notification report = %#v", report.Notify)
	}
	notice, err := os.ReadFile(notifyPath)
	if err != nil {
		t.Fatalf("read notification file: %v", err)
	}
	if !strings.Contains(string(notice), `"taskName":"catalog-smoke"`) || !strings.Contains(string(notice), `"status":"passed"`) {
		t.Fatalf("notification file = %s", notice)
	}

	listOut := runCLI(t, "task", "list", "--store", storeRef, "--json")
	if !strings.Contains(listOut, `"name": "catalog-smoke"`) || !strings.Contains(listOut, `"latestStatus": "passed"`) {
		t.Fatalf("task list should include latest run status:\n%s", listOut)
	}
	logsOut := runCLI(t, "task", "logs", "catalog-smoke", "--store", storeRef, "-n", "1", "--json")
	if !strings.Contains(logsOut, `"status": "passed"`) || !strings.Contains(logsOut, `"command": "commands --filter task --json"`) {
		t.Fatalf("task logs should include recorded command and status:\n%s", logsOut)
	}
}

func TestTaskRunShellExecutesSandboxTriggerCommand(t *testing.T) {
	storePath := filepath.Join(t.TempDir(), "tasks.sqlite")
	storeRef := "sqlite://" + storePath

	out := runCLI(t,
		"task", "run", "mq-trigger",
		"--store", storeRef,
		"--shell",
		"--command", "MID=CAP-ATB-$(printf 42); printf '%s\\n' \"$MID\"",
		"--json",
	)
	var report struct {
		OK   bool `json:"ok"`
		Task struct {
			Kind    string `json:"kind"`
			Command string `json:"command"`
		} `json:"task"`
		Run struct {
			Status   string `json:"status"`
			ExitCode int    `json:"exitCode"`
			Output   string `json:"output"`
		} `json:"run"`
	}
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatalf("decode shell task run report: %v\n%s", err, out)
	}
	if !report.OK || report.Task.Kind != "shell" || report.Run.Status != "passed" || report.Run.ExitCode != 0 || !strings.Contains(report.Run.Output, "CAP-ATB-42") {
		t.Fatalf("shell task run report = %#v", report)
	}

	logsOut := runCLI(t, "task", "logs", "mq-trigger", "--store", storeRef, "-n", "1", "--json")
	if !strings.Contains(logsOut, "CAP-ATB-42") || !strings.Contains(logsOut, `"command": "MID=CAP-ATB-$(printf 42); printf '%s\\n' \"$MID\""`) {
		t.Fatalf("shell task logs should retain command and output:\n%s", logsOut)
	}
}

func TestTaskScheduleWatchStopAndRootAliases(t *testing.T) {
	storePath := filepath.Join(t.TempDir(), "tasks.sqlite")
	storeRef := "sqlite://" + storePath

	scheduleOut := runCLI(t,
		"task", "schedule", "catalog-watch",
		"--store", storeRef,
		"--command", "commands --filter watch --json",
		"--interval", "15m",
		"--notify-file", filepath.Join(t.TempDir(), "watch.jsonl"),
		"--json",
	)
	if !strings.Contains(scheduleOut, `"schedule": "interval:15m"`) || !strings.Contains(scheduleOut, `"status": "scheduled"`) {
		t.Fatalf("task schedule output = %s", scheduleOut)
	}

	watchOut := runCLI(t,
		"watch", "catalog-watch",
		"--store", storeRef,
		"--command", "commands --filter watch --json",
		"--interval", "1ms",
		"--limit", "2",
		"--until", "success",
		"--json",
	)
	if !strings.Contains(watchOut, `"attempts": 1`) || !strings.Contains(watchOut, `"status": "passed"`) {
		t.Fatalf("watch alias output = %s", watchOut)
	}

	statusOut := runCLI(t, "task", "status", "catalog-watch", "--store", storeRef, "--json")
	if !strings.Contains(statusOut, `"runCount": 1`) || !strings.Contains(statusOut, `"latestStatus": "passed"`) {
		t.Fatalf("task status output = %s", statusOut)
	}
	stopOut := runCLI(t, "task", "stop", "catalog-watch", "--store", storeRef, "--json")
	if !strings.Contains(stopOut, `"status": "paused"`) {
		t.Fatalf("task stop output = %s", stopOut)
	}
}

func TestTaskIDsStayUniqueWhenLongNamesSharePrefix(t *testing.T) {
	storePath := filepath.Join(t.TempDir(), "tasks.sqlite")
	storeRef := "sqlite://" + storePath
	firstName := strings.Repeat("shared-prefix-", 8) + "first"
	secondName := strings.Repeat("shared-prefix-", 8) + "second"

	firstOut := runCLI(t, "task", "schedule", firstName, "--store", storeRef, "--command", "commands --json", "--interval", "1h", "--json")
	secondOut := runCLI(t, "task", "schedule", secondName, "--store", storeRef, "--command", "commands --json", "--interval", "1h", "--json")

	firstID := taskIDFromJSONReport(t, firstOut)
	secondID := taskIDFromJSONReport(t, secondOut)
	if firstID == secondID {
		t.Fatalf("long task names should not collide: %q", firstID)
	}
	listOut := runCLI(t, "task", "list", "--store", storeRef, "--json")
	if !strings.Contains(listOut, firstName) || !strings.Contains(listOut, secondName) {
		t.Fatalf("task list should retain both long-name tasks:\n%s", listOut)
	}
}

func TestTaskRunFailsWhenNotificationTargetFails(t *testing.T) {
	storePath := filepath.Join(t.TempDir(), "tasks.sqlite")
	storeRef := "sqlite://" + storePath
	blocker := filepath.Join(t.TempDir(), "blocker")
	if err := os.WriteFile(blocker, []byte("not a directory"), 0o644); err != nil {
		t.Fatalf("write blocker file: %v", err)
	}
	out := runCLIFails(t,
		"task", "run", "notify-fail",
		"--store", storeRef,
		"--command", "commands --filter task --json",
		"--notify-file", filepath.Join(blocker, "notify.jsonl"),
		"--json",
	)
	if !strings.Contains(out, `"ok": false`) || !strings.Contains(out, `"channel": "file"`) {
		t.Fatalf("task run should surface notification failure:\n%s", out)
	}
}

func TestTaskWatchStopsWhenTaskIsPausedExternally(t *testing.T) {
	storePath := filepath.Join(t.TempDir(), "tasks.sqlite")
	storeRef := "sqlite://" + storePath

	out := runCLI(t,
		"watch", "stop-me",
		"--store", storeRef,
		"--command", "task stop stop-me --store "+storeRef+" --json",
		"--interval", "1ms",
		"--limit", "3",
		"--until", "always",
		"--json",
	)
	if !strings.Contains(out, `"attempts": 1`) || !strings.Contains(out, `"status": "paused"`) {
		t.Fatalf("watch should stop after external pause:\n%s", out)
	}
}

func TestNotificationWebhookClientHasTimeout(t *testing.T) {
	if notificationHTTPClient == nil {
		t.Fatal("notification HTTP client is nil")
	}
	if notificationHTTPClient == http.DefaultClient || notificationHTTPClient.Timeout < time.Second {
		t.Fatalf("notification HTTP client should have a bounded timeout, got %#v", notificationHTTPClient)
	}
}

func TestNotifyTestWritesJSONLine(t *testing.T) {
	notifyPath := filepath.Join(t.TempDir(), "notify.jsonl")

	out := runCLI(t, "notify", "test", "--file", notifyPath, "--message", "hello operator", "--json")

	if !strings.Contains(out, `"ok": true`) || !strings.Contains(out, `"channel": "file"`) {
		t.Fatalf("notify test output = %s", out)
	}
	raw, err := os.ReadFile(notifyPath)
	if err != nil {
		t.Fatalf("read notify file: %v", err)
	}
	if !strings.Contains(string(raw), `"message":"hello operator"`) || !strings.Contains(string(raw), `"status":"test"`) {
		t.Fatalf("notify file = %s", raw)
	}
}

func taskIDFromJSONReport(t *testing.T, raw string) string {
	t.Helper()
	var report struct {
		Task struct {
			ID string `json:"id"`
		} `json:"task"`
	}
	if err := json.Unmarshal([]byte(raw), &report); err != nil {
		t.Fatalf("decode task report: %v\n%s", err, raw)
	}
	return report.Task.ID
}

func TestCommandCatalogIncludesOnboardTaskWatchAndNotify(t *testing.T) {
	out := runCLI(t, "commands", "--all", "--filter", "task", "--json")
	for _, want := range []string{`"command": "task run"`, `"command": "task schedule"`, `"command": "task list"`} {
		if !strings.Contains(out, want) {
			t.Fatalf("command catalog missing %s:\n%s", want, out)
		}
	}
	helpOut := runCLI(t)
	for _, want := range []string{
		"agent-testbench onboard",
		"agent-testbench task run NAME --command",
		"agent-testbench watch NAME --command",
		"agent-testbench notify test",
	} {
		if !strings.Contains(helpOut, want) {
			t.Fatalf("help missing %q:\n%s", want, helpOut)
		}
	}
}
