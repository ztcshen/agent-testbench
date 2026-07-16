package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"
)

type caseCatalogHistoryReport struct {
	OK        bool                        `json:"ok"`
	ProfileID string                      `json:"profileId"`
	Revision  int64                       `json:"revision"`
	Versions  []caseCatalogHistoryVersion `json:"versions"`
}

type caseCatalogHistoryVersion struct {
	Revision  int64          `json:"revision"`
	SHA256    string         `json:"sha256"`
	Operation string         `json:"operation"`
	Summary   map[string]any `json:"summary,omitempty"`
	CreatedAt time.Time      `json:"createdAt"`
}

type caseCatalogRollbackReport struct {
	OK             bool   `json:"ok"`
	ProfileID      string `json:"profileId"`
	SourceRevision int64  `json:"sourceRevision"`
	BeforeRevision int64  `json:"beforeRevision"`
	Revision       int64  `json:"revision"`
}

func runCaseCatalogHistory(ctx context.Context, args []string) error {
	flags := flag.NewFlagSet("case catalog history", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	storeRef := flags.String("store", "", "Named Store config or Store DSN")
	storeURL := flags.String("store-url", "", legacyStoreURLFlagHelp)
	profileID := flags.String("profile", "", "Profile id; defaults to the current Store catalog")
	limit := flags.Int("limit", 50, "Maximum history entries")
	jsonOutput := flags.Bool("json", false, "Emit a machine-readable JSON report")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("unexpected case catalog history arguments: %s", strings.Join(flags.Args(), " "))
	}
	if *limit <= 0 || *limit > 500 {
		return errors.New("--limit must be between 1 and 500")
	}
	storeDSN, err := resolveRequiredDailyStoreReference(*storeRef, *storeURL)
	if err != nil {
		return err
	}
	runtime, err := openStore(ctx, storeDSN)
	if err != nil {
		return err
	}
	defer closeCLIStore(runtime)
	snapshot, err := loadMutableProfileCatalogSnapshot(ctx, runtime, *profileID)
	if err != nil {
		return err
	}
	versioned, err := requireVersionedProfileCatalogStore(runtime)
	if err != nil {
		return err
	}
	versions, err := versioned.ListProfileCatalogVersions(ctx, snapshot.Catalog.ProfileID, *limit)
	if err != nil {
		return err
	}
	report := caseCatalogHistoryReport{
		OK:        true,
		ProfileID: snapshot.Catalog.ProfileID,
		Revision:  snapshot.Revision,
		Versions:  make([]caseCatalogHistoryVersion, 0, len(versions)),
	}
	for _, version := range versions {
		report.Versions = append(report.Versions, caseCatalogHistoryVersion{
			Revision:  version.Revision,
			SHA256:    version.SHA256,
			Operation: version.Operation,
			Summary:   safeCatalogMutationSummary(version.SummaryJSON),
			CreatedAt: version.CreatedAt,
		})
	}
	if *jsonOutput {
		return writeIndentedJSON(report)
	}
	fmt.Printf("Case catalog %s is at revision %d\n", report.ProfileID, report.Revision)
	for _, version := range report.Versions {
		fmt.Printf("r%d  %s  %s\n", version.Revision, version.Operation, version.CreatedAt.Format(time.RFC3339))
	}
	return nil
}

func runCaseCatalogRollback(ctx context.Context, args []string) error {
	flags := flag.NewFlagSet("case catalog rollback", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	storeRef := flags.String("store", "", "Named Store config or Store DSN")
	storeURL := flags.String("store-url", "", legacyStoreURLFlagHelp)
	profileID := flags.String("profile", "", "Profile id; defaults to the current Store catalog")
	sourceRevision := flags.Int64("revision", 0, "Historical catalog revision to restore")
	expectedRevision := flags.Int64("expected-revision", -1, "Required current catalog revision for optimistic concurrency")
	jsonOutput := flags.Bool("json", false, "Emit a machine-readable JSON report")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("unexpected case catalog rollback arguments: %s", strings.Join(flags.Args(), " "))
	}
	if *sourceRevision <= 0 {
		return errors.New("--revision must be positive")
	}
	passedFlags := parsedFlagNames(flags)
	if passedFlags["expected-revision"] && *expectedRevision < 0 {
		return errors.New("--expected-revision must be non-negative")
	}
	storeDSN, err := resolveRequiredDailyStoreReference(*storeRef, *storeURL)
	if err != nil {
		return err
	}
	runtime, err := openStore(ctx, storeDSN)
	if err != nil {
		return err
	}
	defer closeCLIStore(runtime)
	var report caseCatalogRollbackReport
	err = withProfileCatalogWriteLock(storeDSN, func() error {
		snapshot, err := loadMutableProfileCatalogSnapshot(ctx, runtime, *profileID)
		if err != nil {
			return err
		}
		if err := requireExpectedProfileCatalogRevision(
			snapshot.Catalog.ProfileID,
			snapshot.Revision,
			optionalExpectedProfileCatalogRevision(passedFlags["expected-revision"], *expectedRevision),
		); err != nil {
			return err
		}
		versioned, err := requireVersionedProfileCatalogStore(runtime)
		if err != nil {
			return err
		}
		target, err := versioned.GetProfileCatalogVersion(ctx, snapshot.Catalog.ProfileID, *sourceRevision)
		if err != nil {
			return err
		}
		if target.SHA256 == snapshot.SHA256 {
			return fmt.Errorf("profile catalog is already semantically equal to revision %d", *sourceRevision)
		}
		written, err := saveProfileCatalogMutation(ctx, runtime, snapshot.Revision, target.Catalog, "rollback", map[string]any{
			"sourceRevision": *sourceRevision,
		})
		if err != nil {
			return err
		}
		report = caseCatalogRollbackReport{
			OK:             true,
			ProfileID:      snapshot.Catalog.ProfileID,
			SourceRevision: *sourceRevision,
			BeforeRevision: snapshot.Revision,
			Revision:       written.Revision,
		}
		return nil
	})
	if err != nil {
		return err
	}
	if *jsonOutput {
		return writeIndentedJSON(report)
	}
	fmt.Printf("Rolled back case catalog %s from r%d to content from r%d as r%d\n", report.ProfileID, report.BeforeRevision, report.SourceRevision, report.Revision)
	return nil
}

func safeCatalogMutationSummary(raw string) map[string]any {
	var decoded map[string]any
	if json.Unmarshal([]byte(raw), &decoded) != nil {
		return nil
	}
	safe := map[string]any{}
	for _, key := range []string{"caseId", "configId", "workflowId", "stepId", "sourceRevision", profileCatalogMutationFieldCreated} {
		if value, ok := decoded[key]; ok {
			safe[key] = value
		}
	}
	if len(safe) == 0 {
		return nil
	}
	return safe
}
