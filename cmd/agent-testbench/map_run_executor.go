package main

import (
	"context"
	"fmt"
	"net/http/httptest"
	"sort"
	"strings"
	"time"

	"agent-testbench/internal/domain/mapplanner"
	"agent-testbench/internal/domain/profile"
	"agent-testbench/internal/server/controlplane"
	"agent-testbench/internal/store"
)

type mapRunExecutor struct {
	ctx     context.Context
	runtime store.Store
	graph   store.TestPlanGraph
	options mapRunOptions

	pathByID      map[string]store.TestPlanPath
	nodeByID      map[string]store.TestPlanNode
	pathStepsByID map[string][]store.TestPlanPathStep
	statusByTask  map[string]string
}

type mapRunStepResult struct {
	StepID       string `json:"stepId"`
	NodeID       string `json:"nodeId"`
	CaseID       string `json:"caseId"`
	RunID        string `json:"runId"`
	APICaseRunID string `json:"apiCaseRunId"`
	Status       string `json:"status"`
	Error        string `json:"error,omitempty"`
}

func newMapRunExecutor(ctx context.Context, runtime store.Store, graph store.TestPlanGraph, options mapRunOptions) mapRunExecutor {
	executor := mapRunExecutor{
		ctx:           ctx,
		runtime:       runtime,
		graph:         graph,
		options:       options,
		pathByID:      map[string]store.TestPlanPath{},
		nodeByID:      map[string]store.TestPlanNode{},
		pathStepsByID: map[string][]store.TestPlanPathStep{},
		statusByTask:  map[string]string{},
	}
	for _, path := range graph.Paths {
		executor.pathByID[path.ID] = path
	}
	for _, node := range graph.Nodes {
		executor.nodeByID[node.ID] = node
	}
	for _, step := range graph.PathSteps {
		executor.pathStepsByID[step.PathID] = append(executor.pathStepsByID[step.PathID], step)
	}
	for pathID := range executor.pathStepsByID {
		sort.SliceStable(executor.pathStepsByID[pathID], func(i, j int) bool {
			return executor.pathStepsByID[pathID][i].StepIndex < executor.pathStepsByID[pathID][j].StepIndex
		})
	}
	return executor
}

func (e mapRunExecutor) execute(record store.TestMapPlanRecord) store.TestMapPlanRecord {
	now := time.Now().UTC()
	for i := range record.Tasks {
		task := &record.Tasks[i]
		if task.Status == mapplanner.TaskStatusSkipped || task.Kind == mapplanner.TaskSkip {
			e.statusByTask[task.ID] = mapplanner.TaskStatusSkipped
			continue
		}
		if !mapRunTaskSelectedForExecution(*task, e.options) || !mapRunTaskRunnable(*task, e.options) {
			e.statusByTask[task.ID] = task.Status
			continue
		}
		if blockedReason := e.blockedByDependency(record.TaskEdges, task.ID); blockedReason != "" {
			e.finishTask(task, mapplanner.TaskStatusBlocked, map[string]any{"error": blockedReason}, now)
			e.statusByTask[task.ID] = task.Status
			continue
		}
		task.StartedAt = time.Now().UTC()
		task.Status = mapplanner.TaskStatusRunning
		switch task.Kind {
		case mapplanner.TaskRunPath:
			e.executePathTask(record.Instance, task, "")
		case mapplanner.TaskRunPathPrefix:
			e.executePathTask(record.Instance, task, taskUntilNodeID(*task))
		case mapplanner.TaskRunCase:
			e.executeCaseTask(record.Instance, task)
		default:
			e.finishTask(task, mapplanner.TaskStatusSkipped, map[string]any{"reason": "unsupported task kind skipped"}, task.StartedAt)
		}
		e.statusByTask[task.ID] = task.Status
	}
	record.Instance.FinishedAt = time.Now().UTC()
	record.Instance.Status = mapRunStatus(record.Tasks)
	record.Instance.SummaryJSON = mustCompactJSON(mapRunSummaryFromTasks(record.Tasks))
	return record
}

func mapRunTaskRunnable(task store.TestMapPlanTask, options mapRunOptions) bool {
	if task.Status == "" || task.Status == mapplanner.TaskStatusPlanned || task.Status == mapplanner.TaskStatusRunning {
		return true
	}
	return options.retryFailed && mapRunTaskFailedOrBlocked(task.Status)
}

func (e mapRunExecutor) blockedByDependency(edges []store.TestMapPlanTaskEdge, taskID string) string {
	for _, edge := range edges {
		if edge.ToTaskID != taskID || !edge.Required {
			continue
		}
		status := e.statusByTask[edge.FromTaskID]
		if status == store.StatusPassed || status == mapplanner.TaskStatusSkipped {
			continue
		}
		return "required dependency did not pass: " + edge.FromTaskID
	}
	return ""
}

func (e mapRunExecutor) executePathTask(instance store.TestMapPlanInstance, task *store.TestMapPlanTask, untilNodeID string) {
	steps := e.stepsForTask(*task, untilNodeID)
	runID := "run." + safeReportID(instance.ID) + "." + safeReportID(task.ID)
	results := make([]mapRunStepResult, 0, len(steps))
	status := store.StatusPassed
	for _, step := range steps {
		result := e.executeStepCase(instance, *task, step, runID)
		results = append(results, result)
		if result.Status != store.StatusPassed {
			status = store.StatusFailed
			break
		}
	}
	finishedAt := time.Now().UTC()
	if len(steps) == 0 {
		e.finishTask(task, mapplanner.TaskStatusSkipped, map[string]any{"steps": results, "reason": "path has no executable steps"}, finishedAt)
		return
	}
	_, err := e.runtime.CreateRun(e.ctx, store.Run{
		ID:                 runID,
		ProfileID:          instance.ProfileID,
		EnvironmentID:      instance.EnvironmentID,
		WorkflowID:         task.WorkflowID,
		Status:             status,
		TestPlanMapID:      instance.MapID,
		TestPlanPathID:     task.PathID,
		PlannerSummaryJSON: mustCompactJSON(map[string]any{"planId": instance.ID, "taskId": task.ID, "kind": task.Kind}),
		SummaryJSON:        mustCompactJSON(map[string]any{"kind": task.Kind, "steps": results}),
		StartedAt:          task.StartedAt,
		FinishedAt:         finishedAt,
		CreatedAt:          task.StartedAt,
		UpdatedAt:          finishedAt,
	})
	task.WorkflowRunID = runID
	if err != nil {
		status = store.StatusFailed
	}
	summary := map[string]any{"steps": results}
	if err != nil {
		summary["error"] = err.Error()
	}
	e.finishTask(task, status, summary, finishedAt)
}

func (e mapRunExecutor) executeCaseTask(instance store.TestMapPlanInstance, task *store.TestMapPlanTask) {
	runID := "run." + safeReportID(instance.ID) + "." + safeReportID(task.ID)
	result, err := e.runCatalogCase(instance, *task, mapRunCaseStep(*task), runID)
	summary := map[string]any{"result": result}
	status := valueString(result["status"])
	if status == "" {
		status = store.StatusFailed
	}
	if err != nil {
		status = store.StatusFailed
		summary["error"] = err.Error()
	}
	task.APICaseRunID = runID + ".case"
	task.EvidenceRoot = mapRunEvidenceRoot(result)
	e.finishTask(task, status, summary, time.Now().UTC())
}

func (e mapRunExecutor) executeStepCase(instance store.TestMapPlanInstance, task store.TestMapPlanTask, step store.TestPlanPathStep, workflowRunID string) mapRunStepResult {
	caseID := firstNonEmpty(step.CaseID, e.nodeByID[step.NodeID].CaseID)
	stepID := firstNonEmpty(step.StepID, step.NodeID, caseID)
	runID := workflowRunID + "." + safeReportID(stepID)
	stepTask := task
	stepTask.NodeID = firstNonEmpty(step.NodeID, stepTask.NodeID)
	stepTask.CaseID = caseID
	result, err := e.runCatalogCase(instance, stepTask, step, runID)
	status := valueString(result["status"])
	if status == "" {
		status = store.StatusFailed
	}
	out := mapRunStepResult{
		StepID:       stepID,
		NodeID:       step.NodeID,
		CaseID:       caseID,
		RunID:        runID,
		APICaseRunID: runID + ".case",
		Status:       status,
	}
	if err != nil {
		out.Status = store.StatusFailed
		out.Error = err.Error()
	} else if errText := valueString(result["error"]); errText != "" {
		out.Error = errText
	}
	return out
}

func (e mapRunExecutor) runCatalogCase(instance store.TestMapPlanInstance, task store.TestMapPlanTask, step store.TestPlanPathStep, runID string) (map[string]any, error) {
	caseID := firstNonEmpty(step.CaseID, task.CaseID)
	payload := map[string]any{
		"caseId":             caseID,
		"runId":              runID,
		"workflowId":         firstNonEmpty(task.WorkflowID, caseID),
		"stepId":             firstNonEmpty(step.StepID, task.NodeID, task.CaseID),
		"baseUrl":            e.options.baseURL,
		"evidenceDir":        e.options.evidenceDir,
		"environmentId":      instance.EnvironmentID,
		"testPlanMapId":      instance.MapID,
		"testPlanPathId":     task.PathID,
		"testPlanNodeId":     firstNonEmpty(step.NodeID, task.NodeID),
		"testPlanOperation":  task.Operation,
		"plannerSummary":     map[string]any{"planId": instance.ID, "taskId": task.ID, "taskKind": task.Kind, "pathId": task.PathID},
		"timeoutSeconds":     e.options.timeoutSeconds,
		"inlineTraceCollect": false,
	}
	if e.options.timeoutSeconds <= 0 {
		delete(payload, "timeoutSeconds")
	}
	return runCatalogCaseOnRuntime(e.ctx, e.runtime, instance.ProfileID, payload)
}

func runCatalogCaseOnRuntime(ctx context.Context, runtime store.Store, profileID string, payload map[string]any) (map[string]any, error) {
	handler := controlplane.NewWithStore(profile.Bundle{ID: strings.TrimSpace(profileID)}, runtime)
	server := httptest.NewServer(handler)
	defer server.Close()
	result, err := postReportMapWithContext(ctx, server.URL+"/api/test-kit/run", payload)
	if err != nil {
		return nil, err
	}
	status := intFromReportAny(result["httpStatus"])
	if status < 200 || status >= 300 {
		return result, fmt.Errorf("case run failed with http status %d: %s", status, valueString(result["error"]))
	}
	return result, nil
}

func (e mapRunExecutor) stepsForTask(task store.TestMapPlanTask, untilNodeID string) []store.TestPlanPathStep {
	steps := e.pathStepsByID[task.PathID]
	if strings.TrimSpace(untilNodeID) == "" {
		return append([]store.TestPlanPathStep(nil), steps...)
	}
	for i, step := range steps {
		if step.NodeID == untilNodeID {
			return append([]store.TestPlanPathStep(nil), steps[:i+1]...)
		}
	}
	return nil
}

func (e mapRunExecutor) finishTask(task *store.TestMapPlanTask, status string, summary map[string]any, finishedAt time.Time) {
	task.Status = status
	task.FinishedAt = finishedAt
	if task.StartedAt.IsZero() {
		task.StartedAt = finishedAt
	}
	if len(summary) > 0 {
		task.SummaryJSON = mustCompactJSON(summary)
	}
	if status == store.StatusFailed || status == mapplanner.TaskStatusBlocked {
		task.Reason = firstNonEmpty(valueString(summary["error"]), task.Reason)
	}
}

func taskUntilNodeID(task store.TestMapPlanTask) string {
	return valueString(jsonObjectString(task.SummaryJSON)["untilNodeId"])
}

func mapRunCaseStep(task store.TestMapPlanTask) store.TestPlanPathStep {
	return store.TestPlanPathStep{
		PathID: task.PathID,
		NodeID: task.NodeID,
		CaseID: task.CaseID,
		StepID: firstNonEmpty(task.NodeID, task.CaseID),
	}
}

func mapRunEvidenceRoot(result map[string]any) string {
	if root := valueString(result["evidenceRoot"]); strings.TrimSpace(root) != "" {
		return root
	}
	viewer := valueString(result["viewerUrl"])
	if strings.TrimSpace(viewer) == "" {
		return ""
	}
	return viewer
}
