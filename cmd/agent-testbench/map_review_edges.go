package main

import (
	"strconv"

	"agent-testbench/internal/store"
)

func mapReviewEdges(graph store.TestPlanGraph) []mapReviewEdge {
	out := make([]mapReviewEdge, 0, len(graph.Edges)+len(graph.PathSteps))
	seen := map[string]bool{}
	for _, edge := range graph.Edges {
		item := mapReviewEdgeFromStore(edge)
		out = append(out, item)
		seen[mapReviewEdgeKey(item.FromNodeID, item.ToNodeID, item.PathID)] = true
	}
	return append(out, generatedMapReviewPathEdges(graph.PathSteps, seen)...)
}

func mapReviewEdgeFromStore(edge store.TestPlanEdge) mapReviewEdge {
	return mapReviewEdge{
		ID:                edge.ID,
		FromNodeID:        edge.FromNodeID,
		ToNodeID:          edge.ToNodeID,
		Kind:              edge.Kind,
		PathID:            edge.PathID,
		MaterializationID: edge.MaterializationID,
		Required:          edge.Required,
		MappingsJSON:      edge.MappingsJSON,
		SummaryJSON:       edge.SummaryJSON,
		SortOrder:         edge.SortOrder,
	}
}

func generatedMapReviewPathEdges(steps []store.TestPlanPathStep, seen map[string]bool) []mapReviewEdge {
	out := []mapReviewEdge{}
	stepsByPath := mapReviewStepsByPath(steps)
	for pathID, pathSteps := range stepsByPath {
		for i := 1; i < len(pathSteps); i++ {
			fromID := pathSteps[i-1].NodeID
			toID := pathSteps[i].NodeID
			key := mapReviewEdgeKey(fromID, toID, pathID)
			if seen[key] {
				continue
			}
			seen[key] = true
			out = append(out, mapReviewEdge{
				ID:         "path:" + pathID + ":" + strconv.Itoa(i),
				FromNodeID: fromID,
				ToNodeID:   toID,
				Kind:       "path",
				PathID:     pathID,
				Required:   pathSteps[i].Required,
				Generated:  true,
				SortOrder:  i,
			})
		}
	}
	return out
}

func mapReviewEdgeKey(fromID string, toID string, pathID string) string {
	return fromID + "\x00" + toID + "\x00" + pathID
}

func mapReviewWarnings(graph store.TestPlanGraph) []string {
	nodeIDs := mapReviewPlanNodeIDs(graph.Nodes)
	nodeUsage := mapReviewNodeConnectivity(graph.PathSteps, graph.Edges)
	warnings := []string{}
	for _, node := range graph.Nodes {
		if nodeUsage[node.ID] == 0 {
			warnings = append(warnings, "node "+node.ID+" is not used by any workflow path")
		}
	}
	return append(warnings, mapReviewEdgeWarnings(graph.Edges, nodeIDs)...)
}

func mapReviewPlanNodeIDs(nodes []store.TestPlanNode) map[string]bool {
	out := map[string]bool{}
	for _, node := range nodes {
		out[node.ID] = true
	}
	return out
}

func mapReviewNodeConnectivity(steps []store.TestPlanPathStep, edges []store.TestPlanEdge) map[string]int {
	out := map[string]int{}
	for _, step := range steps {
		out[step.NodeID]++
	}
	for _, edge := range edges {
		out[edge.FromNodeID]++
		out[edge.ToNodeID]++
	}
	return out
}

func mapReviewEdgeWarnings(edges []store.TestPlanEdge, nodeIDs map[string]bool) []string {
	warnings := []string{}
	for _, edge := range edges {
		if edge.FromNodeID != "" && !nodeIDs[edge.FromNodeID] {
			warnings = append(warnings, "edge "+edge.ID+" references missing source "+edge.FromNodeID)
		}
		if edge.ToNodeID != "" && !nodeIDs[edge.ToNodeID] {
			warnings = append(warnings, "edge "+edge.ID+" references missing target "+edge.ToNodeID)
		}
	}
	return warnings
}

func mapReviewPaletteColor(index int) string {
	palette := []string{"#2563eb", "#16a34a", "#9333ea", "#dc2626", "#0891b2", "#d97706", "#4f46e5", "#be123c", "#0f766e", "#7c3aed"}
	return palette[index%len(palette)]
}
