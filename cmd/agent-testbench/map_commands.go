package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"agent-testbench/internal/domain/mapplanner"
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
	mapplanner.Plan
}

type mapWorkflowsReport struct {
	OK        bool                `json:"ok"`
	MapID     string              `json:"mapId"`
	Filter    string              `json:"filter,omitempty"`
	Count     int                 `json:"count"`
	Workflows []mapWorkflowReport `json:"workflows"`
}

type mapWorkflowReport struct {
	PathID      string `json:"pathId"`
	WorkflowID  string `json:"workflowId"`
	DisplayName string `json:"displayName,omitempty"`
	Status      string `json:"status,omitempty"`
	StepCount   int    `json:"stepCount"`
	FirstNodeID string `json:"firstNodeId,omitempty"`
	LastNodeID  string `json:"lastNodeId,omitempty"`
}

func runMap(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return errors.New("missing map command")
	}
	switch args[0] {
	case "import-workflows":
		return runMapImportWorkflows(ctx, args[1:])
	case "workflows":
		return runMapWorkflows(ctx, args[1:])
	case "explain":
		return runMapExplain(ctx, args[1:])
	case "run":
		return runMapRun(ctx, args[1:])
	case "review-html":
		return runMapReviewHTML(ctx, args[1:])
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

func runMapWorkflows(ctx context.Context, args []string) error {
	flags := flag.NewFlagSet("map workflows", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	storeRef := flags.String("store", "", "Named Store config or Store DSN")
	storeURL := flags.String("store-url", "", legacyStoreURLFlagHelp)
	mapID := flags.String("map", "", "Plan map id")
	filter := flags.String("filter", "", "Filter path id, workflow id, or display name")
	jsonOutput := flags.Bool("json", false, "Emit a machine-readable JSON report")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if strings.TrimSpace(*mapID) == "" {
		return errors.New("--map is required")
	}
	_, graph, cleanup, err := openMapGraphForCLI(ctx, *storeRef, *storeURL, *mapID)
	if err != nil {
		return err
	}
	defer cleanup()
	report := buildMapWorkflowsReport(graph, *filter)
	if *jsonOutput {
		return writeIndentedJSON(report)
	}
	printMapWorkflowsReport(report)
	return nil
}

func runMapExplain(ctx context.Context, args []string) error {
	flags := flag.NewFlagSet("map explain", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	storeRef := flags.String("store", "", "Named Store config or Store DSN")
	storeURL := flags.String("store-url", "", legacyStoreURLFlagHelp)
	mapID := flags.String("map", "", "Plan map id")
	scope := flags.String("scope", "", "Explain scope: all, workflows, cases")
	caseID := flags.String("case", "", "Target case id")
	nodeID := flags.String("node", "", "Target plan node id")
	pathID := flags.String("path", "", "Target map path id")
	workflowID := flags.String("workflow", "", "Target workflow id")
	environmentID := flags.String("environment", "", "Environment id to bind into the planner output")
	savePlan := flags.Bool("save", false, "Persist the generated planner instance and task DAG")
	jsonOutput := flags.Bool("json", false, "Emit a machine-readable JSON report")
	if err := flags.Parse(args); err != nil {
		return err
	}
	runtime, graph, cleanup, err := openRequiredMapGraphForCLI(ctx, *storeRef, *storeURL, *mapID)
	if err != nil {
		return err
	}
	defer cleanup()
	plan, err := mapplanner.Explain(graph, mapplanner.Query{
		MapID:         *mapID,
		EnvironmentID: *environmentID,
		Scope:         *scope,
		CaseID:        *caseID,
		NodeID:        *nodeID,
		PathID:        *pathID,
		WorkflowID:    *workflowID,
		PlannerMode:   mapplanner.ModeExplain,
	})
	if err != nil {
		return err
	}
	if *savePlan {
		now := time.Now().UTC()
		plan.ID = "plan." + safeReportID(plan.MapID) + "." + now.Format("20060102T150405.000000000Z")
		plan.CreatedAt = now
		record, err := mapplanner.RecordFromPlan(plan, now)
		if err != nil {
			return err
		}
		if err := runtime.SaveTestMapPlan(ctx, record); err != nil {
			return err
		}
	}
	report := mapExplainReport{OK: true, Plan: plan}
	if *jsonOutput {
		return writeIndentedJSON(report)
	}
	printMapExplainReport(report)
	return nil
}

func openMapGraphForCLI(ctx context.Context, storeRef string, storeURL string, mapID string) (store.Store, store.TestPlanGraph, func(), error) {
	runtime, cleanup, err := openRequiredCLIStore(ctx, storeRef, storeURL)
	if err != nil {
		return nil, store.TestPlanGraph{}, func() {}, err
	}
	graph, err := runtime.GetTestPlanGraph(ctx, mapID)
	if err != nil {
		cleanup()
		return nil, store.TestPlanGraph{}, func() {}, err
	}
	return runtime, graph, cleanup, nil
}

func openRequiredMapGraphForCLI(ctx context.Context, storeRef string, storeURL string, mapID string) (store.Store, store.TestPlanGraph, func(), error) {
	if strings.TrimSpace(mapID) == "" {
		return nil, store.TestPlanGraph{}, func() {}, errors.New("--map is required")
	}
	return openMapGraphForCLI(ctx, storeRef, storeURL, mapID)
}

func printMapImportReport(report mapImportReport) {
	fmt.Println("Workflow Map")
	fmt.Printf("Map: %s\n", report.Map.ID)
	fmt.Printf("Profile: %s\n", report.Map.ProfileID)
	fmt.Printf("Nodes: %d\n", report.Counts.Nodes)
	fmt.Printf("Paths: %d\n", report.Counts.Paths)
	fmt.Printf("Materializations: %d\n", report.Counts.Materializations)
}

func buildMapWorkflowsReport(graph store.TestPlanGraph, filter string) mapWorkflowsReport {
	displayFilter := strings.TrimSpace(filter)
	filter = strings.ToLower(displayFilter)
	stepCounts := map[string]int{}
	firstNodeByPath := map[string]string{}
	lastNodeByPath := map[string]string{}
	for _, step := range graph.PathSteps {
		stepCounts[step.PathID]++
		if _, ok := firstNodeByPath[step.PathID]; !ok {
			firstNodeByPath[step.PathID] = step.NodeID
		}
		lastNodeByPath[step.PathID] = step.NodeID
	}
	workflows := make([]mapWorkflowReport, 0, len(graph.Paths))
	for _, path := range graph.Paths {
		if filter != "" && !mapWorkflowMatchesFilter(path, filter) {
			continue
		}
		workflows = append(workflows, mapWorkflowReport{
			PathID:      path.ID,
			WorkflowID:  path.WorkflowID,
			DisplayName: path.DisplayName,
			Status:      path.Status,
			StepCount:   stepCounts[path.ID],
			FirstNodeID: firstNodeByPath[path.ID],
			LastNodeID:  lastNodeByPath[path.ID],
		})
	}
	return mapWorkflowsReport{
		OK:        true,
		MapID:     graph.Map.ID,
		Filter:    displayFilter,
		Count:     len(workflows),
		Workflows: workflows,
	}
}

func mapWorkflowMatchesFilter(path store.TestPlanPath, filter string) bool {
	return strings.Contains(strings.ToLower(path.ID), filter) ||
		strings.Contains(strings.ToLower(path.WorkflowID), filter) ||
		strings.Contains(strings.ToLower(path.DisplayName), filter)
}

func printMapWorkflowsReport(report mapWorkflowsReport) {
	fmt.Println("Map Workflows")
	fmt.Printf("Map: %s\n", report.MapID)
	fmt.Printf("Workflows: %d\n", report.Count)
	for _, workflow := range report.Workflows {
		fmt.Printf("- %s workflow=%s steps=%d first=%s last=%s\n",
			workflow.PathID, workflow.WorkflowID, workflow.StepCount, workflow.FirstNodeID, workflow.LastNodeID)
	}
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
