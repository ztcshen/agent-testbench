package main

import (
	"fmt"

	"agent-testbench/internal/domain/plangraph"
	"agent-testbench/internal/store"
)

func buildMapAtlasDocument(graph store.TestPlanGraph, catalog store.ProfileCatalog) mapAtlasDocument {
	nodeContext := newMapAtlasNodeContext(graph, catalog)
	nodes, explainWarnings := buildMapAtlasNodes(graph, nodeContext)
	stepsByPath := mapAtlasStepsByPath(graph.PathSteps)
	paths := buildMapAtlasPaths(graph.Paths, stepsByPath)
	edges := mapAtlasEdges(graph)
	warnings := append(mapAtlasWarnings(graph), explainWarnings...)

	return mapAtlasDocument{
		Version:          "1.0",
		Map:              graph.Map,
		Counts:           mapAtlasCounts(nodes, edges, paths, graph, warnings),
		Nodes:            nodes,
		Edges:            edges,
		Paths:            paths,
		Materializations: graph.Materializations,
		Warnings:         warnings,
		GeneratedBy:      "agent-testbench map atlas",
	}
}

func mapAtlasPlanFromRecord(record store.TestMapPlanRecord) *mapAtlasPlan {
	tasks := make([]mapAtlasTask, 0, len(record.Tasks))
	for _, task := range record.Tasks {
		tasks = append(tasks, mapAtlasTask{
			ID:                task.ID,
			Index:             task.Index,
			Kind:              task.Kind,
			Operation:         task.Operation,
			PathID:            task.PathID,
			WorkflowID:        task.WorkflowID,
			NodeID:            task.NodeID,
			CaseID:            task.CaseID,
			MaterializationID: task.MaterializationID,
			Status:            task.Status,
			Reason:            task.Reason,
			WorkflowRunID:     task.WorkflowRunID,
			APICaseRunID:      task.APICaseRunID,
			EvidenceRoot:      task.EvidenceRoot,
			SummaryJSON:       task.SummaryJSON,
		})
	}
	return &mapAtlasPlan{
		PlanID:      record.Instance.ID,
		Mode:        record.Instance.Mode,
		Status:      record.Instance.Status,
		Scope:       record.Instance.Scope,
		TargetKind:  record.Instance.TargetKind,
		TargetID:    record.Instance.TargetID,
		SummaryJSON: record.Instance.SummaryJSON,
		Tasks:       tasks,
	}
}

func newMapAtlasNodeContext(graph store.TestPlanGraph, catalog store.ProfileCatalog) mapAtlasNodeContext {
	pathByID := mapAtlasPathsByID(graph.Paths)
	return mapAtlasNodeContext{
		cases:       mapAtlasCasesByID(catalog.APICases),
		templates:   mapAtlasTemplatesByID(catalog.RequestTemplates),
		usageByNode: mapAtlasUsageByNode(graph.PathSteps, pathByID),
		layout:      mapAtlasLayout(graph.PathSteps, graph.Nodes),
	}
}

func buildMapAtlasNodes(graph store.TestPlanGraph, context mapAtlasNodeContext) ([]mapAtlasNode, []string) {
	nodes := make([]mapAtlasNode, 0, len(graph.Nodes))
	warnings := []string{}
	for _, node := range graph.Nodes {
		item, warning := buildMapAtlasNode(graph, context, node)
		nodes = append(nodes, item)
		if warning != "" {
			warnings = append(warnings, warning)
		}
	}
	return nodes, warnings
}

func buildMapAtlasNode(graph store.TestPlanGraph, context mapAtlasNodeContext, node store.TestPlanNode) (mapAtlasNode, string) {
	apiCase, hasCase := context.cases[node.CaseID]
	details := mapAtlasDetailsForCase(node, apiCase, hasCase)
	requestTemplateID := stringDefault(node.RequestTemplateID, apiCase.RequestTemplateID)
	explainPtr, warning := mapAtlasExplanationForNode(graph, node.ID)
	paths := context.usageByNode[node.ID]

	return mapAtlasNode{
		ID:                   node.ID,
		CaseID:               node.CaseID,
		DisplayName:          details.displayName,
		Description:          details.description,
		InterfaceNodeID:      node.InterfaceNodeID,
		RequestTemplateID:    requestTemplateID,
		BaseCaseID:           node.BaseCaseID,
		AnchorNodeID:         node.AnchorNodeID,
		Role:                 node.Role,
		StateEffect:          node.StateEffect,
		RenderMode:           node.RenderMode,
		CaseType:             details.caseType,
		Scenario:             details.scenario,
		Tags:                 details.tags,
		Priority:             details.priority,
		Owner:                details.owner,
		PatchJSON:            stringDefault(node.PatchJSON, apiCase.PatchJSON),
		ExpectedJSON:         stringDefault(node.ExpectedJSON, apiCase.ExpectedJSON),
		RequiredPropertyJSON: node.RequiredPropertyJSON,
		ProvidedPropertyJSON: node.ProvidedPropertyJSON,
		SummaryJSON:          node.SummaryJSON,
		RequestTemplate:      mapAtlasTemplatePointer(context.templates, requestTemplateID),
		Paths:                paths,
		Explanation:          explainPtr,
		Layout:               context.layout[node.ID],
		SharedCount:          len(paths),
	}, warning
}

func mapAtlasDetailsForCase(node store.TestPlanNode, apiCase store.CatalogAPICase, hasCase bool) mapAtlasCaseDetails {
	if hasCase {
		return mapAtlasCaseDetails{
			displayName: stringDefault(apiCase.DisplayName, apiCase.ID),
			description: apiCase.Description,
			caseType:    apiCase.CaseType,
			scenario:    apiCase.Scenario,
			tags:        append([]string(nil), apiCase.Tags...),
			priority:    apiCase.Priority,
			owner:       apiCase.Owner,
		}
	}
	return mapAtlasCaseDetails{
		displayName: stringDefault(node.CaseID, node.ID),
	}
}

func mapAtlasTemplatePointer(templates map[string]mapAtlasRequestTemplate, templateID string) *mapAtlasRequestTemplate {
	raw, ok := templates[templateID]
	if !ok {
		return nil
	}
	return &raw
}

func mapAtlasExplanationForNode(graph store.TestPlanGraph, nodeID string) (*plangraph.Explanation, string) {
	explain, err := plangraph.ExplainCase(graph, plangraph.ExplainOptions{NodeID: nodeID})
	if err != nil {
		return nil, fmt.Sprintf("node %s cannot be explained: %v", nodeID, err)
	}
	return &explain, ""
}

func buildMapAtlasPaths(paths []store.TestPlanPath, stepsByPath map[string][]store.TestPlanPathStep) []mapAtlasPath {
	out := make([]mapAtlasPath, 0, len(paths))
	for index, path := range paths {
		out = append(out, mapAtlasPath{
			ID:                   path.ID,
			WorkflowID:           path.WorkflowID,
			DisplayName:          path.DisplayName,
			Status:               path.Status,
			Color:                mapAtlasPaletteColor(index),
			RequiredPropertyJSON: path.RequiredPropertyJSON,
			ProvidedPropertyJSON: path.ProvidedPropertyJSON,
			SummaryJSON:          path.SummaryJSON,
			Steps:                stepsByPath[path.ID],
		})
	}
	return out
}

func mapAtlasCounts(nodes []mapAtlasNode, edges []mapAtlasEdge, paths []mapAtlasPath, graph store.TestPlanGraph, warnings []string) mapAtlasCountsReport {
	return mapAtlasCountsReport{
		Nodes:            len(nodes),
		Edges:            len(edges),
		Paths:            len(paths),
		PathSteps:        len(graph.PathSteps),
		Materializations: len(graph.Materializations),
		Warnings:         len(warnings),
	}
}
