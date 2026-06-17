package plangraph

import "fmt"

func ValidateDAG(graph Graph) error {
	outgoing := map[string][]string{}
	for _, node := range graph.Nodes {
		if _, ok := outgoing[node.ID]; !ok {
			outgoing[node.ID] = nil
		}
	}
	for _, edge := range graph.Edges {
		if edge.FromNodeID == "" || edge.ToNodeID == "" {
			continue
		}
		outgoing[edge.FromNodeID] = append(outgoing[edge.FromNodeID], edge.ToNodeID)
		if _, ok := outgoing[edge.ToNodeID]; !ok {
			outgoing[edge.ToNodeID] = nil
		}
	}

	state := map[string]int{}
	var visit func(string) error
	visit = func(nodeID string) error {
		switch state[nodeID] {
		case 1:
			return fmt.Errorf("plan graph contains cycle at node %q", nodeID)
		case 2:
			return nil
		}
		state[nodeID] = 1
		for _, next := range outgoing[nodeID] {
			if err := visit(next); err != nil {
				return err
			}
		}
		state[nodeID] = 2
		return nil
	}

	for nodeID := range outgoing {
		if err := visit(nodeID); err != nil {
			return err
		}
	}
	return nil
}
