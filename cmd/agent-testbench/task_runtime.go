package main

import (
	"context"
	"errors"
	"fmt"
	"hash/crc32"
	"os"
	"os/exec"
	"strings"
	"time"

	"agent-testbench/internal/store"
)

func openTaskStore(ctx context.Context, storeRef string) (store.Store, func(), error) {
	storeURL, err := resolveRequiredDailyStoreReference(storeRef, "")
	if err != nil {
		return nil, nil, err
	}
	runtime, err := openStore(ctx, storeURL)
	if err != nil {
		return nil, nil, err
	}
	return runtime, cleanupCLIStore(runtime), nil
}

func openTaskByArg(ctx context.Context, storeRef string, taskRef string) (store.Store, func(), store.AgentTask, error) {
	runtime, cleanup, err := openTaskStore(ctx, storeRef)
	if err != nil {
		return nil, nil, store.AgentTask{}, err
	}
	task, err := runtime.GetAgentTask(ctx, taskRef)
	if err != nil {
		cleanup()
		return nil, nil, store.AgentTask{}, err
	}
	return runtime, cleanup, task, nil
}

func upsertCLITask(ctx context.Context, runtime store.Store, name string, command string, schedule string, status string, notify taskNotificationOptions) (store.AgentTask, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return store.AgentTask{}, errors.New("task name is required")
	}
	command = strings.TrimSpace(command)
	if command == "" {
		return store.AgentTask{}, errors.New("task command is required")
	}
	now := time.Now().UTC()
	task := store.AgentTask{
		ID:          "agent-task." + safeTaskIDPart(name),
		Name:        name,
		Kind:        "cli",
		Command:     command,
		Schedule:    schedule,
		Status:      status,
		NotifyJSON:  notifyJSON(notify),
		SummaryJSON: "{}",
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	existing, err := runtime.GetAgentTask(ctx, name)
	if err == nil {
		task.ID = existing.ID
		task.CreatedAt = existing.CreatedAt
		if task.Schedule == "" {
			task.Schedule = existing.Schedule
		}
	} else if !errors.Is(err, store.ErrNotFound) {
		return store.AgentTask{}, err
	}
	return runtime.UpsertAgentTask(ctx, task)
}

func executeAndRecordTaskRun(ctx context.Context, runtime store.Store, task store.AgentTask, command string) (store.AgentTaskRun, error) {
	started := time.Now().UTC()
	output, exitCode, execErr := executeAgentTestBenchCommand(ctx, command)
	finished := time.Now().UTC()
	status := store.StatusPassed
	errorText := ""
	if execErr != nil {
		status = store.StatusFailed
		errorText = execErr.Error()
	}
	run, recordErr := runtime.RecordAgentTaskRun(ctx, store.AgentTaskRun{
		ID:          "agent-task-run." + safeTaskIDPart(task.Name) + "." + finished.Format("20060102T150405.000000000Z"),
		TaskID:      task.ID,
		Status:      status,
		Command:     command,
		StartedAt:   started,
		FinishedAt:  finished,
		ExitCode:    exitCode,
		Output:      output,
		Error:       errorText,
		SummaryJSON: mustCompactJSON(map[string]any{"taskName": task.Name}),
		CreatedAt:   finished,
	})
	if recordErr != nil {
		return store.AgentTaskRun{}, recordErr
	}
	return run, execErr
}

func executeAgentTestBenchCommand(ctx context.Context, command string) (string, int, error) {
	args, err := splitCommandLine(command)
	if err != nil {
		return "", -1, err
	}
	if len(args) == 0 {
		return "", -1, errors.New("task command is empty")
	}
	exe, err := os.Executable()
	if err != nil {
		return "", -1, err
	}
	cmd := exec.CommandContext(ctx, exe, args...)
	cmd.Env = os.Environ()
	out, err := cmd.CombinedOutput()
	if err == nil {
		return string(out), 0, nil
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return string(out), exitErr.ExitCode(), err
	}
	return string(out), -1, err
}

func splitCommandLine(command string) ([]string, error) {
	args := []string{}
	var b strings.Builder
	var quote rune
	escaped := false
	for _, r := range command {
		if escaped {
			b.WriteRune(r)
			escaped = false
			continue
		}
		if r == '\\' {
			escaped = true
			continue
		}
		if quote != 0 {
			if r == quote {
				quote = 0
				continue
			}
			b.WriteRune(r)
			continue
		}
		if r == '\'' || r == '"' {
			quote = r
			continue
		}
		if r == ' ' || r == '\t' || r == '\n' {
			if b.Len() > 0 {
				args = append(args, b.String())
				b.Reset()
			}
			continue
		}
		b.WriteRune(r)
	}
	if escaped {
		b.WriteRune('\\')
	}
	if quote != 0 {
		return nil, errors.New("unterminated quote in task command")
	}
	if b.Len() > 0 {
		args = append(args, b.String())
	}
	return args, nil
}

func taskScheduleValue(interval string, cron string) (string, error) {
	interval = strings.TrimSpace(interval)
	cron = strings.TrimSpace(cron)
	if interval != "" && cron != "" {
		return "", errors.New("--interval and --cron cannot be combined")
	}
	if interval != "" {
		if _, err := time.ParseDuration(interval); err != nil {
			return "", fmt.Errorf("invalid --interval: %w", err)
		}
		return "interval:" + interval, nil
	}
	if cron != "" {
		return "cron:" + cron, nil
	}
	return "", errors.New("schedule requires --interval or --cron")
}

func normalizeTaskWatchStopCondition(until string) (string, error) {
	stopWhen := strings.ToLower(strings.TrimSpace(until))
	switch stopWhen {
	case "always", "success", "failure":
		return stopWhen, nil
	default:
		return "", fmt.Errorf("unsupported watch --until %q; use always, success, or failure", until)
	}
}

func taskWatchShouldStop(until string, status string) bool {
	switch until {
	case "success":
		return status == store.StatusPassed
	case "failure":
		return status == store.StatusFailed
	default:
		return false
	}
}

func safeTaskIDPart(value string) string {
	safe := safeReportID(value)
	if len(safe) > 80 {
		suffix := fmt.Sprintf("%08x", crc32.ChecksumIEEE([]byte(value)))
		prefixLen := 80 - len(suffix) - 1
		return safe[:prefixLen] + "-" + suffix
	}
	return safe
}
