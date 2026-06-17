package main

import (
	"context"
	"crypto/sha1"
	"encoding/json"
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
	matByID       map[string]store.TestPlanMaterialization
	statusByTask  map[string]string
	exportsByTask map[string]map[string]any
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
	for _, i := range mapRunTaskExecutionOrder(record.Tasks, record.TaskEdges) {
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
			e.executeCaseTask(record.Instance, task, record.TaskEdges)
		case mapplanner.TaskReuseMaterialized:
			e.executeMaterializedTask(task)
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

func mapRunTaskExecutionOrder(tasks []store.TestMapPlanTask, edges []store.TestMapPlanTaskEdge) []int {
	taskIndex := map[string]int{}
	for i, task := range tasks {
		taskIndex[task.ID] = i
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
	ready := make([]int, 0, len(tasks))
	for i := range tasks {
		if indegree[i] == 0 {
			ready = append(ready, i)
		}
	}
	order := make([]int, 0, len(tasks))
	queued := map[int]bool{}
	for len(ready) > 0 {
		sort.Ints(ready)
		current := ready[0]
		ready = ready[1:]
		if queued[current] {
			continue
		}
		queued[current] = true
		order = append(order, current)
		for _, dependent := range dependents[current] {
			indegree[dependent]--
			if indegree[dependent] == 0 {
				ready = append(ready, dependent)
			}
		}
	}
	for i := range tasks {
		if !queued[i] {
			order = append(order, i)
		}
	}
	return order
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
	runID := e.taskRunID(instance, *task)
	results := make([]mapRunStepResult, 0, len(steps))
	overrides := map[string]any{}
	status := store.StatusPassed
	for _, step := range steps {
		result := e.executeStepCase(instance, *task, step, runID, overrides)
		results = append(results, result)
		if result.Status != store.StatusPassed {
			status = store.StatusFailed
			break
		}
		for key, value := range e.stepExportedValues(*task, step, result.Raw) {
			overrides[key] = value
		}
	}
	if status == store.StatusPassed && len(overrides) > 0 {
		e.exportsByTask[task.ID] = mapRunCopyStringAnyMap(overrides)
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

func (e mapRunExecutor) executeMaterializedTask(task *store.TestMapPlanTask) {
	materialization, ok := e.matByID[task.MaterializationID]
	if !ok {
		e.finishTask(task, store.StatusFailed, map[string]any{"error": "materialization not found: " + task.MaterializationID}, time.Now().UTC())
		return
	}
	fixture, ok, err := e.catalogFixture(materialization.FixtureID)
	if err != nil {
		e.finishTask(task, store.StatusFailed, map[string]any{"error": err.Error()}, time.Now().UTC())
		return
	}
	if !ok {
		e.finishTask(task, store.StatusFailed, map[string]any{"error": "fixture not found: " + materialization.FixtureID}, time.Now().UTC())
		return
	}
	overrides, err := mapRunMaterializedOverrides(fixture.DataJSON, e.materializationMappings(materialization.ID, fixture.ID))
	if err != nil {
		e.finishTask(task, store.StatusFailed, map[string]any{"error": err.Error()}, time.Now().UTC())
		return
	}
	e.exportsByTask[task.ID] = overrides
	e.finishTask(task, store.StatusPassed, map[string]any{
		"materializationId": materialization.ID,
		"fixtureId":         fixture.ID,
		"sourcePathId":      materialization.SourcePathID,
		"sourceWorkflowId":  materialization.SourceWorkflowID,
		"sourceUntilNodeId": materialization.SourceUntilNodeID,
		"overrides":         mapRunSortedKeys(overrides),
	}, time.Now().UTC())
}

func (e mapRunExecutor) catalogFixture(fixtureID string) (store.CatalogFixture, bool, error) {
	catalog, err := e.runtime.GetProfileCatalog(e.ctx)
	if err != nil {
		return store.CatalogFixture{}, false, err
	}
	for _, fixture := range catalog.Fixtures {
		if fixture.ID != fixtureID {
			continue
		}
		if strings.TrimSpace(fixture.Status) != "" && fixture.Status != "active" {
			return store.CatalogFixture{}, false, fmt.Errorf("fixture is not active: %s", fixture.ID)
		}
		return fixture, true, nil
	}
	return store.CatalogFixture{}, false, nil
}

func (e mapRunExecutor) materializationMappings(materializationID string, fixtureID string) []map[string]any {
	mappings := []map[string]any{}
	for _, edge := range e.graph.Edges {
		if edge.MaterializationID == materializationID {
			mappings = append(mappings, listOfMaps(edge.MappingsJSON)...)
		}
	}
	catalog, err := e.runtime.GetProfileCatalog(e.ctx)
	if err != nil {
		return mappings
	}
	for _, dependency := range catalog.CaseDependencies {
		if dependency.FixtureID == fixtureID {
			mappings = append(mappings, listOfMaps(dependency.MappingsJSON)...)
		}
	}
	return mappings
}

func (e mapRunExecutor) dependencyOverrides(edges []store.TestMapPlanTaskEdge, taskID string) map[string]any {
	out := map[string]any{}
	for _, edge := range edges {
		if edge.ToTaskID != taskID || !edge.Required {
			continue
		}
		for key, value := range e.exportsByTask[edge.FromTaskID] {
			out[key] = value
		}
	}
	return out
}

func mapRunMaterializedOverrides(rawData string, mappings []map[string]any) (map[string]any, error) {
	data := map[string]any{}
	if strings.TrimSpace(rawData) != "" {
		if err := json.Unmarshal([]byte(rawData), &data); err != nil {
			return nil, fmt.Errorf("decode materialized fixture data: %w", err)
		}
	}
	out := mapRunCopyStringAnyMap(data)
	for _, mapping := range mappings {
		value := mapRunJSONPathValue(data, valueString(mapping["from"]))
		if value == nil {
			continue
		}
		for _, key := range mapRunOverrideKeys(valueString(mapping["to"])) {
			out[key] = value
		}
	}
	return out, nil
}

func mapRunJSONPathValue(root map[string]any, path string) any {
	path = strings.TrimSpace(path)
	path = strings.TrimPrefix(path, "$.")
	path = strings.TrimPrefix(path, "$")
	if path == "" {
		return root
	}
	return workflowValueAtPath(root, path)
}

func mapRunOverrideKeys(path string) []string {
	trimmed := strings.TrimSpace(path)
	trimmed = strings.TrimPrefix(trimmed, "$.")
	trimmed = strings.TrimPrefix(trimmed, "$")
	if trimmed == "" {
		return nil
	}
	keys := []string{trimmed}
	parts := strings.Split(trimmed, ".")
	leaf := parts[len(parts)-1]
	if leaf != "" && leaf != trimmed {
		keys = append(keys, leaf)
	}
	return keys
}

func listOfMaps(raw string) []map[string]any {
	var items []map[string]any
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	if err := json.Unmarshal([]byte(raw), &items); err != nil {
		return nil
	}
	return items
}

func mapRunSortedKeys(values map[string]any) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func mapRunCopyStringAnyMap(in map[string]any) map[string]any {
	out := map[string]any{}
	for key, value := range in {
		out[key] = value
	}
	return out
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
	if len(overrides) > 0 {
		payload["overrides"] = overrides
	}
	return runCatalogCaseOnRuntime(e.ctx, e.runtime, instance.ProfileID, payload)
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
		if item.ScopeID == caseID || valueString(config["caseId"]) == caseID {
			return config
		}
	}
	return nil
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
