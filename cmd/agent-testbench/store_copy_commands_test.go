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

func TestCopyStoreCurrentStateCopiesCatalogAndEnvironmentGraph(t *testing.T) {
	ctx := context.Background()
	source, target, targetPath := openStoreCopySQLitePair(t, ctx)
	now := time.Now().UTC()

	seedStoreCopyProfileState(t, ctx, source, now)
	seedStoreCopyEnvironmentState(t, ctx, source, now)
	seedStoreCopyComponentGraph(t, ctx, source)

	report, err := copyStoreCurrentState(ctx, source, target)
	if err != nil {
		t.Fatalf("copy store state: %v", err)
	}
	requireStoreCopyCurrentStateReport(t, report)
	requireStoreCopyRequirementValidation(t, report)
	requireStoreCopyTargetState(t, ctx, target, targetPath)
}

func openStoreCopySQLitePair(t *testing.T, ctx context.Context) (*sqlite.Store, *sqlite.Store, string) {
	t.Helper()

	source, err := sqlite.Open(ctx, sqlite.Config{Path: filepath.Join(t.TempDir(), "source.sqlite")})
	if err != nil {
		t.Fatalf("open source: %v", err)
	}
	t.Cleanup(func() { _ = source.Close() })
	targetPath := filepath.Join(t.TempDir(), "target.sqlite")
	target, err := sqlite.Open(ctx, sqlite.Config{Path: targetPath})
	if err != nil {
		t.Fatalf("open target: %v", err)
	}
	t.Cleanup(func() { _ = target.Close() })
	return source, target, targetPath
}

func seedStoreCopyProfileState(t *testing.T, ctx context.Context, source *sqlite.Store, now time.Time) {
	t.Helper()

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
}

func seedStoreCopyEnvironmentState(t *testing.T, ctx context.Context, source *sqlite.Store, now time.Time) {
	t.Helper()

	if _, err := source.UpsertEnvironment(ctx, store.Environment{
		ID:                     "env.alpha",
		DisplayName:            "Environment Alpha",
		Status:                 "verified",
		Verified:               true,
		ServicesJSON:           `[]`,
		ReposJSON:              `{}`,
		ComposeJSON:            `{"composeFiles":["compose.yml"],"envFiles":["legacy.env"],"generatedFiles":{"compose.yml":"legacy compose should not survive structured copy\n","legacy.env":"LEGACY_MODE=true\n"}}`,
		HealthChecksJSON:       `[]`,
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
	if err := source.ReplaceEnvironmentFiles(ctx, "env.alpha", []store.EnvironmentFile{
		{
			Path:          "compose.yml",
			Kind:          store.EnvironmentFileKindComposeFile,
			ContentInline: "services:\n  service-alpha:\n    image: alpine:3.20\n",
			Required:      true,
			ApplyOrder:    10,
			SummaryJSON:   `{"source":"test.structured"}`,
		},
		{
			Path:        "legacy.env",
			Kind:        store.EnvironmentFileKindComposeEnvFile,
			Required:    true,
			ApplyOrder:  20,
			SummaryJSON: `{"source":"test.reference-only"}`,
		},
	}); err != nil {
		t.Fatalf("seed environment files: %v", err)
	}
	if err := source.ReplaceEnvironmentServices(ctx, "env.alpha", []store.EnvironmentService{{
		ServiceID:   "service.alpha",
		RepoURL:     "https://example.invalid/service-alpha.git",
		Checkout:    "service-alpha",
		SummaryJSON: `{"source":"test.structured"}`,
	}}); err != nil {
		t.Fatalf("seed environment services: %v", err)
	}
	if err := source.ReplaceEnvironmentHealthChecks(ctx, "env.alpha", []store.EnvironmentHealthCheck{{
		CheckID:     "alpha",
		Kind:        "url",
		URL:         "http://127.0.0.1:18080/health",
		ApplyOrder:  1,
		SummaryJSON: `{"source":"test.structured"}`,
	}}); err != nil {
		t.Fatalf("seed environment health checks: %v", err)
	}
}

func seedStoreCopyComponentGraph(t *testing.T, ctx context.Context, source *sqlite.Store) {
	t.Helper()

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
}

func requireStoreCopyCurrentStateReport(t *testing.T, report storeCopyStateReport) {
	t.Helper()

	if report.ProfileCatalogs != 1 || report.ProfileIndexes != 1 || report.ConfigVersions != 1 || len(report.ReadModels) == 0 || !report.RunsSkipped || report.Environments != 1 || report.ComponentGraphs != 1 {
		t.Fatalf("copy report = %#v", report)
	}
	if len(report.EnvironmentIDs) != 1 || report.EnvironmentIDs[0] != "env.alpha" || len(report.EnvironmentRefs) != 1 || !report.EnvironmentRefs[0].Verified || report.EnvironmentRefs[0].VerificationWorkflowID != "workflow.alpha" {
		t.Fatalf("copy environment refs = %#v ids=%#v", report.EnvironmentRefs, report.EnvironmentIDs)
	}
	if len(report.ComponentRefs) != 1 || report.ComponentRefs[0].EnvironmentID != "env.alpha" || report.ComponentRefs[0].Components != 1 || report.ComponentRefs[0].Assets != 1 || report.ComponentRefs[0].InlineAssetBytes != len("services: {}\n") || report.ComponentRefs[0].LargestInlineAssetID != "compose.alpha" {
		t.Fatalf("copy component refs = %#v", report.ComponentRefs)
	}
}

func requireStoreCopyRequirementValidation(t *testing.T, report storeCopyStateReport) {
	t.Helper()

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
}

func requireStoreCopyTargetState(t *testing.T, ctx context.Context, target *sqlite.Store, targetPath string) {
	t.Helper()

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
	if !strings.Contains(env.ComposeJSON, "service-alpha") || !strings.Contains(env.ServicesJSON, "https://example.invalid/service-alpha.git") || !strings.Contains(env.HealthChecksJSON, "127.0.0.1") {
		t.Fatalf("target environment should hydrate structured Docker metadata: compose=%s services=%s health=%s", env.ComposeJSON, env.ServicesJSON, env.HealthChecksJSON)
	}
	rawComposeJSON := sqliteScalar(t, targetPath, `select compose_json from environments where id = 'env.alpha';`)
	if strings.Contains(rawComposeJSON, "service-alpha") || strings.Contains(rawComposeJSON, "legacy compose should not survive") {
		t.Fatalf("raw target compose_json should not carry hydrated structured file content: %s", rawComposeJSON)
	}
	if !strings.Contains(rawComposeJSON, "LEGACY_MODE=true") {
		t.Fatalf("raw target compose_json should preserve legacy generated content not backed by materialized environment_files: %s", rawComposeJSON)
	}
	if rawServicesJSON := sqliteScalar(t, targetPath, `select services_json from environments where id = 'env.alpha';`); rawServicesJSON != "[]" {
		t.Fatalf("raw target services_json should not carry structured services: %s", rawServicesJSON)
	}
	if rawHealthJSON := sqliteScalar(t, targetPath, `select health_checks_json from environments where id = 'env.alpha';`); rawHealthJSON != "[]" {
		t.Fatalf("raw target health_checks_json should not carry structured health checks: %s", rawHealthJSON)
	}
	files, err := target.ListEnvironmentFiles(ctx, "env.alpha")
	if err != nil {
		t.Fatalf("target environment files: %v", err)
	}
	if len(files) != 2 || files[0].Path != "compose.yml" || !strings.Contains(files[0].ContentInline, "service-alpha") || files[1].Path != "legacy.env" || files[1].ContentInline != "" {
		t.Fatalf("target environment files = %#v", files)
	}
	services, err := target.ListEnvironmentServices(ctx, "env.alpha")
	if err != nil {
		t.Fatalf("target environment services: %v", err)
	}
	if len(services) != 1 || services[0].ServiceID != "service.alpha" || services[0].Checkout != "service-alpha" {
		t.Fatalf("target environment services = %#v", services)
	}
	healthChecks, err := target.ListEnvironmentHealthChecks(ctx, "env.alpha")
	if err != nil {
		t.Fatalf("target environment health checks: %v", err)
	}
	if len(healthChecks) != 1 || healthChecks[0].Kind != "url" || healthChecks[0].URL != "http://127.0.0.1:18080/health" {
		t.Fatalf("target environment health checks = %#v", healthChecks)
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
