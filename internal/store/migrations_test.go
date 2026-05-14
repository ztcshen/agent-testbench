package store_test

import (
	"context"
	"path/filepath"
	"testing"

	"open-test-sandbox/internal/store/migrations"
	"open-test-sandbox/internal/store/sqlite"
)

func TestSQLiteMigrationsAreIdempotent(t *testing.T) {
	ctx := context.Background()
	cfg := sqlite.Config{Path: filepath.Join(t.TempDir(), "store.sqlite")}

	status, err := sqlite.MigrationStatus(ctx, cfg)
	if err != nil {
		t.Fatalf("initial migration status: %v", err)
	}
	if status.CurrentVersion != 0 || status.TargetVersion != migrations.CurrentVersion || !status.HasPending() {
		t.Fatalf("initial status = %#v", status)
	}

	first, err := sqlite.Migrate(ctx, cfg)
	if err != nil {
		t.Fatalf("first migrate: %v", err)
	}
	if first.CurrentVersion != migrations.CurrentVersion || first.AppliedCount != len(migrations.All()) || first.HasPending() {
		t.Fatalf("first migrated status = %#v", first)
	}

	second, err := sqlite.Migrate(ctx, cfg)
	if err != nil {
		t.Fatalf("second migrate: %v", err)
	}
	if second.CurrentVersion != migrations.CurrentVersion || second.AppliedCount != 0 || second.HasPending() {
		t.Fatalf("second migrated status = %#v", second)
	}
}
