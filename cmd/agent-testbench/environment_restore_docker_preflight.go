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

func environmentRestorePreflightReport(packageSpec environmentRestorePackageSpec, specs []environmentRestoreRepoSpec, compose map[string]any, workspace string, execute bool, cleanupOptions environmentRestoreDockerCleanupOptions, prepareReposOnly bool) environmentRestorePreflight {
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
		environmentRestoreAddComposePreflight(&report, compose, specs, workspace, execute, cleanupOptions, prepareReposOnly)
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

func environmentRestoreAddComposePreflight(report *environmentRestorePreflight, compose map[string]any, specs []environmentRestoreRepoSpec, workspace string, execute bool, cleanupOptions environmentRestoreDockerCleanupOptions, prepareReposOnly bool) {
	report.Tools = append(report.Tools, environmentRestoreTool("docker", true))
	report.Tools = append(report.Tools, environmentRestoreCommandTool("docker compose", true, "docker", "compose", dockerComposeCommandVersion))
	report.HeavySteps = append(report.HeavySteps, environmentRestoreComposeHeavySteps(compose, cleanupOptions)...)
	environmentRestoreCheckComposeBindMounts(report, compose, workspace)
	environmentRestoreCheckComposeImages(report, compose, workspace, execute)
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

func environmentRestoreCheckComposeImages(report *environmentRestorePreflight, compose map[string]any, workspace string, execute bool) {
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
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		out, err := exec.CommandContext(ctx, dockerPath, "manifest", "inspect", image).CombinedOutput()
		cancel()
		if err == nil {
			continue
		}
		if environmentRestoreLocalDockerImageExists(dockerPath, image) {
			report.Notes = append(report.Notes, "local Docker image is available for compose service "+service+": "+image)
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

func environmentRestoreComposeBindMountSources(compose map[string]any, workspace string) map[string][]string {
	out := map[string][]string{}
	for _, content := range environmentRestoreComposeFileContents(compose, workspace) {
		for service, sources := range parseComposeBindMountSources(content) {
			out[service] = append(out[service], sources...)
		}
	}
	return out
}

func environmentRestoreComposeImageReferences(compose map[string]any, workspace string) map[string]string {
	out := map[string]string{}
	for _, content := range environmentRestoreComposeFileContents(compose, workspace) {
		for service, image := range parseComposeImageReferences(content) {
			out[service] = image
		}
	}
	return out
}

func environmentRestoreComposeFileContents(compose map[string]any, workspace string) []string {
	contents := []string{}
	generated := stringMapFromAny(compose["generatedFiles"])
	for _, file := range environmentRestoreComposeFiles(compose) {
		content := generated[filepath.Clean(file)]
		if content == "" {
			content = generated[file]
		}
		if content == "" {
			raw, err := os.ReadFile(restoreWorkspacePath(workspace, file))
			if err == nil {
				content = string(raw)
			}
		}
		if strings.TrimSpace(content) != "" {
			contents = append(contents, content)
		}
	}
	return contents
}

func parseComposeImageReferences(content string) map[string]string {
	out := map[string]string{}
	walkComposeServiceLines(content, func(service string, trimmed string) {
		if strings.HasPrefix(trimmed, "image:") {
			image := cleanComposeScalar(strings.TrimSpace(strings.TrimPrefix(trimmed, "image:")))
			if image != "" {
				out[service] = image
			}
		}
	})
	return out
}

func parseComposeBindMountSources(content string) map[string][]string {
	out := map[string][]string{}
	state := composeBindMountParseState{}
	for _, line := range strings.Split(content, "\n") {
		service, source := state.bindSource(line)
		if source != "" {
			out[service] = append(out[service], source)
		}
	}
	return out
}

type composeBindMountParseState struct {
	inServices     bool
	servicesIndent int
	serviceIndent  int
	currentService string
	inVolumes      bool
	volumesIndent  int
}

func (state *composeBindMountParseState) bindSource(line string) (string, string) {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" || strings.HasPrefix(trimmed, "#") {
		return "", ""
	}
	indent := leadingSpaceCount(line)
	if !state.inServices {
		state.enterServices(trimmed, indent)
		return "", ""
	}
	if indent <= state.servicesIndent {
		state.reset()
		return "", ""
	}
	if state.enterService(trimmed, indent) {
		return "", ""
	}
	if state.currentService == "" || indent <= state.serviceIndent {
		state.inVolumes = false
		return "", ""
	}
	if state.enterVolumes(trimmed, indent) {
		return "", ""
	}
	if state.inVolumes && indent <= state.volumesIndent {
		state.inVolumes = false
	}
	if !state.inVolumes {
		return "", ""
	}
	return state.currentService, composeVolumeSource(trimmed)
}

func (state *composeBindMountParseState) enterServices(trimmed string, indent int) {
	if trimmed != "services:" {
		return
	}
	state.inServices = true
	state.servicesIndent = indent
	state.serviceIndent = -1
}

func (state *composeBindMountParseState) reset() {
	state.inServices = false
	state.currentService = ""
	state.inVolumes = false
	state.serviceIndent = -1
}

func (state *composeBindMountParseState) enterService(trimmed string, indent int) bool {
	if strings.HasPrefix(trimmed, "-") || !strings.HasSuffix(trimmed, ":") {
		return false
	}
	if state.serviceIndent >= 0 && indent != state.serviceIndent {
		return false
	}
	state.serviceIndent = indent
	state.currentService = strings.TrimSpace(strings.TrimSuffix(trimmed, ":"))
	state.inVolumes = false
	return true
}

func (state *composeBindMountParseState) enterVolumes(trimmed string, indent int) bool {
	if strings.TrimSuffix(trimmed, ":") != "volumes" {
		return false
	}
	state.inVolumes = true
	state.volumesIndent = indent
	return true
}

func composeVolumeSource(trimmed string) string {
	if strings.HasPrefix(trimmed, "- ") {
		return environmentRestoreShortVolumeSource(strings.TrimSpace(strings.TrimPrefix(trimmed, "- ")))
	}
	if strings.HasPrefix(trimmed, "source:") {
		return cleanComposeScalar(strings.TrimSpace(strings.TrimPrefix(trimmed, "source:")))
	}
	return ""
}

func walkComposeServiceLines(content string, visit func(service string, trimmed string)) {
	inServices := false
	servicesIndent := -1
	serviceIndent := -1
	currentService := ""
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		indent := leadingSpaceCount(line)
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
		if strings.HasSuffix(trimmed, ":") && (serviceIndent < 0 || indent == serviceIndent) {
			serviceIndent = indent
			currentService = strings.TrimSpace(strings.TrimSuffix(trimmed, ":"))
			continue
		}
		if currentService != "" && indent > serviceIndent {
			visit(currentService, trimmed)
		}
	}
}

func environmentRestoreShortVolumeSource(entry string) string {
	entry = cleanComposeScalar(entry)
	if entry == "" || strings.HasPrefix(entry, "type:") {
		return ""
	}
	if !strings.HasPrefix(entry, "/") && !strings.HasPrefix(entry, "$") {
		return ""
	}
	source, _, ok := strings.Cut(entry, ":")
	if !ok {
		return ""
	}
	return cleanComposeScalar(source)
}

func cleanComposeScalar(value string) string {
	value = strings.TrimSpace(value)
	value = strings.Trim(value, `"'`)
	if cut, _, ok := strings.Cut(value, " #"); ok {
		value = strings.TrimSpace(cut)
	}
	return strings.Trim(value, `"'`)
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
