package main

import (
	"context"
	"errors"
	"flag"
	"os"
	"strings"

	"agent-testbench/internal/domain/profile"
	"agent-testbench/internal/store"
)

type profileDiscoveryCommandOptions struct {
	ProfilePath            string
	ProfileHome            string
	StoreRef               string
	StoreURL               string
	Filter                 string
	ServiceID              string
	OfflineTemplatePackage bool
	JSONOutput             bool
}

func parseProfileDiscoveryCommandOptions(command string, filterHelp string, args []string) (profileDiscoveryCommandOptions, error) {
	return parseProfileDiscoveryCommandOptionsWith(command, filterHelp, args, nil)
}

func parseProfileDiscoveryCommandOptionsWith(command string, filterHelp string, args []string, bindExtra func(*flag.FlagSet) func(*profileDiscoveryCommandOptions)) (profileDiscoveryCommandOptions, error) {
	flags := flag.NewFlagSet(command, flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	profilePath := flags.String("profile", "", "Template package path or installed template package id")
	profileHome := flags.String("profile-home", "", "Installed template package home")
	storeRef := flags.String("store", "", "Named Store config or Store DSN")
	storeURL := flags.String("store-url", "", legacyStoreURLFlagHelp)
	filter := flags.String("filter", "", filterHelp)
	offlineTemplatePackage := flags.Bool("offline-template-package", false, "Read the template package directly for offline review")
	jsonOutput := flags.Bool("json", false, "Emit a machine-readable JSON report")
	applyExtra := func(*profileDiscoveryCommandOptions) {}
	if bindExtra != nil {
		applyExtra = bindExtra(flags)
	}
	if err := flags.Parse(args); err != nil {
		return profileDiscoveryCommandOptions{}, err
	}
	options := profileDiscoveryCommandOptions{
		ProfilePath:            *profilePath,
		ProfileHome:            *profileHome,
		StoreRef:               *storeRef,
		StoreURL:               *storeURL,
		Filter:                 *filter,
		OfflineTemplatePackage: *offlineTemplatePackage,
		JSONOutput:             *jsonOutput,
	}
	applyExtra(&options)
	return options, nil
}

func (options profileDiscoveryCommandOptions) loadDiscoveryBundle(ctx context.Context) (profile.Bundle, func(), error) {
	discoveryProfileRef, resolvedStoreURL, err := resolveDiscoveryInputs(options.ProfilePath, options.StoreRef, options.StoreURL, options.OfflineTemplatePackage)
	if err != nil {
		return profile.Bundle{}, func() {}, err
	}
	bundle, _, cleanup, err := loadInterfaceNodeReportBundle(ctx, discoveryProfileRef, options.ProfileHome, resolvedStoreURL)
	if err != nil {
		return profile.Bundle{}, cleanup, err
	}
	return bundle, cleanup, nil
}

type profileWorkflowStoreCommandOptions struct {
	ProfilePath string
	ProfileHome string
	StoreRef    string
	StoreURL    string
	WorkflowID  string
	JSONOutput  bool
}

func parseProfileWorkflowStoreCommandOptions(command string, args []string, requireWorkflow bool) (profileWorkflowStoreCommandOptions, error) {
	flags := flag.NewFlagSet(command, flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	profilePath := flags.String("profile", "", "Template package path or installed template package id")
	profileHome := flags.String("profile-home", "", "Installed template package home")
	storeRef := flags.String("store", "", "Named Store config or Store DSN")
	storeURL := flags.String("store-url", "", legacyStoreURLFlagHelp)
	workflowID := flags.String("workflow", "", "Workflow id")
	jsonOutput := flags.Bool("json", false, "Emit a machine-readable JSON report")
	if err := flags.Parse(args); err != nil {
		return profileWorkflowStoreCommandOptions{}, err
	}
	if requireWorkflow && strings.TrimSpace(*workflowID) == "" {
		return profileWorkflowStoreCommandOptions{}, errors.New("--workflow is required")
	}
	return profileWorkflowStoreCommandOptions{
		ProfilePath: *profilePath,
		ProfileHome: *profileHome,
		StoreRef:    *storeRef,
		StoreURL:    *storeURL,
		WorkflowID:  *workflowID,
		JSONOutput:  *jsonOutput,
	}, nil
}

func (options profileWorkflowStoreCommandOptions) loadRequiredBundle(ctx context.Context) (profile.Bundle, store.Store, string, func(), error) {
	return loadRequiredInterfaceNodeReportBundleFromStoreFlags(ctx, options.ProfilePath, options.ProfileHome, options.StoreRef, options.StoreURL)
}
