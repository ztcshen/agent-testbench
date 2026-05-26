package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"agent-testbench/internal/store"
)

func environmentRestoreContainerNameConflicts(compose map[string]any, workspace string) []string {
	wanted := environmentRestoreContainerNames(compose, workspace)
	if len(wanted) == 0 {
		return nil
	}
	path, err := exec.LookPath("docker")
	if err != nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, path, "ps", "-a", "--format", "{{.Names}}").CombinedOutput()
	if err != nil {
		return nil
	}
	existing := map[string]bool{}
	for _, line := range strings.Split(string(out), "\n") {
		name := strings.TrimSpace(line)
		if name != "" {
			existing[name] = true
		}
	}
	conflicts := []string{}
	for _, name := range wanted {
		if existing[name] {
			conflicts = append(conflicts, name)
		}
	}
	sort.Strings(conflicts)
	return conflicts
}

func environmentRestoreContainerNames(compose map[string]any, workspace string) []string {
	byService := environmentRestoreContainerNameByService(compose, workspace)
	names := make([]string, 0, len(byService))
	for _, name := range byService {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func environmentRestoreContainerNameByService(compose map[string]any, workspace string) map[string]string {
	out := map[string]string{}
	addContent := func(content string) {
		for service, container := range parseComposeContainerNames(content) {
			out[service] = container
		}
	}
	for _, content := range stringMapFromAny(compose["generatedFiles"]) {
		addContent(content)
	}
	for _, file := range environmentRestoreComposeFiles(compose) {
		path := restoreWorkspacePath(workspace, file)
		raw, err := os.ReadFile(path)
		if err == nil {
			addContent(string(raw))
		}
	}
	return out
}

func parseComposeContainerNames(content string) map[string]string {
	out := map[string]string{}
	inServices := false
	currentService := ""
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		indent := len(line) - len(strings.TrimLeft(line, " "))
		if indent == 0 {
			inServices = trimmed == "services:"
			currentService = ""
			continue
		}
		if !inServices {
			continue
		}
		if indent == 2 && strings.HasSuffix(trimmed, ":") {
			currentService = strings.TrimSuffix(trimmed, ":")
			continue
		}
		if currentService == "" || !strings.HasPrefix(trimmed, "container_name:") {
			continue
		}
		name := strings.TrimSpace(strings.TrimPrefix(trimmed, "container_name:"))
		name = strings.Trim(name, `"'`)
		if name != "" {
			out[currentService] = name
		}
	}
	return out
}

func environmentRestorePreflightReport(packageSpec environmentRestorePackageSpec, specs []environmentRestoreRepoSpec, compose map[string]any, workspace string, cleanupOptions environmentRestoreDockerCleanupOptions, prepareReposOnly bool) environmentRestorePreflight {
	report := environmentRestorePreflight{
		OK:                true,
		AssumeCleanDocker: cleanupOptions.AssumeCleanDocker,
		Notes: []string{
			"Sandbox control-plane Store must already be reachable outside restored Docker target services.",
			"Heavy Docker image and container validation should be reviewed before deleting or rebuilding existing local Docker state.",
		},
	}
	if environmentRestorePreflightRequiresGit(packageSpec, specs) {
		report.Tools = append(report.Tools, environmentRestoreTool("git", true))
	}
	composeFile := strings.TrimSpace(valueString(compose["composeFile"]))
	startCommand := strings.TrimSpace(valueString(compose["startCommand"]))
	if composeFile != "" {
		environmentRestoreAddComposePreflight(&report, compose, specs, workspace, cleanupOptions, prepareReposOnly)
	} else if startCommand != "" {
		report.HeavySteps = append(report.HeavySteps, "start command may create local runtime processes or containers")
	}
	for _, tool := range report.Tools {
		if tool.Required && !tool.OK {
			report.OK = false
		}
	}
	return report
}

func environmentRestorePreflightRequiresGit(packageSpec environmentRestorePackageSpec, specs []environmentRestoreRepoSpec) bool {
	if strings.TrimSpace(packageSpec.URL) != "" || strings.TrimSpace(packageSpec.Ref) != "" {
		return true
	}
	for _, spec := range specs {
		if strings.TrimSpace(spec.URL) != "" || strings.TrimSpace(spec.Ref) != "" {
			return true
		}
	}
	return false
}

func environmentRestoreAddComposePreflight(report *environmentRestorePreflight, compose map[string]any, specs []environmentRestoreRepoSpec, workspace string, cleanupOptions environmentRestoreDockerCleanupOptions, prepareReposOnly bool) {
	report.Tools = append(report.Tools, environmentRestoreTool("docker", true))
	report.Tools = append(report.Tools, environmentRestoreCommandTool("docker compose", true, "docker", "compose", "version"))
	report.HeavySteps = append(report.HeavySteps, environmentRestoreComposeHeavySteps(compose, cleanupOptions)...)
	environmentRestoreCheckContainerConflicts(report, compose, workspace, cleanupOptions, prepareReposOnly)
	environmentRestoreCheckStartupAssets(report, compose, specs, workspace, cleanupOptions, prepareReposOnly)
	for _, file := range environmentRestoreComposeFiles(compose) {
		if resolved := restoreWorkspacePath(workspace, file); strings.TrimSpace(resolved) != "" {
			report.Notes = append(report.Notes, "compose file must exist before Docker execution: "+resolved)
		}
	}
}

func environmentRestoreComposeHeavySteps(compose map[string]any, cleanupOptions environmentRestoreDockerCleanupOptions) []string {
	steps := []string{}
	if !boolFromReportAny(compose["skipPull"]) {
		steps = append(steps, "docker compose pull may download images")
	}
	if !boolFromReportAny(compose["skipBuild"]) {
		steps = append(steps, "docker compose build may build images from local checkouts")
	}
	steps = append(steps, "docker compose up -d may create or replace containers")
	if cleanupOptions.Requested {
		steps = append(steps, "docker compose down may remove existing containers and orphan containers")
		if cleanupOptions.IncludeImages {
			steps = append(steps, "docker compose down --rmi all may remove local images")
		}
	}
	return steps
}

func environmentRestoreCheckContainerConflicts(report *environmentRestorePreflight, compose map[string]any, workspace string, cleanupOptions environmentRestoreDockerCleanupOptions, prepareReposOnly bool) {
	switch {
	case cleanupOptions.Requested:
		return
	case cleanupOptions.AssumeCleanDocker:
		report.Notes = append(report.Notes, "Clean-machine dry-run assumes target Docker containers do not exist on the colleague machine; current local container names are not treated as blockers.")
	case prepareReposOnly || cleanupOptions.UseExistingContainers:
		return
	default:
		report.ContainerConflicts = environmentRestoreContainerNameConflicts(compose, workspace)
		if len(report.ContainerConflicts) > 0 {
			report.OK = false
		}
	}
}

func environmentRestoreCheckStartupAssets(report *environmentRestorePreflight, compose map[string]any, specs []environmentRestoreRepoSpec, workspace string, cleanupOptions environmentRestoreDockerCleanupOptions, prepareReposOnly bool) {
	if prepareReposOnly || cleanupOptions.UseExistingContainers {
		return
	}
	report.StartupAssets = environmentRestoreStartupAssets(compose, specs, workspace)
	for _, asset := range report.StartupAssets {
		if !asset.OK {
			report.OK = false
		}
	}
}

func environmentRestoreTool(name string, required bool) environmentRestorePreflightTool {
	tool := environmentRestorePreflightTool{Name: name, Required: required}
	path, err := exec.LookPath(name)
	if err != nil {
		tool.OK = false
		tool.Error = err.Error()
		return tool
	}
	tool.OK = true
	tool.Path = path
	return tool
}

func environmentRestoreCommandTool(name string, required bool, command string, args ...string) environmentRestorePreflightTool {
	tool := environmentRestorePreflightTool{Name: name, Required: required}
	path, err := exec.LookPath(command)
	if err != nil {
		tool.OK = false
		tool.Error = err.Error()
		return tool
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, path, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		tool.OK = false
		tool.Path = path
		tool.Error = strings.TrimSpace(string(out))
		if tool.Error == "" {
			tool.Error = err.Error()
		}
		return tool
	}
	tool.OK = true
	tool.Path = path
	return tool
}

func environmentRestoreUseExistingContainers(ctx context.Context, graph store.EnvironmentComponentGraph, compose map[string]any, healthChecks []any, workspace string, execute bool, healthTimeout time.Duration) environmentRestoreDockerReport {
	report := environmentRestoreDockerReport{
		OK:          true,
		Action:      "plan-use-existing-containers",
		Workdir:     workspace,
		ComposeFile: strings.Join(environmentRestoreResolvedComposeFiles(workspace, environmentRestoreComposeFiles(compose)), ","),
		Generated:   prepareEnvironmentRestoreGeneratedFiles(compose, workspace, execute),
	}
	composeBaseArgs := []string{}
	if report.ComposeFile != "" {
		composeBaseArgs = environmentRestoreComposeBaseArgs(compose, workspace, environmentRestoreResolvedComposeFiles(workspace, environmentRestoreComposeFiles(compose)))
	}
	for _, item := range report.Generated {
		if !item.OK {
			report.OK = false
			report.Action = "prepare-generated-files"
			report.Error = item.Error
			return report
		}
	}
	if !execute {
		return report
	}
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		report.OK = false
		report.Action = "prepare-workspace"
		report.Error = err.Error()
		return report
	}
	if envFile, err := writeEnvironmentRestoreGeneratedEnvFile(workspace, compose); err != nil {
		report.OK = false
		report.Action = "prepare-compose-env"
		report.Error = err.Error()
		return report
	} else if envFile != "" {
		report.Output = append(report.Output, "generated compose env file: "+envFile)
	}
	report.Action = "use-existing-containers"
	report.AppliedAssets = environmentRestoreApplyEdgeAssets(ctx, graph, compose, workspace, execute, composeBaseArgs)
	for _, asset := range report.AppliedAssets {
		if !asset.OK {
			report.OK = false
			report.Error = asset.Error
			return report
		}
	}
	report.HealthChecks = waitEnvironmentRestoreHealthChecks(ctx, environmentRestoreAdoptedContainerHealthChecks(healthChecks, compose, workspace), healthTimeout, workspace, nil)
	for _, check := range report.HealthChecks {
		if !check.OK {
			report.OK = false
		}
	}
	return report
}

func environmentRestoreAdoptedContainerHealthChecks(checks []any, compose map[string]any, workspace string) []any {
	containers := environmentRestoreContainerNameByService(compose, workspace)
	out := make([]any, 0, len(checks))
	for _, raw := range checks {
		item, ok := raw.(map[string]any)
		if !ok || strings.TrimSpace(valueString(item["kind"])) != "compose-service" {
			out = append(out, raw)
			continue
		}
		service := strings.TrimSpace(valueString(item["service"]))
		container := strings.TrimSpace(containers[service])
		if service == "" || container == "" {
			out = append(out, raw)
			continue
		}
		converted := map[string]any{}
		for key, value := range item {
			converted[key] = value
		}
		converted["kind"] = "container"
		converted["container"] = container
		out = append(out, converted)
	}
	return out
}

func environmentRestoreDocker(ctx context.Context, graph store.EnvironmentComponentGraph, compose map[string]any, healthChecks []any, workspace string, execute bool, healthTimeout time.Duration, cleanupOptions environmentRestoreDockerCleanupOptions) environmentRestoreDockerReport {
	report, composeBaseArgs := environmentRestoreDockerPlan(compose, workspace, cleanupOptions)
	if !report.OK {
		return report
	}
	environmentRestoreCheckGeneratedFiles(&report, compose, workspace, false)
	if !execute {
		return report
	}
	if !environmentRestorePrepareDockerExecution(&report, compose, workspace) {
		return report
	}
	if !environmentRestoreValidateComposeFiles(&report) {
		return report
	}
	if !environmentRestoreRunCleanup(ctx, &report, workspace) {
		return report
	}
	environmentRestoreMarkDockerExecuting(&report)
	if !environmentRestoreRunCommands(ctx, &report, workspace) {
		return report
	}
	report.AppliedAssets = environmentRestoreApplyEdgeAssets(ctx, graph, compose, workspace, execute, composeBaseArgs)
	for _, asset := range report.AppliedAssets {
		if !asset.OK {
			report.OK = false
			report.Error = asset.Error
			return report
		}
	}
	report.HealthChecks = waitEnvironmentRestoreHealthChecks(ctx, healthChecks, healthTimeout, workspace, composeBaseArgs)
	for _, check := range report.HealthChecks {
		if !check.OK {
			report.OK = false
		}
	}
	return report
}

func environmentRestoreDockerPlan(compose map[string]any, workspace string, cleanupOptions environmentRestoreDockerCleanupOptions) (environmentRestoreDockerReport, []string) {
	report := environmentRestoreDockerReport{OK: true, Workdir: workspace}
	composeFiles := environmentRestoreComposeFiles(compose)
	startCommand := strings.TrimSpace(valueString(compose["startCommand"]))
	if strings.TrimSpace(valueString(compose["composeFile"])) != "" {
		baseArgs := environmentRestorePlanComposeCommands(&report, compose, workspace, composeFiles, cleanupOptions)
		return report, baseArgs
	}
	if startCommand != "" {
		return environmentRestorePlanStartCommand(workspace, startCommand, cleanupOptions), nil
	}
	report.OK = false
	report.Action = "missing-docker-plan"
	report.Error = "composeFile or startCommand is required to restore Docker services"
	return report, nil
}

func environmentRestorePlanComposeCommands(report *environmentRestoreDockerReport, compose map[string]any, workspace string, composeFiles []string, cleanupOptions environmentRestoreDockerCleanupOptions) []string {
	report.Action = "plan-docker-compose"
	resolvedComposeFiles := environmentRestoreResolvedComposeFiles(workspace, composeFiles)
	report.ComposeFile = strings.Join(resolvedComposeFiles, ",")
	baseArgs := environmentRestoreComposeBaseArgs(compose, workspace, resolvedComposeFiles)
	services := stringSliceFromAny(compose["services"])
	report.Cleanup = environmentRestoreDockerCleanupPlan(baseArgs, cleanupOptions)
	imageServices, buildServices := environmentRestoreComposeCommandServices(compose, workspace, composeFiles, services)
	if !boolFromReportAny(compose["skipPull"]) && len(imageServices) > 0 {
		report.Commands = append(report.Commands, append(append([]string{"docker", "compose"}, baseArgs...), append([]string{"pull"}, imageServices...)...))
	}
	if !boolFromReportAny(compose["skipBuild"]) && len(buildServices) > 0 {
		report.Commands = append(report.Commands, append(append([]string{"docker", "compose"}, baseArgs...), append([]string{"build"}, buildServices...)...))
	}
	report.Commands = append(report.Commands, append(append([]string{"docker", "compose"}, baseArgs...), append([]string{"up", "-d"}, services...)...))
	return baseArgs
}

func environmentRestorePlanStartCommand(workspace string, startCommand string, cleanupOptions environmentRestoreDockerCleanupOptions) environmentRestoreDockerReport {
	report := environmentRestoreDockerReport{
		OK:       true,
		Workdir:  workspace,
		Action:   "plan-start-command",
		Commands: [][]string{{"/bin/sh", "-c", startCommand}},
	}
	if cleanupOptions.Requested {
		report.OK = false
		report.Cleanup = environmentRestoreDockerCleanupReport{
			Requested:     true,
			Allowed:       cleanupOptions.Allowed,
			IncludeImages: cleanupOptions.IncludeImages,
			Action:        "unsupported-cleanup",
			Error:         "Docker cleanup requires a recorded composeFile",
		}
		report.Error = report.Cleanup.Error
	}
	return report
}

func environmentRestoreCheckGeneratedFiles(report *environmentRestoreDockerReport, compose map[string]any, workspace string, execute bool) bool {
	report.Generated = prepareEnvironmentRestoreGeneratedFiles(compose, workspace, execute)
	for _, item := range report.Generated {
		if !item.OK {
			report.OK = false
			report.Action = "prepare-generated-files"
			report.Error = item.Error
			return false
		}
	}
	return true
}

func environmentRestorePrepareDockerExecution(report *environmentRestoreDockerReport, compose map[string]any, workspace string) bool {
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		report.OK = false
		report.Action = "prepare-workspace"
		report.Error = err.Error()
		return false
	}
	if !environmentRestoreCheckGeneratedFiles(report, compose, workspace, true) {
		return false
	}
	envFile, err := writeEnvironmentRestoreGeneratedEnvFile(workspace, compose)
	if err != nil {
		report.OK = false
		report.Action = "prepare-compose-env"
		report.Error = err.Error()
		return false
	}
	if envFile != "" {
		report.Output = append(report.Output, "generated compose env file: "+envFile)
	}
	return true
}

func environmentRestoreValidateComposeFiles(report *environmentRestoreDockerReport) bool {
	if report.ComposeFile == "" {
		return true
	}
	for _, composeFile := range strings.Split(report.ComposeFile, ",") {
		composeFile = strings.TrimSpace(composeFile)
		if composeFile == "" {
			continue
		}
		if stat, err := os.Stat(composeFile); err != nil {
			report.OK = false
			report.Action = "missing-compose-file"
			report.Error = fmt.Sprintf("compose file is required before Docker execution: %s", composeFile)
			return false
		} else if stat.IsDir() {
			report.OK = false
			report.Action = "invalid-compose-file"
			report.Error = fmt.Sprintf("compose file path is a directory: %s", composeFile)
			return false
		}
	}
	return true
}

func environmentRestoreRunCleanup(ctx context.Context, report *environmentRestoreDockerReport, workspace string) bool {
	if report.ComposeFile == "" || !report.Cleanup.Requested {
		return true
	}
	if !report.Cleanup.Allowed {
		report.OK = false
		report.Cleanup.Action = "cleanup-blocked"
		report.Cleanup.Error = "Docker cleanup requested during --execute; rerun with --allow-destructive-docker-cleanup after reviewing cleanup commands"
		report.Error = report.Cleanup.Error
		return false
	}
	report.Cleanup.Action = "run-cleanup"
	for _, command := range append(report.Cleanup.BackupCommands, report.Cleanup.Commands...) {
		output, errText := runRestoreCommand(ctx, workspace, command)
		if strings.TrimSpace(output) != "" {
			report.Cleanup.Output = append(report.Cleanup.Output, output)
		}
		if errText != "" {
			report.OK = false
			report.Cleanup.Error = errText
			report.Error = errText
			return false
		}
	}
	return true
}

func environmentRestoreMarkDockerExecuting(report *environmentRestoreDockerReport) {
	if report.Action == "plan-docker-compose" {
		report.Action = "run-docker-compose"
		return
	}
	report.Action = "run-start-command"
}

func environmentRestoreRunCommands(ctx context.Context, report *environmentRestoreDockerReport, workspace string) bool {
	for _, command := range report.Commands {
		output, errText := runRestoreCommand(ctx, workspace, command)
		if strings.TrimSpace(output) != "" {
			report.Output = append(report.Output, output)
		}
		if errText != "" {
			report.OK = false
			report.Error = errText
			return false
		}
	}
	return true
}

func environmentRestoreDockerCleanupPlan(baseArgs []string, options environmentRestoreDockerCleanupOptions) environmentRestoreDockerCleanupReport {
	if !options.Requested {
		return environmentRestoreDockerCleanupReport{}
	}
	cleanup := environmentRestoreDockerCleanupReport{
		Requested:     true,
		Allowed:       options.Allowed,
		IncludeImages: options.IncludeImages,
		Action:        "plan-cleanup",
		Warning:       "Review Docker cleanup commands before simulating a clean colleague machine; the sandbox SQL Store must remain outside these Docker target services.",
	}
	cleanup.BackupCommands = [][]string{
		append(append([]string{"docker", "compose"}, baseArgs...), "ps"),
		append(append([]string{"docker", "compose"}, baseArgs...), "images"),
		append(append([]string{"docker", "compose"}, baseArgs...), "config"),
	}
	down := append(append([]string{"docker", "compose"}, baseArgs...), "down", "--remove-orphans")
	if options.IncludeImages {
		down = append(down, "--rmi", "all")
	}
	cleanup.Commands = [][]string{down}
	return cleanup
}

func environmentRestoreComposeFiles(compose map[string]any) []string {
	files := stringSliceFromAny(compose["composeFiles"])
	if len(files) == 0 {
		if file := strings.TrimSpace(valueString(compose["composeFile"])); file != "" {
			files = []string{file}
		}
	}
	return files
}

func environmentRestoreResolvedComposeFiles(workspace string, files []string) []string {
	out := make([]string, 0, len(files))
	for _, file := range files {
		if resolved := restoreWorkspacePath(workspace, file); strings.TrimSpace(resolved) != "" {
			out = append(out, resolved)
		}
	}
	return out
}

func prepareEnvironmentRestoreGeneratedFiles(compose map[string]any, workspace string, execute bool) []environmentRestoreGeneratedFile {
	files := stringMapFromAny(compose["generatedFiles"])
	if len(files) == 0 {
		return nil
	}
	paths := environmentRestoreGeneratedFilePaths(compose, files)
	out := make([]environmentRestoreGeneratedFile, 0, len(paths))
	for _, path := range paths {
		content := files[path]
		report := environmentRestoreGeneratedFile{
			Path:   restoreWorkspacePath(workspace, path),
			Bytes:  len(content),
			Action: "plan-write",
			OK:     true,
		}
		if ok, errText := environmentRestoreGeneratedFileTargetOK(path, workspace); !ok {
			report.OK = false
			report.Error = errText
			out = append(out, report)
			continue
		}
		if execute {
			report.Action = "write"
			if err := os.MkdirAll(filepath.Dir(report.Path), 0o755); err != nil {
				report.OK = false
				report.Error = err.Error()
			} else if err := os.WriteFile(report.Path, []byte(content), 0o644); err != nil {
				report.OK = false
				report.Error = err.Error()
			}
		}
		out = append(out, report)
	}
	return out
}

func environmentRestoreGeneratedFilePaths(compose map[string]any, files map[string]string) []string {
	paths := make([]string, 0, len(files))
	seen := map[string]bool{}
	for _, path := range stringSliceFromAny(compose["generatedFileOrder"]) {
		clean := filepath.Clean(strings.TrimSpace(path))
		if clean == "." || clean == "" || seen[clean] {
			continue
		}
		if _, exists := files[clean]; !exists {
			continue
		}
		paths = append(paths, clean)
		seen[clean] = true
	}
	remaining := make([]string, 0, len(files)-len(paths))
	for path := range files {
		clean := filepath.Clean(strings.TrimSpace(path))
		if clean == "." || clean == "" || seen[clean] {
			continue
		}
		remaining = append(remaining, clean)
	}
	sort.Strings(remaining)
	paths = append(paths, remaining...)
	return paths
}

func environmentRestoreGeneratedFileTargetOK(path string, workspace string) (bool, string) {
	raw := strings.TrimSpace(path)
	if raw == "" {
		return false, "generated file path is empty"
	}
	if filepath.IsAbs(raw) {
		return false, "generated file path must be relative to the restore workspace: " + raw
	}
	clean := filepath.Clean(raw)
	if clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(os.PathSeparator)) {
		return false, "generated file path must stay inside the restore workspace: " + raw
	}
	target := restoreWorkspacePath(workspace, clean)
	rel, err := filepath.Rel(workspace, target)
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return false, "generated file path must stay inside the restore workspace: " + raw
	}
	return true, ""
}

func environmentRestoreComposeBaseArgs(compose map[string]any, workspace string, composeFiles []string) []string {
	args := []string{}
	for _, composeFile := range composeFiles {
		args = append(args, "-f", composeFile)
	}
	if len(stringMapFromAny(compose["env"])) > 0 {
		args = append(args, "--env-file", environmentRestoreGeneratedEnvFilePath(workspace))
	}
	if projectName := strings.TrimSpace(valueString(compose["projectName"])); projectName != "" {
		args = append(args, "-p", projectName)
	}
	for _, envFile := range stringSliceFromAny(compose["envFiles"]) {
		args = append(args, "--env-file", restoreWorkspacePath(workspace, envFile))
	}
	for _, profile := range stringSliceFromAny(compose["profiles"]) {
		args = append(args, "--profile", profile)
	}
	return args
}

func environmentRestoreGeneratedEnvFilePath(workspace string) string {
	return filepath.Join(workspace, ".agent-testbench", "restore.env")
}

func environmentRestoreComposeCommandServices(compose map[string]any, workspace string, composeFiles []string, selected []string) ([]string, []string) {
	knownServices, buildServices := environmentRestoreComposeBuildServiceSet(compose, workspace, composeFiles)
	services := append([]string{}, selected...)
	if len(services) == 0 && len(knownServices) > 0 {
		services = make([]string, 0, len(knownServices))
		for service := range knownServices {
			services = append(services, service)
		}
		sort.Strings(services)
	}
	imageOut := []string{}
	buildOut := []string{}
	for _, service := range services {
		service = strings.TrimSpace(service)
		if service == "" {
			continue
		}
		if buildServices[service] {
			buildOut = append(buildOut, service)
			continue
		}
		imageOut = append(imageOut, service)
	}
	return imageOut, buildOut
}

func environmentRestoreComposeBuildServiceSet(compose map[string]any, workspace string, composeFiles []string) (map[string]bool, map[string]bool) {
	known := map[string]bool{}
	builds := map[string]bool{}
	generated := stringMapFromAny(compose["generatedFiles"])
	for _, file := range composeFiles {
		content := generated[filepath.Clean(file)]
		if content == "" {
			content = generated[file]
		}
		if content == "" {
			if raw, err := os.ReadFile(restoreWorkspacePath(workspace, file)); err == nil {
				content = string(raw)
			}
		}
		if content == "" {
			continue
		}
		fileKnown, fileBuilds := environmentRestoreComposeBuildServicesFromText(content)
		for service := range fileKnown {
			known[service] = true
		}
		for service := range fileBuilds {
			known[service] = true
			builds[service] = true
		}
	}
	return known, builds
}

func environmentRestoreComposeBuildServicesFromText(content string) (map[string]bool, map[string]bool) {
	known := map[string]bool{}
	builds := map[string]bool{}
	inServices := false
	servicesIndent := -1
	serviceIndent := -1
	currentService := ""
	for _, line := range strings.Split(content, "\n") {
		if strings.TrimSpace(line) == "" || strings.HasPrefix(strings.TrimSpace(line), "#") {
			continue
		}
		indent := leadingSpaceCount(line)
		trimmed := strings.TrimSpace(line)
		if !inServices {
			if trimmed == "services:" {
				inServices = true
				servicesIndent = indent
			}
			continue
		}
		if indent <= servicesIndent {
			break
		}
		if strings.HasPrefix(trimmed, "-") {
			continue
		}
		if strings.HasSuffix(trimmed, ":") {
			key := strings.TrimSuffix(trimmed, ":")
			if serviceIndent < 0 || indent == serviceIndent {
				serviceIndent = indent
				currentService = strings.TrimSpace(key)
				if currentService != "" {
					known[currentService] = true
				}
				continue
			}
		}
		if currentService != "" && indent > serviceIndent && (trimmed == "build:" || strings.HasPrefix(trimmed, "build: ")) {
			builds[currentService] = true
		}
	}
	return known, builds
}

func writeEnvironmentRestoreGeneratedEnvFile(workspace string, compose map[string]any) (string, error) {
	values := stringMapFromAny(compose["env"])
	if len(values) == 0 {
		return "", nil
	}
	path := environmentRestoreGeneratedEnvFilePath(workspace)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	var b strings.Builder
	for _, key := range keys {
		value := strings.ReplaceAll(values[key], "$AGENT_TESTBENCH_WORKSPACE", workspace)
		b.WriteString(key)
		b.WriteString("=")
		b.WriteString(value)
		b.WriteString("\n")
	}
	if err := os.WriteFile(path, []byte(b.String()), 0o644); err != nil {
		return "", err
	}
	return path, nil
}

func stringMapFromAny(value any) map[string]string {
	out := map[string]string{}
	switch typed := value.(type) {
	case map[string]string:
		for key, value := range typed {
			if strings.TrimSpace(key) != "" {
				out[strings.TrimSpace(key)] = strings.TrimSpace(value)
			}
		}
	case map[string]any:
		for key, value := range typed {
			if strings.TrimSpace(key) != "" {
				out[strings.TrimSpace(key)] = strings.TrimSpace(valueString(value))
			}
		}
	}
	return out
}

func stringSliceFromAny(value any) []string {
	values, ok := value.([]any)
	if !ok {
		if typed, ok := value.([]string); ok {
			out := make([]string, 0, len(typed))
			for _, item := range typed {
				if strings.TrimSpace(item) != "" {
					out = append(out, strings.TrimSpace(item))
				}
			}
			return out
		}
		return nil
	}
	out := make([]string, 0, len(values))
	for _, item := range values {
		if value := strings.TrimSpace(valueString(item)); value != "" {
			out = append(out, value)
		}
	}
	return out
}

func runRestoreGitCommand(ctx context.Context, args ...string) (string, string) {
	cmd := exec.CommandContext(ctx, "git", args...)
	out, err := cmd.CombinedOutput()
	output := strings.TrimSpace(string(out))
	if err != nil {
		return output, err.Error()
	}
	return output, ""
}

func runRestoreCommand(ctx context.Context, workdir string, command []string) (string, string) {
	if len(command) == 0 {
		return "", "empty restore command"
	}
	cmd := exec.CommandContext(ctx, command[0], command[1:]...)
	cmd.Dir = workdir
	out, err := cmd.CombinedOutput()
	output := strings.TrimSpace(string(out))
	if err != nil {
		if output != "" {
			return output, err.Error() + ": " + output
		}
		return output, err.Error()
	}
	return output, ""
}

func runRestoreCommandWithInput(ctx context.Context, workdir string, command []string, input string) (string, string) {
	if len(command) == 0 {
		return "", "empty restore command"
	}
	cmd := exec.CommandContext(ctx, command[0], command[1:]...)
	cmd.Dir = workdir
	cmd.Stdin = bytes.NewBufferString(input)
	out, err := cmd.CombinedOutput()
	output := strings.TrimSpace(string(out))
	if err != nil {
		if output != "" {
			return output, err.Error() + ": " + output
		}
		return output, err.Error()
	}
	return output, ""
}

func waitEnvironmentRestoreHealthChecks(ctx context.Context, checks []any, timeout time.Duration, workspace string, composeBaseArgs []string) []environmentRestoreHealthCheckReport {
	out := make([]environmentRestoreHealthCheckReport, 0, len(checks))
	for _, raw := range checks {
		item, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		kind := strings.TrimSpace(valueString(item["kind"]))
		if kind == "" && strings.TrimSpace(valueString(item["url"])) != "" {
			kind = "url"
		}
		check := environmentRestoreHealthCheckReport{
			ID:        strings.TrimSpace(valueString(item["id"])),
			Kind:      kind,
			URL:       strings.TrimSpace(valueString(item["url"])),
			Address:   strings.TrimSpace(valueString(item["address"])),
			Command:   strings.TrimSpace(valueString(item["command"])),
			Service:   strings.TrimSpace(valueString(item["service"])),
			Container: strings.TrimSpace(valueString(item["container"])),
		}
		switch check.Kind {
		case "url", "":
			if check.URL == "" {
				continue
			}
			out = append(out, waitEnvironmentRestoreURLHealthCheck(ctx, check, timeout))
		case "tcp":
			if check.Address == "" {
				continue
			}
			out = append(out, waitEnvironmentRestoreTCPHealthCheck(ctx, check, timeout))
		case "command":
			if check.Command == "" {
				continue
			}
			out = append(out, waitEnvironmentRestoreCommandHealthCheck(ctx, check, timeout, workspace))
		case "compose-service":
			if check.Service == "" {
				continue
			}
			out = append(out, waitEnvironmentRestoreComposeServiceHealthCheck(ctx, check, timeout, workspace, composeBaseArgs))
		case "container":
			if check.Container == "" {
				continue
			}
			out = append(out, waitEnvironmentRestoreContainerHealthCheck(ctx, check, timeout))
		default:
			check.Error = "unsupported health check kind: " + check.Kind
			out = append(out, check)
		}
	}
	return out
}

func waitEnvironmentRestoreURLHealthCheck(ctx context.Context, check environmentRestoreHealthCheckReport, timeout time.Duration) environmentRestoreHealthCheckReport {
	client := &http.Client{Timeout: 2 * time.Second}
	deadline := time.Now().Add(timeout)
	var lastErr string
	for {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, check.URL, nil)
		if err != nil {
			check.Error = err.Error()
			return check
		}
		resp, err := client.Do(req)
		if err == nil {
			check.StatusCode = resp.StatusCode
			if closeErr := resp.Body.Close(); closeErr != nil {
				lastErr = closeErr.Error()
				check.Error = lastErr
				return check
			}
			if resp.StatusCode >= 200 && resp.StatusCode < 300 {
				check.OK = true
				check.Error = ""
				return check
			}
			lastErr = fmt.Sprintf("health check returned HTTP %d", resp.StatusCode)
		} else {
			lastErr = err.Error()
		}
		if time.Now().After(deadline) {
			check.Error = lastErr
			return check
		}
		select {
		case <-ctx.Done():
			check.Error = ctx.Err().Error()
			return check
		case <-time.After(250 * time.Millisecond):
		}
	}
}

func waitEnvironmentRestoreTCPHealthCheck(ctx context.Context, check environmentRestoreHealthCheckReport, timeout time.Duration) environmentRestoreHealthCheckReport {
	deadline := time.Now().Add(timeout)
	var lastErr string
	for {
		dialer := net.Dialer{Timeout: 2 * time.Second}
		conn, err := dialer.DialContext(ctx, "tcp", check.Address)
		if err == nil {
			if closeErr := conn.Close(); closeErr != nil {
				check.Error = closeErr.Error()
				return check
			}
			check.OK = true
			check.Error = ""
			return check
		}
		lastErr = err.Error()
		if time.Now().After(deadline) {
			check.Error = lastErr
			return check
		}
		select {
		case <-ctx.Done():
			check.Error = ctx.Err().Error()
			return check
		case <-time.After(250 * time.Millisecond):
		}
	}
}

func waitEnvironmentRestoreCommandHealthCheck(ctx context.Context, check environmentRestoreHealthCheckReport, timeout time.Duration, workspace string) environmentRestoreHealthCheckReport {
	return waitEnvironmentRestoreCommand(ctx, check, timeout, workspace, []string{"/bin/sh", "-c", check.Command}, func(check *environmentRestoreHealthCheckReport, output string) bool {
		check.Output = truncateReportText(output, 200)
		return true
	})
}

func waitEnvironmentRestoreComposeServiceHealthCheck(ctx context.Context, check environmentRestoreHealthCheckReport, timeout time.Duration, workspace string, composeBaseArgs []string) environmentRestoreHealthCheckReport {
	if len(composeBaseArgs) == 0 {
		check.Error = "compose service health check requires composeFile"
		return check
	}
	command := append(append([]string{"docker", "compose"}, composeBaseArgs...), "ps", "--format", "json", check.Service)
	return waitEnvironmentRestoreCommand(ctx, check, timeout, workspace, command, func(check *environmentRestoreHealthCheckReport, output string) bool {
		check.Output = truncateReportText(output, 200)
		state, health := parseComposeServiceHealth(output)
		check.State = state
		check.Health = health
		return state == "running" && (health == "" || health == "healthy")
	})
}

func waitEnvironmentRestoreContainerHealthCheck(ctx context.Context, check environmentRestoreHealthCheckReport, timeout time.Duration) environmentRestoreHealthCheckReport {
	command := []string{"docker", "inspect", "--format", "{{.State.Status}} {{if .State.Health}}{{.State.Health.Status}}{{end}}", check.Container}
	return waitEnvironmentRestoreCommand(ctx, check, timeout, "", command, func(check *environmentRestoreHealthCheckReport, output string) bool {
		check.Output = truncateReportText(output, 200)
		fields := strings.Fields(output)
		if len(fields) > 0 {
			check.State = strings.TrimSpace(fields[0])
		}
		if len(fields) > 1 {
			check.Health = strings.TrimSpace(fields[1])
		}
		return check.State == "running" && (check.Health == "" || check.Health == "healthy")
	})
}

func waitEnvironmentRestoreCommand(ctx context.Context, check environmentRestoreHealthCheckReport, timeout time.Duration, workspace string, command []string, ok func(*environmentRestoreHealthCheckReport, string) bool) environmentRestoreHealthCheckReport {
	deadline := time.Now().Add(timeout)
	var lastErr string
	for {
		output, errText := runRestoreCommand(ctx, workspace, command)
		if errText == "" && ok(&check, output) {
			check.OK = true
			check.Error = ""
			if check.Output == "" {
				check.Output = truncateReportText(output, 200)
			}
			return check
		}
		if errText != "" {
			lastErr = errText
		} else {
			lastErr = "health command did not report ready"
		}
		if time.Now().After(deadline) {
			check.Error = lastErr
			if check.Output == "" {
				check.Output = truncateReportText(output, 200)
			}
			return check
		}
		select {
		case <-ctx.Done():
			check.Error = ctx.Err().Error()
			return check
		case <-time.After(250 * time.Millisecond):
		}
	}
}

func parseComposeServiceHealth(output string) (string, string) {
	output = strings.TrimSpace(output)
	if output == "" {
		return "", ""
	}
	var object map[string]any
	if err := json.Unmarshal([]byte(output), &object); err == nil && object != nil {
		return strings.ToLower(valueString(firstNonNil(object["State"], object["state"]))), strings.ToLower(valueString(firstNonNil(object["Health"], object["health"])))
	}
	var array []map[string]any
	if err := json.Unmarshal([]byte(output), &array); err == nil && len(array) > 0 {
		return strings.ToLower(valueString(firstNonNil(array[0]["State"], array[0]["state"]))), strings.ToLower(valueString(firstNonNil(array[0]["Health"], array[0]["health"])))
	}
	lower := strings.ToLower(output)
	state := ""
	health := ""
	if strings.Contains(lower, "running") {
		state = "running"
	}
	if strings.Contains(lower, "unhealthy") {
		health = "unhealthy"
	} else if strings.Contains(lower, "healthy") {
		health = "healthy"
	}
	return state, health
}

func firstNonNil(values ...any) any {
	for _, value := range values {
		if value != nil {
			return value
		}
	}
	return nil
}
