// Package schema defines ordered Store schema migration changes.
package schema

var environmentDockerChanges = []Change{
	{
		Version: 20,
		Name:    "add environment docker file registry",
		SQL: `
create table if not exists environment_files (
  env_id text not null,
  file_path text not null,
  file_kind text not null,
  content_inline text not null default '',
  required integer not null default 1,
  apply_order integer not null default 0,
  summary_json text not null default '{}',
  created_at text not null,
  updated_at text not null,
  primary key (env_id, file_path, file_kind),
  foreign key (env_id) references environments(id) on delete cascade
);

create index if not exists idx_environment_files_kind_order
  on environment_files(env_id, file_kind, apply_order, file_path);`,
	},
	{
		Version: 21,
		Name:    "add environment docker service and health registries",
		SQL: `
create table if not exists environment_services (
  env_id text not null,
  service_id text not null,
  repo_url text not null default '',
  branch text not null default '',
  ref text not null default '',
  checkout text not null default '',
  summary_json text not null default '{}',
  created_at text not null,
  updated_at text not null,
  primary key (env_id, service_id),
  foreign key (env_id) references environments(id) on delete cascade
);

create index if not exists idx_environment_services_repo
  on environment_services(env_id, repo_url, service_id);

create table if not exists environment_health_checks (
  env_id text not null,
  check_id text not null,
  check_kind text not null,
  url text not null default '',
  address text not null default '',
  command text not null default '',
  compose_service text not null default '',
  expect text not null default '',
  apply_order integer not null default 0,
  summary_json text not null default '{}',
  created_at text not null,
  updated_at text not null,
  primary key (env_id, check_id),
  foreign key (env_id) references environments(id) on delete cascade
);

create index if not exists idx_environment_health_checks_kind_order
  on environment_health_checks(env_id, check_kind, apply_order, check_id);`,
	},
}
