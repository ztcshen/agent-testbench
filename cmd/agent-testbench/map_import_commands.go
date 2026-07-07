package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"

	"agent-testbench/internal/domain/commandline"
	"agent-testbench/internal/domain/plangraph"
	"agent-testbench/internal/store"
)

type mapImportReport struct {
	OK          bool              `json:"ok"`
	Mode        string            `json:"mode"`
	Map         store.TestPlanMap `json:"map"`
	Counts      mapCountsReport   `json:"counts"`
	Before      *mapCountsReport  `json:"before,omitempty"`
	Imported    mapCountsReport   `json:"imported"`
	Warnings    []string          `json:"warnings,omitempty"`
	NextActions []string          `json:"nextActions,omitempty"`
}

func runMapImportWorkflows(ctx context.Context, args []string) error {
	flags := flag.NewFlagSet("map import-workflows", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	storeRef := flags.String("store", "", "Named Store config or Store DSN")
	storeURL := flags.String("store-url", "", legacyStoreURLFlagHelp)
	mapID := flags.String("map", "", "Plan map id")
	displayName := flags.String("display-name", "", "Plan map display name")
	description := flags.String("description", "", "Plan map description")
	appendMode := flags.Bool("append", false, "Append filtered workflow paths into an existing map instead of replacing the whole map")
	var workflowIDs stringListFlag
	flags.Var(&workflowIDs, "workflow", "Workflow id to import into the map; repeat for multiple workflows; defaults to replace mode unless --append is set")
	jsonOutput := flags.Bool("json", false, "Emit a machine-readable JSON report")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() > 0 {
		return fmt.Errorf("map import-workflows does not accept positional arguments: %s", strings.Join(flags.Args(), " "))
	}
	runtime, cleanup, err := openRequiredCLIStore(ctx, *storeRef, *storeURL)
	if err != nil {
		return err
	}
	defer cleanup()
	catalog, err := runtime.GetProfileCatalog(ctx)
	if err != nil {
		return err
	}
	graph, err := plangraph.ImportCatalog(catalog, plangraph.ImportOptions{
		MapID:       *mapID,
		DisplayName: *displayName,
		Description: *description,
		WorkflowIDs: workflowIDs.Values(),
	})
	if err != nil {
		return err
	}
	importedCounts := mapCountsFromGraph(graph)
	before, hasBefore, err := existingMapCounts(ctx, runtime, graph.Map.ID)
	if err != nil {
		return err
	}
	mode := "replace"
	if *appendMode {
		mode = "append"
		if hasBefore {
			existing, err := runtime.GetTestPlanGraph(ctx, graph.Map.ID)
			if err != nil {
				return err
			}
			graph = appendImportedMapGraph(existing, graph, *displayName, *description)
		}
	}
	if err := runtime.ReplaceTestPlanGraph(ctx, graph); err != nil {
		return err
	}
	report := mapImportReport{
		OK:          true,
		Mode:        mode,
		Map:         graph.Map,
		Counts:      mapCountsFromGraph(graph),
		Imported:    importedCounts,
		Warnings:    mapImportWarnings(mode, workflowIDs.Values(), before, hasBefore, mapCountsFromGraph(graph)),
		NextActions: mapImportNextActions(mode, graph.Map.ID),
	}
	if hasBefore {
		report.Before = &before
	}
	if *jsonOutput {
		return writeIndentedJSON(report)
	}
	printMapImportReport(report)
	return nil
}

func existingMapCounts(ctx context.Context, runtime store.Store, mapID string) (mapCountsReport, bool, error) {
	existing, err := runtime.GetTestPlanGraph(ctx, mapID)
	if errors.Is(err, store.ErrNotFound) {
		return mapCountsReport{}, false, nil
	}
	if err != nil {
		return mapCountsReport{}, false, err
	}
	return mapCountsFromGraph(existing), true, nil
}

func appendImportedMapGraph(existing store.TestPlanGraph, imported store.TestPlanGraph, displayName string, description string) store.TestPlanGraph {
	out := existing
	out.Map.UpdatedAt = imported.Map.UpdatedAt
	if strings.TrimSpace(displayName) != "" {
		out.Map.DisplayName = strings.TrimSpace(displayName)
	}
	if strings.TrimSpace(description) != "" {
		out.Map.Description = strings.TrimSpace(description)
	}
	pathIDs := map[string]bool{}
	for _, path := range imported.Paths {
		pathIDs[path.ID] = true
	}
	out.Paths = upsertMapPaths(out.Paths, imported.Paths)
	out.PathSteps = replaceMapPathSteps(out.PathSteps, imported.PathSteps, pathIDs)
	out.Nodes = upsertMapNodes(out.Nodes, imported.Nodes)
	out.Edges = upsertMapEdges(removeMapEdgesForPaths(out.Edges, pathIDs), imported.Edges)
	out.Materializations = upsertMapMaterializations(out.Materializations, imported.Materializations)
	return out
}

func upsertMapPaths(existing []store.TestPlanPath, incoming []store.TestPlanPath) []store.TestPlanPath {
	indexByID := map[string]int{}
	out := append([]store.TestPlanPath(nil), existing...)
	for index, item := range out {
		indexByID[item.ID] = index
	}
	for _, item := range incoming {
		if index, ok := indexByID[item.ID]; ok {
			out[index] = item
			continue
		}
		indexByID[item.ID] = len(out)
		out = append(out, item)
	}
	return out
}

func replaceMapPathSteps(existing []store.TestPlanPathStep, incoming []store.TestPlanPathStep, pathIDs map[string]bool) []store.TestPlanPathStep {
	out := make([]store.TestPlanPathStep, 0, len(existing)+len(incoming))
	for _, item := range existing {
		if !pathIDs[item.PathID] {
			out = append(out, item)
		}
	}
	return append(out, incoming...)
}

func upsertMapNodes(existing []store.TestPlanNode, incoming []store.TestPlanNode) []store.TestPlanNode {
	indexByID := map[string]int{}
	out := append([]store.TestPlanNode(nil), existing...)
	for index, item := range out {
		indexByID[item.ID] = index
	}
	for _, item := range incoming {
		if index, ok := indexByID[item.ID]; ok {
			out[index] = item
			continue
		}
		indexByID[item.ID] = len(out)
		out = append(out, item)
	}
	return out
}

func removeMapEdgesForPaths(existing []store.TestPlanEdge, pathIDs map[string]bool) []store.TestPlanEdge {
	out := make([]store.TestPlanEdge, 0, len(existing))
	for _, item := range existing {
		if !pathIDs[item.PathID] {
			out = append(out, item)
		}
	}
	return out
}

func upsertMapEdges(existing []store.TestPlanEdge, incoming []store.TestPlanEdge) []store.TestPlanEdge {
	indexByID := map[string]int{}
	out := append([]store.TestPlanEdge(nil), existing...)
	for index, item := range out {
		indexByID[item.ID] = index
	}
	for _, item := range incoming {
		if index, ok := indexByID[item.ID]; ok {
			out[index] = item
			continue
		}
		indexByID[item.ID] = len(out)
		out = append(out, item)
	}
	return out
}

func upsertMapMaterializations(existing []store.TestPlanMaterialization, incoming []store.TestPlanMaterialization) []store.TestPlanMaterialization {
	indexByID := map[string]int{}
	out := append([]store.TestPlanMaterialization(nil), existing...)
	for index, item := range out {
		indexByID[item.ID] = index
	}
	for _, item := range incoming {
		if index, ok := indexByID[item.ID]; ok {
			out[index] = item
			continue
		}
		indexByID[item.ID] = len(out)
		out = append(out, item)
	}
	return out
}

func mapImportWarnings(mode string, workflowIDs []string, before mapCountsReport, hasBefore bool, after mapCountsReport) []string {
	if mode != "replace" || len(workflowIDs) == 0 || !hasBefore || after.Paths >= before.Paths {
		return nil
	}
	return []string{
		fmt.Sprintf("replace mode imported %d selected path(s) over an existing map with %d path(s); use --append to keep existing workflow paths", after.Paths, before.Paths),
	}
}

func mapImportNextActions(mode string, mapID string) []string {
	actions := []string{
		"agent-testbench map inspect --map " + commandline.ShellQuote(mapID) + " --view workflows --json",
	}
	if mode == "replace" {
		actions = append(actions, "use --append when importing a filtered workflow without replacing existing map paths")
	}
	return actions
}

func printMapImportReport(report mapImportReport) {
	fmt.Println("Workflow Map")
	fmt.Printf("Map: %s\n", report.Map.ID)
	fmt.Printf("Profile: %s\n", report.Map.ProfileID)
	fmt.Printf("Nodes: %d\n", report.Counts.Nodes)
	fmt.Printf("Paths: %d\n", report.Counts.Paths)
	fmt.Printf("Materializations: %d\n", report.Counts.Materializations)
}
