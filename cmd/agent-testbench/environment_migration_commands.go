package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"

	"agent-testbench/internal/store"
)

const environmentMigrationAssetKind = "mysql-migration-sql"
const environmentMigrationHistoryTable = "agent_testbench_schema_history"

type environmentMigrationEdge struct {
	Owner    string `json:"owner"`
	Provider string `json:"provider"`
}

type environmentMigrationPrecondition struct {
	Type   string `json:"type"`
	Table  string `json:"table,omitempty"`
	Column string `json:"column,omitempty"`
}

type environmentMigrationMetadata struct {
	Version       string                             `json:"version"`
	Description   string                             `json:"description,omitempty"`
	Database      string                             `json:"database"`
	Preconditions []environmentMigrationPrecondition `json:"preconditions,omitempty"`
	Checksum      string                             `json:"checksum,omitempty"`
}

type environmentMigrationSummary struct {
	Migration environmentMigrationMetadata `json:"migration"`
}

type environmentMigrationItem struct {
	EnvironmentID     string                             `json:"-"`
	AssetID           string                             `json:"assetId"`
	OwnerComponentID  string                             `json:"ownerComponentId"`
	ProviderComponent string                             `json:"providerComponentId"`
	TargetComponentID string                             `json:"targetComponentId"`
	TargetPath        string                             `json:"targetPath,omitempty"`
	AssetKind         string                             `json:"assetKind"`
	Version           string                             `json:"version"`
	Description       string                             `json:"description,omitempty"`
	Database          string                             `json:"database"`
	Checksum          string                             `json:"checksum"`
	Preconditions     []environmentMigrationPrecondition `json:"preconditions,omitempty"`
	ApplyOrder        int                                `json:"applyOrder,omitempty"`
	Bytes             int                                `json:"bytes,omitempty"`
	Content           string                             `json:"-"`
	Status            string                             `json:"status,omitempty"`
	Action            string                             `json:"action,omitempty"`
	Command           []string                           `json:"command,omitempty"`
	Attempts          int                                `json:"attempts,omitempty"`
	OK                bool                               `json:"ok,omitempty"`
	Error             string                             `json:"error,omitempty"`
}

type environmentMigrationReport struct {
	OK            bool                       `json:"ok"`
	EnvironmentID string                     `json:"environmentId"`
	StorePath     string                     `json:"storePath,omitempty"`
	Edge          environmentMigrationEdge   `json:"edge,omitempty"`
	Database      string                     `json:"database,omitempty"`
	Execute       bool                       `json:"execute,omitempty"`
	Workspace     string                     `json:"workspace,omitempty"`
	HistoryTable  string                     `json:"historyTable,omitempty"`
	Count         int                        `json:"count"`
	Migrations    []environmentMigrationItem `json:"migrations"`
}

type environmentMigrationAddOptions struct {
	EnvID      string
	StoreRef   string
	StoreURL   string
	Edge       environmentMigrationEdge
	Metadata   environmentMigrationMetadata
	Content    string
	AssetID    string
	TargetPath string
	ApplyOrder int
	Force      bool
	JSONOutput bool
}

type environmentMigrationTargetOptions struct {
	EnvID          string
	StoreRef       string
	StoreURL       string
	Edge           environmentMigrationEdge
	Database       string
	Workspace      string
	ThroughVersion string
	Execute        bool
	JSONOutput     bool
}

func runEnvironmentMigration(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return errors.New("missing environment migration command")
	}
	switch args[0] {
	case "add":
		return runEnvironmentMigrationAdd(ctx, args[1:])
	case "list":
		return runEnvironmentMigrationList(ctx, args[1:])
	case "plan":
		return runEnvironmentMigrationPlan(ctx, args[1:])
	case "apply":
		return runEnvironmentMigrationApply(ctx, args[1:])
	case "baseline":
		return runEnvironmentMigrationBaseline(ctx, args[1:])
	default:
		return fmt.Errorf("unknown environment migration command: %s", args[0])
	}
}

func runEnvironmentMigrationAdd(ctx context.Context, args []string) error {
	opts, err := parseEnvironmentMigrationAddOptions(args)
	if err != nil {
		return err
	}
	report, err := addEnvironmentMigrationToStore(ctx, opts)
	if err != nil {
		return err
	}
	if opts.JSONOutput {
		return writeIndentedJSON(report)
	}
	printEnvironmentMigrationReport("Environment Migration Added", report)
	return nil
}

func parseEnvironmentMigrationAddOptions(args []string) (environmentMigrationAddOptions, error) {
	flags := flag.NewFlagSet("environment migration add", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	storeRef := flags.String("store", "", "Named Store config or Store DSN")
	storeURL := flags.String("store-url", "", legacyStoreURLFlagHelp)
	edgeRaw := flags.String("edge", "", "Migration edge as OWNER:PROVIDER")
	database := flags.String("database", "", "Target MySQL database name")
	version := flags.String("version", "", "Migration version")
	description := flags.String("description", "", "Migration description")
	file := flags.String("file", "", "SQL migration file")
	assetID := flags.String("asset-id", "", "Stable asset id; defaults from edge and version")
	targetPath := flags.String("target-path", "", "Optional review path for the SQL asset")
	applyOrder := flags.Int("apply-order", 0, "Relative apply order; defaults from numeric version")
	force := flags.Bool("force", false, "Replace an existing migration asset with the same owner and id")
	jsonOutput := flags.Bool("json", false, "Emit a machine-readable JSON report")
	var preconditions stringListFlag
	flags.Var(&preconditions, "precondition", "Migration precondition such as column-not-exists:TABLE.COLUMN; repeatable")
	if err := parseInterspersedFlags(flags, args); err != nil {
		return environmentMigrationAddOptions{}, err
	}
	envID := strings.TrimSpace(flags.Arg(0))
	if envID == "" {
		return environmentMigrationAddOptions{}, errors.New("environment id is required")
	}
	edge, err := parseEnvironmentMigrationEdge(*edgeRaw)
	if err != nil {
		return environmentMigrationAddOptions{}, err
	}
	if strings.TrimSpace(*database) == "" || strings.TrimSpace(*version) == "" || strings.TrimSpace(*file) == "" {
		return environmentMigrationAddOptions{}, errors.New("--database, --version, and --file are required")
	}
	raw, err := os.ReadFile(strings.TrimSpace(*file))
	if err != nil {
		return environmentMigrationAddOptions{}, err
	}
	content := string(raw)
	checksum := sha256Hex(content)
	metadata := environmentMigrationMetadata{
		Version:       strings.TrimSpace(*version),
		Description:   strings.TrimSpace(*description),
		Database:      strings.TrimSpace(*database),
		Preconditions: parseEnvironmentMigrationPreconditions(preconditions),
		Checksum:      checksum,
	}
	id := strings.TrimSpace(*assetID)
	if id == "" {
		id = defaultEnvironmentMigrationAssetID(edge, metadata)
	}
	order := *applyOrder
	if order == 0 {
		order = environmentMigrationDefaultApplyOrder(metadata.Version)
	}
	path := strings.TrimSpace(*targetPath)
	if path == "" {
		path = "migrations/" + id + ".sql"
	}

	return environmentMigrationAddOptions{
		EnvID:      envID,
		StoreRef:   *storeRef,
		StoreURL:   *storeURL,
		Edge:       edge,
		Metadata:   metadata,
		Content:    content,
		AssetID:    id,
		TargetPath: path,
		ApplyOrder: order,
		Force:      *force,
		JSONOutput: *jsonOutput,
	}, nil
}

func addEnvironmentMigrationToStore(ctx context.Context, opts environmentMigrationAddOptions) (environmentMigrationReport, error) {
	runtime, cleanup, resolvedStoreURL, err := openEnvironmentMigrationStore(ctx, opts.StoreRef, opts.StoreURL)
	if err != nil {
		return environmentMigrationReport{}, err
	}
	defer cleanup()
	if _, err := runtime.GetEnvironment(ctx, opts.EnvID); err != nil {
		return environmentMigrationReport{}, err
	}
	graph, err := runtime.GetEnvironmentComponentGraph(ctx, opts.EnvID)
	if err != nil {
		return environmentMigrationReport{}, err
	}
	updated, item, err := addEnvironmentMigrationAsset(graph, opts.Edge, store.ComponentConfigAsset{
		OwnerComponentID:  opts.Edge.Owner,
		AssetID:           opts.AssetID,
		AssetKind:         environmentMigrationAssetKind,
		TargetComponentID: opts.Edge.Provider,
		TargetPath:        opts.TargetPath,
		ContentInline:     opts.Content,
		SHA256:            opts.Metadata.Checksum,
		SizeBytes:         int64(len(opts.Content)),
		ApplyOrder:        opts.ApplyOrder,
		SummaryJSON:       mustCompactJSON(environmentMigrationSummary{Migration: opts.Metadata}),
	}, opts.Force)
	if err != nil {
		return environmentMigrationReport{}, err
	}
	if err := runtime.ReplaceEnvironmentComponentGraph(ctx, opts.EnvID, updated); err != nil {
		return environmentMigrationReport{}, err
	}
	return environmentMigrationReport{
		OK:            true,
		EnvironmentID: opts.EnvID,
		StorePath:     maskStoreURL(resolvedStoreURL),
		Edge:          opts.Edge,
		Database:      opts.Metadata.Database,
		Count:         1,
		Migrations:    []environmentMigrationItem{item},
	}, nil
}

func runEnvironmentMigrationList(ctx context.Context, args []string) error {
	report, jsonOutput, err := environmentMigrationReadOnlyReport(ctx, "environment migration list", args, "registered")
	if err != nil {
		return err
	}
	if jsonOutput {
		return writeIndentedJSON(report)
	}
	printEnvironmentMigrationReport("Environment Migrations", report)
	return nil
}

func runEnvironmentMigrationPlan(ctx context.Context, args []string) error {
	report, jsonOutput, err := environmentMigrationReadOnlyReport(ctx, "environment migration plan", args, "pending")
	if err != nil {
		return err
	}
	for index := range report.Migrations {
		report.Migrations[index].Action = "plan-apply-mysql-migration"
	}
	report.HistoryTable = environmentMigrationHistoryTable
	if jsonOutput {
		return writeIndentedJSON(report)
	}
	printEnvironmentMigrationReport("Environment Migration Plan", report)
	return nil
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
	report, command, err := prepareEnvironmentMigrationTarget(ctx, opts)
	if err != nil {
		return err
	}
	planEnvironmentMigrationTarget(opts, baseline, command, &report)
	if opts.Execute {
		executeEnvironmentMigrationTarget(ctx, opts, baseline, command, &report)
	}
	if opts.JSONOutput {
		return writeIndentedJSON(report)
	}
	if baseline {
		printEnvironmentMigrationReport("Environment Migration Baseline", report)
	} else {
		printEnvironmentMigrationReport("Environment Migration Apply", report)
	}
	if !report.OK {
		return errors.New("one or more environment migrations failed")
	}
	return nil
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
	jsonOutput := flags.Bool("json", false, "Emit a machine-readable JSON report")
	if err := parseInterspersedFlags(flags, args); err != nil {
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
		JSONOutput:     *jsonOutput,
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
			item.Action = "plan-baseline-mysql-migration"
		} else {
			item.Action = "plan-apply-mysql-migration"
		}
		item.Status = "pending"
		item.OK = true
	}
}

func executeEnvironmentMigrationTarget(ctx context.Context, opts environmentMigrationTargetOptions, baseline bool, command []string, report *environmentMigrationReport) {
	for index := range report.Migrations {
		item := &report.Migrations[index]
		if baseline {
			item.Action = "baseline-mysql-migration"
		} else {
			item.Action = "apply-mysql-migration"
		}
		item.Status = "pending"
		item.OK = true
		var input string
		if baseline {
			input = environmentMigrationBaselineSQL(opts.Edge, *item)
		} else {
			input = environmentMigrationApplySQL(opts.Edge, *item)
		}
		attempts, status, errText := runEnvironmentMigrationWithHistory(ctx, opts.Workspace, command, opts.Edge, *item, input, baseline)
		item.Attempts = attempts
		if errText != "" {
			item.OK = false
			item.Error = errText
			report.OK = false
		} else {
			item.Status = status
		}
	}
}

func environmentMigrationReadOnlyReport(ctx context.Context, command string, args []string, status string) (environmentMigrationReport, bool, error) {
	flags := flag.NewFlagSet(command, flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	storeRef := flags.String("store", "", "Named Store config or Store DSN")
	storeURL := flags.String("store-url", "", legacyStoreURLFlagHelp)
	edgeRaw := flags.String("edge", "", "Filter by migration edge OWNER:PROVIDER")
	database := flags.String("database", "", "Filter by target database")
	jsonOutput := flags.Bool("json", false, "Emit a machine-readable JSON report")
	if err := parseInterspersedFlags(flags, args); err != nil {
		return environmentMigrationReport{}, false, err
	}
	envID := strings.TrimSpace(flags.Arg(0))
	if envID == "" {
		return environmentMigrationReport{}, false, errors.New("environment id is required")
	}
	edge, err := parseOptionalEnvironmentMigrationEdge(*edgeRaw)
	if err != nil {
		return environmentMigrationReport{}, false, err
	}
	runtime, cleanup, resolvedStoreURL, err := openEnvironmentMigrationStore(ctx, *storeRef, *storeURL)
	if err != nil {
		return environmentMigrationReport{}, false, err
	}
	defer cleanup()
	if _, err := runtime.GetEnvironment(ctx, envID); err != nil {
		return environmentMigrationReport{}, false, err
	}
	graph, err := runtime.GetEnvironmentComponentGraph(ctx, envID)
	if err != nil {
		return environmentMigrationReport{}, false, err
	}
	items := environmentMigrationItems(graph, edge, strings.TrimSpace(*database), "")
	for index := range items {
		items[index].Status = status
	}
	report := environmentMigrationReport{
		OK:            true,
		EnvironmentID: envID,
		StorePath:     maskStoreURL(resolvedStoreURL),
		Edge:          edge,
		Database:      strings.TrimSpace(*database),
		Count:         len(items),
		Migrations:    items,
	}
	return report, *jsonOutput, nil
}

func openEnvironmentMigrationStore(ctx context.Context, storeRef string, storeURL string) (store.Store, func(), string, error) {
	resolvedStoreURL, err := resolveRequiredDailyStoreReference(storeRef, storeURL)
	if err != nil {
		return nil, nil, "", err
	}
	runtime, err := openStore(ctx, resolvedStoreURL)
	if err != nil {
		return nil, nil, "", err
	}
	return runtime, func() { closeCLIStore(runtime) }, resolvedStoreURL, nil
}

func printEnvironmentMigrationReport(title string, report environmentMigrationReport) {
	fmt.Println(title)
	fmt.Printf("OK: %t\n", report.OK)
	fmt.Printf("Environment: %s\n", report.EnvironmentID)
	if report.Edge.Owner != "" {
		fmt.Printf("Edge: %s:%s\n", report.Edge.Owner, report.Edge.Provider)
	}
	if report.Database != "" {
		fmt.Printf("Database: %s\n", report.Database)
	}
	if report.Execute {
		fmt.Println("Mode: execute")
	}
	fmt.Printf("Count: %d\n", report.Count)
	for _, item := range report.Migrations {
		state := firstNonEmpty(item.Status, item.Action)
		if state == "" {
			state = "registered"
		}
		fmt.Printf("- %s %s [%s]\n", item.Version, item.AssetID, state)
		if item.Description != "" {
			fmt.Printf("  description: %s\n", item.Description)
		}
		if item.Error != "" {
			fmt.Printf("  error: %s\n", item.Error)
		}
	}
}
