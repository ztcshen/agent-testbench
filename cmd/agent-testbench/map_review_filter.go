package main

import (
	"strings"

	"agent-testbench/internal/store"
)

func filterMapReviewGraph(graph store.TestPlanGraph, filter string) store.TestPlanGraph {
	needle := strings.ToLower(strings.TrimSpace(filter))
	if needle == "" {
		return graph
	}
	pathIDs := mapReviewMatchingPathIDs(graph.Paths, needle)
	hasDirectPathMatch := len(pathIDs) > 0
	keptNodes := mapReviewMatchingNodeIDs(graph.Nodes, graph.PathSteps, pathIDs, needle)
	if !hasDirectPathMatch {
		for _, step := range graph.PathSteps {
			if keptNodes[step.NodeID] {
				pathIDs[step.PathID] = true
			}
		}
	}
	for _, step := range graph.PathSteps {
		if pathIDs[step.PathID] {
			keptNodes[step.NodeID] = true
		}
	}
	keptEdges := mapReviewExpandEdges(graph.Edges, pathIDs, keptNodes)
	keptMaterializations := mapReviewMaterializationIDs(keptEdges)

	next := graph
	next.Nodes = filterMapReviewNodes(graph.Nodes, keptNodes)
	next.Paths = filterMapReviewPaths(graph.Paths, pathIDs)
	next.PathSteps = filterMapReviewPathSteps(graph.PathSteps, pathIDs, keptNodes)
	next.Edges = keptEdges
	next.Materializations = filterMapReviewMaterializations(graph.Materializations, pathIDs, keptMaterializations)
	return next
}

func mapReviewMatchingPathIDs(paths []store.TestPlanPath, needle string) map[string]bool {
	out := map[string]bool{}
	for _, path := range paths {
		if mapReviewTextMatches(needle, path.ID, path.WorkflowID, path.DisplayName) {
			out[path.ID] = true
		}
	}
	return out
}

func mapReviewMatchingNodeIDs(nodes []store.TestPlanNode, steps []store.TestPlanPathStep, pathIDs map[string]bool, needle string) map[string]bool {
	out := map[string]bool{}
	for _, step := range steps {
		if pathIDs[step.PathID] {
			out[step.NodeID] = true
		}
	}
	for _, node := range nodes {
		if mapReviewTextMatches(needle, node.ID, node.CaseID, node.InterfaceNodeID, node.RequestTemplateID) {
			out[node.ID] = true
		}
	}
	return out
}

func mapReviewExpandEdges(edges []store.TestPlanEdge, pathIDs map[string]bool, keptNodes map[string]bool) []store.TestPlanEdge {
	keptEdges := []store.TestPlanEdge{}
	changed := true
	for changed {
		changed = false
		for _, edge := range edges {
			if mapReviewEdgeKept(keptEdges, edge.ID) {
				continue
			}
			if pathIDs[edge.PathID] || keptNodes[edge.FromNodeID] || keptNodes[edge.ToNodeID] {
				keptEdges = append(keptEdges, edge)
				changed = mapReviewKeepEdgeNodes(edge, keptNodes) || changed
			}
		}
	}
	return keptEdges
}

func mapReviewKeepEdgeNodes(edge store.TestPlanEdge, keptNodes map[string]bool) bool {
	changed := false
	if edge.FromNodeID != "" && !keptNodes[edge.FromNodeID] {
		keptNodes[edge.FromNodeID] = true
		changed = true
	}
	if edge.ToNodeID != "" && !keptNodes[edge.ToNodeID] {
		keptNodes[edge.ToNodeID] = true
		changed = true
	}
	return changed
}

func mapReviewMaterializationIDs(edges []store.TestPlanEdge) map[string]bool {
	out := map[string]bool{}
	for _, edge := range edges {
		if edge.MaterializationID != "" {
			out[edge.MaterializationID] = true
		}
	}
	return out
}

func mapReviewTextMatches(needle string, values ...string) bool {
	for _, value := range values {
		if strings.Contains(strings.ToLower(value), needle) {
			return true
		}
	}
	return false
}

func mapReviewEdgeKept(edges []store.TestPlanEdge, edgeID string) bool {
	for _, edge := range edges {
		if edge.ID == edgeID {
			return true
		}
	}
	return false
}

func filterMapReviewNodes(nodes []store.TestPlanNode, kept map[string]bool) []store.TestPlanNode {
	out := []store.TestPlanNode{}
	for _, node := range nodes {
		if kept[node.ID] {
			out = append(out, node)
		}
	}
	return out
}

func filterMapReviewPaths(paths []store.TestPlanPath, kept map[string]bool) []store.TestPlanPath {
	out := []store.TestPlanPath{}
	for _, path := range paths {
		if kept[path.ID] {
			out = append(out, path)
		}
	}
	return out
}

func filterMapReviewPathSteps(steps []store.TestPlanPathStep, keptPaths map[string]bool, keptNodes map[string]bool) []store.TestPlanPathStep {
	out := []store.TestPlanPathStep{}
	for _, step := range steps {
		if keptPaths[step.PathID] && keptNodes[step.NodeID] {
			out = append(out, step)
		}
	}
	return out
}

func filterMapReviewMaterializations(materializations []store.TestPlanMaterialization, keptPaths map[string]bool, keptMaterializations map[string]bool) []store.TestPlanMaterialization {
	out := []store.TestPlanMaterialization{}
	for _, item := range materializations {
		if keptPaths[item.SourcePathID] || keptMaterializations[item.ID] {
			out = append(out, item)
		}
	}
	return out
}
