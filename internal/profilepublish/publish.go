// Package profilepublish publishes profile bundles into the active Store.
package profilepublish

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"agent-testbench/internal/domain/profile"
	"agent-testbench/internal/domain/profileaudit"
	"agent-testbench/internal/domain/profilecatalog"
	"agent-testbench/internal/store"
)

type ReadModelPublisher func(context.Context, store.Store, store.ProfileCatalog, string, time.Time) ([]string, error)

type Options struct {
	Path             string
	RequireAuditOK   bool
	UpsertReadModels ReadModelPublisher
	Now              func() time.Time
}

type Result struct {
	Bundle             profile.Bundle
	Digest             string
	SummaryJSON        string
	Counts             profile.Counts
	ImportedAt         time.Time
	Index              store.ProfileIndex
	Catalog            store.ProfileCatalog
	ConfigVersion      store.ConfigVersion
	CatalogIndex       store.ProfileCatalogIndex
	PreviousCatalog    store.ProfileCatalog
	HasPreviousCatalog bool
	ReadModels         []string
}

func Publish(ctx context.Context, runtime store.Store, options Options) (Result, error) {
	bundle, err := profile.Load(options.Path)
	if err != nil {
		return Result{}, fmt.Errorf("load profile %q: %w", options.Path, err)
	}
	if err := requireCleanAudit(ctx, bundle, options); err != nil {
		return Result{}, err
	}
	digest, err := profile.BundleDigest(options.Path)
	if err != nil {
		return Result{}, fmt.Errorf("digest profile %q: %w", options.Path, err)
	}
	counts := bundle.Counts()
	summary, err := json.Marshal(counts)
	if err != nil {
		return Result{}, fmt.Errorf("summarize profile %q: %w", bundle.ID, err)
	}
	importedAt := publishTime(options)
	index, err := runtime.UpsertProfileIndex(ctx, store.ProfileIndex{
		ProfileID:    bundle.ID,
		BundlePath:   options.Path,
		BundleDigest: digest,
		SummaryJSON:  string(summary),
		ImportedAt:   importedAt,
	})
	if err != nil {
		return Result{}, fmt.Errorf("store profile index %q: %w", bundle.ID, err)
	}
	catalog := profilecatalog.FromBundle(bundle, importedAt)
	previousCatalog, hasPreviousCatalog, err := currentCatalog(ctx, runtime)
	if err != nil {
		return Result{}, err
	}
	if err := runtime.ReplaceProfileCatalog(ctx, catalog); err != nil {
		return Result{}, fmt.Errorf("store profile catalog %q: %w", bundle.ID, err)
	}
	configVersion, err := runtime.UpsertConfigVersion(ctx, store.ConfigVersion{
		ID:           ConfigVersionID(bundle.ID, importedAt),
		ProfileID:    bundle.ID,
		SourcePath:   options.Path,
		BundleDigest: digest,
		SummaryJSON:  string(summary),
		Active:       true,
		PublishedAt:  importedAt,
		CreatedAt:    importedAt,
	})
	if err != nil {
		return Result{}, fmt.Errorf("store config version %q: %w", bundle.ID, err)
	}
	readModelKeys, err := publishReadModels(ctx, runtime, catalog, configVersion.ID, importedAt, options)
	if err != nil {
		return Result{}, err
	}
	catalogIndex, err := runtime.GetProfileCatalogIndex(ctx)
	if err != nil {
		return Result{}, fmt.Errorf("read profile catalog index %q: %w", bundle.ID, err)
	}
	return Result{
		Bundle:             bundle,
		Digest:             digest,
		SummaryJSON:        string(summary),
		Counts:             counts,
		ImportedAt:         importedAt,
		Index:              index,
		Catalog:            catalog,
		ConfigVersion:      configVersion,
		CatalogIndex:       catalogIndex,
		PreviousCatalog:    previousCatalog,
		HasPreviousCatalog: hasPreviousCatalog,
		ReadModels:         readModelKeys,
	}, nil
}

func ConfigVersionID(profileID string, publishedAt time.Time) string {
	safeProfileID := strings.NewReplacer("/", "-", "\\", "-", " ", "-", ":", "-").Replace(strings.TrimSpace(profileID))
	if safeProfileID == "" {
		safeProfileID = "profile"
	}
	return "config." + safeProfileID + "." + publishedAt.UTC().Format("20060102T150405.000000000Z")
}

func requireCleanAudit(ctx context.Context, bundle profile.Bundle, options Options) error {
	if !options.RequireAuditOK {
		return nil
	}
	auditReport, err := profileaudit.Audit(ctx, profileaudit.Options{
		Bundle:     bundle,
		BundlePath: options.Path,
	})
	if err != nil {
		return fmt.Errorf("audit profile %q: %w", bundle.ID, err)
	}
	if !auditReport.OK {
		return fmt.Errorf("profile audit failed for profile %q: %s", bundle.ID, profileaudit.FailureSummary(auditReport))
	}
	return nil
}

func currentCatalog(ctx context.Context, runtime store.Store) (store.ProfileCatalog, bool, error) {
	catalog, err := runtime.GetProfileCatalog(ctx)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return store.ProfileCatalog{}, false, nil
		}
		return store.ProfileCatalog{}, false, fmt.Errorf("read current profile catalog: %w", err)
	}
	return catalog, true, nil
}

func publishReadModels(ctx context.Context, runtime store.Store, catalog store.ProfileCatalog, configVersionID string, importedAt time.Time, options Options) ([]string, error) {
	if options.UpsertReadModels == nil {
		return nil, nil
	}
	keys, err := options.UpsertReadModels(ctx, runtime, catalog, configVersionID, importedAt)
	if err != nil {
		return nil, err
	}
	return keys, nil
}

func publishTime(options Options) time.Time {
	if options.Now != nil {
		return options.Now().UTC()
	}
	return time.Now().UTC()
}
