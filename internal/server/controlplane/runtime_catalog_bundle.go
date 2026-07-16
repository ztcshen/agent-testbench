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
