package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"agent-testbench/internal/store"
)

const taskWorkerRecoveryBoundary = "hard crashes leave claimed tasks in running status; automatic lease recovery is unsupported; after verifying side effects, use task stop NAME --recover-running-claim --confirm-side-effects-reviewed to pause without replay before explicitly scheduling it again"

type taskWorkerReport struct {
	OK               bool                   `json:"ok"`
	Once             bool                   `json:"once"`
	Checked          int                    `json:"checked"`
	Scheduled        int                    `json:"scheduled"`
	Due              int                    `json:"due"`
	Claimed          int                    `json:"claimed"`
	Executed         int                    `json:"executed"`
	Skipped          int                    `json:"skipped"`
	Tasks            []taskWorkerTaskReport `json:"tasks"`
	RecoveryBoundary string                 `json:"recoveryBoundary"`
	PolledAt         string                 `json:"polledAt"`
}

type taskWorkerTaskReport struct {
	Task           taskView       `json:"task"`
	Claimed        bool           `json:"claimed,omitempty"`
	Attempted      bool           `json:"attempted,omitempty"`
	UnknownOutcome bool           `json:"unknownOutcome,omitempty"`
	Run            taskRunView    `json:"run,omitempty"`
	Notify         []notifyResult `json:"notify,omitempty"`
	Error          string         `json:"error,omitempty"`
}

func runTaskWorker(ctx context.Context, args []string) error {
	flags := flag.NewFlagSet("task worker", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	storeRef := flags.String("store", "", "Named Store config or Store DSN")
	once := flags.Bool("once", false, "Poll once, execute every due interval task, and exit")
	pollInterval := flags.Duration("poll-interval", 30*time.Second, "Delay between Store polls in foreground mode")
	jsonOutput := flags.Bool("json", false, "Emit one machine-readable report per Store poll")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("unexpected task worker arguments: %s", strings.Join(flags.Args(), " "))
	}
	if *pollInterval <= 0 {
		return errors.New("--poll-interval must be greater than zero")
	}

	workerCtx, stopSignals := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stopSignals()
	runtime, cleanup, err := openTaskStore(workerCtx, *storeRef)
	if err != nil {
		return err
	}
	defer cleanup()

	for {
		report, passErr := runTaskWorkerPass(workerCtx, runtime, time.Now().UTC())
		report.Once = *once
		if emitErr := emitTaskWorkerReport(report, *jsonOutput); emitErr != nil {
			return emitErr
		}
		if passErr != nil {
			if workerCtx.Err() != nil && errors.Is(passErr, context.Canceled) {
				return nil
			}
			return passErr
		}
		if *once {
			if !report.OK {
				return errors.New("one or more scheduled tasks failed")
			}
			return nil
		}
		select {
		case <-workerCtx.Done():
			return nil
		case <-time.After(*pollInterval):
		}
	}
}

func runTaskWorkerPass(ctx context.Context, runtime store.Store, now time.Time) (taskWorkerReport, error) {
	report := taskWorkerReport{
		OK:               true,
		Tasks:            []taskWorkerTaskReport{},
		RecoveryBoundary: taskWorkerRecoveryBoundary,
		PolledAt:         now.UTC().Format(time.RFC3339Nano),
	}
	tasks, err := runtime.ListAgentTasks(ctx)
	if err != nil {
		report.OK = false
		return report, fmt.Errorf("list scheduled tasks: %w", err)
	}
	report.Checked = len(tasks)
	for _, task := range tasks {
		if task.Status != "scheduled" {
			continue
		}
		report.Scheduled++
		due, scheduleErr := scheduledAgentTaskDue(task, now)
		if scheduleErr != nil {
			report.OK = false
			report.Tasks = append(report.Tasks, taskWorkerTaskReport{Task: taskViewFromStore(task), Error: scheduleErr.Error()})
			continue
		}
		if !due {
			report.Skipped++
			continue
		}
		report.Due++
		claim, claimed, claimErr := runtime.ClaimScheduledAgentTask(ctx, task.ID, task.UpdatedAt, now)
		if claimErr != nil {
			report.OK = false
			return report, fmt.Errorf("claim scheduled task %q: %w", task.Name, claimErr)
		}
		if !claimed {
			report.Skipped++
			continue
		}
		report.Claimed++
		item := executeClaimedScheduledTask(ctx, runtime, task)
		if item.Attempted {
			report.Executed++
		}
		if item.Error != "" || item.Run.Status == store.StatusFailed || !notificationResultsOK(item.Notify) {
			report.OK = false
		}
		report.Tasks = append(report.Tasks, item)
		if item.Attempted && item.Run.ID == "" {
			report.OK = false
			report.Tasks[len(report.Tasks)-1].UnknownOutcome = true
			report.Tasks[len(report.Tasks)-1].Error = appendTaskWorkerError(item.Error, errors.New("task execution was attempted but its run was not persisted; claim retained as running to prevent automatic replay"))
			return report, fmt.Errorf("scheduled task %q has an unknown outcome because its run was not persisted", task.Name)
		}

		releaseCtx, cancelRelease := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		released, releaseErr := runtime.ReleaseScheduledAgentTask(releaseCtx, claim, time.Now().UTC())
		cancelRelease()
		if releaseErr != nil {
			report.OK = false
			report.Tasks[len(report.Tasks)-1].Error = appendTaskWorkerError(report.Tasks[len(report.Tasks)-1].Error, releaseErr)
			return report, fmt.Errorf("release scheduled task %q: %w", task.Name, releaseErr)
		}
		if !released {
			// A running task is immutable outside its owning claim. A failed
			// release therefore means this worker no longer owns the row revision.
			report.OK = false
			report.Tasks[len(report.Tasks)-1].Error = "task claim was not released because its revision changed unexpectedly"
			return report, fmt.Errorf("release scheduled task %q: claim revision no longer matches", task.Name)
		}
		if refreshed, refreshErr := runtime.GetAgentTask(context.WithoutCancel(ctx), task.ID); refreshErr == nil {
			report.Tasks[len(report.Tasks)-1].Task = taskViewFromStore(refreshed)
		}
	}
	return report, nil
}

func executeClaimedScheduledTask(ctx context.Context, runtime store.Store, listed store.AgentTask) taskWorkerTaskReport {
	task := listed
	item := taskWorkerTaskReport{Task: taskViewFromStore(task), Claimed: true}
	if refreshed, err := runtime.GetAgentTask(ctx, task.ID); err == nil {
		task = refreshed
		item.Task = taskViewFromStore(task)
	} else {
		item.Error = fmt.Sprintf("reload claimed task: %v", err)
		return item
	}
	notify, err := taskNotificationOptionsFromJSON(task.NotifyJSON)
	if err != nil {
		item.Error = err.Error()
		return item
	}
	item.Attempted = true
	run, execErr := executeAndRecordTaskRun(ctx, runtime, task, task.Command)
	item.Run = taskRunViewFromStore(run)
	item.Notify = sendTaskNotifications(ctx, notify, task, run, "scheduled task run completed")
	if execErr != nil {
		item.Error = execErr.Error()
	}
	if notifyErr := notificationResultsError(item.Notify); notifyErr != nil {
		item.Error = appendTaskWorkerError(item.Error, notifyErr)
	}
	return item
}

func appendTaskWorkerError(current string, err error) string {
	if err == nil {
		return current
	}
	if strings.TrimSpace(current) == "" {
		return err.Error()
	}
	return current + "; " + err.Error()
}

func scheduledAgentTaskDue(task store.AgentTask, now time.Time) (bool, error) {
	const intervalPrefix = "interval:"
	if !strings.HasPrefix(task.Schedule, intervalPrefix) {
		return false, fmt.Errorf("unsupported schedule %q; task worker supports interval schedules only", task.Schedule)
	}
	interval, err := time.ParseDuration(strings.TrimPrefix(task.Schedule, intervalPrefix))
	if err != nil || interval <= 0 {
		return false, fmt.Errorf("invalid interval schedule %q", task.Schedule)
	}
	anchor := task.UpdatedAt
	if anchor.IsZero() {
		anchor = task.CreatedAt
	}
	if task.LastRunAt.After(anchor) {
		anchor = task.LastRunAt
	}
	if anchor.IsZero() {
		return true, nil
	}
	return !now.Before(anchor.Add(interval)), nil
}

func taskNotificationOptionsFromJSON(raw string) (taskNotificationOptions, error) {
	var options struct {
		File    string `json:"file"`
		Webhook string `json:"webhook"`
	}
	if strings.TrimSpace(raw) == "" {
		return taskNotificationOptions{}, nil
	}
	if err := json.Unmarshal([]byte(raw), &options); err != nil {
		return taskNotificationOptions{}, fmt.Errorf("decode task notification settings: %w", err)
	}
	return taskNotificationOptions{File: options.File, Webhook: options.Webhook}, nil
}

func emitTaskWorkerReport(report taskWorkerReport, jsonOutput bool) error {
	if jsonOutput {
		return writeIndentedJSON(report)
	}
	fmt.Printf("Task worker poll: checked=%d scheduled=%d due=%d claimed=%d executed=%d\n", report.Checked, report.Scheduled, report.Due, report.Claimed, report.Executed)
	for _, task := range report.Tasks {
		status := task.Run.Status
		if status == "" {
			status = "not-run"
		}
		fmt.Printf("- %s status=%s", task.Task.Name, status)
		if task.Error != "" {
			fmt.Printf(" error=%s", task.Error)
		}
		fmt.Println()
	}
	fmt.Printf("Recovery boundary: %s\n", report.RecoveryBoundary)
	return nil
}
