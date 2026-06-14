package main

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"agent-testbench/internal/store"
	"agent-testbench/internal/store/sqlite"
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

func TestSandboxRegisterCommandsWriteStoreCatalog(t *testing.T) {
	storePath := filepath.Join(t.TempDir(), "store.sqlite")

	serviceOut := runCLI(t, "sandbox", "service", "register",
		"--store", "sqlite://"+storePath,
		"--id", "service.gateway",
		"--display-name", "Gateway",
		"--kind", "http",
		"--service-port", "18080",
		"--health-url", newHealthyTestURL(t),
	)
	if !strings.Contains(serviceOut, "Registered service: service.gateway") {
		t.Fatalf("service register output = %q", serviceOut)
	}

	interfaceOut := runCLI(t, "sandbox", "interface", "register",
		"--store", "sqlite://"+storePath,
		"--id", "node.create-order",
		"--service-id", "service.gateway",
		"--method", "POST",
		"--path", "/orders",
		"--case-id", "case.create-order",
		"--case-title", "Create order",
		"--required-for-admission",
	)
	if !strings.Contains(interfaceOut, "Registered interface: node.create-order") || !strings.Contains(interfaceOut, "Case: case.create-order") {
		t.Fatalf("interface register output = %q", interfaceOut)
	}

	s, err := sqlite.Open(context.Background(), sqlite.Config{Path: storePath})
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer s.Close()
	catalog, err := s.GetProfileCatalog(context.Background())
	if err != nil {
		t.Fatalf("get catalog: %v", err)
	}
	if catalog.ProfileID != "current" || len(catalog.Services) != 1 || catalog.Services[0].ID != "service.gateway" {
		t.Fatalf("catalog services = %#v", catalog)
	}
	if len(catalog.InterfaceNodes) != 1 || catalog.InterfaceNodes[0].ID != "node.create-order" || catalog.InterfaceNodes[0].ServiceID != "service.gateway" {
		t.Fatalf("catalog interface nodes = %#v", catalog.InterfaceNodes)
	}
	if len(catalog.RequestTemplates) != 1 || catalog.RequestTemplates[0].NodeID != "node.create-order" {
		t.Fatalf("catalog request templates = %#v", catalog.RequestTemplates)
	}
	if len(catalog.APICases) != 1 || catalog.APICases[0].ID != "case.create-order" || !catalog.APICases[0].RequiredForAdmission {
		t.Fatalf("catalog api cases = %#v", catalog.APICases)
	}
}

func TestSandboxServiceRegisterCanRepairStartupCommand(t *testing.T) {
	storePath := filepath.Join(t.TempDir(), "store.sqlite")
	startedPath := filepath.Join(t.TempDir(), "started.txt")
	healthURL := newHealthyTestURL(t)
	runCLI(t, "sandbox", "service", "register",
		"--store", "sqlite://"+storePath,
		"--id", "service.refresh",
		"--display-name", "Refresh Service",
		"--kind", "app",
		"--service-port", "18081",
		"--management-port", "19091",
		"--health-url", healthURL,
	)
	before := runCLIFails(t, "sandbox", "start",
		"--store", "sqlite://"+storePath,
		"--service", "service.refresh",
		"--dry-run",
		"--json",
	)
	if !strings.Contains(before, `"ok": false`) || !strings.Contains(before, `"failed": 1`) || !strings.Contains(before, sandboxStartupCommandEmpty) {
		t.Fatalf("service without startup command should fail before repair: %s", before)
	}

	runCLI(t, "sandbox", "service", "register",
		"--store", "sqlite://"+storePath,
		"--id", "service.refresh",
		"--startup-command", "printf refreshed > "+startedPath,
		"--json",
	)
	after := runCLI(t, "sandbox", "start",
		"--store", "sqlite://"+storePath,
		"--service", "service.refresh",
		"--dry-run",
		"--json",
	)
	if !strings.Contains(after, `"planned": true`) || !strings.Contains(after, startedPath) {
		t.Fatalf("service startup command repair should make dry-run planned: %s", after)
	}
	s, err := sqlite.Open(context.Background(), sqlite.Config{Path: storePath})
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer s.Close()
	catalog, err := s.GetProfileCatalog(context.Background())
	if err != nil {
		t.Fatalf("get catalog: %v", err)
	}
	if len(catalog.Services) != 1 {
		t.Fatalf("catalog services = %#v", catalog.Services)
	}
	service := catalog.Services[0]
	if service.DisplayName != "Refresh Service" || service.Kind != "app" || service.ServicePort != 18081 || service.ManagementPort != 19091 || service.HealthURL != healthURL || service.Status != "active" {
		t.Fatalf("startup command repair should preserve service metadata: %#v", service)
	}
}

func TestSandboxRegisterCommandsUseNamedPostgreSQLActiveStore(t *testing.T) {
	storeRef := configureNamedPostgreSQLActiveStore(t, "daily-sandbox-register-pg")
	runSandboxRegisterCommandsUseNamedActiveStore(t, storeRef, "pg", "PostgreSQL")
}

func TestSandboxRegisterCommandsUseNamedMySQLActiveStore(t *testing.T) {
	storeRef := configureNamedMySQLActiveStore(t, "daily-sandbox-register-mysql")
	runSandboxRegisterCommandsUseNamedActiveStore(t, storeRef, "mysql", "MySQL")
}

func runSandboxRegisterCommandsUseNamedActiveStore(t *testing.T, storeRef string, suffixLabel string, label string) {
	t.Helper()
	suffix := time.Now().UTC().Format("20060102150405.000000000")
	serviceID := "service.gateway." + suffixLabel + "." + suffix
	nodeID := "node.create-order." + suffixLabel + "." + suffix
	caseID := "case.create-order." + suffixLabel + "." + suffix

	registerNamedSandboxService(t, label, serviceID)
	registerNamedSandboxInterface(t, label, serviceID, nodeID, caseID)
	requireNamedSandboxCatalog(t, storeRef, label, serviceID, nodeID, caseID)
}

func registerNamedSandboxService(t *testing.T, label string, serviceID string) {
	t.Helper()

	out := runCLI(t, "sandbox", "service", "register",
		"--id", serviceID,
		"--display-name", "Gateway "+label,
		"--kind", "http",
		"--service-port", "18080",
		"--health-url", newHealthyTestURL(t),
	)
	if !strings.Contains(out, "Registered service: "+serviceID) {
		t.Fatalf("%s service register output = %q", label, out)
	}
}

func registerNamedSandboxInterface(t *testing.T, label string, serviceID string, nodeID string, caseID string) {
	t.Helper()

	out := runCLI(t, "sandbox", "interface", "register",
		"--id", nodeID,
		"--service-id", serviceID,
		"--method", "POST",
		"--path", "/orders",
		"--case-id", caseID,
		"--case-title", "Create order",
		"--required-for-admission",
	)
	if !strings.Contains(out, "Registered interface: "+nodeID) || !strings.Contains(out, "Case: "+caseID) {
		t.Fatalf("%s interface register output = %q", label, out)
	}
}

func requireNamedSandboxCatalog(t *testing.T, storeRef string, label string, serviceID string, nodeID string, caseID string) {
	t.Helper()

	s, err := openStore(context.Background(), storeRef)
	if err != nil {
		t.Fatalf("open SQL Store: %v", err)
	}
	defer s.Close()
	catalog, err := s.GetProfileCatalog(context.Background())
	if err != nil {
		t.Fatalf("get %s catalog: %v", label, err)
	}
	serviceFound := false
	for _, service := range catalog.Services {
		if service.ID == serviceID {
			serviceFound = true
			break
		}
	}
	if !serviceFound {
		t.Fatalf("%s catalog services = %#v", label, catalog.Services)
	}
	nodeFound := false
	for _, node := range catalog.InterfaceNodes {
		if node.ID == nodeID && node.ServiceID == serviceID {
			nodeFound = true
			break
		}
	}
	if !nodeFound {
		t.Fatalf("%s catalog interface nodes = %#v", label, catalog.InterfaceNodes)
	}
	templateFound := false
	for _, template := range catalog.RequestTemplates {
		if template.NodeID == nodeID {
			templateFound = true
			break
		}
	}
	if !templateFound {
		t.Fatalf("%s catalog request templates = %#v", label, catalog.RequestTemplates)
	}
	caseFound := false
	for _, apiCase := range catalog.APICases {
		if apiCase.ID == caseID && apiCase.RequiredForAdmission {
			caseFound = true
			break
		}
	}
	if !caseFound {
		t.Fatalf("%s catalog api cases = %#v", label, catalog.APICases)
	}
}
