package main

import (
	"fmt"
	"strings"

	"agent-testbench/internal/domain/redaction"
)

type environmentRestoreReadiness struct {
	OK                         bool                              `json:"ok"`
	Action                     string                            `json:"action"`
	PauseBeforeHeavyValidation bool                              `json:"pauseBeforeHeavyValidation"`
	NextStep                   string                            `json:"nextStep"`
	Items                      []environmentRestoreReadinessItem `json:"items"`
}

type environmentRestoreReadinessItem struct {
	Name     string `json:"name"`
	Required bool   `json:"required"`
	OK       bool   `json:"ok"`
	Detail   string `json:"detail,omitempty"`
}

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
		"repairItemCount":    len(report.Linkage.RepairPlan),
		"error":              report.Error,
	}
}

func printEnvironmentRestoreReport(report environmentRestoreReport) {
	printEnvironmentRestoreHeader(report)
	printEnvironmentRestoreReadiness(report.Readiness)
	printEnvironmentRestoreRepos(report.Repos)
	printEnvironmentRestoreDocker(report.Docker)
	printEnvironmentRestoreWorkflow(report.Workflow)
	for _, action := range report.NextActions {
		fmt.Printf("Next: %s\n", action)
	}
}

func printEnvironmentRestoreHeader(report environmentRestoreReport) {
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
}

func printEnvironmentRestoreReadiness(readiness environmentRestoreReadiness) {
	if readiness.Action == "" {
		return
	}
	fmt.Printf("Readiness: %s (ok=%t)\n", readiness.Action, readiness.OK)
	for _, item := range readiness.Items {
		state := "ok"
		if !item.OK {
			state = "failed"
		}
		fmt.Printf("  %s: %s\n", item.Name, state)
		if item.Detail != "" {
			fmt.Printf("    %s\n", item.Detail)
		}
	}
	if readiness.NextStep != "" {
		fmt.Printf("  next: %s\n", readiness.NextStep)
	}
}

func printEnvironmentRestoreRepos(repos []environmentRestoreRepoReport) {
	for _, repo := range repos {
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
}

func printEnvironmentRestoreDocker(docker environmentRestoreDockerReport) {
	dockerState := docker.Action
	if !docker.OK {
		dockerState = "failed"
	}
	fmt.Printf("Docker: %s\n", dockerState)
	if docker.ComposeFile != "" {
		fmt.Printf("  compose: %s\n", docker.ComposeFile)
	}
	for _, command := range docker.Commands {
		fmt.Printf("  command: %s\n", strings.Join(command, " "))
	}
	printEnvironmentRestoreDockerCleanup(docker.Cleanup)
	for _, check := range docker.HealthChecks {
		state := "failed"
		if check.OK {
			state = "ok"
		}
		fmt.Printf("  health: %s [%s]\n", check.URL, state)
		if check.Error != "" {
			fmt.Printf("    error: %s\n", check.Error)
		}
	}
	if docker.Error != "" {
		fmt.Printf("  error: %s\n", docker.Error)
	}
}

func printEnvironmentRestoreDockerCleanup(cleanup environmentRestoreDockerCleanupReport) {
	if !cleanup.Requested {
		return
	}
	fmt.Printf("  cleanup: %s\n", cleanup.Action)
	if cleanup.Warning != "" {
		fmt.Printf("    warning: %s\n", cleanup.Warning)
	}
	for _, command := range cleanup.BackupCommands {
		fmt.Printf("    backup: %s\n", strings.Join(command, " "))
	}
	for _, command := range cleanup.Commands {
		fmt.Printf("    cleanup-command: %s\n", strings.Join(command, " "))
	}
	for _, item := range cleanup.Linkage.RepairPlan {
		fmt.Printf("    repair: %s -> %s\n", item.Name, item.Action)
		if len(item.Missing) > 0 {
			fmt.Printf("      missing: %s\n", strings.Join(item.Missing, ", "))
		}
		if item.CommandHint != "" {
			fmt.Printf("      hint: %s\n", item.CommandHint)
		}
	}
	if cleanup.Error != "" {
		fmt.Printf("    error: %s\n", cleanup.Error)
	}
}

func printEnvironmentRestoreWorkflow(workflow environmentRestoreWorkflowRun) {
	fmt.Printf("Workflow: %s [%s]\n", workflow.WorkflowID, workflow.Action)
	if workflow.RunID != "" {
		fmt.Printf("  run: %s\n", workflow.RunID)
	}
	if workflow.OutputDir != "" {
		fmt.Printf("  report: %s\n", workflow.OutputDir)
	}
	if workflow.Error != "" {
		fmt.Printf("  error: %s\n", workflow.Error)
	}
}
