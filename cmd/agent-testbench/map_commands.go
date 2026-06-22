package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"agent-testbench/internal/domain/commandline"
	"agent-testbench/internal/domain/mapplanner"
	"agent-testbench/internal/domain/plangraph"
	"agent-testbench/internal/store"
)

type mapImportReport struct {
	OK     bool              `json:"ok"`
	Map    store.TestPlanMap `json:"map"`
	Counts mapCountsReport   `json:"counts"`
}

type mapUpdateReport struct {
	OK     bool              `json:"ok"`
	Map    store.TestPlanMap `json:"map"`
	Counts mapCountsReport   `json:"counts"`
}

type mapVersionReport struct {
	OK      bool              `json:"ok"`
	Map     store.TestPlanMap `json:"map,omitempty"`
	Version mapVersionItem    `json:"version"`
	Counts  mapCountsReport   `json:"counts"`
}

type mapVersionsReport struct {
	OK       bool             `json:"ok"`
	MapID    string           `json:"mapId"`
	Count    int              `json:"count"`
	Versions []mapVersionItem `json:"versions"`
}

type mapVersionItem struct {
	ID        string `json:"id"`
	MapID     string `json:"mapId"`
	Version   string `json:"version"`
	Status    string `json:"status"`
	Summary   string `json:"summary,omitempty"`
	CreatedAt string `json:"createdAt,omitempty"`
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

type mapListReport struct {
	OK    bool          `json:"ok"`
	Count int           `json:"count"`
	Maps  []mapListItem `json:"maps"`
}

type mapListItem struct {
	ID               string `json:"id"`
	ProfileID        string `json:"profileId"`
	DisplayName      string `json:"displayName,omitempty"`
	Description      string `json:"description,omitempty"`
	Status           string `json:"status,omitempty"`
	NodeCount        int    `json:"nodeCount"`
	EdgeCount        int    `json:"edgeCount"`
	PathCount        int    `json:"pathCount"`
	Materializations int    `json:"materializations"`
	UpdatedAt        string `json:"updatedAt,omitempty"`
}

type mapPlansReport struct {
	OK    bool          `json:"ok"`
	MapID string        `json:"mapId"`
	Count int           `json:"count"`
	Plans []mapPlanItem `json:"plans"`
}

type mapPlanItem struct {
	ID            string `json:"id"`
	Status        string `json:"status,omitempty"`
	Mode          string `json:"mode,omitempty"`
	Scope         string `json:"scope,omitempty"`
	TargetKind    string `json:"targetKind,omitempty"`
	TargetID      string `json:"targetId,omitempty"`
	EnvironmentID string `json:"environmentId,omitempty"`
	CreatedAt     string `json:"createdAt,omitempty"`
	StartedAt     string `json:"startedAt,omitempty"`
	FinishedAt    string `json:"finishedAt,omitempty"`
	AtlasCommand  string `json:"atlasCommand,omitempty"`
	GateCommand   string `json:"gateCommand,omitempty"`
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

type mapGraphCommandFlags struct {
	flags      *flag.FlagSet
	storeRef   *string
	storeURL   *string
	mapID      *string
	jsonOutput *bool
}

func newMapGraphCommandFlags(commandName string) mapGraphCommandFlags {
	flags := flag.NewFlagSet(commandName, flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	return mapGraphCommandFlags{
		flags:      flags,
		storeRef:   flags.String("store", "", "Named Store config or Store DSN"),
		storeURL:   flags.String("store-url", "", legacyStoreURLFlagHelp),
		mapID:      flags.String("map", "", "Plan map id"),
		jsonOutput: flags.Bool("json", false, "Emit a machine-readable JSON report"),
	}
}

func (input mapGraphCommandFlags) parse(args []string) error {
	if err := input.flags.Parse(args); err != nil {
		return err
	}
	if input.flags.NArg() > 0 {
		return fmt.Errorf("%s does not accept positional arguments: %s", input.flags.Name(), strings.Join(input.flags.Args(), " "))
	}
	return nil
}

func runMap(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return printCommandHelp([]string{"map"})
	}
	switch args[0] {
	case "import-workflows":
		return runMapImportWorkflows(ctx, args[1:])
	case cliCommandList:
		return runMapList(ctx, args[1:])
	case "plans":
		return runMapPlans(ctx, args[1:])
	case "update":
		return runMapUpdate(ctx, args[1:])
	case "snapshot":
		return runMapSnapshot(ctx, args[1:])
	case "publish":
		return runMapPublish(ctx, args[1:])
	case "versions":
		return runMapVersions(ctx, args[1:])
	case "coverage":
		return runMapCoverage(ctx, args[1:])
	case cliCommandDoctor:
		return runMapDoctor(ctx, args[1:])
	case "diff":
		return runMapDiff(ctx, args[1:])
	case mapCommandValidation:
		return runMapValidation(ctx, args[1:])
	case "workflows":
		return runMapWorkflows(ctx, args[1:])
	case "inspect":
		return runMapInspect(ctx, args[1:])
	case "explain":
		return runMapExplain(ctx, args[1:])
	case "gate":
		return runMapGate(ctx, args[1:])
	case "run":
		return runMapRun(ctx, args[1:])
	case "plan":
		return runMapPlan(ctx, args[1:])
	case "atlas":
		return runMapAtlas(ctx, args[1:])
	default:
		return fmt.Errorf("unknown map command: %s", args[0])
	}
}

func runMapPlan(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return errors.New("missing map plan command")
	}
	switch args[0] {
	case "inspect":
		return runMapPlanInspect(ctx, args[1:])
	default:
		return fmt.Errorf("unknown map plan command: %s", args[0])
	}
}

type mapInspectOptions struct {
	StoreRef   string
	StoreURL   string
	MapID      string
	PlanID     string
	View       string
	Filter     string
	Limit      int
	JSONOutput bool
}

func runMapInspect(ctx context.Context, args []string) error {
	flags := flag.NewFlagSet("map inspect", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	view := flags.String("view", "list", "Map inspection view: list, workflows, coverage, plans, or plan")
	storeRef := flags.String("store", "", "Named Store config or Store DSN")
	storeURL := flags.String("store-url", "", legacyStoreURLFlagHelp)
	mapID := flags.String("map", "", "Plan map id")
	planID := flags.String("plan", "", "Planner run instance id")
	filter := flags.String("filter", "", "Filter path id, workflow id, or display name")
	limit := flags.Int("limit", 20, "Maximum number of plan history rows")
	jsonOutput := flags.Bool("json", false, "Emit a machine-readable JSON report")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() > 0 {
		return fmt.Errorf("map inspect does not accept positional arguments: %s", strings.Join(flags.Args(), " "))
	}
	options := mapInspectOptions{
		StoreRef:   *storeRef,
		StoreURL:   *storeURL,
		MapID:      *mapID,
		PlanID:     *planID,
		View:       *view,
		Filter:     *filter,
		Limit:      *limit,
		JSONOutput: *jsonOutput,
	}
	switch strings.TrimSpace(options.View) {
	case "", cliCommandList:
		return runMapListWithOptions(ctx, mapListOptions{
			StoreRef:   options.StoreRef,
			StoreURL:   options.StoreURL,
			JSONOutput: options.JSONOutput,
		})
	case "workflows":
		return runMapWorkflowsWithOptions(ctx, mapWorkflowsOptions{
			StoreRef:   options.StoreRef,
			StoreURL:   options.StoreURL,
			MapID:      options.MapID,
			Filter:     options.Filter,
			JSONOutput: options.JSONOutput,
		})
	case "coverage":
		return runMapCoverageWithOptions(ctx, mapCoverageOptions{
			StoreRef:   options.StoreRef,
			StoreURL:   options.StoreURL,
			MapID:      options.MapID,
			JSONOutput: options.JSONOutput,
		})
	case "plans":
		return runMapPlansWithOptions(ctx, mapPlansOptions{
			StoreRef:   options.StoreRef,
			StoreURL:   options.StoreURL,
			MapID:      options.MapID,
			Limit:      options.Limit,
			JSONOutput: options.JSONOutput,
		})
	case "plan":
		return runMapPlanInspectWithOptions(ctx, mapPlanInspectOptions{
			StoreRef:   options.StoreRef,
			StoreURL:   options.StoreURL,
			PlanID:     options.PlanID,
			JSONOutput: options.JSONOutput,
		})
	default:
		return fmt.Errorf("unknown map inspect view: %s", options.View)
	}
}

func runMapUpdate(ctx context.Context, args []string) error {
	flags := flag.NewFlagSet("map update", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	storeRef := flags.String("store", "", "Named Store config or Store DSN")
	storeURL := flags.String("store-url", "", legacyStoreURLFlagHelp)
	mapID := flags.String("map", "", "Plan map id")
	displayName := flags.String("display-name", "", "New map display name")
	description := flags.String("description", "", "New map description")
	status := flags.String("status", "", "New map lifecycle status")
	jsonOutput := flags.Bool("json", false, "Emit a machine-readable JSON report")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() > 0 {
		return fmt.Errorf("map update does not accept positional arguments: %s", strings.Join(flags.Args(), " "))
	}
	runtime, graph, cleanup, err := openRequiredMapGraphForCLI(ctx, *storeRef, *storeURL, *mapID)
	if err != nil {
		return err
	}
	defer cleanup()
	if strings.TrimSpace(*displayName) != "" {
		graph.Map.DisplayName = strings.TrimSpace(*displayName)
	}
	if strings.TrimSpace(*description) != "" {
		graph.Map.Description = strings.TrimSpace(*description)
	}
	if strings.TrimSpace(*status) != "" {
		graph.Map.Status = strings.TrimSpace(*status)
	}
	graph.Map.UpdatedAt = time.Now().UTC()
	if err := runtime.ReplaceTestPlanGraph(ctx, graph); err != nil {
		return err
	}
	report := mapUpdateReport{OK: true, Map: graph.Map, Counts: mapCountsFromGraph(graph)}
	if *jsonOutput {
		return writeIndentedJSON(report)
	}
	printMapUpdateReport(report)
	return nil
}

func runMapSnapshot(ctx context.Context, args []string) error {
	flags := flag.NewFlagSet("map snapshot", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	storeRef := flags.String("store", "", "Named Store config or Store DSN")
	storeURL := flags.String("store-url", "", legacyStoreURLFlagHelp)
	mapID := flags.String("map", "", "Plan map id")
	version := flags.String("version", "", "Version label")
	status := flags.String("status", "snapshot", "Version status")
	summary := flags.String("summary", "", "Version summary")
	jsonOutput := flags.Bool("json", false, "Emit a machine-readable JSON report")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() > 0 {
		return fmt.Errorf("map snapshot does not accept positional arguments: %s", strings.Join(flags.Args(), " "))
	}
	runtime, graph, cleanup, err := openRequiredMapGraphForCLI(ctx, *storeRef, *storeURL, *mapID)
	if err != nil {
		return err
	}
	defer cleanup()
	item, err := saveMapVersion(ctx, runtime, graph, *version, *status, *summary)
	if err != nil {
		return err
	}
	report := mapVersionReport{OK: true, Map: graph.Map, Version: mapVersionItemFromStore(item), Counts: mapCountsFromGraph(graph)}
	if *jsonOutput {
		return writeIndentedJSON(report)
	}
	printMapVersionReport("Test Scenario Atlas Snapshot", report)
	return nil
}

func runMapPublish(ctx context.Context, args []string) error {
	flags := flag.NewFlagSet("map publish", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	storeRef := flags.String("store", "", "Named Store config or Store DSN")
	storeURL := flags.String("store-url", "", legacyStoreURLFlagHelp)
	mapID := flags.String("map", "", "Plan map id")
	version := flags.String("version", "", "Published version label")
	summary := flags.String("summary", "", "Published version summary")
	jsonOutput := flags.Bool("json", false, "Emit a machine-readable JSON report")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() > 0 {
		return fmt.Errorf("map publish does not accept positional arguments: %s", strings.Join(flags.Args(), " "))
	}
	runtime, graph, cleanup, err := openRequiredMapGraphForCLI(ctx, *storeRef, *storeURL, *mapID)
	if err != nil {
		return err
	}
	defer cleanup()
	graph.Map.Status = "active"
	graph.Map.UpdatedAt = time.Now().UTC()
	if err := runtime.ReplaceTestPlanGraph(ctx, graph); err != nil {
		return err
	}
	item, err := saveMapVersion(ctx, runtime, graph, *version, "published", *summary)
	if err != nil {
		return err
	}
	report := mapVersionReport{OK: true, Map: graph.Map, Version: mapVersionItemFromStore(item), Counts: mapCountsFromGraph(graph)}
	if *jsonOutput {
		return writeIndentedJSON(report)
	}
	printMapVersionReport("Test Scenario Atlas Published", report)
	return nil
}

func runMapVersions(ctx context.Context, args []string) error {
	input := newMapGraphCommandFlags("map versions")
	if err := input.parse(args); err != nil {
		return err
	}
	if strings.TrimSpace(*input.mapID) == "" {
		return errors.New("--map is required")
	}
	runtime, cleanup, err := openRequiredCLIStore(ctx, *input.storeRef, *input.storeURL)
	if err != nil {
		return err
	}
	defer cleanup()
	versions, err := runtime.ListTestPlanMapVersions(ctx, *input.mapID)
	if err != nil {
		return err
	}
	report := buildMapVersionsReport(*input.mapID, versions)
	if *input.jsonOutput {
		return writeIndentedJSON(report)
	}
	printMapVersionsReport(report)
	return nil
}

type mapListOptions struct {
	StoreRef   string
	StoreURL   string
	JSONOutput bool
}

func runMapList(ctx context.Context, args []string) error {
	flags := flag.NewFlagSet("map list", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	storeRef := flags.String("store", "", "Named Store config or Store DSN")
	storeURL := flags.String("store-url", "", legacyStoreURLFlagHelp)
	jsonOutput := flags.Bool("json", false, "Emit a machine-readable JSON report")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() > 0 {
		return fmt.Errorf("map list does not accept positional arguments: %s", strings.Join(flags.Args(), " "))
	}
	return runMapListWithOptions(ctx, mapListOptions{
		StoreRef:   *storeRef,
		StoreURL:   *storeURL,
		JSONOutput: *jsonOutput,
	})
}

func runMapListWithOptions(ctx context.Context, options mapListOptions) error {
	runtime, cleanup, err := openRequiredCLIStore(ctx, options.StoreRef, options.StoreURL)
	if err != nil {
		return err
	}
	defer cleanup()
	maps, err := runtime.ListTestPlanMaps(ctx)
	if err != nil {
		return err
	}
	report := buildMapListReport(maps)
	if options.JSONOutput {
		return writeIndentedJSON(report)
	}
	printMapListReport(report)
	return nil
}

type mapPlansOptions struct {
	StoreRef   string
	StoreURL   string
	MapID      string
	Limit      int
	JSONOutput bool
}

func runMapPlans(ctx context.Context, args []string) error {
	flags := flag.NewFlagSet("map plans", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	storeRef := flags.String("store", "", "Named Store config or Store DSN")
	storeURL := flags.String("store-url", "", legacyStoreURLFlagHelp)
	mapID := flags.String("map", "", "Plan map id")
	limit := flags.Int("limit", 20, "Maximum number of plan history rows")
	jsonOutput := flags.Bool("json", false, "Emit a machine-readable JSON report")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() > 0 {
		return fmt.Errorf("map plans does not accept positional arguments: %s", strings.Join(flags.Args(), " "))
	}
	return runMapPlansWithOptions(ctx, mapPlansOptions{
		StoreRef:   *storeRef,
		StoreURL:   *storeURL,
		MapID:      *mapID,
		Limit:      *limit,
		JSONOutput: *jsonOutput,
	})
}

func runMapPlansWithOptions(ctx context.Context, options mapPlansOptions) error {
	if strings.TrimSpace(options.MapID) == "" {
		return errors.New("--map is required")
	}
	runtime, cleanup, err := openRequiredCLIStore(ctx, options.StoreRef, options.StoreURL)
	if err != nil {
		return err
	}
	defer cleanup()
	plans, err := runtime.ListTestMapPlans(ctx, options.MapID, options.Limit)
	if err != nil {
		return err
	}
	report := buildMapPlansReport(options.MapID, plans)
	if options.JSONOutput {
		return writeIndentedJSON(report)
	}
	printMapPlansReport(report)
	return nil
}

func runMapImportWorkflows(ctx context.Context, args []string) error {
	flags := flag.NewFlagSet("map import-workflows", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	storeRef := flags.String("store", "", "Named Store config or Store DSN")
	storeURL := flags.String("store-url", "", legacyStoreURLFlagHelp)
	mapID := flags.String("map", "", "Plan map id")
	displayName := flags.String("display-name", "", "Plan map display name")
	description := flags.String("description", "", "Plan map description")
	var workflowIDs stringListFlag
	flags.Var(&workflowIDs, "workflow", "Workflow id to import into the map; repeat for multiple workflows")
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
	if err := runtime.ReplaceTestPlanGraph(ctx, graph); err != nil {
		return err
	}
	report := mapImportReport{
		OK:     true,
		Map:    graph.Map,
		Counts: mapCountsFromGraph(graph),
	}
	if *jsonOutput {
		return writeIndentedJSON(report)
	}
	printMapImportReport(report)
	return nil
}

type mapWorkflowsOptions struct {
	StoreRef   string
	StoreURL   string
	MapID      string
	Filter     string
	JSONOutput bool
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
	return runMapWorkflowsWithOptions(ctx, mapWorkflowsOptions{
		StoreRef:   *storeRef,
		StoreURL:   *storeURL,
		MapID:      *mapID,
		Filter:     *filter,
		JSONOutput: *jsonOutput,
	})
}

func runMapWorkflowsWithOptions(ctx context.Context, options mapWorkflowsOptions) error {
	if strings.TrimSpace(options.MapID) == "" {
		return errors.New("--map is required")
	}
	_, graph, cleanup, err := openMapGraphForCLI(ctx, options.StoreRef, options.StoreURL, options.MapID)
	if err != nil {
		return err
	}
	defer cleanup()
	report := buildMapWorkflowsReport(graph, options.Filter)
	if options.JSONOutput {
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
	interfaceID := flags.String("interface", "", "Filter validation cases by interface node id")
	validationFamily := flags.String("validation-family", "", "Filter validation cases by family such as empty/null, length, type, enum, state, boundary, or contract")
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
		MapID:            *mapID,
		EnvironmentID:    *environmentID,
		Scope:            *scope,
		CaseID:           *caseID,
		NodeID:           *nodeID,
		PathID:           *pathID,
		WorkflowID:       *workflowID,
		InterfaceNodeID:  *interfaceID,
		ValidationFamily: *validationFamily,
		PlannerMode:      mapplanner.ModeExplain,
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

func printMapUpdateReport(report mapUpdateReport) {
	fmt.Println("Workflow Map Updated")
	fmt.Printf("Map: %s\n", report.Map.ID)
	fmt.Printf("Status: %s\n", report.Map.Status)
	fmt.Printf("Nodes: %d\n", report.Counts.Nodes)
	fmt.Printf("Paths: %d\n", report.Counts.Paths)
}

func saveMapVersion(ctx context.Context, runtime store.Store, graph store.TestPlanGraph, version string, status string, summary string) (store.TestPlanMapVersion, error) {
	version = strings.TrimSpace(version)
	if version == "" {
		return store.TestPlanMapVersion{}, errors.New("--version is required")
	}
	raw, err := json.Marshal(graph)
	if err != nil {
		return store.TestPlanMapVersion{}, err
	}
	return runtime.SaveTestPlanMapVersion(ctx, store.TestPlanMapVersion{
		MapID:     graph.Map.ID,
		Version:   version,
		Status:    strings.TrimSpace(status),
		Summary:   strings.TrimSpace(summary),
		GraphJSON: string(raw),
		CreatedAt: time.Now().UTC(),
	})
}

func buildMapVersionsReport(mapID string, versions []store.TestPlanMapVersion) mapVersionsReport {
	items := make([]mapVersionItem, 0, len(versions))
	for _, item := range versions {
		items = append(items, mapVersionItemFromStore(item))
	}
	return mapVersionsReport{OK: true, MapID: strings.TrimSpace(mapID), Count: len(items), Versions: items}
}

func mapVersionItemFromStore(item store.TestPlanMapVersion) mapVersionItem {
	return mapVersionItem{
		ID:        item.ID,
		MapID:     item.MapID,
		Version:   item.Version,
		Status:    item.Status,
		Summary:   item.Summary,
		CreatedAt: formatMapTime(item.CreatedAt),
	}
}

func printMapVersionReport(title string, report mapVersionReport) {
	fmt.Println(title)
	fmt.Printf("Map: %s\n", report.Version.MapID)
	fmt.Printf("Version: %s\n", report.Version.Version)
	fmt.Printf("Status: %s\n", report.Version.Status)
}

func printMapVersionsReport(report mapVersionsReport) {
	fmt.Println("Test Scenario Atlas Versions")
	fmt.Printf("Map: %s\n", report.MapID)
	fmt.Printf("Versions: %d\n", report.Count)
	for _, item := range report.Versions {
		fmt.Printf("- %s status=%s summary=%s\n", item.Version, item.Status, item.Summary)
	}
}

func mapCountsFromGraph(graph store.TestPlanGraph) mapCountsReport {
	return mapCountsReport{
		Nodes:            len(graph.Nodes),
		Edges:            len(graph.Edges),
		Paths:            len(graph.Paths),
		PathSteps:        len(graph.PathSteps),
		Materializations: len(graph.Materializations),
	}
}

func buildMapListReport(maps []store.TestPlanMapSummary) mapListReport {
	items := make([]mapListItem, 0, len(maps))
	for _, item := range maps {
		items = append(items, mapListItem{
			ID:               item.ID,
			ProfileID:        item.ProfileID,
			DisplayName:      item.DisplayName,
			Description:      item.Description,
			Status:           item.Status,
			NodeCount:        item.NodeCount,
			EdgeCount:        item.EdgeCount,
			PathCount:        item.PathCount,
			Materializations: item.MaterializationCount,
			UpdatedAt:        formatMapTime(item.UpdatedAt),
		})
	}
	return mapListReport{OK: true, Count: len(items), Maps: items}
}

func buildMapPlansReport(mapID string, plans []store.TestMapPlanInstance) mapPlansReport {
	items := make([]mapPlanItem, 0, len(plans))
	for _, item := range plans {
		items = append(items, mapPlanItem{
			ID:            item.ID,
			Status:        item.Status,
			Mode:          item.Mode,
			Scope:         item.Scope,
			TargetKind:    item.TargetKind,
			TargetID:      item.TargetID,
			EnvironmentID: item.EnvironmentID,
			CreatedAt:     formatMapTime(item.CreatedAt),
			StartedAt:     formatMapTime(item.StartedAt),
			FinishedAt:    formatMapTime(item.FinishedAt),
			AtlasCommand:  "agent-testbench map atlas --map " + commandline.ShellQuote(item.MapID) + " --plan " + commandline.ShellQuote(item.ID) + " --json",
			GateCommand:   "agent-testbench map gate --plan " + commandline.ShellQuote(item.ID) + " --require-passed --require-tasks --require-evidence --json",
		})
	}
	return mapPlansReport{OK: true, MapID: mapID, Count: len(items), Plans: items}
}

func formatMapTime(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.UTC().Format(time.RFC3339)
}

func printMapListReport(report mapListReport) {
	fmt.Println("Workflow Maps")
	fmt.Printf("Maps: %d\n", report.Count)
	for _, item := range report.Maps {
		fmt.Printf("- %s profile=%s status=%s nodes=%d paths=%d materializations=%d\n",
			item.ID, item.ProfileID, item.Status, item.NodeCount, item.PathCount, item.Materializations)
	}
}

func printMapPlansReport(report mapPlansReport) {
	fmt.Println("Map Plans")
	fmt.Printf("Map: %s\n", report.MapID)
	fmt.Printf("Plans: %d\n", report.Count)
	for _, item := range report.Plans {
		fmt.Printf("- %s status=%s mode=%s scope=%s env=%s\n",
			item.ID, item.Status, item.Mode, item.Scope, item.EnvironmentID)
	}
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
