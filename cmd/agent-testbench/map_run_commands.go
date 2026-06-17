package main

import (
	"context"
	"errors"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"time"

	"agent-testbench/internal/domain/mapplanner"
	"agent-testbench/internal/store"
)

type mapRunOptions struct {
	storeRef       string
	storeURL       string
	mapID          string
	planID         string
	scope          string
	caseID         string
	nodeID         string
	pathID         string
	workflowID     string
	environmentID  string
	baseURL        string
	evidenceDir    string
	timeoutSeconds int
	jsonOutput     bool
}

func runMapRun(ctx context.Context, args []string) error {
	if len(args) > 0 && args[0] == "explain" {
		return runMapRunExplain(ctx, args[1:])
	}
	options, err := parseMapRunOptions(args)
	if err != nil {
		return err
	}
	runtime, graph, cleanup, err := openMapRunRuntime(ctx, options)
	if err != nil {
		return err
	}
	defer cleanup()
	record, err := mapRunPlanRecord(ctx, runtime, graph, options)
	if err != nil {
		return err
	}
	if err := runtime.SaveTestMapPlan(ctx, record); err != nil {
		return err
	}
	executor := newMapRunExecutor(ctx, runtime, graph, options)
	record = executor.execute(record)
	if err := runtime.SaveTestMapPlan(ctx, record); err != nil {
		return err
	}
	report := mapRunReportFromRecord(record)
	if options.jsonOutput {
		if err := writeIndentedJSON(report); err != nil {
			return err
		}
	} else {
		printMapRunReport(report)
	}
	if report.Status == store.StatusFailed || report.Status == mapplanner.TaskStatusBlocked {
		return errors.New("map run failed")
	}
	return nil
}

func parseMapRunOptions(args []string) (mapRunOptions, error) {
	flags := flag.NewFlagSet("map run", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	options := mapRunOptions{}
	storeRef := flags.String("store", "", "Named Store config or Store DSN")
	storeURL := flags.String("store-url", "", legacyStoreURLFlagHelp)
	mapID := flags.String("map", "", "Plan map id")
	planID := flags.String("plan", "", "Existing planner instance id to execute")
	scope := flags.String("scope", "", "Run scope: all, workflows, cases")
	caseID := flags.String("case", "", "Target case id")
	nodeID := flags.String("node", "", "Target plan node id")
	pathID := flags.String("path", "", "Target map path id")
	workflowID := flags.String("workflow", "", "Target workflow id")
	environmentID := flags.String("environment", "", "Environment id to bind into runs")
	baseURL := flags.String("base-url", "", "Base URL override for API case execution")
	evidenceDir := flags.String("evidence-dir", filepath.Join(".runtime", "map-runs"), "Evidence output directory")
	timeoutSeconds := flags.Int("timeout-seconds", 0, "Request timeout in seconds for Store catalog case execution")
	jsonOutput := flags.Bool("json", false, "Emit a machine-readable JSON report")
	if err := flags.Parse(args); err != nil {
		return mapRunOptions{}, err
	}
	options.storeRef = *storeRef
	options.storeURL = *storeURL
	options.mapID = strings.TrimSpace(*mapID)
	options.planID = strings.TrimSpace(*planID)
	options.scope = strings.TrimSpace(*scope)
	options.caseID = strings.TrimSpace(*caseID)
	options.nodeID = strings.TrimSpace(*nodeID)
	options.pathID = strings.TrimSpace(*pathID)
	options.workflowID = strings.TrimSpace(*workflowID)
	options.environmentID = strings.TrimSpace(*environmentID)
	options.baseURL = strings.TrimSpace(*baseURL)
	options.evidenceDir = strings.TrimSpace(*evidenceDir)
	options.timeoutSeconds = *timeoutSeconds
	options.jsonOutput = *jsonOutput
	return options, nil
}

func openMapRunRuntime(ctx context.Context, options mapRunOptions) (store.Store, store.TestPlanGraph, func(), error) {
	runtime, cleanup, err := openRequiredCLIStore(ctx, options.storeRef, options.storeURL)
	if err != nil {
		return nil, store.TestPlanGraph{}, func() {}, err
	}
	mapID := options.mapID
	if mapID == "" && options.planID != "" {
		record, err := runtime.GetTestMapPlan(ctx, options.planID)
		if err != nil {
			cleanup()
			return nil, store.TestPlanGraph{}, func() {}, err
		}
		mapID = record.Instance.MapID
	}
	if mapID == "" {
		cleanup()
		return nil, store.TestPlanGraph{}, func() {}, errors.New("--map or --plan is required")
	}
	graph, err := runtime.GetTestPlanGraph(ctx, mapID)
	if err != nil {
		cleanup()
		return nil, store.TestPlanGraph{}, func() {}, err
	}
	return runtime, graph, cleanup, nil
}

func mapRunPlanRecord(ctx context.Context, runtime store.Store, graph store.TestPlanGraph, options mapRunOptions) (store.TestMapPlanRecord, error) {
	if options.planID != "" {
		record, err := runtime.GetTestMapPlan(ctx, options.planID)
		if err != nil {
			return store.TestMapPlanRecord{}, err
		}
		return prepareExistingMapRunRecord(record, options), nil
	}
	plan, err := mapplanner.Explain(graph, mapplanner.Query{
		MapID:         options.mapID,
		EnvironmentID: options.environmentID,
		Scope:         options.scope,
		CaseID:        options.caseID,
		NodeID:        options.nodeID,
		PathID:        options.pathID,
		WorkflowID:    options.workflowID,
		PlannerMode:   mapplanner.ModeRun,
	})
	if err != nil {
		return store.TestMapPlanRecord{}, err
	}
	now := time.Now().UTC()
	plan.ID = "runplan." + safeReportID(plan.MapID) + "." + now.Format("20060102T150405.000000000Z")
	plan.Mode = mapplanner.ModeRun
	plan.Status = mapplanner.TaskStatusRunning
	plan.CreatedAt = now
	record, err := mapplanner.RecordFromPlan(plan, now)
	if err != nil {
		return store.TestMapPlanRecord{}, err
	}
	record.Instance.Status = mapplanner.TaskStatusRunning
	record.Instance.StartedAt = now
	record.Instance.FinishedAt = time.Time{}
	for i := range record.Tasks {
		if record.Tasks[i].Status == mapplanner.TaskStatusSkipped {
			continue
		}
		record.Tasks[i].Status = mapplanner.TaskStatusPlanned
	}
	return record, nil
}

func prepareExistingMapRunRecord(record store.TestMapPlanRecord, options mapRunOptions) store.TestMapPlanRecord {
	now := time.Now().UTC()
	record.Instance.Mode = mapplanner.ModeRun
	record.Instance.Status = mapplanner.TaskStatusRunning
	if strings.TrimSpace(options.environmentID) != "" {
		record.Instance.EnvironmentID = options.environmentID
	}
	if record.Instance.StartedAt.IsZero() {
		record.Instance.StartedAt = now
	}
	record.Instance.FinishedAt = time.Time{}
	for i := range record.Tasks {
		task := &record.Tasks[i]
		if task.Status == mapplanner.TaskStatusSkipped || task.Kind == mapplanner.TaskSkip {
			task.Status = mapplanner.TaskStatusSkipped
			continue
		}
		task.Status = mapplanner.TaskStatusPlanned
		task.WorkflowRunID = ""
		task.APICaseRunID = ""
		task.EvidenceRoot = ""
		task.StartedAt = time.Time{}
		task.FinishedAt = time.Time{}
	}
	return record
}

func runMapRunExplain(ctx context.Context, args []string) error {
	flags := flag.NewFlagSet("map run explain", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	storeRef := flags.String("store", "", "Named Store config or Store DSN")
	storeURL := flags.String("store-url", "", legacyStoreURLFlagHelp)
	planID := flags.String("plan", "", "Planner run instance id")
	jsonOutput := flags.Bool("json", false, "Emit a machine-readable JSON report")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if strings.TrimSpace(*planID) == "" {
		return errors.New("--plan is required")
	}
	runtime, cleanup, err := openRequiredCLIStore(ctx, *storeRef, *storeURL)
	if err != nil {
		return err
	}
	defer cleanup()
	record, err := runtime.GetTestMapPlan(ctx, *planID)
	if err != nil {
		return err
	}
	report := mapRunReportFromRecord(record)
	report.OK = true
	if *jsonOutput {
		return writeIndentedJSON(report)
	}
	printMapRunReport(report)
	return nil
}
