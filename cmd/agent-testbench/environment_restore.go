package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"agent-testbench/internal/server/controlplane"
	"agent-testbench/internal/store"
)

const environmentRestoreAttemptLimit = 20

type environmentRestoreReport struct {
	OK                   bool                                         `json:"ok"`
	RestoreID            string                                       `json:"restoreId"`
	Executed             bool                                         `json:"executed"`
	EnvironmentID        string                                       `json:"environmentId"`
	VerificationWorkflow string                                       `json:"verificationWorkflow"`
	Workspace            string                                       `json:"workspace"`
	Environment          map[string]any                               `json:"environment,omitempty"`
	Error                string                                       `json:"error,omitempty"`
	Package              environmentRestorePackageReport              `json:"package,omitempty"`
	Repos                []environmentRestoreRepoReport               `json:"repos"`
	SourcePolicy         environmentRestoreSourcePolicy               `json:"sourcePolicy,omitempty"`
	ComponentGraph       environmentRestoreComponentGraph             `json:"componentGraph,omitempty"`
	ComponentStartupPlan controlplane.EnvironmentComponentStartupPlan `json:"componentStartupPlan,omitempty"`
	ComponentAssets      []environmentRestoreComponentAsset           `json:"componentAssets,omitempty"`
	Compose              map[string]any                               `json:"compose"`
	HealthChecks         []any                                        `json:"healthChecks"`
	Preflight            environmentRestorePreflight                  `json:"preflight"`
	Readiness            environmentRestoreReadiness                  `json:"readiness"`
	Docker               environmentRestoreDockerReport               `json:"docker"`
	Workflow             environmentRestoreWorkflowRun                `json:"workflow"`
	CleanMachine         environmentRestoreCleanMachinePlan           `json:"cleanMachine,omitempty"`
	NextActions          []string                                     `json:"nextActions"`
}

type environmentRestoreCleanMachinePlan struct {
	Ready          bool                                         `json:"ready"`
	Summary        environmentRestoreCleanMachineSummary        `json:"summary,omitempty"`
	PrepareCommand []string                                     `json:"prepareCommand,omitempty"`
	ExecuteCommand []string                                     `json:"executeCommand,omitempty"`
	Prerequisites  []environmentRestoreCleanMachinePrerequisite `json:"prerequisites,omitempty"`
	Notes          []string                                     `json:"notes,omitempty"`
}

type environmentRestoreCleanMachineSummary struct {
	EnvironmentID           string `json:"environmentId,omitempty"`
	VerificationWorkflow    string `json:"verificationWorkflow,omitempty"`
	Components              int    `json:"components"`
	StartupBatches          int    `json:"startupBatches"`
	HealthGates             int    `json:"healthGates"`
	ServiceRepositories     int    `json:"serviceRepositories"`
	StartupAssets           int    `json:"startupAssets"`
	RemoteComponentAssets   int    `json:"remoteComponentAssets"`
	InlineAssetBytes        int64  `json:"inlineAssetBytes"`
	RemoteAssetBytes        int64  `json:"remoteAssetBytes"`
	GraphMetadataLimitBytes int    `json:"graphMetadataLimitBytes"`
	InlineAssetLimitBytes   int    `json:"inlineAssetLimitBytes"`
	DockerImagesStored      bool   `json:"dockerImagesStored"`
	LargeBinariesStored     bool   `json:"largeBinariesStored"`
}

type environmentRestoreCleanMachinePrerequisite struct {
	Name     string `json:"name"`
	Required bool   `json:"required"`
	OK       bool   `json:"ok"`
	Detail   string `json:"detail,omitempty"`
}

type environmentRestoreSourcePolicy struct {
	RemoteOnly bool     `json:"remoteOnly"`
	OK         bool     `json:"ok"`
	Violations []string `json:"violations,omitempty"`
}

type environmentRestoreComponentGraph = controlplane.EnvironmentComponentGraphReadiness

type environmentRestoreComponentAsset struct {
	AssetID          string   `json:"assetId"`
	OwnerComponentID string   `json:"ownerComponentId,omitempty"`
	SourceURL        string   `json:"sourceUrl,omitempty"`
	SourcePath       string   `json:"sourcePath,omitempty"`
	Checkout         string   `json:"checkout,omitempty"`
	TargetPath       string   `json:"targetPath"`
	Bytes            int64    `json:"bytes,omitempty"`
	ApplyOrder       int      `json:"applyOrder,omitempty"`
	Action           string   `json:"action"`
	RepoAction       string   `json:"repoAction,omitempty"`
	Command          []string `json:"command,omitempty"`
	OK               bool     `json:"ok"`
	Error            string   `json:"error,omitempty"`
}

type environmentRestorePackageReport struct {
	Configured bool     `json:"configured"`
	URL        string   `json:"url,omitempty"`
	Branch     string   `json:"branch,omitempty"`
	Ref        string   `json:"ref,omitempty"`
	Checkout   string   `json:"checkout,omitempty"`
	Exists     bool     `json:"exists"`
	Action     string   `json:"action"`
	Command    []string `json:"command,omitempty"`
	OK         bool     `json:"ok"`
	Output     string   `json:"output,omitempty"`
	Error      string   `json:"error,omitempty"`
}

type environmentRestorePackageSpec struct {
	URL      string
	Branch   string
	Ref      string
	Checkout string
}

type environmentRestoreRepoReport struct {
	ServiceID string   `json:"serviceId"`
	URL       string   `json:"url,omitempty"`
	Branch    string   `json:"branch,omitempty"`
	Ref       string   `json:"ref,omitempty"`
	Checkout  string   `json:"checkout"`
	Exists    bool     `json:"exists"`
	Action    string   `json:"action"`
	Command   []string `json:"command,omitempty"`
	OK        bool     `json:"ok"`
	Output    string   `json:"output,omitempty"`
	Error     string   `json:"error,omitempty"`
}

type environmentRestoreRepoSpec struct {
	ServiceID string
	URL       string
	Branch    string
	Ref       string
	Checkout  string
}

type environmentRestorePreflight struct {
	OK                 bool                              `json:"ok"`
	AssumeCleanDocker  bool                              `json:"assumeCleanDocker,omitempty"`
	Tools              []environmentRestorePreflightTool `json:"tools"`
	HeavySteps         []string                          `json:"heavySteps,omitempty"`
	ContainerConflicts []string                          `json:"containerConflicts,omitempty"`
	StartupAssets      []environmentRestoreStartupAsset  `json:"startupAssets,omitempty"`
	Notes              []string                          `json:"notes,omitempty"`
}

type environmentRestoreStartupAsset struct {
	Path        string `json:"path"`
	Source      string `json:"source,omitempty"`
	ComposeFile string `json:"composeFile,omitempty"`
	Kind        string `json:"kind"`
	OK          bool   `json:"ok"`
	Error       string `json:"error,omitempty"`
}

type environmentRestoreStartupAssetCandidate struct {
	path        string
	source      string
	composeFile string
	kind        string
}

type environmentRestorePreflightTool struct {
	Name     string `json:"name"`
	Required bool   `json:"required"`
	OK       bool   `json:"ok"`
	Path     string `json:"path,omitempty"`
	Error    string `json:"error,omitempty"`
}

type environmentRestoreReadiness struct {
	OK                         bool                              `json:"ok"`
	Action                     string                            `json:"action"`
	PauseBeforeHeavyValidation bool                              `json:"pauseBeforeHeavyValidation"`
	NextStep                   string                            `json:"nextStep"`
	Items                      []environmentRestoreReadinessItem `json:"items"`
}

type environmentRestoreReadinessItem struct {
	Name     string `json:"name"`
	Required bool   `json:"required"`
	OK       bool   `json:"ok"`
	Detail   string `json:"detail,omitempty"`
}

type environmentRestoreDockerReport struct {
	OK            bool                                  `json:"ok"`
	Action        string                                `json:"action"`
	ComposeFile   string                                `json:"composeFile,omitempty"`
	Workdir       string                                `json:"workdir,omitempty"`
	Generated     []environmentRestoreGeneratedFile     `json:"generatedFiles,omitempty"`
	AppliedAssets []environmentRestoreAppliedAsset      `json:"appliedAssets,omitempty"`
	Cleanup       environmentRestoreDockerCleanupReport `json:"cleanup,omitempty"`
	Commands      [][]string                            `json:"commands,omitempty"`
	Output        []string                              `json:"output,omitempty"`
	Error         string                                `json:"error,omitempty"`
	HealthChecks  []environmentRestoreHealthCheckReport `json:"healthChecks,omitempty"`
}

type environmentRestoreGeneratedFile struct {
	Path   string `json:"path"`
	Bytes  int    `json:"bytes"`
	Action string `json:"action"`
	OK     bool   `json:"ok"`
	Error  string `json:"error,omitempty"`
}

type environmentRestoreAppliedAsset struct {
	AssetID              string   `json:"assetId"`
	OwnerComponentID     string   `json:"ownerComponentId,omitempty"`
	TargetComponentID    string   `json:"targetComponentId,omitempty"`
	TargetComposeService string   `json:"targetComposeService,omitempty"`
	DependencyConsumer   string   `json:"dependencyConsumer,omitempty"`
	DependencyProvider   string   `json:"dependencyProvider,omitempty"`
	TargetPath           string   `json:"targetPath,omitempty"`
	Bytes                int      `json:"bytes,omitempty"`
	ApplyOrder           int      `json:"applyOrder,omitempty"`
	Action               string   `json:"action"`
	Command              []string `json:"command,omitempty"`
	Attempts             int      `json:"attempts,omitempty"`
	OK                   bool     `json:"ok"`
	Error                string   `json:"error,omitempty"`
}

type environmentRestoreDockerCleanupReport struct {
	Requested      bool       `json:"requested,omitempty"`
	Allowed        bool       `json:"allowed,omitempty"`
	IncludeImages  bool       `json:"includeImages,omitempty"`
	Action         string     `json:"action,omitempty"`
	BackupCommands [][]string `json:"backupCommands,omitempty"`
	Commands       [][]string `json:"commands,omitempty"`
	Output         []string   `json:"output,omitempty"`
	Error          string     `json:"error,omitempty"`
	Warning        string     `json:"warning,omitempty"`
}

type environmentRestoreHealthCheckReport struct {
	ID         string `json:"id,omitempty"`
	Kind       string `json:"kind"`
	URL        string `json:"url"`
	Address    string `json:"address,omitempty"`
	Command    string `json:"command,omitempty"`
	Service    string `json:"service,omitempty"`
	Container  string `json:"container,omitempty"`
	OK         bool   `json:"ok"`
	StatusCode int    `json:"statusCode,omitempty"`
	State      string `json:"state,omitempty"`
	Health     string `json:"health,omitempty"`
	Output     string `json:"output,omitempty"`
	Error      string `json:"error,omitempty"`
}

type environmentRestoreWorkflowRun struct {
	OK         bool                                 `json:"ok"`
	Action     string                               `json:"action"`
	WorkflowID string                               `json:"workflowId"`
	RunID      string                               `json:"runId,omitempty"`
	OutputDir  string                               `json:"outputDir,omitempty"`
	ReportURL  string                               `json:"reportUrl,omitempty"`
	Counts     workflowCaseReportCounts             `json:"counts,omitempty"`
	Acceptance environmentRestoreWorkflowAcceptance `json:"acceptance,omitempty"`
	Error      string                               `json:"error,omitempty"`
}

type environmentRestoreWorkflowAcceptance struct {
	OK               bool   `json:"ok"`
	TemplateID       string `json:"templateId,omitempty"`
	WorkflowID       string `json:"workflowId,omitempty"`
	ExpectedSteps    int    `json:"expectedSteps,omitempty"`
	CompletedSteps   int    `json:"completedSteps,omitempty"`
	PassedSteps      int    `json:"passedSteps,omitempty"`
	FailedSteps      int    `json:"failedSteps,omitempty"`
	TopologyProvider string `json:"topologyProvider,omitempty"`
}

type environmentRestoreWorkflowOptions struct {
	Run            bool
	EnvironmentID  string
	StoreRef       string
	StoreURL       string
	ServerURL      string
	BaseURL        string
	OutputDir      string
	TimeoutSeconds int
}

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
	JSONOutput       bool
}

type environmentRestoreBuildPlan struct {
	WorkflowID           string
	Workspace            string
	Specs                []environmentRestoreRepoSpec
	Compose              map[string]any
	ComponentGraph       store.EnvironmentComponentGraph
	PackageSpec          environmentRestorePackageSpec
	HealthChecks         []any
	ComponentGraphReport environmentRestoreComponentGraph
	ComponentStartupPlan controlplane.EnvironmentComponentStartupPlan
	AttemptedAt          time.Time
	RemoteOnly           bool
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
		JSONOutput: *jsonOutput,
	}, nil
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

func runEnvironmentRestore(ctx context.Context, args []string) error {
	options, err := parseEnvironmentRestoreCommandOptions(args)
	if err != nil {
		return err
	}
	runtime, err := openStore(ctx, options.StoreURL)
	if err != nil {
		return err
	}
	defer closeCLIStore(runtime)
	env, err := runtime.GetEnvironment(ctx, options.EnvironmentID)
	if err != nil {
		return err
	}
	componentGraph, err := runtime.GetEnvironmentComponentGraph(ctx, env.ID)
	if err != nil {
		return err
	}
	options.Workflow.EnvironmentID = env.ID
	report, err := buildEnvironmentRestoreReport(ctx, env, options.Workspace, options.Execute, options.Pull, options.PrepareReposOnly, options.HealthTimeout, options.Workflow, options.Cleanup, componentGraph)
	if err != nil {
		return err
	}
	if options.JSONOutput {
		if encodeErr := writeIndentedJSON(report); encodeErr != nil {
			return encodeErr
		}
	} else {
		printEnvironmentRestoreReport(report)
	}
	if !report.OK {
		return errors.New("environment restore did not complete")
	}
	return nil
}

func buildEnvironmentRestoreReport(ctx context.Context, env store.Environment, workspace string, execute bool, pull bool, prepareReposOnly bool, healthTimeout time.Duration, workflowOptions environmentRestoreWorkflowOptions, cleanupOptions environmentRestoreDockerCleanupOptions, componentGraphs ...store.EnvironmentComponentGraph) (environmentRestoreReport, error) {
	workflowID := strings.TrimSpace(env.VerificationWorkflowID)
	if workflowID == "" {
		return environmentRestoreReport{}, fmt.Errorf("environment %s has no verification workflow; restore must be anchored to a verified workflow", env.ID)
	}
	plan, err := environmentRestoreBuildPlanFromEnvironment(env, workflowID, workspace, workflowOptions.StoreURL, componentGraphs...)
	if err != nil {
		return environmentRestoreReport{}, err
	}
	report := newEnvironmentRestoreReport(env, plan, execute, workflowOptions, cleanupOptions, prepareReposOnly)
	environmentRestoreAddSourceReports(ctx, &report, plan, execute, pull)
	report.Docker = environmentRestoreDockerForReport(ctx, report, plan, execute, prepareReposOnly, healthTimeout, cleanupOptions)
	if !report.Docker.OK {
		report.OK = false
	}
	environmentRestoreMaybeRunWorkflow(ctx, &env, &report, plan, workflowOptions)
	environmentRestoreAddDryRunNextAction(&report, execute, cleanupOptions)
	report.Readiness = environmentRestoreReadinessReport(report, plan.PackageSpec, plan.Specs, cleanupOptions)
	if !report.Readiness.OK {
		report.OK = false
		if strings.TrimSpace(report.Error) == "" {
			report.Error = "restore readiness did not pass"
		}
	}
	report.CleanMachine = environmentRestoreCleanMachinePlanForReport(report, workflowOptions, cleanupOptions)
	environmentRestoreMaybePersist(ctx, &env, &report, plan, workflowOptions, cleanupOptions)
	return report, nil
}

func environmentRestoreBuildPlanFromEnvironment(env store.Environment, workflowID string, workspace string, storeURL string, componentGraphs ...store.EnvironmentComponentGraph) (environmentRestoreBuildPlan, error) {
	workspace, err := filepath.Abs(strings.TrimSpace(workspace))
	if err != nil {
		return environmentRestoreBuildPlan{}, err
	}
	graph := store.EnvironmentComponentGraph{}
	if len(componentGraphs) > 0 {
		graph = componentGraphs[0]
	}
	compose := environmentRestoreComposeWithComponentAssets(env.ID, jsonObjectString(env.ComposeJSON), graph)
	return environmentRestoreBuildPlan{
		WorkflowID:           workflowID,
		Workspace:            workspace,
		Specs:                environmentRestoreRepoSpecs(env, workspace),
		Compose:              compose,
		ComponentGraph:       graph,
		PackageSpec:          environmentRestorePackageSpecFromCompose(compose, workspace),
		HealthChecks:         environmentRestoreEffectiveHealthChecks(jsonArrayString(env.HealthChecksJSON), compose, graph),
		ComponentGraphReport: environmentRestoreComponentGraphReport(env.ID, graph),
		ComponentStartupPlan: controlplane.EnvironmentComponentStartupPlanReport(env.ID, graph),
		AttemptedAt:          time.Now().UTC(),
		RemoteOnly:           environmentRestoreRequiresRemoteSources(storeURL),
	}, nil
}

func newEnvironmentRestoreReport(env store.Environment, plan environmentRestoreBuildPlan, execute bool, workflowOptions environmentRestoreWorkflowOptions, cleanupOptions environmentRestoreDockerCleanupOptions, prepareReposOnly bool) environmentRestoreReport {
	report := environmentRestoreReport{
		OK:                   true,
		RestoreID:            "restore." + safeReportID(env.ID) + "." + plan.AttemptedAt.Format("20060102T150405.000000000Z"),
		Executed:             execute,
		EnvironmentID:        env.ID,
		VerificationWorkflow: plan.WorkflowID,
		Workspace:            plan.Workspace,
		Compose:              plan.Compose,
		HealthChecks:         plan.HealthChecks,
		ComponentGraph:       plan.ComponentGraphReport,
		ComponentStartupPlan: plan.ComponentStartupPlan,
		Preflight:            environmentRestorePreflightReport(plan.PackageSpec, plan.Specs, plan.Compose, plan.Workspace, cleanupOptions, prepareReposOnly),
		SourcePolicy:         environmentRestoreSourcePolicyReport(plan.PackageSpec, plan.Specs, plan.RemoteOnly),
		Workflow: environmentRestoreWorkflowRun{
			OK:         !workflowOptions.Run,
			Action:     "not-requested",
			WorkflowID: plan.WorkflowID,
		},
		NextActions: []string{
			"run verification workflow " + plan.WorkflowID,
		},
	}
	if !report.Preflight.OK {
		report.OK = false
	}
	if !report.SourcePolicy.OK {
		report.OK = false
	}
	if report.ComponentGraph.Configured && !report.ComponentGraph.OK {
		report.OK = false
	}
	return report
}

func environmentRestoreAddSourceReports(ctx context.Context, report *environmentRestoreReport, plan environmentRestoreBuildPlan, execute bool, pull bool) {
	report.Package = environmentRestorePackage(ctx, plan.PackageSpec, execute, pull, plan.RemoteOnly)
	if !report.Package.OK {
		report.OK = false
	}
	for _, spec := range plan.Specs {
		item := environmentRestoreRepo(ctx, spec, execute, pull)
		if !item.OK {
			report.OK = false
		}
		report.Repos = append(report.Repos, item)
	}
	report.ComponentAssets = environmentRestoreRemoteComponentAssets(ctx, report.EnvironmentID, plan.ComponentGraph, plan.Workspace, execute, pull)
	for _, item := range report.ComponentAssets {
		if !item.OK {
			report.OK = false
		}
	}
}

func environmentRestoreDockerForReport(ctx context.Context, report environmentRestoreReport, plan environmentRestoreBuildPlan, execute bool, prepareReposOnly bool, healthTimeout time.Duration, cleanupOptions environmentRestoreDockerCleanupOptions) environmentRestoreDockerReport {
	if report.OK && prepareReposOnly {
		return environmentRestorePrepareReposOnlyDockerReport(plan, execute)
	}
	if report.OK && cleanupOptions.UseExistingContainers {
		return environmentRestoreUseExistingContainers(ctx, plan.ComponentGraph, plan.Compose, plan.HealthChecks, plan.Workspace, execute, healthTimeout)
	}
	if report.OK {
		return environmentRestoreDocker(ctx, plan.ComponentGraph, plan.Compose, plan.HealthChecks, plan.Workspace, execute, healthTimeout, cleanupOptions)
	}
	return environmentRestoreSkippedDockerReport(report, plan.Workspace)
}

func environmentRestorePrepareReposOnlyDockerReport(plan environmentRestoreBuildPlan, execute bool) environmentRestoreDockerReport {
	docker := environmentRestoreDockerReport{
		OK:        true,
		Action:    "skipped-after-repository-preparation",
		Workdir:   plan.Workspace,
		Generated: prepareEnvironmentRestoreGeneratedFiles(plan.Compose, plan.Workspace, execute),
	}
	for _, item := range docker.Generated {
		if !item.OK {
			docker.OK = false
			docker.Action = "prepare-generated-files"
			docker.Error = item.Error
			break
		}
	}
	return docker
}

func environmentRestoreSkippedDockerReport(report environmentRestoreReport, workspace string) environmentRestoreDockerReport {
	if !report.Preflight.OK {
		return environmentRestoreDockerReport{
			OK:      false,
			Action:  "skipped-due-to-preflight",
			Workdir: workspace,
			Error:   "restore preflight did not pass",
		}
	}
	if !report.SourcePolicy.OK {
		return environmentRestoreDockerReport{
			OK:      false,
			Action:  "skipped-due-to-source-policy",
			Workdir: workspace,
			Error:   "remote Git source policy did not pass",
		}
	}
	return environmentRestoreDockerReport{
		OK:      false,
		Action:  "skipped-due-to-repository-error",
		Workdir: workspace,
		Error:   "repository preparation did not complete",
	}
}

func environmentRestoreMaybeRunWorkflow(ctx context.Context, env *store.Environment, report *environmentRestoreReport, plan environmentRestoreBuildPlan, workflowOptions environmentRestoreWorkflowOptions) {
	if report.OK && workflowOptions.Run {
		report.Workflow = environmentRestoreRunWorkflow(ctx, plan.WorkflowID, plan.Workspace, workflowOptions)
		if !report.Workflow.OK {
			report.OK = false
		}
		if report.Workflow.RunID != "" {
			env.LastVerificationRunID = report.Workflow.RunID
			env.LastVerificationStatus = statusText(report.Workflow.OK)
			env.EvidenceComplete = report.Workflow.OK && report.Workflow.Acceptance.OK
			env.TopologyComplete = report.Workflow.OK && report.Workflow.Acceptance.OK
			env.Verified = false
			env.Status = "verification-recorded"
		}
	}
}

func environmentRestoreAddDryRunNextAction(report *environmentRestoreReport, execute bool, cleanupOptions environmentRestoreDockerCleanupOptions) {
	if !execute {
		nextAction := "review the Docker Compose plan, then rerun with --execute"
		if cleanupOptions.AssumeCleanDocker {
			nextAction = "run this environment on the colleague machine with --execute after reviewing the clean-machine Docker plan"
		}
		report.NextActions = append([]string{nextAction}, report.NextActions...)
	}
}

func environmentRestoreMaybePersist(ctx context.Context, env *store.Environment, report *environmentRestoreReport, plan environmentRestoreBuildPlan, workflowOptions environmentRestoreWorkflowOptions, cleanupOptions environmentRestoreDockerCleanupOptions) {
	if strings.TrimSpace(workflowOptions.StoreURL) != "" {
		persisted, err := environmentRestorePersistEnvironment(ctx, workflowOptions.StoreURL, *env, *report, plan.AttemptedAt)
		if err != nil {
			report.OK = false
			report.Error = err.Error()
			if report.Workflow.Action == "run-verification-workflow" {
				report.Workflow.OK = false
				report.Workflow.Error = err.Error()
			}
			report.Readiness = environmentRestoreReadinessReport(*report, plan.PackageSpec, plan.Specs, cleanupOptions)
		} else {
			report.Environment = environmentPayload(persisted)
		}
	}
}

func environmentRestoreCleanMachinePlanForReport(report environmentRestoreReport, workflowOptions environmentRestoreWorkflowOptions, cleanupOptions environmentRestoreDockerCleanupOptions) environmentRestoreCleanMachinePlan {
	if !cleanupOptions.AssumeCleanDocker {
		return environmentRestoreCleanMachinePlan{}
	}
	storeRef := strings.TrimSpace(workflowOptions.StoreRef)
	if storeRef == "" {
		storeRef = "STORE_NAME_OR_SQL_DSN"
	}
	plan := environmentRestoreCleanMachinePlan{
		Ready: report.OK,
		Summary: environmentRestoreCleanMachineSummary{
			EnvironmentID:           report.EnvironmentID,
			VerificationWorkflow:    report.VerificationWorkflow,
			Components:              report.ComponentGraph.Components,
			StartupBatches:          len(report.ComponentStartupPlan.Batches),
			HealthGates:             len(report.ComponentStartupPlan.HealthGates),
			ServiceRepositories:     len(report.Repos),
			StartupAssets:           len(report.Preflight.StartupAssets),
			RemoteComponentAssets:   report.ComponentGraph.RemoteAssets,
			InlineAssetBytes:        report.ComponentGraph.InlineAssetBytes,
			RemoteAssetBytes:        report.ComponentGraph.RemoteAssetBytes,
			GraphMetadataLimitBytes: store.ComponentGraphMaxBytes,
			InlineAssetLimitBytes:   store.ComponentAssetInlineMaxBytes,
			DockerImagesStored:      false,
			LargeBinariesStored:     false,
		},
		PrepareCommand: []string{
			"agent-testbench",
			"environment",
			"restore",
			report.EnvironmentID,
			"--store",
			storeRef,
			"--workspace",
			report.Workspace,
			"--execute",
			"--prepare-repos-only",
			"--json",
		},
		ExecuteCommand: []string{
			"agent-testbench",
			"environment",
			"restore",
			report.EnvironmentID,
			"--store",
			storeRef,
			"--workspace",
			report.Workspace,
			"--execute",
			"--json",
		},
		Prerequisites: environmentRestoreCleanMachinePrerequisites(report, workflowOptions),
		Notes: []string{
			"Run prepareCommand on the colleague/new machine first to clone or validate repositories and write Store-generated startup files without starting Docker.",
			"Run executeCommand after prepareCommand passes to start Docker and wait for health gates.",
			"The dry-run assumption is not included in the execute command; Docker will be checked on the target machine before startup.",
			"Add --run-workflow --server-url URL after Docker health passes when the control plane is running for acceptance verification.",
		},
	}
	if !report.Readiness.OK {
		plan.Ready = false
	}
	return plan
}

func environmentRestoreCleanMachinePrerequisites(report environmentRestoreReport, workflowOptions environmentRestoreWorkflowOptions) []environmentRestoreCleanMachinePrerequisite {
	out := []environmentRestoreCleanMachinePrerequisite{
		{
			Name:     "sql-store",
			Required: true,
			OK:       environmentRestoreRequiresRemoteSources(workflowOptions.StoreURL),
			Detail:   "configure the named SQL Store before running restore; the Store must stay outside the target Docker environment",
		},
	}
	for _, tool := range report.Preflight.Tools {
		detail := "required on the colleague machine"
		if tool.Path != "" {
			detail += "; current dry-run found " + tool.Path
		}
		if tool.Error != "" {
			detail = tool.Error
		}
		out = append(out, environmentRestoreCleanMachinePrerequisite{
			Name:     "tool:" + tool.Name,
			Required: tool.Required,
			OK:       tool.OK,
			Detail:   detail,
		})
	}
	for _, name := range []string{
		"component-graph",
		"component-startup-plan",
		"remote-git-sources",
		"store-startup-files",
		"startup-assets",
		"service-repositories",
		"docker-start-plan",
		"health-probes",
	} {
		if item, ok := environmentRestoreReadinessItemByName(report.Readiness.Items, name); ok {
			out = append(out, environmentRestoreCleanMachinePrerequisite{
				Name:     name,
				Required: item.Required,
				OK:       item.OK,
				Detail:   item.Detail,
			})
		}
	}
	return out
}

func environmentRestoreReadinessItemByName(items []environmentRestoreReadinessItem, name string) (environmentRestoreReadinessItem, bool) {
	for _, item := range items {
		if item.Name == name {
			return item, true
		}
	}
	return environmentRestoreReadinessItem{}, false
}

func environmentRestorePersistEnvironment(ctx context.Context, storeURL string, env store.Environment, report environmentRestoreReport, attemptedAt time.Time) (store.Environment, error) {
	env.SummaryJSON = environmentRestoreSummaryJSON(env.SummaryJSON, report, attemptedAt)
	env.UpdatedAt = time.Now().UTC()
	runtime, err := openStore(ctx, storeURL)
	if err != nil {
		return env, err
	}
	defer closeCLIStore(runtime)
	return runtime.UpsertEnvironment(ctx, env)
}

func environmentRestoreSummaryJSON(existing string, report environmentRestoreReport, attemptedAt time.Time) string {
	summary := jsonObjectString(existing)
	finishedAt := time.Now().UTC()
	lastRestore := map[string]any{
		"id":                   report.RestoreID,
		"attemptedAt":          attemptedAt.Format(time.RFC3339Nano),
		"finishedAt":           finishedAt.Format(time.RFC3339Nano),
		"durationMs":           maxInt64(0, finishedAt.Sub(attemptedAt).Milliseconds()),
		"ok":                   report.OK,
		"executed":             report.Executed,
		"phase":                environmentRestorePhase(report),
		"environmentId":        report.EnvironmentID,
		"verificationWorkflow": report.VerificationWorkflow,
		"workspace":            report.Workspace,
		"preflight": map[string]any{
			"ok":                 report.Preflight.OK,
			"tools":              environmentRestoreSummaryTools(report.Preflight.Tools),
			"heavySteps":         report.Preflight.HeavySteps,
			"containerConflicts": report.Preflight.ContainerConflicts,
			"startupAssets":      environmentRestoreSummaryStartupAssets(report.Preflight.StartupAssets),
		},
		"package":      environmentRestoreSummaryPackage(report.Package),
		"sourcePolicy": report.SourcePolicy,
		"repositories": environmentRestoreSummaryRepos(report.Repos),
		"readiness":    environmentRestoreSummaryReadiness(report.Readiness),
		"docker":       environmentRestoreSummaryDocker(report.Docker),
		"workflow": map[string]any{
			"action":     report.Workflow.Action,
			"ok":         report.Workflow.OK,
			"workflowId": report.Workflow.WorkflowID,
			"runId":      report.Workflow.RunID,
			"outputDir":  report.Workflow.OutputDir,
			"reportUrl":  report.Workflow.ReportURL,
			"counts":     report.Workflow.Counts,
			"acceptance": report.Workflow.Acceptance,
			"error":      report.Workflow.Error,
		},
		"environmentMutation": map[string]any{
			"lastVerificationRunId":  report.Workflow.RunID,
			"lastVerificationStatus": statusText(report.Workflow.OK),
			"evidenceComplete":       report.Workflow.Action == "run-acceptance-workflow" && report.Workflow.OK && report.Workflow.Acceptance.OK,
			"topologyComplete":       report.Workflow.Action == "run-acceptance-workflow" && report.Workflow.OK && report.Workflow.Acceptance.OK,
			"verified":               false,
		},
		"nextActions": report.NextActions,
	}
	if strings.TrimSpace(report.Error) != "" {
		lastRestore["error"] = report.Error
	}
	summary["lastRestore"] = lastRestore
	attempts := appendRestoreAttemptSummary(summary["restoreAttempts"], lastRestore)
	summary["restoreAttempts"] = attempts
	raw := mustCompactJSON(summary)
	for len(raw) > store.EnvironmentSummaryMaxBytes && len(attempts) > 1 {
		attempts = attempts[1:]
		summary["restoreAttempts"] = attempts
		raw = mustCompactJSON(summary)
	}
	if len(raw) > store.EnvironmentSummaryMaxBytes {
		summary["restoreAttempts"] = []any{}
		raw = mustCompactJSON(summary)
	}
	return raw
}

func appendRestoreAttemptSummary(existing any, attempt map[string]any) []any {
	out := []any{}
	if values, ok := existing.([]any); ok {
		for _, value := range values {
			out = append(out, compactRestoreAttemptSummary(mapFromReportAny(value)))
		}
	}
	out = append(out, compactRestoreAttemptSummary(attempt))
	if len(out) > environmentRestoreAttemptLimit {
		out = out[len(out)-environmentRestoreAttemptLimit:]
	}
	return out
}

func compactRestoreAttemptSummary(attempt map[string]any) map[string]any {
	preflight := mapFromReportAny(attempt["preflight"])
	sourcePolicy := mapFromReportAny(attempt["sourcePolicy"])
	readiness := mapFromReportAny(attempt["readiness"])
	docker := mapFromReportAny(attempt["docker"])
	workflow := mapFromReportAny(attempt["workflow"])
	out := map[string]any{
		"id":          valueString(attempt["id"]),
		"attemptedAt": valueString(attempt["attemptedAt"]),
		"finishedAt":  valueString(attempt["finishedAt"]),
		"durationMs":  intFromReportAny(attempt["durationMs"]),
		"ok":          boolFromReportAny(attempt["ok"]),
		"executed":    boolFromReportAny(attempt["executed"]),
		"phase":       valueString(attempt["phase"]),
		"preflight": map[string]any{
			"ok": boolFromReportAny(preflight["ok"]),
		},
		"sourcePolicy": map[string]any{
			"ok":         boolFromReportAny(sourcePolicy["ok"]),
			"remoteOnly": boolFromReportAny(sourcePolicy["remoteOnly"]),
		},
		"readiness": map[string]any{
			"ok":          boolFromReportAny(readiness["ok"]),
			"action":      valueString(readiness["action"]),
			"failedItems": listFromReportAny(readiness["failedItems"]),
		},
		"docker": map[string]any{
			"ok":           boolFromReportAny(docker["ok"]),
			"action":       valueString(docker["action"]),
			"commandCount": intFromReportAny(docker["commandCount"]),
		},
		"workflow": map[string]any{
			"ok":     boolFromReportAny(workflow["ok"]),
			"action": valueString(workflow["action"]),
			"runId":  valueString(workflow["runId"]),
		},
	}
	if environmentID := valueString(attempt["environmentId"]); environmentID != "" {
		out["environmentId"] = environmentID
	}
	if errText := valueString(attempt["error"]); errText != "" {
		out["error"] = truncateReportText(errText, 500)
	}
	return out
}

func environmentRestorePhase(report environmentRestoreReport) string {
	if report.OK {
		return "completed"
	}
	if !report.Preflight.OK {
		return "preflight"
	}
	if report.Package.Configured && !report.Package.OK {
		return "package"
	}
	for _, item := range report.Repos {
		if !item.OK {
			return "repository"
		}
	}
	if !report.Docker.OK {
		for _, item := range report.Docker.HealthChecks {
			if !item.OK {
				return "health-check"
			}
		}
		return "docker"
	}
	if !report.Readiness.OK {
		return "readiness"
	}
	if report.Workflow.Action == "run-verification-workflow" && !report.Workflow.OK {
		return "workflow"
	}
	if strings.TrimSpace(report.Error) != "" {
		return "persist"
	}
	return "completed"
}
