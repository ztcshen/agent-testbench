package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"agent-testbench/internal/store"
)

type environmentLifecycleOptions struct {
	EnvironmentID string
	StoreRef      string
	StoreURL      string
	Workspace     string
	HealthTimeout time.Duration
	JSONOutput    bool
}

func parseEnvironmentLifecycleOptions(name string, args []string) (environmentLifecycleOptions, error) {
	flags := flag.NewFlagSet(name, flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	storeRef := flags.String("store", "", "Named Store config or Store DSN")
	storeURL := flags.String("store-url", "", legacyStoreURLFlagHelp)
	workspace := flags.String("workspace", "", "Local workspace for generated compose artifacts")
	healthTimeoutSeconds := flags.Int("health-timeout-seconds", 5, "Maximum seconds to wait for a non-Compose status health probe")
	jsonOutput := flags.Bool("json", false, "Emit a machine-readable JSON report")
	if err := parseInterspersedFlags(flags, args); err != nil {
		return environmentLifecycleOptions{}, err
	}
	id := strings.TrimSpace(flags.Arg(0))
	if id == "" {
		return environmentLifecycleOptions{}, errors.New("environment id is required")
	}
	if strings.TrimSpace(*workspace) == "" {
		return environmentLifecycleOptions{}, errors.New("--workspace is required")
	}
	if *healthTimeoutSeconds <= 0 {
		return environmentLifecycleOptions{}, errors.New("--health-timeout-seconds must be greater than zero")
	}
	resolvedStoreURL, err := resolveRequiredDailyStoreReference(*storeRef, *storeURL)
	if err != nil {
		return environmentLifecycleOptions{}, err
	}
	return environmentLifecycleOptions{
		EnvironmentID: id,
		StoreRef:      *storeRef,
		StoreURL:      resolvedStoreURL,
		Workspace:     *workspace,
		HealthTimeout: time.Duration(*healthTimeoutSeconds) * time.Second,
		JSONOutput:    *jsonOutput,
	}, nil
}

func loadEnvironmentLifecyclePlan(ctx context.Context, options environmentLifecycleOptions) (store.Store, store.Environment, store.EnvironmentComponentGraph, environmentRestoreBuildPlan, func(), error) {
	runtime, err := openStore(ctx, options.StoreURL)
	if err != nil {
		return nil, store.Environment{}, store.EnvironmentComponentGraph{}, environmentRestoreBuildPlan{}, func() {}, err
	}
	cleanup := cleanupCLIStore(runtime)
	env, err := runtime.GetEnvironment(ctx, options.EnvironmentID)
	if err != nil {
		cleanup()
		return nil, store.Environment{}, store.EnvironmentComponentGraph{}, environmentRestoreBuildPlan{}, func() {}, err
	}
	graph, err := runtime.GetEnvironmentComponentGraph(ctx, env.ID)
	if err != nil {
		cleanup()
		return nil, store.Environment{}, store.EnvironmentComponentGraph{}, environmentRestoreBuildPlan{}, func() {}, err
	}
	files, err := runtime.ListEnvironmentFiles(ctx, env.ID)
	if err != nil {
		cleanup()
		return nil, store.Environment{}, store.EnvironmentComponentGraph{}, environmentRestoreBuildPlan{}, func() {}, err
	}
	services, err := runtime.ListEnvironmentServices(ctx, env.ID)
	if err != nil {
		cleanup()
		return nil, store.Environment{}, store.EnvironmentComponentGraph{}, environmentRestoreBuildPlan{}, func() {}, err
	}
	healthChecks, err := runtime.ListEnvironmentHealthChecks(ctx, env.ID)
	if err != nil {
		cleanup()
		return nil, store.Environment{}, store.EnvironmentComponentGraph{}, environmentRestoreBuildPlan{}, func() {}, err
	}
	workflowID := strings.TrimSpace(env.VerificationWorkflowID)
	if workflowID == "" {
		cleanup()
		return nil, store.Environment{}, store.EnvironmentComponentGraph{}, environmentRestoreBuildPlan{}, func() {}, fmt.Errorf("environment %s has no verification workflow; lifecycle commands must be anchored to a verified workflow", env.ID)
	}
	plan, err := environmentRestoreBuildPlanFromEnvironmentWithStructuredState(env, workflowID, options.Workspace, options.StoreURL, files, services, healthChecks, graph)
	if err != nil {
		cleanup()
		return nil, store.Environment{}, store.EnvironmentComponentGraph{}, environmentRestoreBuildPlan{}, func() {}, err
	}
	return runtime, env, graph, plan, cleanup, nil
}

func prepareEnvironmentLifecycleComposeFiles(report *environmentStatusDockerReport, compose map[string]any, workspace string) bool {
	for _, item := range prepareEnvironmentRestoreGeneratedFiles(compose, workspace, true) {
		if !item.OK {
			report.OK = false
			report.Action = "prepare-generated-files"
			report.Error = item.Error
			return false
		}
	}
	if _, err := writeEnvironmentRestoreGeneratedEnvFile(workspace, compose); err != nil {
		report.OK = false
		report.Action = "prepare-compose-env"
		report.Error = err.Error()
		return false
	}
	return true
}

func validateEnvironmentLifecycleComposeFiles(report *environmentStatusDockerReport, compose map[string]any, workspace string) bool {
	generatedFiles := generatedFileContentMapFromAny(compose["generatedFiles"])
	generatedModes := environmentRestoreGeneratedFileModes(compose)
	for _, relativePath := range environmentRestoreGeneratedFilePaths(compose, generatedFiles) {
		if ok, errText := environmentRestoreGeneratedFileTargetOK(relativePath, workspace); !ok {
			return failEnvironmentLifecycleProjectionValidation(report, relativePath, errText)
		}
		mode := generatedModes[filepath.Clean(relativePath)]
		if mode == 0 {
			mode = 0o644
		}
		content, err := readEnvironmentLifecycleProjectionFile(workspace, relativePath, mode)
		if err != nil {
			return failEnvironmentLifecycleProjectionValidation(report, restoreWorkspacePath(workspace, relativePath), err.Error())
		}
		if content != generatedFiles[relativePath] {
			return failEnvironmentLifecycleProjectionValidation(report, restoreWorkspacePath(workspace, relativePath), "content differs from the active Store projection")
		}
	}
	for _, path := range append(environmentRestoreComposeFiles(compose), stringSliceFromAny(compose["envFiles"])...) {
		if _, err := readEnvironmentLifecycleProjectionFile(workspace, path, 0); err != nil {
			return failEnvironmentLifecycleProjectionValidation(report, restoreWorkspacePath(workspace, path), err.Error())
		}
	}
	envRelativePath := filepath.Join(".agent-testbench", "restore.env")
	envPath := restoreWorkspacePath(workspace, envRelativePath)
	envContent, err := readEnvironmentLifecycleProjectionFile(workspace, envRelativePath, 0o600)
	if err != nil {
		return failEnvironmentLifecycleProjectionValidation(report, envPath, err.Error())
	}
	if envContent != environmentRestoreGeneratedEnvFileContent(workspace, compose) {
		return failEnvironmentLifecycleProjectionValidation(report, envPath, "content differs from the active Store projection")
	}
	return true
}

func readEnvironmentLifecycleProjectionFile(workspace string, relativePath string, expectedMode os.FileMode) (string, error) {
	raw, info, err := readEnvironmentRestoreWorkspaceFileWithInfo(workspace, relativePath)
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("projection must be a regular file")
	}
	if expectedMode != 0 && info.Mode().Perm() != expectedMode {
		return "", fmt.Errorf("projection mode is %04o; expected %04o", info.Mode().Perm(), expectedMode)
	}
	return string(raw), nil
}

func failEnvironmentLifecycleProjectionValidation(report *environmentStatusDockerReport, path string, reason string) bool {
	report.OK = false
	report.Action = "validate-lifecycle-projection"
	report.Error = fmt.Sprintf("environment lifecycle inspection is read-only; projection %s is not ready: %s; run environment restore --execute --prepare-repos-only to materialize the active Store projection", path, reason)
	return false
}

func environmentLifecycleComposeServices(compose map[string]any, workspace string) []string {
	services := dedupeStrings(stringSliceFromAny(compose["services"]))
	if len(services) == 0 {
		known, _, _ := environmentRestoreComposeServiceDefinitions(compose, workspace, environmentRestoreComposeFiles(compose))
		for service := range known {
			services = append(services, service)
		}
	}
	sort.Strings(services)
	return services
}
