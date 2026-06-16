package plangraph

import (
	"errors"
	"fmt"
	"strings"
)

func ExplainCase(graph Graph, options ExplainOptions) (Explanation, error) {
	node, ok := findExplainTarget(graph, options)
	if !ok {
		ref := strings.TrimSpace(options.CaseID)
		if ref == "" {
			ref = strings.TrimSpace(options.NodeID)
		}
		return Explanation{}, fmt.Errorf("plan graph node not found: %s", ref)
	}
	explain := Explanation{
		MapID:                graph.Map.ID,
		TargetNodeID:         node.ID,
		TargetCaseID:         node.CaseID,
		RequiredPropertyJSON: node.RequiredPropertyJSON,
		ProvidedPropertyJSON: node.ProvidedPropertyJSON,
		Operations:           []PhysicalOperation{},
	}
	if node.Role == NodeRoleValidation || node.StateEffect == StateEffectUnchanged {
		return explainValidationNode(graph, node, explain)
	}
	pathID, steps := pathPrefixToNode(graph, node.ID)
	if pathID == "" {
		return Explanation{}, errors.New("target node is not reachable from any path")
	}
	explain.PathID = pathID
	explain.PathSteps = steps
	explain.LogicalPath = steps
	explain = explainPathSelection(graph, explain, pathID, "target node is reachable on this mapped path")
	explain.Operations = append(explain.Operations, PhysicalOperation{
		Kind:                 OperationRunPathPrefix,
		PathID:               pathID,
		UntilNodeID:          node.ID,
		Reason:               "run mapped workflow path until target node",
		RequiredPropertyJSON: node.RequiredPropertyJSON,
		ProvidedPropertyJSON: node.ProvidedPropertyJSON,
	})
	return explain, nil
}

func explainValidationNode(graph Graph, node Node, explain Explanation) (Explanation, error) {
	materialization, ok := materializationForTarget(graph, node.ID)
	if ok {
		explain.PathID = materialization.SourcePathID
		explain.PathSteps = pathPrefixUntilNode(graph, materialization.SourcePathID, materialization.SourceUntilNodeID)
		explain.LogicalPath = explain.PathSteps
		explain.Operations = append(explain.Operations, PhysicalOperation{
			Kind:                 OperationRunPathPrefix,
			PathID:               materialization.SourcePathID,
			UntilNodeID:          materialization.SourceUntilNodeID,
			Reason:               "replay workflow prefix required by validation case fixture",
			RequiredPropertyJSON: node.RequiredPropertyJSON,
		})
	} else if node.AnchorNodeID != "" {
		pathID, steps := pathPrefixBeforeNode(graph, node.AnchorNodeID)
		if pathID != "" {
			explain.PathID = pathID
			explain.PathSteps = steps
			explain.LogicalPath = steps
			untilNodeID := ""
			if len(steps) > 0 {
				untilNodeID = steps[len(steps)-1].NodeID
			}
			explain.Operations = append(explain.Operations, PhysicalOperation{
				Kind:                 OperationRunPathPrefix,
				PathID:               pathID,
				UntilNodeID:          untilNodeID,
				Reason:               "replay path prefix before validation anchor",
				RequiredPropertyJSON: node.RequiredPropertyJSON,
			})
		}
	}
	explain.Operations = append(explain.Operations, PhysicalOperation{
		Kind:                 OperationRunCase,
		NodeID:               node.ID,
		CaseID:               node.CaseID,
		Reason:               "run validation case as a patched single request",
		PatchJSON:            node.PatchJSON,
		RequiredPropertyJSON: node.RequiredPropertyJSON,
		ProvidedPropertyJSON: node.ProvidedPropertyJSON,
	})
	explain = explainPathSelection(graph, explain, explain.PathID, "selected replay path for target case precondition")
	return explain, nil
}

func explainPathSelection(graph Graph, explain Explanation, selectedPathID string, selectedReason string) Explanation {
	var selected []CandidatePath
	var rejected []CandidatePath
	for _, path := range graph.Paths {
		candidate := CandidatePath{
			PathID:     path.ID,
			WorkflowID: path.WorkflowID,
		}
		if path.ID == selectedPathID && selectedPathID != "" {
			candidate.Selected = true
			candidate.Reason = selectedReason
			selected = append(selected, candidate)
		} else {
			candidate.Reason = "path does not satisfy selected target precondition"
			rejected = append(rejected, candidate)
			explain.RejectedReasons = append(explain.RejectedReasons, RejectedReason{
				PathID: path.ID,
				Reason: candidate.Reason,
			})
		}
	}
	explain.CandidatePaths = append(selected, rejected...)
	return explain
}

func findExplainTarget(graph Graph, options ExplainOptions) (Node, bool) {
	nodeID := strings.TrimSpace(options.NodeID)
	caseID := strings.TrimSpace(options.CaseID)
	for _, node := range graph.Nodes {
		if nodeID != "" && node.ID == nodeID {
			return node, true
		}
		if caseID != "" && node.CaseID == caseID {
			return node, true
		}
	}
	return Node{}, false
}

func materializationForTarget(graph Graph, nodeID string) (Materialization, bool) {
	materializationByID := map[string]Materialization{}
	for _, item := range graph.Materializations {
		materializationByID[item.ID] = item
	}
	for _, edge := range graph.Edges {
		if edge.ToNodeID != nodeID || edge.Kind != EdgeKindFixture {
			continue
		}
		if item, ok := materializationByID[edge.MaterializationID]; ok {
			return item, true
		}
	}
	return Materialization{}, false
}

func pathPrefixToNode(graph Graph, nodeID string) (string, []PathStep) {
	for _, path := range graph.Paths {
		steps := pathPrefixUntilNode(graph, path.ID, nodeID)
		if len(steps) > 0 && steps[len(steps)-1].NodeID == nodeID {
			return path.ID, steps
		}
	}
	return "", nil
}

func pathPrefixBeforeNode(graph Graph, nodeID string) (string, []PathStep) {
	for _, path := range graph.Paths {
		steps := pathSteps(graph, path.ID)
		for index, step := range steps {
			if step.NodeID == nodeID {
				return path.ID, append([]PathStep(nil), steps[:index]...)
			}
		}
	}
	return "", nil
}

func pathPrefixUntilNode(graph Graph, pathID string, nodeID string) []PathStep {
	steps := pathSteps(graph, pathID)
	if nodeID == "" {
		return steps
	}
	for index, step := range steps {
		if step.NodeID == nodeID {
			return append([]PathStep(nil), steps[:index+1]...)
		}
	}
	return nil
}

func pathSteps(graph Graph, pathID string) []PathStep {
	steps := make([]PathStep, 0)
	for _, step := range graph.PathSteps {
		if step.PathID == pathID {
			steps = append(steps, step)
		}
	}
	return steps
}
