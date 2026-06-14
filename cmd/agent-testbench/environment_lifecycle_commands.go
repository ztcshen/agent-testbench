package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"

	"agent-testbench/internal/store"
)

type environmentLifecycleOptions struct {
	EnvironmentID string
	StoreRef      string
	StoreURL      string
	Workspace     string
	JSONOutput    bool
}

func parseEnvironmentLifecycleOptions(name string, args []string) (environmentLifecycleOptions, error) {
	flags := flag.NewFlagSet(name, flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	storeRef := flags.String("store", "", "Named Store config or Store DSN")
	storeURL := flags.String("store-url", "", legacyStoreURLFlagHelp)
	workspace := flags.String("workspace", "", "Local workspace for generated compose artifacts")
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
	resolvedStoreURL, err := resolveRequiredDailyStoreReference(*storeRef, *storeURL)
	if err != nil {
		return environmentLifecycleOptions{}, err
	}
	return environmentLifecycleOptions{
		EnvironmentID: id,
		StoreRef:      *storeRef,
		StoreURL:      resolvedStoreURL,
		Workspace:     *workspace,
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
