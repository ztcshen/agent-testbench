package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"

	"agent-testbench/internal/store"
)

type mapDiffReport struct {
	OK               bool           `json:"ok"`
	MapID            string         `json:"mapId"`
	From             string         `json:"from"`
	To               string         `json:"to"`
	Changed          bool           `json:"changed"`
	Nodes            mapDiffSection `json:"nodes"`
	Edges            mapDiffSection `json:"edges"`
	Paths            mapDiffSection `json:"paths"`
	Materializations mapDiffSection `json:"materializations"`
}

type mapDiffSection struct {
	Before  int      `json:"before"`
	After   int      `json:"after"`
	Added   []string `json:"added,omitempty"`
	Removed []string `json:"removed,omitempty"`
}

func runMapDiff(ctx context.Context, args []string) error {
	flags := flag.NewFlagSet("map diff", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	storeRef := flags.String("store", "", "Named Store config or Store DSN")
	storeURL := flags.String("store-url", "", legacyStoreURLFlagHelp)
	mapID := flags.String("map", "", "Plan map id")
	from := flags.String("from", "", "Source version label")
	to := flags.String("to", "working", "Target version label or working")
	jsonOutput := flags.Bool("json", false, "Emit a machine-readable JSON report")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() > 0 {
		return fmt.Errorf("map diff does not accept positional arguments: %s", strings.Join(flags.Args(), " "))
	}
	if strings.TrimSpace(*from) == "" {
		return errors.New("--from is required")
	}
	runtime, cleanup, err := openRequiredCLIStore(ctx, *storeRef, *storeURL)
	if err != nil {
		return err
	}
	defer cleanup()
	before, err := loadMapGraphRevision(ctx, runtime, *mapID, *from)
	if err != nil {
		return err
	}
	after, err := loadMapGraphRevision(ctx, runtime, *mapID, *to)
	if err != nil {
		return err
	}
	report := buildMapDiffReport(before, after, *from, *to)
	if *jsonOutput {
		return writeIndentedJSON(report)
	}
	printMapDiffReport(report)
	return nil
}

func loadMapGraphRevision(ctx context.Context, runtime store.Store, mapID string, revision string) (store.TestPlanGraph, error) {
	mapID = strings.TrimSpace(mapID)
	revision = strings.TrimSpace(revision)
	if mapID == "" {
		return store.TestPlanGraph{}, errors.New("--map is required")
	}
	if revision == "" || revision == "working" || revision == "current" {
		return runtime.GetTestPlanGraph(ctx, mapID)
	}
	versions, err := runtime.ListTestPlanMapVersions(ctx, mapID)
	if err != nil {
		return store.TestPlanGraph{}, err
	}
	for _, item := range versions {
		if item.Version != revision && item.ID != revision {
			continue
		}
		var graph store.TestPlanGraph
		if err := json.Unmarshal([]byte(item.GraphJSON), &graph); err != nil {
			return store.TestPlanGraph{}, fmt.Errorf("decode map version %s: %w", revision, err)
		}
		return graph, nil
	}
	return store.TestPlanGraph{}, fmt.Errorf("map version not found: %s", revision)
}

func buildMapDiffReport(before store.TestPlanGraph, after store.TestPlanGraph, from string, to string) mapDiffReport {
	report := mapDiffReport{
		OK:               true,
		MapID:            stringDefault(after.Map.ID, before.Map.ID),
		From:             strings.TrimSpace(from),
		To:               strings.TrimSpace(to),
		Nodes:            mapDiffSectionByID(testPlanNodeIDs(before.Nodes), testPlanNodeIDs(after.Nodes)),
		Edges:            mapDiffSectionByID(testPlanEdgeIDs(before.Edges), testPlanEdgeIDs(after.Edges)),
		Paths:            mapDiffSectionByID(testPlanPathIDs(before.Paths), testPlanPathIDs(after.Paths)),
		Materializations: mapDiffSectionByID(testPlanMaterializationIDs(before.Materializations), testPlanMaterializationIDs(after.Materializations)),
	}
	report.Changed = mapDiffSectionChanged(report.Nodes) || mapDiffSectionChanged(report.Edges) || mapDiffSectionChanged(report.Paths) || mapDiffSectionChanged(report.Materializations)
	return report
}

func mapDiffSectionByID(before []string, after []string) mapDiffSection {
	beforeSet := stringSet(before)
	afterSet := stringSet(after)
	return mapDiffSection{
		Before:  len(beforeSet),
		After:   len(afterSet),
		Added:   sortedSetDifference(afterSet, beforeSet),
		Removed: sortedSetDifference(beforeSet, afterSet),
	}
}

func stringSet(values []string) map[string]bool {
	out := map[string]bool{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			out[value] = true
		}
	}
	return out
}

func sortedSetDifference(left map[string]bool, right map[string]bool) []string {
	out := []string{}
	for value := range left {
		if !right[value] {
			out = append(out, value)
		}
	}
	sort.Strings(out)
	return out
}

func mapDiffSectionChanged(section mapDiffSection) bool {
	return len(section.Added) > 0 || len(section.Removed) > 0 || section.Before != section.After
}

func testPlanNodeIDs(items []store.TestPlanNode) []string {
	out := make([]string, 0, len(items))
	for _, item := range items {
		out = append(out, item.ID)
	}
	return out
}

func testPlanEdgeIDs(items []store.TestPlanEdge) []string {
	out := make([]string, 0, len(items))
	for _, item := range items {
		out = append(out, item.ID)
	}
	return out
}

func testPlanPathIDs(items []store.TestPlanPath) []string {
	out := make([]string, 0, len(items))
	for _, item := range items {
		out = append(out, item.ID)
	}
	return out
}

func testPlanMaterializationIDs(items []store.TestPlanMaterialization) []string {
	out := make([]string, 0, len(items))
	for _, item := range items {
		out = append(out, item.ID)
	}
	return out
}

func printMapDiffReport(report mapDiffReport) {
	fmt.Println("Map Diff")
	fmt.Printf("Map: %s\n", report.MapID)
	fmt.Printf("From: %s\n", report.From)
	fmt.Printf("To: %s\n", report.To)
	fmt.Printf("Changed: %t\n", report.Changed)
	fmt.Printf("Nodes: %d -> %d added=%d removed=%d\n", report.Nodes.Before, report.Nodes.After, len(report.Nodes.Added), len(report.Nodes.Removed))
}
