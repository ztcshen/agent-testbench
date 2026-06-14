package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"

	"agent-testbench/internal/environmentprojection"
	"agent-testbench/internal/server/controlplane"
	"agent-testbench/internal/store"
)

func runEnvironmentRegister(ctx context.Context, args []string) error {
	flags := flag.NewFlagSet("environment register", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	storeRef := flags.String("store", "", "Named Store config or Store DSN")
	storeURL := flags.String("store-url", "", legacyStoreURLFlagHelp)
	id := flags.String("id", "", "Environment id")
	displayName := flags.String("display-name", "", "Environment display name")
	description := flags.String("description", "", "Environment description")
	status := flags.String("status", "draft", "Environment status")
	verificationWorkflowID := flags.String("verification-workflow", "", "Verification workflow id")
	composeProjectName := flags.String("compose-project-name", "", "Docker Compose project name")
	composeSkipPull := flags.Bool("compose-skip-pull", false, "Skip Docker Compose image pull during restore")
	composeSkipBuild := flags.Bool("compose-skip-build", false, "Skip Docker Compose build during restore")
	packageRepo := flags.String("package-repo", "", "Environment package Git URL containing compose files and local validation assets")
	packageBranch := flags.String("package-branch", "", "Environment package Git branch")
	packageRef := flags.String("package-ref", "", "Environment package Git ref to checkout detached")
	startCommand := flags.String("start-command", "", "Local startup command")
	jsonOutput := flags.Bool("json", false, "Emit a machine-readable JSON report")
	var services, repos, branches, repoRefs, checkouts, healthURLs, healthTCPs, healthCommands, healthComposeServices, composeFiles, composeGeneratedFiles, composeEnvFiles, composeEnvs, composeProfiles, composeServices stringListFlag
	flags.Var(&services, "service", "Service id; repeat for multiple services")
	flags.Var(&repos, "repo", "Service repo as SERVICE=PATH_OR_URL; repeat for multiple services")
	flags.Var(&branches, "branch", "Service branch as SERVICE=BRANCH; repeat for multiple services")
	flags.Var(&repoRefs, "repo-ref", "Service Git ref as SERVICE=REF; repeat for multiple services")
	flags.Var(&checkouts, "checkout", "Service checkout path as SERVICE=PATH; repeat for multiple services")
	flags.Var(&composeFiles, "compose-file", "Local compose file path; repeat for multiple compose files")
	flags.Var(&composeGeneratedFiles, "compose-generated-file", "Store-backed generated file as TARGET=SOURCE_FILE; repeat for compose/env startup files")
	flags.Var(&composeEnvFiles, "compose-env-file", "Docker Compose env file path; repeat for multiple files")
	flags.Var(&composeEnvs, "compose-env", "Generated Docker Compose env entry as KEY=VALUE; repeat for multiple entries")
	flags.Var(&composeProfiles, "compose-profile", "Docker Compose profile; repeat for multiple profiles")
	flags.Var(&composeServices, "compose-service", "Docker Compose service to start; repeat for multiple services")
	flags.Var(&healthURLs, "health-url", "Health check URL; repeat for multiple checks")
	flags.Var(&healthTCPs, "health-tcp", "TCP health check address as HOST:PORT; repeat for multiple checks")
	flags.Var(&healthCommands, "health-command", "Shell command health check; repeat for multiple checks")
	flags.Var(&healthComposeServices, "health-compose-service", "Docker Compose service health check; repeat for multiple services")
	if err := parseInterspersedFlags(flags, args); err != nil {
		return err
	}
	if strings.TrimSpace(*id) == "" {
		return errors.New("--id is required")
	}
	if strings.TrimSpace(*verificationWorkflowID) == "" {
		return errors.New("--verification-workflow is required for environment acceptance")
	}
	runtime, cleanup, err := openRequiredCLIStore(ctx, *storeRef, *storeURL)
	if err != nil {
		return err
	}
	defer cleanup()
	composeConfig, err := environmentComposeConfig(composeFiles, composeGeneratedFiles, *startCommand, *composeProjectName, composeEnvFiles, composeEnvs, composeProfiles, composeServices, *composeSkipPull, *composeSkipBuild, *packageRepo, *packageBranch, *packageRef)
	if err != nil {
		return err
	}
	environmentFiles := environmentFilesFromComposeConfig(composeConfig)
	environmentServices := environmentServiceRows(services, repos, branches, repoRefs, checkouts)
	environmentHealthChecks := environmentHealthCheckRows(healthURLs, healthTCPs, healthCommands, healthComposeServices)
	env := store.Environment{
		ID:                     strings.TrimSpace(*id),
		DisplayName:            strings.TrimSpace(*displayName),
		Description:            strings.TrimSpace(*description),
		Status:                 stringDefault(strings.TrimSpace(*status), "draft"),
		ServicesJSON:           "[]",
		ReposJSON:              "{}",
		ComposeJSON:            mustCompactJSON(environmentComposeConfigWithoutGeneratedFiles(composeConfig)),
		HealthChecksJSON:       "[]",
		VerificationWorkflowID: strings.TrimSpace(*verificationWorkflowID),
		SummaryJSON:            mustCompactJSON(map[string]any{"source": "cli"}),
	}
	env, err = upsertEnvironmentRegistrationState(ctx, runtime, env, environmentFiles, environmentServices, environmentHealthChecks)
	if err != nil {
		return err
	}
	if len(environmentFiles) > 0 || len(environmentServices) > 0 || len(environmentHealthChecks) > 0 {
		env, err = runtime.GetEnvironment(ctx, env.ID)
		if err != nil {
			return err
		}
	}
	return printEnvironmentCommandResult(env, *jsonOutput)
}

type environmentStructuredStateUpserter interface {
	UpsertEnvironmentStructuredState(context.Context, store.Environment, []store.EnvironmentFile, []store.EnvironmentService, []store.EnvironmentHealthCheck) (store.Environment, error)
}

func upsertEnvironmentRegistrationState(ctx context.Context, runtime store.Store, env store.Environment, files []store.EnvironmentFile, services []store.EnvironmentService, checks []store.EnvironmentHealthCheck) (store.Environment, error) {
	if atomic, ok := runtime.(environmentStructuredStateUpserter); ok {
		return atomic.UpsertEnvironmentStructuredState(ctx, env, files, services, checks)
	}
	if err := store.ValidateEnvironmentFiles(env.ID, store.NormalizeEnvironmentFiles(files)); err != nil {
		return store.Environment{}, err
	}
	if err := store.ValidateEnvironmentServices(env.ID, store.NormalizeEnvironmentServices(services)); err != nil {
		return store.Environment{}, err
	}
	if err := store.ValidateEnvironmentHealthChecks(env.ID, store.NormalizeEnvironmentHealthChecks(checks)); err != nil {
		return store.Environment{}, err
	}
	written, err := runtime.UpsertEnvironment(ctx, env)
	if err != nil {
		return store.Environment{}, err
	}
	if err := runtime.ReplaceEnvironmentFiles(ctx, written.ID, files); err != nil {
		return store.Environment{}, err
	}
	if err := runtime.ReplaceEnvironmentServices(ctx, written.ID, services); err != nil {
		return store.Environment{}, err
	}
	if err := runtime.ReplaceEnvironmentHealthChecks(ctx, written.ID, checks); err != nil {
		return store.Environment{}, err
	}
	return written, nil
}

func runEnvironmentDiscover(ctx context.Context, args []string) error {
	flags := flag.NewFlagSet("environment discover", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	storeRef := flags.String("store", "", "Named Store config or Store DSN")
	storeURL := flags.String("store-url", "", legacyStoreURLFlagHelp)
	includeAll := flags.Bool("all", false, "Include environments that are not verified")
	jsonOutput := flags.Bool("json", false, "Emit a machine-readable JSON report")
	if err := parseInterspersedFlags(flags, args); err != nil {
		return err
	}
	runtime, cleanup, err := openRequiredCLIStore(ctx, *storeRef, *storeURL)
	if err != nil {
		return err
	}
	defer cleanup()
	items, err := runtime.ListEnvironments(ctx)
	if err != nil {
		return err
	}
	filtered := make([]store.Environment, 0, len(items))
	for _, item := range items {
		if *includeAll || item.Verified {
			filtered = append(filtered, item)
		}
	}
	payload := map[string]any{"ok": true, "count": len(filtered), "items": environmentPayloads(filtered)}
	if *jsonOutput {
		return writeIndentedJSON(payload)
	}
	fmt.Printf("Environments: %d\n", len(filtered))
	for _, item := range filtered {
		fmt.Printf("- %s [%s] verified=%t workflow=%s\n", item.ID, item.Status, item.Verified, item.VerificationWorkflowID)
	}
	return nil
}

func runEnvironmentInspect(ctx context.Context, args []string) error {
	options, err := parseEnvironmentIDFlags("environment inspect", args)
	if err != nil {
		return err
	}
	env, componentGraph, files, err := loadEnvironmentAndComponentGraphForCLI(ctx, options.StoreRef, options.StoreURL, options.ID)
	if err != nil {
		return err
	}
	return printEnvironmentCommandResultWithFiles(env, options.JSONOutput, componentGraph, files)
}

func runEnvironmentBootstrap(ctx context.Context, args []string) error {
	options, err := parseEnvironmentIDFlags("environment bootstrap", args)
	if err != nil {
		return err
	}
	env, componentGraph, files, err := loadEnvironmentAndComponentGraphForCLI(ctx, options.StoreRef, options.StoreURL, options.ID)
	if err != nil {
		return err
	}
	bootstrapPlan := controlplane.EnvironmentBootstrapPlan(env)
	componentReadiness := environmentRestoreComponentGraphReport(env.ID, componentGraph)
	componentStartupPlan := controlplane.EnvironmentComponentStartupPlanReport(env.ID, componentGraph)
	fileProjection := environmentprojection.FromEnvironmentWithEnvironmentFiles(env, componentGraph, files)
	bootstrapPlan["componentGraph"] = componentReadiness
	bootstrapPlan["componentStartupPlan"] = componentStartupPlan
	bootstrapPlan["fileProjection"] = fileProjection
	if restorePlan, ok := bootstrapPlan["restore"].(map[string]any); ok {
		restorePlan["componentGraph"] = componentReadiness
		restorePlan["componentStartupPlan"] = componentStartupPlan
		restorePlan["fileProjection"] = fileProjection
	}
	payload := map[string]any{
		"ok":          true,
		"environment": environmentPayload(env),
		"plan":        bootstrapPlan,
	}
	if options.JSONOutput {
		return writeIndentedJSON(payload)
	}
	fmt.Printf("Environment Bootstrap Plan: %s\n", env.ID)
	fmt.Printf("Verification Workflow: %s\n", env.VerificationWorkflowID)
	fmt.Printf("Component Restore-ready: %t\n", componentReadiness.OK)
	if len(componentReadiness.BlockingOrder) > 0 {
		fmt.Printf("Component Blocking Order: %s\n", strings.Join(componentReadiness.BlockingOrder, " -> "))
	}
	fmt.Printf("Repos: %s\n", env.ReposJSON)
	fmt.Printf("Compose: %s\n", env.ComposeJSON)
	return nil
}
