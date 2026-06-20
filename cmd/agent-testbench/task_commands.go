package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"agent-testbench/internal/store"
)

func runTask(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return errors.New("missing task command")
	}
	switch args[0] {
	case "catalog":
		return runTaskCatalog(args[1:])
	case "suggest":
		return runTaskSuggest(args[1:])
	case "plan":
		return runTaskPlan(args[1:])
	case "run":
		return runTaskRun(ctx, args[1:])
	case "schedule":
		return runTaskSchedule(ctx, args[1:])
	case "watch":
		return runTaskWatch(ctx, args[1:])
	case cliCommandList:
		return runTaskList(ctx, args[1:])
	case "status":
		return runTaskStatus(ctx, args[1:])
	case "logs":
		return runTaskLogs(ctx, args[1:])
	case "stop":
		return runTaskStop(ctx, args[1:])
	default:
		return fmt.Errorf("unknown task command: %s", args[0])
	}
}

func runTaskRun(ctx context.Context, args []string) error {
	flags := flag.NewFlagSet("task run", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	storeRef := flags.String("store", "", "Named Store config or Store DSN")
	command := flags.String("command", "", "AgentTestBench command to execute")
	mapID := flags.String("map", "", "Test map ID for built-in map tasks")
	environmentID := flags.String("environment", "", "Environment ID for built-in environment tasks")
	workspace := flags.String("workspace", "", "Workspace path for built-in environment tasks")
	caseRunID := flags.String("case-run", "", "Case run ID for built-in diagnosis tasks")
	runID := flags.String("run", "", "Run ID for built-in diagnosis tasks")
	dryRun := flags.Bool("dry-run", false, "Plan a built-in task without executing commands")
	shellMode := flags.Bool("shell", false, "Execute --command through /bin/sh -c for local sandbox trigger commands")
	notifyFile := flags.String("notify-file", "", "Append completion notifications to a JSONL file")
	notifyWebhook := flags.String("notify-webhook", "", "POST completion notifications to a webhook")
	jsonOutput := flags.Bool("json", false, "Emit a machine-readable task run report")
	if err := parseInterspersedFlags(flags, args); err != nil {
		return err
	}
	if flags.NArg() != 1 {
		return errors.New("task name is required")
	}
	name := flags.Arg(0)
	if strings.TrimSpace(*command) == "" {
		inputs := builtInTaskInputs{
			Map:         *mapID,
			Environment: *environmentID,
			Workspace:   *workspace,
			CaseRun:     *caseRunID,
			Run:         *runID,
			Store:       *storeRef,
		}
		return runBuiltInTask(ctx, name, inputs, *dryRun, *jsonOutput)
	}
	runtime, cleanup, err := openTaskStore(ctx, *storeRef)
	if err != nil {
		return err
	}
	defer cleanup()
	kind := "cli"
	if *shellMode {
		kind = "shell"
	}
	task, err := upsertTask(ctx, runtime, name, *command, "", "active", kind, taskNotificationOptions{File: *notifyFile, Webhook: *notifyWebhook})
	if err != nil {
		return err
	}
	run, execErr := executeAndRecordTaskRun(ctx, runtime, task, *command)
	if refreshed, refreshErr := runtime.GetAgentTask(ctx, task.ID); refreshErr == nil {
		task = refreshed
	}
	notify := sendTaskNotifications(ctx, taskNotificationOptions{File: *notifyFile, Webhook: *notifyWebhook}, task, run, "task run completed")
	notifyErr := notificationResultsError(notify)
	report := taskCommandReport{OK: execErr == nil && notifyErr == nil, Task: taskViewFromStore(task), Run: taskRunViewFromStore(run), Notify: notify}
	if *jsonOutput {
		if writeErr := writeIndentedJSON(report); writeErr != nil {
			return writeErr
		}
	}
	if execErr != nil {
		return execErr
	}
	if notifyErr != nil {
		return notifyErr
	}
	if !*jsonOutput {
		printTaskRunReport(report)
	}
	return nil
}

func runTaskSchedule(ctx context.Context, args []string) error {
	flags := flag.NewFlagSet("task schedule", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	storeRef := flags.String("store", "", "Named Store config or Store DSN")
	command := flags.String("command", "", "AgentTestBench command to execute")
	interval := flags.String("interval", "", "Schedule interval, such as 15m")
	cron := flags.String("cron", "", "Cron expression metadata")
	notifyFile := flags.String("notify-file", "", "Append completion notifications to a JSONL file")
	notifyWebhook := flags.String("notify-webhook", "", "POST completion notifications to a webhook")
	jsonOutput := flags.Bool("json", false, "Emit a machine-readable task schedule report")
	if err := parseInterspersedFlags(flags, args); err != nil {
		return err
	}
	if flags.NArg() != 1 {
		return errors.New("task name is required")
	}
	if strings.TrimSpace(*command) == "" {
		return errors.New("--command is required")
	}
	schedule, err := taskScheduleValue(*interval, *cron)
	if err != nil {
		return err
	}
	runtime, cleanup, err := openTaskStore(ctx, *storeRef)
	if err != nil {
		return err
	}
	defer cleanup()
	task, err := upsertCLITask(ctx, runtime, flags.Arg(0), *command, schedule, "scheduled", taskNotificationOptions{File: *notifyFile, Webhook: *notifyWebhook})
	if err != nil {
		return err
	}
	report := taskCommandReport{OK: true, Task: taskViewFromStore(task)}
	if *jsonOutput {
		return writeIndentedJSON(report)
	}
	fmt.Printf("Scheduled task: %s\n", task.Name)
	fmt.Printf("Schedule: %s\n", task.Schedule)
	return nil
}

func runTaskWatch(ctx context.Context, args []string) error {
	flags := flag.NewFlagSet("task watch", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	storeRef := flags.String("store", "", "Named Store config or Store DSN")
	command := flags.String("command", "", "AgentTestBench command to execute")
	interval := flags.Duration("interval", time.Minute, "Delay between attempts")
	limit := flags.Int("limit", 1, "Maximum attempts; use 0 for unlimited")
	until := flags.String("until", "always", "Stop condition: always, success, or failure")
	notifyFile := flags.String("notify-file", "", "Append completion notifications to a JSONL file")
	notifyWebhook := flags.String("notify-webhook", "", "POST completion notifications to a webhook")
	jsonOutput := flags.Bool("json", false, "Emit a machine-readable watch report")
	if err := parseInterspersedFlags(flags, args); err != nil {
		return err
	}
	if flags.NArg() != 1 {
		return errors.New("task name is required")
	}
	if strings.TrimSpace(*command) == "" {
		return errors.New("--command is required")
	}
	stopWhen, err := normalizeTaskWatchStopCondition(*until)
	if err != nil {
		return err
	}
	runtime, cleanup, err := openTaskStore(ctx, *storeRef)
	if err != nil {
		return err
	}
	defer cleanup()
	task, err := upsertCLITask(ctx, runtime, flags.Arg(0), *command, "watch:"+interval.String(), "active", taskNotificationOptions{File: *notifyFile, Webhook: *notifyWebhook})
	if err != nil {
		return err
	}
	result := runTaskWatchAttempts(ctx, runtime, task, *command, *interval, *limit, stopWhen, taskNotificationOptions{File: *notifyFile, Webhook: *notifyWebhook})
	task = result.Task
	if refreshed, refreshErr := runtime.GetAgentTask(ctx, task.ID); refreshErr == nil {
		task = refreshed
	}
	report := map[string]any{
		"ok":           result.ExecErr == nil && result.NotifyErr == nil,
		"attempts":     result.Attempts,
		cliCommandTask: taskViewFromStore(task),
		"notify":       result.Notify,
		"run":          taskRunViewFromStore(result.Latest),
		"status":       result.Latest.Status,
	}
	if *jsonOutput {
		if writeErr := writeIndentedJSON(report); writeErr != nil {
			return writeErr
		}
	}
	if result.ExecErr != nil {
		return result.ExecErr
	}
	if result.NotifyErr != nil {
		return result.NotifyErr
	}
	if !*jsonOutput {
		fmt.Printf("Watch: %s\n", task.Name)
		fmt.Printf("Attempts: %d\n", result.Attempts)
		fmt.Printf("Status: %s\n", result.Latest.Status)
	}
	return nil
}

type taskWatchAttemptResult struct {
	Task      store.AgentTask
	Latest    store.AgentTaskRun
	Notify    []notifyResult
	Attempts  int
	ExecErr   error
	NotifyErr error
}

func runTaskWatchAttempts(ctx context.Context, runtime store.Store, task store.AgentTask, command string, interval time.Duration, limit int, stopWhen string, notifyOpts taskNotificationOptions) taskWatchAttemptResult {
	result := taskWatchAttemptResult{Task: task}
	for {
		result.Attempts++
		result.Latest, result.ExecErr = executeAndRecordTaskRun(ctx, runtime, result.Task, command)
		result.Notify = sendTaskNotifications(ctx, notifyOpts, result.Task, result.Latest, "task watch attempt completed")
		if result.NotifyErr = notificationResultsError(result.Notify); result.NotifyErr != nil {
			return result
		}
		if taskWatchShouldStop(stopWhen, result.Latest.Status) {
			return result
		}
		if refreshed, refreshErr := runtime.GetAgentTask(ctx, result.Task.ID); refreshErr == nil {
			result.Task = refreshed
			if result.Task.Status != "active" {
				return result
			}
		}
		if limit > 0 && result.Attempts >= limit {
			return result
		}
		if interval > 0 {
			select {
			case <-ctx.Done():
				result.ExecErr = ctx.Err()
				return result
			case <-time.After(interval):
			}
		}
	}
}

func runTaskList(ctx context.Context, args []string) error {
	flags := flag.NewFlagSet("task list", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	storeRef := flags.String("store", "", "Named Store config or Store DSN")
	jsonOutput := flags.Bool("json", false, "Emit a machine-readable task list")
	if err := flags.Parse(args); err != nil {
		return err
	}
	runtime, cleanup, err := openTaskStore(ctx, *storeRef)
	if err != nil {
		return err
	}
	defer cleanup()
	tasks, err := runtime.ListAgentTasks(ctx)
	if err != nil {
		return err
	}
	views := make([]taskView, 0, len(tasks))
	for _, task := range tasks {
		views = append(views, taskViewFromStore(task))
	}
	report := map[string]any{"ok": true, "count": len(views), "tasks": views}
	if *jsonOutput {
		return writeIndentedJSON(report)
	}
	fmt.Println("Tasks")
	for _, task := range views {
		fmt.Printf("- %s %s latest=%s runs=%d\n", task.Name, task.Status, task.LatestStatus, task.RunCount)
	}
	return nil
}

func runTaskStatus(ctx context.Context, args []string) error {
	flags := flag.NewFlagSet("task status", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	storeRef := flags.String("store", "", "Named Store config or Store DSN")
	jsonOutput := flags.Bool("json", false, "Emit a machine-readable task status")
	if err := parseInterspersedFlags(flags, args); err != nil {
		return err
	}
	if flags.NArg() != 1 {
		return errors.New("task name is required")
	}
	_, cleanup, task, err := openTaskByArg(ctx, *storeRef, flags.Arg(0))
	if err != nil {
		return err
	}
	defer cleanup()
	report := map[string]any{"ok": true, cliCommandTask: taskViewFromStore(task)}
	if *jsonOutput {
		return writeIndentedJSON(report)
	}
	fmt.Printf("Task: %s\n", task.Name)
	fmt.Printf("Status: %s\n", task.Status)
	fmt.Printf("Latest: %s\n", task.LatestStatus)
	return nil
}

func runTaskLogs(ctx context.Context, args []string) error {
	flags := flag.NewFlagSet("task logs", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	storeRef := flags.String("store", "", "Named Store config or Store DSN")
	limit := flags.Int("n", 20, "Number of task runs to list")
	jsonOutput := flags.Bool("json", false, "Emit machine-readable task logs")
	if err := parseInterspersedFlags(flags, args); err != nil {
		return err
	}
	if flags.NArg() != 1 {
		return errors.New("task name is required")
	}
	runtime, cleanup, task, err := openTaskByArg(ctx, *storeRef, flags.Arg(0))
	if err != nil {
		return err
	}
	defer cleanup()
	runs, err := runtime.ListAgentTaskRuns(ctx, task.ID, *limit)
	if err != nil {
		return err
	}
	views := make([]taskRunView, 0, len(runs))
	for _, run := range runs {
		views = append(views, taskRunViewFromStore(run))
	}
	report := map[string]any{"ok": true, cliCommandTask: taskViewFromStore(task), "count": len(views), "runs": views}
	if *jsonOutput {
		return writeIndentedJSON(report)
	}
	fmt.Printf("Task Logs: %s\n", task.Name)
	for _, run := range views {
		fmt.Printf("- %s %s exit=%d\n", run.ID, run.Status, run.ExitCode)
	}
	return nil
}

func runTaskStop(ctx context.Context, args []string) error {
	flags := flag.NewFlagSet("task stop", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	storeRef := flags.String("store", "", "Named Store config or Store DSN")
	jsonOutput := flags.Bool("json", false, "Emit a machine-readable task stop report")
	if err := parseInterspersedFlags(flags, args); err != nil {
		return err
	}
	if flags.NArg() != 1 {
		return errors.New("task name is required")
	}
	runtime, cleanup, task, err := openTaskByArg(ctx, *storeRef, flags.Arg(0))
	if err != nil {
		return err
	}
	defer cleanup()
	task.Status = "paused"
	task.UpdatedAt = time.Now().UTC()
	task, err = runtime.UpsertAgentTask(ctx, task)
	if err != nil {
		return err
	}
	report := taskCommandReport{OK: true, Task: taskViewFromStore(task)}
	if *jsonOutput {
		return writeIndentedJSON(report)
	}
	fmt.Printf("Paused task: %s\n", task.Name)
	return nil
}
