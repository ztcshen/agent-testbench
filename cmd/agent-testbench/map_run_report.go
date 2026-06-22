package main

import (
	"fmt"
	"strings"

	"agent-testbench/internal/domain/commandline"
	"agent-testbench/internal/domain/mapplanner"
	"agent-testbench/internal/store"
)

type mapRunReport struct {
	OK            bool               `json:"ok"`
	PlanID        string             `json:"planId"`
	MapID         string             `json:"mapId"`
	ProfileID     string             `json:"profileId,omitempty"`
	EnvironmentID string             `json:"environmentId,omitempty"`
	Scope         string             `json:"scope"`
	TargetKind    string             `json:"targetKind"`
	TargetID      string             `json:"targetId"`
	Status        string             `json:"status"`
	Summary       mapRunSummary      `json:"summary"`
	Tasks         []mapRunTaskReport `json:"tasks"`
	NextActions   []string           `json:"nextActions,omitempty"`
}

type mapRunSummary struct {
	TotalTasks   int `json:"totalTasks"`
	PassedTasks  int `json:"passedTasks"`
	FailedTasks  int `json:"failedTasks"`
	SkippedTasks int `json:"skippedTasks"`
	BlockedTasks int `json:"blockedTasks"`
	WorkflowRuns int `json:"workflowRuns"`
	APICaseRuns  int `json:"apiCaseRuns"`
}

type mapRunTaskReport struct {
	ID               string         `json:"id"`
	Index            int            `json:"index"`
	Kind             string         `json:"kind"`
	Operation        string         `json:"operation"`
	PathID           string         `json:"pathId,omitempty"`
	WorkflowID       string         `json:"workflowId,omitempty"`
	NodeID           string         `json:"nodeId,omitempty"`
	CaseID           string         `json:"caseId,omitempty"`
	ReplayGroupID    string         `json:"replayGroupId,omitempty"`
	InterfaceNodeID  string         `json:"interfaceNodeId,omitempty"`
	AnchorNodeID     string         `json:"anchorNodeId,omitempty"`
	ValidationFamily string         `json:"validationFamily,omitempty"`
	Status           string         `json:"status"`
	Reason           string         `json:"reason,omitempty"`
	WorkflowRunID    string         `json:"workflowRunId,omitempty"`
	APICaseRunID     string         `json:"apiCaseRunId,omitempty"`
	EvidenceRoot     string         `json:"evidenceRoot,omitempty"`
	Summary          map[string]any `json:"summary,omitempty"`
}

func mapRunStatus(tasks []store.TestMapPlanTask) string {
	status := store.StatusPassed
	for _, task := range tasks {
		switch task.Status {
		case store.StatusFailed:
			return store.StatusFailed
		case mapplanner.TaskStatusBlocked:
			status = mapplanner.TaskStatusBlocked
		case mapplanner.TaskStatusPlanned, mapplanner.TaskStatusRunning:
			if status != mapplanner.TaskStatusBlocked {
				status = mapplanner.TaskStatusRunning
			}
		}
	}
	return status
}

func mapRunSummaryFromTasks(tasks []store.TestMapPlanTask) mapRunSummary {
	summary := mapRunSummary{TotalTasks: len(tasks)}
	for _, task := range tasks {
		switch task.Status {
		case store.StatusPassed:
			summary.PassedTasks++
		case store.StatusFailed:
			summary.FailedTasks++
		case mapplanner.TaskStatusSkipped:
			summary.SkippedTasks++
		case mapplanner.TaskStatusBlocked:
			summary.BlockedTasks++
		}
		if task.WorkflowRunID != "" {
			summary.WorkflowRuns++
		}
		if task.APICaseRunID != "" {
			summary.APICaseRuns++
			continue
		}
		taskSummary := jsonObjectString(task.SummaryJSON)
		summary.APICaseRuns += len(listFromReportAny(taskSummary["steps"]))
		if step := mapFromReportAny(taskSummary["result"]); valueString(step["caseRunId"]) != "" {
			summary.APICaseRuns++
		}
	}
	return summary
}

func mapRunReportFromRecord(record store.TestMapPlanRecord) mapRunReport {
	tasks := make([]mapRunTaskReport, 0, len(record.Tasks))
	for _, task := range record.Tasks {
		summary := jsonObjectString(task.SummaryJSON)
		tasks = append(tasks, mapRunTaskReport{
			ID:               task.ID,
			Index:            task.Index,
			Kind:             task.Kind,
			Operation:        task.Operation,
			PathID:           task.PathID,
			WorkflowID:       task.WorkflowID,
			NodeID:           task.NodeID,
			CaseID:           task.CaseID,
			ReplayGroupID:    valueString(summary["replayGroupId"]),
			InterfaceNodeID:  valueString(summary["interfaceNodeId"]),
			AnchorNodeID:     valueString(summary["anchorNodeId"]),
			ValidationFamily: valueString(summary["validationFamily"]),
			Status:           task.Status,
			Reason:           task.Reason,
			WorkflowRunID:    task.WorkflowRunID,
			APICaseRunID:     task.APICaseRunID,
			EvidenceRoot:     task.EvidenceRoot,
			Summary:          summary,
		})
	}
	status := record.Instance.Status
	if status == "" {
		status = mapRunStatus(record.Tasks)
	}
	return mapRunReport{
		OK:            status == store.StatusPassed || status == mapplanner.TaskStatusSkipped,
		PlanID:        record.Instance.ID,
		MapID:         record.Instance.MapID,
		ProfileID:     record.Instance.ProfileID,
		EnvironmentID: record.Instance.EnvironmentID,
		Scope:         record.Instance.Scope,
		TargetKind:    record.Instance.TargetKind,
		TargetID:      record.Instance.TargetID,
		Status:        status,
		Summary:       mapRunSummaryFromTasks(record.Tasks),
		Tasks:         tasks,
		NextActions:   mapRunNextActions(record.Instance.ID),
	}
}

func mapRunNextActions(planID string) []string {
	if strings.TrimSpace(planID) == "" {
		return nil
	}
	return []string{"agent-testbench map inspect --view plan --plan " + commandline.ShellQuote(planID) + " --json"}
}

func printMapRunReport(report mapRunReport) {
	fmt.Printf("Map Run: %s\n", report.PlanID)
	fmt.Printf("Map: %s\n", report.MapID)
	fmt.Printf("Status: %s\n", report.Status)
	fmt.Printf("Tasks: %d passed=%d failed=%d skipped=%d blocked=%d\n", report.Summary.TotalTasks, report.Summary.PassedTasks, report.Summary.FailedTasks, report.Summary.SkippedTasks, report.Summary.BlockedTasks)
	for _, task := range report.Tasks {
		fmt.Printf("- %s [%s] %s\n", task.ID, task.Kind, task.Status)
	}
}
