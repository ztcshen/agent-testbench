package main

import (
	"fmt"
	"path/filepath"
	"strings"

	"agent-testbench/internal/domain/redaction"
)

func environmentRestoreSummaryTools(tools []environmentRestorePreflightTool) []map[string]any {
	out := make([]map[string]any, 0, len(tools))
	for _, item := range tools {
		out = append(out, map[string]any{
			"name":     item.Name,
			"required": item.Required,
			"ok":       item.OK,
			"error":    item.Error,
		})
	}
	return out
}

func environmentRestoreSummaryStartupAssets(assets []environmentRestoreStartupAsset) []map[string]any {
	out := make([]map[string]any, 0, len(assets))
	for _, item := range assets {
		out = append(out, map[string]any{
			"path":        item.Path,
			"source":      item.Source,
			"composeFile": item.ComposeFile,
			"kind":        item.Kind,
			"ok":          item.OK,
			"error":       item.Error,
		})
	}
	return out
}

func environmentRestoreSummaryPackage(report environmentRestorePackageReport) map[string]any {
	return map[string]any{
		"configured": report.Configured,
		"action":     report.Action,
		"ok":         report.OK,
		"url":        report.URL,
		"branch":     report.Branch,
		"ref":        report.Ref,
		"checkout":   report.Checkout,
		"exists":     report.Exists,
		"error":      report.Error,
	}
}

func environmentRestoreSummaryRepos(repos []environmentRestoreRepoReport) []map[string]any {
	out := make([]map[string]any, 0, len(repos))
	for _, item := range repos {
		out = append(out, map[string]any{
			"serviceId": item.ServiceID,
			"action":    item.Action,
			"ok":        item.OK,
			"exists":    item.Exists,
			"branch":    item.Branch,
			"ref":       item.Ref,
			"checkout":  item.Checkout,
			"error":     item.Error,
		})
	}
	return out
}

func environmentRestoreSummaryReadiness(readiness environmentRestoreReadiness) map[string]any {
	failed := []map[string]any{}
	for _, item := range readiness.Items {
		if item.OK {
			continue
		}
		failed = append(failed, map[string]any{
			"name":     item.Name,
			"required": item.Required,
			"detail":   item.Detail,
		})
	}
	return map[string]any{
		"ok":                         readiness.OK,
		"action":                     readiness.Action,
		"pauseBeforeHeavyValidation": readiness.PauseBeforeHeavyValidation,
		"nextStep":                   readiness.NextStep,
		"failedItems":                failed,
	}
}

func environmentRestoreSummaryDocker(report environmentRestoreDockerReport) map[string]any {
	passedHealth := 0
	for _, item := range report.HealthChecks {
		if item.OK {
			passedHealth++
		}
	}
	out := map[string]any{
		"action":         report.Action,
		"ok":             report.OK,
		"composeFile":    report.ComposeFile,
		"commandCount":   len(report.Commands),
		"healthChecks":   len(report.HealthChecks),
		"healthPassed":   passedHealth,
		"healthFailed":   environmentRestoreSummaryFailedHealth(report.HealthChecks),
		"cleanup":        environmentRestoreSummaryCleanup(report.Cleanup),
		"error":          report.Error,
		"capturedOutput": len(report.Output),
	}
	return out
}

func environmentRestoreSummaryFailedHealth(checks []environmentRestoreHealthCheckReport) []map[string]any {
	out := []map[string]any{}
	for _, item := range checks {
		if item.OK {
			continue
		}
		out = append(out, map[string]any{
			"id":         item.ID,
			"kind":       item.Kind,
			"url":        redaction.URL(item.URL),
			"address":    item.Address,
			"service":    item.Service,
			"container":  item.Container,
			"statusCode": item.StatusCode,
			"state":      item.State,
			"health":     item.Health,
			"error":      item.Error,
		})
	}
	return out
}

func environmentRestoreSummaryCleanup(report environmentRestoreDockerCleanupReport) map[string]any {
	return map[string]any{
		"requested":          report.Requested,
		"allowed":            report.Allowed,
		"includeImages":      report.IncludeImages,
		"action":             report.Action,
		"reviewCommandCount": len(report.BackupCommands),
		"commandCount":       len(report.Commands),
		"error":              report.Error,
	}
}

func environmentRestoreReadinessReport(report environmentRestoreReport, packageSpec environmentRestorePackageSpec, specs []environmentRestoreRepoSpec, cleanupOptions environmentRestoreDockerCleanupOptions) environmentRestoreReadiness {
	readiness := environmentRestoreReadiness{
		OK:                         true,
		Action:                     "ready-for-operator-review",
		PauseBeforeHeavyValidation: true,
	}
	addItem := func(name string, required bool, ok bool, detail string) {
		readiness.Items = append(readiness.Items, environmentRestoreReadinessItem{
			Name:     name,
			Required: required,
			OK:       ok,
			Detail:   detail,
		})
		if required && !ok {
			readiness.OK = false
		}
	}

	addItem("store-boundary", true, true, "sandbox SQL Store must stay outside the restored Docker target environment")
	addItem("verification-workflow", true, strings.TrimSpace(report.VerificationWorkflow) != "", "restore is anchored to workflow "+strings.TrimSpace(report.VerificationWorkflow))
	if report.ComponentGraph.Configured {
		detail := fmt.Sprintf("%d component(s), %d blocking dependency edge(s), %d runtime edge(s), %d asset(s), %d inline asset bytes, %d remote asset(s)",
			report.ComponentGraph.Components, report.ComponentGraph.BlockingDependencies, report.ComponentGraph.RuntimeDependencies,
			report.ComponentGraph.Assets, report.ComponentGraph.InlineAssetBytes, report.ComponentGraph.RemoteAssets)
		if strings.TrimSpace(report.ComponentGraph.Error) != "" {
			detail = report.ComponentGraph.Error
		}
		addItem("component-graph", true, report.ComponentGraph.OK, detail)
		startupDetail := fmt.Sprintf("%d startup batch(es), %d health gate(s)", len(report.ComponentStartupPlan.Batches), len(report.ComponentStartupPlan.HealthGates))
		if strings.TrimSpace(report.ComponentStartupPlan.Error) != "" {
			startupDetail = report.ComponentStartupPlan.Error
		}
		addItem("component-startup-plan", true, report.ComponentStartupPlan.OK, startupDetail)
	} else if report.SourcePolicy.RemoteOnly {
		addItem("component-graph", true, false, "SQL Store one-click Docker restore requires a Store component graph for services, middleware, mocks, observability, dependencies, assets, and health gates")
	} else {
		addItem("component-graph", false, true, "no Store component graph recorded yet; restore will use legacy service and compose metadata")
	}
	if len(report.Preflight.ContainerConflicts) > 0 {
		addItem("docker-container-conflicts", true, false, "existing Docker containers would be reused or replaced by fixed container_name values: "+strings.Join(report.Preflight.ContainerConflicts, ", "))
	} else if cleanupOptions.AssumeCleanDocker {
		addItem("docker-container-conflicts", true, true, "clean-machine dry-run assumes target Docker containers are absent; no local Docker deletion was performed")
	} else if cleanupOptions.UseExistingContainers {
		addItem("docker-container-conflicts", true, true, "existing fixed-name Docker containers are explicitly adopted; Docker Compose up will not run")
	} else if strings.TrimSpace(valueString(report.Compose["composeFile"])) != "" {
		addItem("docker-container-conflicts", true, true, "no existing Docker container_name conflicts detected for non-destructive restore")
	}
	if report.SourcePolicy.RemoteOnly {
		detail := "all component source repositories must be remote Git URLs for SQL Store-backed one-click environments; environment startup files come from compact Store metadata"
		if len(report.SourcePolicy.Violations) > 0 {
			detail = strings.Join(report.SourcePolicy.Violations, "; ")
		}
		addItem("remote-git-sources", true, report.SourcePolicy.OK, detail)
	}
	if strings.TrimSpace(packageSpec.URL) != "" {
		detail := "environment package will be cloned or validated before Docker startup"
		if report.Package.Action != "" {
			detail = "environment package " + report.Package.Action + " at " + report.Package.Checkout
		}
		addItem("environment-package", true, report.Package.OK, detail)
	}
	if report.SourcePolicy.RemoteOnly {
		ok, detail := environmentRestoreStoreStartupFilesReady(report.Compose)
		addItem("store-startup-files", true, ok, detail)
	}
	startupAssetsOK, startupAssetsDetail := environmentRestoreStartupAssetsReadiness(report.Preflight.StartupAssets)
	addItem("startup-assets", true, startupAssetsOK, startupAssetsDetail)

	repoOK := true
	for _, item := range report.Repos {
		if !item.OK {
			repoOK = false
			break
		}
	}
	switch {
	case len(specs) == 0:
		addItem("component-repositories", true, true, "no component repositories recorded; Docker uses the recorded compose/start plan and existing local context")
	case report.Executed:
		addItem("component-repositories", true, repoOK, fmt.Sprintf("%d component repository checkout(s) prepared before Docker startup", len(specs)))
	default:
		addItem("component-repositories", true, repoOK, fmt.Sprintf("%d component repository checkout(s) will be cloned or validated before Docker startup", len(specs)))
	}

	dockerPlanOK := report.Docker.OK && (report.Docker.Action == "plan-docker-compose" || report.Docker.Action == "run-docker-compose" || report.Docker.Action == "plan-start-command" || report.Docker.Action == "run-start-command" || report.Docker.Action == "plan-use-existing-containers" || report.Docker.Action == "use-existing-containers" || report.Docker.Action == "skipped-after-repository-preparation")
	addItem("docker-start-plan", true, dockerPlanOK, environmentRestoreReadinessDockerDetail(report))

	composeServices := stringSliceFromAny(report.Compose["services"])
	if strings.TrimSpace(valueString(report.Compose["composeFile"])) != "" {
		detail := "Docker Compose will start all services in the recorded file, including middleware images such as Apollo or MySQL when present"
		if len(composeServices) > 0 {
			detail = "Docker Compose service allow-list: " + strings.Join(composeServices, ", ")
		}
		addItem("compose-services-and-middleware", true, true, detail)
	}

	healthProbeCount := len(report.HealthChecks)
	addItem("health-probes", true, healthProbeCount > 0, fmt.Sprintf("%d Store-backed health probe(s) recorded for post-start readiness", healthProbeCount))

	cleanupOK := true
	cleanupDetail := "Docker cleanup not requested"
	if cleanupOptions.Requested || report.Docker.Cleanup.Requested {
		cleanupOK = report.Docker.Cleanup.Requested && len(report.Docker.Cleanup.BackupCommands) > 0 && len(report.Docker.Cleanup.Commands) > 0
		if report.Executed && !report.Docker.Cleanup.Allowed {
			cleanupOK = false
		}
		cleanupDetail = "Compose-scoped cleanup must be reviewed before simulating a clean colleague machine"
	}
	addItem("docker-cleanup-review", true, cleanupOK, cleanupDetail)

	workflowReady := strings.TrimSpace(report.VerificationWorkflow) != ""
	workflowDetail := "rerun with --execute --run-workflow --server-url URL after Docker health passes"
	if report.Workflow.Action == "run-acceptance-workflow" {
		workflowReady = report.Workflow.OK
		workflowDetail = "async acceptance report status: " + statusText(report.Workflow.OK)
	}
	addItem("workflow-run-gate", true, workflowReady, workflowDetail)
	addItem("operator-pause", true, true, "pause before deleting containers/images or running long image downloads for clean-machine validation")

	if !readiness.OK {
		readiness.Action = "fix-readiness-items-before-docker"
		readiness.NextStep = "fix failed readiness items before real clean-machine validation"
		return readiness
	}
	if report.Executed && report.Workflow.Action == "run-acceptance-workflow" && report.Workflow.OK {
		readiness.Action = "restore-executed-and-workflow-verified"
		readiness.NextStep = "publish only after the async acceptance report and verified discovery gates pass"
		return readiness
	}
	if report.Executed {
		readiness.Action = "ready-for-workflow-verification"
		readiness.NextStep = "run the anchored async environment acceptance workflow and collect Evidence/topology"
		return readiness
	}
	if cleanupOptions.AssumeCleanDocker {
		readiness.Action = "ready-for-clean-machine-execute"
		readiness.NextStep = "run the same restore on the colleague machine with --execute; this dry-run did not delete or reuse local Docker containers"
		return readiness
	}
	readiness.NextStep = "review the plan, then ask for operator approval before destructive Docker cleanup or image removal"
	return readiness
}

func environmentRestoreReadinessDockerDetail(report environmentRestoreReport) string {
	switch report.Docker.Action {
	case "plan-docker-compose", "run-docker-compose":
		if report.Docker.ComposeFile != "" {
			return "Docker Compose plan uses " + report.Docker.ComposeFile
		}
		return "Docker Compose plan is recorded"
	case "plan-start-command", "run-start-command":
		return "recorded start command will run from workspace"
	case "plan-use-existing-containers", "use-existing-containers":
		return "existing Docker containers are adopted; Docker Compose startup is skipped"
	case "skipped-due-to-repository-error":
		return "Docker startup is blocked until repository preparation succeeds"
	case "skipped-due-to-preflight":
		return "Docker startup is blocked until restore preflight succeeds"
	case "skipped-after-repository-preparation":
		return "repository preparation completed; Docker startup intentionally skipped"
	case "skipped-due-to-source-policy":
		return "Docker startup is blocked until package and component sources use remote Git URLs"
	case "missing-docker-plan":
		return "composeFile or startCommand is required"
	default:
		if strings.TrimSpace(report.Docker.Error) != "" {
			return report.Docker.Error
		}
		return "Docker startup plan is not ready"
	}
}

func environmentRestoreStoreStartupFilesReady(compose map[string]any) (bool, string) {
	composeFiles := environmentRestoreComposeFiles(compose)
	if len(composeFiles) == 0 {
		if strings.TrimSpace(valueString(compose["startCommand"])) != "" {
			return true, "restore uses a recorded start command; no compose startup file is required"
		}
		return false, "composeFile or startCommand is required"
	}
	generated := stringMapFromAny(compose["generatedFiles"])
	missing := []string{}
	for _, file := range composeFiles {
		clean := filepath.Clean(strings.TrimSpace(file))
		if _, ok := generated[clean]; !ok {
			missing = append(missing, file)
		}
	}
	if len(missing) > 0 {
		return false, "SQL Store restore must write compose startup files from compact Store metadata; missing generatedFiles for: " + strings.Join(missing, ", ")
	}
	return true, fmt.Sprintf("%d compose startup file(s) will be generated from Store metadata", len(composeFiles))
}

func environmentRestoreStartupAssetsReadiness(assets []environmentRestoreStartupAsset) (bool, string) {
	if len(assets) == 0 {
		return true, "no additional Compose startup assets are required for this restore path"
	}
	missing := []string{}
	for _, asset := range assets {
		if asset.OK {
			continue
		}
		missing = append(missing, asset.Path)
	}
	if len(missing) > 0 {
		return false, "missing Compose startup assets before Docker startup: " + strings.Join(missing, ", ")
	}
	return true, fmt.Sprintf("%d Compose startup asset(s) are available before Docker startup", len(assets))
}

func printEnvironmentRestoreReport(report environmentRestoreReport) {
	fmt.Printf("Environment Restore: %s\n", report.EnvironmentID)
	fmt.Printf("OK: %t\n", report.OK)
	fmt.Printf("Executed: %t\n", report.Executed)
	fmt.Printf("Workspace: %s\n", report.Workspace)
	fmt.Printf("Verification Workflow: %s\n", report.VerificationWorkflow)
	if report.RestoreID != "" {
		fmt.Printf("Restore ID: %s\n", report.RestoreID)
	}
	if report.Error != "" {
		fmt.Printf("Error: %s\n", report.Error)
	}
	if report.Readiness.Action != "" {
		fmt.Printf("Readiness: %s (ok=%t)\n", report.Readiness.Action, report.Readiness.OK)
		for _, item := range report.Readiness.Items {
			state := "ok"
			if !item.OK {
				state = "failed"
			}
			fmt.Printf("  %s: %s\n", item.Name, state)
			if item.Detail != "" {
				fmt.Printf("    %s\n", item.Detail)
			}
		}
		if report.Readiness.NextStep != "" {
			fmt.Printf("  next: %s\n", report.Readiness.NextStep)
		}
	}
	for _, repo := range report.Repos {
		state := repo.Action
		if !repo.OK {
			state = "failed"
		}
		fmt.Printf("- %s [%s]\n", repo.ServiceID, state)
		fmt.Printf("  checkout: %s\n", repo.Checkout)
		if repo.URL != "" {
			fmt.Printf("  repo: %s\n", repo.URL)
		}
		if repo.Branch != "" {
			fmt.Printf("  branch: %s\n", repo.Branch)
		}
		if repo.Error != "" {
			fmt.Printf("  error: %s\n", repo.Error)
		}
	}
	dockerState := report.Docker.Action
	if !report.Docker.OK {
		dockerState = "failed"
	}
	fmt.Printf("Docker: %s\n", dockerState)
	if report.Docker.ComposeFile != "" {
		fmt.Printf("  compose: %s\n", report.Docker.ComposeFile)
	}
	for _, command := range report.Docker.Commands {
		fmt.Printf("  command: %s\n", strings.Join(command, " "))
	}
	if report.Docker.Cleanup.Requested {
		fmt.Printf("  cleanup: %s\n", report.Docker.Cleanup.Action)
		if report.Docker.Cleanup.Warning != "" {
			fmt.Printf("    warning: %s\n", report.Docker.Cleanup.Warning)
		}
		for _, command := range report.Docker.Cleanup.BackupCommands {
			fmt.Printf("    backup: %s\n", strings.Join(command, " "))
		}
		for _, command := range report.Docker.Cleanup.Commands {
			fmt.Printf("    cleanup-command: %s\n", strings.Join(command, " "))
		}
		if report.Docker.Cleanup.Error != "" {
			fmt.Printf("    error: %s\n", report.Docker.Cleanup.Error)
		}
	}
	for _, check := range report.Docker.HealthChecks {
		state := "failed"
		if check.OK {
			state = "ok"
		}
		fmt.Printf("  health: %s [%s]\n", check.URL, state)
		if check.Error != "" {
			fmt.Printf("    error: %s\n", check.Error)
		}
	}
	if report.Docker.Error != "" {
		fmt.Printf("  error: %s\n", report.Docker.Error)
	}
	fmt.Printf("Workflow: %s [%s]\n", report.Workflow.WorkflowID, report.Workflow.Action)
	if report.Workflow.RunID != "" {
		fmt.Printf("  run: %s\n", report.Workflow.RunID)
	}
	if report.Workflow.OutputDir != "" {
		fmt.Printf("  report: %s\n", report.Workflow.OutputDir)
	}
	if report.Workflow.Error != "" {
		fmt.Printf("  error: %s\n", report.Workflow.Error)
	}
	for _, action := range report.NextActions {
		fmt.Printf("Next: %s\n", action)
	}
}
