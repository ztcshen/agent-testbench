package controlplane_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"agent-testbench/internal/store/sqlite"
)

func openAPICaseBatchSQLiteStore(t *testing.T) (context.Context, *sqlite.Store) {
	t.Helper()
	ctx := context.Background()
	s, err := sqlite.Open(ctx, sqlite.Config{Path: filepath.Join(t.TempDir(), "sandbox.sqlite")})
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() {
		if err := s.Close(); err != nil {
			t.Fatalf("close store: %v", err)
		}
	})
	return ctx, s
}

func writeAPICaseBatchGETCase(t *testing.T, dir string, id string, path string) string {
	t.Helper()
	out := filepath.Join(dir, id+".json")
	if err := os.WriteFile(out, []byte(fmt.Sprintf(`{
  "id": %q,
  "title": %q,
  "request": {"method": "GET", "path": %q},
  "assertions": {"expectedStatusCodes": [200], "responseContains": ["ok"]}
}`, id, id, path)), 0o644); err != nil {
		t.Fatalf("write api case %s: %v", id, err)
	}
	return out
}
