package main

import (
	"strconv"

	"agent-testbench/internal/store"
)

func mapAtlasEdges(graph store.TestPlanGraph) []mapAtlasEdge {
	out := make([]mapAtlasEdge, 0, len(graph.Edges)+len(graph.PathSteps))
	seen := map[string]bool{}
	for _, edge := range graph.Edges {
		item := mapAtlasEdgeFromStore(edge)
		out = append(out, item)
		seen[mapAtlasEdgeKey(item.FromNodeID, item.ToNodeID, item.PathID)] = true
	}
	return append(out, generatedMapAtlasPathEdges(graph.PathSteps, seen)...)
}

func mapAtlasEdgeFromStore(edge store.TestPlanEdge) mapAtlasEdge {
	return mapAtlasEdge{
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

func generatedMapAtlasPathEdges(steps []store.TestPlanPathStep, seen map[string]bool) []mapAtlasEdge {
	out := []mapAtlasEdge{}
	stepsByPath := mapAtlasStepsByPath(steps)
	for pathID, pathSteps := range stepsByPath {
		for i := 1; i < len(pathSteps); i++ {
			fromID := pathSteps[i-1].NodeID
			toID := pathSteps[i].NodeID
			key := mapAtlasEdgeKey(fromID, toID, pathID)
			if seen[key] {
				continue
			}
			seen[key] = true
			out = append(out, mapAtlasEdge{
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

func mapAtlasEdgeKey(fromID string, toID string, pathID string) string {
	return fromID + "\x00" + toID + "\x00" + pathID
}

func mapAtlasWarnings(graph store.TestPlanGraph) []string {
	nodeIDs := mapAtlasPlanNodeIDs(graph.Nodes)
	nodeUsage := mapAtlasNodeConnectivity(graph.PathSteps, graph.Edges)
	warnings := []string{}
	for _, node := range graph.Nodes {
		if nodeUsage[node.ID] == 0 {
			warnings = append(warnings, "node "+node.ID+" is not used by any workflow path")
		}
	}
	return append(warnings, mapAtlasEdgeWarnings(graph.Edges, nodeIDs)...)
}

func mapAtlasPlanNodeIDs(nodes []store.TestPlanNode) map[string]bool {
	out := map[string]bool{}
	for _, node := range nodes {
		out[node.ID] = true
	}
	return out
}

func mapAtlasNodeConnectivity(steps []store.TestPlanPathStep, edges []store.TestPlanEdge) map[string]int {
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

func mapAtlasEdgeWarnings(edges []store.TestPlanEdge, nodeIDs map[string]bool) []string {
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

func mapAtlasPaletteColor(index int) string {
	palette := []string{"#2563eb", "#16a34a", "#9333ea", "#dc2626", "#0891b2", "#d97706", "#4f46e5", "#be123c", "#0f766e", "#7c3aed"}
	return palette[index%len(palette)]
}
