package store_test

import (
	"context"
	"path/filepath"
	"testing"

	"open-test-sandbox/internal/store/schema"
	"open-test-sandbox/internal/store/sqlite"
)

func TestSQLiteSchemaUpgradesAreIdempotent(t *testing.T) {
	ctx := context.Background()
	cfg := sqlite.Config{Path: filepath.Join(t.TempDir(), "store.sqlite")}

	status, err := sqlite.SchemaStatus(ctx, cfg)
	if err != nil {
		t.Fatalf("initial schema upgrade status: %v", err)
	}
	if status.CurrentVersion != 0 || status.TargetVersion != schema.CurrentVersion || !status.HasPending() {
		t.Fatalf("initial status = %#v", status)
	}

	first, err := sqlite.UpgradeSchema(ctx, cfg)
	if err != nil {
		t.Fatalf("first upgrade: %v", err)
	}
	if first.CurrentVersion != schema.CurrentVersion || first.AppliedCount != len(schema.All()) || first.HasPending() {
		t.Fatalf("first upgraded status = %#v", first)
	}

	second, err := sqlite.UpgradeSchema(ctx, cfg)
	if err != nil {
		t.Fatalf("second upgrade: %v", err)
	}
	if second.CurrentVersion != schema.CurrentVersion || second.AppliedCount != 0 || second.HasPending() {
		t.Fatalf("second upgraded status = %#v", second)
	}
}
