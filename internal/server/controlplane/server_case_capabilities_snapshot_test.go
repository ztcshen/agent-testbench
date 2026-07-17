package controlplane_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"agent-testbench/internal/domain/profile"
	"agent-testbench/internal/server/controlplane"
	"agent-testbench/internal/store"
	"agent-testbench/internal/store/sqlite"
)

type advancingCatalogSnapshotStore struct {
	store.Store
	versioned store.VersionedProfileCatalogStore
	snapshot  store.ProfileCatalogSnapshot
}

func (s *advancingCatalogSnapshotStore) GetProfileCatalogSnapshot(context.Context, string) (store.ProfileCatalogSnapshot, error) {
	return s.snapshot, nil
}

func (s *advancingCatalogSnapshotStore) CompareAndSwapProfileCatalog(ctx context.Context, revision int64, value store.ProfileCatalog, mutation store.ProfileCatalogMutation) (store.ProfileCatalogSnapshot, error) {
	return s.versioned.CompareAndSwapProfileCatalog(ctx, revision, value, mutation)
}

func (s *advancingCatalogSnapshotStore) ListProfileCatalogVersions(ctx context.Context, profileID string, limit int) ([]store.ProfileCatalogVersion, error) {
	return s.versioned.ListProfileCatalogVersions(ctx, profileID, limit)
}

func (s *advancingCatalogSnapshotStore) GetProfileCatalogVersion(ctx context.Context, profileID string, revision int64) (store.ProfileCatalogVersion, error) {
	return s.versioned.GetProfileCatalogVersion(ctx, profileID, revision)
}

func TestServerCaseCapabilitiesUsesCasesAndRevisionFromOneSnapshot(t *testing.T) {
	ctx := context.Background()
	runtime, err := sqlite.Open(ctx, sqlite.Config{Path: filepath.Join(t.TempDir(), "snapshot.sqlite")})
	if err != nil {
		t.Fatalf("open sqlite store: %v", err)
	}
	defer runtime.Close()
	older := store.ProfileCatalog{
		ProfileID: "sample", IndexedAt: time.Now().UTC(),
		APICases: []store.CatalogAPICase{{ID: "case.revision-1", Status: "draft"}},
	}
	if err := runtime.ReplaceProfileCatalog(ctx, older); err != nil {
		t.Fatalf("replace older catalog: %v", err)
	}
	newer := older
	newer.APICases = []store.CatalogAPICase{{ID: "case.revision-2", Status: "draft"}}
	wrapped := &advancingCatalogSnapshotStore{
		Store: runtime, versioned: runtime,
		snapshot: store.ProfileCatalogSnapshot{Catalog: newer, Revision: 2, SHA256: "revision-2"},
	}
	handler := controlplane.NewWithStore(profile.Bundle{ID: "bootstrap"}, wrapped)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/cases/capabilities", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("capabilities status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	var payload map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode capabilities: %v", err)
	}
	if payload["catalogRevision"] != float64(2) {
		t.Fatalf("catalog revision = %#v", payload)
	}
	cases := payload["cases"].([]any)
	if len(cases) != 1 || cases[0].(map[string]any)["id"] != "case.revision-2" {
		t.Fatalf("capability cases must come from revision 2 snapshot: %#v", payload)
	}
}
