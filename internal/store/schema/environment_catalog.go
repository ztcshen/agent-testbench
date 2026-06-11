package schema

var environmentCatalogChanges = []Change{
	{
		Version: 15,
		Name:    "add environment catalog",
		SQL: `
create table if not exists environments (
  id text primary key,
  display_name text not null default '',
  description text not null default '',
  status text not null default 'draft',
  verified integer not null default 0,
  services_json text not null default '[]',
  repos_json text not null default '{}',
  compose_json text not null default '{}',
  health_checks_json text not null default '[]',
  verification_workflow_id text not null default '',
  last_verification_run_id text not null default '',
  last_verification_status text not null default '',
  evidence_complete integer not null default 0,
  topology_complete integer not null default 0,
  last_verified_at text,
  summary_json text not null default '{}',
  created_at text not null,
  updated_at text not null
);

create index if not exists idx_environments_verified_status
  on environments(verified, status, updated_at, id);

create index if not exists idx_environments_verification
  on environments(verification_workflow_id, last_verification_status, updated_at, id);`,
	},
	{
		Version: 16,
		Name:    "add environment component assets",
		SQL: `
create table if not exists environment_components (
  env_id text not null,
  component_id text not null,
  display_name text not null default '',
  kind text not null default '',
  role text not null default '',
  compose_service text not null default '',
  image text not null default '',
  required integer not null default 1,
  runtime_json text not null default '{}',
  healthcheck_json text not null default '{}',
  summary_json text not null default '{}',
  created_at text not null,
  updated_at text not null,
  primary key (env_id, component_id),
  foreign key (env_id) references environments(id) on delete cascade
);

create index if not exists idx_environment_components_kind
  on environment_components(env_id, kind, role, component_id);

create table if not exists service_dependencies (
  env_id text not null,
  service_id text not null,
  dependency_component_id text not null,
  dependency_kind text not null default '',
  required integer not null default 1,
  profile_json text not null default '{}',
  created_at text not null,
  updated_at text not null,
  primary key (env_id, service_id, dependency_component_id, dependency_kind),
  foreign key (env_id, service_id) references environment_components(env_id, component_id) on delete cascade,
  foreign key (env_id, dependency_component_id) references environment_components(env_id, component_id) on delete cascade
);

create index if not exists idx_service_dependencies_component
  on service_dependencies(env_id, dependency_component_id, dependency_kind, service_id);

create table if not exists service_config_assets (
  env_id text not null,
  service_id text not null,
  asset_id text not null,
  asset_kind text not null default '',
  target_component_id text not null default '',
  target_path text not null default '',
  content_inline text not null default '',
  remote_ref_json text not null default '{}',
  sha256 text not null default '',
  size_bytes integer not null default 0,
  apply_order integer not null default 0,
  sensitive integer not null default 0,
  summary_json text not null default '{}',
  created_at text not null,
  updated_at text not null,
  primary key (env_id, service_id, asset_id),
  foreign key (env_id, service_id) references environment_components(env_id, component_id) on delete cascade,
  foreign key (env_id, target_component_id) references environment_components(env_id, component_id) on delete cascade
);

create index if not exists idx_service_config_assets_target
  on service_config_assets(env_id, target_component_id, asset_kind, apply_order, asset_id);

create index if not exists idx_service_config_assets_service_order
  on service_config_assets(env_id, service_id, apply_order, asset_id);`,
	},
	{
		Version: 17,
		Name:    "generalize environment component graph",
		SQL: `
create table if not exists component_dependencies (
  env_id text not null,
  consumer_component_id text not null,
  provider_component_id text not null,
  phase text not null default '',
  capability text not null default '',
  required integer not null default 1,
  profile_json text not null default '{}',
  created_at text not null,
  updated_at text not null,
  primary key (env_id, consumer_component_id, provider_component_id, phase, capability),
  foreign key (env_id, consumer_component_id) references environment_components(env_id, component_id) on delete cascade,
  foreign key (env_id, provider_component_id) references environment_components(env_id, component_id) on delete cascade
);

create index if not exists idx_component_dependencies_provider
  on component_dependencies(env_id, provider_component_id, phase, capability, consumer_component_id);

create index if not exists idx_component_dependencies_phase
  on component_dependencies(env_id, phase, capability, consumer_component_id, provider_component_id);

create table if not exists component_config_assets (
  env_id text not null,
  owner_component_id text not null,
  asset_id text not null,
  asset_kind text not null default '',
  target_component_id text not null default '',
  target_path text not null default '',
  content_inline text not null default '',
  remote_ref_json text not null default '{}',
  sha256 text not null default '',
  size_bytes integer not null default 0,
  apply_order integer not null default 0,
  sensitive integer not null default 0,
  summary_json text not null default '{}',
  created_at text not null,
  updated_at text not null,
  primary key (env_id, owner_component_id, asset_id),
  foreign key (env_id, owner_component_id) references environment_components(env_id, component_id) on delete cascade,
  foreign key (env_id, target_component_id) references environment_components(env_id, component_id) on delete cascade
);

create index if not exists idx_component_config_assets_target
  on component_config_assets(env_id, target_component_id, asset_kind, apply_order, asset_id);

create index if not exists idx_component_config_assets_owner_order
  on component_config_assets(env_id, owner_component_id, apply_order, asset_id);`,
	},
	{
		Version: 18,
		Name:    "link workflow runs to environments",
		SQL: `
alter table runs add column environment_id text not null default '';`,
	},
}
