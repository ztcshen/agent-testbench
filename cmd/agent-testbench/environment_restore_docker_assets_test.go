package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"agent-testbench/internal/store"
)

func TestEnvironmentRestoreAppliesAssetsBoundToDependencyEdges(t *testing.T) {
	workspace := filepath.Join(t.TempDir(), "workspace")
	fakeDockerEnv, dockerCallsPath := fakeDockerCommand(t)
	healthURL := newHealthyTestURL(t)
	for _, kv := range fakeDockerEnv {
		parts := strings.SplitN(kv, "=", 2)
		t.Setenv(parts[0], parts[1])
	}
	report, err := buildEnvironmentRestoreReport(context.Background(), store.Environment{
		ID: "env.edge.assets",
		ComposeJSON: `{
			"composeFile":"compose.yml",
			"generatedFiles":{
				"compose.yml":"services:\n  mysql:\n    image: mysql:8\n  apollo:\n    image: wiremock/wiremock\n  app:\n    image: alpine:3.20\n",
				"compose/platform/apollo/mappings/app.json":"{\"request\":{\"url\":\"/configs/app\"},\"response\":{\"status\":200}}\n"
			},
			"services":["mysql","apollo","app"],
			"skipPull":true,
			"skipBuild":true
		}`,
		HealthChecksJSON:       `[]`,
		VerificationWorkflowID: "workflow.edge-assets",
	}, workspace, true, false, false, time.Second, environmentRestoreWorkflowOptions{}, environmentRestoreDockerCleanupOptions{}, store.EnvironmentComponentGraph{
		Components: []store.EnvironmentComponent{
			{ComponentID: "mysql", Kind: "middleware", Role: "database", ComposeService: "mysql", Required: true, HealthCheckJSON: `{"type":"compose-service","service":"mysql"}`},
			{ComponentID: "apollo", Kind: "middleware", Role: "config", ComposeService: "apollo", Required: true, HealthCheckJSON: `{"type":"compose-service","service":"apollo"}`},
			{ComponentID: "app", Kind: "app", Role: "business-service", ComposeService: "app", Required: true, HealthCheckJSON: `{"type":"url","url":"` + healthURL + `"}`},
		},
		Dependencies: []store.ComponentDependency{
			{ConsumerComponentID: "app", ProviderComponentID: "mysql", Phase: "startup", Capability: "sql", Required: true, ProfileJSON: `{"assetIds":["app.mysql.schema"]}`},
			{ConsumerComponentID: "app", ProviderComponentID: "apollo", Phase: "startup", Capability: "config", Required: true, ProfileJSON: `{"assetIds":["app.apollo.config"]}`},
		},
		Assets: []store.ComponentConfigAsset{
			{OwnerComponentID: "app", AssetID: "app.mysql.schema", AssetKind: "mysql-ddl", TargetComponentID: "mysql", TargetPath: "compose/mysql/init/app.sql", ContentInline: "create database if not exists app;\n", SizeBytes: int64(len("create database if not exists app;\n")), ApplyOrder: 10, SummaryJSON: `{}`},
			{OwnerComponentID: "app", AssetID: "app.apollo.config", AssetKind: "apollo-config", TargetComponentID: "apollo", TargetPath: "compose/platform/apollo/mappings/app.json", ContentInline: "{\"request\":{\"url\":\"/configs/app\"},\"response\":{\"status\":200}}\n", ApplyOrder: 20, SummaryJSON: `{}`},
		},
	})
	if err != nil {
		t.Fatalf("build edge asset restore report: %v", err)
	}
	if !report.OK || !report.Docker.OK || len(report.Docker.AppliedAssets) != 2 {
		t.Fatalf("edge asset restore report = %#v", report.Docker)
	}
	actions := map[string]string{}
	for _, asset := range report.Docker.AppliedAssets {
		actions[asset.AssetID] = asset.Action
	}
	if actions["app.mysql.schema"] != "apply-mysql-sql" || actions["app.apollo.config"] != "verify-generated-file" {
		t.Fatalf("edge asset actions = %#v assets=%#v", actions, report.Docker.AppliedAssets)
	}
	dockerCalls, err := os.ReadFile(dockerCallsPath)
	if err != nil {
		t.Fatalf("read fake docker calls: %v", err)
	}
	if !strings.Contains(string(dockerCalls), "compose -f "+filepath.Join(workspace, "compose.yml")+" up -d mysql apollo app") ||
		!strings.Contains(string(dockerCalls), "compose -f "+filepath.Join(workspace, "compose.yml")+" exec -T mysql sh -lc") ||
		strings.Contains(string(dockerCalls), "-proot") {
		t.Fatalf("edge asset docker calls:\n%s", dockerCalls)
	}
}

func TestEnvironmentRestoreEdgeAssetsAvoidNonSQLMySQLAndDuplicateApply(t *testing.T) {
	workspace := filepath.Join(t.TempDir(), "workspace")
	writeFile(t, filepath.Join(workspace, "compose", "mysql", "config.cnf"), "[mysqld]\n")
	graph := store.EnvironmentComponentGraph{
		Components: []store.EnvironmentComponent{
			{ComponentID: "mysql", Kind: "middleware", Role: "database", ComposeService: "mysql"},
			{ComponentID: "app", Kind: "app", Role: "business-service", ComposeService: "app"},
			{ComponentID: "worker", Kind: "app", Role: "worker", ComposeService: "worker"},
		},
		Dependencies: []store.ComponentDependency{
			{ConsumerComponentID: "app", ProviderComponentID: "mysql", Capability: "config", ProfileJSON: `{"assetIds":["mysql.config"]}`},
			{ConsumerComponentID: "app", ProviderComponentID: "mysql", Capability: "sql", ProfileJSON: `{"assetIds":["shared.schema"]}`},
			{ConsumerComponentID: "worker", ProviderComponentID: "mysql", Capability: "sql", ProfileJSON: `{"assetIds":["shared.schema"]}`},
		},
		Assets: []store.ComponentConfigAsset{
			{OwnerComponentID: "mysql", AssetID: "mysql.config", AssetKind: "mysql-config", TargetComponentID: "mysql", TargetPath: "compose/mysql/config.cnf"},
			{OwnerComponentID: "app", AssetID: "shared.schema", AssetKind: "mysql-ddl", TargetComponentID: "mysql", TargetPath: "compose/mysql/init/shared.sql", ContentInline: "create database if not exists app;\n"},
		},
	}
	items := environmentRestoreApplyEdgeAssets(context.Background(), graph, map[string]any{
		"generatedFiles": map[string]any{
			"compose/mysql/config.cnf": "[mysqld]\n",
		},
	}, workspace, false, []string{"-f", "compose.yml"})
	if len(items) != 2 {
		t.Fatalf("edge assets should dedupe repeated asset ids, got %#v", items)
	}
	actions := map[string]string{}
	commands := map[string]string{}
	for _, item := range items {
		actions[item.AssetID] = item.Action
		commands[item.AssetID] = strings.Join(item.Command, " ")
	}
	if actions["mysql.config"] != "project-generated-file" || commands["mysql.config"] != "" {
		t.Fatalf("non-SQL MySQL asset should not run through mysql client: actions=%#v commands=%#v", actions, commands)
	}
	if actions["shared.schema"] != "plan-apply-mysql-sql" || strings.Contains(commands["shared.schema"], "-proot") || !strings.Contains(commands["shared.schema"], "MYSQL_ROOT_PASSWORD") {
		t.Fatalf("SQL MySQL asset command should use container env credentials: actions=%#v commands=%#v", actions, commands)
	}
	if strings.Contains(commands["shared.schema"], "MYSQL_DATABASE") || !strings.Contains(commands["shared.schema"], "AGENT_TESTBENCH_MYSQL_APPLY_DATABASE") {
		t.Fatalf("SQL MySQL asset command should not force MYSQL_DATABASE by default: %#v", commands)
	}
}

func TestEnvironmentRestoreEdgeAssetsRequireMySQLProviderSignal(t *testing.T) {
	workspace := t.TempDir()
	graph := store.EnvironmentComponentGraph{
		Components: []store.EnvironmentComponent{
			{ComponentID: "postgres", Kind: "middleware", Role: "database", ComposeService: "postgres"},
			{ComponentID: "mysql.primary", Kind: "middleware", Role: "database", ComposeService: "mysql"},
			{ComponentID: "app", Kind: "app", Role: "business-service", ComposeService: "app"},
			{ComponentID: "worker", Kind: "app", Role: "worker", ComposeService: "worker"},
		},
		Dependencies: []store.ComponentDependency{
			{ConsumerComponentID: "app", ProviderComponentID: "postgres", Capability: "sql", ProfileJSON: `{"assetIds":["postgres.schema"]}`},
			{ConsumerComponentID: "app", ProviderComponentID: "mysql.primary", Capability: "sql", ProfileJSON: `{"assetIds":["shared.schema"]}`},
			{ConsumerComponentID: "worker", ProviderComponentID: "postgres", Capability: "sql", ProfileJSON: `{"assetIds":["shared.schema"]}`},
		},
		Assets: []store.ComponentConfigAsset{
			{OwnerComponentID: "app", AssetID: "postgres.schema", AssetKind: "postgres-ddl", TargetComponentID: "postgres", TargetPath: "postgres.sql", ContentInline: "create schema app;\n"},
			{OwnerComponentID: "app", AssetID: "shared.schema", AssetKind: "schema", TargetPath: "shared.sql", ContentInline: "create database if not exists shared;\n"},
		},
	}
	items := environmentRestoreApplyEdgeAssets(context.Background(), graph, nil, workspace, false, []string{"-f", "compose.yml"})
	if len(items) != 3 {
		t.Fatalf("shared asset should be applied once per effective target, got %#v", items)
	}
	actionsByTarget := map[string]string{}
	for _, item := range items {
		actionsByTarget[item.AssetID+"@"+item.TargetComponentID] = item.Action
	}
	if actionsByTarget["postgres.schema@postgres"] == "plan-apply-mysql-sql" {
		t.Fatalf("postgres SQL asset should not use MySQL apply: %#v", actionsByTarget)
	}
	if actionsByTarget["shared.schema@mysql.primary"] != "plan-apply-mysql-sql" {
		t.Fatalf("shared schema should use MySQL apply for MySQL target: %#v", actionsByTarget)
	}
	if actionsByTarget["shared.schema@postgres"] == "plan-apply-mysql-sql" {
		t.Fatalf("shared schema should not use MySQL apply for PostgreSQL target: %#v", actionsByTarget)
	}
}

func TestEnvironmentRestoreSeedsObjectStorageEdgeAsset(t *testing.T) {
	workspace := t.TempDir()
	seedPath := filepath.Join(workspace, "seeded-object.txt")
	graph := store.EnvironmentComponentGraph{
		Components: []store.EnvironmentComponent{
			{
				ComponentID:    "object-store",
				Kind:           "middleware",
				Role:           "object-storage",
				ComposeService: "minio",
				RuntimeJSON:    `{"objectStorage":{"seedCommand":["sh","-lc","cat > seeded-object.txt"]}}`,
			},
			{ComponentID: "worker", Kind: "app", Role: "worker", ComposeService: "worker"},
		},
		Dependencies: []store.ComponentDependency{
			{ConsumerComponentID: "worker", ProviderComponentID: "object-store", Capability: "object-storage", ProfileJSON: `{"assetIds":["object.fixture"]}`},
		},
		Assets: []store.ComponentConfigAsset{
			{
				OwnerComponentID:  "worker",
				AssetID:           "object.fixture",
				AssetKind:         "object-storage-object",
				TargetComponentID: "object-store",
				ContentInline:     "fixture-body",
				SummaryJSON:       `{"bucket":"fixtures","key":"cases/input.json"}`,
			},
		},
	}

	items := environmentRestoreApplyEdgeAssets(context.Background(), graph, nil, workspace, true, []string{"-f", "compose.yml"})
	if len(items) != 1 {
		t.Fatalf("object storage asset count = %d items=%#v", len(items), items)
	}
	item := items[0]
	if !item.OK || item.Action != actionSeedObjectStorage || item.TargetComposeService != "minio" || item.TargetPath != "fixtures/cases/input.json" || item.Bytes != len("fixture-body") {
		t.Fatalf("object storage asset item = %#v", item)
	}
	raw, err := os.ReadFile(seedPath)
	if err != nil || string(raw) != "fixture-body" {
		t.Fatalf("seeded object content = %q err=%v", raw, err)
	}
}

func TestEnvironmentRestoreSeedsS3ObjectAssetKindWithoutCapability(t *testing.T) {
	workspace := t.TempDir()
	seedPath := filepath.Join(workspace, "seeded-s3-object.txt")
	graph := store.EnvironmentComponentGraph{
		Components: []store.EnvironmentComponent{
			{
				ComponentID:    "object-store",
				Kind:           "middleware",
				Role:           "object-storage",
				ComposeService: "minio",
				RuntimeJSON:    `{"objectStorage":{"seedCommand":["sh","-lc","cat > seeded-s3-object.txt"]}}`,
			},
			{ComponentID: "worker", Kind: "app", Role: "worker", ComposeService: "worker"},
		},
		Dependencies: []store.ComponentDependency{
			{ConsumerComponentID: "worker", ProviderComponentID: "object-store", Capability: "blob", ProfileJSON: `{"assetIds":["s3.fixture"]}`},
		},
		Assets: []store.ComponentConfigAsset{
			{OwnerComponentID: "worker", AssetID: "s3.fixture", AssetKind: "s3-object", TargetComponentID: "object-store", ContentInline: "fixture-body", SummaryJSON: `{"bucket":"fixtures","key":"cases/input.json"}`},
		},
	}

	items := environmentRestoreApplyEdgeAssets(context.Background(), graph, nil, workspace, true, []string{"-f", "compose.yml"})
	if len(items) != 1 || !items[0].OK || items[0].Action != actionSeedObjectStorage {
		t.Fatalf("s3 object kind should seed through object-storage path: %#v", items)
	}
	raw, err := os.ReadFile(seedPath)
	if err != nil || string(raw) != "fixture-body" {
		t.Fatalf("seeded s3 object content = %q err=%v", raw, err)
	}
}

func TestEnvironmentRestoreObjectStorageIgnoresRemoteSourcePathForObjectKey(t *testing.T) {
	workspace := t.TempDir()
	locationPath := filepath.Join(workspace, "seeded-location.txt")
	graph := store.EnvironmentComponentGraph{
		Components: []store.EnvironmentComponent{
			{
				ComponentID:    "object-store",
				Kind:           "middleware",
				Role:           "object-storage",
				ComposeService: "minio",
				RuntimeJSON:    `{"objectStorage":{"seedCommand":["sh","-lc","printf '%s/%s' '{bucket}' '{key}' > seeded-location.txt"]}}`,
			},
			{ComponentID: "worker", Kind: "app", Role: "worker", ComposeService: "worker"},
		},
		Dependencies: []store.ComponentDependency{
			{ConsumerComponentID: "worker", ProviderComponentID: "object-store", Capability: objectStorageCapability, ProfileJSON: `{"assetIds":["object.remote"]}`},
		},
		Assets: []store.ComponentConfigAsset{
			{
				OwnerComponentID:  "worker",
				AssetID:           "object.remote",
				AssetKind:         "object-storage-object",
				TargetComponentID: "object-store",
				TargetPath:        "fixtures/cases/input.json",
				ContentInline:     "fixture-body",
				RemoteRefJSON:     `{"path":"private/source/file.json"}`,
			},
		},
	}

	items := environmentRestoreApplyEdgeAssets(context.Background(), graph, nil, workspace, true, []string{"-f", "compose.yml"})
	if len(items) != 1 || !items[0].OK || items[0].TargetPath != "fixtures/cases/input.json" {
		t.Fatalf("remote source path should not override target bucket/key: %#v", items)
	}
	raw, err := os.ReadFile(locationPath)
	if err != nil || string(raw) != "fixtures/cases/input.json" {
		t.Fatalf("seeded object location = %q err=%v", raw, err)
	}
}

func TestEnvironmentRestoreObjectStoragePreservesMissingSourceError(t *testing.T) {
	workspace := t.TempDir()
	graph := store.EnvironmentComponentGraph{
		Components: []store.EnvironmentComponent{
			{
				ComponentID:    "object-store",
				Kind:           "middleware",
				Role:           "object-storage",
				ComposeService: "minio",
				RuntimeJSON:    `{"objectStorage":{"seedCommand":["sh","-lc","cat > seeded-object.txt"]}}`,
			},
			{ComponentID: "worker", Kind: "app", Role: "worker", ComposeService: "worker"},
		},
		Dependencies: []store.ComponentDependency{
			{ConsumerComponentID: "worker", ProviderComponentID: "object-store", Capability: objectStorageCapability, ProfileJSON: `{"assetIds":["object.missing"]}`},
		},
		Assets: []store.ComponentConfigAsset{
			{OwnerComponentID: "worker", AssetID: "object.missing", AssetKind: "object-storage-object", TargetComponentID: "object-store", TargetPath: "fixtures/missing.json"},
		},
	}

	items := environmentRestoreApplyEdgeAssets(context.Background(), graph, nil, workspace, true, []string{"-f", "compose.yml"})
	if len(items) != 1 || items[0].OK || !strings.Contains(items[0].Error, "read edge asset content from fixtures/missing.json") {
		t.Fatalf("missing object source should fail instead of seeding empty content: %#v", items)
	}
}

func TestEnvironmentRestoreObjectStorageDependencyKeepsGeneratedConfigAsset(t *testing.T) {
	workspace := t.TempDir()
	writeFile(t, filepath.Join(workspace, "compose", "object-store", "config.env"), "ACCESS_KEY=test\n")
	graph := store.EnvironmentComponentGraph{
		Components: []store.EnvironmentComponent{
			{ComponentID: "object-store", Kind: "middleware", Role: "object-storage", ComposeService: "minio"},
			{ComponentID: "worker", Kind: "app", Role: "worker", ComposeService: "worker"},
		},
		Dependencies: []store.ComponentDependency{
			{ConsumerComponentID: "worker", ProviderComponentID: "object-store", Capability: objectStorageCapability, ProfileJSON: `{"assetIds":["object.config"]}`},
		},
		Assets: []store.ComponentConfigAsset{
			{OwnerComponentID: "object-store", AssetID: "object.config", AssetKind: "object-storage-config", TargetComponentID: "object-store", TargetPath: "compose/object-store/config.env"},
		},
	}

	items := environmentRestoreApplyEdgeAssets(context.Background(), graph, map[string]any{
		"generatedFiles": map[string]any{
			"compose/object-store/config.env": "ACCESS_KEY=test\n",
		},
	}, workspace, true, []string{"-f", "compose.yml"})
	if len(items) != 1 || !items[0].OK || items[0].Action != "verify-generated-file" {
		t.Fatalf("object-storage dependency config asset should stay generated-file asset: %#v", items)
	}
}

func TestEnvironmentRestoreObjectStorageRequiresExplicitEmptyMetadata(t *testing.T) {
	workspace := t.TempDir()
	graph := store.EnvironmentComponentGraph{
		Components: []store.EnvironmentComponent{
			{
				ComponentID:    "object-store",
				Kind:           "middleware",
				Role:           "object-storage",
				ComposeService: "minio",
				RuntimeJSON:    `{"objectStorage":{"seedCommand":["sh","-lc","cat > seeded-object.txt"]}}`,
			},
			{ComponentID: "worker", Kind: "app", Role: "worker", ComposeService: "worker"},
		},
		Dependencies: []store.ComponentDependency{
			{ConsumerComponentID: "worker", ProviderComponentID: "object-store", Capability: objectStorageCapability, ProfileJSON: `{"assetIds":["object.no-source"]}`},
		},
		Assets: []store.ComponentConfigAsset{
			{OwnerComponentID: "worker", AssetID: "object.no-source", AssetKind: "object-storage-object", TargetComponentID: "object-store", SummaryJSON: `{"bucket":"fixtures","key":"empty.marker"}`},
		},
	}

	items := environmentRestoreApplyEdgeAssets(context.Background(), graph, nil, workspace, true, []string{"-f", "compose.yml"})
	if len(items) != 1 || items[0].OK || !strings.Contains(items[0].Error, "edge asset target path is required") {
		t.Fatalf("empty object should require explicit empty metadata: %#v", items)
	}
}

func TestEnvironmentRestoreObjectStorageRequiresNumericZeroEmptyMetadata(t *testing.T) {
	workspace := t.TempDir()
	graph := store.EnvironmentComponentGraph{
		Components: []store.EnvironmentComponent{
			{
				ComponentID:    "object-store",
				Kind:           "middleware",
				Role:           "object-storage",
				ComposeService: "minio",
				RuntimeJSON:    `{"objectStorage":{"seedCommand":["sh","-lc","cat > seeded-object.txt"]}}`,
			},
			{ComponentID: "worker", Kind: "app", Role: "worker", ComposeService: "worker"},
		},
		Dependencies: []store.ComponentDependency{
			{ConsumerComponentID: "worker", ProviderComponentID: "object-store", Capability: objectStorageCapability, ProfileJSON: `{"assetIds":["object.string-size"]}`},
		},
		Assets: []store.ComponentConfigAsset{
			{OwnerComponentID: "worker", AssetID: "object.string-size", AssetKind: "object-storage-object", TargetComponentID: "object-store", SummaryJSON: `{"bucket":"fixtures","key":"empty.marker","sizeBytes":"0"}`},
		},
	}

	items := environmentRestoreApplyEdgeAssets(context.Background(), graph, nil, workspace, true, []string{"-f", "compose.yml"})
	if len(items) != 1 || items[0].OK || !strings.Contains(items[0].Error, "edge asset target path is required") {
		t.Fatalf("string size metadata should not authorize empty object content: %#v", items)
	}
}

func TestEnvironmentRestoreObjectStorageCapabilitySeedsGenericTargetPathObject(t *testing.T) {
	workspace := t.TempDir()
	seedPath := filepath.Join(workspace, "generic-object.txt")
	graph := store.EnvironmentComponentGraph{
		Components: []store.EnvironmentComponent{
			{
				ComponentID:    "object-store",
				Kind:           "middleware",
				Role:           "object-storage",
				ComposeService: "minio",
				RuntimeJSON:    `{"objectStorage":{"seedCommand":["sh","-lc","printf '%s/%s:' '{bucket}' '{key}' > generic-object.txt; cat >> generic-object.txt"]}}`,
			},
			{ComponentID: "worker", Kind: "app", Role: "worker", ComposeService: "worker"},
		},
		Dependencies: []store.ComponentDependency{
			{ConsumerComponentID: "worker", ProviderComponentID: "object-store", Capability: objectStorageCapability, ProfileJSON: `{"assetIds":["generic.fixture"]}`},
		},
		Assets: []store.ComponentConfigAsset{
			{OwnerComponentID: "worker", AssetID: "generic.fixture", AssetKind: "fixture", TargetComponentID: "object-store", TargetPath: "fixtures/cases/input.json", ContentInline: "fixture-body"},
		},
	}

	items := environmentRestoreApplyEdgeAssets(context.Background(), graph, nil, workspace, true, []string{"-f", "compose.yml"})
	if len(items) != 1 || !items[0].OK || items[0].Action != actionSeedObjectStorage || items[0].TargetPath != "fixtures/cases/input.json" {
		t.Fatalf("generic target-path object should seed for object-storage dependency: %#v", items)
	}
	raw, err := os.ReadFile(seedPath)
	if err != nil || string(raw) != "fixtures/cases/input.json:fixture-body" {
		t.Fatalf("generic target-path object seed = %q err=%v", raw, err)
	}
}

func TestEnvironmentRestoreRetriesObjectStorageSeedUntilServiceReady(t *testing.T) {
	workspace := t.TempDir()
	seedPath := filepath.Join(workspace, "seeded-object.txt")
	attemptsPath := filepath.Join(workspace, "seed-attempts.txt")
	graph := store.EnvironmentComponentGraph{
		Components: []store.EnvironmentComponent{
			{
				ComponentID:    "object-store",
				Kind:           "middleware",
				Role:           "object-storage",
				ComposeService: "minio",
				RuntimeJSON:    `{"objectStorage":{"seedCommand":["sh","-lc","attempts=0; if [ -f seed-attempts.txt ]; then attempts=$(cat seed-attempts.txt); fi; attempts=$((attempts + 1)); printf '%s\\n' \"$attempts\" > seed-attempts.txt; if [ \"$attempts\" -eq 1 ]; then exit 1; fi; cat > seeded-object.txt"]}}`,
			},
			{ComponentID: "worker", Kind: "app", Role: "worker", ComposeService: "worker"},
		},
		Dependencies: []store.ComponentDependency{
			{ConsumerComponentID: "worker", ProviderComponentID: "object-store", Capability: "object-storage", ProfileJSON: `{"assetIds":["object.fixture"]}`},
		},
		Assets: []store.ComponentConfigAsset{
			{OwnerComponentID: "worker", AssetID: "object.fixture", AssetKind: "object-storage-object", TargetComponentID: "object-store", ContentInline: "fixture-body", SummaryJSON: `{"bucket":"fixtures","key":"cases/input.json"}`},
		},
	}

	items := environmentRestoreApplyEdgeAssets(context.Background(), graph, nil, workspace, true, []string{"-f", "compose.yml"})
	if len(items) != 1 || !items[0].OK || items[0].Attempts != 2 {
		t.Fatalf("object storage seed should retry once then pass: %#v", items)
	}
	raw, err := os.ReadFile(seedPath)
	if err != nil || string(raw) != "fixture-body" {
		t.Fatalf("seeded object content = %q err=%v", raw, err)
	}
	attempts, err := os.ReadFile(attemptsPath)
	if err != nil || strings.TrimSpace(string(attempts)) != "2" {
		t.Fatalf("seed attempts = %q err=%v", attempts, err)
	}
}

func TestEnvironmentRestoreSeedsEmptyObjectStorageAsset(t *testing.T) {
	workspace := t.TempDir()
	seedPath := filepath.Join(workspace, "empty-object.txt")
	graph := store.EnvironmentComponentGraph{
		Components: []store.EnvironmentComponent{
			{
				ComponentID:    "object-store",
				Kind:           "middleware",
				Role:           "object-storage",
				ComposeService: "minio",
				RuntimeJSON:    `{"objectStorage":{"seedCommand":["sh","-lc","cat > empty-object.txt"]}}`,
			},
			{ComponentID: "worker", Kind: "app", Role: "worker", ComposeService: "worker"},
		},
		Dependencies: []store.ComponentDependency{
			{ConsumerComponentID: "worker", ProviderComponentID: "object-store", Capability: objectStorageCapability, ProfileJSON: `{"assetIds":["object.empty"]}`},
		},
		Assets: []store.ComponentConfigAsset{
			{OwnerComponentID: "worker", AssetID: "object.empty", AssetKind: "object-storage-object", TargetComponentID: "object-store", SizeBytes: 0, SummaryJSON: `{"bucket":"fixtures","key":"empty.marker","sizeBytes":0}`},
		},
	}

	items := environmentRestoreApplyEdgeAssets(context.Background(), graph, nil, workspace, true, []string{"-f", "compose.yml"})
	if len(items) != 1 || !items[0].OK || items[0].Bytes != 0 {
		t.Fatalf("empty object storage asset should seed successfully: %#v", items)
	}
	raw, err := os.ReadFile(seedPath)
	if err != nil || len(raw) != 0 {
		t.Fatalf("seeded empty object content length=%d err=%v", len(raw), err)
	}
}

func TestEnvironmentRestoreSeedsEmptyObjectStorageAssetFromTargetPath(t *testing.T) {
	workspace := t.TempDir()
	seedPath := filepath.Join(workspace, "empty-target-object.txt")
	graph := store.EnvironmentComponentGraph{
		Components: []store.EnvironmentComponent{
			{
				ComponentID:    "object-store",
				Kind:           "middleware",
				Role:           "object-storage",
				ComposeService: "minio",
				RuntimeJSON:    `{"objectStorage":{"seedCommand":["sh","-lc","cat > empty-target-object.txt"]}}`,
			},
			{ComponentID: "worker", Kind: "app", Role: "worker", ComposeService: "worker"},
		},
		Dependencies: []store.ComponentDependency{
			{ConsumerComponentID: "worker", ProviderComponentID: "object-store", Capability: objectStorageCapability, ProfileJSON: `{"assetIds":["object.empty.target"]}`},
		},
		Assets: []store.ComponentConfigAsset{
			{OwnerComponentID: "worker", AssetID: "object.empty.target", AssetKind: "object-storage-object", TargetComponentID: "object-store", TargetPath: "fixtures/empty.marker", SizeBytes: 0, SummaryJSON: `{"sizeBytes":0}`},
		},
	}

	items := environmentRestoreApplyEdgeAssets(context.Background(), graph, nil, workspace, true, []string{"-f", "compose.yml"})
	if len(items) != 1 || !items[0].OK || items[0].TargetPath != "fixtures/empty.marker" || items[0].Bytes != 0 {
		t.Fatalf("empty target-path object should seed successfully: %#v", items)
	}
	raw, err := os.ReadFile(seedPath)
	if err != nil || len(raw) != 0 {
		t.Fatalf("seeded target-path empty object content length=%d err=%v", len(raw), err)
	}
}

func TestEnvironmentRestoreEdgeAssetContentRejectsParentPath(t *testing.T) {
	item := environmentRestoreApplyEdgeAsset(context.Background(),
		store.ComponentDependency{ConsumerComponentID: "app", ProviderComponentID: "mysql", Capability: "sql", ProfileJSON: `{"assetIds":["bad.schema"]}`},
		store.ComponentConfigAsset{OwnerComponentID: "app", AssetID: "bad.schema", AssetKind: "mysql-ddl", TargetComponentID: "mysql", TargetPath: ".."},
		map[string]store.EnvironmentComponent{"mysql": {ComponentID: "mysql", ComposeService: "mysql"}},
		nil,
		t.TempDir(),
		false,
		[]string{"-f", "compose.yml"},
	)
	if item.OK || !strings.Contains(item.Error, "target path is required") {
		t.Fatalf("parent path edge asset should be rejected: %#v", item)
	}
}

func TestEnvironmentRestoreRetriesMySQLAssetUntilServiceReady(t *testing.T) {
	workspace := t.TempDir()
	command, callsPath := fakeMySQLApplyCommandWithFirstFailure(t)
	attempts, errText := runRestoreMySQLCommandWithInputRetry(context.Background(), workspace, command, "create database if not exists app;\n")
	if errText != "" {
		t.Fatalf("mysql retry command failed: %s", errText)
	}
	if attempts != 2 {
		t.Fatalf("mysql asset attempts = %d, want 2", attempts)
	}
	calls, err := os.ReadFile(callsPath)
	if err != nil {
		t.Fatalf("read mysql retry calls: %v", err)
	}
	if got := strings.Count(string(calls), "apply"); got != 2 {
		t.Fatalf("mysql command calls = %d, want 2\n%s", got, calls)
	}
}
