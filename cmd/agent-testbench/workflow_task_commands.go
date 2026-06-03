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

type workflowTaskRunReport struct {
	OK         bool                      `json:"ok"`
	RunID      string                    `json:"runId"`
	WorkflowID string                    `json:"workflowId"`
	Status     string                    `json:"status"`
	Counts     workflowTaskRunCounts     `json:"counts"`
	Steps      []workflowTaskRunStepView `json:"steps"`
}

type workflowTaskRunCounts struct {
	Total  int `json:"total"`
	Passed int `json:"passed"`
	Failed int `json:"failed"`
}

type workflowTaskRunStepView struct {
	StepID    string `json:"stepId"`
	Kind      string `json:"kind"`
	TaskID    string `json:"taskId"`
	TaskName  string `json:"taskName"`
	TaskRunID string `json:"taskRunId"`
	Status    string `json:"status"`
	ExitCode  int    `json:"exitCode"`
	ElapsedMs int64  `json:"elapsedMs"`
	Error     string `json:"error,omitempty"`
}

type workflowTaskStepSpec struct {
	StepID  string
	TaskRef string
}

type workflowTaskRunStepResult struct {
	View    workflowTaskRunStepView
	TaskRun store.AgentTaskRun
}

type workflowTaskResolvedStep struct {
	Spec workflowTaskStepSpec
	Task store.AgentTask
}

func runWorkflowTask(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return errors.New("missing workflow task command")
	}
	switch args[0] {
	case "run":
		return runWorkflowTaskRun(ctx, args[1:])
	default:
		return fmt.Errorf("unknown workflow task command: %s", args[0])
	}
}

func runWorkflowTaskRun(ctx context.Context, args []string) error {
	flags := flag.NewFlagSet("workflow task run", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	storeRef := flags.String("store", "", "Named Store config or Store DSN")
	storeURL := flags.String("store-url", "", legacyStoreURLFlagHelp)
	workflowID := flags.String("workflow", "", "Workflow id")
	var steps stringListFlag
	flags.Var(&steps, "step", "Workflow task step in STEP_ID=TASK_NAME_OR_ID form; repeat for ordered steps")
	jsonOutput := flags.Bool("json", false, "Emit a machine-readable workflow task report")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if strings.TrimSpace(*workflowID) == "" {
		return errors.New("--workflow is required")
	}
	specs, err := workflowTaskStepSpecs(steps.Values())
	if err != nil {
		return err
	}
	if len(specs) == 0 {
		return errors.New("--step is required")
	}
	runtime, cleanup, err := openRequiredCLIStore(ctx, *storeRef, *storeURL)
	if err != nil {
		return err
	}
	defer cleanup()
	report, err := executeWorkflowTaskRun(ctx, runtime, strings.TrimSpace(*workflowID), specs)
	if err != nil {
		return err
	}
	if *jsonOutput {
		if err := writeIndentedJSON(report); err != nil {
			return err
		}
	} else {
		printWorkflowTaskRunReport(report)
	}
	if !report.OK {
		return errors.New("workflow task run failed")
	}
	return nil
}

func workflowTaskStepSpecs(values []string) ([]workflowTaskStepSpec, error) {
	out := make([]workflowTaskStepSpec, 0, len(values))
	seen := map[string]bool{}
	seenEvidenceID := map[string]bool{}
	for _, raw := range values {
		stepID, taskRef, ok := strings.Cut(raw, "=")
		stepID = strings.TrimSpace(stepID)
		taskRef = strings.TrimSpace(taskRef)
		if !ok || stepID == "" || taskRef == "" {
			return nil, fmt.Errorf("--step must be STEP_ID=TASK_NAME_OR_ID, got %q", raw)
		}
		if seen[stepID] {
			return nil, fmt.Errorf("duplicate workflow task step id: %s", stepID)
		}
		seen[stepID] = true
		evidenceID := safeReportID(stepID)
		if evidenceID == "" {
			return nil, fmt.Errorf("workflow task step id must contain an alphanumeric character: %s", stepID)
		}
		if seenEvidenceID[evidenceID] {
			return nil, fmt.Errorf("duplicate workflow task step evidence id after normalization: %s", stepID)
		}
		seenEvidenceID[evidenceID] = true
		out = append(out, workflowTaskStepSpec{StepID: stepID, TaskRef: taskRef})
	}
	return out, nil
}

func executeWorkflowTaskRun(ctx context.Context, runtime store.Store, workflowID string, specs []workflowTaskStepSpec) (workflowTaskRunReport, error) {
	started := time.Now().UTC()
	runID := "run." + safeReportID(workflowID) + ".task." + started.Format("20060102T150405.000000000Z")
	report := workflowTaskRunReport{
		OK:         true,
		RunID:      runID,
		WorkflowID: workflowID,
		Status:     store.StatusPassed,
		Steps:      make([]workflowTaskRunStepView, 0, len(specs)),
		Counts:     workflowTaskRunCounts{Total: len(specs)},
	}
	resolved, err := resolveWorkflowTaskSteps(ctx, runtime, specs)
	if err != nil {
		return workflowTaskRunReport{}, err
	}
	stepResults := make([]workflowTaskRunStepResult, 0, len(specs))
	for _, step := range resolved {
		result, err := executeWorkflowTaskStep(ctx, runtime, step)
		if err != nil {
			return workflowTaskRunReport{}, err
		}
		step := result.View
		stepResults = append(stepResults, result)
		report.Steps = append(report.Steps, step)
		if strings.EqualFold(step.Status, store.StatusPassed) {
			report.Counts.Passed++
			continue
		}
		report.Counts.Failed++
		report.OK = false
		report.Status = store.StatusFailed
		break
	}
	finished := time.Now().UTC()
	if _, err := runtime.CreateRun(ctx, store.Run{
		ID:          runID,
		WorkflowID:  workflowID,
		Status:      report.Status,
		SummaryJSON: mustCompactJSON(workflowTaskRunSummary(report)),
		StartedAt:   started,
		FinishedAt:  finished,
		CreatedAt:   started,
		UpdatedAt:   finished,
	}); err != nil {
		return workflowTaskRunReport{}, err
	}
	for _, result := range stepResults {
		if err := recordWorkflowTaskEvidence(ctx, runtime, runID, workflowID, result.View, result.TaskRun); err != nil {
			return workflowTaskRunReport{}, err
		}
	}
	return report, nil
}

func resolveWorkflowTaskSteps(ctx context.Context, runtime store.Store, specs []workflowTaskStepSpec) ([]workflowTaskResolvedStep, error) {
	out := make([]workflowTaskResolvedStep, 0, len(specs))
	for _, spec := range specs {
		task, err := runtime.GetAgentTask(ctx, spec.TaskRef)
		if err != nil {
			return nil, err
		}
		out = append(out, workflowTaskResolvedStep{Spec: spec, Task: task})
	}
	return out, nil
}

func executeWorkflowTaskStep(ctx context.Context, runtime store.Store, resolved workflowTaskResolvedStep) (workflowTaskRunStepResult, error) {
	spec := resolved.Spec
	task := resolved.Task
	taskRun, execErr := executeAndRecordTaskRun(ctx, runtime, task, task.Command)
	if taskRun.ID == "" && execErr != nil {
		return workflowTaskRunStepResult{}, execErr
	}
	step := workflowTaskRunStepView{
		StepID:    spec.StepID,
		Kind:      cliCommandTask,
		TaskID:    task.ID,
		TaskName:  task.Name,
		TaskRunID: taskRun.ID,
		Status:    taskRun.Status,
		ExitCode:  taskRun.ExitCode,
		ElapsedMs: taskRun.DurationMs,
		Error:     taskRun.Error,
	}
	return workflowTaskRunStepResult{View: step, TaskRun: taskRun}, nil
}

func recordWorkflowTaskEvidence(ctx context.Context, runtime store.Store, runID string, workflowID string, step workflowTaskRunStepView, taskRun store.AgentTaskRun) error {
	_, err := runtime.RecordEvidence(ctx, store.EvidenceRecord{
		ID:        runID + "." + safeReportID(step.StepID) + ".task-run",
		RunID:     runID,
		StepID:    step.StepID,
		Kind:      "task-run",
		MediaType: "application/json",
		Summary: mustCompactJSON(map[string]any{
			"workflowId": workflowID,
			"stepId":     step.StepID,
			"taskId":     step.TaskID,
			"taskName":   step.TaskName,
			"taskRunId":  step.TaskRunID,
			"status":     step.Status,
			"exitCode":   step.ExitCode,
			"output":     truncateReportText(taskRun.Output, 500),
			"error":      taskRun.Error,
		}),
		CreatedAt: time.Now().UTC(),
	})
	return err
}

func workflowTaskRunSummary(report workflowTaskRunReport) map[string]any {
	steps := make([]map[string]any, 0, len(report.Steps))
	for _, step := range report.Steps {
		steps = append(steps, map[string]any{
			"stepId":    step.StepID,
			"kind":      step.Kind,
			"taskId":    step.TaskID,
			"taskName":  step.TaskName,
			"taskRunId": step.TaskRunID,
			"status":    step.Status,
			"exitCode":  step.ExitCode,
			"elapsedMs": step.ElapsedMs,
			"error":     step.Error,
		})
	}
	return map[string]any{
		"summary": map[string]any{
			"stepCount": len(report.Steps),
			"passed":    report.Counts.Passed,
			"failed":    report.Counts.Failed,
		},
		"steps": steps,
	}
}

func printWorkflowTaskRunReport(report workflowTaskRunReport) {
	fmt.Printf("Workflow Task Run: %s\n", report.RunID)
	fmt.Printf("Workflow: %s\n", report.WorkflowID)
	fmt.Printf("Status: %s\n", report.Status)
	fmt.Printf("Total: %d Passed: %d Failed: %d\n", report.Counts.Total, report.Counts.Passed, report.Counts.Failed)
}
