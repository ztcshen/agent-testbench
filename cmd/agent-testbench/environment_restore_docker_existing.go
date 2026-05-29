package main

import (
	"context"
	"os"
	"strings"
	"time"

	"agent-testbench/internal/store"
)

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
			if report.Error == "" {
				report.Error = environmentRestoreHealthFailureError(check)
			}
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
