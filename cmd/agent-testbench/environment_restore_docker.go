package main

import (
	"context"
	"strings"
	"time"

	"agent-testbench/internal/store"
)

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
	if !environmentRestoreProjectDockerNativeAssets(&report, graph, compose, workspace, execute) {
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
	healthChecks = environmentRestoreRefreshCompletedExpectations(healthChecks, compose, workspace)
	report.HealthChecks = waitEnvironmentRestoreHealthChecks(ctx, healthChecks, healthTimeout, workspace, composeBaseArgs)
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

func environmentRestoreHealthFailureError(check environmentRestoreHealthCheckReport) string {
	target := environmentRestoreHealthProgressTarget(check)
	if strings.TrimSpace(check.Error) != "" {
		return target + ": " + check.Error
	}
	return target + ": health check did not pass"
}
