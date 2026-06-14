package sqlite

import (
	"context"
	"path/filepath"
	"testing"
)

func TestOpenDBConstrainsSQLiteToConfiguredConnection(t *testing.T) {
	db, err := openDB(context.Background(), Config{Path: filepath.Join(t.TempDir(), "store.sqlite")})
	if err != nil {
		t.Fatalf("open sqlite db: %v", err)
	}
	defer db.Close()

	if got := db.Stats().MaxOpenConnections; got != 1 {
		t.Fatalf("sqlite db max open connections = %d, want 1 so connection-local pragmas stay enforced", got)
	}
}
