package main

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"agent-testbench/internal/store"
	"agent-testbench/internal/store/sqlite"
)

type environmentComponentReadinessFixture struct {
	storePath string
	graphPath string
	envID     string
}

func writeEnvironmentComponentReadinessFixture(t *testing.T, envID string, includeAsset bool) environmentComponentReadinessFixture {
	t.Helper()

	storePath := filepath.Join(t.TempDir(), "store.sqlite")
	graphPath := filepath.Join(t.TempDir(), "graph.json")
	runCLI(t, "environment", "register",
		"--store", "sqlite://"+storePath,
		"--id", envID,
		"--start-command", "true",
		"--verification-workflow", "workflow.core-10",
	)
	graph := store.EnvironmentComponentGraph{
		Components: []store.EnvironmentComponent{
			{ComponentID: "db", Kind: "middleware", Role: "database", ComposeService: "db", Required: true, HealthCheckJSON: `{"type":"compose-service"}`, RuntimeJSON: `{}`, SummaryJSON: `{}`},
			{ComponentID: "app", Kind: "app", Role: "business-service", ComposeService: "app", Required: true, HealthCheckJSON: `{"type":"url","url":"http://127.0.0.1:18080/health"}`, RuntimeJSON: `{}`, SummaryJSON: `{}`},
		},
		Dependencies: []store.ComponentDependency{
			{ConsumerComponentID: "app", ProviderComponentID: "db", Phase: "startup", Capability: "sql", Required: true, ProfileJSON: `{}`},
		},
	}
	if includeAsset {
		graph.Assets = []store.ComponentConfigAsset{
			{OwnerComponentID: "app", AssetID: "app.schema", AssetKind: "mysql-ddl", TargetComponentID: "db", TargetPath: "compose/mysql/init/app.sql", ContentInline: "create database app;\n", ApplyOrder: 10, SummaryJSON: `{}`},
		}
	}
	writeFile(t, graphPath, mustJSON(t, graph))
	return environmentComponentReadinessFixture{storePath: storePath, graphPath: graphPath, envID: envID}
}

func TestEnvironmentComponentsReplaceRejectsBlockingDependencyCycle(t *testing.T) {
	storePath := filepath.Join(t.TempDir(), "store.sqlite")
	graphPath := filepath.Join(t.TempDir(), "graph.json")
	runCLI(t, "environment", "register",
		"--store", "sqlite://"+storePath,
		"--id", "env.component.replace-cycle",
		"--start-command", "true",
		"--verification-workflow", "workflow.core-10",
	)
	writeFile(t, graphPath, mustJSON(t, store.EnvironmentComponentGraph{
		Components: []store.EnvironmentComponent{
			{ComponentID: "app.a", Kind: "app", Role: "business-service", ComposeService: "app-a", Required: true, HealthCheckJSON: `{"type":"url","url":"http://127.0.0.1:18081/health"}`, RuntimeJSON: `{}`, SummaryJSON: `{}`},
			{ComponentID: "app.b", Kind: "app", Role: "business-service", ComposeService: "app-b", Required: true, HealthCheckJSON: `{"type":"url","url":"http://127.0.0.1:18082/health"}`, RuntimeJSON: `{}`, SummaryJSON: `{}`},
		},
		Dependencies: []store.ComponentDependency{
			{ConsumerComponentID: "app.a", ProviderComponentID: "app.b", Phase: "startup", Capability: "http", Required: true, ProfileJSON: `{}`},
			{ConsumerComponentID: "app.b", ProviderComponentID: "app.a", Phase: "startup", Capability: "http", Required: true, ProfileJSON: `{}`},
		},
	}))
	out := runCLIFails(t, "environment", "components", "replace", "--store", "sqlite://"+storePath, "--file", graphPath, "env.component.replace-cycle")
	if !strings.Contains(out, "component graph restore readiness failed") || !strings.Contains(out, "cycle") || !strings.Contains(out, "app.a") || !strings.Contains(out, "app.b") {
		t.Fatalf("replace cycle failure output = %q", out)
	}
}

func TestEnvironmentComponentsReplaceRejectsInvalidComponentHealthCheck(t *testing.T) {
	storePath := filepath.Join(t.TempDir(), "store.sqlite")
	graphPath := filepath.Join(t.TempDir(), "graph.json")
	runCLI(t, "environment", "register",
		"--store", "sqlite://"+storePath,
		"--id", "env.component.replace-health",
		"--start-command", "true",
		"--verification-workflow", "workflow.core-10",
	)
	writeFile(t, graphPath, mustJSON(t, store.EnvironmentComponentGraph{
		Components: []store.EnvironmentComponent{
			{ComponentID: "app", Kind: "app", Role: "business-service", Required: true, HealthCheckJSON: `{"kind":"url"}`, RuntimeJSON: `{}`, SummaryJSON: `{}`},
		},
	}))
	out := runCLIFails(t, "environment", "components", "replace", "--store", "sqlite://"+storePath, "--file", graphPath, "env.component.replace-health")
	if !strings.Contains(out, "component graph restore readiness failed") || !strings.Contains(out, "url health check requires url") {
		t.Fatalf("replace invalid health failure output = %q", out)
	}
}

func TestEnvironmentComponentsReplaceRejectsRemoteComponentAssetWithoutURLPath(t *testing.T) {
	storePath := filepath.Join(t.TempDir(), "store.sqlite")
	graphPath := filepath.Join(t.TempDir(), "graph.json")
	runCLI(t, "environment", "register",
		"--store", "sqlite://"+storePath,
		"--id", "env.component.replace-remote-asset",
		"--start-command", "true",
		"--verification-workflow", "workflow.core-10",
	)
	writeFile(t, graphPath, mustJSON(t, store.EnvironmentComponentGraph{
		Components: []store.EnvironmentComponent{
			{ComponentID: "app", Kind: "app", Role: "business-service", ComposeService: "app", Required: true, HealthCheckJSON: `{"type":"url","url":"http://127.0.0.1:18080/health"}`, RuntimeJSON: `{}`, SummaryJSON: `{}`},
		},
		Assets: []store.ComponentConfigAsset{
			{OwnerComponentID: "app", AssetID: "app.remote-ddl", AssetKind: "mysql-ddl", TargetPath: "compose/mysql/init/app.sql", RemoteRefJSON: `{"path":"compose/mysql/init/app.sql"}`, SizeBytes: 48 * 1024, SummaryJSON: `{}`},
		},
	}))
	out := runCLIFails(t, "environment", "components", "replace", "--store", "sqlite://"+storePath, "--file", graphPath, "env.component.replace-remote-asset")
	if !strings.Contains(out, "component graph restore readiness failed") || !strings.Contains(out, "remote Git URL/path") {
		t.Fatalf("replace invalid remote asset output = %q", out)
	}
}

func TestEnvironmentComponentsInspectReportsRestoreReadiness(t *testing.T) {
	fixture := writeEnvironmentComponentReadinessFixture(t, "env.component.inspect-readiness", true)
	replaceOut := runCLI(t, "environment", "components", "replace", "--store", "sqlite://"+fixture.storePath, "--file", fixture.graphPath, "--json", fixture.envID)
	inspectOut := runCLI(t, "environment", "components", "inspect", "--store", "sqlite://"+fixture.storePath, "--json", fixture.envID)
	documentedReplaceOut := runCLI(t, "environment", "components", "replace", fixture.envID, "--store", "sqlite://"+fixture.storePath, "--file", fixture.graphPath, "--json")
	documentedInspectOut := runCLI(t, "environment", "components", "inspect", fixture.envID, "--store", "sqlite://"+fixture.storePath, "--json")
	for _, out := range []string{replaceOut, inspectOut, documentedReplaceOut, documentedInspectOut} {
		var payload struct {
			ComponentGraph struct {
				RestoreReadiness struct {
					OK                   bool     `json:"ok"`
					BlockingDependencies int      `json:"blockingDependencies"`
					Assets               int      `json:"assets"`
					BlockingOrder        []string `json:"blockingOrder"`
				} `json:"restoreReadiness"`
			} `json:"componentGraph"`
		}
		if err := json.Unmarshal([]byte(out), &payload); err != nil {
			t.Fatalf("decode components readiness json: %v\n%s", err, out)
		}
		readiness := payload.ComponentGraph.RestoreReadiness
		if !readiness.OK || readiness.BlockingDependencies != 1 || readiness.Assets != 1 || strings.Join(readiness.BlockingOrder, ",") != "db,app" {
			t.Fatalf("components readiness payload = %#v\n%s", readiness, out)
		}
	}
}

func TestEnvironmentConfigureComponentsReplacesAndInspectsGraph(t *testing.T) {
	fixture := writeEnvironmentComponentReadinessFixture(t, "env.configure.components", true)
	replaceOut := runCLI(t, "environment", "configure",
		"--view", "components",
		"--store", "sqlite://"+fixture.storePath,
		"--file", fixture.graphPath,
		"--json",
		fixture.envID,
	)
	inspectOut := runCLI(t, "environment", "configure",
		"--view", "components",
		"--store", "sqlite://"+fixture.storePath,
		"--json",
		fixture.envID,
	)
	for _, out := range []string{replaceOut, inspectOut} {
		var payload struct {
			ComponentGraph struct {
				Counts struct {
					Components int `json:"components"`
					Assets     int `json:"assets"`
				} `json:"counts"`
				RestoreReadiness struct {
					OK            bool     `json:"ok"`
					BlockingOrder []string `json:"blockingOrder"`
				} `json:"restoreReadiness"`
			} `json:"componentGraph"`
		}
		if err := json.Unmarshal([]byte(out), &payload); err != nil {
			t.Fatalf("decode configure components json: %v\n%s", err, out)
		}
		if payload.ComponentGraph.Counts.Components != 2 || payload.ComponentGraph.Counts.Assets != 1 || !payload.ComponentGraph.RestoreReadiness.OK || strings.Join(payload.ComponentGraph.RestoreReadiness.BlockingOrder, ",") != "db,app" {
			t.Fatalf("configure components payload = %#v\n%s", payload.ComponentGraph, out)
		}
	}
}

func TestEnvironmentInspectReportsComponentGraphReadiness(t *testing.T) {
	fixture := writeEnvironmentComponentReadinessFixture(t, "env.inspect.component-readiness", false)
	runCLI(t, "environment", "components", "replace", "--store", "sqlite://"+fixture.storePath, "--file", fixture.graphPath, fixture.envID)
	out := runCLI(t, "environment", "inspect", "--store", "sqlite://"+fixture.storePath, "--json", fixture.envID)
	var payload struct {
		ComponentGraph struct {
			OK                   bool     `json:"ok"`
			Components           int      `json:"components"`
			BlockingDependencies int      `json:"blockingDependencies"`
			BlockingOrder        []string `json:"blockingOrder"`
		} `json:"componentGraph"`
	}
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		t.Fatalf("decode environment inspect component readiness json: %v\n%s", err, out)
	}
	if !payload.ComponentGraph.OK || payload.ComponentGraph.Components != 2 || payload.ComponentGraph.BlockingDependencies != 1 || strings.Join(payload.ComponentGraph.BlockingOrder, ",") != "db,app" {
		t.Fatalf("environment inspect component readiness = %#v", payload.ComponentGraph)
	}
}

func TestEnvironmentBootstrapReportsComponentGraphReadiness(t *testing.T) {
	storePath := seedEnvironmentBootstrapComponentReadiness(t)
	payload := runEnvironmentBootstrapComponentReadinessJSON(t, storePath)
	requireEnvironmentBootstrapComponentReadiness(t, payload)
}

type environmentBootstrapComponentReadinessPayload struct {
	Plan struct {
		ComponentGraph struct {
			OK                   bool     `json:"ok"`
			BlockingDependencies int      `json:"blockingDependencies"`
			BlockingOrder        []string `json:"blockingOrder"`
		} `json:"componentGraph"`
		ComponentStartupPlan struct {
			OK      bool `json:"ok"`
			Batches []struct {
				Components []struct {
					ComponentID string `json:"componentId"`
				} `json:"components"`
			} `json:"batches"`
			HealthGates []struct {
				ComponentID string `json:"componentId"`
			} `json:"healthGates"`
		} `json:"componentStartupPlan"`
		Restore struct {
			ComponentGraph struct {
				OK            bool     `json:"ok"`
				BlockingOrder []string `json:"blockingOrder"`
			} `json:"componentGraph"`
			ComponentStartupPlan struct {
				OK bool `json:"ok"`
			} `json:"componentStartupPlan"`
		} `json:"restore"`
	} `json:"plan"`
}

func seedEnvironmentBootstrapComponentReadiness(t *testing.T) string {
	t.Helper()

	fixture := writeEnvironmentComponentReadinessFixture(t, "env.component.bootstrap-readiness", false)
	runCLI(t, "environment", "components", "replace", "--store", "sqlite://"+fixture.storePath, "--file", fixture.graphPath, fixture.envID)
	return fixture.storePath
}

func runEnvironmentBootstrapComponentReadinessJSON(t *testing.T, storePath string) environmentBootstrapComponentReadinessPayload {
	t.Helper()

	out := runCLI(t, "environment", "bootstrap", "--store", "sqlite://"+storePath, "--json", "env.component.bootstrap-readiness")
	var payload environmentBootstrapComponentReadinessPayload
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		t.Fatalf("decode bootstrap component readiness json: %v\n%s", err, out)
	}
	return payload
}

func requireEnvironmentBootstrapComponentReadiness(t *testing.T, payload environmentBootstrapComponentReadinessPayload) {
	t.Helper()

	if !payload.Plan.ComponentGraph.OK || payload.Plan.ComponentGraph.BlockingDependencies != 1 || strings.Join(payload.Plan.ComponentGraph.BlockingOrder, ",") != "db,app" {
		t.Fatalf("bootstrap component graph readiness = %#v", payload.Plan.ComponentGraph)
	}
	if !payload.Plan.Restore.ComponentGraph.OK || strings.Join(payload.Plan.Restore.ComponentGraph.BlockingOrder, ",") != "db,app" {
		t.Fatalf("bootstrap restore component graph readiness = %#v", payload.Plan.Restore.ComponentGraph)
	}
	if !payload.Plan.ComponentStartupPlan.OK || len(payload.Plan.ComponentStartupPlan.Batches) != 2 || payload.Plan.ComponentStartupPlan.Batches[0].Components[0].ComponentID != "db" || payload.Plan.ComponentStartupPlan.Batches[1].Components[0].ComponentID != "app" || len(payload.Plan.ComponentStartupPlan.HealthGates) != 2 {
		t.Fatalf("bootstrap component startup plan = %#v", payload.Plan.ComponentStartupPlan)
	}
	if !payload.Plan.Restore.ComponentStartupPlan.OK {
		t.Fatalf("bootstrap restore component startup plan = %#v", payload.Plan.Restore.ComponentStartupPlan)
	}
}

func TestEnvironmentStartupFilePutMergesGeneratedFilesWithoutReRegistering(t *testing.T) {
	storePath := filepath.Join(t.TempDir(), "store.sqlite")
	sourceCompose := filepath.Join(t.TempDir(), "source-compose.yml")
	writeFile(t, sourceCompose, "services:\n  generated-service:\n    image: alpine:3.20\n")
	runCLI(t, "environment", "register",
		"--store", "sqlite://"+storePath,
		"--id", "env.startup.files",
		"--repo", "entry-gateway=https://example.com/team/entry-gateway.git",
		"--checkout", "entry-gateway=services/entry-gateway",
		"--compose-file", "compose/docker-compose.yml",
		"--health-url", newHealthyTestURL(t),
		"--verification-workflow", "workflow.core-10",
	)

	out := runCLI(t, "environment", "startup-file", "put",
		"--store", "sqlite://"+storePath,
		"--file", "compose/docker-compose.yml="+sourceCompose,
		"--json",
		"env.startup.files",
	)
	var payload struct {
		GeneratedFiles []struct {
			Path  string `json:"path"`
			Bytes int    `json:"bytes"`
		} `json:"generatedFiles"`
		Environment struct {
			Repos   map[string]any `json:"repos"`
			Compose struct {
				GeneratedFiles map[string]string `json:"generatedFiles"`
			} `json:"compose"`
			Summary struct {
				StartupFiles struct {
					Files []struct {
						Path string `json:"path"`
					} `json:"files"`
				} `json:"startupFiles"`
			} `json:"summary"`
		} `json:"environment"`
	}
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		t.Fatalf("decode startup-file put json: %v\n%s", err, out)
	}
	if len(payload.GeneratedFiles) != 1 || payload.GeneratedFiles[0].Path != "compose/docker-compose.yml" || payload.GeneratedFiles[0].Bytes == 0 {
		t.Fatalf("startup-file payload = %#v", payload.GeneratedFiles)
	}
	if payload.Environment.Repos["entry-gateway"] == nil {
		t.Fatalf("startup-file put should preserve existing repositories: %#v", payload.Environment.Repos)
	}
	if !strings.Contains(payload.Environment.Compose.GeneratedFiles["compose/docker-compose.yml"], "generated-service") {
		t.Fatalf("generated file was not stored in compose metadata: %#v", payload.Environment.Compose.GeneratedFiles)
	}
	if len(payload.Environment.Summary.StartupFiles.Files) != 1 || payload.Environment.Summary.StartupFiles.Files[0].Path != "compose/docker-compose.yml" {
		t.Fatalf("startup-file summary = %#v", payload.Environment.Summary.StartupFiles)
	}
}

func TestEnvironmentConfigureStartupFilesMergesGeneratedFiles(t *testing.T) {
	storePath := filepath.Join(t.TempDir(), "store.sqlite")
	sourceCompose := filepath.Join(t.TempDir(), "source-compose.yml")
	writeFile(t, sourceCompose, "services:\n  generated-service:\n    image: alpine:3.20\n")
	runCLI(t, "environment", "register",
		"--store", "sqlite://"+storePath,
		"--id", "env.configure.startup-files",
		"--compose-file", "compose/docker-compose.yml",
		"--verification-workflow", "workflow.core-10",
	)

	out := runCLI(t, "environment", "configure",
		"--view", "startup-files",
		"--store", "sqlite://"+storePath,
		"--file", "compose/docker-compose.yml="+sourceCompose,
		"--json",
		"env.configure.startup-files",
	)
	var payload struct {
		GeneratedFiles []struct {
			Path  string `json:"path"`
			Bytes int    `json:"bytes"`
		} `json:"generatedFiles"`
	}
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		t.Fatalf("decode configure startup-files json: %v\n%s", err, out)
	}
	if len(payload.GeneratedFiles) != 1 || payload.GeneratedFiles[0].Path != "compose/docker-compose.yml" || payload.GeneratedFiles[0].Bytes == 0 {
		t.Fatalf("configure startup-files payload = %#v", payload.GeneratedFiles)
	}
}

func TestEnvironmentConfigureStartupFilesRejectsRepoFlags(t *testing.T) {
	storePath := filepath.Join(t.TempDir(), "store.sqlite")
	sourceCompose := filepath.Join(t.TempDir(), "source-compose.yml")
	writeFile(t, sourceCompose, "services: {}\n")
	runCLI(t, "environment", "register",
		"--store", "sqlite://"+storePath,
		"--id", "env.configure.startup-file-repo-flag",
		"--compose-file", "compose/docker-compose.yml",
		"--verification-workflow", "workflow.core-10",
	)

	out := runCLIFails(t, "environment", "configure",
		"--view", "startup-files",
		"--store", "sqlite://"+storePath,
		"--file", "compose/docker-compose.yml="+sourceCompose,
		"--repo-ref", "app=v1",
		"env.configure.startup-file-repo-flag",
	)
	if !strings.Contains(out, "only supported for --view repos") {
		t.Fatalf("startup-files should reject repo flags, got: %q", out)
	}
}

func TestEnvironmentConfigureStartupFilesRequiresFile(t *testing.T) {
	storePath := filepath.Join(t.TempDir(), "store.sqlite")
	runCLI(t, "environment", "register",
		"--store", "sqlite://"+storePath,
		"--id", "env.configure.startup-file-required",
		"--compose-file", "compose/docker-compose.yml",
		"--verification-workflow", "workflow.core-10",
	)

	out := runCLIFails(t, "environment", "configure",
		"--view", "startup-files",
		"--store", "sqlite://"+storePath,
		"env.configure.startup-file-required",
	)
	if !strings.Contains(out, "--file TARGET=SOURCE_FILE is required") {
		t.Fatalf("startup-files should require --file, got: %q", out)
	}
}

func TestEnvironmentStartupFilePutPreservesMixedLegacyGeneratedFiles(t *testing.T) {
	storePath := filepath.Join(t.TempDir(), "store.sqlite")
	storeRef := "sqlite://" + storePath
	sourceCompose := filepath.Join(t.TempDir(), "source-compose.yml")
	sourceEnv := filepath.Join(t.TempDir(), "runtime.env")
	writeFile(t, sourceCompose, "services:\n  app:\n    image: alpine:3.20\n")
	writeFile(t, sourceEnv, "APP_MODE=test\n")
	runCLI(t, "environment", "register",
		"--store", storeRef,
		"--id", "env.mixed.startup-files",
		"--compose-file", "compose/docker-compose.yml",
		"--compose-generated-file", "compose/docker-compose.yml="+sourceCompose,
		"--verification-workflow", "workflow.core-10",
	)
	seedLegacyEnvironmentGeneratedFiles(t, storePath)

	out := runCLI(t, "environment", "startup-file", "put",
		"--store", storeRef,
		"--file", "compose/runtime.env="+sourceEnv,
		"--json",
		"env.mixed.startup-files",
	)
	var payload struct {
		Environment struct {
			Compose struct {
				GeneratedFiles map[string]string `json:"generatedFiles"`
			} `json:"compose"`
		} `json:"environment"`
	}
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		t.Fatalf("decode startup-file put json: %v\n%s", err, out)
	}
	requireMixedStartupFileProjection(t, payload.Environment.Compose.GeneratedFiles)
	requireMixedStartupFileRows(t, storePath)
	if rawComposeJSON := sqliteScalar(t, storePath, `select compose_json from environments where id = 'env.mixed.startup-files';`); strings.Contains(rawComposeJSON, "generatedFiles") {
		t.Fatalf("raw compose_json should migrate mixed generated files into structured rows: %s", rawComposeJSON)
	}
}

func seedLegacyEnvironmentGeneratedFiles(t *testing.T, storePath string) {
	t.Helper()
	ctx := context.Background()
	runtime, err := sqlite.Open(ctx, sqlite.Config{Path: storePath})
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer runtime.Close()
	env, err := runtime.GetEnvironment(ctx, "env.mixed.startup-files")
	if err != nil {
		t.Fatalf("get environment: %v", err)
	}
	env.ComposeJSON = `{"composeFile":"compose/docker-compose.yml","composeFiles":["compose/docker-compose.yml"],"generatedFiles":{"compose/legacy.env":"LEGACY_MODE=true\n"}}`
	if _, err := runtime.UpsertEnvironment(ctx, env); err != nil {
		t.Fatalf("seed legacy generated files: %v", err)
	}
}

func requireMixedStartupFileProjection(t *testing.T, generated map[string]string) {
	t.Helper()
	if !strings.Contains(generated["compose/docker-compose.yml"], "app:") {
		t.Fatalf("structured compose file should be preserved: %#v", generated)
	}
	if generated["compose/legacy.env"] != "LEGACY_MODE=true\n" {
		t.Fatalf("legacy generated file should be preserved: %#v", generated)
	}
	if generated["compose/runtime.env"] != "APP_MODE=test\n" {
		t.Fatalf("new generated file should be stored: %#v", generated)
	}
}

func requireMixedStartupFileRows(t *testing.T, storePath string) {
	t.Helper()
	ctx := context.Background()
	runtime, err := sqlite.Open(ctx, sqlite.Config{Path: storePath})
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer runtime.Close()
	files, err := runtime.ListEnvironmentFiles(ctx, "env.mixed.startup-files")
	if err != nil {
		t.Fatalf("list environment files: %v", err)
	}
	if len(files) != 3 {
		t.Fatalf("startup-file put should migrate mixed generated files into structured rows: %#v", files)
	}
}

func TestEnvironmentInspectAndBootstrapExposeFileProjectionGaps(t *testing.T) {
	storePath := filepath.Join(t.TempDir(), "store.sqlite")
	sourceCompose := filepath.Join(t.TempDir(), "source-compose.yml")
	sourceEnv := filepath.Join(t.TempDir(), "runtime.env")
	writeFile(t, sourceCompose, "services:\n  web:\n    image: alpine:3.20\n")
	writeFile(t, sourceEnv, "APP_MODE=test\n")
	runCLI(t, "environment", "register",
		"--store", "sqlite://"+storePath,
		"--id", "env.file.projection",
		"--compose-file", "compose/docker-compose.yml",
		"--compose-generated-file", "compose/docker-compose.yml="+sourceCompose,
		"--compose-env-file", "compose/runtime.env",
		"--verification-workflow", "workflow.core-10",
	)

	inspectOut := runCLI(t, "environment", "inspect", "--store", "sqlite://"+storePath, "--json", "env.file.projection")
	var inspectPayload struct {
		FileProjection struct {
			OK      bool `json:"ok"`
			Missing []struct {
				Path   string `json:"path"`
				Kind   string `json:"kind"`
				Source string `json:"source"`
			} `json:"missing"`
			RepairPlan []struct {
				Name          string   `json:"name"`
				Target        string   `json:"target"`
				Missing       []string `json:"missing"`
				BlocksRestore bool     `json:"blocksRestore"`
			} `json:"repairPlan"`
		} `json:"fileProjection"`
	}
	if err := json.Unmarshal([]byte(inspectOut), &inspectPayload); err != nil {
		t.Fatalf("decode inspect file projection: %v\n%s", err, inspectOut)
	}
	if inspectPayload.FileProjection.OK || len(inspectPayload.FileProjection.Missing) != 1 || inspectPayload.FileProjection.Missing[0].Path != "compose/runtime.env" || inspectPayload.FileProjection.Missing[0].Kind != "env-file" {
		t.Fatalf("inspect file projection should expose missing env file: %#v", inspectPayload.FileProjection)
	}
	if len(inspectPayload.FileProjection.RepairPlan) != 1 ||
		inspectPayload.FileProjection.RepairPlan[0].Name != "compose-file-projection" ||
		inspectPayload.FileProjection.RepairPlan[0].Target != "fileProjection.missing" ||
		!stringSliceContains(inspectPayload.FileProjection.RepairPlan[0].Missing, "env-file:compose/runtime.env") ||
		!inspectPayload.FileProjection.RepairPlan[0].BlocksRestore {
		t.Fatalf("inspect file projection repair plan = %#v", inspectPayload.FileProjection.RepairPlan)
	}

	bootstrapOut := runCLI(t, "environment", "bootstrap", "--store", "sqlite://"+storePath, "--json", "env.file.projection")
	var bootstrapPayload struct {
		Plan struct {
			FileProjection struct {
				OK         bool  `json:"ok"`
				RepairPlan []any `json:"repairPlan"`
			} `json:"fileProjection"`
			Restore struct {
				FileProjection struct {
					OK         bool  `json:"ok"`
					RepairPlan []any `json:"repairPlan"`
				} `json:"fileProjection"`
			} `json:"restore"`
		} `json:"plan"`
	}
	if err := json.Unmarshal([]byte(bootstrapOut), &bootstrapPayload); err != nil {
		t.Fatalf("decode bootstrap file projection: %v\n%s", err, bootstrapOut)
	}
	if bootstrapPayload.Plan.FileProjection.OK || bootstrapPayload.Plan.Restore.FileProjection.OK {
		t.Fatalf("bootstrap should carry missing file projection: %#v", bootstrapPayload.Plan)
	}
	if len(bootstrapPayload.Plan.FileProjection.RepairPlan) != 1 || len(bootstrapPayload.Plan.Restore.FileProjection.RepairPlan) != 1 {
		t.Fatalf("bootstrap should carry file projection repair plans: %#v", bootstrapPayload.Plan)
	}

	runCLI(t, "environment", "startup-file", "put",
		"--store", "sqlite://"+storePath,
		"--file", "compose/runtime.env="+sourceEnv,
		"--json",
		"env.file.projection",
	)
	repairedOut := runCLI(t, "environment", "inspect", "--store", "sqlite://"+storePath, "--json", "env.file.projection")
	var repairedPayload struct {
		FileProjection struct {
			OK      bool  `json:"ok"`
			Missing []any `json:"missing"`
		} `json:"fileProjection"`
	}
	if err := json.Unmarshal([]byte(repairedOut), &repairedPayload); err != nil {
		t.Fatalf("decode repaired inspect file projection: %v\n%s", err, repairedOut)
	}
	if !repairedPayload.FileProjection.OK || len(repairedPayload.FileProjection.Missing) != 0 {
		t.Fatalf("repaired file projection should pass: %#v", repairedPayload.FileProjection)
	}
}
