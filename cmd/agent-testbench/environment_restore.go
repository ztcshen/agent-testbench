package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"agent-testbench/internal/domain/environmentfiles"
	"agent-testbench/internal/domain/environmentsource"
	"agent-testbench/internal/environmentprojection"
	"agent-testbench/internal/server/controlplane"
	"agent-testbench/internal/store"
)

const (
	environmentRestoreAttemptLimit                      = 20
	environmentRestoreDockerActionSkippedFileProjection = "skipped-due-to-file-projection"
	environmentRestorePlanProgressEnv                   = "AGENT_TESTBENCH_RESTORE_PLAN_PROGRESS_INTERVAL_MS"
	environmentRestorePlanTimeoutEnv                    = "AGENT_TESTBENCH_RESTORE_PLAN_TIMEOUT_MS"
)

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
	FileProjection       environmentfiles.ProjectionReport            `json:"fileProjection"`
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
	StoreFiles           []store.EnvironmentFile
	ComponentGraphReport environmentRestoreComponentGraph
	ComponentStartupPlan controlplane.EnvironmentComponentStartupPlan
	AttemptedAt          time.Time
	RemoteOnly           bool
}

type environmentRestorePlanBuildResult struct {
	Plan   environmentRestoreBuildPlan
	Report environmentRestoreReport
	Err    error
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
	files, err := runtime.ListEnvironmentFiles(ctx, env.ID)
	if err != nil {
		return err
	}
	services, err := runtime.ListEnvironmentServices(ctx, env.ID)
	if err != nil {
		return err
	}
	healthChecks, err := runtime.ListEnvironmentHealthChecks(ctx, env.ID)
	if err != nil {
		return err
	}
	options.Workflow.EnvironmentID = env.ID
	report, err := buildEnvironmentRestoreReportWithStructuredState(ctx, env, options.Workspace, options.Execute, options.Pull, options.PrepareReposOnly, options.HealthTimeout, options.Workflow, options.Cleanup, files, services, healthChecks, componentGraph)
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
	return buildEnvironmentRestoreReportWithFiles(ctx, env, workspace, execute, pull, prepareReposOnly, healthTimeout, workflowOptions, cleanupOptions, nil, componentGraphs...)
}

func buildEnvironmentRestoreReportWithFiles(ctx context.Context, env store.Environment, workspace string, execute bool, pull bool, prepareReposOnly bool, healthTimeout time.Duration, workflowOptions environmentRestoreWorkflowOptions, cleanupOptions environmentRestoreDockerCleanupOptions, files []store.EnvironmentFile, componentGraphs ...store.EnvironmentComponentGraph) (environmentRestoreReport, error) {
	return buildEnvironmentRestoreReportWithStructuredState(ctx, env, workspace, execute, pull, prepareReposOnly, healthTimeout, workflowOptions, cleanupOptions, files, nil, nil, componentGraphs...)
}

func buildEnvironmentRestoreReportWithStructuredState(ctx context.Context, env store.Environment, workspace string, execute bool, pull bool, prepareReposOnly bool, healthTimeout time.Duration, workflowOptions environmentRestoreWorkflowOptions, cleanupOptions environmentRestoreDockerCleanupOptions, files []store.EnvironmentFile, services []store.EnvironmentService, checks []store.EnvironmentHealthCheck, componentGraphs ...store.EnvironmentComponentGraph) (environmentRestoreReport, error) {
	workflowID := strings.TrimSpace(env.VerificationWorkflowID)
	if workflowID == "" {
		return environmentRestoreReport{}, fmt.Errorf("environment %s has no verification workflow; restore must be anchored to a verified workflow", env.ID)
	}
	attemptedAt := time.Now().UTC()
	environmentRestoreEmitRunPlanningStarted(ctx, env.ID, attemptedAt)
	result, timedOut := environmentRestoreBuildPlanAndReportWithWatchdog(ctx, env, workflowID, workspace, execute, pull, workflowOptions, cleanupOptions, prepareReposOnly, files, services, checks, attemptedAt, componentGraphs...)
	if result.Err != nil {
		if timedOut {
			report := newEnvironmentRestorePlanErrorReport(env, workflowID, workspace, execute, attemptedAt, result.Err.Error())
			environmentRestoreEmitStep(ctx, "step_completed", "environment.restore.plan", "failed", report.EnvironmentID, "environment restore plan failed", report.Error)
			return report, nil
		}
		return environmentRestoreReport{}, result.Err
	}
	plan := result.Plan
	report := result.Report
	environmentRestoreEmitStep(ctx, "step_completed", "environment.restore.plan", "passed", report.EnvironmentID, "environment restore plan prepared", "")
	environmentRestoreAddSourceReports(ctx, &report, plan, execute, pull)
	environmentRestoreApplyPreDockerReadinessGates(&report, cleanupOptions)
	report.Docker = environmentRestoreDockerForReport(ctx, report, plan, execute, pull, prepareReposOnly, healthTimeout, cleanupOptions)
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

func environmentRestoreBuildPlanAndReportWithWatchdog(ctx context.Context, env store.Environment, workflowID string, workspace string, execute bool, pull bool, workflowOptions environmentRestoreWorkflowOptions, cleanupOptions environmentRestoreDockerCleanupOptions, prepareReposOnly bool, files []store.EnvironmentFile, services []store.EnvironmentService, checks []store.EnvironmentHealthCheck, attemptedAt time.Time, componentGraphs ...store.EnvironmentComponentGraph) (environmentRestorePlanBuildResult, bool) {
	results := make(chan environmentRestorePlanBuildResult, 1)
	started := time.Now()
	go func() {
		plan, err := environmentRestoreBuildPlanFromEnvironmentWithStructuredState(env, workflowID, workspace, workflowOptions.StoreURL, files, services, checks, componentGraphs...)
		if err != nil {
			results <- environmentRestorePlanBuildResult{Err: err}
			return
		}
		plan.AttemptedAt = attemptedAt
		results <- environmentRestorePlanBuildResult{
			Plan:   plan,
			Report: newEnvironmentRestoreReport(env, plan, execute, pull, workflowOptions, cleanupOptions, prepareReposOnly),
		}
	}()

	interval := environmentRestorePlanProgressInterval()
	timeout := environmentRestorePlanTimeout()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	for {
		select {
		case result := <-results:
			return result, false
		case <-ticker.C:
			elapsed := time.Since(started)
			agentEmitEvent(ctx, agentStreamEvent{
				Type:      "tool_observation",
				Phase:     "environment.restore.plan",
				Status:    agentCommandStatusWaiting,
				Target:    "docker.compose.version",
				Message:   "environment restore plan still running before Docker execution",
				ElapsedMs: elapsed.Milliseconds(),
			})
		case <-timer.C:
			return environmentRestorePlanBuildResult{Err: fmt.Errorf("environment restore plan timed out after %s before Docker execution; last observed operation: docker.compose.version; command: docker compose version; no child stdout/stderr was captured before the watchdog fired, so this may be a Docker CLI hang or restore preflight bookkeeping waiting on that command", timeout)}, true
		case <-ctx.Done():
			return environmentRestorePlanBuildResult{Err: fmt.Errorf("environment restore plan canceled before Docker execution: %w", ctx.Err())}, true
		}
	}
}

func environmentRestorePlanProgressInterval() time.Duration {
	return positiveDurationFromEnv(environmentRestorePlanProgressEnv, 5*time.Second)
}

func environmentRestorePlanTimeout() time.Duration {
	return positiveDurationFromEnv(environmentRestorePlanTimeoutEnv, 2*time.Minute)
}

func positiveDurationFromEnv(name string, defaultDuration time.Duration) time.Duration {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return defaultDuration
	}
	ms, err := strconv.Atoi(raw)
	if err != nil || ms <= 0 {
		return defaultDuration
	}
	return time.Duration(ms) * time.Millisecond
}

func newEnvironmentRestorePlanErrorReport(env store.Environment, workflowID string, workspace string, execute bool, attemptedAt time.Time, errText string) environmentRestoreReport {
	absoluteWorkspace := strings.TrimSpace(workspace)
	if absoluteWorkspace != "" {
		if resolved, err := filepath.Abs(absoluteWorkspace); err == nil {
			absoluteWorkspace = resolved
		}
	}
	return environmentRestoreReport{
		OK:                   false,
		RestoreID:            "restore." + safeReportID(env.ID) + "." + attemptedAt.Format("20060102T150405.000000000Z"),
		Executed:             execute,
		EnvironmentID:        env.ID,
		VerificationWorkflow: workflowID,
		Workspace:            absoluteWorkspace,
		Error:                errText,
		Workflow: environmentRestoreWorkflowRun{
			OK:         false,
			Action:     "not-started",
			WorkflowID: workflowID,
			Error:      errText,
		},
		NextActions: []string{
			"retry environment restore after the reported Store or Docker preflight blocker is resolved",
		},
	}
}

func environmentRestoreBuildPlanFromEnvironmentWithStructuredState(env store.Environment, workflowID string, workspace string, storeURL string, files []store.EnvironmentFile, services []store.EnvironmentService, checks []store.EnvironmentHealthCheck, componentGraphs ...store.EnvironmentComponentGraph) (environmentRestoreBuildPlan, error) {
	workspace, err := filepath.Abs(strings.TrimSpace(workspace))
	if err != nil {
		return environmentRestoreBuildPlan{}, err
	}
	graph := store.EnvironmentComponentGraph{}
	if len(componentGraphs) > 0 {
		graph = componentGraphs[0]
	}
	env, err = environmentRestoreEnvironmentForPlan(env, services, checks)
	if err != nil {
		return environmentRestoreBuildPlan{}, err
	}
	compose, err := environmentRestoreComposeForPlan(env, files)
	if err != nil {
		return environmentRestoreBuildPlan{}, err
	}
	compose = environmentRestoreComposeWithComponentAssets(env.ID, compose, graph)
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
		StoreFiles:           store.NormalizeEnvironmentFiles(files),
		ComponentGraphReport: environmentRestoreComponentGraphReport(env.ID, graph),
		ComponentStartupPlan: controlplane.EnvironmentComponentStartupPlanReport(env.ID, graph),
		AttemptedAt:          time.Now().UTC(),
		RemoteOnly:           environmentRestoreRequiresRemoteSources(storeURL),
	}, nil
}

func environmentRestoreEnvironmentForPlan(env store.Environment, services []store.EnvironmentService, checks []store.EnvironmentHealthCheck) (store.Environment, error) {
	if len(services) == 0 && len(checks) == 0 {
		return env, nil
	}
	return store.MergeEnvironmentRuntimeMetadataIntoJSON(env, services, checks)
}

func environmentRestoreComposeForPlan(env store.Environment, files []store.EnvironmentFile) (map[string]any, error) {
	if len(files) == 0 {
		return jsonObjectString(env.ComposeJSON), nil
	}
	base := env
	base.ComposeJSON = mustCompactJSON(environmentComposeConfigWithoutMaterializedEnvironmentFiles(jsonObjectString(env.ComposeJSON), files))
	projected, err := store.MergeEnvironmentFilesIntoComposeJSON(base, files)
	if err != nil {
		return nil, err
	}
	return jsonObjectString(projected.ComposeJSON), nil
}

func newEnvironmentRestoreReport(env store.Environment, plan environmentRestoreBuildPlan, execute bool, pull bool, workflowOptions environmentRestoreWorkflowOptions, cleanupOptions environmentRestoreDockerCleanupOptions, prepareReposOnly bool) environmentRestoreReport {
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
		Preflight:            environmentRestorePreflightReport(plan.PackageSpec, plan.Specs, plan.Compose, plan.Workspace, execute, pull, cleanupOptions, prepareReposOnly, plan.RemoteOnly),
		FileProjection:       environmentRestoreFileProjection(env, plan),
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

func environmentRestoreApplyPreDockerReadinessGates(report *environmentRestoreReport, cleanupOptions environmentRestoreDockerCleanupOptions) {
	if environmentRestoreFileProjectionReadyForDocker(*report, cleanupOptions) {
		return
	}
	report.OK = false
	if strings.TrimSpace(report.Error) == "" {
		report.Error = "file projection readiness did not pass before Docker startup"
	}
}

func environmentRestoreFileProjectionReadyForDocker(report environmentRestoreReport, cleanupOptions environmentRestoreDockerCleanupOptions) bool {
	if !environmentRestoreFileProjectionRequired(report, cleanupOptions) {
		return true
	}
	if len(report.FileProjection.Files) == 0 && strings.TrimSpace(valueString(report.Compose["startCommand"])) != "" {
		return true
	}
	return len(report.FileProjection.Files) > 0 && report.FileProjection.OK
}

func environmentRestoreFileProjection(env store.Environment, plan environmentRestoreBuildPlan) environmentfiles.ProjectionReport {
	compose := plan.Compose
	if plan.RemoteOnly {
		compose = map[string]any{}
		for key, value := range plan.Compose {
			if key != "package" {
				compose[key] = value
			}
		}
	}
	return environmentprojection.FromComposeWithEnvironmentFiles(compose, jsonObjectString(env.SummaryJSON), plan.ComponentGraph, plan.StoreFiles)
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

func environmentRestoreDockerForReport(ctx context.Context, report environmentRestoreReport, plan environmentRestoreBuildPlan, execute bool, pull bool, prepareReposOnly bool, healthTimeout time.Duration, cleanupOptions environmentRestoreDockerCleanupOptions) environmentRestoreDockerReport {
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
		compose := environmentRestoreComposeWithPullPolicy(plan.Compose, pull)
		docker = environmentRestoreDocker(ctx, plan.ComponentGraph, compose, plan.HealthChecks, plan.Workspace, execute, healthTimeout, cleanupOptions)
		environmentRestoreEmitStep(ctx, "step_completed", "docker.restore", statusText(docker.OK), report.EnvironmentID, docker.Action, docker.Error)
		return docker
	}
	docker = environmentRestoreSkippedDockerReport(report, plan.Workspace, cleanupOptions)
	environmentRestoreEmitStep(ctx, "step_completed", "docker.restore", statusText(docker.OK), report.EnvironmentID, docker.Action, docker.Error)
	return docker
}

func environmentRestoreComposeWithPullPolicy(compose map[string]any, pull bool) map[string]any {
	if pull || boolFromReportAny(compose["skipPull"]) {
		return compose
	}
	out := map[string]any{}
	for key, value := range compose {
		out[key] = value
	}
	out["skipPull"] = true
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

func environmentRestoreSkippedDockerReport(report environmentRestoreReport, workspace string, cleanupOptions environmentRestoreDockerCleanupOptions) environmentRestoreDockerReport {
	if !report.SourcePolicy.OK {
		return environmentRestoreDockerReport{
			OK:      false,
			Action:  "skipped-due-to-source-policy",
			Workdir: workspace,
			Error:   "remote Git source policy did not pass",
		}
	}
	if !environmentRestoreFileProjectionReadyForDocker(report, cleanupOptions) {
		return environmentRestoreDockerReport{
			OK:      false,
			Action:  environmentRestoreDockerActionSkippedFileProjection,
			Workdir: workspace,
			Error:   "file projection readiness did not pass before Docker startup",
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
	generated := generatedFileContentMapFromAny(compose["generatedFiles"])
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
