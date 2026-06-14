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

func TestEnvironmentRestoreAcceptsComposeManagedObjectStorageSeed(t *testing.T) {
	fixture := newEnvironmentRestoreDockerCLIFixture(t)
	for _, entry := range fixture.DockerEnv {
		key, value, ok := strings.Cut(entry, "=")
		if !ok {
			t.Fatalf("invalid fake docker env entry %q", entry)
		}
		t.Setenv(key, value)
	}
	fixture.writeDockerTool(t, `#!/usr/bin/env bash
printf '%s\n' "$*" >> "$DOCKER_CALLS_FILE"
if [ "$1" = "compose" ] && [ "$2" = "version" ]; then
  printf 'Docker Compose version v2.0.0\n'
  exit 0
fi
if [ "$1" = "manifest" ] && [ "$2" = "inspect" ]; then
  exit 0
fi
if [ "$1" = "compose" ] && [[ "$*" == *" ps -a --format json "* ]]; then
  service="${@: -1}"
  if [ "$service" = "object-seed" ]; then
    printf '[{"Name":"demo-object-seed","Service":"object-seed","State":"exited","ExitCode":0}]\n'
  else
    printf '{"Name":"demo-%s","Service":"%s","State":"running","Health":"healthy"}\n' "$service" "$service"
  fi
  exit 0
fi
exit 0
`)
	healthURL := newHealthyTestURL(t)
	report, err := buildEnvironmentRestoreReport(context.Background(), store.Environment{
		ID: "env.object.compose.seed",
		ComposeJSON: `{
			"composeFile":"compose.yml",
			"services":["object-seed","worker"],
			"generatedFiles":{
				"compose.yml":"services:\n  worker:\n    image: alpine:3.20\n    depends_on:\n      object-seed:\n        condition: service_completed_successfully\n  object-seed:\n    image: minio/mc:RELEASE.2024-05-09T17-04-24Z\n"
			},
			"skipPull":true,
			"skipBuild":true
		}`,
		HealthChecksJSON:       `[{"kind":"compose-service","service":"object-seed"}]`,
		VerificationWorkflowID: "workflow.object-compose-seed",
	}, fixture.Workspace, true, false, false, time.Second, environmentRestoreWorkflowOptions{}, environmentRestoreDockerCleanupOptions{}, store.EnvironmentComponentGraph{
		Components: []store.EnvironmentComponent{
			{ComponentID: "object-store", Kind: "middleware", Role: "object-storage", ComposeService: "object-seed", Required: true, HealthCheckJSON: `{"kind":"compose-service","service":"object-seed"}`},
			{ComponentID: "worker", Kind: "app", Role: "worker", ComposeService: "worker", Required: true, HealthCheckJSON: `{"kind":"url","url":"` + healthURL + `"}`},
		},
		Dependencies: []store.ComponentDependency{
			{ConsumerComponentID: "worker", ProviderComponentID: "object-store", Capability: objectStorageCapability, Required: true, ProfileJSON: `{"assetIds":["object.fixture"]}`},
		},
		Assets: []store.ComponentConfigAsset{
			{OwnerComponentID: "worker", AssetID: "object.fixture", AssetKind: "object-storage-object", TargetComponentID: "object-store", ContentInline: "fixture-body", SummaryJSON: `{"bucket":"fixtures","key":"cases/input.json"}`},
		},
	})
	if err != nil {
		t.Fatalf("build restore report: %v", err)
	}
	if !report.OK || !report.Docker.OK || len(report.Docker.AppliedAssets) != 1 {
		t.Fatalf("compose-managed object seed restore = %#v", report.Docker)
	}
	item := report.Docker.AppliedAssets[0]
	if item.Action != "object-storage-seed-satisfied-by-compose" || !item.OK || item.TargetComposeService != "object-seed" {
		t.Fatalf("compose-managed object seed asset = %#v", item)
	}
}

func TestEnvironmentRestoreUseExistingContainersRequiresObjectStorageSeedCommand(t *testing.T) {
	fixture := newEnvironmentRestoreDockerCLIFixture(t)
	for _, entry := range fixture.DockerEnv {
		key, value, ok := strings.Cut(entry, "=")
		if !ok {
			t.Fatalf("invalid fake docker env entry %q", entry)
		}
		t.Setenv(key, value)
	}
	fixture.writeDockerTool(t, `#!/usr/bin/env bash
printf '%s\n' "$*" >> "$DOCKER_CALLS_FILE"
if [ "$1" = "compose" ] && [ "$2" = "version" ]; then
  printf 'Docker Compose version v2.0.0\n'
  exit 0
fi
if [ "$1" = "inspect" ]; then
  printf 'running	healthy	0\n'
  exit 0
fi
exit 0
`)
	healthURL := newHealthyTestURL(t)
	report, err := buildEnvironmentRestoreReport(context.Background(), store.Environment{
		ID: "env.existing.object.seed",
		ComposeJSON: `{
			"composeFile":"compose.yml",
			"services":["object-seed","worker"],
			"generatedFiles":{
				"compose.yml":"services:\n  worker:\n    image: alpine:3.20\n    container_name: sandbox-worker\n    depends_on:\n      object-seed:\n        condition: service_completed_successfully\n  object-seed:\n    image: minio/mc:RELEASE.2024-05-09T17-04-24Z\n    container_name: sandbox-object-seed\n"
			},
			"skipPull":true,
			"skipBuild":true
		}`,
		HealthChecksJSON:       `[{"kind":"compose-service","service":"object-seed"}]`,
		VerificationWorkflowID: "workflow.existing-object-seed",
	}, fixture.Workspace, true, false, false, time.Second, environmentRestoreWorkflowOptions{}, environmentRestoreDockerCleanupOptions{
		UseExistingContainers: true,
	}, store.EnvironmentComponentGraph{
		Components: []store.EnvironmentComponent{
			{ComponentID: "object-store", Kind: "middleware", Role: "object-storage", ComposeService: "object-seed", Required: true, HealthCheckJSON: `{"kind":"compose-service","service":"object-seed"}`},
			{ComponentID: "worker", Kind: "app", Role: "worker", ComposeService: "worker", Required: true, HealthCheckJSON: `{"kind":"url","url":"` + healthURL + `"}`},
		},
		Dependencies: []store.ComponentDependency{
			{ConsumerComponentID: "worker", ProviderComponentID: "object-store", Capability: objectStorageCapability, Required: true, ProfileJSON: `{"assetIds":["object.fixture"]}`},
		},
		Assets: []store.ComponentConfigAsset{
			{OwnerComponentID: "worker", AssetID: "object.fixture", AssetKind: "object-storage-object", TargetComponentID: "object-store", ContentInline: "fixture-body", SummaryJSON: `{"bucket":"fixtures","key":"cases/input.json"}`},
		},
	})
	if err != nil {
		t.Fatalf("build restore report: %v", err)
	}
	if report.OK || report.Docker.OK || len(report.Docker.AppliedAssets) != 1 {
		t.Fatalf("existing-container object seed should require an explicit seed command: %#v", report.Docker)
	}
	item := report.Docker.AppliedAssets[0]
	if item.Action == "object-storage-seed-satisfied-by-compose" || !strings.Contains(item.Error, "objectStorage.seedCommand") {
		t.Fatalf("existing-container object seed asset = %#v", item)
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
