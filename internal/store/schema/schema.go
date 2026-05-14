package schema

type Change struct {
	Version int
	Name    string
	SQL     string
}

const CurrentVersion = 2

func All() []Change {
	return []Change{
		{
			Version: 1,
			Name:    "create runtime store tables",
			SQL: `
create table if not exists runs (
  id text primary key,
  profile_id text not null,
  workflow_id text not null,
  status text not null,
  evidence_root text not null,
  summary_json text not null default '',
  started_at text,
  finished_at text,
  created_at text not null,
  updated_at text not null
);

create table if not exists api_case_runs (
  id text primary key,
  run_id text not null,
  case_id text not null,
  status text not null,
  request_summary_json text not null default '',
  assertion_summary_json text not null default '',
  started_at text,
  finished_at text,
  created_at text not null,
  foreign key (run_id) references runs(id) on delete cascade
);

create index if not exists idx_api_case_runs_run_id_created_at
  on api_case_runs(run_id, created_at, id);

create table if not exists evidence_records (
  id text primary key,
  run_id text not null,
  case_run_id text not null default '',
  kind text not null,
  uri text not null,
  media_type text not null default '',
  sha256 text not null default '',
  size_bytes integer not null default 0,
  summary text not null default '',
  created_at text not null,
  foreign key (run_id) references runs(id) on delete cascade
);

create index if not exists idx_evidence_records_run_id_created_at
  on evidence_records(run_id, created_at, id);

create table if not exists baseline_gates (
  profile_id text not null,
  subject_id text not null,
  status text not null,
  required integer not null,
  summary_json text not null default '',
  checked_at text,
  updated_at text not null,
  primary key (profile_id, subject_id)
);

create table if not exists profile_indexes (
  profile_id text primary key,
  bundle_path text not null,
  bundle_digest text not null,
  summary_json text not null default '',
  imported_at text,
  updated_at text not null
);`,
		},
		{
			Version: 2,
			Name:    "add template config catalog tables",
			SQL: `
create table if not exists kv (
  key text primary key,
  value text not null,
  updated_at text not null
);

create table if not exists template (
  id text primary key,
  name text not null default '',
  kind text not null default '',
  version text not null default '',
  parent_id text not null default '',
  path text not null default '',
  watermark text not null default '',
  status text not null default 'active',
  sort_order integer not null default 0
);

create index if not exists idx_template_parent_sort
  on template(parent_id, sort_order, id);

create table if not exists template_config (
  id text primary key,
  template_id text not null,
  node_id text not null default '',
  workflow_id text not null default '',
  scope_type text not null default '',
  scope_id text not null default '',
  title text not null default '',
  description text not null default '',
  config_json text not null default '{}',
  status text not null default 'active',
  sort_order integer not null default 0
);

create index if not exists idx_template_config_template_sort
  on template_config(template_id, sort_order, id);

create index if not exists idx_template_config_node
  on template_config(node_id, scope_type, sort_order, id);

create index if not exists idx_template_config_workflow
  on template_config(workflow_id, scope_type, sort_order, id);

create table if not exists node_config (
  id text primary key,
  display_name text not null default '',
  role text not null default '',
  attached_template_ids text not null default '[]',
  git_url text not null default '',
  git_branch text not null default '',
  repo_env text not null default '',
  container_name text not null default '',
  image text not null default '',
  docker_service text not null default '',
  service_port integer not null default 0,
  management_port integer not null default 0,
  memory_mb integer not null default 0,
  cpu_milli integer not null default 0,
  startup_command text not null default '',
  health_url text not null default '',
  log_path text not null default '',
  status text not null default 'active',
  sort_order integer not null default 0
);

create index if not exists idx_node_config_role_sort
  on node_config(role, sort_order, id);

create table if not exists workflow (
  id text primary key,
  name text not null default '',
  template_id text not null,
  template_config_id text not null,
  description text not null default '',
  status text not null default 'active',
  sort_order integer not null default 0
);

create index if not exists idx_workflow_template_sort
  on workflow(template_id, sort_order, id);

create table if not exists workflow_node (
  workflow_id text not null,
  node_id text not null,
  relation_type text not null default 'required',
  required integer not null default 1,
  sort_order integer not null default 0,
  primary key(workflow_id, node_id, relation_type)
);

create index if not exists idx_workflow_node_node
  on workflow_node(node_id, workflow_id);

create table if not exists interface_node (
  id text primary key,
  display_name text not null default '',
  service_id text not null default '',
  operation text not null default '',
  method text not null default '',
  path text not null default '',
  template_id text not null default '',
  version text not null default 'v1',
  status text not null default 'draft',
  tags_json text not null default '[]',
  description text not null default '',
  sort_order integer not null default 0,
  created_at text not null,
  updated_at text not null
);

create index if not exists idx_interface_node_service_operation
  on interface_node(service_id, operation, status);

create index if not exists idx_interface_node_template_sort
  on interface_node(template_id, sort_order, id);

create table if not exists interface_node_field (
  id text primary key,
  node_id text not null,
  direction text not null,
  field_path text not null,
  display_name text not null default '',
  data_type text not null default '',
  required integer not null default 0,
  bindable integer not null default 0,
  port_type text not null default 'DATA',
  status text not null default 'active',
  sort_order integer not null default 0
);

create index if not exists idx_interface_node_field_node_direction
  on interface_node_field(node_id, direction, sort_order, id);

create table if not exists interface_node_request_template (
  id text primary key,
  node_id text not null,
  name text not null default '',
  template_json text not null default '{}',
  version text not null default 'v1',
  status text not null default 'active',
  sort_order integer not null default 0,
  created_at text not null,
  updated_at text not null
);

create index if not exists idx_interface_node_request_template_node
  on interface_node_request_template(node_id, status, sort_order, id);

create table if not exists interface_node_case (
  id text primary key,
  node_id text not null,
  title text not null default '',
  case_type text not null,
  scenario text not null default '',
  payload_template_json text not null default '{}',
  request_template_id text not null default '',
  patch_json text not null default '[]',
  render_mode text not null default 'legacy_payload',
  expected_json text not null default '{}',
  required_for_admission integer not null default 1,
  status text not null default 'active',
  sort_order integer not null default 0,
  created_at text not null,
  updated_at text not null
);

create index if not exists idx_interface_node_case_node_type
  on interface_node_case(node_id, case_type, sort_order, id);

create table if not exists workflow_interface_node (
  workflow_id text not null,
  step_id text not null,
  node_id text not null,
  case_id text not null default '',
  required integer not null default 1,
  sort_order integer not null default 0,
  primary key(workflow_id, step_id)
);

create index if not exists idx_workflow_interface_node_node
  on workflow_interface_node(node_id, workflow_id);

create index if not exists idx_workflow_interface_node_case
  on workflow_interface_node(case_id, workflow_id);

create table if not exists fixture_profile (
  id text primary key,
  name text not null default '',
  source_type text not null default '',
  source_workflow_id text not null default '',
  source_until_step text not null default '',
  ttl_seconds integer not null default 0,
  status text not null default 'active',
  description text not null default '',
  sort_order integer not null default 0,
  created_at text not null,
  updated_at text not null
);

create index if not exists idx_fixture_profile_source
  on fixture_profile(source_type, source_workflow_id, source_until_step, status);

create table if not exists fixture_table_binding (
  id text primary key,
  profile_id text not null,
  schema_name text not null default '',
  table_name text not null default '',
  key_fields_json text not null default '[]',
  extract_sql text not null default '',
  apply_mode text not null default 'upsert',
  status text not null default 'active',
  sort_order integer not null default 0
);

create index if not exists idx_fixture_table_binding_profile
  on fixture_table_binding(profile_id, sort_order, id);

create table if not exists interface_node_case_dependency (
  id text primary key,
  case_id text not null,
  fixture_profile_id text not null,
  required integer not null default 1,
  mappings_json text not null default '[]',
  status text not null default 'active',
  sort_order integer not null default 0
);

create index if not exists idx_interface_node_case_dependency_case
  on interface_node_case_dependency(case_id, status, sort_order);

create index if not exists idx_interface_node_case_dependency_fixture
  on interface_node_case_dependency(fixture_profile_id, status);`,
		},
	}
}
