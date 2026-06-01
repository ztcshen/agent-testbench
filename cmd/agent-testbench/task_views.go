package main

import (
	"fmt"
	"time"

	"agent-testbench/internal/store"
)

type taskView struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	Kind         string `json:"kind"`
	Command      string `json:"command"`
	Schedule     string `json:"schedule,omitempty"`
	Status       string `json:"status"`
	NotifyJSON   string `json:"notifyJson,omitempty"`
	SummaryJSON  string `json:"summaryJson,omitempty"`
	CreatedAt    string `json:"createdAt,omitempty"`
	UpdatedAt    string `json:"updatedAt,omitempty"`
	LatestStatus string `json:"latestStatus,omitempty"`
	LatestRunID  string `json:"latestRunId,omitempty"`
	LastRunAt    string `json:"lastRunAt,omitempty"`
	RunCount     int    `json:"runCount"`
}

type taskRunView struct {
	ID          string `json:"id"`
	TaskID      string `json:"taskId"`
	Status      string `json:"status"`
	Command     string `json:"command"`
	StartedAt   string `json:"startedAt,omitempty"`
	FinishedAt  string `json:"finishedAt,omitempty"`
	DurationMs  int64  `json:"durationMs"`
	ExitCode    int    `json:"exitCode"`
	Output      string `json:"output,omitempty"`
	Error       string `json:"error,omitempty"`
	SummaryJSON string `json:"summaryJson,omitempty"`
	CreatedAt   string `json:"createdAt,omitempty"`
}

type taskCommandReport struct {
	OK     bool           `json:"ok"`
	Task   taskView       `json:"task"`
	Run    taskRunView    `json:"run,omitempty"`
	Notify []notifyResult `json:"notify,omitempty"`
}

func taskViewFromStore(task store.AgentTask) taskView {
	return taskView{
		ID:           task.ID,
		Name:         task.Name,
		Kind:         task.Kind,
		Command:      task.Command,
		Schedule:     task.Schedule,
		Status:       task.Status,
		NotifyJSON:   task.NotifyJSON,
		SummaryJSON:  task.SummaryJSON,
		CreatedAt:    formatTaskTime(task.CreatedAt),
		UpdatedAt:    formatTaskTime(task.UpdatedAt),
		LatestStatus: task.LatestStatus,
		LatestRunID:  task.LatestRunID,
		LastRunAt:    formatTaskTime(task.LastRunAt),
		RunCount:     task.RunCount,
	}
}

func taskRunViewFromStore(run store.AgentTaskRun) taskRunView {
	return taskRunView{
		ID:          run.ID,
		TaskID:      run.TaskID,
		Status:      run.Status,
		Command:     run.Command,
		StartedAt:   formatTaskTime(run.StartedAt),
		FinishedAt:  formatTaskTime(run.FinishedAt),
		DurationMs:  run.DurationMs,
		ExitCode:    run.ExitCode,
		Output:      run.Output,
		Error:       run.Error,
		SummaryJSON: run.SummaryJSON,
		CreatedAt:   formatTaskTime(run.CreatedAt),
	}
}

func formatTaskTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339Nano)
}

func printTaskRunReport(report taskCommandReport) {
	fmt.Printf("Task: %s\n", report.Task.Name)
	fmt.Printf("Run: %s\n", report.Run.ID)
	fmt.Printf("Status: %s\n", report.Run.Status)
}
