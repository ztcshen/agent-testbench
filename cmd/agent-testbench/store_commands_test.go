package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"agent-testbench/internal/domain/profile"
	"agent-testbench/internal/store"
	"agent-testbench/internal/store/mysql"
	"agent-testbench/internal/store/postgres"
	"agent-testbench/internal/store/schema"
	"agent-testbench/internal/store/sqlite"
	"agent-testbench/internal/store/sqlstore"
)

func TestStoreUpgradeAndStatusCommands(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "store.sqlite")

	initial := runCLI(t, "store", "status", "--store", "sqlite://"+dbPath)
	if !strings.Contains(initial, "Version: 0") || !strings.Contains(initial, fmt.Sprintf("Pending: %d", schema.CurrentVersion)) {
		t.Fatalf("initial status output = %q", initial)
	}

	upgraded := runCLI(t, "store", "upgrade", "--store", "sqlite://"+dbPath)
	if !strings.Contains(upgraded, fmt.Sprintf("Upgraded store schema to version %d", schema.CurrentVersion)) {
		t.Fatalf("upgrade output = %q", upgraded)
	}

	current := runCLI(t, "store", "status", "--store", "sqlite://"+dbPath)
	if !strings.Contains(current, fmt.Sprintf("Version: %d", schema.CurrentVersion)) || !strings.Contains(current, "Pending: 0") {
		t.Fatalf("current status output = %q", current)
	}
}

func TestCopyStoreCurrentStateCopiesCatalogAndEnvironmentGraph(t *testing.T) {
	ctx := context.Background()
	source, err := sqlite.Open(ctx, sqlite.Config{Path: filepath.Join(t.TempDir(), "source.sqlite")})
	if err != nil {
		t.Fatalf("open source: %v", err)
	}
	defer source.Close()
	target, err := sqlite.Open(ctx, sqlite.Config{Path: filepath.Join(t.TempDir(), "target.sqlite")})
	if err != nil {
		t.Fatalf("open target: %v", err)
	}
	defer target.Close()
	now := time.Now().UTC()
	if _, err := source.UpsertProfileIndex(ctx, store.ProfileIndex{
		ProfileID:   "profile.alpha",
		BundlePath:  "store://profile.alpha",
		SummaryJSON: `{"source":"test"}`,
		ImportedAt:  now,
		UpdatedAt:   now,
	}); err != nil {
		t.Fatalf("seed profile index: %v", err)
	}
	if err := source.ReplaceProfileCatalog(ctx, store.ProfileCatalog{
		ProfileID: "profile.alpha",
		IndexedAt: now,
		Services:  []store.CatalogService{{ID: "service.alpha", DisplayName: "Service Alpha"}},
		Workflows: []store.CatalogWorkflow{{ID: "workflow.alpha", DisplayName: "Workflow Alpha"}},
	}); err != nil {
		t.Fatalf("seed profile catalog: %v", err)
	}
	if _, err := source.UpsertConfigVersion(ctx, store.ConfigVersion{
		ID:           "config.profile.alpha.001",
		ProfileID:    "profile.alpha",
		SourcePath:   "store://profile.alpha",
		BundleDigest: "sha256:test",
		SummaryJSON:  `{"source":"test"}`,
		Active:       true,
		PublishedAt:  now,
		CreatedAt:    now,
	}); err != nil {
		t.Fatalf("seed config version: %v", err)
	}
	if _, err := source.UpsertEnvironment(ctx, store.Environment{
		ID:                     "env.alpha",
		DisplayName:            "Environment Alpha",
		Status:                 "verified",
		Verified:               true,
		ServicesJSON:           `[{"id":"service.alpha"}]`,
		ReposJSON:              `{"service.alpha":{"url":"https://example.invalid/service-alpha.git"}}`,
		ComposeJSON:            `{"composeFiles":["compose.yml"]}`,
		HealthChecksJSON:       `[{"id":"alpha","url":"http://127.0.0.1:18080/health"}]`,
		VerificationWorkflowID: "workflow.alpha",
		LastVerificationStatus: "passed",
		EvidenceComplete:       true,
		TopologyComplete:       true,
		SummaryJSON:            `{"restoreReady":true}`,
		CreatedAt:              now,
		UpdatedAt:              now,
	}); err != nil {
		t.Fatalf("seed environment: %v", err)
	}
	if err := source.ReplaceEnvironmentComponentGraph(ctx, "env.alpha", store.EnvironmentComponentGraph{
		Components: []store.EnvironmentComponent{{
			ComponentID:    "service.alpha",
			DisplayName:    "Service Alpha",
			Kind:           "service",
			ComposeService: "service-alpha",
			Required:       true,
			RuntimeJSON:    `{}`,
			SummaryJSON:    `{}`,
		}},
		Assets: []store.ComponentConfigAsset{{
			OwnerComponentID: "service.alpha",
			AssetID:          "compose.alpha",
			AssetKind:        "compose-file",
			TargetPath:       "compose.yml",
			ContentInline:    "services: {}\n",
			SummaryJSON:      `{}`,
		}},
	}); err != nil {
		t.Fatalf("seed component graph: %v", err)
	}

	report, err := copyStoreCurrentState(ctx, source, target)
	if err != nil {
		t.Fatalf("copy store state: %v", err)
	}
	if report.ProfileCatalogs != 1 || report.ProfileIndexes != 1 || report.ConfigVersions != 1 || len(report.ReadModels) == 0 || !report.RunsSkipped || report.Environments != 1 || report.ComponentGraphs != 1 {
		t.Fatalf("copy report = %#v", report)
	}
	if len(report.EnvironmentIDs) != 1 || report.EnvironmentIDs[0] != "env.alpha" || len(report.EnvironmentRefs) != 1 || !report.EnvironmentRefs[0].Verified || report.EnvironmentRefs[0].VerificationWorkflowID != "workflow.alpha" {
		t.Fatalf("copy environment refs = %#v ids=%#v", report.EnvironmentRefs, report.EnvironmentIDs)
	}
	if len(report.ComponentRefs) != 1 || report.ComponentRefs[0].EnvironmentID != "env.alpha" || report.ComponentRefs[0].Components != 1 || report.ComponentRefs[0].Assets != 1 || report.ComponentRefs[0].InlineAssetBytes != len("services: {}\n") || report.ComponentRefs[0].LargestInlineAssetID != "compose.alpha" {
		t.Fatalf("copy component refs = %#v", report.ComponentRefs)
	}
	if err := validateStoreCopyRequirements(report, storeCopyRequirements{EnvironmentID: "env.alpha", VerificationWorkflowID: "workflow.alpha", VerifiedEnvironment: true, MinComponents: 1, MinAssets: 1, MinInlineAssetBytes: len("services: {}\n")}); err != nil {
		t.Fatalf("expected env.alpha copy requirements to pass: %v", err)
	}
	if err := validateStoreCopyRequirements(report, storeCopyRequirements{EnvironmentID: "env.missing"}); err == nil || !strings.Contains(err.Error(), "was not copied") {
		t.Fatalf("expected missing environment requirement failure, got %v", err)
	}
	if err := validateStoreCopyRequirements(report, storeCopyRequirements{EnvironmentID: "env.alpha", VerificationWorkflowID: "workflow.other"}); err == nil || !strings.Contains(err.Error(), "verification workflow") {
		t.Fatalf("expected workflow requirement failure, got %v", err)
	}
	if err := validateStoreCopyRequirements(report, storeCopyRequirements{EnvironmentID: "env.alpha", MinComponents: 2}); err == nil || !strings.Contains(err.Error(), "component count") {
		t.Fatalf("expected min component requirement failure, got %v", err)
	}
	graphlessReport := storeCopyStateReport{
		OK: true,
		EnvironmentRefs: []storeCopyEnvironmentReport{{
			ID:     "env.graphless",
			Status: "draft",
		}},
	}
	if err := validateStoreCopyRequirements(graphlessReport, storeCopyRequirements{EnvironmentID: "env.graphless"}); err != nil {
		t.Fatalf("presence-only environment requirement should not require a component graph: %v", err)
	}
	if err := validateStoreCopyRequirements(graphlessReport, storeCopyRequirements{EnvironmentID: "env.graphless", MinComponents: 1}); err == nil || !strings.Contains(err.Error(), "component graph") {
		t.Fatalf("component minimum should require a component graph, got %v", err)
	}
	catalog, err := target.GetProfileCatalog(ctx)
	if err != nil {
		t.Fatalf("target profile catalog: %v", err)
	}
	if catalog.ProfileID != "profile.alpha" || len(catalog.Services) != 1 || len(catalog.Workflows) != 1 {
		t.Fatalf("target catalog = %#v", catalog)
	}
	configVersion, err := target.GetActiveConfigVersion(ctx)
	if err != nil {
		t.Fatalf("target active config version: %v", err)
	}
	if configVersion.ID != "config.profile.alpha.001" || !configVersion.Active {
		t.Fatalf("target active config version = %#v", configVersion)
	}
	if _, err := target.GetReadModel(ctx, "profile.alpha", "catalog"); err != nil {
		t.Fatalf("target catalog read model: %v", err)
	}
	env, err := target.GetEnvironment(ctx, "env.alpha")
	if err != nil {
		t.Fatalf("target environment: %v", err)
	}
	if env.VerificationWorkflowID != "workflow.alpha" || !env.Verified {
		t.Fatalf("target environment = %#v", env)
	}
	graph, err := target.GetEnvironmentComponentGraph(ctx, "env.alpha")
	if err != nil {
		t.Fatalf("target component graph: %v", err)
	}
	if len(graph.Components) != 1 || len(graph.Assets) != 1 {
		t.Fatalf("target component graph = %#v", graph)
	}
}

func TestStoreCopyRequirementFailureJSONReportsNotOK(t *testing.T) {
	sourcePath := filepath.Join(t.TempDir(), "source.sqlite")
	targetPath := filepath.Join(t.TempDir(), "target.sqlite")
	sourceRef := "sqlite://" + sourcePath
	targetRef := "sqlite://" + targetPath
	ctx := context.Background()
	source, err := sqlite.Open(ctx, sqlite.Config{Path: sourcePath})
	if err != nil {
		t.Fatalf("open source Store: %v", err)
	}
	defer source.Close()
	now := time.Now().UTC()
	if _, err := source.UpsertEnvironment(ctx, store.Environment{
		ID:        "env.graphless",
		Status:    "draft",
		CreatedAt: now,
		UpdatedAt: now,
	}); err != nil {
		t.Fatalf("seed graphless environment: %v", err)
	}

	out := runCLIFails(t, "store", "copy", "--from", sourceRef, "--to", targetRef, "--require-environment", "env.graphless", "--require-min-components", "1", "--json")
	var report storeCopyStateReport
	if err := json.Unmarshal([]byte(extractJSONObject(t, out)), &report); err != nil {
		t.Fatalf("decode store copy failure json: %v\n%s", err, out)
	}
	if report.OK || !strings.Contains(report.Error, "component graph") {
		t.Fatalf("store copy failure report = %#v raw=%s", report, out)
	}
}

func TestProfileExportWritesActiveStoreCatalogAsProfileBundle(t *testing.T) {
	ctx := context.Background()
	storePath := filepath.Join(t.TempDir(), "store.sqlite")
	runtime, err := sqlite.Open(ctx, sqlite.Config{Path: storePath})
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	now := time.Now().UTC()
	if err := runtime.ReplaceProfileCatalog(ctx, store.ProfileCatalog{
		ProfileID: "profile.export",
		IndexedAt: now,
		Services: []store.CatalogService{{
			ID:          "service.alpha",
			DisplayName: "Service Alpha",
			ServicePort: 18080,
		}},
		Workflows: []store.CatalogWorkflow{{
			ID:          "workflow.alpha",
			DisplayName: "Workflow Alpha",
		}},
		InterfaceNodes: []store.CatalogInterfaceNode{{
			ID:          "node.alpha",
			DisplayName: "Node Alpha",
			ServiceID:   "service.alpha",
			Method:      "GET",
			Path:        "/v1/items",
		}},
		APICases: []store.CatalogAPICase{{
			ID:          "case.alpha",
			DisplayName: "Case Alpha",
			NodeID:      "node.alpha",
			Status:      "active",
		}},
		TemplateConfigs: []store.CatalogTemplateConfig{{
			ID:         "cfg.case.alpha",
			TemplateID: "case-execution",
			NodeID:     "node.alpha",
			ScopeType:  "case",
			ScopeID:    "case.alpha",
			ConfigJSON: `{"caseId":"case.alpha","caseExecution":{"method":"GET","nodeId":"node.alpha","path":"/v1/items","query":{"id":"item-001"},"expectedHttpCodes":[200]}}`,
			Status:     "active",
		}},
	}); err != nil {
		t.Fatalf("seed profile catalog: %v", err)
	}
	if err := runtime.Close(); err != nil {
		t.Fatalf("close store: %v", err)
	}

	outputDir := filepath.Join(t.TempDir(), "exported-profile")
	out := runCLI(t, "profile", "export", "--store", "sqlite://"+storePath, "--output", outputDir, "--json")
	var report struct {
		OK        bool   `json:"ok"`
		ProfileID string `json:"profileId"`
		Output    string `json:"output"`
		Counts    struct {
			Services        int `json:"services"`
			Workflows       int `json:"workflows"`
			InterfaceNodes  int `json:"interfaceNodes"`
			APICases        int `json:"apiCases"`
			TemplateConfigs int `json:"templateConfigs"`
		} `json:"counts"`
	}
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatalf("decode export report: %v\n%s", err, out)
	}
	if !report.OK || report.ProfileID != "profile.export" || report.Output != outputDir || report.Counts.TemplateConfigs != 2 {
		t.Fatalf("export report = %#v", report)
	}
	bundle, err := profile.Load(outputDir)
	if err != nil {
		t.Fatalf("load exported profile: %v", err)
	}
	if bundle.ID != "profile.export" || len(bundle.Services) != 1 || len(bundle.APICases) != 1 || len(bundle.TemplateConfigs) != 2 {
		t.Fatalf("exported bundle = %#v", bundle)
	}
	configs := caseExecutionConfigIDs(bundle.TemplateConfigs)
	if configs["case.alpha"] != "cfg.case.alpha" || !strings.Contains(bundle.TemplateConfigs[1].ConfigJSON+bundle.TemplateConfigs[0].ConfigJSON, `"query":{"id":"item-001"}`) {
		t.Fatalf("exported template configs lost case query: %#v", bundle.TemplateConfigs)
	}
}

func TestStoreDDLCommandPrintsPostgreSQLSchema(t *testing.T) {
	out := runStoreCommand(t, "ddl", "--backend", "postgres")
	if !strings.Contains(out, "create table if not exists schema_versions") {
		t.Fatalf("postgres ddl should include schema_versions table:\n%s", out)
	}
	if !strings.Contains(out, "jsonb") {
		t.Fatalf("postgres ddl should use PostgreSQL jsonb columns:\n%s", out)
	}
}

func TestStoreDDLCommandPrintsMySQLSchema(t *testing.T) {
	out := runStoreCommand(t, "ddl", "--backend", "mysql")
	if !strings.Contains(out, "create table if not exists schema_versions") {
		t.Fatalf("mysql ddl should include schema_versions table:\n%s", out)
	}
	if !strings.Contains(out, "json not null") || !strings.Contains(out, "datetime(6)") {
		t.Fatalf("mysql ddl should use MySQL json and datetime columns:\n%s", out)
	}
	if strings.Contains(out, "create index if not exists") {
		t.Fatalf("mysql ddl should not emit unsupported index-if-not-exists syntax:\n%s", out)
	}
	if !strings.Contains(out, "id varchar(255) primary key") || !strings.Contains(out, "profile_id varchar(128) not null") || !strings.Contains(out, "environment_id varchar(128) not null") {
		t.Fatalf("mysql ddl should use long runtime IDs and bounded graph keys:\n%s", out)
	}
	if !strings.Contains(out, "content_inline mediumtext not null") || !strings.Contains(out, "evidence_root mediumtext not null") {
		t.Fatalf("mysql ddl should use mediumtext so Store metadata is not constrained by small text columns:\n%s", out)
	}
	if strings.Contains(out, "service_dependencies") || strings.Contains(out, "service_config_assets") {
		t.Fatalf("mysql ddl should not include legacy service-only graph tables:\n%s", out)
	}
}

func TestStoreDDLCommandInfersMySQLBackendFromNamedStore(t *testing.T) {
	configHome := t.TempDir()
	t.Setenv("AGENT_TESTBENCH_CONFIG_HOME", configHome)
	runStoreCommand(t, "config", "set", "team-mysql", "--url", "mysql://user:secret@example.com:3306/team_verified?tls=false")

	out := runStoreCommand(t, "ddl", "--store", "team-mysql")
	if !strings.Contains(out, "create table if not exists schema_versions") {
		t.Fatalf("mysql ddl should include schema_versions table:\n%s", out)
	}
	if !strings.Contains(out, "json not null") || !strings.Contains(out, "datetime(6)") {
		t.Fatalf("named mysql ddl should use MySQL json and datetime columns:\n%s", out)
	}
	if strings.Contains(out, "jsonb") || strings.Contains(out, "create index if not exists") {
		t.Fatalf("named mysql ddl should not emit PostgreSQL-specific DDL:\n%s", out)
	}
}

func TestStoreDDLCommandInfersActiveMySQLBackend(t *testing.T) {
	configHome := t.TempDir()
	t.Setenv("AGENT_TESTBENCH_CONFIG_HOME", configHome)
	runStoreCommand(t, "config", "set", "local-mysql", "--url", "mysql://user:secret@example.com:3306/agent_testbench_local?tls=false")
	runStoreCommand(t, "use", "local-mysql")

	out := runStoreCommand(t, "ddl")
	if !strings.Contains(out, "json not null") || !strings.Contains(out, "datetime(6)") {
		t.Fatalf("active mysql ddl should use MySQL DDL:\n%s", out)
	}
	if strings.Contains(out, "jsonb") || strings.Contains(out, "create index if not exists") {
		t.Fatalf("active mysql ddl should not emit PostgreSQL-specific DDL:\n%s", out)
	}
}

func TestStoreConfigCommandsManageActivePostgresStore(t *testing.T) {
	configHome := t.TempDir()
	env := []string{"AGENT_TESTBENCH_CONFIG_HOME=" + configHome}
	dsn := "postgres://user:secret@example.com:5432/agent_testbench_local?sslmode=disable"

	setOut := runCLIWithEnv(t, env, "store", "config", "set", "local-personal", "--url", dsn)
	if !strings.Contains(setOut, "Configured store: local-personal") || !strings.Contains(setOut, "Backend: postgres") {
		t.Fatalf("store config set output = %q", setOut)
	}

	listOut := runCLIWithEnv(t, env, "store", "config", "list")
	if !strings.Contains(listOut, "local-personal") || !strings.Contains(listOut, "postgres://user:xxxxx@example.com:5432/agent_testbench_local?sslmode=disable") {
		t.Fatalf("store config list output = %q", listOut)
	}
	listJSONOut := runCLIWithEnv(t, env, "store", "config", "list", "--json")
	if strings.Contains(listJSONOut, "secret") || !strings.Contains(listJSONOut, "postgres://user:xxxxx@example.com:5432/agent_testbench_local?sslmode=disable") {
		t.Fatalf("store config list json should mask credentials = %q", listJSONOut)
	}

	useOut := runCLIWithEnv(t, env, "store", "use", "local-personal")
	if !strings.Contains(useOut, "Active store: local-personal") {
		t.Fatalf("store use output = %q", useOut)
	}

	currentOut := runCLIWithEnv(t, env, "store", "current", "--json")
	var current struct {
		OK      bool   `json:"ok"`
		Name    string `json:"name"`
		Backend string `json:"backend"`
		URL     string `json:"url"`
	}
	if err := json.Unmarshal([]byte(currentOut), &current); err != nil {
		t.Fatalf("decode current store: %v\n%s", err, currentOut)
	}
	if !current.OK || current.Name != "local-personal" || current.Backend != "postgres" || current.URL != "postgres://user:xxxxx@example.com:5432/agent_testbench_local?sslmode=disable" {
		t.Fatalf("current store = %#v", current)
	}
	if strings.Contains(currentOut, "secret") {
		t.Fatalf("store current json should mask credentials = %q", currentOut)
	}
}

func TestStoreConfigCommandsManageActiveMySQLStore(t *testing.T) {
	configHome := t.TempDir()
	env := []string{"AGENT_TESTBENCH_CONFIG_HOME=" + configHome}
	dsn := "mysql://user:secret@example.com:3306/agent_testbench_local?tls=false"

	setOut := runCLIWithEnv(t, env, "store", "config", "set", "local-mysql", "--url", dsn)
	if !strings.Contains(setOut, "Configured store: local-mysql") || !strings.Contains(setOut, "Backend: mysql") {
		t.Fatalf("store config set output = %q", setOut)
	}

	listJSONOut := runCLIWithEnv(t, env, "store", "config", "list", "--json")
	if strings.Contains(listJSONOut, "secret") || !strings.Contains(listJSONOut, "mysql://user:xxxxx@example.com:3306/agent_testbench_local?tls=false") {
		t.Fatalf("store config list json should mask mysql credentials = %q", listJSONOut)
	}

	runCLIWithEnv(t, env, "store", "use", "local-mysql")
	currentOut := runCLIWithEnv(t, env, "store", "current", "--json")
	var current currentStoreReport
	if err := json.Unmarshal([]byte(currentOut), &current); err != nil {
		t.Fatalf("decode current store: %v\n%s", err, currentOut)
	}
	if !current.OK || current.Name != "local-mysql" || current.Backend != "mysql" || current.URL != "mysql://user:xxxxx@example.com:3306/agent_testbench_local?tls=false" {
		t.Fatalf("current store = %#v", current)
	}
}

func TestStoreConfigSetRejectsInvalidMySQLDSNBeforePersisting(t *testing.T) {
	configHome := t.TempDir()
	env := []string{"AGENT_TESTBENCH_CONFIG_HOME=" + configHome}

	out := runCLIFailsWithEnv(t, env, "store", "config", "set", "broken-mysql", "--url", "mysql://user:secret@example.com:3306")
	if !strings.Contains(out, `store config "broken-mysql" has invalid mysql DSN`) || !strings.Contains(out, "requires database name") {
		t.Fatalf("invalid mysql config output = %q", out)
	}

	listOut := runCLIWithEnv(t, env, "store", "config", "list", "--json")
	if strings.Contains(listOut, "broken-mysql") || strings.Contains(listOut, "secret") {
		t.Fatalf("invalid mysql config should not be persisted or leak credentials = %q", listOut)
	}
}

func TestStoreStatusAndUpgradeRequireActiveStore(t *testing.T) {
	env := []string{"AGENT_TESTBENCH_CONFIG_HOME=" + t.TempDir()}
	for _, command := range []string{"status", "upgrade"} {
		out := runCLIFailsWithEnv(t, env, "store", command)
		if !strings.Contains(out, "no active store configured") || !strings.Contains(out, "store config set NAME --url postgres://") || !strings.Contains(out, "store config set NAME --url mysql://") {
			t.Fatalf("store %s should guide active SQL Store setup, got %q", command, out)
		}
	}
}

func TestStoreStatusSupportsMySQLURLs(t *testing.T) {
	withMySQLSchemaStatus(t, func(_ context.Context, cfg mysql.Config) (mysql.SchemaStatusResult, error) {
		return mysql.SchemaStatusResult{URL: cfg.URL, CurrentVersion: 0, TargetVersion: sqlstore.CurrentSchemaVersion}, nil
	})

	out := runStoreCommand(t, "status", "--store-url", "mysql://user:secret@localhost:3306/agent_testbench")
	if !strings.Contains(out, "Store: mysql") || !strings.Contains(out, "agent_testbench") || strings.Contains(out, "secret") || !strings.Contains(out, fmt.Sprintf("Pending: %d", sqlstore.CurrentSchemaVersion)) {
		t.Fatalf("mysql status output = %q", out)
	}
}

func TestStoreStatusSupportsPostgresURLs(t *testing.T) {
	withPostgresSchemaStatus(t, func(_ context.Context, cfg postgres.Config) (postgres.SchemaStatusResult, error) {
		return postgres.SchemaStatusResult{URL: cfg.URL, CurrentVersion: 0, TargetVersion: sqlstore.CurrentSchemaVersion}, nil
	})

	out := runStoreCommand(t, "status", "--store-url", "postgres://localhost/agent_testbench")
	if !strings.Contains(out, "Store: postgres") || !strings.Contains(out, "Version: 0") || !strings.Contains(out, fmt.Sprintf("Pending: %d", sqlstore.CurrentSchemaVersion)) {
		t.Fatalf("postgres status output = %q", out)
	}
}

func TestStoreStatusCanUseNamedPostgresStore(t *testing.T) {
	configHome := t.TempDir()
	t.Setenv("AGENT_TESTBENCH_CONFIG_HOME", configHome)
	withPostgresSchemaStatus(t, func(_ context.Context, cfg postgres.Config) (postgres.SchemaStatusResult, error) {
		return postgres.SchemaStatusResult{URL: cfg.URL, CurrentVersion: 0, TargetVersion: sqlstore.CurrentSchemaVersion}, nil
	})
	runStoreCommand(t, "config", "set", "team-verified", "--url", "postgres://user:secret@example.com:5432/team_verified?sslmode=disable")

	out := runStoreCommand(t, "status", "--store", "team-verified")
	if !strings.Contains(out, "Store: postgres") || !strings.Contains(out, "team_verified") || strings.Contains(out, "secret") {
		t.Fatalf("named postgres status output = %q", out)
	}
}

func TestStoreStatusCanUseNamedMySQLStore(t *testing.T) {
	configHome := t.TempDir()
	t.Setenv("AGENT_TESTBENCH_CONFIG_HOME", configHome)
	withMySQLSchemaStatus(t, func(_ context.Context, cfg mysql.Config) (mysql.SchemaStatusResult, error) {
		return mysql.SchemaStatusResult{URL: cfg.URL, CurrentVersion: 0, TargetVersion: sqlstore.CurrentSchemaVersion}, nil
	})
	runStoreCommand(t, "config", "set", "team-mysql", "--url", "mysql://user:secret@example.com:3306/team_verified?tls=false")

	out := runStoreCommand(t, "status", "--store", "team-mysql")
	if !strings.Contains(out, "Store: mysql") || !strings.Contains(out, "team_verified") || strings.Contains(out, "secret") {
		t.Fatalf("named mysql status output = %q", out)
	}
}

func TestStoreStatusCanEmitJSONForNamedMySQLStore(t *testing.T) {
	configHome := t.TempDir()
	t.Setenv("AGENT_TESTBENCH_CONFIG_HOME", configHome)
	withMySQLSchemaStatus(t, func(_ context.Context, cfg mysql.Config) (mysql.SchemaStatusResult, error) {
		return mysql.SchemaStatusResult{URL: cfg.URL, CurrentVersion: 1, TargetVersion: sqlstore.CurrentSchemaVersion}, nil
	})
	runStoreCommand(t, "config", "set", "team-mysql", "--url", "mysql://user:secret@example.com:3306/team_verified?tls=false")

	out := runStoreCommand(t, "status", "--store", "team-mysql", "--json")
	var report struct {
		OK             bool   `json:"ok"`
		Backend        string `json:"backend"`
		URL            string `json:"url"`
		CurrentVersion int    `json:"currentVersion"`
		TargetVersion  int    `json:"targetVersion"`
		Pending        int    `json:"pending"`
	}
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatalf("decode status json: %v\n%s", err, out)
	}
	if !report.OK || report.Backend != "mysql" || !strings.Contains(report.URL, "team_verified") || strings.Contains(report.URL, "secret") || report.CurrentVersion != 1 || report.TargetVersion != sqlstore.CurrentSchemaVersion || report.Pending != sqlstore.CurrentSchemaVersion-1 {
		t.Fatalf("mysql status json = %#v raw=%s", report, out)
	}
}

func TestStoreProvisionCanCreateNamedMySQLDatabase(t *testing.T) {
	configHome := t.TempDir()
	t.Setenv("AGENT_TESTBENCH_CONFIG_HOME", configHome)
	withMySQLProvisionDatabase(t, func(_ context.Context, cfg mysql.Config) (mysql.ProvisionDatabaseResult, error) {
		return mysql.ProvisionDatabaseResult{URL: cfg.URL, Database: "team_verified", Created: true}, nil
	})
	runStoreCommand(t, "config", "set", "team-mysql", "--url", "mysql://user:secret@example.com:3306/team_verified?tls=false")

	out := runStoreCommand(t, "provision", "--store", "team-mysql", "--json")
	var report struct {
		OK       bool   `json:"ok"`
		Backend  string `json:"backend"`
		URL      string `json:"url"`
		Database string `json:"database"`
		Created  bool   `json:"created"`
	}
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatalf("decode provision json: %v\n%s", err, out)
	}
	if !report.OK || report.Backend != "mysql" || report.Database != "team_verified" || !report.Created || strings.Contains(report.URL, "secret") {
		t.Fatalf("mysql provision json = %#v raw=%s", report, out)
	}
}

func TestStoreProvisionJSONReportsMySQLConnectionError(t *testing.T) {
	configHome := t.TempDir()
	t.Setenv("AGENT_TESTBENCH_CONFIG_HOME", configHome)
	withMySQLProvisionDatabase(t, func(context.Context, mysql.Config) (mysql.ProvisionDatabaseResult, error) {
		return mysql.ProvisionDatabaseResult{}, errors.New("dial tcp 10.0.20.108:3306: i/o timeout")
	})
	runStoreCommand(t, "config", "set", "team-mysql", "--url", "mysql://user:secret@10.0.20.108:3306/AGENT_TESTBENCH_TEST?tls=false")

	out := runStoreCommandFails(t, "provision", "--store", "team-mysql", "--json")
	var report struct {
		OK            bool   `json:"ok"`
		Backend       string `json:"backend"`
		URL           string `json:"url"`
		TargetVersion int    `json:"targetVersion"`
		Pending       int    `json:"pending"`
		Error         string `json:"error"`
	}
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatalf("decode provision error json: %v\n%s", err, out)
	}
	if report.OK || report.Backend != "mysql" || !strings.Contains(report.URL, "AGENT_TESTBENCH_TEST") || strings.Contains(report.URL, "secret") || !strings.Contains(report.Error, "i/o timeout") {
		t.Fatalf("mysql provision error json = %#v raw=%s", report, out)
	}
}

func TestStoreStatusJSONReportsMySQLConnectionError(t *testing.T) {
	configHome := t.TempDir()
	t.Setenv("AGENT_TESTBENCH_CONFIG_HOME", configHome)
	withMySQLSchemaStatus(t, func(context.Context, mysql.Config) (mysql.SchemaStatusResult, error) {
		return mysql.SchemaStatusResult{}, errors.New("dial tcp 10.0.20.108:3306: i/o timeout")
	})
	runStoreCommand(t, "config", "set", "team-mysql", "--url", "mysql://user:secret@10.0.20.108:3306/AGENT_TESTBENCH_TEST?tls=false")

	out := runStoreCommandFails(t, "status", "--store", "team-mysql", "--json")
	var report struct {
		OK            bool   `json:"ok"`
		Backend       string `json:"backend"`
		URL           string `json:"url"`
		TargetVersion int    `json:"targetVersion"`
		Pending       int    `json:"pending"`
		Error         string `json:"error"`
	}
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatalf("decode status error json: %v\n%s", err, out)
	}
	if report.OK || report.Backend != "mysql" || !strings.Contains(report.URL, "AGENT_TESTBENCH_TEST") || strings.Contains(report.URL, "secret") || report.TargetVersion != sqlstore.CurrentSchemaVersion || report.Pending != sqlstore.CurrentSchemaVersion || !strings.Contains(report.Error, "i/o timeout") {
		t.Fatalf("mysql status error json = %#v raw=%s", report, out)
	}
}

func TestStoreReferenceResolutionKeepsLocalAndRemotePostgresCommandShape(t *testing.T) {
	configHome := t.TempDir()
	t.Setenv("AGENT_TESTBENCH_CONFIG_HOME", configHome)
	cfg := storeConfigFile{Stores: map[string]storeConfigEntry{}}
	local, err := newStoreConfigEntry("local-personal", "postgres://tester:secret@localhost:5432/local_personal?sslmode=disable")
	if err != nil {
		t.Fatalf("local config entry: %v", err)
	}
	remote, err := newStoreConfigEntry("team-verified", "postgres://tester:secret@pg.example.com:5432/team_verified?sslmode=require")
	if err != nil {
		t.Fatalf("remote config entry: %v", err)
	}
	cfg.Stores[local.Name] = local
	cfg.Stores[remote.Name] = remote
	cfg.Active = local.Name
	if err := saveStoreConfig(cfg); err != nil {
		t.Fatalf("save store config: %v", err)
	}

	localURL, err := resolveStoreReference("local-personal", "")
	if err != nil {
		t.Fatalf("resolve local store: %v", err)
	}
	remoteURL, err := resolveStoreReference("team-verified", "")
	if err != nil {
		t.Fatalf("resolve remote store: %v", err)
	}
	activeURL, err := resolveStoreReference("", "")
	if err != nil {
		t.Fatalf("resolve active store: %v", err)
	}
	if localURL != local.URL || remoteURL != remote.URL || activeURL != local.URL {
		t.Fatalf("resolved urls local=%q remote=%q active=%q", localURL, remoteURL, activeURL)
	}
}

func TestLegacyStoreURLPathIsExplicitSQLiteCompatibility(t *testing.T) {
	storePath := filepath.Join(t.TempDir(), "store.sqlite")
	resolved, err := resolveRequiredStoreReference("", storePath)
	if err != nil {
		t.Fatalf("resolve legacy store url path: %v", err)
	}
	if resolved != "sqlite://"+storePath {
		t.Fatalf("legacy store url path = %q want sqlite://%s", resolved, storePath)
	}
}

func TestDailyStoreReferenceRejectsLegacySQLiteStoreURL(t *testing.T) {
	storePath := filepath.Join(t.TempDir(), "store.sqlite")
	for _, legacyStoreURL := range []string{storePath, "sqlite://" + storePath} {
		_, err := resolveRequiredDailyStoreReference("", legacyStoreURL)
		if err == nil {
			t.Fatalf("daily Store reference should reject legacy SQLite store URL %q", legacyStoreURL)
		}
		for _, want := range []string{"--store-url", "compatibility", "daily commands", "--store NAME_OR_DSN", "sqlite://PATH"} {
			if !strings.Contains(err.Error(), want) {
				t.Fatalf("daily Store reference error missing %q: %v", want, err)
			}
		}
	}
}

func TestDailyStoreReferenceAcceptsNamedSQLiteConfig(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("AGENT_TESTBENCH_CONFIG_HOME", filepath.Join(dir, "config"))
	storeRef := "sqlite://" + filepath.Join(dir, "store.sqlite")
	if err := saveStoreConfig(storeConfigFile{
		Stores: map[string]storeConfigEntry{
			"local-sqlite": {Name: "local-sqlite", URL: storeRef, Backend: "sqlite"},
		},
	}); err != nil {
		t.Fatalf("save store config: %v", err)
	}

	resolved, err := resolveRequiredDailyStoreReference("local-sqlite", "")
	if err != nil {
		t.Fatalf("daily Store reference should accept named SQLite config: %v", err)
	}
	if resolved != storeRef {
		t.Fatalf("named SQLite Store resolved to %q want %q", resolved, storeRef)
	}
}

func TestDailyStoreReferenceAcceptsDirectSQLiteStoreFlag(t *testing.T) {
	storePath := filepath.Join(t.TempDir(), "store.sqlite")
	for _, tc := range []struct {
		storeRef string
		want     string
	}{
		{storeRef: "sqlite://" + storePath, want: "sqlite://" + storePath},
		{storeRef: "file://" + storePath, want: "file://" + storePath},
	} {
		resolved, err := resolveRequiredDailyStoreReference(tc.storeRef, "")
		if err != nil {
			t.Fatalf("daily Store reference should accept explicit SQLite Store flag %q: %v", tc.storeRef, err)
		}
		if resolved != tc.want {
			t.Fatalf("direct SQLite Store flag = %q want %q", resolved, tc.want)
		}
	}
}
