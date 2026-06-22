package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
)

type environmentConfigureOptions struct {
	view       string
	storeRef   string
	storeURL   string
	jsonOutput bool
	id         string
	repos      stringListFlag
	branches   stringListFlag
	repoRefs   stringListFlag
	checkouts  stringListFlag
	files      stringListFlag
}

const (
	environmentConfigureViewRepos      = "repos"
	environmentConfigureViewComponents = "components"
)

func runEnvironmentConfigure(ctx context.Context, args []string) error {
	options, err := parseEnvironmentConfigureOptions(args)
	if err != nil {
		return err
	}
	switch options.view {
	case environmentConfigureViewRepos:
		return runEnvironmentConfigureRepos(ctx, options)
	case "startup-files":
		if environmentConfigureHasRepoUpdates(options) {
			return errors.New("--repo, --branch, --repo-ref, and --checkout are only supported for --view repos")
		}
		if len(options.files.Values()) == 0 {
			return errors.New("--file TARGET=SOURCE_FILE is required")
		}
		return runEnvironmentStartupFilePutWithOptions(ctx, environmentStartupFilePutOptions{
			storeRef:   options.storeRef,
			storeURL:   options.storeURL,
			jsonOutput: options.jsonOutput,
			id:         options.id,
			files:      options.files,
		})
	case environmentConfigureViewComponents:
		return runEnvironmentConfigureComponents(ctx, options)
	default:
		return fmt.Errorf("unknown environment configure view: %s", options.view)
	}
}

func parseEnvironmentConfigureOptions(args []string) (environmentConfigureOptions, error) {
	flags := flag.NewFlagSet("environment configure", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	view := flags.String("view", "", "Configuration view: components, repos, startup-files")
	storeRef := flags.String("store", "", "Named Store config or Store DSN")
	storeURL := flags.String("store-url", "", legacyStoreURLFlagHelp)
	jsonOutput := flags.Bool("json", false, "Emit a machine-readable JSON report")
	var repos, branches, repoRefs, checkouts, files stringListFlag
	flags.Var(&repos, "repo", "Service repo as SERVICE=PATH_OR_URL; repeat for multiple services")
	flags.Var(&branches, "branch", "Service branch as SERVICE=BRANCH; repeat for multiple services")
	flags.Var(&repoRefs, "repo-ref", "Service Git ref as SERVICE=REF; repeat for multiple services")
	flags.Var(&checkouts, "checkout", "Service checkout path as SERVICE=PATH; repeat for multiple services")
	flags.Var(&files, "file", "Startup file as TARGET=SOURCE_FILE, or component graph JSON for --view components")
	if err := parseInterspersedFlags(flags, args); err != nil {
		return environmentConfigureOptions{}, err
	}
	id := strings.TrimSpace(flags.Arg(0))
	if id == "" {
		return environmentConfigureOptions{}, errors.New("environment id is required")
	}
	if flags.NArg() > 1 {
		return environmentConfigureOptions{}, fmt.Errorf("unexpected environment configure arguments: %s", strings.Join(flags.Args()[1:], " "))
	}
	normalizedView, err := normalizeEnvironmentConfigureView(*view)
	if err != nil {
		return environmentConfigureOptions{}, err
	}
	return environmentConfigureOptions{
		view:       normalizedView,
		storeRef:   *storeRef,
		storeURL:   *storeURL,
		jsonOutput: *jsonOutput,
		id:         id,
		repos:      repos,
		branches:   branches,
		repoRefs:   repoRefs,
		checkouts:  checkouts,
		files:      files,
	}, nil
}

func normalizeEnvironmentConfigureView(view string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(view)) {
	case "":
		return "", errors.New("environment configure --view is required")
	case "repo", environmentConfigureViewRepos, "repositories":
		return environmentConfigureViewRepos, nil
	case "startup-file", "startup-files":
		return "startup-files", nil
	case "component", environmentConfigureViewComponents:
		return environmentConfigureViewComponents, nil
	default:
		return "", fmt.Errorf("unknown environment configure view: %s", view)
	}
}

func runEnvironmentConfigureRepos(ctx context.Context, options environmentConfigureOptions) error {
	if len(options.files.Values()) > 0 {
		return errors.New("--file is only supported for --view startup-files or --view components")
	}
	updates := environmentRepoUpdateMap(options.repos, options.branches, options.repoRefs, options.checkouts)
	if len(updates) == 0 {
		return errors.New("at least one --repo, --branch, --repo-ref, or --checkout update is required")
	}
	return runEnvironmentRepoSetWithOptions(ctx, environmentRepoSetOptions{
		storeRef:   options.storeRef,
		storeURL:   options.storeURL,
		jsonOutput: options.jsonOutput,
		id:         options.id,
		updates:    updates,
	})
}

func runEnvironmentConfigureComponents(ctx context.Context, options environmentConfigureOptions) error {
	if environmentConfigureHasRepoUpdates(options) {
		return errors.New("--repo, --branch, --repo-ref, and --checkout are only supported for --view repos")
	}
	files := options.files.Values()
	if len(files) == 0 {
		return runEnvironmentComponentsInspectWithOptions(ctx, environmentIDFlags{
			StoreRef:   options.storeRef,
			StoreURL:   options.storeURL,
			ID:         options.id,
			JSONOutput: options.jsonOutput,
		})
	}
	if len(files) > 1 {
		return errors.New("--view components accepts at most one --file COMPONENT_GRAPH_JSON")
	}
	return runEnvironmentComponentsReplaceWithOptions(ctx, environmentComponentsReplaceOptions{
		storeRef:   options.storeRef,
		storeURL:   options.storeURL,
		jsonOutput: options.jsonOutput,
		id:         options.id,
		file:       files[0],
	})
}

func environmentConfigureHasRepoUpdates(options environmentConfigureOptions) bool {
	return len(options.repos.Values()) > 0 ||
		len(options.branches.Values()) > 0 ||
		len(options.repoRefs.Values()) > 0 ||
		len(options.checkouts.Values()) > 0
}
