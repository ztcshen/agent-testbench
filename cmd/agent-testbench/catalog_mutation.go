package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"agent-testbench/internal/store"
)

type mutableProfileCatalogSnapshot struct {
	Catalog  store.ProfileCatalog
	Revision int64
	SHA256   string
}

const profileCatalogMutationFieldCreated = "created"

func requireVersionedProfileCatalogStore(runtime store.Store) (store.VersionedProfileCatalogStore, error) {
	versioned, ok := runtime.(store.VersionedProfileCatalogStore)
	if !ok {
		return nil, errors.New("store does not support versioned profile catalog writes; upgrade the Store implementation")
	}
	return versioned, nil
}

func loadMutableProfileCatalogSnapshot(ctx context.Context, runtime store.Store, requestedProfileID string) (mutableProfileCatalogSnapshot, error) {
	requestedProfileID = strings.TrimSpace(requestedProfileID)
	var catalog store.ProfileCatalog
	var err error
	if requestedProfileID == "" {
		catalog, err = runtime.GetProfileCatalog(ctx)
	} else {
		catalog, err = runtime.GetProfileCatalogByID(ctx, requestedProfileID)
	}
	if errors.Is(err, store.ErrNotFound) {
		return mutableProfileCatalogSnapshot{
			Catalog: store.ProfileCatalog{
				ProfileID: firstNonEmpty(requestedProfileID, "default"),
				IndexedAt: time.Now().UTC(),
			},
			Revision: 0,
		}, nil
	}
	if err != nil {
		return mutableProfileCatalogSnapshot{}, err
	}
	if strings.TrimSpace(catalog.ProfileID) == "" {
		catalog.ProfileID = firstNonEmpty(requestedProfileID, "default")
	}
	if requestedProfileID != "" && catalog.ProfileID != requestedProfileID {
		return mutableProfileCatalogSnapshot{}, fmt.Errorf("store profile catalog is %q, not %q", catalog.ProfileID, requestedProfileID)
	}
	versioned, err := requireVersionedProfileCatalogStore(runtime)
	if err != nil {
		return mutableProfileCatalogSnapshot{}, err
	}
	current, err := versioned.GetProfileCatalogSnapshot(ctx, catalog.ProfileID)
	if err != nil {
		return mutableProfileCatalogSnapshot{}, err
	}
	return mutableProfileCatalogSnapshot{Catalog: current.Catalog, Revision: current.Revision, SHA256: current.SHA256}, nil
}

func saveProfileCatalogMutation(
	ctx context.Context,
	runtime store.Store,
	expectedRevision int64,
	catalog store.ProfileCatalog,
	operation string,
	summary map[string]any,
) (store.ProfileCatalogSnapshot, error) {
	versioned, err := requireVersionedProfileCatalogStore(runtime)
	if err != nil {
		return store.ProfileCatalogSnapshot{}, err
	}
	summaryJSON, err := json.Marshal(summary)
	if err != nil {
		return store.ProfileCatalogSnapshot{}, fmt.Errorf("encode profile catalog mutation summary: %w", err)
	}
	catalog.IndexedAt = time.Now().UTC()
	return versioned.CompareAndSwapProfileCatalog(ctx, expectedRevision, catalog, store.ProfileCatalogMutation{
		Operation:   operation,
		SummaryJSON: string(summaryJSON),
	})
}

func requireExpectedProfileCatalogRevision(profileID string, actual int64, expected *int64) error {
	if expected == nil || *expected == actual {
		return nil
	}
	return &store.ProfileCatalogRevisionConflictError{
		ProfileID:        profileID,
		ExpectedRevision: *expected,
		ActualRevision:   actual,
	}
}

func optionalExpectedProfileCatalogRevision(passed bool, value int64) *int64 {
	if !passed {
		return nil
	}
	return &value
}
