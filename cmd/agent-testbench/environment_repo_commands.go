package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"agent-testbench/internal/store"
)

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
	opts, err := parseEnvironmentRepoSetOptions(args)
	if err != nil {
		return err
	}
	return runEnvironmentRepoSetWithOptions(ctx, opts)
}

func runEnvironmentRepoSetWithOptions(ctx context.Context, opts environmentRepoSetOptions) error {
	runtime, cleanup, err := openRequiredCLIStore(ctx, opts.storeRef, opts.storeURL)
	if err != nil {
		return err
	}
	defer cleanup()
	env, err := updateEnvironmentRepositories(ctx, runtime, opts.id, opts.updates)
	if err != nil {
		return err
	}
	payload := map[string]any{
		"ok":           true,
		"environment":  environmentPayload(env),
		"updatedRepos": opts.updates,
	}
	if opts.jsonOutput {
		return writeIndentedJSON(payload)
	}
	fmt.Printf("Updated Environment Repositories: %s\n", env.ID)
	for _, serviceID := range sortedMapKeys(opts.updates) {
		fmt.Printf("- %s\n", serviceID)
	}
	return nil
}

type environmentRepoSetOptions struct {
	storeRef   string
	storeURL   string
	jsonOutput bool
	id         string
	updates    map[string]map[string]string
}

func parseEnvironmentRepoSetOptions(args []string) (environmentRepoSetOptions, error) {
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
		return environmentRepoSetOptions{}, err
	}
	id := strings.TrimSpace(flags.Arg(0))
	if id == "" {
		return environmentRepoSetOptions{}, errors.New("environment id is required")
	}
	updates := environmentRepoUpdateMap(repos, branches, repoRefs, checkouts)
	if len(updates) == 0 {
		return environmentRepoSetOptions{}, errors.New("at least one --repo, --branch, --repo-ref, or --checkout update is required")
	}
	return environmentRepoSetOptions{
		storeRef:   *storeRef,
		storeURL:   *storeURL,
		jsonOutput: *jsonOutput,
		id:         id,
		updates:    updates,
	}, nil
}

func updateEnvironmentRepositories(ctx context.Context, runtime store.EnvironmentStore, id string, updates map[string]map[string]string) (store.Environment, error) {
	env, err := runtime.GetEnvironment(ctx, id)
	if err != nil {
		return store.Environment{}, err
	}
	services, err := runtime.ListEnvironmentServices(ctx, env.ID)
	if err != nil {
		return store.Environment{}, err
	}
	services = environmentServicesWithLegacyJSON(services, env.ServicesJSON, env.ReposJSON)
	services = storeEnvironmentServicesWithRepoUpdates(services, updates)
	services = store.NormalizeEnvironmentServices(services)
	env.ReposJSON = "{}"
	env.ServicesJSON = "[]"
	env.UpdatedAt = time.Now().UTC()
	env, err = runtime.UpsertEnvironment(ctx, env)
	if err != nil {
		return store.Environment{}, err
	}
	if err := runtime.ReplaceEnvironmentServices(ctx, env.ID, services); err != nil {
		return store.Environment{}, err
	}
	return runtime.GetEnvironment(ctx, env.ID)
}

func environmentServicesWithLegacyJSON(services []store.EnvironmentService, servicesJSON string, reposJSON string) []store.EnvironmentService {
	byID := map[string]store.EnvironmentService{}
	for _, service := range environmentServiceRowsFromJSON(jsonArrayString(servicesJSON), jsonObjectString(reposJSON)) {
		if strings.TrimSpace(service.ServiceID) != "" {
			byID[service.ServiceID] = service
		}
	}
	for _, service := range services {
		if strings.TrimSpace(service.ServiceID) != "" {
			byID[service.ServiceID] = service
		}
	}
	out := make([]store.EnvironmentService, 0, len(byID))
	for _, service := range byID {
		out = append(out, service)
	}
	return store.NormalizeEnvironmentServices(out)
}

func storeEnvironmentServicesWithRepoUpdates(services []store.EnvironmentService, updates map[string]map[string]string) []store.EnvironmentService {
	byID := map[string]store.EnvironmentService{}
	for _, service := range services {
		byID[service.ServiceID] = service
	}
	for serviceID, update := range updates {
		current := byID[serviceID]
		current.ServiceID = serviceID
		applyStoreEnvironmentServiceRepoUpdate(&current, update)
		current.SummaryJSON = `{"source":"environment.repo.set"}`
		byID[serviceID] = current
	}
	out := make([]store.EnvironmentService, 0, len(byID))
	for _, service := range byID {
		out = append(out, service)
	}
	return out
}

func applyStoreEnvironmentServiceRepoUpdate(service *store.EnvironmentService, update map[string]string) {
	for key, value := range update {
		value = strings.TrimSpace(value)
		switch key {
		case "url":
			service.RepoURL = value
		case "branch":
			service.Branch = value
		case "ref":
			service.Ref = value
		case "checkout":
			service.Checkout = value
		}
	}
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
	opts, err := parseEnvironmentStartupFilePutOptions(args)
	if err != nil {
		return err
	}
	return runEnvironmentStartupFilePutWithOptions(ctx, opts)
}

type environmentStartupFilePutOptions struct {
	storeRef   string
	storeURL   string
	jsonOutput bool
	id         string
	files      stringListFlag
}

func parseEnvironmentStartupFilePutOptions(args []string) (environmentStartupFilePutOptions, error) {
	flags := flag.NewFlagSet("environment startup-file put", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	storeRef := flags.String("store", "", "Named Store config or Store DSN")
	storeURL := flags.String("store-url", "", legacyStoreURLFlagHelp)
	jsonOutput := flags.Bool("json", false, "Emit a machine-readable JSON report")
	var files stringListFlag
	flags.Var(&files, "file", "Generated startup file as TARGET=SOURCE_FILE; repeat for multiple files")
	if err := parseInterspersedFlags(flags, args); err != nil {
		return environmentStartupFilePutOptions{}, err
	}
	id := strings.TrimSpace(flags.Arg(0))
	if id == "" {
		return environmentStartupFilePutOptions{}, errors.New("environment id is required")
	}
	if len(files.Values()) == 0 {
		return environmentStartupFilePutOptions{}, errors.New("--file TARGET=SOURCE_FILE is required")
	}
	return environmentStartupFilePutOptions{
		storeRef:   *storeRef,
		storeURL:   *storeURL,
		jsonOutput: *jsonOutput,
		id:         id,
		files:      files,
	}, nil
}

func runEnvironmentStartupFilePutWithOptions(ctx context.Context, opts environmentStartupFilePutOptions) error {
	runtime, cleanup, err := openRequiredCLIStore(ctx, opts.storeRef, opts.storeURL)
	if err != nil {
		return err
	}
	defer cleanup()
	env, err := runtime.GetEnvironment(ctx, opts.id)
	if err != nil {
		return err
	}
	generated, err := generatedFileContentMapFromFlags(opts.files)
	if err != nil {
		return err
	}
	compose := jsonObjectString(env.ComposeJSON)
	existingFiles, err := runtime.ListEnvironmentFiles(ctx, env.ID)
	if err != nil {
		return err
	}
	existingFiles = environmentFilesWithLegacyCompose(existingFiles, compose)
	current := map[string]string{}
	for path, content := range generated {
		current[path] = content
	}
	compose["generatedFiles"] = current
	updatedFiles := environmentFilesForGeneratedUpdates(compose, generated)
	env.ComposeJSON = mustCompactJSON(environmentComposeConfigWithoutGeneratedFiles(compose))
	env.SummaryJSON = environmentStartupFileSummaryJSON(env.SummaryJSON, generated)
	env, err = runtime.UpsertEnvironment(ctx, env)
	if err != nil {
		return err
	}
	if err := runtime.ReplaceEnvironmentFiles(ctx, env.ID, mergeEnvironmentFiles(existingFiles, updatedFiles)); err != nil {
		return err
	}
	env, err = runtime.GetEnvironment(ctx, env.ID)
	if err != nil {
		return err
	}
	payload := map[string]any{
		"environment":    environmentPayload(env),
		"generatedFiles": environmentStartupFilePayload(generated),
	}
	if opts.jsonOutput {
		return writeIndentedJSON(payload)
	}
	fmt.Printf("Updated Environment Startup Files: %s\n", env.ID)
	for _, item := range environmentStartupFilePayload(generated) {
		fmt.Printf("- %s (%d bytes)\n", item["path"], item["bytes"])
	}
	return nil
}

func environmentFilesWithLegacyCompose(files []store.EnvironmentFile, compose map[string]any) []store.EnvironmentFile {
	return mergeEnvironmentFiles(environmentFilesFromComposeConfig(compose), files)
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
