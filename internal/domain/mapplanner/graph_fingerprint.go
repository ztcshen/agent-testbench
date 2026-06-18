package mapplanner

import (
	"crypto/sha1"
	"encoding/json"
	"fmt"
	"sort"

	"agent-testbench/internal/domain/plangraph"
)

type graphFingerprintPayload struct {
	MapID            string                            `json:"mapId"`
	Nodes            []graphFingerprintNode            `json:"nodes"`
	Edges            []graphFingerprintEdge            `json:"edges"`
	Paths            []graphFingerprintPath            `json:"paths"`
	Steps            []graphFingerprintStep            `json:"steps"`
	Materializations []graphFingerprintMaterialization `json:"materializations"`
}

type graphFingerprintNode struct {
	ID                   string `json:"id"`
	CaseID               string `json:"caseId"`
	InterfaceNodeID      string `json:"interfaceNodeId"`
	RequestTemplateID    string `json:"requestTemplateId"`
	BaseCaseID           string `json:"baseCaseId"`
	AnchorNodeID         string `json:"anchorNodeId"`
	Role                 string `json:"role"`
	StateEffect          string `json:"stateEffect"`
	RenderMode           string `json:"renderMode"`
	PatchJSON            string `json:"patchJson"`
	ExpectedJSON         string `json:"expectedJson"`
	RequiredPropertyJSON string `json:"requiredPropertyJson"`
	ProvidedPropertyJSON string `json:"providedPropertyJson"`
	SummaryJSON          string `json:"summaryJson"`
}

type graphFingerprintEdge struct {
	ID                string `json:"id"`
	FromNodeID        string `json:"fromNodeId"`
	ToNodeID          string `json:"toNodeId"`
	Kind              string `json:"kind"`
	PathID            string `json:"pathId"`
	MaterializationID string `json:"materializationId"`
	Required          bool   `json:"required"`
	MappingsJSON      string `json:"mappingsJson"`
	SummaryJSON       string `json:"summaryJson"`
}

type graphFingerprintPath struct {
	ID                   string `json:"id"`
	WorkflowID           string `json:"workflowId"`
	DisplayName          string `json:"displayName"`
	Status               string `json:"status"`
	RequiredPropertyJSON string `json:"requiredPropertyJson"`
	ProvidedPropertyJSON string `json:"providedPropertyJson"`
	SummaryJSON          string `json:"summaryJson"`
}

type graphFingerprintStep struct {
	PathID      string `json:"pathId"`
	Index       int    `json:"index"`
	StepID      string `json:"stepId"`
	NodeID      string `json:"nodeId"`
	CaseID      string `json:"caseId"`
	Required    bool   `json:"required"`
	Material    bool   `json:"materializeAfter"`
	SummaryJSON string `json:"summaryJson"`
}

type graphFingerprintMaterialization struct {
	ID                string `json:"id"`
	FixtureID         string `json:"fixtureId"`
	SourcePathID      string `json:"sourcePathId"`
	SourceWorkflowID  string `json:"sourceWorkflowId"`
	SourceUntilStep   string `json:"sourceUntilStep"`
	SourceUntilNodeID string `json:"sourceUntilNodeId"`
	SnapshotKind      string `json:"snapshotKind"`
	TTLSeconds        int    `json:"ttlSeconds"`
	Status            string `json:"status"`
	SummaryJSON       string `json:"summaryJson"`
}

func GraphFingerprint(graph plangraph.Graph) string {
	raw, err := json.Marshal(graphFingerprintPayloadFromGraph(graph))
	if err != nil {
		return ""
	}
	sum := sha1.Sum(raw)
	return fmt.Sprintf("sha1:%x", sum[:])
}

func graphFingerprintPayloadFromGraph(graph plangraph.Graph) graphFingerprintPayload {
	payload := graphFingerprintPayload{
		MapID:            graph.Map.ID,
		Nodes:            graphFingerprintNodes(graph.Nodes),
		Edges:            graphFingerprintEdges(graph.Edges),
		Paths:            graphFingerprintPaths(graph.Paths),
		Steps:            graphFingerprintSteps(graph.PathSteps),
		Materializations: graphFingerprintMaterializations(graph.Materializations),
	}
	sortGraphFingerprintPayload(&payload)
	return payload
}

func graphFingerprintNodes(nodes []plangraph.Node) []graphFingerprintNode {
	out := make([]graphFingerprintNode, 0, len(nodes))
	for _, node := range nodes {
		out = append(out, graphFingerprintNode{
			ID:                   node.ID,
			CaseID:               node.CaseID,
			InterfaceNodeID:      node.InterfaceNodeID,
			RequestTemplateID:    node.RequestTemplateID,
			BaseCaseID:           node.BaseCaseID,
			AnchorNodeID:         node.AnchorNodeID,
			Role:                 node.Role,
			StateEffect:          node.StateEffect,
			RenderMode:           node.RenderMode,
			PatchJSON:            node.PatchJSON,
			ExpectedJSON:         node.ExpectedJSON,
			RequiredPropertyJSON: node.RequiredPropertyJSON,
			ProvidedPropertyJSON: node.ProvidedPropertyJSON,
			SummaryJSON:          node.SummaryJSON,
		})
	}
	return out
}

func graphFingerprintEdges(edges []plangraph.Edge) []graphFingerprintEdge {
	out := make([]graphFingerprintEdge, 0, len(edges))
	for _, edge := range edges {
		out = append(out, graphFingerprintEdge{
			ID:                edge.ID,
			FromNodeID:        edge.FromNodeID,
			ToNodeID:          edge.ToNodeID,
			Kind:              edge.Kind,
			PathID:            edge.PathID,
			MaterializationID: edge.MaterializationID,
			Required:          edge.Required,
			MappingsJSON:      edge.MappingsJSON,
			SummaryJSON:       edge.SummaryJSON,
		})
	}
	return out
}

func graphFingerprintPaths(paths []plangraph.Path) []graphFingerprintPath {
	out := make([]graphFingerprintPath, 0, len(paths))
	for _, path := range paths {
		out = append(out, graphFingerprintPath{
			ID:                   path.ID,
			WorkflowID:           path.WorkflowID,
			DisplayName:          path.DisplayName,
			Status:               path.Status,
			RequiredPropertyJSON: path.RequiredPropertyJSON,
			ProvidedPropertyJSON: path.ProvidedPropertyJSON,
			SummaryJSON:          path.SummaryJSON,
		})
	}
	return out
}

func graphFingerprintSteps(steps []plangraph.PathStep) []graphFingerprintStep {
	out := make([]graphFingerprintStep, 0, len(steps))
	for _, step := range steps {
		out = append(out, graphFingerprintStep{
			PathID:      step.PathID,
			Index:       step.StepIndex,
			StepID:      step.StepID,
			NodeID:      step.NodeID,
			CaseID:      step.CaseID,
			Required:    step.Required,
			Material:    step.MaterializeAfter,
			SummaryJSON: step.SummaryJSON,
		})
	}
	return out
}

func graphFingerprintMaterializations(items []plangraph.Materialization) []graphFingerprintMaterialization {
	out := make([]graphFingerprintMaterialization, 0, len(items))
	for _, item := range items {
		out = append(out, graphFingerprintMaterialization{
			ID:                item.ID,
			FixtureID:         item.FixtureID,
			SourcePathID:      item.SourcePathID,
			SourceWorkflowID:  item.SourceWorkflowID,
			SourceUntilStep:   item.SourceUntilStep,
			SourceUntilNodeID: item.SourceUntilNodeID,
			SnapshotKind:      item.SnapshotKind,
			TTLSeconds:        item.TTLSeconds,
			Status:            item.Status,
			SummaryJSON:       item.SummaryJSON,
		})
	}
	return out
}

func sortGraphFingerprintPayload(payload *graphFingerprintPayload) {
	sort.Slice(payload.Nodes, func(i, j int) bool { return payload.Nodes[i].ID < payload.Nodes[j].ID })
	sort.Slice(payload.Edges, func(i, j int) bool { return payload.Edges[i].ID < payload.Edges[j].ID })
	sort.Slice(payload.Paths, func(i, j int) bool { return payload.Paths[i].ID < payload.Paths[j].ID })
	sort.Slice(payload.Steps, func(i, j int) bool {
		if payload.Steps[i].PathID != payload.Steps[j].PathID {
			return payload.Steps[i].PathID < payload.Steps[j].PathID
		}
		if payload.Steps[i].Index != payload.Steps[j].Index {
			return payload.Steps[i].Index < payload.Steps[j].Index
		}
		return payload.Steps[i].StepID < payload.Steps[j].StepID
	})
	sort.Slice(payload.Materializations, func(i, j int) bool {
		return payload.Materializations[i].ID < payload.Materializations[j].ID
	})
}
