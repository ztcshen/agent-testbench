package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

var environmentMigrationProgressInterval = 5 * time.Second

type environmentMigrationTargetOptions struct {
	EnvID          string
	StoreRef       string
	StoreURL       string
	Edge           environmentMigrationEdge
	Database       string
	Workspace      string
	ThroughVersion string
	Execute        bool
	OutputFormat   string
	JSONOutput     bool
}

func runEnvironmentMigrationApply(ctx context.Context, args []string) error {
	return runEnvironmentMigrationTargetCommand(ctx, args, "environment migration apply", false)
}

func runEnvironmentMigrationBaseline(ctx context.Context, args []string) error {
	return runEnvironmentMigrationTargetCommand(ctx, args, "environment migration baseline", true)
}

func runEnvironmentMigrationTargetCommand(ctx context.Context, args []string, commandName string, baseline bool) error {
	opts, err := parseEnvironmentMigrationTargetOptions(args, commandName)
	if err != nil {
		return err
	}
	if opts.OutputFormat == cliOutputFormatStreamJSON {
		ctx = contextWithAgentEventStream(ctx, os.Stdout)
	}
	agentEmitRunStarted(ctx, newEnvironmentMigrationRunID(baseline), environmentMigrationRunPhase(baseline), opts.EnvID, environmentMigrationRunMessage(baseline, "started"))
	report, command, err := prepareEnvironmentMigrationTarget(ctx, opts)
	if err != nil {
		emitEnvironmentMigrationStreamFailure(ctx, opts, baseline, err)
		return err
	}
	planEnvironmentMigrationTarget(opts, baseline, command, &report)
	if opts.Execute {
		executeEnvironmentMigrationTarget(ctx, opts, baseline, command, &report)
		if err := persistEnvironmentMigrationTargetStatuses(ctx, opts, &report); err != nil {
			report.OK = false
			if len(report.Migrations) > 0 {
				report.Migrations[0].OK = false
				report.Migrations[0].Error = err.Error()
			}
		}
	}
	if opts.OutputFormat == cliOutputFormatStreamJSON {
		agentEmitRunCompleted(ctx, environmentMigrationRunPhase(baseline), statusText(report.OK), opts.EnvID, environmentMigrationRunMessage(baseline, "completed"), environmentMigrationReportError(report), report)
	} else if opts.JSONOutput {
		if err := writeIndentedJSON(report); err != nil {
			return err
		}
	} else {
		if baseline {
			printEnvironmentMigrationReport("Environment Migration Baseline", report)
		} else {
			printEnvironmentMigrationReport("Environment Migration Apply", report)
		}
	}
	if !report.OK {
		return errors.New("one or more environment migrations failed")
	}
	return nil
}

func emitEnvironmentMigrationStreamFailure(ctx context.Context, opts environmentMigrationTargetOptions, baseline bool, err error) {
	if !agentHasEventStream(ctx) {
		return
	}
	report := environmentMigrationReport{
		OK:            false,
		EnvironmentID: opts.EnvID,
		Edge:          opts.Edge,
		Database:      opts.Database,
		Execute:       opts.Execute,
		Workspace:     opts.Workspace,
		HistoryTable:  environmentMigrationHistoryTable,
	}
	agentEmitRunCompleted(ctx, environmentMigrationRunPhase(baseline), "failed", opts.EnvID, environmentMigrationRunMessage(baseline, "failed"), err.Error(), report)
}

func parseEnvironmentMigrationTargetOptions(args []string, commandName string) (environmentMigrationTargetOptions, error) {
	flags := flag.NewFlagSet(commandName, flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	storeRef := flags.String("store", "", "Named Store config or Store DSN")
	storeURL := flags.String("store-url", "", legacyStoreURLFlagHelp)
	edgeRaw := flags.String("edge", "", "Migration edge as OWNER:PROVIDER")
	database := flags.String("database", "", "Target MySQL database name")
	workspace := flags.String("workspace", "", "Restore workspace containing generated Compose files")
	throughVersion := flags.String("through-version", "", "Only apply or baseline migrations up to this version")
	execute := flags.Bool("execute", false, "Execute against the target MySQL container")
	outputFormat := flags.String("output-format", "", "Output format: text, json, or stream-json")
	jsonOutput := flags.Bool("json", false, "Emit a machine-readable JSON report")
	if err := parseInterspersedFlags(flags, args); err != nil {
		return environmentMigrationTargetOptions{}, err
	}
	resolvedOutputFormat, err := resolveCLIOutputFormat(*outputFormat, *jsonOutput)
	if err != nil {
		return environmentMigrationTargetOptions{}, err
	}
	envID := strings.TrimSpace(flags.Arg(0))
	if envID == "" {
		return environmentMigrationTargetOptions{}, errors.New("environment id is required")
	}
	if strings.TrimSpace(*workspace) == "" {
		return environmentMigrationTargetOptions{}, errors.New("--workspace is required")
	}
	edge, err := parseEnvironmentMigrationEdge(*edgeRaw)
	if err != nil {
		return environmentMigrationTargetOptions{}, err
	}
	if strings.TrimSpace(*database) == "" {
		return environmentMigrationTargetOptions{}, errors.New("--database is required")
	}

	return environmentMigrationTargetOptions{
		EnvID:          envID,
		StoreRef:       *storeRef,
		StoreURL:       *storeURL,
		Edge:           edge,
		Database:       strings.TrimSpace(*database),
		Workspace:      strings.TrimSpace(*workspace),
		ThroughVersion: strings.TrimSpace(*throughVersion),
		Execute:        *execute,
		OutputFormat:   resolvedOutputFormat,
		JSONOutput:     resolvedOutputFormat == cliOutputFormatJSON,
	}, nil
}

func prepareEnvironmentMigrationTarget(ctx context.Context, opts environmentMigrationTargetOptions) (environmentMigrationReport, []string, error) {
	runtime, cleanup, resolvedStoreURL, err := openEnvironmentMigrationStore(ctx, opts.StoreRef, opts.StoreURL)
	if err != nil {
		return environmentMigrationReport{}, nil, err
	}
	defer cleanup()
	env, err := runtime.GetEnvironment(ctx, opts.EnvID)
	if err != nil {
		return environmentMigrationReport{}, nil, err
	}
	graph, err := runtime.GetEnvironmentComponentGraph(ctx, opts.EnvID)
	if err != nil {
		return environmentMigrationReport{}, nil, err
	}
	compose := jsonObjectString(env.ComposeJSON)
	if opts.Execute {
		for _, generated := range prepareEnvironmentRestoreGeneratedFiles(compose, opts.Workspace, true) {
			if !generated.OK {
				return environmentMigrationReport{}, nil, fmt.Errorf("prepare generated file %s: %s", generated.Path, generated.Error)
			}
		}
		if _, err := writeEnvironmentRestoreGeneratedEnvFile(opts.Workspace, compose); err != nil {
			return environmentMigrationReport{}, nil, err
		}
	}
	composeFiles := environmentRestoreResolvedComposeFiles(opts.Workspace, environmentRestoreComposeFiles(compose))
	composeBaseArgs := environmentRestoreComposeBaseArgs(compose, opts.Workspace, composeFiles)
	items := environmentMigrationItems(graph, opts.Edge, opts.Database, opts.ThroughVersion)
	report := environmentMigrationReport{
		OK:            true,
		EnvironmentID: opts.EnvID,
		StorePath:     maskStoreURL(resolvedStoreURL),
		Edge:          opts.Edge,
		Database:      opts.Database,
		Execute:       opts.Execute,
		Workspace:     opts.Workspace,
		HistoryTable:  environmentMigrationHistoryTable,
		Count:         len(items),
		Migrations:    items,
	}
	targetService := environmentMigrationTargetService(graph, opts.Edge.Provider)
	command := environmentRestoreMySQLApplyCommand(composeBaseArgs, targetService)
	if len(composeBaseArgs) == 0 || targetService == "" {
		return environmentMigrationReport{}, nil, errors.New("migration apply requires a Docker Compose file and target component compose service")
	}
	return report, command, nil
}

func planEnvironmentMigrationTarget(opts environmentMigrationTargetOptions, baseline bool, command []string, report *environmentMigrationReport) {
	for index := range report.Migrations {
		item := &report.Migrations[index]
		item.EnvironmentID = opts.EnvID
		item.Command = command
		if baseline {
			item.Action = environmentMigrationActionPlanBaselineMySQL
		} else {
			item.Action = environmentMigrationActionPlanApplyMySQL
		}
		item.Status = "pending"
		item.OK = true
	}
}

func executeEnvironmentMigrationTarget(ctx context.Context, opts environmentMigrationTargetOptions, baseline bool, command []string, report *environmentMigrationReport) {
	for index := range report.Migrations {
		item := &report.Migrations[index]
		agentEmitStep(ctx, "step_started", "environment.migration", "running", item.AssetID, environmentMigrationItemMessage(baseline, "started", *item), "")
		if baseline {
			item.Action = environmentMigrationActionBaselineMySQL
		} else {
			item.Action = environmentMigrationActionApplyMySQL
		}
		item.Status = "pending"
		item.OK = true
		var input string
		if baseline {
			input = environmentMigrationBaselineSQL(opts.Edge, *item)
		} else {
			input = environmentMigrationApplySQL(opts.Edge, *item)
		}
		attempts, status, errText := runEnvironmentMigrationWithProgress(ctx, opts, baseline, command, *item, input)
		item.Attempts = attempts
		if errText != "" {
			item.OK = false
			item.Error = errText
			report.OK = false
		} else {
			item.Status = status
		}
		agentEmitStep(ctx, "step_completed", "environment.migration", environmentMigrationItemStatus(*item), item.AssetID, environmentMigrationItemMessage(baseline, "completed", *item), item.Error)
		if !item.OK {
			break
		}
	}
}

func runEnvironmentMigrationWithProgress(ctx context.Context, opts environmentMigrationTargetOptions, baseline bool, command []string, item environmentMigrationItem, input string) (int, string, string) {
	type migrationResult struct {
		attempts int
		status   string
		errText  string
	}
	if !agentHasEventStream(ctx) {
		return runEnvironmentMigrationWithHistory(ctx, opts.Workspace, command, opts.Edge, item, input, baseline)
	}
	resultCh := make(chan migrationResult, 1)
	started := time.Now()
	go func() {
		attempts, status, errText := runEnvironmentMigrationWithHistory(ctx, opts.Workspace, command, opts.Edge, item, input, baseline)
		resultCh <- migrationResult{attempts: attempts, status: status, errText: errText}
	}()
	ticker := time.NewTicker(environmentMigrationProgressIntervalValue())
	defer ticker.Stop()
	for {
		select {
		case result := <-resultCh:
			return result.attempts, result.status, result.errText
		case <-ticker.C:
			agentEmitEvent(ctx, agentStreamEvent{
				Type:      "tool_observation",
				Phase:     "environment.migration",
				Status:    "waiting",
				Target:    item.AssetID,
				Message:   environmentMigrationItemMessage(baseline, "still running", item),
				ElapsedMs: time.Since(started).Milliseconds(),
			})
		}
	}
}

func environmentMigrationProgressIntervalValue() time.Duration {
	if raw := strings.TrimSpace(os.Getenv("AGENT_TESTBENCH_MIGRATION_PROGRESS_INTERVAL_MS")); raw != "" {
		if millis, err := strconv.Atoi(raw); err == nil && millis > 0 {
			return time.Duration(millis) * time.Millisecond
		}
	}
	return environmentMigrationProgressInterval
}

func persistEnvironmentMigrationTargetStatuses(ctx context.Context, opts environmentMigrationTargetOptions, report *environmentMigrationReport) error {
	statusByAsset := map[string]string{}
	for _, item := range report.Migrations {
		if !item.OK || !environmentMigrationStatusComplete(item.Status) {
			continue
		}
		statusByAsset[item.OwnerComponentID+"\x00"+item.AssetID] = item.Status
	}
	if len(statusByAsset) == 0 {
		return nil
	}
	runtime, cleanup, _, err := openEnvironmentMigrationStore(ctx, opts.StoreRef, opts.StoreURL)
	if err != nil {
		return err
	}
	defer cleanup()
	graph, err := runtime.GetEnvironmentComponentGraph(ctx, opts.EnvID)
	if err != nil {
		return err
	}
	changed := false
	for index := range graph.Assets {
		asset := &graph.Assets[index]
		status, ok := statusByAsset[asset.OwnerComponentID+"\x00"+asset.AssetID]
		if !ok {
			continue
		}
		metadata := environmentMigrationAssetMetadata(*asset)
		if metadata.Version == "" || metadata.Database == "" {
			continue
		}
		metadata.Status = status
		asset.SummaryJSON = mustCompactJSON(environmentMigrationSummary{Migration: metadata})
		changed = true
	}
	if !changed {
		return nil
	}
	return runtime.ReplaceEnvironmentComponentGraph(ctx, opts.EnvID, graph)
}
