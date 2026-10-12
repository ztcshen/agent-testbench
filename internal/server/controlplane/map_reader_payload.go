package controlplane

import (
	"agent-testbench/internal/domain/redaction"
	"agent-testbench/internal/store"
)

func mapReadContext(graph store.TestPlanGraph, catalog store.ProfileCatalog) map[string]any {
	caseIDs := map[string]bool{}
	for _, node := range graph.Nodes {
		caseIDs[node.CaseID], caseIDs[node.BaseCaseID] = true, true
	}
	for _, step := range graph.PathSteps {
		caseIDs[step.CaseID] = true
	}
	cases := []map[string]any{}
	nodeIDs := map[string]bool{}
	for _, item := range catalog.APICases {
		if !caseIDs[item.ID] {
			continue
		}
		nodeIDs[item.NodeID] = true
		cases = append(cases, map[string]any{"id": item.ID, "displayName": item.DisplayName, "description": item.Description, "nodeId": item.NodeID, "requestTemplateId": item.RequestTemplateID, "caseType": item.CaseType, "scenario": item.Scenario, "tags": item.Tags, "status": item.Status, "priority": item.Priority})
	}
	for _, node := range graph.Nodes {
		nodeIDs[node.InterfaceNodeID] = true
	}
	interfaces := []map[string]any{}
	for _, item := range catalog.InterfaceNodes {
		if nodeIDs[item.ID] {
			interfaces = append(interfaces, map[string]any{"id": item.ID, "displayName": item.DisplayName, "method": item.Method, "path": item.Path, "serviceId": item.ServiceID, "description": item.Description, "status": item.Status})
		}
	}
	return redaction.Value(map[string]any{"profileId": graph.Map.ProfileID, "cases": cases, "interfaces": interfaces}).(map[string]any)
}

func mapPlanSummary(item store.TestMapPlanInstance) map[string]any {
	return map[string]any{"id": item.ID, "mapId": item.MapID, "profileId": item.ProfileID, "environmentId": item.EnvironmentID, "mode": item.Mode, "status": item.Status, "scope": item.Scope, "targetKind": item.TargetKind, "targetId": item.TargetID, "summaryJson": redaction.Text(item.SummaryJSON), "createdAt": item.CreatedAt, "startedAt": item.StartedAt, "finishedAt": item.FinishedAt}
}

func mapPlanReadPayload(record store.TestMapPlanRecord) map[string]any {
	item := mapPlanSummary(record.Instance)
	item["logicalPlanJson"] = redaction.Text(record.Instance.LogicalPlanJSON)
	item["physicalPlanJson"] = redaction.Text(record.Instance.PhysicalPlanJSON)
	item["ruleTraceJson"] = redaction.Text(record.Instance.RuleTraceJSON)
	tasks := make([]map[string]any, 0, len(record.Tasks))
	for _, task := range record.Tasks {
		tasks = append(tasks, map[string]any{"id": task.ID, "index": task.Index, "kind": task.Kind, "operation": task.Operation, "pathId": task.PathID, "workflowId": task.WorkflowID, "nodeId": task.NodeID, "caseId": task.CaseID, "materializationId": task.MaterializationID, "status": task.Status, "reason": task.Reason, "workflowRunId": task.WorkflowRunID, "apiCaseRunId": task.APICaseRunID, "evidenceRoot": task.EvidenceRoot, "summaryJson": redaction.Text(task.SummaryJSON), "startedAt": task.StartedAt, "finishedAt": task.FinishedAt})
	}
	item["tasks"] = tasks
	item["taskEdges"] = redaction.Value(cloneJSONObject(map[string]any{"items": record.TaskEdges})).(map[string]any)["items"]
	return redaction.Value(item).(map[string]any)
}
