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
