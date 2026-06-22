package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type environmentRestorePreflight struct {
	OK                 bool                              `json:"ok"`
	AssumeCleanDocker  bool                              `json:"assumeCleanDocker,omitempty"`
	Tools              []environmentRestorePreflightTool `json:"tools"`
	HeavySteps         []string                          `json:"heavySteps,omitempty"`
	ContainerConflicts []string                          `json:"containerConflicts,omitempty"`
	StartupAssets      []environmentRestoreStartupAsset  `json:"startupAssets,omitempty"`
	ComposeIssues      []string                          `json:"composeIssues,omitempty"`
	LocalImageServices []string                          `json:"localImageServices,omitempty"`
	Notes              []string                          `json:"notes,omitempty"`
}

type environmentRestorePreflightTool struct {
	Name     string `json:"name"`
	Required bool   `json:"required"`
	OK       bool   `json:"ok"`
	Path     string `json:"path,omitempty"`
	Error    string `json:"error,omitempty"`
}

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

func environmentRestorePreflightReport(packageSpec environmentRestorePackageSpec, specs []environmentRestoreRepoSpec, compose map[string]any, workspace string, execute bool, pull bool, cleanupOptions environmentRestoreDockerCleanupOptions, prepareReposOnly bool, packageIgnored bool) environmentRestorePreflight {
	report := environmentRestorePreflight{
		OK:                true,
		AssumeCleanDocker: cleanupOptions.AssumeCleanDocker,
		Notes: []string{
			"Sandbox control-plane Store must already be reachable outside restored Docker target services.",
			"Heavy Docker image and container validation should be reviewed before deleting or rebuilding existing local Docker state.",
		},
	}
	if environmentRestorePreflightRequiresGit(packageSpec, specs, packageIgnored) {
		report.Tools = append(report.Tools, environmentRestoreTool("git", true))
	}
	composeFile := strings.TrimSpace(valueString(compose["composeFile"]))
	startCommand := strings.TrimSpace(valueString(compose["startCommand"]))
	if composeFile != "" {
		environmentRestoreAddComposePreflight(&report, compose, specs, workspace, execute, pull, cleanupOptions, prepareReposOnly)
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

func environmentRestorePreflightRequiresGit(packageSpec environmentRestorePackageSpec, specs []environmentRestoreRepoSpec, packageIgnored bool) bool {
	if !packageIgnored && (strings.TrimSpace(packageSpec.URL) != "" || strings.TrimSpace(packageSpec.Ref) != "") {
		return true
	}
	for _, spec := range specs {
		if strings.TrimSpace(spec.URL) != "" || strings.TrimSpace(spec.Ref) != "" {
			return true
		}
	}
	return false
}

func environmentRestoreAddComposePreflight(report *environmentRestorePreflight, compose map[string]any, specs []environmentRestoreRepoSpec, workspace string, execute bool, pull bool, cleanupOptions environmentRestoreDockerCleanupOptions, prepareReposOnly bool) {
	report.Tools = append(report.Tools, environmentRestoreTool("docker", true))
	report.Tools = append(report.Tools, environmentRestoreCommandTool("docker compose", true, "docker", "compose", dockerComposeCommandVersion))
	report.HeavySteps = append(report.HeavySteps, environmentRestoreComposeHeavySteps(compose, cleanupOptions, pull)...)
	environmentRestoreCheckComposeBindMounts(report, compose, workspace)
	environmentRestoreCheckComposeImages(report, compose, workspace, execute, pull)
	environmentRestoreCheckContainerConflicts(report, compose, workspace, cleanupOptions, prepareReposOnly)
	environmentRestoreCheckStartupAssets(report, compose, specs, workspace, cleanupOptions, prepareReposOnly)
	for _, file := range environmentRestoreComposeFiles(compose) {
		if resolved := restoreWorkspacePath(workspace, file); strings.TrimSpace(resolved) != "" {
			report.Notes = append(report.Notes, "compose file must exist before Docker execution: "+resolved)
		}
	}
}

func environmentRestoreCheckComposeBindMounts(report *environmentRestorePreflight, compose map[string]any, workspace string) {
	selected := environmentRestoreStringSet(stringSliceFromAny(compose["services"]))
	for service, sources := range environmentRestoreComposeBindMountSources(compose, workspace) {
		if len(selected) > 0 && !selected[service] {
			continue
		}
		for _, source := range sources {
			if strings.Contains(source, "$") || !filepath.IsAbs(source) {
				continue
			}
			if _, err := os.Stat(source); err == nil {
				continue
			}
			issue := "missing host bind mount source for compose service " + service + ": " + source
			report.ComposeIssues = append(report.ComposeIssues, issue)
			report.Notes = append(report.Notes, issue)
			report.OK = false
		}
	}
}

func environmentRestoreCheckComposeImages(report *environmentRestorePreflight, compose map[string]any, workspace string, execute bool, pull bool) {
	if !execute || boolFromReportAny(compose["skipPull"]) {
		return
	}
	dockerPath, err := exec.LookPath("docker")
	if err != nil {
		return
	}
	selected := environmentRestoreStringSet(stringSliceFromAny(compose["services"]))
	_, buildServices, _ := environmentRestoreComposeServiceDefinitions(compose, workspace, environmentRestoreComposeFiles(compose))
	for service, image := range environmentRestoreComposeImageReferences(compose, workspace) {
		if len(selected) > 0 && !selected[service] {
			continue
		}
		if buildServices[service] || strings.TrimSpace(image) == "" || strings.Contains(image, "$") {
			continue
		}
		if environmentRestoreLocalDockerImageExists(dockerPath, image) {
			report.LocalImageServices = append(report.LocalImageServices, service)
			report.Notes = append(report.Notes, "local Docker image is available for compose service "+service+": "+image)
		}
		if !pull {
			continue
		}
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		out, err := exec.CommandContext(ctx, dockerPath, "manifest", "inspect", image).CombinedOutput()
		cancel()
		if err == nil {
			continue
		}
		detail := strings.TrimSpace(string(out))
		issue := "unavailable compose image for service " + service + ": " + image
		if detail != "" {
			issue += ": " + detail
		}
		report.ComposeIssues = append(report.ComposeIssues, issue)
		report.Notes = append(report.Notes, issue)
		report.OK = false
	}
}

func environmentRestoreLocalDockerImageExists(dockerPath string, image string) bool {
	image = strings.TrimSpace(image)
	if image == "" {
		return false
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return exec.CommandContext(ctx, dockerPath, "image", "inspect", image).Run() == nil
}

func environmentRestoreComposeHeavySteps(compose map[string]any, cleanupOptions environmentRestoreDockerCleanupOptions, pull bool) []string {
	steps := []string{}
	if pull && !boolFromReportAny(compose["skipPull"]) {
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
