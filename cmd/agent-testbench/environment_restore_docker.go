package main

import (
	"context"
	"strings"
	"time"

	"agent-testbench/internal/store"
)

func environmentRestoreDocker(ctx context.Context, graph store.EnvironmentComponentGraph, compose map[string]any, healthChecks []any, workspace string, execute bool, healthTimeout time.Duration, cleanupOptions environmentRestoreDockerCleanupOptions) environmentRestoreDockerReport {
	report, composeBaseArgs := environmentRestoreDockerPlan(graph, compose, workspace, cleanupOptions)
	if !report.OK {
		return report
	}
	environmentRestoreCheckGeneratedFiles(&report, compose, workspace, false)
	if !execute {
		return report
	}
	environmentRestoreEmitPhaseStarted(ctx, "docker.prepare", workspace, "preparing Docker workspace and Store-projected files")
	if !environmentRestorePrepareDockerExecution(&report, compose, workspace) {
		environmentRestoreEmitPhaseCompleted(ctx, "docker.prepare", workspace, false, report.Action, report.Error)
		return report
	}
	environmentRestoreEmitPhaseCompleted(ctx, "docker.prepare", workspace, true, "Docker workspace prepared", "")
	environmentRestoreEmitPhaseStarted(ctx, "docker.compose.validate", report.ComposeFile, "validating Docker Compose files")
	if !environmentRestoreValidateComposeFiles(&report) {
		environmentRestoreEmitPhaseCompleted(ctx, "docker.compose.validate", report.ComposeFile, false, report.Action, report.Error)
		return report
	}
	environmentRestoreEmitPhaseCompleted(ctx, "docker.compose.validate", report.ComposeFile, true, "Docker Compose files validated", "")
	if report.Cleanup.Requested {
		environmentRestoreEmitPhaseStarted(ctx, "docker.cleanup", report.ComposeFile, "checking Docker cleanup safety gates")
	}
	if !environmentRestoreRunCleanup(ctx, &report, workspace) {
		if report.Cleanup.Requested {
			environmentRestoreEmitPhaseCompleted(ctx, "docker.cleanup", report.ComposeFile, false, report.Cleanup.Action, report.Cleanup.Error)
		}
		return report
	}
	if report.Cleanup.Requested {
		environmentRestoreEmitPhaseCompleted(ctx, "docker.cleanup", report.ComposeFile, true, report.Cleanup.Action, "")
	}
	nativeAssetTarget := environmentRestoreDockerReportTarget(report)
	environmentRestoreEmitPhaseStarted(ctx, "docker.native-assets", nativeAssetTarget, "projecting Docker-native Store assets")
	if !environmentRestoreProjectDockerNativeAssets(&report, graph, compose, workspace, execute) {
		environmentRestoreEmitPhaseCompleted(ctx, "docker.native-assets", nativeAssetTarget, false, report.Action, report.Error)
		return report
	}
	environmentRestoreEmitPhaseCompleted(ctx, "docker.native-assets", nativeAssetTarget, true, "Docker-native Store assets projected", "")
	environmentRestoreMarkDockerExecuting(&report)
	environmentRestoreEmitPhaseStarted(ctx, "docker.compose.execute", report.Action, "running Docker service startup commands")
	if !environmentRestoreRunCommands(ctx, &report, workspace) {
		environmentRestoreEmitPhaseCompleted(ctx, "docker.compose.execute", report.Action, false, "Docker service startup failed", report.Error)
		return report
	}
	environmentRestoreEmitPhaseCompleted(ctx, "docker.compose.execute", report.Action, true, "Docker service startup completed", "")
	environmentRestoreEmitPhaseStarted(ctx, "docker.edge-assets", report.Action, "applying post-start Store edge assets")
	report.AppliedAssets = environmentRestoreApplyEdgeAssets(ctx, graph, compose, workspace, execute, composeBaseArgs)
	for _, asset := range report.AppliedAssets {
		if !asset.OK {
			report.OK = false
			report.Error = asset.Error
			environmentRestoreEmitPhaseCompleted(ctx, "docker.edge-assets", report.Action, false, asset.Action, asset.Error)
			return report
		}
	}
	environmentRestoreEmitPhaseCompleted(ctx, "docker.edge-assets", report.Action, true, "post-start Store edge assets applied", "")
	healthChecks = environmentRestoreRefreshCompletedExpectations(healthChecks, compose, workspace)
	environmentRestoreEmitPhaseStarted(ctx, "docker.health", report.Action, "waiting for Docker service health checks")
	report.HealthChecks = waitEnvironmentRestoreHealthChecks(ctx, healthChecks, healthTimeout, workspace, composeBaseArgs)
	for _, check := range report.HealthChecks {
		if !check.OK {
			report.OK = false
			if report.Error == "" {
				report.Error = environmentRestoreHealthFailureError(check)
			}
			environmentRestoreEmitPhaseCompleted(ctx, "docker.health", report.Action, false, "Docker service health check failed", report.Error)
			return report
		}
	}
	environmentRestoreEmitPhaseCompleted(ctx, "docker.health", report.Action, true, "Docker service health checks passed", "")
	return report
}

func environmentRestoreHealthFailureError(check environmentRestoreHealthCheckReport) string {
	target := environmentRestoreHealthProgressTarget(check)
	if strings.TrimSpace(check.Error) != "" {
		return target + ": " + check.Error
	}
	return target + ": health check did not pass"
}

func environmentRestoreDockerReportTarget(report environmentRestoreDockerReport) string {
	if strings.TrimSpace(report.ComposeFile) != "" {
		return report.ComposeFile
	}
	return report.Workdir
}
