package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"

	"agent-testbench/internal/domain/commandline"
	"agent-testbench/internal/domain/mapplanner"
	"agent-testbench/internal/store"
)

type mapGateReport struct {
	OK              bool              `json:"ok"`
	PlanID          string            `json:"planId"`
	MapID           string            `json:"mapId"`
	Status          string            `json:"status"`
	Counts          mapGateCounts     `json:"counts"`
	Gates           mapGateGates      `json:"gates"`
	FailedTasks     []mapGateTaskItem `json:"failedTasks"`
	MissingEvidence []mapGateTaskItem `json:"missingEvidence"`
	NextActions     []string          `json:"nextActions"`
	Warnings        []string          `json:"warnings"`
}

type mapGateCounts struct {
	TotalTasks       int `json:"totalTasks"`
	PassedTasks      int `json:"passedTasks"`
	FailedTasks      int `json:"failedTasks"`
	BlockedTasks     int `json:"blockedTasks"`
	SkippedTasks     int `json:"skippedTasks"`
	OtherTasks       int `json:"otherTasks"`
	WorkflowRuns     int `json:"workflowRuns"`
	APICaseRuns      int `json:"apiCaseRuns"`
	EvidenceComplete int `json:"evidenceComplete"`
}

type mapGateGates struct {
	PlanPassed       bool `json:"planPassed"`
	TasksPresent     bool `json:"tasksPresent"`
	TasksPassed      bool `json:"tasksPassed"`
	EvidenceComplete bool `json:"evidenceComplete"`
}

type mapGateTaskItem struct {
	ID            string `json:"id"`
	Index         int    `json:"index"`
	Kind          string `json:"kind"`
	PathID        string `json:"pathId,omitempty"`
	WorkflowID    string `json:"workflowId,omitempty"`
	NodeID        string `json:"nodeId,omitempty"`
	CaseID        string `json:"caseId,omitempty"`
	Status        string `json:"status"`
	Reason        string `json:"reason,omitempty"`
	WorkflowRunID string `json:"workflowRunId,omitempty"`
	APICaseRunID  string `json:"apiCaseRunId,omitempty"`
	EvidenceCount int    `json:"evidenceCount"`
}

type mapGateOptions struct {
	PlanID          string
	RequirePassed   bool
	RequireTasks    bool
	RequireEvidence bool
}

type mapGateCLIOptions struct {
	storeRef   string
	storeURL   string
	jsonOutput bool
	gate       mapGateOptions
}

func runMapGate(ctx context.Context, args []string) error {
	options, err := parseMapGateOptions(args)
	if err != nil {
		return err
	}
	runtime, cleanup, err := openRequiredCLIStore(ctx, options.storeRef, options.storeURL)
	if err != nil {
		return err
	}
	defer cleanup()
	report, err := buildMapGateReport(ctx, runtime, options.gate)
	if err != nil {
		return err
	}
	if options.jsonOutput {
		if err := writeIndentedJSON(report); err != nil {
			return err
		}
	} else {
		printMapGate(report)
	}
	if !report.OK {
		return errors.New("map gate failed")
	}
	return nil
}

func parseMapGateOptions(args []string) (mapGateCLIOptions, error) {
	flags := flag.NewFlagSet("map gate", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	storeRef := flags.String("store", "", "Named Store config or Store DSN")
	storeURL := flags.String("store-url", "", legacyStoreURLFlagHelp)
	planID := flags.String("plan", "", "Planner run instance id")
	requirePassed := flags.Bool("require-passed", false, "Fail unless the map plan status is passed")
	requireTasks := flags.Bool("require-tasks", false, "Fail unless map plan tasks exist and all executable tasks passed")
	requireEvidence := flags.Bool("require-evidence", false, "Fail unless every executed task has indexed Evidence")
	jsonOutput := flags.Bool("json", false, "Emit a machine-readable JSON report")
	if err := flags.Parse(args); err != nil {
		return mapGateCLIOptions{}, err
	}
	if strings.TrimSpace(*planID) == "" {
		return mapGateCLIOptions{}, errors.New("--plan is required")
	}
	return mapGateCLIOptions{
		storeRef:   *storeRef,
		storeURL:   *storeURL,
		jsonOutput: *jsonOutput,
		gate: mapGateOptions{
			PlanID:          *planID,
			RequirePassed:   *requirePassed,
			RequireTasks:    *requireTasks,
			RequireEvidence: *requireEvidence,
		},
	}, nil
}

func buildMapGateReport(ctx context.Context, runtime store.Store, options mapGateOptions) (mapGateReport, error) {
	record, err := runtime.GetTestMapPlan(ctx, strings.TrimSpace(options.PlanID))
	if err != nil {
		return mapGateReport{}, err
	}
	report := mapGateReport{
		PlanID:          record.Instance.ID,
		MapID:           record.Instance.MapID,
		Status:          firstNonEmpty(record.Instance.Status, mapRunStatus(record.Tasks)),
		FailedTasks:     []mapGateTaskItem{},
		MissingEvidence: []mapGateTaskItem{},
		NextActions:     []string{},
		Warnings:        []string{},
	}
	report.Counts.TotalTasks = len(record.Tasks)
	for _, task := range record.Tasks {
		item, err := mapGateTaskFromRecord(ctx, runtime, task)
		if err != nil {
			return mapGateReport{}, err
		}
		addMapGateTask(&report, item)
	}
	report.Gates = mapGateGates{
		PlanPassed:       strings.EqualFold(report.Status, store.StatusPassed),
		TasksPresent:     report.Counts.TotalTasks > 0,
		TasksPassed:      report.Counts.TotalTasks > 0 && report.Counts.FailedTasks == 0 && report.Counts.BlockedTasks == 0 && report.Counts.OtherTasks == 0,
		EvidenceComplete: report.Counts.TotalTasks > 0 && len(report.MissingEvidence) == 0,
	}
	report.OK = (!options.RequirePassed || report.Gates.PlanPassed) &&
		(!options.RequireTasks || (report.Gates.TasksPresent && report.Gates.TasksPassed)) &&
		(!options.RequireEvidence || report.Gates.EvidenceComplete)
	report.NextActions = mapGateNextActions(report, options)
	return report, nil
}

func mapGateTaskFromRecord(ctx context.Context, runtime store.Store, task store.TestMapPlanTask) (mapGateTaskItem, error) {
	evidenceCount, err := mapGateTaskEvidenceCount(ctx, runtime, task)
	if err != nil {
		return mapGateTaskItem{}, err
	}
	return mapGateTaskItem{
		ID:            task.ID,
		Index:         task.Index,
		Kind:          task.Kind,
		PathID:        task.PathID,
		WorkflowID:    task.WorkflowID,
		NodeID:        task.NodeID,
		CaseID:        task.CaseID,
		Status:        task.Status,
		Reason:        task.Reason,
		WorkflowRunID: task.WorkflowRunID,
		APICaseRunID:  task.APICaseRunID,
		EvidenceCount: evidenceCount,
	}, nil
}

func mapGateTaskEvidenceCount(ctx context.Context, runtime store.Store, task store.TestMapPlanTask) (int, error) {
	count := 0
	for _, runID := range mapGateTaskEvidenceRunIDs(task) {
		records, err := runtime.ListEvidence(ctx, runID)
		if err != nil {
			return 0, fmt.Errorf("list task evidence %s: %w", runID, err)
		}
		count += len(records)
	}
	return count, nil
}

func mapGateTaskEvidenceRunIDs(task store.TestMapPlanTask) []string {
	ids := []string{}
	if task.WorkflowRunID != "" {
		ids = append(ids, task.WorkflowRunID)
	}
	if task.APICaseRunID != "" {
		ids = append(ids, task.APICaseRunID)
		ids = append(ids, mapGateCaseParentRunID(task.APICaseRunID))
	}
	summary := jsonObjectString(task.SummaryJSON)
	for _, raw := range listFromReportAny(summary["steps"]) {
		step := mapFromReportAny(raw)
		ids = append(ids, valueString(step["runId"]))
		if caseRunID := valueString(step["apiCaseRunId"]); caseRunID != "" {
			ids = append(ids, caseRunID)
			ids = append(ids, mapGateCaseParentRunID(caseRunID))
		}
	}
	if result := mapFromReportAny(summary["result"]); len(result) > 0 {
		ids = append(ids, valueString(result["runId"]))
		if caseRunID := valueString(result["caseRunId"]); caseRunID != "" {
			ids = append(ids, caseRunID)
			ids = append(ids, mapGateCaseParentRunID(caseRunID))
		}
	}
	return compactUniqueStringListPreserveOrder(ids)
}

func mapGateCaseParentRunID(caseRunID string) string {
	return strings.TrimSuffix(strings.TrimSpace(caseRunID), ".case")
}

func addMapGateTask(report *mapGateReport, item mapGateTaskItem) {
	switch {
	case strings.EqualFold(item.Status, store.StatusPassed):
		report.Counts.PassedTasks++
	case strings.EqualFold(item.Status, store.StatusFailed):
		report.Counts.FailedTasks++
		report.FailedTasks = append(report.FailedTasks, item)
	case strings.EqualFold(item.Status, mapplanner.TaskStatusBlocked):
		report.Counts.BlockedTasks++
		report.FailedTasks = append(report.FailedTasks, item)
	case strings.EqualFold(item.Status, mapplanner.TaskStatusSkipped):
		report.Counts.SkippedTasks++
	default:
		report.Counts.OtherTasks++
		report.FailedTasks = append(report.FailedTasks, item)
	}
	if item.WorkflowRunID != "" {
		report.Counts.WorkflowRuns++
	}
	if item.APICaseRunID != "" {
		report.Counts.APICaseRuns++
	}
	if item.EvidenceCount > 0 || mapGateTaskEvidenceExempt(item) {
		report.Counts.EvidenceComplete++
		return
	}
	report.MissingEvidence = append(report.MissingEvidence, item)
}

func mapGateTaskEvidenceExempt(item mapGateTaskItem) bool {
	if strings.EqualFold(item.Status, mapplanner.TaskStatusSkipped) {
		return true
	}
	return strings.EqualFold(item.Status, store.StatusPassed) && item.Kind == mapplanner.TaskReuseMaterialized
}

func mapGateNextActions(report mapGateReport, options mapGateOptions) []string {
	actions := []string{}
	if !report.Gates.TasksPresent {
		return []string{"agent-testbench map run explain --plan " + commandline.ShellQuote(report.PlanID) + " --json"}
	}
	if len(report.FailedTasks) > 0 {
		actions = append(actions, "agent-testbench map run --plan "+commandline.ShellQuote(report.PlanID)+" --retry-failed --json")
	}
	for index, item := range report.FailedTasks {
		if index >= 3 {
			break
		}
		if item.APICaseRunID != "" {
			actions = append(actions, "agent-testbench case diagnose --case-run "+commandline.ShellQuote(item.APICaseRunID)+" --json")
		}
	}
	if options.RequireEvidence {
		for index, item := range report.MissingEvidence {
			if index >= 3 {
				break
			}
			actions = append(actions, "agent-testbench map run --plan "+commandline.ShellQuote(report.PlanID)+" --rerun-task "+commandline.ShellQuote(item.ID)+" --json")
		}
	}
	if len(actions) == 0 {
		actions = append(actions, "Map gate passed; no action needed")
	}
	return actions
}

func printMapGate(report mapGateReport) {
	fmt.Println("Map Gate")
	fmt.Printf("OK: %t\n", report.OK)
	fmt.Printf("Plan: %s\n", report.PlanID)
	fmt.Printf("Map: %s\n", report.MapID)
	fmt.Printf("Status: %s\n", report.Status)
	fmt.Printf("Tasks: %d passed=%d failed=%d blocked=%d skipped=%d other=%d evidenceComplete=%d\n", report.Counts.TotalTasks, report.Counts.PassedTasks, report.Counts.FailedTasks, report.Counts.BlockedTasks, report.Counts.SkippedTasks, report.Counts.OtherTasks, report.Counts.EvidenceComplete)
	fmt.Printf("Gates: planPassed=%t tasksPresent=%t tasksPassed=%t evidenceComplete=%t\n", report.Gates.PlanPassed, report.Gates.TasksPresent, report.Gates.TasksPassed, report.Gates.EvidenceComplete)
	for _, item := range report.FailedTasks {
		fmt.Printf("Failed Task: %s %s %s\n", item.ID, item.Kind, item.Status)
	}
	for _, item := range report.MissingEvidence {
		fmt.Printf("Missing Evidence: %s %s %s\n", item.ID, item.Kind, item.Status)
	}
	for _, action := range report.NextActions {
		fmt.Printf("Next: %s\n", action)
	}
}
