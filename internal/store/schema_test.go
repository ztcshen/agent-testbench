package store_test

import (
	"context"
	"encoding/json"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"agent-testbench/internal/store/schema"
	"agent-testbench/internal/store/sqlite"
	"agent-testbench/internal/store/sqlstore"
)

func TestLegacySQLiteSchemaChangesAreContiguous(t *testing.T) {
	changes := schema.All()
	if len(changes) != schema.CurrentVersion {
		t.Fatalf("schema.All() length = %d, want CurrentVersion %d", len(changes), schema.CurrentVersion)
	}
	for i, change := range changes {
		wantVersion := i + 1
		if change.Version != wantVersion {
			t.Fatalf("schema change at index %d has version %d, want %d", i, change.Version, wantVersion)
		}
		if strings.TrimSpace(change.Name) == "" {
			t.Fatalf("schema change %d has empty name", change.Version)
		}
		if strings.TrimSpace(change.SQL) == "" {
			t.Fatalf("schema change %d has empty SQL", change.Version)
		}
	}
}

func TestSQLiteSchemaUpgradesAreIdempotent(t *testing.T) {
	ctx := context.Background()
	cfg := sqlite.Config{Path: filepath.Join(t.TempDir(), "store.sqlite")}

	status, err := sqlite.SchemaStatus(ctx, cfg)
	if err != nil {
		t.Fatalf("initial schema upgrade status: %v", err)
	}
	if status.CurrentVersion != 0 || status.TargetVersion != sqlstore.CurrentSchemaVersion || !status.HasPending() {
		t.Fatalf("initial status = %#v", status)
	}

	first, err := sqlite.UpgradeSchema(ctx, cfg)
	if err != nil {
		t.Fatalf("first upgrade: %v", err)
	}
	if first.CurrentVersion != sqlstore.CurrentSchemaVersion || first.AppliedCount != 1 || first.HasPending() {
		t.Fatalf("first upgraded status = %#v", first)
	}

	second, err := sqlite.UpgradeSchema(ctx, cfg)
	if err != nil {
		t.Fatalf("second upgrade: %v", err)
	}
	if second.CurrentVersion != sqlstore.CurrentSchemaVersion || second.AppliedCount != 0 || second.HasPending() {
		t.Fatalf("second upgraded status = %#v", second)
	}
}

func TestSQLiteFreshSchemaOmitsLegacyTemplateConfigModel(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "store.sqlite")
	if _, err := sqlite.UpgradeSchema(ctx, sqlite.Config{Path: dbPath}); err != nil {
		t.Fatalf("upgrade schema: %v", err)
	}

	tables := sqliteTableNames(t, dbPath)
	for _, table := range []string{
		"template",
		"template_config",
		"node_config",
		"workflow",
		"interface_node",
		"interface_node_field",
		"interface_node_request_template",
		"interface_node_case",
		"workflow_interface_node",
		"fixture_profile",
		"fixture_table_binding",
		"interface_node_case_dependency",
	} {
		if tables[table] {
			t.Fatalf("fresh sqlite schema should not include legacy template config table %q in %#v", table, tables)
		}
	}
}

func TestSQLiteSchemaIncludesEvidenceStepRelation(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "store.sqlite")
	if _, err := sqlite.UpgradeSchema(ctx, sqlite.Config{Path: dbPath}); err != nil {
		t.Fatalf("upgrade schema: %v", err)
	}

	columns := sqliteTableColumns(t, dbPath, "evidence_records")
	if !columns["step_id"] {
		t.Fatalf("missing evidence_records.step_id in %#v", columns)
	}
}

func TestSQLiteSchemaIncludesEnvironmentComponentAssets(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "store.sqlite")
	if _, err := sqlite.UpgradeSchema(ctx, sqlite.Config{Path: dbPath}); err != nil {
		t.Fatalf("upgrade schema: %v", err)
	}

	tables := sqliteTableNames(t, dbPath)
	for _, table := range []string{
		"environment_components",
		"component_dependencies",
		"component_config_assets",
		"environment_files",
		"environment_services",
		"environment_health_checks",
	} {
		if !tables[table] {
			t.Fatalf("missing environment component asset table %q in %#v", table, tables)
		}
	}
	for _, table := range []string{"service_dependencies", "service_config_assets"} {
		if tables[table] {
			t.Fatalf("fresh sqlite schema should not include legacy service graph table %q in %#v", table, tables)
		}
	}
	dependencyColumns := sqliteTableColumns(t, dbPath, "component_dependencies")
	for _, column := range []string{
		"consumer_component_id",
		"provider_component_id",
		"phase",
		"capability",
		"profile_json",
	} {
		if !dependencyColumns[column] {
			t.Fatalf("missing component_dependencies.%s in %#v", column, dependencyColumns)
		}
	}
	componentAssetColumns := sqliteTableColumns(t, dbPath, "component_config_assets")
	for _, column := range []string{
		"owner_component_id",
		"asset_kind",
		"target_component_id",
		"content_inline",
		"remote_ref_json",
		"size_bytes",
		"apply_order",
		"sensitive",
	} {
		if !componentAssetColumns[column] {
			t.Fatalf("missing component_config_assets.%s in %#v", column, componentAssetColumns)
		}
	}
	fileColumns := sqliteTableColumns(t, dbPath, "environment_files")
	for _, column := range []string{
		"file_path",
		"file_kind",
		"content_inline",
		"required",
		"apply_order",
		"summary_json",
	} {
		if !fileColumns[column] {
			t.Fatalf("missing environment_files.%s in %#v", column, fileColumns)
		}
	}
	serviceColumns := sqliteTableColumns(t, dbPath, "environment_services")
	for _, column := range []string{"service_id", "repo_url", "branch", "ref", "checkout", "summary_json"} {
		if !serviceColumns[column] {
			t.Fatalf("missing environment_services.%s in %#v", column, serviceColumns)
		}
	}
	healthColumns := sqliteTableColumns(t, dbPath, "environment_health_checks")
	for _, column := range []string{"check_id", "check_kind", "url", "address", "command", "compose_service", "expect", "apply_order", "summary_json"} {
		if !healthColumns[column] {
			t.Fatalf("missing environment_health_checks.%s in %#v", column, healthColumns)
		}
	}
}

func TestSQLiteFreshSchemaUsesSharedCoreTableSet(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "store.sqlite")
	if _, err := sqlite.UpgradeSchema(ctx, sqlite.Config{Path: dbPath}); err != nil {
		t.Fatalf("upgrade schema: %v", err)
	}

	actual := sqliteTableNames(t, dbPath)
	expected := sharedCoreTableNames(t)
	if len(actual) != len(expected) {
		t.Fatalf("sqlite table count = %d, want shared core count %d\nextra=%v\nmissing=%v", len(actual), len(expected), tableNameDiff(actual, expected), tableNameDiff(expected, actual))
	}
	for table := range expected {
		if !actual[table] {
			t.Fatalf("sqlite fresh schema missing shared core table %q\nextra=%v", table, tableNameDiff(actual, expected))
		}
	}
}

func TestSQLiteFreshSchemaColumnsAndIndexesMatchSharedCoreSchema(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "store.sqlite")
	if _, err := sqlite.UpgradeSchema(ctx, sqlite.Config{Path: dbPath}); err != nil {
		t.Fatalf("upgrade schema: %v", err)
	}

	assertSQLiteCoreSchemaShape(t, dbPath)
}

func TestSQLiteLegacySchemaMigratesToSharedCoreTableSet(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "legacy.sqlite")
	createLegacySQLiteSchema(t, dbPath)

	before := sqliteTableNames(t, dbPath)
	if !before["template_config"] || !before["service_dependencies"] {
		t.Fatalf("legacy fixture missing legacy tables: %#v", before)
	}
	seedLegacyEnvironmentGraph(t, dbPath)

	status, err := sqlite.UpgradeSchema(ctx, sqlite.Config{Path: dbPath})
	if err != nil {
		t.Fatalf("upgrade legacy sqlite schema: %v", err)
	}
	if status.CurrentVersion != sqlstore.CurrentSchemaVersion || status.HasPending() {
		t.Fatalf("legacy upgraded status = %#v", status)
	}

	actual := sqliteTableNames(t, dbPath)
	expected := sharedCoreTableNames(t)
	if len(actual) != len(expected) {
		t.Fatalf("legacy sqlite table count = %d, want shared core count %d\nextra=%v\nmissing=%v", len(actual), len(expected), tableNameDiff(actual, expected), tableNameDiff(expected, actual))
	}
	for table := range expected {
		if !actual[table] {
			t.Fatalf("legacy sqlite schema missing shared core table %q\nextra=%v", table, tableNameDiff(actual, expected))
		}
	}
	assertSQLiteCoreSchemaShape(t, dbPath)
	if got := sqliteCount(t, dbPath, "component_dependencies"); got != 1 {
		t.Fatalf("migrated component dependency count = %d, want 1", got)
	}
	if got := sqliteCount(t, dbPath, "component_config_assets"); got != 1 {
		t.Fatalf("migrated component config asset count = %d, want 1", got)
	}
}

func createLegacySQLiteSchema(t *testing.T, dbPath string) {
	t.Helper()
	createVersionTable := `
create table if not exists schema_versions (
  version integer primary key,
  name text not null,
  applied_at text not null
);`
	sqliteExec(t, dbPath, createVersionTable)
	for _, change := range schema.All() {
		sqliteExec(t, dbPath, change.SQL)
		sqliteExec(t, dbPath, `insert into schema_versions (version, name, applied_at) values (`+sqliteInt(change.Version)+`, `+sqliteQuote(change.Name)+`, '2026-01-01T00:00:00Z');`)
	}
}

func seedLegacyEnvironmentGraph(t *testing.T, dbPath string) {
	t.Helper()
	sqliteExec(t, dbPath, `
insert into environments (
  id, display_name, description, status, verified, services_json, repos_json, compose_json,
  health_checks_json, verification_workflow_id, last_verification_run_id, last_verification_status,
  evidence_complete, topology_complete, summary_json, created_at, updated_at
) values (
  'env.legacy', 'Legacy', '', 'draft', 0, '[]', '{}', '{}',
  '[]', 'workflow.smoke', '', '', 0, 0, '{}', '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z'
);`)
	sqliteExec(t, dbPath, `
insert into environment_components (
  env_id, component_id, display_name, kind, role, compose_service, image, required,
  runtime_json, healthcheck_json, summary_json, created_at, updated_at
) values
  ('env.legacy', 'app', 'App', 'service', 'app', 'app', 'alpine:3.20', 1, '{}', '{}', '{}', '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z'),
  ('env.legacy', 'mysql', 'MySQL', 'database', 'mysql', 'mysql', 'mysql:8', 1, '{}', '{}', '{}', '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z');
`)
	sqliteExec(t, dbPath, `
insert into service_dependencies (
  env_id, service_id, dependency_component_id, dependency_kind, required, profile_json, created_at, updated_at
) values (
  'env.legacy', 'app', 'mysql', 'sql', 1, '{"assetIds":["schema"]}', '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z'
);`)
	sqliteExec(t, dbPath, `
insert into service_config_assets (
  env_id, service_id, asset_id, asset_kind, target_component_id, target_path, content_inline,
  remote_ref_json, sha256, size_bytes, apply_order, sensitive, summary_json, created_at, updated_at
) values (
  'env.legacy', 'app', 'schema', 'mysql-ddl', 'mysql', 'mysql/init/schema.sql', 'create database app;',
  '{}', 'abc123', 20, 1, 0, '{}', '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z'
);`)
}

func sqliteTableNames(t *testing.T, dbPath string) map[string]bool {
	t.Helper()
	out, err := exec.Command("sqlite3", "-json", dbPath, `select name from sqlite_master where type = 'table';`).CombinedOutput()
	if err != nil {
		t.Fatalf("list sqlite tables: %v: %s", err, out)
	}
	var rows []struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(out, &rows); err != nil {
		t.Fatalf("decode sqlite tables: %v: %s", err, out)
	}
	tables := map[string]bool{}
	for _, row := range rows {
		tables[row.Name] = true
	}
	return tables
}

func sqliteCount(t *testing.T, dbPath string, table string) int {
	t.Helper()
	out, err := exec.Command("sqlite3", "-json", dbPath, `select count(*) as count from `+table+`;`).CombinedOutput()
	if err != nil {
		t.Fatalf("count sqlite table %s: %v: %s", table, err, out)
	}
	var rows []struct {
		Count int `json:"count"`
	}
	if err := json.Unmarshal(out, &rows); err != nil {
		t.Fatalf("decode sqlite count for %s: %v: %s", table, err, out)
	}
	if len(rows) != 1 {
		t.Fatalf("sqlite count rows for %s = %#v", table, rows)
	}
	return rows[0].Count
}

func sqliteTableColumns(t *testing.T, dbPath string, table string) map[string]bool {
	t.Helper()
	out, err := exec.Command("sqlite3", "-json", dbPath, `pragma table_info(`+table+`);`).CombinedOutput()
	if err != nil {
		t.Fatalf("list sqlite columns: %v: %s", err, out)
	}
	var rows []struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(out, &rows); err != nil {
		t.Fatalf("decode sqlite columns: %v: %s", err, out)
	}
	columns := map[string]bool{}
	for _, row := range rows {
		columns[row.Name] = true
	}
	return columns
}

func sqliteIndexNames(t *testing.T, dbPath string) map[string]bool {
	t.Helper()
	out, err := exec.Command("sqlite3", "-json", dbPath, `select name from sqlite_master where type = 'index' and name not like 'sqlite_autoindex%';`).CombinedOutput()
	if err != nil {
		t.Fatalf("list sqlite indexes: %v: %s", err, out)
	}
	var rows []struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(out, &rows); err != nil {
		t.Fatalf("decode sqlite indexes: %v: %s", err, out)
	}
	indexes := map[string]bool{}
	for _, row := range rows {
		indexes[row.Name] = true
	}
	return indexes
}

func sqliteExec(t *testing.T, dbPath string, statement string) {
	t.Helper()
	if out, err := exec.Command("sqlite3", dbPath, statement).CombinedOutput(); err != nil {
		t.Fatalf("exec sqlite: %v: %s\n%s", err, out, statement)
	}
}

func sqliteQuote(value string) string {
	return `'` + regexp.MustCompile(`'`).ReplaceAllString(value, `''`) + `'`
}

func sqliteInt(value int) string {
	return strconv.Itoa(value)
}

func sharedCoreTableNames(t *testing.T) map[string]bool {
	t.Helper()
	re := regexp.MustCompile(`(?is)create\s+table\s+if\s+not\s+exists\s+["` + "`" + `]?([a-zA-Z0-9_]+)["` + "`" + `]?`)
	tables := map[string]bool{}
	for _, statement := range sqlstore.CoreSchemaSQL(sqlstore.SQLiteDialect{}) {
		match := re.FindStringSubmatch(statement)
		if len(match) == 2 {
			tables[match[1]] = true
		}
	}
	return tables
}

func assertSQLiteCoreSchemaShape(t *testing.T, dbPath string) {
	t.Helper()
	for table, expected := range sharedCoreColumns(t) {
		actual := sqliteTableColumns(t, dbPath, table)
		if len(actual) != len(expected) {
			t.Fatalf("sqlite table %s column count = %d, want %d\nextra=%v\nmissing=%v", table, len(actual), len(expected), tableNameDiff(actual, expected), tableNameDiff(expected, actual))
		}
		for column := range expected {
			if !actual[column] {
				t.Fatalf("sqlite table %s missing shared core column %q\nextra=%v", table, column, tableNameDiff(actual, expected))
			}
		}
	}
	actualIndexes := sqliteIndexNames(t, dbPath)
	expectedIndexes := sharedCoreIndexNames(t)
	if len(actualIndexes) != len(expectedIndexes) {
		t.Fatalf("sqlite index count = %d, want shared core count %d\nextra=%v\nmissing=%v", len(actualIndexes), len(expectedIndexes), tableNameDiff(actualIndexes, expectedIndexes), tableNameDiff(expectedIndexes, actualIndexes))
	}
	for index := range expectedIndexes {
		if !actualIndexes[index] {
			t.Fatalf("sqlite schema missing shared core index %q\nextra=%v", index, tableNameDiff(actualIndexes, expectedIndexes))
		}
	}
}

func sharedCoreColumns(t *testing.T) map[string]map[string]bool {
	t.Helper()
	statements := strings.Join(sqlstore.CoreSchemaSQL(sqlstore.SQLiteDialect{}), "\n")
	tableRe := regexp.MustCompile(`(?is)create\s+table\s+if\s+not\s+exists\s+["` + "`" + `]?([a-zA-Z0-9_]+)["` + "`" + `]?\s*\((.*?)\);`)
	columnRe := regexp.MustCompile(`(?m)^\s+["` + "`" + `]?([a-zA-Z_][a-zA-Z0-9_]*)["` + "`" + `]?\s+`)
	out := map[string]map[string]bool{}
	for _, match := range tableRe.FindAllStringSubmatch(statements, -1) {
		table := match[1]
		columns := map[string]bool{}
		for _, line := range strings.Split(match[2], "\n") {
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(strings.ToLower(trimmed), "primary key") || strings.HasPrefix(strings.ToLower(trimmed), "foreign key") {
				continue
			}
			columnMatch := columnRe.FindStringSubmatch(line)
			if len(columnMatch) == 2 {
				columns[columnMatch[1]] = true
			}
		}
		out[table] = columns
	}
	return out
}

func sharedCoreIndexNames(t *testing.T) map[string]bool {
	t.Helper()
	re := regexp.MustCompile(`(?is)create\s+index(?:\s+if\s+not\s+exists)?\s+["` + "`" + `]?([a-zA-Z0-9_]+)["` + "`" + `]?`)
	indexes := map[string]bool{}
	for _, statement := range sqlstore.CoreSchemaSQL(sqlstore.SQLiteDialect{}) {
		match := re.FindStringSubmatch(statement)
		if len(match) == 2 {
			indexes[match[1]] = true
		}
	}
	return indexes
}

func tableNameDiff(left map[string]bool, right map[string]bool) []string {
	var out []string
	for table := range left {
		if !right[table] {
			out = append(out, table)
		}
	}
	return out
}
