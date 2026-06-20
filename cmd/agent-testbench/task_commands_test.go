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
		"task", "watch", "catalog-watch",
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
		"task", "watch", "stop-me",
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

type taskCatalogTestReport struct {
	OK    bool `json:"ok"`
	Count int  `json:"count"`
	Tasks []struct {
		ID             string   `json:"id"`
		Goal           string   `json:"goal"`
		Tags           []string `json:"tags"`
		RequiredInputs []struct {
			Name     string `json:"name"`
			Flag     string `json:"flag"`
			Required bool   `json:"required"`
		} `json:"requiredInputs"`
		Steps []struct {
			ID       string `json:"id"`
			Command  string `json:"command"`
			ReadOnly bool   `json:"readOnly"`
		} `json:"steps"`
	} `json:"tasks"`
}

func TestTaskCatalogExposesBuiltInTasks(t *testing.T) {
	catalog := decodeTaskCatalogTestReport(t, runCLI(t, "task", "catalog", "--json"))
	if !catalog.OK || catalog.Count < 4 {
		t.Fatalf("task catalog should expose built-in tasks: %#v", catalog)
	}
	tasks := taskCatalogEntriesByID(catalog)
	maintain := tasks["map-maintain"]
	if maintain.Goal == "" || !stringSliceContains(maintain.Tags, "maintain map") {
		t.Fatalf("map-maintain catalog entry = %#v", maintain)
	}
	if len(maintain.RequiredInputs) == 0 || maintain.RequiredInputs[0].Flag != "--map" || len(maintain.Steps) < 3 {
		t.Fatalf("map-maintain catalog inputs/steps = %#v", maintain)
	}
	if _, ok := tasks["map-execute"]; !ok {
		t.Fatalf("task catalog missing map-execute: %#v", tasks)
	}
}

func TestTaskSuggestBuiltInTasks(t *testing.T) {
	suggestOut := runCLI(t, "task", "suggest", "--goal", "maintain map", "--json")
	var suggest struct {
		OK          bool `json:"ok"`
		Suggestions []struct {
			ID     string `json:"id"`
			Reason string `json:"reason"`
		} `json:"suggestions"`
	}
	if err := json.Unmarshal([]byte(suggestOut), &suggest); err != nil {
		t.Fatalf("decode task suggest: %v\n%s", err, suggestOut)
	}
	if !suggest.OK || len(suggest.Suggestions) == 0 || suggest.Suggestions[0].ID != "map-maintain" || suggest.Suggestions[0].Reason == "" {
		t.Fatalf("maintain map suggestion = %#v", suggest)
	}
	executeSuggest := runCLI(t, "task", "suggest", "--goal", "execute map", "--json")
	if !strings.Contains(executeSuggest, `"id": "map-execute"`) {
		t.Fatalf("execute map suggestion should include map-execute:\n%s", executeSuggest)
	}
}

func TestTaskPlanBuiltInMapMaintain(t *testing.T) {
	planOut := runCLI(t, "task", "plan", "map-maintain", "--map", "map.demo", "--json")
	var plan struct {
		OK      bool              `json:"ok"`
		DryRun  bool              `json:"dryRun"`
		Inputs  map[string]string `json:"inputs"`
		Missing []string          `json:"missing"`
		Task    struct {
			ID string `json:"id"`
		} `json:"task"`
		Steps []struct {
			ID      string `json:"id"`
			Command string `json:"command"`
			Execute bool   `json:"execute"`
		} `json:"steps"`
	}
	if err := json.Unmarshal([]byte(planOut), &plan); err != nil {
		t.Fatalf("decode task plan: %v\n%s", err, planOut)
	}
	if !plan.OK || plan.Task.ID != "map-maintain" || plan.Inputs["map"] != "map.demo" || len(plan.Missing) != 0 || len(plan.Steps) < 4 {
		t.Fatalf("map-maintain plan = %#v", plan)
	}
	wantCommands := []string{
		"map doctor --map map.demo",
		"map inspect --view coverage --map map.demo",
		"map diff --map map.demo --from published",
		"map validation list --map map.demo",
	}
	for i, want := range wantCommands {
		if plan.Steps[i].Command != want || plan.Steps[i].Execute {
			t.Fatalf("plan step %d = %#v, want command %q and execute=false", i, plan.Steps[i], want)
		}
	}

	missingOut := runCLIFails(t, "task", "plan", "map-maintain", "--json")
	if !strings.Contains(missingOut, "--map") || !strings.Contains(missingOut, "missing") {
		t.Fatalf("task plan should explain missing inputs:\n%s", missingOut)
	}
}

func decodeTaskCatalogTestReport(t *testing.T, raw string) taskCatalogTestReport {
	t.Helper()
	var report taskCatalogTestReport
	if err := json.Unmarshal([]byte(raw), &report); err != nil {
		t.Fatalf("decode task catalog: %v\n%s", err, raw)
	}
	return report
}

func taskCatalogEntriesByID(report taskCatalogTestReport) map[string]struct {
	Goal           string
	Tags           []string
	RequiredInputs []struct {
		Name     string `json:"name"`
		Flag     string `json:"flag"`
		Required bool   `json:"required"`
	}
	Steps []struct {
		ID       string `json:"id"`
		Command  string `json:"command"`
		ReadOnly bool   `json:"readOnly"`
	}
} {
	out := map[string]struct {
		Goal           string
		Tags           []string
		RequiredInputs []struct {
			Name     string `json:"name"`
			Flag     string `json:"flag"`
			Required bool   `json:"required"`
		}
		Steps []struct {
			ID       string `json:"id"`
			Command  string `json:"command"`
			ReadOnly bool   `json:"readOnly"`
		}
	}{}
	for _, task := range report.Tasks {
		out[task.ID] = struct {
			Goal           string
			Tags           []string
			RequiredInputs []struct {
				Name     string `json:"name"`
				Flag     string `json:"flag"`
				Required bool   `json:"required"`
			}
			Steps []struct {
				ID       string `json:"id"`
				Command  string `json:"command"`
				ReadOnly bool   `json:"readOnly"`
			}
		}{Goal: task.Goal, Tags: task.Tags, RequiredInputs: task.RequiredInputs, Steps: task.Steps}
	}
	return out
}

func TestTaskRunBuiltInMapTasksDryRun(t *testing.T) {
	out := runCLI(t, "task", "run", "map-maintain", "--map", "map.demo", "--dry-run", "--json")
	var report struct {
		OK     bool `json:"ok"`
		DryRun bool `json:"dryRun"`
		Task   struct {
			ID string `json:"id"`
		} `json:"task"`
		Steps []struct {
			Command  string `json:"command"`
			Execute  bool   `json:"execute"`
			ReadOnly bool   `json:"readOnly"`
		} `json:"steps"`
	}
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatalf("decode task run dry-run: %v\n%s", err, out)
	}
	if !report.OK || !report.DryRun || report.Task.ID != "map-maintain" || len(report.Steps) < 4 {
		t.Fatalf("task run map-maintain dry-run = %#v", report)
	}
	for _, step := range report.Steps {
		if step.Execute || !step.ReadOnly {
			t.Fatalf("map-maintain dry-run should only plan read-only steps: %#v", step)
		}
	}

	executePlan := runCLI(t, "task", "run", "map-execute", "--map", "map.demo", "--dry-run", "--json")
	if !strings.Contains(executePlan, `"id": "map-execute"`) || !strings.Contains(executePlan, "map explain --map map.demo") || !strings.Contains(executePlan, "map run --map map.demo") {
		t.Fatalf("task run map-execute dry-run should plan execution lifecycle:\n%s", executePlan)
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
		"agent-testbench task watch catalog-smoke --command",
		"agent-testbench notify test",
	} {
		if !strings.Contains(helpOut, want) {
			t.Fatalf("help missing %q:\n%s", want, helpOut)
		}
	}
}
