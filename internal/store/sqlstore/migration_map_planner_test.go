package sqlstore_test

import (
	"strings"
	"testing"

	"agent-testbench/internal/store/sqlstore"
)

func TestCoreSchemaSQLIncludesMapPlannerTables(t *testing.T) {
	statements := sqlstore.CoreSchemaSQL(sqlstore.PostgresDialect{})
	joined := strings.Join(statements, "\n")
	assertSQLContains(t, joined, "core schema map planner",
		"create table if not exists test_map_plan_instances",
		"plan_id text primary key",
		"map_id text not null",
		"logical_plan_json jsonb not null",
		"physical_plan_json jsonb not null",
		"rule_trace_json jsonb not null",
		"candidate_plan_json jsonb not null",
		"create table if not exists test_map_plan_tasks",
		"task_id text not null",
		"task_kind text not null",
		"workflow_run_id text not null",
		"api_case_run_id text not null",
		"create table if not exists test_map_plan_task_edges",
		"from_task_id text not null",
		"to_task_id text not null",
		"edge_kind text not null",
		"idx_test_map_plan_instances_map",
		"idx_test_map_plan_tasks_status",
	)
}

func TestUpgradeSchemaAddsMapPlannerTablesFromVersionSixteen(t *testing.T) {
	tests := []struct {
		name    string
		dialect sqlstore.Dialect
		want    []string
	}{
		{
			name:    "postgres",
			dialect: sqlstore.PostgresDialect{},
			want: []string{
				"create table if not exists test_map_plan_instances",
				"plan_id text primary key",
				"logical_plan_json jsonb not null",
				"create table if not exists test_map_plan_tasks",
				"create table if not exists test_map_plan_task_edges",
			},
		},
		{
			name:    "mysql",
			dialect: sqlstore.MySQLDialect{},
			want: []string{
				"create table if not exists test_map_plan_instances",
				"plan_id varchar(255) primary key",
				"logical_plan_json json not null",
				"create table if not exists test_map_plan_tasks",
				"create table if not exists test_map_plan_task_edges",
			},
		},
		{
			name:    "sqlite",
			dialect: sqlstore.SQLiteDialect{},
			want: []string{
				"create table if not exists test_map_plan_instances",
				"plan_id text primary key",
				"logical_plan_json text not null",
				"create table if not exists test_map_plan_tasks",
				"create table if not exists test_map_plan_task_edges",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			migration := newMigrationDB(t)
			migration.queueUpgradeFromSchemaVersion(16)

			status := migration.upgradeSchema(t, tt.dialect, "upgrade v16 schema")
			assertAppliedCoreSchema(t, status, "upgraded v16 schema status")
			assertSQLContains(t, migration.execSQL(), tt.name+" v16 map planner upgrade", tt.want...)
		})
	}
}
