package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"agent-testbench/internal/server/controlplane"
	"agent-testbench/internal/store"
)

func runEnvironment(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return errors.New("missing environment command")
	}
	switch args[0] {
	case "register":
		return runEnvironmentRegister(ctx, args[1:])
	case "discover":
		return runEnvironmentDiscover(ctx, args[1:])
	case "inspect":
		return runEnvironmentInspect(ctx, args[1:])
	case "bootstrap":
		return runEnvironmentBootstrap(ctx, args[1:])
	case "repo":
		return runEnvironmentRepo(ctx, args[1:])
	case "startup-file":
		return runEnvironmentStartupFile(ctx, args[1:])
	case "components":
		return runEnvironmentComponents(ctx, args[1:])
	case "restore":
		return runEnvironmentRestore(ctx, args[1:])
	case "acceptance":
		return runEnvironmentAcceptance(ctx, args[1:])
	case "verify":
		return runEnvironmentVerify(ctx, args[1:])
	case "publish-verified":
		return runEnvironmentPublishVerified(ctx, args[1:])
	default:
		return fmt.Errorf("unknown environment command: %s", args[0])
	}
}

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
	env := store.Environment{
		ID:                     strings.TrimSpace(*id),
		DisplayName:            strings.TrimSpace(*displayName),
		Description:            strings.TrimSpace(*description),
		Status:                 stringDefault(strings.TrimSpace(*status), "draft"),
		ServicesJSON:           mustCompactJSON(environmentServices(services, repos, branches, repoRefs, checkouts)),
		ReposJSON:              mustCompactJSON(environmentRepoMap(repos, branches, repoRefs, checkouts)),
		ComposeJSON:            mustCompactJSON(composeConfig),
		HealthChecksJSON:       mustCompactJSON(environmentHealthChecks(healthURLs, healthTCPs, healthCommands, healthComposeServices)),
		VerificationWorkflowID: strings.TrimSpace(*verificationWorkflowID),
		SummaryJSON:            mustCompactJSON(map[string]any{"source": "cli"}),
	}
	env, err = runtime.UpsertEnvironment(ctx, env)
	if err != nil {
		return err
	}
	return printEnvironmentCommandResult(env, *jsonOutput)
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
	flags := flag.NewFlagSet("environment inspect", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	storeRef := flags.String("store", "", "Named Store config or Store DSN")
	storeURL := flags.String("store-url", "", legacyStoreURLFlagHelp)
	jsonOutput := flags.Bool("json", false, "Emit a machine-readable JSON report")
	if err := parseInterspersedFlags(flags, args); err != nil {
		return err
	}
	id := strings.TrimSpace(flags.Arg(0))
	if id == "" {
		return errors.New("environment id is required")
	}
	env, componentGraph, err := loadEnvironmentAndComponentGraphForCLI(ctx, *storeRef, *storeURL, id)
	if err != nil {
		return err
	}
	return printEnvironmentCommandResult(env, *jsonOutput, componentGraph)
}

func runEnvironmentBootstrap(ctx context.Context, args []string) error {
	flags := flag.NewFlagSet("environment bootstrap", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	storeRef := flags.String("store", "", "Named Store config or Store DSN")
	storeURL := flags.String("store-url", "", legacyStoreURLFlagHelp)
	jsonOutput := flags.Bool("json", false, "Emit a machine-readable JSON report")
	if err := parseInterspersedFlags(flags, args); err != nil {
		return err
	}
	id := strings.TrimSpace(flags.Arg(0))
	if id == "" {
		return errors.New("environment id is required")
	}
	env, componentGraph, err := loadEnvironmentAndComponentGraphForCLI(ctx, *storeRef, *storeURL, id)
	if err != nil {
		return err
	}
	bootstrapPlan := controlplane.EnvironmentBootstrapPlan(env)
	componentReadiness := environmentRestoreComponentGraphReport(env.ID, componentGraph)
	componentStartupPlan := controlplane.EnvironmentComponentStartupPlanReport(env.ID, componentGraph)
	bootstrapPlan["componentGraph"] = componentReadiness
	bootstrapPlan["componentStartupPlan"] = componentStartupPlan
	if restorePlan, ok := bootstrapPlan["restore"].(map[string]any); ok {
		restorePlan["componentGraph"] = componentReadiness
		restorePlan["componentStartupPlan"] = componentStartupPlan
	}
	payload := map[string]any{
		"ok":          true,
		"environment": environmentPayload(env),
		"plan":        bootstrapPlan,
	}
	if *jsonOutput {
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

func runEnvironmentRepo(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return errors.New("missing environment repo command")
	}
	switch args[0] {
	case "set":
		return runEnvironmentRepoSet(ctx, args[1:])
	default:
		return fmt.Errorf("unknown environment repo command: %s", args[0])
	}
}

func runEnvironmentRepoSet(ctx context.Context, args []string) error {
	flags := flag.NewFlagSet("environment repo set", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	storeRef := flags.String("store", "", "Named Store config or Store DSN")
	storeURL := flags.String("store-url", "", legacyStoreURLFlagHelp)
	jsonOutput := flags.Bool("json", false, "Emit a machine-readable JSON report")
	var repos, branches, repoRefs, checkouts stringListFlag
	flags.Var(&repos, "repo", "Service repo as SERVICE=PATH_OR_URL; repeat for multiple services")
	flags.Var(&branches, "branch", "Service branch as SERVICE=BRANCH; repeat for multiple services")
	flags.Var(&repoRefs, "repo-ref", "Service Git ref as SERVICE=REF; repeat for multiple services")
	flags.Var(&checkouts, "checkout", "Service checkout path as SERVICE=PATH; repeat for multiple services")
	if err := parseInterspersedFlags(flags, args); err != nil {
		return err
	}
	id := strings.TrimSpace(flags.Arg(0))
	if id == "" {
		return errors.New("environment id is required")
	}
	updates := environmentRepoUpdateMap(repos, branches, repoRefs, checkouts)
	if len(updates) == 0 {
		return errors.New("at least one --repo, --branch, --repo-ref, or --checkout update is required")
	}
	runtime, cleanup, err := openRequiredCLIStore(ctx, *storeRef, *storeURL)
	if err != nil {
		return err
	}
	defer cleanup()
	env, err := runtime.GetEnvironment(ctx, id)
	if err != nil {
		return err
	}
	repoMap := jsonObjectString(env.ReposJSON)
	for serviceID, update := range updates {
		current := jsonObjectFromAny(repoMap[serviceID])
		for key, value := range update {
			if strings.TrimSpace(value) == "" {
				delete(current, key)
				continue
			}
			current[key] = value
		}
		repoMap[serviceID] = current
	}
	env.ReposJSON = mustCompactJSON(repoMap)
	env.ServicesJSON = mustCompactJSON(environmentServicesWithRepoUpdates(jsonArrayString(env.ServicesJSON), updates))
	env.UpdatedAt = time.Now().UTC()
	env, err = runtime.UpsertEnvironment(ctx, env)
	if err != nil {
		return err
	}
	payload := map[string]any{
		"ok":           true,
		"environment":  environmentPayload(env),
		"updatedRepos": updates,
	}
	if *jsonOutput {
		return writeIndentedJSON(payload)
	}
	fmt.Printf("Updated Environment Repositories: %s\n", env.ID)
	for _, serviceID := range sortedMapKeys(updates) {
		fmt.Printf("- %s\n", serviceID)
	}
	return nil
}

func runEnvironmentStartupFile(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return errors.New("missing environment startup-file command")
	}
	switch args[0] {
	case "put":
		return runEnvironmentStartupFilePut(ctx, args[1:])
	default:
		return fmt.Errorf("unknown environment startup-file command: %s", args[0])
	}
}

func runEnvironmentStartupFilePut(ctx context.Context, args []string) error {
	flags := flag.NewFlagSet("environment startup-file put", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	storeRef := flags.String("store", "", "Named Store config or Store DSN")
	storeURL := flags.String("store-url", "", legacyStoreURLFlagHelp)
	jsonOutput := flags.Bool("json", false, "Emit a machine-readable JSON report")
	var files stringListFlag
	flags.Var(&files, "file", "Generated startup file as TARGET=SOURCE_FILE; repeat for multiple files")
	if err := parseInterspersedFlags(flags, args); err != nil {
		return err
	}
	id := strings.TrimSpace(flags.Arg(0))
	if id == "" {
		return errors.New("environment id is required")
	}
	if len(files.Values()) == 0 {
		return errors.New("--file TARGET=SOURCE_FILE is required")
	}
	runtime, cleanup, err := openRequiredCLIStore(ctx, *storeRef, *storeURL)
	if err != nil {
		return err
	}
	defer cleanup()
	env, err := runtime.GetEnvironment(ctx, id)
	if err != nil {
		return err
	}
	generated, err := generatedFileContentMapFromFlags(files)
	if err != nil {
		return err
	}
	compose := jsonObjectString(env.ComposeJSON)
	current := stringMapFromAny(compose["generatedFiles"])
	for path, content := range generated {
		current[path] = content
	}
	compose["generatedFiles"] = current
	env.ComposeJSON = mustCompactJSON(compose)
	env.SummaryJSON = environmentStartupFileSummaryJSON(env.SummaryJSON, generated)
	env, err = runtime.UpsertEnvironment(ctx, env)
	if err != nil {
		return err
	}
	payload := map[string]any{
		"environment":    environmentPayload(env),
		"generatedFiles": environmentStartupFilePayload(generated),
	}
	if *jsonOutput {
		return writeIndentedJSON(payload)
	}
	fmt.Printf("Updated Environment Startup Files: %s\n", env.ID)
	for _, item := range environmentStartupFilePayload(generated) {
		fmt.Printf("- %s (%d bytes)\n", item["path"], item["bytes"])
	}
	return nil
}

func runEnvironmentComponents(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return errors.New("missing environment components command")
	}
	switch args[0] {
	case "inspect":
		return runEnvironmentComponentsInspect(ctx, args[1:])
	case "replace":
		return runEnvironmentComponentsReplace(ctx, args[1:])
	default:
		return fmt.Errorf("unknown environment components command: %s", args[0])
	}
}

func runEnvironmentComponentsInspect(ctx context.Context, args []string) error {
	flags := flag.NewFlagSet("environment components inspect", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	storeRef := flags.String("store", "", "Named Store config or Store DSN")
	storeURL := flags.String("store-url", "", legacyStoreURLFlagHelp)
	jsonOutput := flags.Bool("json", false, "Emit a machine-readable JSON report")
	if err := parseInterspersedFlags(flags, args); err != nil {
		return err
	}
	id := strings.TrimSpace(flags.Arg(0))
	if id == "" {
		return errors.New("environment id is required")
	}
	runtime, cleanup, err := openRequiredCLIStore(ctx, *storeRef, *storeURL)
	if err != nil {
		return err
	}
	defer cleanup()
	if _, err := runtime.GetEnvironment(ctx, id); err != nil {
		return err
	}
	graph, err := runtime.GetEnvironmentComponentGraph(ctx, id)
	if err != nil {
		return err
	}
	return printEnvironmentComponentGraph(id, graph, *jsonOutput)
}

func runEnvironmentComponentsReplace(ctx context.Context, args []string) error {
	flags := flag.NewFlagSet("environment components replace", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	storeRef := flags.String("store", "", "Named Store config or Store DSN")
	storeURL := flags.String("store-url", "", legacyStoreURLFlagHelp)
	file := flags.String("file", "", "Component graph JSON file")
	jsonOutput := flags.Bool("json", false, "Emit a machine-readable JSON report")
	if err := parseInterspersedFlags(flags, args); err != nil {
		return err
	}
	id := strings.TrimSpace(flags.Arg(0))
	if id == "" {
		return errors.New("environment id is required")
	}
	if strings.TrimSpace(*file) == "" {
		return errors.New("--file COMPONENT_GRAPH_JSON is required")
	}
	raw, err := os.ReadFile(strings.TrimSpace(*file))
	if err != nil {
		return err
	}
	var graph store.EnvironmentComponentGraph
	if err := json.Unmarshal(raw, &graph); err != nil {
		return fmt.Errorf("decode component graph JSON: %w", err)
	}
	runtime, cleanup, err := openRequiredCLIStore(ctx, *storeRef, *storeURL)
	if err != nil {
		return err
	}
	defer cleanup()
	if _, err := runtime.GetEnvironment(ctx, id); err != nil {
		return err
	}
	readiness := environmentRestoreComponentGraphReport(id, graph)
	if readiness.Configured && !readiness.OK {
		return fmt.Errorf("component graph restore readiness failed: %s", readiness.Error)
	}
	if err := runtime.ReplaceEnvironmentComponentGraph(ctx, id, graph); err != nil {
		return err
	}
	graph, err = runtime.GetEnvironmentComponentGraph(ctx, id)
	if err != nil {
		return err
	}
	return printEnvironmentComponentGraph(id, graph, *jsonOutput)
}

func printEnvironmentComponentGraph(envID string, graph store.EnvironmentComponentGraph, jsonOutput bool) error {
	readiness := environmentRestoreComponentGraphReport(envID, graph)
	payload := map[string]any{
		"ok":            true,
		"environmentId": envID,
		"componentGraph": map[string]any{
			"components":       graph.Components,
			"dependencies":     graph.Dependencies,
			"assets":           graph.Assets,
			"restoreReadiness": readiness,
			"counts": map[string]int{
				"components":   len(graph.Components),
				"dependencies": len(graph.Dependencies),
				"assets":       len(graph.Assets),
			},
		},
	}
	if jsonOutput {
		return writeIndentedJSON(payload)
	}
	fmt.Printf("Environment Component Graph: %s\n", envID)
	fmt.Printf("Components: %d\n", len(graph.Components))
	fmt.Printf("Dependencies: %d\n", len(graph.Dependencies))
	fmt.Printf("Assets: %d\n", len(graph.Assets))
	fmt.Printf("Restore-ready: %t\n", readiness.OK)
	if len(readiness.BlockingOrder) > 0 {
		fmt.Printf("Blocking order: %s\n", strings.Join(readiness.BlockingOrder, " -> "))
	}
	if strings.TrimSpace(readiness.Error) != "" {
		fmt.Printf("Readiness error: %s\n", readiness.Error)
	}
	for _, component := range graph.Components {
		label := strings.TrimSpace(component.DisplayName)
		if label == "" {
			label = component.ComponentID
		}
		fmt.Printf("- %s [%s/%s] compose=%s required=%t\n", component.ComponentID, component.Kind, component.Role, component.ComposeService, component.Required)
		if label != component.ComponentID {
			fmt.Printf("  name: %s\n", label)
		}
	}
	return nil
}

func environmentStartupFilePayload(files map[string]string) []map[string]any {
	paths := make([]string, 0, len(files))
	for path := range files {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	out := make([]map[string]any, 0, len(paths))
	for _, path := range paths {
		out = append(out, map[string]any{
			"path":  path,
			"bytes": len(files[path]),
		})
	}
	return out
}

func environmentStartupFileSummaryJSON(existing string, files map[string]string) string {
	summary := jsonObjectString(existing)
	summary["startupFiles"] = map[string]any{
		"updatedAt": time.Now().UTC().Format(time.RFC3339Nano),
		"files":     environmentStartupFilePayload(files),
	}
	return mustCompactJSON(summary)
}

func runEnvironmentAcceptance(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return errors.New("missing environment acceptance command")
	}
	switch args[0] {
	case "start":
		return runEnvironmentAcceptanceStart(ctx, args[1:])
	case "report":
		return runEnvironmentAcceptanceReport(ctx, args[1:])
	default:
		return fmt.Errorf("unknown environment acceptance command: %s", args[0])
	}
}

func runEnvironmentAcceptanceStart(ctx context.Context, args []string) error {
	flags := flag.NewFlagSet("environment acceptance start", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	serverURL := flags.String("server-url", "", "Running control plane base URL")
	requestID := flags.String("request-id", "", "Acceptance request id")
	baseURL := flags.String("base-url", "", "Base URL for live request execution")
	evidenceDir := flags.String("evidence-dir", "", "Evidence output directory")
	timeoutSeconds := flags.Int("timeout-seconds", 0, "Per-step timeout in seconds")
	jsonOutput := flags.Bool("json", false, "Emit machine-readable JSON")
	if err := parseInterspersedFlags(flags, args); err != nil {
		return err
	}
	envID := strings.TrimSpace(flags.Arg(0))
	if envID == "" || strings.TrimSpace(*serverURL) == "" || strings.TrimSpace(*requestID) == "" {
		return errors.New("environment id, --server-url, and --request-id are required")
	}
	payload := map[string]any{"requestId": strings.TrimSpace(*requestID)}
	if strings.TrimSpace(*baseURL) != "" {
		payload["baseUrl"] = strings.TrimSpace(*baseURL)
	}
	if strings.TrimSpace(*evidenceDir) != "" {
		payload["evidenceDir"] = strings.TrimSpace(*evidenceDir)
	}
	if *timeoutSeconds > 0 {
		payload["timeoutSeconds"] = *timeoutSeconds
	}
	result, err := postWorkflowAcceptanceJSON(ctx, environmentAcceptanceRunURL(*serverURL, envID, ""), payload)
	if err != nil {
		return err
	}
	if *jsonOutput {
		return writeIndentedJSON(result)
	}
	printEnvironmentAcceptanceStart(result)
	return nil
}

func runEnvironmentAcceptanceReport(ctx context.Context, args []string) error {
	flags := flag.NewFlagSet("environment acceptance report", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	serverURL := flags.String("server-url", "", "Running control plane base URL")
	runID := flags.String("run", "", "Acceptance batch run id")
	jsonOutput := flags.Bool("json", false, "Emit machine-readable JSON")
	if err := parseInterspersedFlags(flags, args); err != nil {
		return err
	}
	envID := strings.TrimSpace(flags.Arg(0))
	if envID == "" || strings.TrimSpace(*serverURL) == "" || strings.TrimSpace(*runID) == "" {
		return errors.New("environment id, --server-url, and --run are required")
	}
	result, err := fetchWorkflowAcceptanceJSON(ctx, environmentAcceptanceRunURL(*serverURL, envID, *runID))
	if err != nil {
		return err
	}
	if *jsonOutput {
		return writeIndentedJSON(result)
	}
	printEnvironmentAcceptanceReport(result)
	return nil
}

func environmentAcceptanceRunURL(serverURL string, envID string, runID string) string {
	base := strings.TrimRight(strings.TrimSpace(serverURL), "/") + "/api/environments/" + url.PathEscape(strings.TrimSpace(envID)) + "/acceptance-runs"
	if strings.TrimSpace(runID) != "" {
		base += "/" + url.PathEscape(strings.TrimSpace(runID))
	}
	return base
}

func composeHostSourceLooksLikePath(source string) bool {
	return strings.HasPrefix(source, ".") ||
		strings.HasPrefix(source, "/") ||
		strings.HasPrefix(source, "~") ||
		strings.HasPrefix(source, "$") ||
		strings.HasPrefix(source, "${")
}

func extractSandboxComposePaths(value string) []string {
	out := []string{}
	for _, field := range strings.FieldsFunc(value, func(r rune) bool {
		return r == '"' || r == '\'' || r == ',' || r == '[' || r == ']' || r == ' ' || r == '\t'
	}) {
		field = strings.TrimSpace(field)
		if !strings.HasPrefix(field, "/sandbox/compose/") {
			continue
		}
		out = append(out, filepath.Clean(strings.TrimPrefix(field, "/sandbox/")))
	}
	return out
}

func dedupeStrings(values []string) []string {
	out := make([]string, 0, len(values))
	seen := map[string]bool{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}

func leadingSpaceCount(value string) int {
	count := 0
	for _, r := range value {
		if r != ' ' {
			break
		}
		count++
	}
	return count
}

func runEnvironmentVerify(ctx context.Context, args []string) error {
	flags := flag.NewFlagSet("environment verify", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	storeRef := flags.String("store", "", "Named Store config or Store DSN")
	storeURL := flags.String("store-url", "", legacyStoreURLFlagHelp)
	runID := flags.String("run", "", "Verification run id")
	status := flags.String("status", "", "Verification status")
	evidenceComplete := flags.Bool("evidence-complete", false, "Evidence is complete for the verification run")
	topologyComplete := flags.Bool("topology-complete", false, "SkyWalking topology is complete for the verification run")
	jsonOutput := flags.Bool("json", false, "Emit a machine-readable JSON report")
	if err := parseInterspersedFlags(flags, args); err != nil {
		return err
	}
	id := strings.TrimSpace(flags.Arg(0))
	if id == "" {
		return errors.New("environment id is required")
	}
	if strings.TrimSpace(*runID) == "" || strings.TrimSpace(*status) == "" {
		return errors.New("--run and --status are required")
	}
	runtime, cleanup, err := openRequiredCLIStore(ctx, *storeRef, *storeURL)
	if err != nil {
		return err
	}
	defer cleanup()
	env, err := runtime.GetEnvironment(ctx, id)
	if err != nil {
		return err
	}
	env.LastVerificationRunID = strings.TrimSpace(*runID)
	env.LastVerificationStatus = strings.TrimSpace(*status)
	env.EvidenceComplete = *evidenceComplete
	env.TopologyComplete = *topologyComplete
	env.Verified = false
	env.Status = "verification-recorded"
	if env.LastVerificationStatus == store.StatusPassed && env.EvidenceComplete && env.TopologyComplete {
		env.Status = "verified-ready"
		env.LastVerifiedAt = time.Now().UTC()
	}
	env, err = runtime.UpsertEnvironment(ctx, env)
	if err != nil {
		return err
	}
	return printEnvironmentCommandResult(env, *jsonOutput)
}

func runEnvironmentPublishVerified(ctx context.Context, args []string) error {
	flags := flag.NewFlagSet("environment publish-verified", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	storeRef := flags.String("store", "", "Named Store config or Store DSN")
	storeURL := flags.String("store-url", "", legacyStoreURLFlagHelp)
	jsonOutput := flags.Bool("json", false, "Emit a machine-readable JSON report")
	if err := parseInterspersedFlags(flags, args); err != nil {
		return err
	}
	id := strings.TrimSpace(flags.Arg(0))
	if id == "" {
		return errors.New("environment id is required")
	}
	runtime, cleanup, err := openRequiredCLIStore(ctx, *storeRef, *storeURL)
	if err != nil {
		return err
	}
	defer cleanup()
	env, err := runtime.GetEnvironment(ctx, id)
	if err != nil {
		return err
	}
	if err := controlplane.ValidateEnvironmentPublishable(ctx, runtime, env); err != nil {
		return err
	}
	env.Verified = true
	env.Status = "verified"
	if env.LastVerifiedAt.IsZero() {
		env.LastVerifiedAt = time.Now().UTC()
	}
	env, err = runtime.UpsertEnvironment(ctx, env)
	if err != nil {
		return err
	}
	return printEnvironmentCommandResult(env, *jsonOutput)
}

func openRequiredCLIStore(ctx context.Context, storeRef string, legacyStoreURL string) (store.Store, func(), error) {
	resolvedStoreURL, err := resolveRequiredDailyStoreReference(storeRef, legacyStoreURL)
	if err != nil {
		return nil, func() {}, err
	}
	runtime, err := openStore(ctx, resolvedStoreURL)
	if err != nil {
		return nil, func() {}, err
	}
	return runtime, func() { _ = runtime.Close() }, nil
}

func loadEnvironmentForCLI(ctx context.Context, storeRef string, legacyStoreURL string, id string) (store.Environment, error) {
	runtime, cleanup, err := openRequiredCLIStore(ctx, storeRef, legacyStoreURL)
	if err != nil {
		return store.Environment{}, err
	}
	defer cleanup()
	return runtime.GetEnvironment(ctx, id)
}

func loadEnvironmentAndComponentGraphForCLI(ctx context.Context, storeRef string, legacyStoreURL string, id string) (store.Environment, store.EnvironmentComponentGraph, error) {
	runtime, cleanup, err := openRequiredCLIStore(ctx, storeRef, legacyStoreURL)
	if err != nil {
		return store.Environment{}, store.EnvironmentComponentGraph{}, err
	}
	defer cleanup()
	env, err := runtime.GetEnvironment(ctx, id)
	if err != nil {
		return store.Environment{}, store.EnvironmentComponentGraph{}, err
	}
	graph, err := runtime.GetEnvironmentComponentGraph(ctx, id)
	if err != nil {
		return store.Environment{}, store.EnvironmentComponentGraph{}, err
	}
	return env, graph, nil
}

func printEnvironmentCommandResult(env store.Environment, jsonOutput bool, componentGraphs ...store.EnvironmentComponentGraph) error {
	payload := map[string]any{"ok": true, "environment": environmentPayload(env)}
	if len(componentGraphs) > 0 {
		payload["componentGraph"] = environmentRestoreComponentGraphReport(env.ID, componentGraphs[0])
	}
	if jsonOutput {
		return writeIndentedJSON(payload)
	}
	fmt.Printf("Environment: %s\n", env.ID)
	fmt.Printf("Status: %s\n", env.Status)
	fmt.Printf("Verified: %t\n", env.Verified)
	if env.VerificationWorkflowID != "" {
		fmt.Printf("Verification Workflow: %s\n", env.VerificationWorkflowID)
	}
	if env.LastVerificationRunID != "" {
		fmt.Printf("Last Verification Run: %s [%s]\n", env.LastVerificationRunID, env.LastVerificationStatus)
	}
	fmt.Printf("Evidence Complete: %t\n", env.EvidenceComplete)
	fmt.Printf("SkyWalking Topology Complete: %t\n", env.TopologyComplete)
	if len(componentGraphs) > 0 {
		readiness := environmentRestoreComponentGraphReport(env.ID, componentGraphs[0])
		fmt.Printf("Component Restore-ready: %t\n", readiness.OK)
		if len(readiness.BlockingOrder) > 0 {
			fmt.Printf("Component Blocking Order: %s\n", strings.Join(readiness.BlockingOrder, " -> "))
		}
		if strings.TrimSpace(readiness.Error) != "" {
			fmt.Printf("Component Readiness Error: %s\n", readiness.Error)
		}
	}
	return nil
}

func environmentPayloads(items []store.Environment) []map[string]any {
	out := make([]map[string]any, 0, len(items))
	for _, item := range items {
		out = append(out, environmentPayload(item))
	}
	return out
}

func environmentPayload(env store.Environment) map[string]any {
	payload := map[string]any{
		"id":                     env.ID,
		"displayName":            env.DisplayName,
		"description":            env.Description,
		"status":                 env.Status,
		"verified":               env.Verified,
		"services":               jsonArrayString(env.ServicesJSON),
		"repos":                  jsonObjectString(env.ReposJSON),
		"compose":                jsonObjectString(env.ComposeJSON),
		"healthChecks":           jsonArrayString(env.HealthChecksJSON),
		"verificationWorkflowId": env.VerificationWorkflowID,
		"lastVerificationRunId":  env.LastVerificationRunID,
		"lastVerificationStatus": env.LastVerificationStatus,
		"evidenceComplete":       env.EvidenceComplete,
		"topologyComplete":       env.TopologyComplete,
		"summary":                jsonObjectString(env.SummaryJSON),
		"createdAt":              env.CreatedAt,
		"updatedAt":              env.UpdatedAt,
	}
	if !env.LastVerifiedAt.IsZero() {
		payload["lastVerifiedAt"] = env.LastVerifiedAt
	}
	return payload
}

func environmentServices(services stringListFlag, repos stringListFlag, branches stringListFlag, repoRefs stringListFlag, checkouts stringListFlag) []map[string]any {
	repoByService := environmentKeyValueMap(repos)
	branchByService := environmentKeyValueMap(branches)
	refByService := environmentKeyValueMap(repoRefs)
	checkoutByService := environmentKeyValueMap(checkouts)
	ids := map[string]bool{}
	for _, id := range services.Values() {
		ids[id] = true
	}
	for id := range repoByService {
		ids[id] = true
	}
	for id := range branchByService {
		ids[id] = true
	}
	for id := range refByService {
		ids[id] = true
	}
	for id := range checkoutByService {
		ids[id] = true
	}
	ordered := make([]string, 0, len(ids))
	for id := range ids {
		ordered = append(ordered, id)
	}
	sort.Strings(ordered)
	out := make([]map[string]any, 0, len(ordered))
	for _, id := range ordered {
		item := map[string]any{"id": id}
		if repo := repoByService[id]; repo != "" {
			item["repo"] = repo
		}
		if branch := branchByService[id]; branch != "" {
			item["branch"] = branch
		}
		if ref := refByService[id]; ref != "" {
			item["ref"] = ref
		}
		if checkout := checkoutByService[id]; checkout != "" {
			item["checkout"] = checkout
		}
		out = append(out, item)
	}
	return out
}

func environmentRepoMap(repos stringListFlag, branches stringListFlag, repoRefs stringListFlag, checkouts stringListFlag) map[string]any {
	repoByService := environmentKeyValueMap(repos)
	branchByService := environmentKeyValueMap(branches)
	refByService := environmentKeyValueMap(repoRefs)
	checkoutByService := environmentKeyValueMap(checkouts)
	ids := map[string]bool{}
	for id := range repoByService {
		ids[id] = true
	}
	for id := range branchByService {
		ids[id] = true
	}
	for id := range refByService {
		ids[id] = true
	}
	for id := range checkoutByService {
		ids[id] = true
	}
	out := map[string]any{}
	for id := range ids {
		item := map[string]any{}
		if repo := repoByService[id]; repo != "" {
			item["url"] = repo
		}
		if branch := branchByService[id]; branch != "" {
			item["branch"] = branch
		}
		if ref := refByService[id]; ref != "" {
			item["ref"] = ref
		}
		if checkout := checkoutByService[id]; checkout != "" {
			item["checkout"] = checkout
		}
		out[id] = item
	}
	return out
}

func environmentComposeConfig(composeFiles stringListFlag, generatedFiles stringListFlag, startCommand string, projectName string, envFiles stringListFlag, envs stringListFlag, profiles stringListFlag, services stringListFlag, skipPull bool, skipBuild bool, packageRepo string, packageBranch string, packageRef string) (map[string]any, error) {
	files := composeFiles.Values()
	composeFile := ""
	if len(files) > 0 {
		composeFile = strings.TrimSpace(files[0])
	}
	out := map[string]any{
		"composeFile":  composeFile,
		"startCommand": strings.TrimSpace(startCommand),
	}
	if len(files) > 0 {
		out["composeFiles"] = files
	}
	generated, err := generatedFileContentMapFromFlags(generatedFiles)
	if err != nil {
		return nil, err
	}
	if len(generated) > 0 {
		out["generatedFiles"] = generated
	}
	if strings.TrimSpace(projectName) != "" {
		out["projectName"] = strings.TrimSpace(projectName)
	}
	if len(envFiles.Values()) > 0 {
		out["envFiles"] = envFiles.Values()
	}
	if values := keyValueMapFromFlags(envs); len(values) > 0 {
		out["env"] = values
	}
	if len(profiles.Values()) > 0 {
		out["profiles"] = profiles.Values()
	}
	if len(services.Values()) > 0 {
		out["services"] = services.Values()
	}
	if skipPull {
		out["skipPull"] = true
	}
	if skipBuild {
		out["skipBuild"] = true
	}
	packageConfig := map[string]string{}
	if strings.TrimSpace(packageRepo) != "" {
		packageConfig["url"] = strings.TrimSpace(packageRepo)
	}
	if strings.TrimSpace(packageBranch) != "" {
		packageConfig["branch"] = strings.TrimSpace(packageBranch)
	}
	if strings.TrimSpace(packageRef) != "" {
		packageConfig["ref"] = strings.TrimSpace(packageRef)
	}
	if len(packageConfig) > 0 {
		packageConfig["checkout"] = "."
		out["package"] = packageConfig
	}
	return out, nil
}

func generatedFileContentMapFromFlags(values stringListFlag) (map[string]string, error) {
	out := map[string]string{}
	for _, raw := range values.Values() {
		target, source, ok := strings.Cut(raw, "=")
		target = strings.TrimSpace(target)
		source = strings.TrimSpace(source)
		if !ok || target == "" || source == "" {
			return nil, fmt.Errorf("generated compose file must be TARGET=SOURCE_FILE, got %q", raw)
		}
		if filepath.IsAbs(target) || target == "." || target == ".." || strings.HasPrefix(filepath.Clean(target), ".."+string(os.PathSeparator)) {
			return nil, fmt.Errorf("generated compose file target must be relative to the restore workspace: %s", target)
		}
		content, err := os.ReadFile(source)
		if err != nil {
			return nil, fmt.Errorf("read generated compose source %s: %w", source, err)
		}
		out[filepath.Clean(target)] = string(content)
	}
	return out, nil
}

func keyValueMapFromFlags(values stringListFlag) map[string]string {
	out := map[string]string{}
	for _, raw := range values.Values() {
		key, value, ok := strings.Cut(raw, "=")
		key = strings.TrimSpace(key)
		if !ok || key == "" {
			continue
		}
		out[key] = strings.TrimSpace(value)
	}
	return out
}

func environmentHealthChecks(urls stringListFlag, tcpAddresses stringListFlag, commands stringListFlag, composeServices stringListFlag) []map[string]any {
	out := make([]map[string]any, 0, len(urls.Values())+len(tcpAddresses.Values())+len(commands.Values())+len(composeServices.Values()))
	index := 1
	for _, url := range urls.Values() {
		out = append(out, map[string]any{"id": fmt.Sprintf("health-%02d", index), "kind": "url", "url": url})
		index++
	}
	for _, address := range tcpAddresses.Values() {
		out = append(out, map[string]any{"id": fmt.Sprintf("health-%02d", index), "kind": "tcp", "address": address})
		index++
	}
	for _, command := range commands.Values() {
		out = append(out, map[string]any{"id": fmt.Sprintf("health-%02d", index), "kind": "command", "command": command})
		index++
	}
	for _, service := range composeServices.Values() {
		out = append(out, map[string]any{"id": fmt.Sprintf("health-%02d", index), "kind": "compose-service", "service": service})
		index++
	}
	return out
}

func environmentRepoUpdateMap(repos stringListFlag, branches stringListFlag, repoRefs stringListFlag, checkouts stringListFlag) map[string]map[string]string {
	repoByService := environmentKeyValueMap(repos)
	branchByService := environmentKeyValueMap(branches)
	refByService := environmentKeyValueMap(repoRefs)
	checkoutByService := environmentKeyValueMap(checkouts)
	updates := map[string]map[string]string{}
	add := func(serviceID, key, value string) {
		serviceID = strings.TrimSpace(serviceID)
		if serviceID == "" {
			return
		}
		if _, ok := updates[serviceID]; !ok {
			updates[serviceID] = map[string]string{}
		}
		updates[serviceID][key] = value
	}
	for serviceID, value := range repoByService {
		add(serviceID, "url", value)
	}
	for serviceID, value := range branchByService {
		add(serviceID, "branch", value)
	}
	for serviceID, value := range refByService {
		add(serviceID, "ref", value)
	}
	for serviceID, value := range checkoutByService {
		add(serviceID, "checkout", value)
	}
	return updates
}

func environmentServicesWithRepoUpdates(existing []any, updates map[string]map[string]string) []any {
	out := make([]any, 0, len(existing)+len(updates))
	seen := map[string]bool{}
	for _, raw := range existing {
		item := jsonObjectFromAny(raw)
		serviceID := strings.TrimSpace(valueString(item["id"]))
		if serviceID == "" {
			continue
		}
		if update, ok := updates[serviceID]; ok {
			applyEnvironmentServiceRepoUpdate(item, update)
		}
		seen[serviceID] = true
		out = append(out, item)
	}
	for _, serviceID := range sortedMapKeys(updates) {
		if seen[serviceID] {
			continue
		}
		item := map[string]any{"id": serviceID}
		applyEnvironmentServiceRepoUpdate(item, updates[serviceID])
		out = append(out, item)
	}
	return out
}

func environmentKeyValueMap(values stringListFlag) map[string]string {
	out := map[string]string{}
	for _, value := range values.Values() {
		key, raw, ok := strings.Cut(value, "=")
		key = strings.TrimSpace(key)
		raw = strings.TrimSpace(raw)
		if !ok || key == "" || raw == "" {
			continue
		}
		out[key] = raw
	}
	return out
}

func printEnvironmentAcceptanceStart(payload map[string]any) {
	fmt.Printf("Environment Acceptance Run: %s\n", valueString(payload["batchRunId"]))
	fmt.Printf("Environment: %s\n", valueString(payload["environmentId"]))
	fmt.Printf("Workflow: %s\n", valueString(payload["workflowId"]))
	fmt.Printf("Status: %s\n", valueString(payload["status"]))
	fmt.Printf("Report: %s\n", valueString(payload["reportUrl"]))
}

func printEnvironmentAcceptanceReport(payload map[string]any) {
	acceptance := mapFromReportAny(payload["acceptance"])
	health := mapFromReportAny(acceptance["healthSummary"])
	fmt.Printf("Environment Acceptance Report: %s\n", valueString(payload["batchRunId"]))
	fmt.Printf("Environment: %s\n", valueString(payload["environmentId"]))
	fmt.Printf("Workflow: %s\n", firstNonEmpty(valueString(acceptance["workflowId"]), valueString(payload["workflowId"])))
	fmt.Printf("Status: %s\n", valueString(payload["status"]))
	fmt.Printf("Accepted: %t\n", boolFromReportAny(acceptance["ok"]))
	fmt.Printf("Health: %d/%d\n", intFromReportAny(health["passed"]), intFromReportAny(health["total"]))
}
