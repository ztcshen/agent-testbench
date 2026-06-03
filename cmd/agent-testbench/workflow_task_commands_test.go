package main

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"agent-testbench/internal/store"
	"agent-testbench/internal/store/sqlite"
)

func TestWorkflowTaskRunRecordsShellTriggerAndPostconditionSteps(t *testing.T) {
	ctx := context.Background()
	storePath := filepath.Join(t.TempDir(), "store.sqlite")
	runtime, err := sqlite.Open(ctx, sqlite.Config{Path: storePath})
	if err != nil {
		t.Fatalf("open sqlite store: %v", err)
	}
	started := time.Now().UTC()
	for _, task := range []store.AgentTask{
		{ID: "agent-task.publish", Name: "publish-message", Kind: "shell", Command: "printf publish-ok", Status: "active", CreatedAt: started, UpdatedAt: started},
		{ID: "agent-task.consumer-check", Name: "consumer-postcondition", Kind: "shell", Command: "printf consumer-ok", Status: "active", CreatedAt: started, UpdatedAt: started},
	} {
		if _, err := runtime.UpsertAgentTask(ctx, task); err != nil {
			t.Fatalf("upsert task %s: %v", task.Name, err)
		}
	}
	if err := runtime.Close(); err != nil {
		t.Fatalf("close store: %v", err)
	}

	out := runCLI(t, "workflow", "task", "run",
		"--store", "sqlite://"+storePath,
		"--workflow", "workflow.mq-smoke",
		"--step", "trigger=publish-message",
		"--step", "postcondition=consumer-postcondition",
		"--json",
	)
	var report struct {
		OK     bool   `json:"ok"`
		RunID  string `json:"runId"`
		Status string `json:"status"`
		Counts struct {
			Total  int `json:"total"`
			Passed int `json:"passed"`
			Failed int `json:"failed"`
		} `json:"counts"`
		Steps []struct {
			StepID    string `json:"stepId"`
			Kind      string `json:"kind"`
			TaskID    string `json:"taskId"`
			TaskRunID string `json:"taskRunId"`
			Status    string `json:"status"`
			ExitCode  int    `json:"exitCode"`
		} `json:"steps"`
	}
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatalf("decode workflow task report: %v\n%s", err, out)
	}
	if !report.OK || report.Status != store.StatusPassed || report.Counts.Total != 2 || report.Counts.Passed != 2 || len(report.Steps) != 2 {
		t.Fatalf("workflow task report = %#v", report)
	}
	if report.Steps[0].StepID != "trigger" || report.Steps[0].Kind != "task" || report.Steps[0].TaskRunID == "" || report.Steps[0].ExitCode != 0 {
		t.Fatalf("trigger step = %#v", report.Steps[0])
	}
	if report.Steps[1].StepID != "postcondition" || report.Steps[1].Kind != "task" || report.Steps[1].TaskRunID == "" || report.Steps[1].ExitCode != 0 {
		t.Fatalf("postcondition step = %#v", report.Steps[1])
	}

	gateOut := runCLI(t, "workflow", "gate", "--store", "sqlite://"+storePath, "--run", report.RunID, "--require-passed", "--require-steps", "--require-evidence", "--json")
	gate := decodeWorkflowGateFullReport(t, gateOut)
	if !gate.OK || !gate.Gates.EvidenceComplete || gate.Counts.Steps != 2 || gate.Counts.EvidenceComplete != 2 {
		t.Fatalf("workflow gate should accept task-step evidence = %#v", gate)
	}
}

func TestWorkflowTaskStepSpecsRejectDuplicateStepIDs(t *testing.T) {
	_, err := workflowTaskStepSpecs([]string{"trigger=publish-message", "trigger=consumer-postcondition"})
	if err == nil {
		t.Fatalf("duplicate workflow task step id should fail")
	}
}

func TestWorkflowTaskStepSpecsRejectDuplicateEvidenceIDs(t *testing.T) {
	_, err := workflowTaskStepSpecs([]string{"seed object=publish-message", "seed_object=consumer-postcondition"})
	if err == nil {
		t.Fatalf("duplicate normalized workflow task step evidence id should fail")
	}
}

func TestWorkflowTaskRunResolvesAllTaskRefsBeforeExecution(t *testing.T) {
	ctx := context.Background()
	storePath := filepath.Join(t.TempDir(), "store.sqlite")
	runtime, err := sqlite.Open(ctx, sqlite.Config{Path: storePath})
	if err != nil {
		t.Fatalf("open sqlite store: %v", err)
	}
	started := time.Now().UTC()
	task := store.AgentTask{
		ID:        "agent-task.publish",
		Name:      "publish-message",
		Kind:      "shell",
		Command:   "printf publish-ok",
		Status:    "active",
		CreatedAt: started,
		UpdatedAt: started,
	}
	if _, err := runtime.UpsertAgentTask(ctx, task); err != nil {
		t.Fatalf("upsert task: %v", err)
	}
	if err := runtime.Close(); err != nil {
		t.Fatalf("close store: %v", err)
	}

	runCLIFails(t, "workflow", "task", "run",
		"--store", "sqlite://"+storePath,
		"--workflow", "workflow.mq-smoke",
		"--step", "trigger=publish-message",
		"--step", "postcondition=missing-task",
		"--json",
	)

	runtime, err = sqlite.Open(ctx, sqlite.Config{Path: storePath})
	if err != nil {
		t.Fatalf("reopen sqlite store: %v", err)
	}
	defer runtime.Close()
	runs, err := runtime.ListAgentTaskRuns(ctx, task.ID, 10)
	if err != nil {
		t.Fatalf("list task runs: %v", err)
	}
	if len(runs) != 0 {
		t.Fatalf("workflow task should not execute earlier steps before resolving all refs: %#v", runs)
	}
}
