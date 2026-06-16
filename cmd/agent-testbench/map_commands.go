package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"

	"agent-testbench/internal/domain/plangraph"
	"agent-testbench/internal/store"
)

type mapImportReport struct {
	OK     bool              `json:"ok"`
	Map    store.TestPlanMap `json:"map"`
	Counts mapCountsReport   `json:"counts"`
}

type mapCountsReport struct {
	Nodes            int `json:"nodes"`
	Edges            int `json:"edges"`
	Paths            int `json:"paths"`
	PathSteps        int `json:"pathSteps"`
	Materializations int `json:"materializations"`
}

type mapExplainReport struct {
	OK bool `json:"ok"`
	plangraph.Explanation
}

func runMap(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return errors.New("missing map command")
	}
	switch args[0] {
	case "import-workflows":
		return runMapImportWorkflows(ctx, args[1:])
	case "explain":
		return runMapExplain(ctx, args[1:])
	default:
		return fmt.Errorf("unknown map command: %s", args[0])
	}
}

func runMapImportWorkflows(ctx context.Context, args []string) error {
	flags := flag.NewFlagSet("map import-workflows", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	storeRef := flags.String("store", "", "Named Store config or Store DSN")
	storeURL := flags.String("store-url", "", legacyStoreURLFlagHelp)
	mapID := flags.String("map", "", "Plan map id")
	displayName := flags.String("display-name", "", "Plan map display name")
	description := flags.String("description", "", "Plan map description")
	jsonOutput := flags.Bool("json", false, "Emit a machine-readable JSON report")
	if err := flags.Parse(args); err != nil {
		return err
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
	})
	if err != nil {
		return err
	}
	if err := runtime.ReplaceTestPlanGraph(ctx, graph); err != nil {
		return err
	}
	report := mapImportReport{
		OK:  true,
		Map: graph.Map,
		Counts: mapCountsReport{
			Nodes:            len(graph.Nodes),
			Edges:            len(graph.Edges),
			Paths:            len(graph.Paths),
			PathSteps:        len(graph.PathSteps),
			Materializations: len(graph.Materializations),
		},
	}
	if *jsonOutput {
		return writeIndentedJSON(report)
	}
	printMapImportReport(report)
	return nil
}

func runMapExplain(ctx context.Context, args []string) error {
	flags := flag.NewFlagSet("map explain", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	storeRef := flags.String("store", "", "Named Store config or Store DSN")
	storeURL := flags.String("store-url", "", legacyStoreURLFlagHelp)
	mapID := flags.String("map", "", "Plan map id")
	caseID := flags.String("case", "", "Target case id")
	nodeID := flags.String("node", "", "Target plan node id")
	jsonOutput := flags.Bool("json", false, "Emit a machine-readable JSON report")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if strings.TrimSpace(*mapID) == "" {
		return errors.New("--map is required")
	}
	if strings.TrimSpace(*caseID) == "" && strings.TrimSpace(*nodeID) == "" {
		return errors.New("--case or --node is required")
	}
	runtime, cleanup, err := openRequiredCLIStore(ctx, *storeRef, *storeURL)
	if err != nil {
		return err
	}
	defer cleanup()
	graph, err := runtime.GetTestPlanGraph(ctx, *mapID)
	if err != nil {
		return err
	}
	explain, err := plangraph.ExplainCase(graph, plangraph.ExplainOptions{CaseID: *caseID, NodeID: *nodeID})
	if err != nil {
		return err
	}
	report := mapExplainReport{OK: true, Explanation: explain}
	if *jsonOutput {
		return writeIndentedJSON(report)
	}
	printMapExplainReport(report)
	return nil
}

func printMapImportReport(report mapImportReport) {
	fmt.Println("Workflow Map")
	fmt.Printf("Map: %s\n", report.Map.ID)
	fmt.Printf("Profile: %s\n", report.Map.ProfileID)
	fmt.Printf("Nodes: %d\n", report.Counts.Nodes)
	fmt.Printf("Paths: %d\n", report.Counts.Paths)
	fmt.Printf("Materializations: %d\n", report.Counts.Materializations)
}

func printMapExplainReport(report mapExplainReport) {
	fmt.Println("Map Explain")
	fmt.Printf("Map: %s\n", report.MapID)
	fmt.Printf("Target: %s\n", stringDefault(report.TargetCaseID, report.TargetNodeID))
	fmt.Printf("Operations: %d\n", len(report.Operations))
	for _, operation := range report.Operations {
		line := "- " + operation.Kind
		if operation.PathID != "" {
			line += " path=" + operation.PathID
		}
		if operation.UntilNodeID != "" {
			line += " until=" + operation.UntilNodeID
		}
		if operation.CaseID != "" {
			line += " case=" + operation.CaseID
		}
		fmt.Println(line)
	}
}
