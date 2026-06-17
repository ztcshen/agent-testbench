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

type mapReviewHTMLReport struct {
	OK     bool                  `json:"ok"`
	MapID  string                `json:"mapId"`
	Filter string                `json:"filter,omitempty"`
	Output string                `json:"output"`
	Counts mapReviewCountsReport `json:"counts"`
}

func runMapReviewHTML(ctx context.Context, args []string) error {
	flags := flag.NewFlagSet("map review-html", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	storeRef := flags.String("store", "", "Named Store config or Store DSN")
	storeURL := flags.String("store-url", "", legacyStoreURLFlagHelp)
	mapID := flags.String("map", "", "Plan map id")
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

	graph = filterMapReviewGraph(graph, *filter)
	catalog, err := runtime.GetProfileCatalog(ctx)
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		return err
	}
	if errors.Is(err, store.ErrNotFound) {
		catalog = store.ProfileCatalog{}
	}
	document := buildMapReviewDocument(graph, catalog)
	outputPath := strings.TrimSpace(*output)
	if outputPath == "" {
		outputPath = defaultMapReviewHTMLOutputPath(graph.Map.ID)
	}
	if err := writeMapReviewHTML(outputPath, document); err != nil {
		return err
	}
	report := mapReviewHTMLReport{
		OK:     true,
		MapID:  graph.Map.ID,
		Filter: strings.TrimSpace(*filter),
		Output: outputPath,
		Counts: document.Counts,
	}
	if *jsonOutput {
		return writeIndentedJSON(report)
	}
	printMapReviewHTMLReport(report)
	return nil
}

func defaultMapReviewHTMLOutputPath(mapID string) string {
	name := strings.NewReplacer("/", "-", "\\", "-", ":", "-", " ", "-").Replace(strings.TrimSpace(mapID))
	if name == "" {
		name = "map"
	}
	return filepath.Join(".runtime", "map-review", name+".html")
}

func writeMapReviewHTML(path string, document mapReviewDocument) error {
	if dir := filepath.Dir(path); dir != "." && dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	content, err := renderMapReviewHTML(document)
	if err != nil {
		return err
	}
	return os.WriteFile(path, []byte(content), 0o644)
}

func printMapReviewHTMLReport(report mapReviewHTMLReport) {
	fmt.Println("Map Review HTML")
	fmt.Printf("Map: %s\n", report.MapID)
	fmt.Printf("Output: %s\n", report.Output)
	fmt.Printf("Nodes: %d\n", report.Counts.Nodes)
	fmt.Printf("Paths: %d\n", report.Counts.Paths)
}
