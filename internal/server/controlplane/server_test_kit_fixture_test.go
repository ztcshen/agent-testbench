package controlplane_test

import (
	"context"
	"path/filepath"
	"testing"

	"agent-testbench/internal/store/sqlite"
)

func openTestKitSQLiteStore(t *testing.T, ctx context.Context, name string) *sqlite.Store {
	t.Helper()

	s, err := sqlite.Open(ctx, sqlite.Config{Path: filepath.Join(t.TempDir(), name)})
	if err != nil {
		t.Fatalf("open sqlite store: %v", err)
	}
	t.Cleanup(func() {
		_ = s.Close()
	})
	return s
}
