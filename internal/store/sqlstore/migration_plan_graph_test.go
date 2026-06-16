package sqlstore_test

import (
	"errors"
	"strings"
	"testing"

	"agent-testbench/internal/store/sqlstore"
)

func TestCoreSchemaSQLIncludesPlannerAssociationsOnRuns(t *testing.T) {
	statements := sqlstore.CoreSchemaSQL(sqlstore.PostgresDialect{})
	joined := strings.Join(statements, "\n")
	assertSQLContains(t, joined, "core schema planner associations",
		"test_plan_map_id text not null",
		"test_plan_path_id text not null",
		"planner_summary_json jsonb not null",
		"test_plan_node_id text not null",
		"test_plan_operation text not null",
	)
}

func TestUpgradeSchemaAddsPlannerAssociationsFromVersionFifteen(t *testing.T) {
	tests := []struct {
		name    string
		dialect sqlstore.Dialect
		want    []string
	}{
		{
			name:    "postgres",
			dialect: sqlstore.PostgresDialect{},
			want: []string{
				`alter table "runs" add column "test_plan_map_id" text not null default ''`,
				`alter table "runs" add column "test_plan_path_id" text not null default ''`,
				`alter table "runs" add column "planner_summary_json" jsonb not null default '{}'`,
				`alter table "api_case_runs" add column "test_plan_node_id" text not null default ''`,
				`alter table "api_case_runs" add column "test_plan_operation" text not null default ''`,
				`alter table "api_case_runs" add column "planner_summary_json" jsonb not null default '{}'`,
			},
		},
		{
			name:    "mysql",
			dialect: sqlstore.MySQLDialect{},
			want: []string{
				"alter table `runs` add column `test_plan_map_id` varchar(128) not null default ''",
				"alter table `runs` add column `test_plan_path_id` varchar(128) not null default ''",
				"alter table `runs` add column `planner_summary_json` json null",
				"update `runs` set `planner_summary_json` = json_object() where `planner_summary_json` is null",
				"alter table `runs` modify column `planner_summary_json` json not null",
				"alter table `api_case_runs` add column `test_plan_node_id` varchar(128) not null default ''",
				"alter table `api_case_runs` add column `test_plan_operation` varchar(128) not null default ''",
				"alter table `api_case_runs` add column `planner_summary_json` json null",
				"update `api_case_runs` set `planner_summary_json` = json_object() where `planner_summary_json` is null",
				"alter table `api_case_runs` modify column `planner_summary_json` json not null",
			},
		},
		{
			name:    "sqlite",
			dialect: sqlstore.SQLiteDialect{},
			want: []string{
				`alter table "runs" add column "test_plan_map_id" text not null default ''`,
				`alter table "runs" add column "test_plan_path_id" text not null default ''`,
				`alter table "runs" add column "planner_summary_json" text not null default '{}'`,
				`alter table "api_case_runs" add column "test_plan_node_id" text not null default ''`,
				`alter table "api_case_runs" add column "test_plan_operation" text not null default ''`,
				`alter table "api_case_runs" add column "planner_summary_json" text not null default '{}'`,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			migration := newMigrationDB(t)
			migration.queueUpgradeFromSchemaVersion(15)

			status := migration.upgradeSchema(t, tt.dialect, "upgrade v15 schema")
			assertAppliedCoreSchema(t, status, "upgraded v15 schema status")
			assertSQLContains(t, migration.execSQL(), tt.name+" v15 planner association upgrade", tt.want...)
		})
	}
}

func TestUpgradeSchemaIgnoresDuplicateMySQLColumnsDuringIncrementalReplay(t *testing.T) {
	migration := newMigrationDB(t)
	dialect := sqlstore.MySQLDialect{}
	migration.queueExistingSchemaVersion(15)

	for i := 0; i < len(sqlstore.CoreSchemaSQL(dialect)); i++ {
		migration.state.queueExecError(nil)
	}
	migration.state.queueExecError(errors.New("Error 1060 (42S21): Duplicate column name 'test_plan_map_id'"))
	migration.state.queueExecError(errors.New("Error 1060 (42S21): Duplicate column name 'test_plan_path_id'"))
	migration.queueExistingSchemaVersion(sqlstore.CurrentSchemaVersion)

	status := migration.upgradeSchema(t, dialect, "upgrade mysql v15 schema with partially applied planner columns")
	assertAppliedCoreSchema(t, status, "upgraded mysql schema status")
	assertSQLContains(t, migration.execSQL(), "mysql v15 incremental replay",
		"alter table `runs` add column `planner_summary_json` json null",
		"alter table `runs` modify column `planner_summary_json` json not null",
	)
}
