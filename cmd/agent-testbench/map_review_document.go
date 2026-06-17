package main

import (
	"fmt"

	"agent-testbench/internal/domain/plangraph"
	"agent-testbench/internal/store"
)

func buildMapReviewDocument(graph store.TestPlanGraph, catalog store.ProfileCatalog) mapReviewDocument {
	nodeContext := newMapReviewNodeContext(graph, catalog)
	nodes, explainWarnings := buildMapReviewNodes(graph, nodeContext)
	stepsByPath := mapReviewStepsByPath(graph.PathSteps)
	paths := buildMapReviewPaths(graph.Paths, stepsByPath)
	edges := mapReviewEdges(graph)
	warnings := append(mapReviewWarnings(graph), explainWarnings...)

	return mapReviewDocument{
		Version:          "1.0",
		Map:              graph.Map,
		Counts:           mapReviewCounts(nodes, edges, paths, graph, warnings),
		Nodes:            nodes,
		Edges:            edges,
		Paths:            paths,
		Materializations: graph.Materializations,
		Warnings:         warnings,
		GeneratedBy:      "agent-testbench map review-html",
	}
}

func newMapReviewNodeContext(graph store.TestPlanGraph, catalog store.ProfileCatalog) mapReviewNodeContext {
	pathByID := mapReviewPathsByID(graph.Paths)
	return mapReviewNodeContext{
		cases:       mapReviewCasesByID(catalog.APICases),
		templates:   mapReviewTemplatesByID(catalog.RequestTemplates),
		usageByNode: mapReviewUsageByNode(graph.PathSteps, pathByID),
		layout:      mapReviewLayout(graph.PathSteps, graph.Nodes),
	}
}

func buildMapReviewNodes(graph store.TestPlanGraph, context mapReviewNodeContext) ([]mapReviewNode, []string) {
	nodes := make([]mapReviewNode, 0, len(graph.Nodes))
	warnings := []string{}
	for _, node := range graph.Nodes {
		item, warning := buildMapReviewNode(graph, context, node)
		nodes = append(nodes, item)
		if warning != "" {
			warnings = append(warnings, warning)
		}
	}
	return nodes, warnings
}

func buildMapReviewNode(graph store.TestPlanGraph, context mapReviewNodeContext, node store.TestPlanNode) (mapReviewNode, string) {
	apiCase, hasCase := context.cases[node.CaseID]
	details := mapReviewDetailsForCase(node, apiCase, hasCase)
	requestTemplateID := stringDefault(node.RequestTemplateID, apiCase.RequestTemplateID)
	explainPtr, warning := mapReviewExplanationForNode(graph, node.ID)
	paths := context.usageByNode[node.ID]

	return mapReviewNode{
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
		RequestTemplate:      mapReviewTemplatePointer(context.templates, requestTemplateID),
		Paths:                paths,
		Explanation:          explainPtr,
		Layout:               context.layout[node.ID],
		SharedCount:          len(paths),
	}, warning
}

func mapReviewDetailsForCase(node store.TestPlanNode, apiCase store.CatalogAPICase, hasCase bool) mapReviewCaseDetails {
	if hasCase {
		return mapReviewCaseDetails{
			displayName: stringDefault(apiCase.DisplayName, apiCase.ID),
			description: apiCase.Description,
			caseType:    apiCase.CaseType,
			scenario:    apiCase.Scenario,
			tags:        append([]string(nil), apiCase.Tags...),
			priority:    apiCase.Priority,
			owner:       apiCase.Owner,
		}
	}
	return mapReviewCaseDetails{
		displayName: stringDefault(node.CaseID, node.ID),
	}
}

func mapReviewTemplatePointer(templates map[string]mapReviewRequestTemplate, templateID string) *mapReviewRequestTemplate {
	raw, ok := templates[templateID]
	if !ok {
		return nil
	}
	return &raw
}

func mapReviewExplanationForNode(graph store.TestPlanGraph, nodeID string) (*plangraph.Explanation, string) {
	explain, err := plangraph.ExplainCase(graph, plangraph.ExplainOptions{NodeID: nodeID})
	if err != nil {
		return nil, fmt.Sprintf("node %s cannot be explained: %v", nodeID, err)
	}
	return &explain, ""
}

func buildMapReviewPaths(paths []store.TestPlanPath, stepsByPath map[string][]store.TestPlanPathStep) []mapReviewPath {
	out := make([]mapReviewPath, 0, len(paths))
	for index, path := range paths {
		out = append(out, mapReviewPath{
			ID:                   path.ID,
			WorkflowID:           path.WorkflowID,
			DisplayName:          path.DisplayName,
			Status:               path.Status,
			Color:                mapReviewPaletteColor(index),
			RequiredPropertyJSON: path.RequiredPropertyJSON,
			ProvidedPropertyJSON: path.ProvidedPropertyJSON,
			SummaryJSON:          path.SummaryJSON,
			Steps:                stepsByPath[path.ID],
		})
	}
	return out
}

func mapReviewCounts(nodes []mapReviewNode, edges []mapReviewEdge, paths []mapReviewPath, graph store.TestPlanGraph, warnings []string) mapReviewCountsReport {
	return mapReviewCountsReport{
		Nodes:            len(nodes),
		Edges:            len(edges),
		Paths:            len(paths),
		PathSteps:        len(graph.PathSteps),
		Materializations: len(graph.Materializations),
		Warnings:         len(warnings),
	}
}
