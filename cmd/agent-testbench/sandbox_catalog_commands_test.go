package main

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"agent-testbench/internal/store"
)

func TestSandboxServiceListReportsRegisteredServicesReadOnly(t *testing.T) {
	fixture := writeSandboxStartStoreFixture(t)

	out := runCLI(t, "sandbox", "service", "list", "--store", "sqlite://"+fixture.storePath, "--json")
	var report struct {
		OK       bool `json:"ok"`
		Count    int  `json:"count"`
		Services []struct {
			ID             string `json:"id"`
			Kind           string `json:"kind"`
			StartupCommand string `json:"startupCommand"`
		} `json:"services"`
	}
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatalf("decode sandbox service list json: %v\n%s", err, out)
	}
	if !report.OK || report.Count != 3 || len(report.Services) != 3 {
		t.Fatalf("sandbox service list report = %#v", report)
	}
	if report.Services[0].ID != "entry-service" || report.Services[0].Kind != "app" || report.Services[0].StartupCommand == "" {
		t.Fatalf("sandbox service list first service = %#v", report.Services[0])
	}
	requireSandboxNoStartupSideEffects(t, fixture)
}

func TestSandboxServiceListCanIncludeEnvironmentComponentGraph(t *testing.T) {
	fixture := writeSandboxStartStoreFixture(t)
	ctx := context.Background()
	s, err := openStore(ctx, "sqlite://"+fixture.storePath)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer s.Close()
	now := time.Now().UTC()
	if _, err := s.UpsertEnvironment(ctx, store.Environment{ID: "env.sandbox.components", DisplayName: "Sandbox Components", CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatalf("upsert environment: %v", err)
	}
	if err := s.ReplaceEnvironmentComponentGraph(ctx, "env.sandbox.components", store.EnvironmentComponentGraph{
		Components: []store.EnvironmentComponent{
			{ComponentID: "entry-service", DisplayName: "Entry Component", Kind: "app", Role: "consumer", ComposeService: "entry", Required: true},
			{ComponentID: "mysql", DisplayName: "MySQL", Kind: "database", Role: "provider", ComposeService: "mysql", Required: true},
		},
	}); err != nil {
		t.Fatalf("replace component graph: %v", err)
	}

	out := runCLI(t, "sandbox", "service", "list", "--store", "sqlite://"+fixture.storePath, "--environment", "env.sandbox.components", "--include-components", "--json")
	var report struct {
		OK       bool `json:"ok"`
		Count    int  `json:"count"`
		Services []struct {
			ID                string   `json:"id"`
			Sources           []string `json:"sources"`
			InProfileRegistry bool     `json:"inProfileRegistry"`
			InComponentGraph  bool     `json:"inComponentGraph"`
			ComposeService    string   `json:"composeService"`
			Required          bool     `json:"required"`
		} `json:"services"`
	}
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatalf("decode sandbox service/component list json: %v\n%s", err, out)
	}
	if !report.OK || report.Count != 4 {
		t.Fatalf("sandbox service/component list report = %#v", report)
	}
	byID := map[string]struct {
		sources           []string
		inProfileRegistry bool
		inComponentGraph  bool
		composeService    string
		required          bool
	}{}
	for _, item := range report.Services {
		byID[item.ID] = struct {
			sources           []string
			inProfileRegistry bool
			inComponentGraph  bool
			composeService    string
			required          bool
		}{item.Sources, item.InProfileRegistry, item.InComponentGraph, item.ComposeService, item.Required}
	}
	entry := byID["entry-service"]
	if !entry.inProfileRegistry || !entry.inComponentGraph || entry.composeService != "entry" {
		t.Fatalf("merged entry-service item = %#v", entry)
	}
	mysql := byID["mysql"]
	if mysql.inProfileRegistry || !mysql.inComponentGraph || mysql.composeService != "mysql" || !mysql.required || strings.Join(mysql.sources, ",") != "environment-component-graph" {
		t.Fatalf("component-only mysql item = %#v", mysql)
	}

	filtered := runCLI(t, "sandbox", "service", "list", "--store", "sqlite://"+fixture.storePath, "--environment", "env.sandbox.components", "--include-components", "--service", "mysql", "--json")
	var filteredReport struct {
		Count    int `json:"count"`
		Services []struct {
			ID string `json:"id"`
		} `json:"services"`
	}
	if err := json.Unmarshal([]byte(filtered), &filteredReport); err != nil {
		t.Fatalf("decode filtered sandbox service/component list json: %v\n%s", err, filtered)
	}
	if filteredReport.Count != 1 || filteredReport.Services[0].ID != "mysql" {
		t.Fatalf("filtered component-only service = %#v", filteredReport)
	}
}

func TestSandboxServiceListCanReadComponentOnlyEnvironment(t *testing.T) {
	storePath := filepath.Join(t.TempDir(), "component-only.sqlite")
	ctx := context.Background()
	s, err := openStore(ctx, "sqlite://"+storePath)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer s.Close()
	now := time.Now().UTC()
	if _, err := s.UpsertEnvironment(ctx, store.Environment{ID: "env.component.only", DisplayName: "Component Only", CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatalf("upsert environment: %v", err)
	}
	if err := s.ReplaceEnvironmentComponentGraph(ctx, "env.component.only", store.EnvironmentComponentGraph{
		Components: []store.EnvironmentComponent{
			{ComponentID: "mysql", DisplayName: "MySQL", Kind: "database", Role: "provider", ComposeService: "mysql", Required: true},
		},
	}); err != nil {
		t.Fatalf("replace component graph: %v", err)
	}

	out := runCLI(t, "sandbox", "service", "list", "--store", "sqlite://"+storePath, "--environment", "env.component.only", "--include-components", "--json")
	var report struct {
		OK       bool `json:"ok"`
		Count    int  `json:"count"`
		Services []struct {
			ID                string `json:"id"`
			InProfileRegistry bool   `json:"inProfileRegistry"`
			InComponentGraph  bool   `json:"inComponentGraph"`
		} `json:"services"`
	}
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatalf("decode component-only sandbox service list json: %v\n%s", err, out)
	}
	if !report.OK || report.Count != 1 || report.Services[0].ID != "mysql" || report.Services[0].InProfileRegistry || !report.Services[0].InComponentGraph {
		t.Fatalf("component-only sandbox service list = %#v", report)
	}
}
