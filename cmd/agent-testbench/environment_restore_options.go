package main

import (
	"errors"
	"flag"
	"os"
	"strings"
	"time"
)

type environmentRestoreDockerCleanupOptions struct {
	Requested             bool
	IncludeImages         bool
	Allowed               bool
	UseExistingContainers bool
	AssumeCleanDocker     bool
}

type environmentRestoreCommandOptions struct {
	EnvironmentID    string
	StoreRef         string
	StoreURL         string
	Workspace        string
	Execute          bool
	Pull             bool
	PrepareReposOnly bool
	HealthTimeout    time.Duration
	Workflow         environmentRestoreWorkflowOptions
	Cleanup          environmentRestoreDockerCleanupOptions
	OutputFormat     string
	JSONOutput       bool
}

func parseEnvironmentRestoreCommandOptions(args []string) (environmentRestoreCommandOptions, error) {
	flags := flag.NewFlagSet("environment restore", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	storeRef := flags.String("store", "", "Named Store config or Store DSN")
	storeURL := flags.String("store-url", "", legacyStoreURLFlagHelp)
	workspace := flags.String("workspace", "", "Local workspace for cloned or existing service checkouts")
	execute := flags.Bool("execute", false, "Clone or update component repositories, run Docker Compose, and wait for health checks")
	pull := flags.Bool("pull", false, "Run git pull --ff-only for existing checkouts when --execute is set")
	prepareReposOnly := flags.Bool("prepare-repos-only", false, "When --execute is set, clone or validate repositories and stop before Docker startup")
	runWorkflow := flags.Bool("run-workflow", false, "Run the environment verification workflow after Docker health checks pass")
	serverURL := flags.String("server-url", "", "Running control plane base URL for async environment acceptance")
	baseURL := flags.String("base-url", "", "Base URL for verification workflow execution")
	workflowOutputDir := flags.String("workflow-output-dir", "", "Verification workflow report output directory")
	acceptanceTimeoutSeconds := flags.Int("acceptance-timeout-seconds", 120, "Seconds to wait for async environment acceptance report")
	healthTimeoutSeconds := flags.Int("health-timeout-seconds", 60, "Seconds to wait for recorded Docker service health checks")
	useExistingContainers := flags.Bool("use-existing-containers", false, "Adopt already-running fixed-name Docker containers instead of running Docker Compose up")
	assumeCleanDocker := flags.Bool("assume-clean-docker", false, "Dry-run as a colleague/new machine with no existing target Docker containers")
	cleanDockerState := flags.Bool("clean-docker-state", false, "Plan or run Docker Compose cleanup before startup")
	cleanDockerImages := flags.Bool("clean-docker-images", false, "Include Docker Compose image removal in cleanup plan")
	allowDestructiveDockerCleanup := flags.Bool("allow-destructive-docker-cleanup", false, "Allow --execute to run requested Docker cleanup commands")
	outputFormat := flags.String("output-format", "", "Output format: text, json, or stream-json")
	jsonOutput := flags.Bool("json", false, "Emit a machine-readable JSON report")
	if err := parseInterspersedFlags(flags, args); err != nil {
		return environmentRestoreCommandOptions{}, err
	}
	id := strings.TrimSpace(flags.Arg(0))
	if id == "" {
		return environmentRestoreCommandOptions{}, errors.New("environment id is required")
	}
	if err := validateEnvironmentRestoreCommandFlags(environmentRestoreCommandFlagValues{
		workspace:                *workspace,
		execute:                  *execute,
		prepareReposOnly:         *prepareReposOnly,
		runWorkflow:              *runWorkflow,
		serverURL:                *serverURL,
		healthTimeoutSeconds:     *healthTimeoutSeconds,
		acceptanceTimeoutSeconds: *acceptanceTimeoutSeconds,
		useExistingContainers:    *useExistingContainers,
		assumeCleanDocker:        *assumeCleanDocker,
		cleanDockerState:         *cleanDockerState,
		cleanDockerImages:        *cleanDockerImages,
	}); err != nil {
		return environmentRestoreCommandOptions{}, err
	}
	resolvedStoreURL, err := resolveRequiredDailyStoreReference(*storeRef, *storeURL)
	if err != nil {
		return environmentRestoreCommandOptions{}, err
	}
	resolvedOutputFormat, err := resolveCLIOutputFormat(*outputFormat, *jsonOutput)
	if err != nil {
		return environmentRestoreCommandOptions{}, err
	}
	return environmentRestoreCommandOptions{
		EnvironmentID:    id,
		StoreRef:         *storeRef,
		StoreURL:         resolvedStoreURL,
		Workspace:        *workspace,
		Execute:          *execute,
		Pull:             *pull,
		PrepareReposOnly: *prepareReposOnly,
		HealthTimeout:    time.Duration(*healthTimeoutSeconds) * time.Second,
		Workflow: environmentRestoreWorkflowOptions{
			Run:            *runWorkflow,
			StoreRef:       *storeRef,
			StoreURL:       resolvedStoreURL,
			ServerURL:      *serverURL,
			BaseURL:        *baseURL,
			OutputDir:      *workflowOutputDir,
			TimeoutSeconds: *acceptanceTimeoutSeconds,
		},
		Cleanup: environmentRestoreDockerCleanupOptions{
			Requested:             *cleanDockerState || *cleanDockerImages,
			IncludeImages:         *cleanDockerImages,
			Allowed:               *allowDestructiveDockerCleanup,
			UseExistingContainers: *useExistingContainers,
			AssumeCleanDocker:     *assumeCleanDocker,
		},
		OutputFormat: resolvedOutputFormat,
		JSONOutput:   resolvedOutputFormat == cliOutputFormatJSON,
	}, nil
}

func resolveCLIOutputFormat(outputFormat string, jsonOutput bool) (string, error) {
	outputFormat = strings.TrimSpace(outputFormat)
	if outputFormat == "" {
		outputFormat = cliOutputFormatText
	}
	if jsonOutput {
		if outputFormat != cliOutputFormatText && outputFormat != cliOutputFormatJSON {
			return "", errors.New("--json cannot be combined with --output-format " + outputFormat)
		}
		outputFormat = cliOutputFormatJSON
	}
	switch outputFormat {
	case cliOutputFormatText, cliOutputFormatJSON, cliOutputFormatStreamJSON:
		return outputFormat, nil
	default:
		return "", errors.New("--output-format must be text, json, or stream-json")
	}
}

type environmentRestoreCommandFlagValues struct {
	workspace                string
	execute                  bool
	prepareReposOnly         bool
	runWorkflow              bool
	serverURL                string
	healthTimeoutSeconds     int
	acceptanceTimeoutSeconds int
	useExistingContainers    bool
	assumeCleanDocker        bool
	cleanDockerState         bool
	cleanDockerImages        bool
}

func validateEnvironmentRestoreCommandFlags(values environmentRestoreCommandFlagValues) error {
	checks := []struct {
		invalid bool
		message string
	}{
		{strings.TrimSpace(values.workspace) == "", "--workspace is required"},
		{values.healthTimeoutSeconds <= 0, "--health-timeout-seconds must be positive"},
		{values.runWorkflow && !values.execute, "--run-workflow requires --execute"},
		{values.prepareReposOnly && !values.execute, "--prepare-repos-only requires --execute"},
		{values.prepareReposOnly && values.runWorkflow, "--prepare-repos-only cannot be combined with --run-workflow"},
		{values.useExistingContainers && (values.cleanDockerState || values.cleanDockerImages), "--use-existing-containers cannot be combined with Docker cleanup flags"},
		{values.assumeCleanDocker && values.execute, "--assume-clean-docker is a dry-run planning mode and cannot be combined with --execute"},
		{values.assumeCleanDocker && (values.useExistingContainers || values.cleanDockerState || values.cleanDockerImages), "--assume-clean-docker cannot be combined with Docker adoption or cleanup flags"},
		{values.runWorkflow && strings.TrimSpace(values.serverURL) == "", "--run-workflow requires --server-url for async environment acceptance"},
		{values.acceptanceTimeoutSeconds <= 0, "--acceptance-timeout-seconds must be positive"},
	}
	for _, check := range checks {
		if check.invalid {
			return errors.New(check.message)
		}
	}
	return nil
}
