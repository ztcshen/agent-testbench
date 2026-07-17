package controlplane

import (
	"context"
	"errors"

	"agent-testbench/internal/domain/profile"
	"agent-testbench/internal/domain/profilecatalog"
	"agent-testbench/internal/store"
)

func currentProfileBundle(ctx context.Context, runtime store.Store, bootstrap profile.Bundle) (profile.Bundle, error) {
	if runtime == nil {
		return bootstrap, nil
	}
	catalog, err := runtime.GetProfileCatalog(ctx)
	if errors.Is(err, store.ErrNotFound) {
		return bootstrap, nil
	}
	if err != nil {
		return profile.Bundle{}, err
	}
	return profilecatalog.RuntimeBundle(catalog, bootstrap), nil
}

func currentProfileBundleSnapshot(ctx context.Context, runtime store.Store, bootstrap profile.Bundle) (profile.Bundle, int64, error) {
	if runtime == nil {
		return bootstrap, 0, nil
	}
	catalog, err := runtime.GetProfileCatalog(ctx)
	if errors.Is(err, store.ErrNotFound) {
		return bootstrap, 0, nil
	}
	if err != nil {
		return profile.Bundle{}, 0, err
	}
	versioned, ok := runtime.(store.VersionedProfileCatalogStore)
	if !ok || catalog.ProfileID == "" {
		return profilecatalog.RuntimeBundle(catalog, bootstrap), 0, nil
	}
	snapshot, err := versioned.GetProfileCatalogSnapshot(ctx, catalog.ProfileID)
	if errors.Is(err, store.ErrNotFound) {
		return profilecatalog.RuntimeBundle(catalog, bootstrap), 0, nil
	}
	if err != nil {
		return profile.Bundle{}, 0, err
	}
	return profilecatalog.RuntimeBundle(snapshot.Catalog, bootstrap), snapshot.Revision, nil
}
