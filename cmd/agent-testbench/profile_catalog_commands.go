package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"agent-testbench/internal/store"
)

func runProfileCatalog(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return errors.New("missing profile catalog command")
	}
	switch args[0] {
	case cliCommandList:
		return runProfileCatalogList(ctx, args[1:])
	case "restore":
		return runProfileCatalogRestore(ctx, args[1:])
	default:
		return fmt.Errorf("unknown profile catalog command: %s", args[0])
	}
}

func runProfileCatalogList(ctx context.Context, args []string) error {
	options, err := parseProfileCatalogReadOptions("profile catalog list", args)
	if err != nil {
		return err
	}
	report, err := listProfileCatalogs(ctx, options.StoreURL)
	if err != nil {
		return err
	}
	if options.JSONOutput {
		return writeIndentedJSON(report)
	}
	printProfileCatalogList(report)
	return nil
}

func runProfileCatalogRestore(ctx context.Context, args []string) error {
	flags := flag.NewFlagSet("profile catalog restore", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	profileID := flags.String("profile", "", "Profile id to restore from the Store catalog history")
	storeRef := flags.String("store", "", "Named Store config or Store DSN")
	storeURL := flags.String("store-url", "", legacyStoreURLFlagHelp)
	jsonOutput := flags.Bool("json", false, "Emit a machine-readable JSON report")
	if err := flags.Parse(args); err != nil {
		return err
	}
	targetProfileID := strings.TrimSpace(*profileID)
	if targetProfileID == "" {
		return errors.New("--profile is required")
	}
	resolvedStoreURL, err := resolveRequiredDailyStoreReference(*storeRef, *storeURL)
	if err != nil {
		return err
	}
	report, err := restoreProfileCatalog(ctx, resolvedStoreURL, targetProfileID)
	if err != nil {
		return err
	}
	if *jsonOutput {
		return writeIndentedJSON(report)
	}
	printProfileCatalogRestore(report)
	return nil
}

func listProfileCatalogs(ctx context.Context, storeURL string) (profileCatalogListReport, error) {
	runtime, err := openStore(ctx, storeURL)
	if err != nil {
		return profileCatalogListReport{}, err
	}
	defer closeCLIStore(runtime)
	indexes, err := runtime.ListProfileCatalogIndexes(ctx)
	if err != nil {
		return profileCatalogListReport{}, err
	}
	items := make([]profileCatalogIndex, 0, len(indexes))
	for _, item := range indexes {
		items = append(items, profileCatalogIndexFromStore(item))
	}
	return profileCatalogListReport{OK: true, Count: len(items), Items: items}, nil
}

func restoreProfileCatalog(ctx context.Context, storeURL string, profileID string) (profileCatalogRestoreReport, error) {
	runtime, err := openStore(ctx, storeURL)
	if err != nil {
		return profileCatalogRestoreReport{}, err
	}
	defer closeCLIStore(runtime)
	before, err := runtime.GetProfileCatalogIndex(ctx)
	if err != nil {
		return profileCatalogRestoreReport{}, err
	}
	catalog, err := runtime.GetProfileCatalogByID(ctx, profileID)
	if err != nil {
		return profileCatalogRestoreReport{}, fmt.Errorf("read profile catalog %q: %w", profileID, err)
	}
	restoredAt := time.Now().UTC()
	catalog.IndexedAt = restoredAt
	if err := runtime.ReplaceProfileCatalog(ctx, catalog); err != nil {
		return profileCatalogRestoreReport{}, fmt.Errorf("restore profile catalog %q: %w", profileID, err)
	}
	notes := []string{}
	var configVersion *profileConfigVersion
	if version, err := runtime.ActivateLatestConfigVersion(ctx, profileID); err == nil {
		value := profileConfigVersionFromStore(version)
		configVersion = &value
	} else if errors.Is(err, store.ErrNotFound) {
		notes = append(notes, "no config version found for restored profile; restored catalog only")
	} else {
		return profileCatalogRestoreReport{}, err
	}
	after, err := runtime.GetProfileCatalogIndex(ctx)
	if err != nil {
		return profileCatalogRestoreReport{}, err
	}
	return profileCatalogRestoreReport{
		OK:            true,
		ProfileID:     profileID,
		RestoredAt:    restoredAt,
		Before:        profileCatalogIndexFromStore(before),
		After:         profileCatalogIndexFromStore(after),
		ConfigVersion: configVersion,
		Notes:         notes,
	}, nil
}

type profileCatalogReadOptions struct {
	StoreURL   string
	JSONOutput bool
}

func parseProfileCatalogReadOptions(command string, args []string) (profileCatalogReadOptions, error) {
	flags := flag.NewFlagSet(command, flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	storeRef := flags.String("store", "", "Named Store config or Store DSN")
	storeURL := flags.String("store-url", "", legacyStoreURLFlagHelp)
	jsonOutput := flags.Bool("json", false, "Emit a machine-readable JSON report")
	if err := flags.Parse(args); err != nil {
		return profileCatalogReadOptions{}, err
	}
	resolvedStoreURL, err := resolveRequiredDailyStoreReference(*storeRef, *storeURL)
	if err != nil {
		return profileCatalogReadOptions{}, err
	}
	return profileCatalogReadOptions{StoreURL: resolvedStoreURL, JSONOutput: *jsonOutput}, nil
}
