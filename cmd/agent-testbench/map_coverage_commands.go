package main

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"agent-testbench/internal/store"
)

type mapCoverageReport struct {
	OK               bool                       `json:"ok"`
	MapID            string                     `json:"mapId"`
	Workflows        mapCoverageWorkflowCounts  `json:"workflows"`
	Cases            mapCoverageCaseCounts      `json:"cases"`
	Materializations int                        `json:"materializations"`
	MissingWorkflows []string                   `json:"missingWorkflows,omitempty"`
	ReusedCases      []mapCoverageReusedCaseRow `json:"reusedCases,omitempty"`
}

type mapCoverageWorkflowCounts struct {
	Catalog int `json:"catalog"`
	Mapped  int `json:"mapped"`
	Missing int `json:"missing"`
}

type mapCoverageCaseCounts struct {
	Nodes          int `json:"nodes"`
	PathReferences int `json:"pathReferences"`
	Reused         int `json:"reused"`
}

type mapCoverageReusedCaseRow struct {
	CaseID     string   `json:"caseId"`
	References int      `json:"references"`
	Paths      []string `json:"paths"`
}

func runMapCoverage(ctx context.Context, args []string) error {
	input := newMapGraphCommandFlags("map coverage")
	if err := input.parse(args); err != nil {
		return err
	}
	runtime, graph, cleanup, err := openRequiredMapGraphForCLI(ctx, *input.storeRef, *input.storeURL, *input.mapID)
	if err != nil {
		return err
	}
	defer cleanup()
	catalog, err := runtime.GetProfileCatalog(ctx)
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		return err
	}
	if errors.Is(err, store.ErrNotFound) {
		catalog = store.ProfileCatalog{}
	}
	report := buildMapCoverageReport(graph, catalog)
	if *input.jsonOutput {
		return writeIndentedJSON(report)
	}
	printMapCoverageReport(report)
	return nil
}

func buildMapCoverageReport(graph store.TestPlanGraph, catalog store.ProfileCatalog) mapCoverageReport {
	mappedWorkflows := map[string]bool{}
	for _, path := range graph.Paths {
		if strings.TrimSpace(path.WorkflowID) != "" {
			mappedWorkflows[path.WorkflowID] = true
		}
	}
	missing := []string{}
	for _, workflow := range catalog.Workflows {
		if !mappedWorkflows[workflow.ID] {
			missing = append(missing, workflow.ID)
		}
	}
	sort.Strings(missing)

	caseReferences := map[string]int{}
	casePaths := map[string]map[string]bool{}
	for _, step := range graph.PathSteps {
		caseID := strings.TrimSpace(step.CaseID)
		if caseID == "" {
			caseID = graphNodeCaseID(graph.Nodes, step.NodeID)
		}
		if caseID == "" {
			continue
		}
		caseReferences[caseID]++
		if casePaths[caseID] == nil {
			casePaths[caseID] = map[string]bool{}
		}
		casePaths[caseID][step.PathID] = true
	}
	reused := []mapCoverageReusedCaseRow{}
	for caseID, count := range caseReferences {
		if count < 2 {
			continue
		}
		paths := make([]string, 0, len(casePaths[caseID]))
		for pathID := range casePaths[caseID] {
			paths = append(paths, pathID)
		}
		sort.Strings(paths)
		reused = append(reused, mapCoverageReusedCaseRow{CaseID: caseID, References: count, Paths: paths})
	}
	sort.Slice(reused, func(i, j int) bool {
		return reused[i].CaseID < reused[j].CaseID
	})

	return mapCoverageReport{
		OK:    true,
		MapID: graph.Map.ID,
		Workflows: mapCoverageWorkflowCounts{
			Catalog: len(catalog.Workflows),
			Mapped:  len(mappedWorkflows),
			Missing: len(missing),
		},
		Cases: mapCoverageCaseCounts{
			Nodes:          len(graph.Nodes),
			PathReferences: len(graph.PathSteps),
			Reused:         len(reused),
		},
		Materializations: len(graph.Materializations),
		MissingWorkflows: missing,
		ReusedCases:      reused,
	}
}

func graphNodeCaseID(nodes []store.TestPlanNode, nodeID string) string {
	for _, node := range nodes {
		if node.ID == nodeID {
			return node.CaseID
		}
	}
	return ""
}

func printMapCoverageReport(report mapCoverageReport) {
	fmt.Println("Test Scenario Atlas Coverage")
	fmt.Printf("Map: %s\n", report.MapID)
	fmt.Printf("Workflows: mapped=%d catalog=%d missing=%d\n", report.Workflows.Mapped, report.Workflows.Catalog, report.Workflows.Missing)
	fmt.Printf("Cases: nodes=%d pathReferences=%d reused=%d\n", report.Cases.Nodes, report.Cases.PathReferences, report.Cases.Reused)
	fmt.Printf("Materializations: %d\n", report.Materializations)
}
