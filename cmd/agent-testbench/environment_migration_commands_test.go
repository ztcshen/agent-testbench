package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"agent-testbench/internal/store"
	"agent-testbench/internal/store/sqlite"
)

func TestEnvironmentMigrationAddAndListRegistersVersionedMySQLAsset(t *testing.T) {
	fixture := writeEnvironmentMigrationStoreFixture(t)
	sqlPath := filepath.Join(t.TempDir(), "V0011__add_score.sql")
	writeFile(t, sqlPath, "ALTER TABLE risk_result ADD COLUMN risk_score DECIMAL(10,2) NULL;\n")

	out := runCLI(t, "environment", "migration", "add", "env.migration",
		"--store", "sqlite://"+fixture.storePath,
		"--edge", "scf-risk:mysql",
		"--database", "scf_risk",
		"--version", "0011",
		"--description", "add risk score",
		"--precondition", "column-not-exists:risk_result.risk_score",
		"--file", sqlPath,
		"--json",
	)
	var addReport environmentMigrationReport
	if err := json.Unmarshal([]byte(out), &addReport); err != nil {
		t.Fatalf("decode migration add report: %v\n%s", err, out)
	}
	if !addReport.OK || addReport.Count != 1 || addReport.Migrations[0].Version != "0011" || addReport.Migrations[0].Checksum == "" {
		t.Fatalf("migration add report = %#v", addReport)
	}

	listOut := runCLI(t, "environment", "migration", cliCommandList, "env.migration",
		"--store", "sqlite://"+fixture.storePath,
		"--edge", "scf-risk:mysql",
		"--database", "scf_risk",
		"--json",
	)
	var listReport environmentMigrationReport
	if err := json.Unmarshal([]byte(listOut), &listReport); err != nil {
		t.Fatalf("decode migration list report: %v\n%s", err, listOut)
	}
	if !listReport.OK || listReport.Count != 1 || listReport.Migrations[0].Status != "registered" {
		t.Fatalf("migration list report = %#v", listReport)
	}

	s := openMigrationFixtureStore(t, fixture.storePath)
	defer s.Close()
	graph, err := s.GetEnvironmentComponentGraph(context.Background(), "env.migration")
	if err != nil {
		t.Fatalf("get graph: %v", err)
	}
	if len(graph.Assets) != 1 || graph.Assets[0].AssetKind != environmentMigrationAssetKind || !strings.Contains(graph.Dependencies[0].ProfileJSON, graph.Assets[0].AssetID) {
		t.Fatalf("graph after migration add = %#v", graph)
	}
}

func TestEnvironmentMigrationPlanAndApplyDryRunReportCommands(t *testing.T) {
	fixture := writeEnvironmentMigrationStoreFixture(t)
	seedEnvironmentMigrationAsset(t, fixture.storePath)

	out := runCLI(t, "environment", "migration", "apply", "env.migration",
		"--store", "sqlite://"+fixture.storePath,
		"--edge", "scf-risk:mysql",
		"--database", "scf_risk",
		"--workspace", fixture.workspace,
		"--json",
	)
	var report environmentMigrationReport
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatalf("decode migration apply dry-run report: %v\n%s", err, out)
	}
	if !report.OK || report.Execute || report.Count != 1 || report.Migrations[0].Action != environmentMigrationActionPlanApplyMySQL {
		t.Fatalf("migration dry-run report = %#v", report)
	}
	if got := strings.Join(report.Migrations[0].Command, " "); !strings.Contains(got, "compose") || !strings.Contains(got, "exec -T mysql") {
		t.Fatalf("migration dry-run command = %q", got)
	}
}

func TestEnvironmentMigrationApplySQLUsesHistoryChecksumAndPreconditions(t *testing.T) {
	item := environmentMigrationItem{
		EnvironmentID:    "env.migration",
		AssetID:          "scf-risk.mysql.migration.0011",
		OwnerComponentID: "scf-risk",
		Version:          "0011",
		Database:         "scf_risk",
		Checksum:         strings.Repeat("a", 64),
		Content:          "ALTER TABLE risk_result ADD COLUMN risk_score DECIMAL(10,2) NULL, ALGORITHM=INSTANT, LOCK=NONE;",
		Preconditions: []environmentMigrationPrecondition{{
			Type:   environmentMigrationPreconditionColumnNotExists,
			Table:  "risk_result",
			Column: "risk_score",
		}},
	}
	sql := environmentMigrationApplySQL(environmentMigrationEdge{Owner: "scf-risk", Provider: "mysql"}, item)
	ensureSQL := environmentMigrationEnsureSQL(item)
	preconditionSQL := environmentMigrationPreconditionQuerySQL(item, item.Preconditions[0])
	for _, want := range []string{"ALTER TABLE risk_result ADD COLUMN risk_score", "ALGORITHM=INSTANT", "LOCK=NONE"} {
		if !strings.Contains(sql, want) {
			t.Fatalf("migration apply sql missing %q:\n%s", want, sql)
		}
	}
	if !strings.Contains(ensureSQL, "CREATE TABLE IF NOT EXISTS agent_testbench_schema_history") {
		t.Fatalf("migration ensure sql missing history table:\n%s", ensureSQL)
	}
	if !strings.Contains(preconditionSQL, "information_schema.COLUMNS") || !strings.Contains(preconditionSQL, "risk_score") {
		t.Fatalf("migration precondition sql = %s", preconditionSQL)
	}
}

func TestEnvironmentMigrationThroughVersionUsesNumericOrder(t *testing.T) {
	if !environmentMigrationVersionAfter("0010", 10, "0002") {
		t.Fatalf("version 0010 should be after 0002")
	}
	if environmentMigrationVersionAfter("0002", 2, "0010") {
		t.Fatalf("version 0002 should not be after 0010")
	}
	if !environmentMigrationVersionAfter("B", 0, "A") {
		t.Fatalf("non-numeric versions should fall back to lexical order")
	}
}

func TestEnvironmentRestoreAppliesMySQLMigrationThroughHistoryTable(t *testing.T) {
	workspace := filepath.Join(t.TempDir(), "workspace")
	dockerEnv, dockerCallsPath, stdinPath := fakeDockerCommandCapturingExecStdin(t)
	healthURL := newHealthyTestURL(t)
	for _, kv := range dockerEnv {
		parts := strings.SplitN(kv, "=", 2)
		t.Setenv(parts[0], parts[1])
	}
	content := "ALTER TABLE risk_result ADD COLUMN risk_score DECIMAL(10,2) NULL;\n"
	report, err := buildEnvironmentRestoreReport(context.Background(), store.Environment{
		ID: "env.migration",
		ComposeJSON: `{
			"composeFile":"compose.yml",
			"generatedFiles":{"compose.yml":"services:\n  mysql:\n    image: mysql:8\n  scf-risk:\n    image: alpine:3.20\n"},
			"services":["mysql","scf-risk"],
			"skipPull":true,
			"skipBuild":true
		}`,
		HealthChecksJSON:       `[]`,
		VerificationWorkflowID: "workflow.migration",
	}, workspace, true, false, false, time.Second, environmentRestoreWorkflowOptions{}, environmentRestoreDockerCleanupOptions{}, store.EnvironmentComponentGraph{
		Components: []store.EnvironmentComponent{
			{ComponentID: "mysql", Kind: "middleware", Role: "database", ComposeService: "mysql", Required: true, HealthCheckJSON: `{"kind":"compose-service","service":"mysql"}`},
			{ComponentID: "scf-risk", Kind: "app", Role: "business-service", ComposeService: "scf-risk", Required: true, HealthCheckJSON: `{"kind":"url","url":"` + healthURL + `"}`},
		},
		Dependencies: []store.ComponentDependency{
			{EnvID: "env.migration", ConsumerComponentID: "scf-risk", ProviderComponentID: "mysql", Phase: "startup", Capability: "sql", Required: true, ProfileJSON: `{"assetIds":["scf-risk.mysql.migration.0011"]}`},
		},
		Assets: []store.ComponentConfigAsset{
			{
				EnvID:             "env.migration",
				OwnerComponentID:  "scf-risk",
				AssetID:           "scf-risk.mysql.migration.0011",
				AssetKind:         environmentMigrationAssetKind,
				TargetComponentID: "mysql",
				TargetPath:        "migrations/V0011__add_score.sql",
				ContentInline:     content,
				SHA256:            sha256Hex(content),
				ApplyOrder:        11,
				SummaryJSON: mustCompactJSON(environmentMigrationSummary{Migration: environmentMigrationMetadata{
					Version:     "0011",
					Description: "add risk score",
					Database:    "scf_risk",
					Checksum:    sha256Hex(content),
				}}),
			},
		},
	})
	if err != nil {
		t.Fatalf("build migration restore report: %v", err)
	}
	if !report.OK || len(report.Docker.AppliedAssets) != 1 || report.Docker.AppliedAssets[0].Action != "apply-mysql-migration" {
		t.Fatalf("migration restore report = %#v", report.Docker)
	}
	calls, err := os.ReadFile(dockerCallsPath)
	if err != nil {
		t.Fatalf("read docker calls: %v", err)
	}
	if !strings.Contains(string(calls), "exec -T mysql sh -lc") {
		t.Fatalf("docker calls should exec mysql:\n%s", calls)
	}
	stdin, err := os.ReadFile(stdinPath)
	if err != nil {
		t.Fatalf("read mysql stdin: %v", err)
	}
	if !strings.Contains(string(stdin), environmentMigrationHistoryTable) || !strings.Contains(string(stdin), "ALTER TABLE risk_result") {
		t.Fatalf("mysql migration stdin:\n%s", stdin)
	}
}

type environmentMigrationStoreFixture struct {
	storePath string
	workspace string
}

func writeEnvironmentMigrationStoreFixture(t *testing.T) environmentMigrationStoreFixture {
	t.Helper()
	dir := t.TempDir()
	storePath := filepath.Join(dir, "store.sqlite")
	workspace := filepath.Join(dir, "workspace")
	writeFile(t, filepath.Join(workspace, "compose.yml"), "services:\n  mysql:\n    image: mysql:8\n")
	ctx := context.Background()
	s, err := sqlite.Open(ctx, sqlite.Config{Path: storePath})
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer s.Close()
	if _, err := s.UpsertEnvironment(ctx, store.Environment{
		ID:                     "env.migration",
		DisplayName:            "Migration Environment",
		Status:                 "draft",
		ComposeJSON:            `{"composeFile":"compose.yml","services":["mysql","scf-risk"],"skipPull":true,"skipBuild":true}`,
		HealthChecksJSON:       `[]`,
		VerificationWorkflowID: "workflow.migration",
		SummaryJSON:            `{}`,
		CreatedAt:              time.Now().UTC(),
		UpdatedAt:              time.Now().UTC(),
	}); err != nil {
		t.Fatalf("seed environment: %v", err)
	}
	if err := s.ReplaceEnvironmentComponentGraph(ctx, "env.migration", store.EnvironmentComponentGraph{
		Components: []store.EnvironmentComponent{
			{ComponentID: "mysql", Kind: "middleware", Role: "database", ComposeService: "mysql", Required: true, HealthCheckJSON: `{"kind":"compose-service","service":"mysql"}`},
			{ComponentID: "scf-risk", Kind: "app", Role: "business-service", ComposeService: "scf-risk", Required: true, HealthCheckJSON: `{"kind":"url","url":"http://127.0.0.1:18080/health"}`},
		},
		Dependencies: []store.ComponentDependency{
			{ConsumerComponentID: "scf-risk", ProviderComponentID: "mysql", Phase: "startup", Capability: "sql", Required: true, ProfileJSON: `{}`},
		},
	}); err != nil {
		t.Fatalf("seed component graph: %v", err)
	}
	return environmentMigrationStoreFixture{storePath: storePath, workspace: workspace}
}

func seedEnvironmentMigrationAsset(t *testing.T, storePath string) {
	t.Helper()
	sqlPath := filepath.Join(t.TempDir(), "V0011__add_score.sql")
	writeFile(t, sqlPath, "ALTER TABLE risk_result ADD COLUMN risk_score DECIMAL(10,2) NULL;\n")
	runCLI(t, "environment", "migration", "add", "env.migration",
		"--store", "sqlite://"+storePath,
		"--edge", "scf-risk:mysql",
		"--database", "scf_risk",
		"--version", "0011",
		"--description", "add risk score",
		"--file", sqlPath,
		"--json",
	)
}

func openMigrationFixtureStore(t *testing.T, storePath string) *sqlite.Store {
	t.Helper()
	s, err := sqlite.Open(context.Background(), sqlite.Config{Path: storePath})
	if err != nil {
		t.Fatalf("open migration fixture store: %v", err)
	}
	return s
}

func fakeDockerCommandCapturingExecStdin(t *testing.T) ([]string, string, string) {
	t.Helper()
	dir := t.TempDir()
	callsPath := filepath.Join(dir, "docker-calls.txt")
	stdinPath := filepath.Join(dir, "mysql-stdin.sql")
	dockerPath := filepath.Join(dir, "docker")
	writeFile(t, dockerPath, `#!/usr/bin/env bash
set -euo pipefail
printf '%s\n' "$*" >> "$DOCKER_CALLS_FILE"
if [[ "$1" == "compose" && "$2" == "version" ]]; then
  printf 'Docker Compose version v2.0.0\n'
  exit 0
fi
if [[ "$*" == *" exec -T mysql "* ]]; then
  cat >> "$MYSQL_STDIN_FILE"
  printf '\n-- agent-testbench-call-boundary --\n' >> "$MYSQL_STDIN_FILE"
fi
if [[ "$*" == *" ps --format json "* ]]; then
  service="${@: -1}"
  printf '{"Name":"%s","Service":"%s","State":"running","Health":"healthy"}\n' "$service" "$service"
fi
`)
	if err := os.Chmod(dockerPath, 0o755); err != nil {
		t.Fatalf("chmod fake docker: %v", err)
	}
	return []string{
		"PATH=" + dir + string(os.PathListSeparator) + os.Getenv("PATH"),
		"DOCKER_CALLS_FILE=" + callsPath,
		"MYSQL_STDIN_FILE=" + stdinPath,
	}, callsPath, stdinPath
}
