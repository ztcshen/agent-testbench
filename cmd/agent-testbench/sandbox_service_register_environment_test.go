package main

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"agent-testbench/internal/store"
	"agent-testbench/internal/store/sqlite"
)

func TestSandboxServiceRegisterRepairsStartupCommandFromEnvironmentComponent(t *testing.T) {
	storePath := filepath.Join(t.TempDir(), "store.sqlite")
	startedPath := filepath.Join(t.TempDir(), "started-from-env.txt")
	ctx := context.Background()
	s, err := sqlite.Open(ctx, sqlite.Config{Path: storePath})
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	now := time.Now().UTC()
	if _, err := s.UpsertEnvironment(ctx, store.Environment{ID: "env.startup-repair", DisplayName: "Startup Repair", CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatalf("upsert environment: %v", err)
	}
	if err := s.ReplaceEnvironmentComponentGraph(ctx, "env.startup-repair", store.EnvironmentComponentGraph{
		Components: []store.EnvironmentComponent{
			{
				ComponentID: "service.refresh",
				DisplayName: "Refresh Service",
				Kind:        "app",
				Required:    true,
				RuntimeJSON: `{"startupCommand":"printf repaired-from-env > ` + startedPath + `"}`,
			},
		},
	}); err != nil {
		t.Fatalf("replace component graph: %v", err)
	}
	if err := s.Close(); err != nil {
		t.Fatalf("close store: %v", err)
	}

	runCLI(t, "sandbox", "service", "register", "--store", "sqlite://"+storePath, "--id", "service.refresh")
	before := runCLIFails(t, "sandbox", "start", "--store", "sqlite://"+storePath, "--service", "service.refresh", "--dry-run", "--json")
	if !strings.Contains(before, sandboxStartupCommandEmpty) {
		t.Fatalf("service without startup command should fail before environment repair: %s", before)
	}
	runCLI(t, "sandbox", "service", "register", "--store", "sqlite://"+storePath, "--id", "service.refresh", "--from-environment", "env.startup-repair", "--json")
	after := runCLI(t, "sandbox", "start", "--store", "sqlite://"+storePath, "--service", "service.refresh", "--dry-run", "--json")
	if !strings.Contains(after, `"planned": true`) || !strings.Contains(after, startedPath) || !strings.Contains(after, `"kind": "app"`) {
		t.Fatalf("environment startup metadata repair should make dry-run planned: %s", after)
	}
}
