package sqlstore_test

import (
	"context"
	"database/sql/driver"
	"errors"
	"strings"
	"testing"
	"time"

	"agent-testbench/internal/store"
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
		"create table if not exists test_map_plan_leases",
		"lease_token text not null",
		"lease_expires_at timestamptz not null",
		"idx_test_map_plan_leases_expiry",
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

func TestMapPlanLeaseSchemaSupportsFreshAndVersionNineteenUpgrade(t *testing.T) {
	tests := []struct {
		name     string
		dialect  sqlstore.Dialect
		planType string
		keyType  string
		timeType string
	}{
		{name: "postgres", dialect: sqlstore.PostgresDialect{}, planType: "text", keyType: "text", timeType: "timestamptz"},
		{name: "mysql", dialect: sqlstore.MySQLDialect{}, planType: "varchar(255)", keyType: "varchar(128)", timeType: "datetime(6)"},
		{name: "sqlite", dialect: sqlstore.SQLiteDialect{}, planType: "text", keyType: "text", timeType: "text"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			want := []string{
				"create table if not exists test_map_plan_leases",
				"plan_id " + tt.planType + " primary key",
				"owner_id " + tt.keyType + " not null",
				"lease_token " + tt.keyType + " not null",
				"lease_expires_at " + tt.timeType + " not null",
				"idx_test_map_plan_leases_expiry",
			}
			assertSQLContains(t, strings.Join(sqlstore.CoreSchemaSQL(tt.dialect), "\n"), tt.name+" fresh lease schema", want...)

			migration := newMigrationDB(t)
			migration.queueUpgradeFromSchemaVersion(19)
			status := migration.upgradeSchema(t, tt.dialect, "upgrade v19 map plan lease schema")
			assertAppliedCoreSchema(t, status, "upgraded v19 map plan lease schema")
			assertSQLContains(t, migration.execSQL(), tt.name+" v19 lease upgrade", want...)
		})
	}
}

func TestRenewTestMapPlanLeaseAcceptsMySQLNoopUpdateForActiveOwner(t *testing.T) {
	ctx := context.Background()
	db, state := openFakeSQLDB(t)
	defer db.Close()
	runtime := sqlstore.New(db, sqlstore.MySQLDialect{})
	now := time.Date(2026, 7, 17, 10, 0, 0, 0, time.UTC)
	lease := store.TestMapPlanLease{
		PlanID: "plan.noop", OwnerID: "owner.one", Token: "token-one", ExpiresAt: now.Add(time.Minute),
	}

	state.queueExecRowsAffected(0)
	state.queueRows(fakeRows{
		columns: []string{"count"},
		values:  [][]driver.Value{{int64(1)}},
	})
	renewed, err := runtime.RenewTestMapPlanLease(ctx, lease, now, lease.ExpiresAt)
	if err != nil {
		t.Fatalf("renew active no-op MySQL lease: %v", err)
	}
	if !renewed.ExpiresAt.Equal(lease.ExpiresAt) || !renewed.UpdatedAt.Equal(now) {
		t.Fatalf("renewed lease = %#v", renewed)
	}
	query := state.lastQuery(t)
	for _, fragment := range []string{"test_map_plan_leases", "owner_id", "lease_token", "lease_expires_at >"} {
		if !strings.Contains(query.query, fragment) {
			t.Fatalf("active lease verification query missing %q: %s", fragment, query.query)
		}
	}
	commits, rollbacks := state.txCounts()
	if commits != 1 || rollbacks != 0 {
		t.Fatalf("active no-op lease tx counts commits=%d rollbacks=%d", commits, rollbacks)
	}
}

func TestRenewTestMapPlanLeaseRejectsZeroRowUpdateWhenOwnerIsInactive(t *testing.T) {
	ctx := context.Background()
	db, state := openFakeSQLDB(t)
	defer db.Close()
	runtime := sqlstore.New(db, sqlstore.MySQLDialect{})
	now := time.Date(2026, 7, 17, 10, 0, 0, 0, time.UTC)
	lease := store.TestMapPlanLease{
		PlanID: "plan.lost", OwnerID: "owner.one", Token: "token-one", ExpiresAt: now.Add(time.Minute),
	}

	state.queueExecRowsAffected(0)
	state.queueRows(fakeRows{
		columns: []string{"count"},
		values:  [][]driver.Value{{int64(0)}},
	})
	_, err := runtime.RenewTestMapPlanLease(ctx, lease, now, lease.ExpiresAt)
	if !errors.Is(err, store.ErrTestMapPlanLeaseLost) {
		t.Fatalf("inactive lease renewal error = %v, want lease lost", err)
	}
	commits, rollbacks := state.txCounts()
	if commits != 0 || rollbacks != 1 {
		t.Fatalf("inactive lease tx counts commits=%d rollbacks=%d", commits, rollbacks)
	}
}
