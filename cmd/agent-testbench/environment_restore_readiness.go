package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

func environmentRestoreReadinessReport(report environmentRestoreReport, packageSpec environmentRestorePackageSpec, specs []environmentRestoreRepoSpec, cleanupOptions environmentRestoreDockerCleanupOptions) environmentRestoreReadiness {
	readiness := environmentRestoreReadiness{
		OK:                         true,
		Action:                     "ready-for-operator-review",
		PauseBeforeHeavyValidation: true,
	}
	builder := environmentRestoreReadinessBuilder{readiness: &readiness}
	builder.add("store-boundary", true, true, "sandbox SQL Store must stay outside the restored Docker target environment")
	builder.add("verification-workflow", true, strings.TrimSpace(report.VerificationWorkflow) != "", "restore is anchored to workflow "+strings.TrimSpace(report.VerificationWorkflow))
	environmentRestoreAddComponentReadiness(&builder, report)
	environmentRestoreAddContainerConflictReadiness(&builder, report, cleanupOptions)
	environmentRestoreAddSourceReadiness(&builder, report, packageSpec)
	startupAssetsOK, startupAssetsDetail := environmentRestoreStartupAssetsReadiness(report.Preflight.StartupAssets)
	builder.add("startup-assets", true, startupAssetsOK, startupAssetsDetail)
	environmentRestoreAddRepositoryReadiness(&builder, report, specs)
	dockerPlanOK := environmentRestoreDockerPlanReady(report)
	builder.add("docker-start-plan", true, dockerPlanOK, environmentRestoreReadinessDockerDetail(report))
	environmentRestoreAddComposeServiceReadiness(&builder, report)
	healthProbeCount := len(report.HealthChecks)
	builder.add("health-probes", true, healthProbeCount > 0, fmt.Sprintf("%d Store-backed health probe(s) recorded for post-start readiness", healthProbeCount))
	environmentRestoreAddCleanupReadiness(&builder, report, cleanupOptions)
	environmentRestoreAddWorkflowReadiness(&builder, report)
	builder.add("operator-pause", true, true, "pause before deleting containers/images or running long image downloads for clean-machine validation")
	environmentRestoreFinalizeReadiness(&readiness, report, cleanupOptions)
	return readiness
}

type environmentRestoreReadinessBuilder struct {
	readiness *environmentRestoreReadiness
}

func (b environmentRestoreReadinessBuilder) add(name string, required bool, ok bool, detail string) {
	b.readiness.Items = append(b.readiness.Items, environmentRestoreReadinessItem{
		Name:     name,
		Required: required,
		OK:       ok,
		Detail:   detail,
	})
	if required && !ok {
		b.readiness.OK = false
	}
}

func environmentRestoreAddComponentReadiness(builder *environmentRestoreReadinessBuilder, report environmentRestoreReport) {
	if report.ComponentGraph.Configured {
		detail := fmt.Sprintf("%d component(s), %d blocking dependency edge(s), %d runtime edge(s), %d asset(s), %d inline asset bytes, %d remote asset(s)",
			report.ComponentGraph.Components, report.ComponentGraph.BlockingDependencies, report.ComponentGraph.RuntimeDependencies,
			report.ComponentGraph.Assets, report.ComponentGraph.InlineAssetBytes, report.ComponentGraph.RemoteAssets)
		if strings.TrimSpace(report.ComponentGraph.Error) != "" {
			detail = report.ComponentGraph.Error
		}
		builder.add("component-graph", true, report.ComponentGraph.OK, detail)
		environmentRestoreAddStartupPlanReadiness(builder, report)
		return
	}
	if report.SourcePolicy.RemoteOnly {
		builder.add("component-graph", true, false, "SQL Store one-click Docker restore requires a Store component graph for services, middleware, mocks, observability, dependencies, assets, and health gates")
		return
	}
	builder.add("component-graph", false, true, "no Store component graph recorded yet; restore will use legacy service and compose metadata")
}

func environmentRestoreAddStartupPlanReadiness(builder *environmentRestoreReadinessBuilder, report environmentRestoreReport) {
	detail := fmt.Sprintf("%d startup batch(es), %d health gate(s)", len(report.ComponentStartupPlan.Batches), len(report.ComponentStartupPlan.HealthGates))
	if strings.TrimSpace(report.ComponentStartupPlan.Error) != "" {
		detail = report.ComponentStartupPlan.Error
	}
	builder.add("component-startup-plan", true, report.ComponentStartupPlan.OK, detail)
}

func environmentRestoreAddContainerConflictReadiness(builder *environmentRestoreReadinessBuilder, report environmentRestoreReport, cleanupOptions environmentRestoreDockerCleanupOptions) {
	switch {
	case len(report.Preflight.ContainerConflicts) > 0:
		builder.add("docker-container-conflicts", true, false, "existing Docker containers would be reused or replaced by fixed container_name values: "+strings.Join(report.Preflight.ContainerConflicts, ", "))
	case cleanupOptions.AssumeCleanDocker:
		builder.add("docker-container-conflicts", true, true, "clean-machine dry-run assumes target Docker containers are absent; no local Docker deletion was performed")
	case cleanupOptions.UseExistingContainers:
		builder.add("docker-container-conflicts", true, true, "existing fixed-name Docker containers are explicitly adopted; Docker Compose up will not run")
	case strings.TrimSpace(valueString(report.Compose["composeFile"])) != "":
		builder.add("docker-container-conflicts", true, true, "no existing Docker container_name conflicts detected for non-destructive restore")
	}
}

func environmentRestoreAddSourceReadiness(builder *environmentRestoreReadinessBuilder, report environmentRestoreReport, packageSpec environmentRestorePackageSpec) {
	if report.SourcePolicy.RemoteOnly {
		detail := "all component source repositories must be remote Git URLs for SQL Store-backed one-click environments; environment startup files come from compact Store metadata"
		if len(report.SourcePolicy.Violations) > 0 {
			detail = strings.Join(report.SourcePolicy.Violations, "; ")
		}
		builder.add("remote-git-sources", true, report.SourcePolicy.OK, detail)
		ok, startupDetail := environmentRestoreStoreStartupFilesReady(report)
		builder.add("store-startup-files", true, ok, startupDetail)
	}
	if strings.TrimSpace(packageSpec.URL) != "" {
		detail := "environment package will be cloned or validated before Docker startup"
		if report.Package.Action != "" {
			detail = "environment package " + report.Package.Action + " at " + report.Package.Checkout
		}
		builder.add("environment-package", true, report.Package.OK, detail)
	}
}

func environmentRestoreAddRepositoryReadiness(builder *environmentRestoreReadinessBuilder, report environmentRestoreReport, specs []environmentRestoreRepoSpec) {
	repoOK := true
	for _, item := range report.Repos {
		if !item.OK {
			repoOK = false
			break
		}
	}
	switch {
	case len(specs) == 0:
		builder.add("component-repositories", true, true, "no component repositories recorded; Docker uses the recorded compose/start plan and existing local context")
	case report.Executed:
		builder.add("component-repositories", true, repoOK, fmt.Sprintf("%d component repository checkout(s) prepared before Docker startup", len(specs)))
	default:
		builder.add("component-repositories", true, repoOK, fmt.Sprintf("%d component repository checkout(s) will be cloned or validated before Docker startup", len(specs)))
	}
}

func environmentRestoreAddComposeServiceReadiness(builder *environmentRestoreReadinessBuilder, report environmentRestoreReport) {
	composeFiles := environmentRestoreComposeFiles(report.Compose)
	if len(composeFiles) == 0 {
		return
	}
	ok, detail := environmentRestoreComposeServiceReadinessDetail(report, composeFiles)
	builder.add("compose-services-and-middleware", true, ok, detail)
}

func environmentRestoreComposeServiceReadinessDetail(report environmentRestoreReport, composeFiles []string) (bool, string) {
	required := environmentRestoreRequiredComposeServices(report)
	selected := dedupeStrings(stringSliceFromAny(report.Compose["services"]))
	known, _, inspected := environmentRestoreComposeServiceDefinitions(report.Compose, report.Workspace, composeFiles)
	missingSelected := []string{}
	if len(selected) > 0 {
		missingSelected = environmentRestoreMissingStrings(required, environmentRestoreStringSet(selected))
	}
	missingDefinitions := []string{}
	if inspected {
		missingDefinitions = environmentRestoreMissingStrings(required, known)
	}
	if len(missingSelected) > 0 || len(missingDefinitions) > 0 {
		details := []string{}
		if len(required) > 0 {
			details = append(details, "component graph requires Compose services: "+strings.Join(required, ", "))
		}
		if len(missingSelected) > 0 {
			details = append(details, "missing from recorded compose service allow-list: "+strings.Join(missingSelected, ", "))
		}
		if len(missingDefinitions) > 0 {
			details = append(details, "missing from recorded compose file definitions: "+strings.Join(missingDefinitions, ", "))
		}
		details = append(details, "update the Store compose startup file and compose service allow-list before rerunning environment restore")
		return false, strings.Join(details, "; ")
	}
	if len(selected) > 0 {
		detail := "Docker Compose service allow-list covers required component services, including middleware: " + strings.Join(selected, ", ")
		if len(required) > 0 {
			detail = fmt.Sprintf("Docker Compose service allow-list covers %d required component service(s), including middleware: %s", len(required), strings.Join(selected, ", "))
		}
		return true, detail
	}
	detail := "Docker Compose will start all services in the recorded file, including middleware images such as Apollo or MySQL when present"
	if len(required) == 0 {
		return true, detail
	}
	if inspected {
		return true, fmt.Sprintf("%s; compose files define %d required component service(s): %s", detail, len(required), strings.Join(required, ", "))
	}
	return true, fmt.Sprintf("%s; %d required component service(s) will be checked once compose files are generated or present", detail, len(required))
}

func environmentRestoreRequiredComposeServices(report environmentRestoreReport) []string {
	seen := map[string]bool{}
	for _, batch := range report.ComponentStartupPlan.Batches {
		for _, component := range batch.Components {
			service := strings.TrimSpace(component.ComposeService)
			if !component.Required || service == "" {
				continue
			}
			seen[service] = true
		}
	}
	out := make([]string, 0, len(seen))
	for service := range seen {
		out = append(out, service)
	}
	sort.Strings(out)
	return out
}

func environmentRestoreStringSet(values []string) map[string]bool {
	out := map[string]bool{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			out[value] = true
		}
	}
	return out
}

func environmentRestoreMissingStrings(required []string, available map[string]bool) []string {
	missing := []string{}
	for _, value := range required {
		if !available[value] {
			missing = append(missing, value)
		}
	}
	return missing
}

func environmentRestoreAddCleanupReadiness(builder *environmentRestoreReadinessBuilder, report environmentRestoreReport, cleanupOptions environmentRestoreDockerCleanupOptions) {
	cleanupOK := true
	cleanupDetail := "Docker cleanup not requested"
	if cleanupOptions.Requested || report.Docker.Cleanup.Requested {
		cleanupOK = report.Docker.Cleanup.Requested && len(report.Docker.Cleanup.BackupCommands) > 0 && len(report.Docker.Cleanup.Commands) > 0
		if report.Docker.Cleanup.Linkage.Error != "" {
			cleanupOK = false
			cleanupDetail = report.Docker.Cleanup.Linkage.Error
		}
		if report.Executed && !report.Docker.Cleanup.Allowed {
			cleanupOK = false
		}
		if cleanupDetail == "Docker cleanup not requested" {
			cleanupDetail = "Compose-scoped cleanup must be reviewed before simulating a clean colleague machine"
		}
	}
	builder.add("docker-cleanup-review", true, cleanupOK, cleanupDetail)
}

func environmentRestoreAddWorkflowReadiness(builder *environmentRestoreReadinessBuilder, report environmentRestoreReport) {
	workflowReady := strings.TrimSpace(report.VerificationWorkflow) != ""
	workflowDetail := "rerun with --execute --run-workflow --server-url URL after Docker health passes"
	if report.Workflow.Action == "run-acceptance-workflow" {
		workflowReady = report.Workflow.OK
		workflowDetail = "async acceptance report status: " + statusText(report.Workflow.OK)
	}
	builder.add("workflow-run-gate", true, workflowReady, workflowDetail)
}

func environmentRestoreFinalizeReadiness(readiness *environmentRestoreReadiness, report environmentRestoreReport, cleanupOptions environmentRestoreDockerCleanupOptions) {
	switch {
	case !readiness.OK:
		readiness.Action = "fix-readiness-items-before-docker"
		readiness.NextStep = "fix failed readiness items before real clean-machine validation"
	case report.Executed && report.Workflow.Action == "run-acceptance-workflow" && report.Workflow.OK:
		readiness.Action = "restore-executed-and-workflow-verified"
		readiness.NextStep = "publish only after the async acceptance report and verified discovery gates pass"
	case report.Executed:
		readiness.Action = "ready-for-workflow-verification"
		readiness.NextStep = "run the anchored async environment acceptance workflow and collect Evidence/topology"
	case cleanupOptions.AssumeCleanDocker:
		readiness.Action = "ready-for-clean-machine-execute"
		readiness.NextStep = "run the same restore on the colleague machine with --execute; this dry-run did not delete or reuse local Docker containers"
	default:
		readiness.NextStep = "review the plan, then ask for operator approval before destructive Docker cleanup or image removal"
	}
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

func environmentRestoreDockerPlanReady(report environmentRestoreReport) bool {
	switch report.Docker.Action {
	case "plan-docker-compose", "run-docker-compose":
		return report.Docker.OK && (!report.Executed || environmentRestoreComposeFilesReady(report))
	case "plan-start-command", "run-start-command", "plan-use-existing-containers", "use-existing-containers", "skipped-after-repository-preparation":
		return report.Docker.OK
	default:
		return false
	}
}

func environmentRestoreComposeFilesReady(report environmentRestoreReport) bool {
	composeFileList := strings.TrimSpace(report.Docker.ComposeFile)
	if composeFileList == "" {
		return false
	}
	for _, composeFile := range strings.Split(composeFileList, ",") {
		composeFile = strings.TrimSpace(composeFile)
		if composeFile == "" {
			continue
		}
		if environmentRestoreGeneratedFileReady(report.Docker.Generated, composeFile) {
			continue
		}
		if stat, err := os.Stat(composeFile); err == nil && !stat.IsDir() {
			continue
		} else {
			return false
		}
	}
	return true
}

func environmentRestoreGeneratedFileReady(generatedFiles []environmentRestoreGeneratedFile, path string) bool {
	target := filepath.Clean(strings.TrimSpace(path))
	if target == "" || target == "." {
		return false
	}
	for _, generated := range generatedFiles {
		if generated.OK && filepath.Clean(generated.Path) == target {
			return true
		}
	}
	return false
}

func environmentRestoreStoreStartupFilesReady(report environmentRestoreReport) (bool, string) {
	composeFiles := environmentRestoreComposeFiles(report.Compose)
	if len(composeFiles) == 0 {
		if strings.TrimSpace(valueString(report.Compose["startCommand"])) != "" {
			return true, "restore uses a recorded start command; no compose startup file is required"
		}
		return false, "composeFile or startCommand is required"
	}
	generated := stringMapFromAny(report.Compose["generatedFiles"])
	missing := []string{}
	for _, file := range composeFiles {
		clean := filepath.Clean(strings.TrimSpace(file))
		if _, ok := generated[clean]; ok {
			continue
		}
		if environmentRestoreWorkspaceStartupFileExists(report, clean) {
			continue
		}
		missing = append(missing, file)
	}
	if len(missing) > 0 {
		return false, "SQL Store restore must write compose startup files from compact Store metadata; missing generatedFiles for: " + strings.Join(missing, ", ")
	}
	return true, fmt.Sprintf("%d compose startup file(s) will be generated from Store metadata", len(composeFiles))
}

func environmentRestoreWorkspaceStartupFileExists(report environmentRestoreReport, cleanPath string) bool {
	target := restoreWorkspacePath(report.Workspace, cleanPath)
	if target == "" {
		return false
	}
	for _, generated := range report.Docker.Generated {
		if filepath.Clean(generated.Path) == filepath.Clean(target) && generated.OK {
			return true
		}
	}
	return false
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
