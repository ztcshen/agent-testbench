package sqlstore

import "fmt"

func coreEnvironmentCatalogSchemaSQL(d Dialect, types coreSchemaTypes) []string {
	statements := []string{
		fmt.Sprintf(`
create table if not exists environments (
  id %s primary key,
  display_name %s not null,
  description %s not null,
  status %s not null,
  verified %s not null,
  services_json %s not null,
  repos_json %s not null,
  compose_json %s not null,
  health_checks_json %s not null,
  verification_workflow_id %s not null,
  last_verification_run_id %s not null,
  last_verification_status %s not null,
  evidence_complete %s not null,
  topology_complete %s not null,
  last_verified_at %s,
  summary_json %s not null,
  created_at %s not null,
  updated_at %s not null
);`, types.keyText, types.text, types.text, types.keyText, types.boolType, types.jsonType, types.jsonType, types.jsonType, types.jsonType, types.keyText, types.runIDText, types.keyText, types.boolType, types.boolType, types.timeType, types.jsonType, types.timeType, types.timeType),
		d.CreateIndexSQL("idx_environments_verified_status", "environments", []string{"verified", "status", "updated_at", "id"}),
		d.CreateIndexSQL("idx_environments_verification", "environments", []string{"verification_workflow_id", "last_verification_status", "updated_at", "id"}),
		fmt.Sprintf(`
create table if not exists environment_components (
  env_id %s not null,
  component_id %s not null,
  display_name %s not null,
  kind %s not null,
  role %s not null,
  compose_service %s not null,
  image %s not null,
  required %s not null,
  runtime_json %s not null,
  healthcheck_json %s not null,
  summary_json %s not null,
  created_at %s not null,
  updated_at %s not null,
  primary key (env_id, component_id),
  foreign key (env_id) references environments(id) on delete cascade
);`, types.keyText, types.keyText, types.text, types.keyText, types.keyText, types.keyText, types.text, types.boolType, types.jsonType, types.jsonType, types.jsonType, types.timeType, types.timeType),
		d.CreateIndexSQL("idx_environment_components_kind", "environment_components", []string{"env_id", "kind", "role", "component_id"}),
		fmt.Sprintf(`
create table if not exists component_dependencies (
  env_id %s not null,
  consumer_component_id %s not null,
  provider_component_id %s not null,
  phase %s not null,
  capability %s not null,
  required %s not null,
  profile_json %s not null,
  created_at %s not null,
  updated_at %s not null,
  primary key (env_id, consumer_component_id, provider_component_id, phase, capability),
  foreign key (env_id, consumer_component_id) references environment_components(env_id, component_id) on delete cascade,
  foreign key (env_id, provider_component_id) references environment_components(env_id, component_id) on delete cascade
);`, types.keyText, types.keyText, types.keyText, types.keyText, types.keyText, types.boolType, types.jsonType, types.timeType, types.timeType),
		d.CreateIndexSQL("idx_component_dependencies_provider", "component_dependencies", []string{"env_id", "provider_component_id", "phase", "capability", "consumer_component_id"}),
		d.CreateIndexSQL("idx_component_dependencies_phase", "component_dependencies", []string{"env_id", "phase", "capability", "consumer_component_id", "provider_component_id"}),
		fmt.Sprintf(`
create table if not exists component_config_assets (
  env_id %s not null,
  owner_component_id %s not null,
  asset_id %s not null,
  asset_kind %s not null,
  target_component_id %s not null,
  target_path %s not null,
  content_inline %s not null,
  remote_ref_json %s not null,
  sha256 %s not null,
  size_bytes %s not null,
  apply_order %s not null,
  %s %s not null,
  summary_json %s not null,
  created_at %s not null,
  updated_at %s not null,
  primary key (env_id, owner_component_id, asset_id),
  foreign key (env_id, owner_component_id) references environment_components(env_id, component_id) on delete cascade,
  foreign key (env_id, target_component_id) references environment_components(env_id, component_id) on delete cascade
);`, types.keyText, types.keyText, types.keyText, types.keyText, types.keyText, types.text, types.text, types.jsonType, types.text, types.intType, types.intType, d.QuoteIdent("sensitive"), types.boolType, types.jsonType, types.timeType, types.timeType),
		d.CreateIndexSQL("idx_component_config_assets_target", "component_config_assets", []string{"env_id", "target_component_id", "asset_kind", "apply_order", "asset_id"}),
		d.CreateIndexSQL("idx_component_config_assets_owner_order", "component_config_assets", []string{"env_id", "owner_component_id", "apply_order", "asset_id"}),
	}
	statements = append(statements, coreEnvironmentFileSchemaSQL(d, types)...)
	statements = append(statements, coreEnvironmentRuntimeMetadataSchemaSQL(d, types)...)
	return statements
}

func coreEnvironmentFileSchemaSQL(d Dialect, types coreSchemaTypes) []string {
	return []string{
		fmt.Sprintf(`
create table if not exists environment_files (
  env_id %s not null,
  file_path %s not null,
  file_kind %s not null,
  content_inline %s not null,
  required %s not null,
  apply_order %s not null,
  summary_json %s not null,
  created_at %s not null,
  updated_at %s not null,
  primary key (env_id, file_path, file_kind),
  foreign key (env_id) references environments(id) on delete cascade
);`, types.keyText, types.keyText, types.keyText, types.text, types.boolType, types.intType, types.jsonType, types.timeType, types.timeType),
		d.CreateIndexSQL("idx_environment_files_kind_order", "environment_files", []string{"env_id", "file_kind", "apply_order", "file_path"}),
	}
}

func coreEnvironmentRuntimeMetadataSchemaSQL(d Dialect, types coreSchemaTypes) []string {
	return []string{
		fmt.Sprintf(`
create table if not exists environment_services (
  env_id %s not null,
  service_id %s not null,
  repo_url %s not null,
  branch %s not null,
  ref %s not null,
  checkout %s not null,
  summary_json %s not null,
  created_at %s not null,
  updated_at %s not null,
  primary key (env_id, service_id),
  foreign key (env_id) references environments(id) on delete cascade
);`, types.keyText, types.keyText, types.text, types.keyText, types.text, types.text, types.jsonType, types.timeType, types.timeType),
		environmentServicesRepoIndexSQL(d),
		fmt.Sprintf(`
create table if not exists environment_health_checks (
  env_id %s not null,
  check_id %s not null,
  check_kind %s not null,
  url %s not null,
  address %s not null,
  command %s not null,
  compose_service %s not null,
  expect %s not null,
  apply_order %s not null,
  summary_json %s not null,
  created_at %s not null,
  updated_at %s not null,
  primary key (env_id, check_id),
  foreign key (env_id) references environments(id) on delete cascade
);`, types.keyText, types.keyText, types.keyText, types.text, types.text, types.text, types.keyText, types.keyText, types.intType, types.jsonType, types.timeType, types.timeType),
		d.CreateIndexSQL("idx_environment_health_checks_kind_order", "environment_health_checks", []string{"env_id", "check_kind", "apply_order", "check_id"}),
	}
}

func environmentServicesRepoIndexSQL(d Dialect) string {
	if d.Name() == "mysql" {
		return fmt.Sprintf("create index %s\n  on %s(%s, %s(191), %s);",
			d.QuoteIdent("idx_environment_services_repo"),
			d.QuoteIdent("environment_services"),
			d.QuoteIdent("env_id"),
			d.QuoteIdent("repo_url"),
			d.QuoteIdent("service_id"))
	}
	return d.CreateIndexSQL("idx_environment_services_repo", "environment_services", []string{"env_id", "repo_url", "service_id"})
}
