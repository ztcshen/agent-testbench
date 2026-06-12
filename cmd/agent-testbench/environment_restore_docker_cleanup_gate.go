package main

import (
	"path/filepath"
	"sort"
	"strings"

	"agent-testbench/internal/domain/environmentfiles"
	"agent-testbench/internal/store"
)

func environmentRestoreDockerCleanupLinkage(compose map[string]any, graph store.EnvironmentComponentGraph, workspace string, composeFiles []string) environmentRestoreDockerCleanupLinkageReport {
	report := environmentRestoreDockerCleanupLinkageReport{
		OK:             true,
		ComposeProject: strings.TrimSpace(valueString(compose["projectName"])),
		StoreAssets:    len(graph.Assets),
	}
	if report.ComposeProject == "" {
		report.OK = false
		report.MissingComposeProject = true
	}
	composeServices := environmentRestoreCleanupComposeServices(compose, workspace, composeFiles)
	report.ComposeServices = composeServices
	serviceSet := environmentRestoreStringSet(composeServices)
	if len(graph.Components) == 0 {
		report.OK = false
		report.MissingComponentGraph = true
	}
	for _, component := range graph.Components {
		if !component.Required {
			continue
		}
		id := strings.TrimSpace(component.ComponentID)
		if id != "" {
			report.RequiredComponents = append(report.RequiredComponents, id)
		}
		service := environmentRestoreComponentComposeService(component, id)
		if service == "" {
			report.OK = false
			report.MissingComponentServices = append(report.MissingComponentServices, id)
			continue
		}
		if !serviceSet[service] {
			report.OK = false
			report.MissingComposeServices = append(report.MissingComposeServices, service)
		}
	}
	report.RequiredComponents = dedupeStrings(report.RequiredComponents)
	report.MissingComponentServices = dedupeStrings(report.MissingComponentServices)
	report.MissingComposeServices = dedupeStrings(report.MissingComposeServices)
	report.MissingProjectedFiles = environmentRestoreCleanupMissingProjectedFiles(compose, graph, composeFiles)
	if len(report.MissingProjectedFiles) > 0 {
		report.OK = false
	}
	if !report.OK {
		report.Error = environmentRestoreCleanupLinkageError(report)
	}
	return report
}

func environmentRestoreCleanupComposeServices(compose map[string]any, workspace string, composeFiles []string) []string {
	services := dedupeStrings(stringSliceFromAny(compose["services"]))
	if len(services) == 0 {
		known, _, _ := environmentRestoreComposeServiceDefinitions(compose, workspace, composeFiles)
		for service := range known {
			services = append(services, service)
		}
	}
	sort.Strings(services)
	return services
}

func environmentRestoreCleanupMissingProjectedFiles(compose map[string]any, graph store.EnvironmentComponentGraph, composeFiles []string) []string {
	projectionCompose := map[string]any{}
	for key, value := range compose {
		projectionCompose[key] = value
	}
	if len(composeFiles) > 0 {
		projectionCompose["composeFiles"] = composeFiles
	}
	projection := environmentfiles.FromCompose(projectionCompose, nil, graph)
	missing := make([]string, 0, len(projection.Missing))
	for _, file := range projection.Missing {
		missing = append(missing, file.Kind+":"+filepath.ToSlash(file.Path))
	}
	return dedupeStrings(missing)
}

func environmentRestoreCleanupLinkageError(report environmentRestoreDockerCleanupLinkageReport) string {
	reasons := []string{}
	if report.MissingComposeProject {
		reasons = append(reasons, "compose projectName is required")
	}
	if report.MissingComponentGraph {
		reasons = append(reasons, "Store component graph is required")
	}
	if len(report.MissingComponentServices) > 0 {
		reasons = append(reasons, "required components missing composeService: "+strings.Join(report.MissingComponentServices, ","))
	}
	if len(report.MissingComposeServices) > 0 {
		reasons = append(reasons, "required compose services not in Compose plan: "+strings.Join(report.MissingComposeServices, ","))
	}
	if len(report.MissingProjectedFiles) > 0 {
		reasons = append(reasons, "files must be Store-projected before cleanup: "+strings.Join(report.MissingProjectedFiles, ","))
	}
	if len(reasons) == 0 {
		return "Docker cleanup requires complete Store-to-Compose environment linkage"
	}
	return "Docker cleanup requires complete Store-to-Compose environment linkage: " + strings.Join(reasons, "; ")
}
