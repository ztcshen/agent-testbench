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
	if err := ensureLegacySharedColumns(ctx, tx); err != nil {
		return false, err
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
  env_id, service_id, asset_id, asset_kind, coalesce(nullif(target_component_id, ''), service_id), target_path,
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
    'Services', json(coalesce((select json_group_array(json_object(
      'ID', id,
      'DisplayName', display_name,
      'Kind', role,
      'AttachedTemplateIDs', json(attached_template_ids),
      'GitURL', git_url,
      'GitBranch', git_branch,
      'RepoEnv', repo_env,
      'ContainerName', container_name,
      'Image', image,
      'DockerService', docker_service,
      'ServicePort', service_port,
      'ManagementPort', management_port,
      'MemoryMb', memory_mb,
      'CPUMilli', cpu_milli,
      'StartupCommand', startup_command,
      'HealthURL', health_url,
      'LogPath', log_path,
      'Status', status,
      'SortOrder', sort_order
    )) from node_config), '[]')),
    'Workflows', json(coalesce((select json_group_array(json_object(
      'ID', id,
      'DisplayName', name,
      'Description', description
    )) from workflow), '[]')),
    'InterfaceNodes', json(coalesce((select json_group_array(json_object(
      'ID', id,
      'DisplayName', display_name,
      'ServiceID', service_id,
      'Operation', operation,
      'Method', method,
      'Path', path,
      'TemplateID', template_id,
      'Version', version,
      'Status', status,
      'Tags', json(tags_json),
      'Description', description,
      'SortOrder', sort_order,
      'CreatedAt', created_at,
      'UpdatedAt', updated_at
    )) from interface_node), '[]')),
    'InterfaceFields', json('[]'),
    'APICases', json(coalesce((select json_group_array(json_object(
      'ID', id,
      'DisplayName', title,
      'NodeID', node_id,
      'CaseType', case_type,
      'Scenario', scenario,
      'PayloadTemplateJSON', payload_template_json,
      'RequestTemplateID', request_template_id,
      'PatchJSON', patch_json,
      'RenderMode', render_mode,
      'ExpectedJSON', expected_json,
      'RequiredForAdmission', required_for_admission,
      'Status', status,
      'SortOrder', sort_order
    )) from interface_node_case), '[]')),
    'RequestTemplates', json(coalesce((select json_group_array(json_object(
      'ID', id,
      'DisplayName', name,
      'NodeID', node_id,
      'TemplateJSON', template_json,
      'Version', version,
      'Status', status,
      'SortOrder', sort_order
    )) from interface_node_request_template), '[]')),
    'WorkflowBindings', json(coalesce((select json_group_array(json_object(
      'WorkflowID', workflow_id,
      'StepID', step_id,
      'NodeID', node_id,
      'CaseID', case_id,
      'Required', required,
      'SortOrder', sort_order
    )) from workflow_interface_node), '[]')),
    'CaseDependencies', json(coalesce((select json_group_array(json_object(
      'ID', id,
      'CaseID', case_id,
      'FixtureID', fixture_profile_id,
      'MappingsJSON', mappings_json,
      'Required', required,
      'Status', status,
      'SortOrder', sort_order
    )) from interface_node_case_dependency), '[]')),
    'Fixtures', json(coalesce((select json_group_array(json_object(
      'ID', id,
      'DisplayName', name,
      'Kind', source_type,
      'SourceWorkflowID', source_workflow_id,
      'SourceUntilStep', source_until_step,
      'TTLSeconds', ttl_seconds,
      'Status', status,
      'SortOrder', sort_order
    )) from fixture_profile), '[]')),
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

func ensureLegacySharedColumns(ctx context.Context, tx *sql.Tx) error {
	hasRuns, err := txHasTable(ctx, tx, "runs")
	if err != nil {
		return err
	}
	if !hasRuns {
		return nil
	}
	hasEnvironmentID, err := txHasColumn(ctx, tx, "runs", "environment_id")
	if err != nil {
		return err
	}
	if hasEnvironmentID {
		return nil
	}
	if _, err := tx.ExecContext(ctx, `alter table runs add column environment_id text not null default '';`); err != nil {
		return fmt.Errorf("add missing legacy runs.environment_id column: %w", err)
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

func txHasColumn(ctx context.Context, tx *sql.Tx, table string, column string) (bool, error) {
	rows, err := tx.QueryContext(ctx, fmt.Sprintf("pragma table_info(%s);", sqlstore.SQLiteDialect{}.QuoteIdent(table)))
	if err != nil {
		return false, fmt.Errorf("list sqlite columns for %q: %w", table, err)
	}
	defer rows.Close()
	for rows.Next() {
		var cid int
		var name string
		var dataType string
		var notNull int
		var defaultValue any
		var pk int
		if err := rows.Scan(&cid, &name, &dataType, &notNull, &defaultValue, &pk); err != nil {
			return false, fmt.Errorf("scan sqlite column for %q: %w", table, err)
		}
		if name == column {
			return true, nil
		}
	}
	if err := rows.Err(); err != nil {
		return false, fmt.Errorf("iterate sqlite columns for %q: %w", table, err)
	}
	return false, nil
}
