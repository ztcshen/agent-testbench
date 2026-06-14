package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"agent-testbench/internal/store/sqlstore"
)

var legacyOnlyTables = []string{
	"interface_node_case_dependency",
	"fixture_table_binding",
	"fixture_profile",
	"workflow_interface_node",
	"interface_node_case",
	"interface_node_request_template",
	"interface_node_field",
	"interface_node",
	"workflow_node",
	"workflow",
	"node_config",
	"template_config",
	"template",
	"kv",
	"service_config_assets",
	"service_dependencies",
}

var legacyOnlyIndexes = []string{
	"idx_api_case_runs_case_created",
	"idx_config_read_model_version",
	"idx_config_versions_active",
	"idx_config_versions_profile_published",
	"idx_evidence_records_category",
	"idx_evidence_records_step",
	"idx_post_process_tasks_kind_status",
	"idx_post_process_tasks_run_created",
	"idx_trace_topologies_case",
	"idx_trace_topologies_workflow_run",
}

func normalizeLegacySchema(ctx context.Context, db *sql.DB) (bool, error) {
	hasLegacy, err := hasAnyTable(ctx, db, legacyOnlyTables)
	if err != nil {
		return false, err
	}
	if !hasLegacy {
		return false, nil
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return false, fmt.Errorf("begin legacy sqlite schema normalization: %w", err)
	}
	committed := false
	defer rollbackLegacyTx(tx, &committed)

	dialect := sqlstore.SQLiteDialect{}
	for _, statement := range sqlstore.CoreSchemaSQL(dialect) {
		if _, err := tx.ExecContext(ctx, statement); err != nil {
			return false, fmt.Errorf("apply shared sqlite schema during legacy normalization: %w", err)
		}
	}
	if err := migrateLegacyServiceGraph(ctx, tx); err != nil {
		return false, err
	}
	if err := ensureLegacyProfileCatalogAnchor(ctx, tx); err != nil {
		return false, err
	}
	for _, table := range legacyOnlyTables {
		if _, err := tx.ExecContext(ctx, fmt.Sprintf("drop table if exists %s;", dialect.QuoteIdent(table))); err != nil {
			return false, fmt.Errorf("drop legacy sqlite table %q: %w", table, err)
		}
	}
	for _, index := range legacyOnlyIndexes {
		if _, err := tx.ExecContext(ctx, fmt.Sprintf("drop index if exists %s;", dialect.QuoteIdent(index))); err != nil {
			return false, fmt.Errorf("drop legacy sqlite index %q: %w", index, err)
		}
	}
	if _, err := tx.ExecContext(ctx, `delete from schema_versions;`); err != nil {
		return false, fmt.Errorf("reset legacy sqlite schema versions: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
insert into schema_versions (version, name, applied_at)
values (?, ?, ?);`, sqlstore.CurrentSchemaVersion, sqlstore.CoreSchemaName, time.Now().UTC()); err != nil {
		return false, fmt.Errorf("record normalized sqlite schema version: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return false, fmt.Errorf("commit legacy sqlite schema normalization: %w", err)
	}
	committed = true
	return true, nil
}

func rollbackLegacyTx(tx *sql.Tx, committed *bool) {
	if tx == nil || *committed {
		return
	}
	if err := tx.Rollback(); err != nil {
		return
	}
}

func migrateLegacyServiceGraph(ctx context.Context, tx *sql.Tx) error {
	hasDependencies, err := txHasTable(ctx, tx, "service_dependencies")
	if err != nil {
		return err
	}
	if hasDependencies {
		if _, err := tx.ExecContext(ctx, `
insert or ignore into component_dependencies (
  env_id, consumer_component_id, provider_component_id, phase, capability,
  required, profile_json, created_at, updated_at
)
select
  env_id, service_id, dependency_component_id, '', dependency_kind,
  required, profile_json, created_at, updated_at
from service_dependencies;`); err != nil {
			return fmt.Errorf("migrate legacy service dependencies: %w", err)
		}
	}

	hasAssets, err := txHasTable(ctx, tx, "service_config_assets")
	if err != nil {
		return err
	}
	if hasAssets {
		if _, err := tx.ExecContext(ctx, `
insert or ignore into component_config_assets (
  env_id, owner_component_id, asset_id, asset_kind, target_component_id, target_path,
  content_inline, remote_ref_json, sha256, size_bytes, apply_order, sensitive,
  summary_json, created_at, updated_at
)
select
  env_id, service_id, asset_id, asset_kind, target_component_id, target_path,
  content_inline, remote_ref_json, sha256, size_bytes, apply_order, sensitive,
  summary_json, created_at, updated_at
from service_config_assets;`); err != nil {
			return fmt.Errorf("migrate legacy service config assets: %w", err)
		}
	}
	return nil
}

func ensureLegacyProfileCatalogAnchor(ctx context.Context, tx *sql.Tx) error {
	hasKV, err := txHasTable(ctx, tx, "kv")
	if err != nil {
		return err
	}
	if !hasKV {
		return nil
	}
	if _, err := tx.ExecContext(ctx, `
insert or ignore into profile_catalogs (
  profile_id, indexed_at, catalog_json, services, workflows, interface_nodes, api_cases,
  request_templates, workflow_bindings, case_dependencies, fixtures, templates, template_configs
)
select
  value,
  coalesce(nullif(updated_at, ''), ?),
  json_object(
    'ProfileID', value,
    'Services', json('[]'),
    'Workflows', json('[]'),
    'InterfaceNodes', json('[]'),
    'InterfaceFields', json('[]'),
    'APICases', json('[]'),
    'RequestTemplates', json('[]'),
    'WorkflowBindings', json('[]'),
    'CaseDependencies', json('[]'),
    'Fixtures', json('[]'),
    'TemplateConfigs', json('[]')
  ),
  (select count(*) from node_config),
  (select count(*) from workflow),
  (select count(*) from interface_node),
  (select count(*) from interface_node_case),
  (select count(*) from interface_node_request_template),
  (select count(*) from workflow_interface_node),
  (select count(*) from interface_node_case_dependency),
  (select count(*) from fixture_profile),
  (select count(*) from template),
  (select count(*) from template_config)
from kv
where key = 'active_profile_id' and value <> '';`, time.Now().UTC()); err != nil {
		return fmt.Errorf("anchor legacy profile catalog: %w", err)
	}
	return nil
}

func hasAnyTable(ctx context.Context, db *sql.DB, tables []string) (bool, error) {
	for _, table := range tables {
		var exists int
		if err := db.QueryRowContext(ctx, sqlstore.SQLiteDialect{}.TableExistsSQL(table)).Scan(&exists); err != nil {
			return false, fmt.Errorf("check sqlite table %q: %w", table, err)
		}
		if exists != 0 {
			return true, nil
		}
	}
	return false, nil
}

func txHasTable(ctx context.Context, tx *sql.Tx, table string) (bool, error) {
	var exists int
	if err := tx.QueryRowContext(ctx, sqlstore.SQLiteDialect{}.TableExistsSQL(table)).Scan(&exists); err != nil {
		return false, fmt.Errorf("check sqlite table %q: %w", table, err)
	}
	return exists != 0, nil
}
