package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"agent-testbench/internal/store"
)

type mapAtlasReport struct {
	OK     bool                 `json:"ok"`
	MapID  string               `json:"mapId"`
	PlanID string               `json:"planId,omitempty"`
	Filter string               `json:"filter,omitempty"`
	Output string               `json:"output"`
	Counts mapAtlasCountsReport `json:"counts"`
}

func runMapAtlas(ctx context.Context, args []string) error {
	return runMapAtlasCommand(ctx, args)
}

func runMapAtlasCommand(ctx context.Context, args []string) error {
	flags := flag.NewFlagSet("map atlas", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	storeRef := flags.String("store", "", "Named Store config or Store DSN")
	storeURL := flags.String("store-url", "", legacyStoreURLFlagHelp)
	mapID := flags.String("map", "", "Plan map id")
	planID := flags.String("plan", "", "Saved planner instance id to overlay task status")
	filter := flags.String("filter", "", "Filter path id, workflow id, display name, node id, or case id")
	output := flags.String("output", "", "HTML output path")
	jsonOutput := flags.Bool("json", false, "Emit a machine-readable JSON report")
	if err := flags.Parse(args); err != nil {
		return err
	}
	runtime, graph, cleanup, err := openRequiredMapGraphForCLI(ctx, *storeRef, *storeURL, *mapID)
	if err != nil {
		return err
	}
	defer cleanup()

	graph = filterMapAtlasGraph(graph, *filter)
	catalog, err := runtime.GetProfileCatalog(ctx)
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		return err
	}
	if errors.Is(err, store.ErrNotFound) {
		catalog = store.ProfileCatalog{}
	}
	document := buildMapAtlasDocument(graph, catalog)
	planIDValue := strings.TrimSpace(*planID)
	if planIDValue != "" {
		record, err := runtime.GetTestMapPlan(ctx, planIDValue)
		if err != nil {
			return err
		}
		if strings.TrimSpace(record.Instance.MapID) != graph.Map.ID {
			return fmt.Errorf("--plan %s belongs to map %s, not %s", planIDValue, record.Instance.MapID, graph.Map.ID)
		}
		document.Plan = mapAtlasPlanFromRecord(record)
	}
	outputPath := strings.TrimSpace(*output)
	if outputPath == "" {
		outputPath = defaultMapAtlasOutputPath(graph.Map.ID)
	}
	if err := writeMapAtlasHTML(outputPath, document); err != nil {
		return err
	}
	report := mapAtlasReport{
		OK:     true,
		MapID:  graph.Map.ID,
		PlanID: planIDValue,
		Filter: strings.TrimSpace(*filter),
		Output: outputPath,
		Counts: document.Counts,
	}
	if *jsonOutput {
		return writeIndentedJSON(report)
	}
	printMapAtlasReport(report)
	return nil
}

func defaultMapAtlasOutputPath(mapID string) string {
	name := strings.NewReplacer("/", "-", "\\", "-", ":", "-", " ", "-").Replace(strings.TrimSpace(mapID))
	if name == "" {
		name = "map"
	}
	return filepath.Join(".runtime", "test-scenario-atlas", name+".html")
}

func writeMapAtlasHTML(path string, document mapAtlasDocument) error {
	if dir := filepath.Dir(path); dir != "." && dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	content, err := renderMapAtlasHTML(document)
	if err != nil {
		return err
	}
	return os.WriteFile(path, []byte(content), 0o644)
}

func printMapAtlasReport(report mapAtlasReport) {
	fmt.Println("Test Scenario Atlas")
	fmt.Printf("Map: %s\n", report.MapID)
	fmt.Printf("Output: %s\n", report.Output)
	fmt.Printf("Nodes: %d\n", report.Counts.Nodes)
	fmt.Printf("Paths: %d\n", report.Counts.Paths)
}
