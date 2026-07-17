package storecontract

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"agent-testbench/internal/store"
	"agent-testbench/internal/store/sqlite"
)

func TestSQLiteProfileCatalogVersionContractRejectsLostUpdatesAndKeepsHistory(t *testing.T) {
	ctx := context.Background()
	s, err := sqlite.Open(ctx, sqlite.Config{Path: filepath.Join(t.TempDir(), "store.sqlite")})
	if err != nil {
		t.Fatalf("open sqlite store: %v", err)
	}
	defer s.Close()

	initial := contractProfileCatalog(time.Date(2026, 7, 16, 9, 0, 0, 0, time.UTC))
	if err := s.ReplaceProfileCatalog(ctx, initial); err != nil {
		t.Fatalf("replace initial profile catalog: %v", err)
	}

	versioned := store.VersionedProfileCatalogStore(s)
	firstRead, err := versioned.GetProfileCatalogSnapshot(ctx, initial.ProfileID)
	if err != nil {
		t.Fatalf("get first profile catalog snapshot: %v", err)
	}
	secondRead := firstRead
	if firstRead.Revision != 1 || firstRead.SHA256 == "" {
		t.Fatalf("initial snapshot = %#v", firstRead)
	}

	firstRead.Catalog.APICases[0].Description = "saved by first editor"
	firstWrite, err := versioned.CompareAndSwapProfileCatalog(ctx, firstRead.Revision, firstRead.Catalog, store.ProfileCatalogMutation{
		Operation:   "case-upsert",
		SummaryJSON: `{"caseId":"case.health"}`,
	})
	if err != nil {
		t.Fatalf("save first editor catalog: %v", err)
	}
	if firstWrite.Revision != 2 {
		t.Fatalf("first write revision = %d, want 2", firstWrite.Revision)
	}

	secondRead.Catalog.APICases[0].Description = "stale overwrite"
	_, err = versioned.CompareAndSwapProfileCatalog(ctx, secondRead.Revision, secondRead.Catalog, store.ProfileCatalogMutation{Operation: "case-upsert"})
	if !errors.Is(err, store.ErrProfileCatalogRevisionConflict) {
		t.Fatalf("stale write error = %v, want revision conflict", err)
	}
	var conflict *store.ProfileCatalogRevisionConflictError
	if !errors.As(err, &conflict) || conflict.ExpectedRevision != 1 || conflict.ActualRevision != 2 {
		t.Fatalf("stale write conflict = %#v", conflict)
	}

	current, err := versioned.GetProfileCatalogSnapshot(ctx, initial.ProfileID)
	if err != nil {
		t.Fatalf("get current profile catalog snapshot: %v", err)
	}
	if got := current.Catalog.APICases[0].Description; got != "saved by first editor" {
		t.Fatalf("current case description = %q", got)
	}

	versions, err := versioned.ListProfileCatalogVersions(ctx, initial.ProfileID, 10)
	if err != nil {
		t.Fatalf("list profile catalog versions: %v", err)
	}
	if len(versions) != 2 || versions[0].Revision != 2 || versions[1].Revision != 1 {
		t.Fatalf("profile catalog versions = %#v", versions)
	}

	versionOne, err := versioned.GetProfileCatalogVersion(ctx, initial.ProfileID, 1)
	if err != nil {
		t.Fatalf("get profile catalog version one: %v", err)
	}
	rolledBack, err := versioned.CompareAndSwapProfileCatalog(ctx, current.Revision, versionOne.Catalog, store.ProfileCatalogMutation{
		Operation:   "rollback",
		SummaryJSON: `{"sourceRevision":1}`,
	})
	if err != nil {
		t.Fatalf("roll back profile catalog: %v", err)
	}
	if rolledBack.Revision != 3 || rolledBack.Catalog.APICases[0].Description != initial.APICases[0].Description {
		t.Fatalf("rolled back profile catalog = %#v", rolledBack)
	}

	noOp, err := versioned.CompareAndSwapProfileCatalog(ctx, rolledBack.Revision, rolledBack.Catalog, store.ProfileCatalogMutation{Operation: "case-upsert"})
	if err != nil {
		t.Fatalf("save unchanged profile catalog: %v", err)
	}
	if noOp.Revision != rolledBack.Revision {
		t.Fatalf("unchanged write revision = %d, want %d", noOp.Revision, rolledBack.Revision)
	}
	versions, err = versioned.ListProfileCatalogVersions(ctx, initial.ProfileID, 10)
	if err != nil {
		t.Fatalf("list profile catalog versions after no-op: %v", err)
	}
	if len(versions) != 3 {
		t.Fatalf("versions after no-op = %d, want 3", len(versions))
	}
}
