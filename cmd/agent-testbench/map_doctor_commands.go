package main

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"agent-testbench/internal/domain/plangraph"
	"agent-testbench/internal/store"
)

type mapDoctorReport struct {
	OK         bool             `json:"ok"`
	MapID      string           `json:"mapId"`
	Counts     mapCountsReport  `json:"counts"`
	IssueCount int              `json:"issueCount"`
	Checks     []mapDoctorCheck `json:"checks"`
	Next       []string         `json:"next,omitempty"`
}

type mapDoctorCheck struct {
	Code     string `json:"code"`
	OK       bool   `json:"ok"`
	Severity string `json:"severity"`
	Detail   string `json:"detail"`
	Fix      string `json:"fix,omitempty"`
}

func runMapDoctor(ctx context.Context, args []string) error {
	input := newMapGraphCommandFlags("map doctor")
	if err := input.parse(args); err != nil {
		return err
	}
	_, graph, cleanup, err := openRequiredMapGraphForCLI(ctx, *input.storeRef, *input.storeURL, *input.mapID)
	if err != nil {
		return err
	}
	defer cleanup()
	report := buildMapDoctorReport(graph)
	if *input.jsonOutput {
		if err := writeIndentedJSON(report); err != nil {
			return err
		}
	} else {
		printMapDoctorReport(report)
	}
	if report.IssueCount > 0 {
		return errors.New("map doctor found issues")
	}
	return nil
}

func buildMapDoctorReport(graph store.TestPlanGraph) mapDoctorReport {
	checkCapacity := 1 + len(graph.PathSteps)*2 + len(graph.Edges)*4 + len(graph.Materializations)*2 + len(graph.Nodes)
	checks := make([]mapDoctorCheck, 0, checkCapacity)
	index := mapDoctorReferenceIndexFromGraph(graph)
	checks = append(checks, mapDoctorDAGChecks(graph)...)
	checks = append(checks, mapDoctorPathStepChecks(graph.PathSteps, index)...)
	checks = append(checks, mapDoctorEdgeChecks(graph.Edges, index)...)
	checks = append(checks, mapDoctorMaterializationChecks(graph.Materializations, index)...)
	checks = append(checks, mapDoctorValidationAnchorChecks(graph.Nodes, index)...)
	issueCount := mapDoctorIssueCount(checks)
	report := mapDoctorReport{OK: issueCount == 0, MapID: graph.Map.ID, Counts: mapCountsFromGraph(graph), IssueCount: issueCount, Checks: checks}
	if issueCount > 0 {
		report.Next = []string{"run agent-testbench map validation list to inspect anchors", "repair map references, then rerun agent-testbench map doctor"}
	}
	return report
}

type mapDoctorReferenceIndex struct {
	NodeIDs            map[string]bool
	PathIDs            map[string]bool
	MaterializationIDs map[string]bool
}

func mapDoctorReferenceIndexFromGraph(graph store.TestPlanGraph) mapDoctorReferenceIndex {
	index := mapDoctorReferenceIndex{
		NodeIDs:            map[string]bool{},
		PathIDs:            map[string]bool{},
		MaterializationIDs: map[string]bool{},
	}
	for _, node := range graph.Nodes {
		index.NodeIDs[node.ID] = true
	}
	for _, path := range graph.Paths {
		index.PathIDs[path.ID] = true
	}
	for _, item := range graph.Materializations {
		index.MaterializationIDs[item.ID] = true
	}
	return index
}

func mapDoctorDAGChecks(graph store.TestPlanGraph) []mapDoctorCheck {
	if err := plangraph.ValidateDAG(graph); err != nil {
		return []mapDoctorCheck{mapDoctorIssue("dag", "P1", err.Error(), "remove or reverse the cycle before publishing the map")}
	}
	return []mapDoctorCheck{mapDoctorOK("dag", "graph is acyclic")}
}

func mapDoctorPathStepChecks(steps []store.TestPlanPathStep, index mapDoctorReferenceIndex) []mapDoctorCheck {
	checks := []mapDoctorCheck{}
	for _, step := range steps {
		if !index.PathIDs[step.PathID] {
			checks = append(checks, mapDoctorIssue("path-step.path", "P1", fmt.Sprintf("path step %s/%d references missing path %s", step.PathID, step.StepIndex, step.PathID), "repair path steps or re-import the map from Store catalog"))
		}
		if !index.NodeIDs[step.NodeID] {
			checks = append(checks, mapDoctorIssue("path-step.node", "P1", fmt.Sprintf("path step %s/%d references missing node %s", step.PathID, step.StepIndex, step.NodeID), "attach the missing node or remove the stale path step"))
		}
	}
	return checks
}

func mapDoctorEdgeChecks(edges []store.TestPlanEdge, index mapDoctorReferenceIndex) []mapDoctorCheck {
	checks := []mapDoctorCheck{}
	for _, edge := range edges {
		if edge.FromNodeID != "" && !index.NodeIDs[edge.FromNodeID] {
			checks = append(checks, mapDoctorIssue("edge.from-node", "P1", fmt.Sprintf("edge %s references missing from-node %s", edge.ID, edge.FromNodeID), "repair or remove the edge"))
		}
		if edge.ToNodeID != "" && !index.NodeIDs[edge.ToNodeID] {
			checks = append(checks, mapDoctorIssue("edge.to-node", "P1", fmt.Sprintf("edge %s references missing to-node %s", edge.ID, edge.ToNodeID), "repair or remove the edge"))
		}
		if edge.PathID != "" && !index.PathIDs[edge.PathID] {
			checks = append(checks, mapDoctorIssue("edge.path", "P2", fmt.Sprintf("edge %s references missing path %s", edge.ID, edge.PathID), "repair path ownership metadata on the edge"))
		}
		if edge.MaterializationID != "" && !index.MaterializationIDs[edge.MaterializationID] {
			checks = append(checks, mapDoctorIssue("edge.materialization", "P1", fmt.Sprintf("edge %s references missing materialization %s", edge.ID, edge.MaterializationID), "restore the materialization or detach the fixture edge"))
		}
	}
	return checks
}

func mapDoctorMaterializationChecks(materializations []store.TestPlanMaterialization, index mapDoctorReferenceIndex) []mapDoctorCheck {
	checks := []mapDoctorCheck{}
	for _, item := range materializations {
		if item.SourcePathID != "" && !index.PathIDs[item.SourcePathID] {
			checks = append(checks, mapDoctorIssue("materialization.source-path", "P1", fmt.Sprintf("materialization %s references missing source path %s", item.ID, item.SourcePathID), "repair the replay source workflow path"))
		}
		if item.SourceUntilNodeID != "" && !index.NodeIDs[item.SourceUntilNodeID] {
			checks = append(checks, mapDoctorIssue("materialization.until-node", "P1", fmt.Sprintf("materialization %s references missing until-node %s", item.ID, item.SourceUntilNodeID), "repair the replay checkpoint node"))
		}
	}
	return checks
}

func mapDoctorValidationAnchorChecks(nodes []store.TestPlanNode, index mapDoctorReferenceIndex) []mapDoctorCheck {
	checks := []mapDoctorCheck{}
	for _, node := range nodes {
		if !mapNodeIsValidation(node) {
			continue
		}
		anchor := strings.TrimSpace(stringDefault(node.AnchorNodeID, node.BaseCaseID))
		if anchor == "" {
			checks = append(checks, mapDoctorIssue("validation.anchor", "P2", fmt.Sprintf("validation node %s has no anchor", node.ID), "attach the validation case with map validation attach"))
			continue
		}
		if !index.NodeIDs[anchor] {
			checks = append(checks, mapDoctorIssue("validation.anchor", "P1", fmt.Sprintf("validation node %s references missing anchor %s", node.ID, anchor), "attach the validation case to an existing interface anchor"))
		}
	}
	return checks
}

func mapDoctorIssueCount(checks []mapDoctorCheck) int {
	issueCount := 0
	for _, check := range checks {
		if !check.OK {
			issueCount++
		}
	}
	return issueCount
}

func mapDoctorOK(code string, detail string) mapDoctorCheck {
	return mapDoctorCheck{Code: code, OK: true, Severity: "info", Detail: detail}
}

func mapDoctorIssue(code string, severity string, detail string, fix string) mapDoctorCheck {
	return mapDoctorCheck{Code: code, OK: false, Severity: severity, Detail: detail, Fix: fix}
}

func printMapDoctorReport(report mapDoctorReport) {
	fmt.Println("Map Doctor")
	fmt.Printf("Map: %s\n", report.MapID)
	fmt.Printf("Issues: %d\n", report.IssueCount)
	for _, check := range report.Checks {
		status := "ok"
		if !check.OK {
			status = "issue"
		}
		fmt.Printf("- %s [%s] %s\n", check.Code, status, check.Detail)
	}
}
