package main

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestStoreMigrateAndStatusCommands(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "store.sqlite")

	initial := runCLI(t, "store", "status", "--store-url", dbPath)
	if !strings.Contains(initial, "Version: 0") || !strings.Contains(initial, "Pending: 1") {
		t.Fatalf("initial status output = %q", initial)
	}

	migrated := runCLI(t, "store", "migrate", "--store-url", dbPath)
	if !strings.Contains(migrated, "Migrated store to version 1") {
		t.Fatalf("migrate output = %q", migrated)
	}

	current := runCLI(t, "store", "status", "--store-url", dbPath)
	if !strings.Contains(current, "Version: 1") || !strings.Contains(current, "Pending: 0") {
		t.Fatalf("current status output = %q", current)
	}
}

func runCLI(t *testing.T, args ...string) string {
	t.Helper()
	cmd := exec.Command("go", append([]string{"run", "."}, args...)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go run . %s failed: %v\n%s", strings.Join(args, " "), err, out)
	}
	return string(out)
}
