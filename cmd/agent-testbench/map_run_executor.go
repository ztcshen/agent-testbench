package main

import (
	"context"
	"crypto/sha1"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
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
	matByID       map[string]store.TestPlanMaterialization
	statusByTask  map[string]string
	exportsByTask map[string]map[string]any
	checkpoint    *mapRunCheckpointState
	lease         *mapRunLeaseState
	mu            *sync.Mutex
}

type mapRunCheckpointState struct {
	err error
}

type mapRunStepResult struct {
	StepID       string         `json:"stepId"`
	NodeID       string         `json:"nodeId"`
	CaseID       string         `json:"caseId"`
	RunID        string         `json:"runId"`
	APICaseRunID string         `json:"apiCaseRunId"`
	Status       string         `json:"status"`
	Error        string         `json:"error,omitempty"`
	Raw          map[string]any `json:"-"`
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
		matByID:       map[string]store.TestPlanMaterialization{},
		statusByTask:  map[string]string{},
		exportsByTask: map[string]map[string]any{},
		checkpoint:    &mapRunCheckpointState{},
		mu:            &sync.Mutex{},
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
	for _, materialization := range graph.Materializations {
		executor.matByID[materialization.ID] = materialization
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
	concurrency := e.options.concurrency
	if concurrency <= 0 {
		concurrency = 1
	}
	for _, wave := range mapRunTaskExecutionWaves(record.Tasks, record.TaskEdges) {
		if e.executionStopped() {
			break
		}
		if concurrency == 1 || len(wave) == 1 {
			for _, index := range wave {
				if e.executionStopped() {
					break
				}
				e.executeTask(record.Instance, record.TaskEdges, &record.Tasks[index], now)
			}
			continue
		}
		limit := make(chan struct{}, concurrency)
		var group sync.WaitGroup
		for _, index := range wave {
			index := index
			group.Add(1)
			go func() {
				defer group.Done()
				if e.executionStopped() {
					return
				}
				limit <- struct{}{}
				defer func() { <-limit }()
				if e.executionStopped() {
					return
				}
				e.executeTask(record.Instance, record.TaskEdges, &record.Tasks[index], now)
			}()
		}
		group.Wait()
	}
	record.Instance.FinishedAt = time.Now().UTC()
	record.Instance.Status = mapRunStatus(record.Tasks)
	record.Instance.SummaryJSON = mustCompactJSON(mapRunSummaryFromTasks(record.Tasks))
	if e.lease != nil {
		e.recordLeaseError(e.finishClaimedInstance(record.Instance))
	} else {
		e.checkpointInstance(record.Instance)
	}
	return record
}

func (e mapRunExecutor) executeTask(instance store.TestMapPlanInstance, edges []store.TestMapPlanTaskEdge, task *store.TestMapPlanTask, now time.Time) {
	if e.executionStopped() {
		return
	}
	if task.Status == mapplanner.TaskStatusSkipped || task.Kind == mapplanner.TaskSkip {
		e.restoreTaskExports(*task)
		e.setTaskStatus(task.ID, mapplanner.TaskStatusSkipped)
		return
	}
	if !mapRunTaskSelectedForExecution(*task, e.options) || !mapRunTaskRunnable(*task, e.options) {
		e.restoreTaskExports(*task)
		e.setTaskStatus(task.ID, task.Status)
		return
	}
	if blockedReason := e.blockedByDependency(edges, task.ID); blockedReason != "" {
		e.finishTask(task, mapplanner.TaskStatusBlocked, map[string]any{"error": blockedReason}, now)
		e.checkpointTask(*task)
		e.setTaskStatus(task.ID, task.Status)
		return
	}
	task.StartedAt = time.Now().UTC()
	task.Status = mapplanner.TaskStatusRunning
	e.checkpointTask(*task)
	if err := e.executionError(); err != nil {
		e.finishTask(task, store.StatusFailed, map[string]any{
			"error":           "persist running task checkpoint: " + err.Error(),
			"failureCategory": "checkpoint-persistence-error",
		}, time.Now().UTC())
		e.setTaskStatus(task.ID, task.Status)
		return
	}
	switch task.Kind {
	case mapplanner.TaskRunPath:
		e.executePathTask(instance, task, "")
	case mapplanner.TaskRunPathPrefix:
		e.executePathTask(instance, task, taskUntilNodeID(*task))
	case mapplanner.TaskRunCase:
		e.executeCaseTask(instance, task, edges)
	case mapplanner.TaskReuseMaterialized:
		e.executeMaterializedTask(task)
	default:
		e.finishTask(task, store.StatusFailed, map[string]any{"error": "unsupported map task kind: " + task.Kind}, task.StartedAt)
	}
	e.setTaskStatus(task.ID, task.Status)
	e.checkpointTask(*task)
}

func (e mapRunExecutor) checkpointTask(task store.TestMapPlanTask) {
	if e.runtime == nil || e.checkpoint == nil {
		return
	}
	e.mu.Lock()
	failed := e.checkpoint.err != nil
	e.mu.Unlock()
	if failed {
		return
	}
	if e.lease != nil {
		e.recordLeaseError(e.checkpointClaimedTask(task))
		return
	}
	checkpointStore, ok := e.runtime.(store.MapPlannerCheckpointStore)
	if !ok {
		e.recordCheckpointError(errors.New("Store does not support test map task checkpoints"))
		return
	}
	e.recordCheckpointError(checkpointStore.UpdateTestMapPlanTask(e.ctx, task))
}

func (e mapRunExecutor) checkpointInstance(instance store.TestMapPlanInstance) {
	if e.runtime == nil || e.checkpoint == nil {
		return
	}
	e.mu.Lock()
	failed := e.checkpoint.err != nil
	e.mu.Unlock()
	if failed {
		return
	}
	checkpointStore, ok := e.runtime.(store.MapPlannerCheckpointStore)
	if !ok {
		e.recordCheckpointError(errors.New("Store does not support test map plan checkpoints"))
		return
	}
	e.recordCheckpointError(checkpointStore.UpdateTestMapPlanInstance(e.ctx, instance))
}

func (e mapRunExecutor) checkpointError() error {
	if e.checkpoint == nil {
		return nil
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.checkpoint.err
}

func (e mapRunExecutor) executionError() error {
	if err := e.checkpointError(); err != nil {
		return err
	}
	return context.Cause(e.ctx)
}

func (e mapRunExecutor) executionStopped() bool {
	return e.executionError() != nil
}

func (e mapRunExecutor) recordCheckpointError(err error) {
	if err == nil {
		return
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.checkpoint.err == nil {
		e.checkpoint.err = err
	}
}

func (e mapRunExecutor) setTaskStatus(taskID string, status string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.statusByTask[taskID] = status
}

func (e mapRunExecutor) taskStatus(taskID string) string {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.statusByTask[taskID]
}

func (e mapRunExecutor) setTaskExports(taskID string, exports map[string]any) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.exportsByTask[taskID] = mapRunCopyStringAnyMap(exports)
}

func (e mapRunExecutor) taskExports(taskID string) map[string]any {
	e.mu.Lock()
	defer e.mu.Unlock()
	return mapRunCopyStringAnyMap(e.exportsByTask[taskID])
}

func mapRunTaskExecutionWaves(tasks []store.TestMapPlanTask, edges []store.TestMapPlanTaskEdge) [][]int {
	taskIndex := map[string]int{}
	for index, task := range tasks {
		taskIndex[task.ID] = index
	}
	indegree := make([]int, len(tasks))
	dependents := map[int][]int{}
	for _, edge := range edges {
		if !edge.Required {
			continue
		}
		from, fromOK := taskIndex[edge.FromTaskID]
		to, toOK := taskIndex[edge.ToTaskID]
		if !fromOK || !toOK {
			continue
		}
		indegree[to]++
		dependents[from] = append(dependents[from], to)
	}
	ready := []int{}
	for index := range tasks {
		if indegree[index] == 0 {
			ready = append(ready, index)
		}
	}
	waves := [][]int{}
	queued := map[int]bool{}
	for len(ready) > 0 {
		sort.Ints(ready)
		wave := append([]int(nil), ready...)
		waves = append(waves, wave)
		next := []int{}
		for _, current := range wave {
			queued[current] = true
			for _, dependent := range dependents[current] {
				indegree[dependent]--
				if indegree[dependent] == 0 {
					next = append(next, dependent)
				}
			}
		}
		ready = next
	}
	for index := range tasks {
		if !queued[index] {
			waves = append(waves, []int{index})
		}
	}
	return waves
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
		status := e.taskStatus(edge.FromTaskID)
		if status == store.StatusPassed || status == mapplanner.TaskStatusSkipped {
			continue
		}
		return "required dependency did not pass: " + edge.FromTaskID
	}
	return ""
}

func (e mapRunExecutor) executePathTask(instance store.TestMapPlanInstance, task *store.TestMapPlanTask, untilNodeID string) {
	steps := e.stepsForTask(*task, untilNodeID)
	runID := e.taskRunID(instance, *task)
	results := make([]mapRunStepResult, 0, len(steps))
	overrides := map[string]any{}
	exports := map[string]any{}
	status := store.StatusPassed
	for _, step := range steps {
		if err := e.executionError(); err != nil {
			status = store.StatusFailed
			break
		}
		result := e.executeStepCase(instance, *task, step, runID, overrides)
		results = append(results, result)
		task.SummaryJSON = mustCompactJSON(map[string]any{"steps": results, "checkpoint": true})
		e.checkpointTask(*task)
		if err := e.executionError(); err != nil {
			results[len(results)-1].Status = store.StatusFailed
			results[len(results)-1].Error = "persist step checkpoint: " + err.Error()
			status = store.StatusFailed
			break
		}
		if result.Status != store.StatusPassed {
			status = store.StatusFailed
			break
		}
		for key, value := range e.stepExportedValues(*task, step, result.Raw) {
			overrides[key] = value
		}
	}
	if status == store.StatusPassed && len(overrides) > 0 {
		exports = mapRunCopyStringAnyMap(overrides)
		e.setTaskExports(task.ID, exports)
	}
	finishedAt := time.Now().UTC()
	if len(steps) == 0 {
		e.finishTask(task, mapplanner.TaskStatusSkipped, map[string]any{"steps": results, "reason": "path has no executable steps"}, finishedAt)
		return
	}
	runSummary := map[string]any{"kind": task.Kind, "steps": results}
	taskSummary := map[string]any{"steps": results}
	if len(exports) > 0 {
		runSummary["exports"] = exports
		taskSummary["exports"] = exports
	}
	if err := e.fenceMapRunOwnership(); err != nil {
		taskSummary["error"] = "map plan ownership changed before aggregate run persistence: " + err.Error()
		e.finishTask(task, store.StatusFailed, taskSummary, finishedAt)
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
		SummaryJSON:        mustCompactJSON(runSummary),
		StartedAt:          task.StartedAt,
		FinishedAt:         finishedAt,
		CreatedAt:          task.StartedAt,
		UpdatedAt:          finishedAt,
	})
	task.WorkflowRunID = runID
	if err != nil {
		status = store.StatusFailed
	}
	summary := taskSummary
	if status == store.StatusFailed {
		for _, result := range results {
			if result.Error != "" {
				summary["error"] = result.Error
				break
			}
		}
	}
	if err != nil {
		summary["error"] = err.Error()
	}
	e.finishTask(task, status, summary, finishedAt)
}

func (e mapRunExecutor) executeCaseTask(instance store.TestMapPlanInstance, task *store.TestMapPlanTask, edges []store.TestMapPlanTaskEdge) {
	runID := e.taskRunID(instance, *task)
	result, err := e.runCatalogCase(instance, *task, mapRunCaseStep(*task), runID, e.dependencyOverrides(edges, task.ID))
	summary := map[string]any{"result": result}
	status := valueString(result["status"])
	if status == "" {
		status = store.StatusFailed
	}
	if err != nil {
		status = store.StatusFailed
		summary["error"] = err.Error()
	}
	task.APICaseRunID = valueString(result["caseRunId"])
	task.EvidenceRoot = mapRunEvidenceRoot(result)
	e.finishTask(task, status, summary, time.Now().UTC())
}

func (e mapRunExecutor) dependencyOverrides(edges []store.TestMapPlanTaskEdge, taskID string) map[string]any {
	out := map[string]any{}
	for _, edge := range edges {
		if edge.ToTaskID != taskID || !edge.Required {
			continue
		}
		for key, value := range e.taskExports(edge.FromTaskID) {
			out[key] = value
		}
	}
	return out
}

func (e mapRunExecutor) restoreTaskExports(task store.TestMapPlanTask) {
	if task.Status != store.StatusPassed {
		return
	}
	exports := mapFromReportAny(jsonObjectString(task.SummaryJSON)["exports"])
	if len(exports) == 0 {
		return
	}
	e.setTaskExports(task.ID, exports)
}

func (e mapRunExecutor) executeStepCase(instance store.TestMapPlanInstance, task store.TestMapPlanTask, step store.TestPlanPathStep, workflowRunID string, overrides map[string]any) mapRunStepResult {
	caseID := firstNonEmpty(step.CaseID, e.nodeByID[step.NodeID].CaseID)
	stepID := firstNonEmpty(step.StepID, step.NodeID, caseID)
	runID := workflowRunID + "." + safeBoundedReportID(stepID, 40)
	stepTask := task
	stepTask.NodeID = firstNonEmpty(step.NodeID, stepTask.NodeID)
	stepTask.CaseID = caseID
	result, err := e.runCatalogCase(instance, stepTask, step, runID, overrides)
	status := valueString(result["status"])
	if status == "" {
		status = store.StatusFailed
	}
	out := mapRunStepResult{
		StepID:       stepID,
		NodeID:       step.NodeID,
		CaseID:       caseID,
		RunID:        runID,
		APICaseRunID: valueString(result["caseRunId"]),
		Status:       status,
		Raw:          result,
	}
	if err != nil {
		out.Status = store.StatusFailed
		out.Error = err.Error()
	} else if errText := valueString(result["error"]); errText != "" {
		out.Error = errText
	}
	return out
}

func (e mapRunExecutor) runCatalogCase(instance store.TestMapPlanInstance, task store.TestMapPlanTask, step store.TestPlanPathStep, runID string, overrides map[string]any) (map[string]any, error) {
	caseID := firstNonEmpty(step.CaseID, task.CaseID)
	runner := e.mapCaseRunnerForCase(caseID)
	request := mapCaseRunRequest{
		Instance:  instance,
		Task:      task,
		Step:      step,
		CaseID:    caseID,
		RunID:     runID,
		Overrides: overrides,
	}
	if err := e.fenceMapRunOwnership(); err != nil {
		return map[string]any{"status": store.StatusFailed, "error": err.Error()}, err
	}
	result, err := runner.Run(e.ctx, request)
	if leaseErr := e.executionError(); leaseErr != nil {
		return result, fmt.Errorf("map plan ownership changed during case execution: %w", leaseErr)
	}
	return result, err
}

func (e mapRunExecutor) catalogCasePayload(request mapCaseRunRequest) map[string]any {
	payload := map[string]any{
		"caseId":             request.CaseID,
		"runId":              request.RunID,
		"workflowId":         firstNonEmpty(request.Task.WorkflowID, request.CaseID),
		"stepId":             firstNonEmpty(request.Step.StepID, request.Task.NodeID, request.Task.CaseID),
		"baseUrl":            e.options.baseURL,
		"evidenceDir":        e.options.evidenceDir,
		"environmentId":      request.Instance.EnvironmentID,
		"testPlanMapId":      request.Instance.MapID,
		"testPlanPathId":     request.Task.PathID,
		"testPlanNodeId":     firstNonEmpty(request.Step.NodeID, request.Task.NodeID),
		"testPlanOperation":  request.Task.Operation,
		"plannerSummary":     map[string]any{"planId": request.Instance.ID, "taskId": request.Task.ID, "taskKind": request.Task.Kind, "pathId": request.Task.PathID},
		"timeoutSeconds":     e.options.timeoutSeconds,
		"inlineTraceCollect": false,
	}
	if e.options.timeoutSeconds <= 0 {
		delete(payload, "timeoutSeconds")
	}
	if len(request.Overrides) > 0 {
		payload["overrides"] = request.Overrides
	}
	return payload
}

func (e mapRunExecutor) mapCaseRunnerForCase(caseID string) mapCaseRunner {
	apiCase, ok := e.catalogAPICase(caseID)
	if !ok {
		return mapHTTPCaseRunner{executor: e}
	}
	runnerID := mapCaseRunnerID(apiCase)
	if mapCaseRunnerSupportedByHTTP(runnerID) {
		return mapHTTPCaseRunner{executor: e}
	}
	return unsupportedMapCaseRunner{runnerID: runnerID, sourceKind: apiCase.SourceKind}
}

func (e mapRunExecutor) catalogAPICase(caseID string) (store.CatalogAPICase, bool) {
	catalog, err := e.runtime.GetProfileCatalog(e.ctx)
	if err != nil {
		return store.CatalogAPICase{}, false
	}
	for _, apiCase := range catalog.APICases {
		if apiCase.ID == caseID {
			return apiCase, true
		}
	}
	return store.CatalogAPICase{}, false
}

func mapCaseRunnerID(apiCase store.CatalogAPICase) string {
	if strings.TrimSpace(apiCase.ExecutorID) != "" {
		return strings.TrimSpace(apiCase.ExecutorID)
	}
	if strings.TrimSpace(apiCase.SourceKind) != "" {
		return strings.TrimSpace(apiCase.SourceKind)
	}
	return mapCaseRunnerHTTP
}

func mapCaseRunnerSupportedByHTTP(runnerID string) bool {
	switch strings.ToLower(strings.TrimSpace(runnerID)) {
	case "", mapCaseRunnerHTTP, mapCaseRunnerHTTPS, mapCaseRunnerOpenAPI, mapCaseRunnerKarate, mapCaseRunnerExecutorHTTP, mapCaseRunnerExecutorOpenAPI, mapCaseRunnerExecutorKarate:
		return true
	default:
		return false
	}
}

func (e mapRunExecutor) taskRunID(instance store.TestMapPlanInstance, task store.TestMapPlanTask) string {
	runID := "run." + safeBoundedReportID(instance.ID, 96) + "." + safeBoundedReportID(task.ID, 64)
	if strings.TrimSpace(e.options.planID) != "" {
		runID += ".attempt." + time.Now().UTC().Format("20060102T150405.000000000Z")
	}
	return runID
}

func safeBoundedReportID(value string, limit int) string {
	safe := safeReportID(value)
	if limit <= 0 || len(safe) <= limit {
		return safe
	}
	sum := sha1.Sum([]byte(safe))
	hash := fmt.Sprintf("%x", sum[:4])
	prefixLimit := limit - len(hash) - 1
	if prefixLimit < 1 {
		return hash[:limit]
	}
	return safe[:prefixLimit] + "-" + hash
}

func (e mapRunExecutor) stepExportedValues(task store.TestMapPlanTask, step store.TestPlanPathStep, result map[string]any) map[string]any {
	config := e.stepExecutionConfig(task, step)
	if len(config) == 0 {
		return nil
	}
	return workflowExportedValues(config, result)
}

func (e mapRunExecutor) stepExecutionConfig(task store.TestMapPlanTask, step store.TestPlanPathStep) map[string]any {
	if e.runtime == nil {
		return nil
	}
	catalog, err := e.runtime.GetProfileCatalog(e.ctx)
	if err != nil {
		return nil
	}
	caseID := firstNonEmpty(step.CaseID, task.CaseID)
	var caseConfig map[string]any
	for _, item := range catalog.TemplateConfigs {
		if strings.TrimSpace(item.Status) != "" && item.Status != "active" {
			continue
		}
		config := map[string]any{}
		if err := json.Unmarshal([]byte(item.ConfigJSON), &config); err != nil {
			continue
		}
		if item.WorkflowID == task.WorkflowID && strings.TrimSpace(step.StepID) != "" && item.ScopeID == step.StepID {
			return config
		}
		if caseConfig == nil && (item.ScopeID == caseID || valueString(config["caseId"]) == caseID) {
			caseConfig = config
		}
	}
	return caseConfig
}

func runCatalogCaseOnRuntime(ctx context.Context, runtime store.Store, profileID string, payload map[string]any) (map[string]any, error) {
	result, err := controlplane.RunTrustedTestKitCase(ctx, profile.Bundle{ID: strings.TrimSpace(profileID)}, runtime, controlplane.TrustedTestKitRunRequest{
		CaseID:             valueString(payload["caseId"]),
		WorkflowID:         valueString(payload["workflowId"]),
		StepID:             valueString(payload["stepId"]),
		Overrides:          mapFromReportAny(payload["overrides"]),
		TimeoutSeconds:     intFromReportAny(payload["timeoutSeconds"]),
		BaseURL:            valueString(payload["baseUrl"]),
		EvidenceDir:        valueString(payload["evidenceDir"]),
		RunID:              valueString(payload["runId"]),
		EnvironmentID:      valueString(payload["environmentId"]),
		TestPlanMapID:      valueString(payload["testPlanMapId"]),
		TestPlanPathID:     valueString(payload["testPlanPathId"]),
		TestPlanNodeID:     valueString(payload["testPlanNodeId"]),
		TestPlanOperation:  valueString(payload["testPlanOperation"]),
		PlannerSummary:     mapFromReportAny(payload["plannerSummary"]),
		InlineTraceCollect: boolFromReportAny(payload["inlineTraceCollect"]),
	})
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
		if task.Kind == mapplanner.TaskRunPathPrefix {
			return nil
		}
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
		summary = mapRunTaskSummaryWithPlannerMetadata(task.SummaryJSON, summary)
		task.SummaryJSON = mustCompactJSON(summary)
	}
	if status == store.StatusFailed || status == mapplanner.TaskStatusBlocked {
		task.Reason = firstNonEmpty(valueString(summary["error"]), task.Reason)
	}
}

func mapRunTaskSummaryWithPlannerMetadata(existingRaw string, summary map[string]any) map[string]any {
	existing := jsonObjectString(existingRaw)
	out := map[string]any{}
	for key, value := range summary {
		out[key] = value
	}
	for _, key := range []string{"replayGroupId", "interfaceNodeId", "anchorNodeId", "validationFamily"} {
		if _, ok := out[key]; ok {
			continue
		}
		if value := valueString(existing[key]); value != "" {
			out[key] = value
		}
	}
	return out
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
