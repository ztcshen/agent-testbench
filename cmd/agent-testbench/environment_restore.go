package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"agent-testbench/internal/domain/environmentsource"
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

type environmentRestoreComponentGraph = controlplane.EnvironmentComponentGraphReadiness

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

func runEnvironmentRestore(ctx context.Context, args []string) error {
	options, err := parseEnvironmentRestoreCommandOptions(args)
	if err != nil {
		return err
	}
	if options.OutputFormat == cliOutputFormatStreamJSON {
		ctx = contextWithEnvironmentRestoreEventStream(ctx, os.Stdout)
	} else if options.Execute && !options.JSONOutput {
		ctx = contextWithEnvironmentRestoreProgress(ctx, os.Stderr)
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
	if options.OutputFormat == cliOutputFormatStreamJSON {
		environmentRestoreEmitRunCompleted(ctx, report)
	} else if options.JSONOutput {
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
	environmentRestoreEmitRunStarted(ctx, report)
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
	specs := environmentsource.RepoSpecs(env.ReposJSON, env.ServicesJSON, workspace)
	compose = environmentRestoreComposeWithRepoCheckouts(compose, specs)
	return environmentRestoreBuildPlan{
		WorkflowID:           workflowID,
		Workspace:            workspace,
		Specs:                specs,
		Compose:              compose,
		ComponentGraph:       graph,
		PackageSpec:          environmentsource.PackageSpecFromCompose(compose, workspace),
		HealthChecks:         environmentRestoreEffectiveHealthChecks(jsonArrayString(env.HealthChecksJSON), compose, graph, workspace),
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
		Preflight:            environmentRestorePreflightReport(plan.PackageSpec, plan.Specs, plan.Compose, plan.Workspace, execute, cleanupOptions, prepareReposOnly),
		SourcePolicy:         environmentsource.SourcePolicyReport(plan.Specs, plan.RemoteOnly),
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
	environmentRestoreEmitStep(ctx, "step_started", "source.package", "running", plan.PackageSpec.Checkout, "preparing environment package source", "")
	report.Package = environmentRestorePackage(ctx, plan.PackageSpec, execute, pull, plan.RemoteOnly)
	environmentRestoreEmitStep(ctx, "step_completed", "source.package", statusText(report.Package.OK), plan.PackageSpec.Checkout, report.Package.Action, report.Package.Error)
	if !report.Package.OK {
		report.OK = false
	}
	for _, spec := range plan.Specs {
		environmentRestoreEmitStep(ctx, "step_started", "source.repository", "running", spec.ServiceID, "preparing repository source", "")
		item := environmentRestoreRepo(ctx, spec, execute, pull)
		environmentRestoreEmitStep(ctx, "step_completed", "source.repository", statusText(item.OK), spec.ServiceID, item.Action, item.Error)
		if !item.OK {
			report.OK = false
		}
		report.Repos = append(report.Repos, item)
	}
	environmentRestoreEmitStep(ctx, "step_started", "source.component-assets", "running", report.EnvironmentID, "preparing remote component assets", "")
	report.ComponentAssets = environmentRestoreRemoteComponentAssets(ctx, report.EnvironmentID, plan.ComponentGraph, plan.Workspace, execute, pull)
	componentAssetsOK := true
	for _, item := range report.ComponentAssets {
		if !item.OK {
			report.OK = false
			componentAssetsOK = false
		}
	}
	environmentRestoreEmitStep(ctx, "step_completed", "source.component-assets", statusText(componentAssetsOK), report.EnvironmentID, fmt.Sprintf("%d remote component asset(s)", len(report.ComponentAssets)), "")
}

func environmentRestoreDockerForReport(ctx context.Context, report environmentRestoreReport, plan environmentRestoreBuildPlan, execute bool, prepareReposOnly bool, healthTimeout time.Duration, cleanupOptions environmentRestoreDockerCleanupOptions) environmentRestoreDockerReport {
	environmentRestoreEmitStep(ctx, "step_started", "docker.restore", "running", report.EnvironmentID, "preparing Docker restore phase", "")
	var docker environmentRestoreDockerReport
	if report.OK && prepareReposOnly {
		docker = environmentRestorePrepareReposOnlyDockerReport(plan, execute)
		environmentRestoreEmitStep(ctx, "step_completed", "docker.restore", statusText(docker.OK), report.EnvironmentID, docker.Action, docker.Error)
		return docker
	}
	if report.OK && cleanupOptions.UseExistingContainers {
		docker = environmentRestoreUseExistingContainers(ctx, plan.ComponentGraph, plan.Compose, plan.HealthChecks, plan.Workspace, execute, healthTimeout)
		environmentRestoreEmitStep(ctx, "step_completed", "docker.restore", statusText(docker.OK), report.EnvironmentID, docker.Action, docker.Error)
		return docker
	}
	if report.OK {
		compose := environmentRestoreComposeWithPullSkipServices(plan.Compose, report.Preflight.LocalImageServices)
		docker = environmentRestoreDocker(ctx, plan.ComponentGraph, compose, plan.HealthChecks, plan.Workspace, execute, healthTimeout, cleanupOptions)
		environmentRestoreEmitStep(ctx, "step_completed", "docker.restore", statusText(docker.OK), report.EnvironmentID, docker.Action, docker.Error)
		return docker
	}
	docker = environmentRestoreSkippedDockerReport(report, plan.Workspace)
	environmentRestoreEmitStep(ctx, "step_completed", "docker.restore", statusText(docker.OK), report.EnvironmentID, docker.Action, docker.Error)
	return docker
}

func environmentRestoreComposeWithPullSkipServices(compose map[string]any, services []string) map[string]any {
	services = dedupeStrings(services)
	if len(services) == 0 {
		return compose
	}
	out := map[string]any{}
	for key, value := range compose {
		out[key] = value
	}
	out["skipPullServices"] = services
	return out
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
	if !report.SourcePolicy.OK {
		return environmentRestoreDockerReport{
			OK:      false,
			Action:  "skipped-due-to-source-policy",
			Workdir: workspace,
			Error:   "remote Git source policy did not pass",
		}
	}
	if !report.Preflight.OK {
		if missing := environmentRestoreMissingComposeFile(report.Compose, workspace); missing != "" {
			return environmentRestoreDockerReport{
				OK:          false,
				Action:      "missing-compose-file",
				ComposeFile: missing,
				Workdir:     workspace,
				Error:       fmt.Sprintf("compose file is required before Docker execution: %s", missing),
			}
		}
		return environmentRestoreDockerReport{
			OK:      false,
			Action:  "skipped-due-to-preflight",
			Workdir: workspace,
			Error:   "restore preflight did not pass",
		}
	}
	return environmentRestoreDockerReport{
		OK:      false,
		Action:  "skipped-due-to-repository-error",
		Workdir: workspace,
		Error:   "repository preparation did not complete",
	}
}

func environmentRestoreMissingComposeFile(compose map[string]any, workspace string) string {
	generated := stringMapFromAny(compose["generatedFiles"])
	for _, composeFile := range environmentRestoreComposeFiles(compose) {
		clean := filepath.Clean(strings.TrimSpace(composeFile))
		if clean == "" || clean == "." {
			continue
		}
		if _, ok := generated[clean]; ok {
			continue
		}
		resolved := restoreWorkspacePath(workspace, clean)
		if stat, err := os.Stat(resolved); err != nil || stat.IsDir() {
			return resolved
		}
	}
	return ""
}

func environmentRestoreMaybeRunWorkflow(ctx context.Context, env *store.Environment, report *environmentRestoreReport, plan environmentRestoreBuildPlan, workflowOptions environmentRestoreWorkflowOptions) {
	if !workflowOptions.Run {
		return
	}
	if !report.OK {
		environmentRestoreEmitStep(ctx, "step_completed", "workflow.acceptance", "skipped", plan.WorkflowID, "acceptance workflow skipped because environment restore is not ready", report.Error)
		return
	}
	environmentRestoreEmitPhaseStarted(ctx, "workflow.acceptance", plan.WorkflowID, "running environment acceptance workflow")
	report.Workflow = environmentRestoreRunWorkflow(ctx, plan.WorkflowID, plan.Workspace, workflowOptions)
	environmentRestoreEmitPhaseCompleted(ctx, "workflow.acceptance", plan.WorkflowID, report.Workflow.OK, report.Workflow.Action, report.Workflow.Error)
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
		if err := environmentRestorePersistAppliedMigrationStatuses(ctx, workflowOptions.StoreURL, env.ID, report.Docker.AppliedAssets); err != nil {
			report.OK = false
			report.Error = err.Error()
			report.Readiness = environmentRestoreReadinessReport(*report, plan.PackageSpec, plan.Specs, cleanupOptions)
		}
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
