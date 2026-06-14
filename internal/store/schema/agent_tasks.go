// Package schema defines ordered Store schema migration changes.
package schema

var agentTaskChanges = []Change{
	{
		Version: 19,
		Name:    "add agent task registry",
		SQL: `
create table if not exists agent_tasks (
  id text primary key,
  name text not null unique,
  kind text not null default 'cli',
  command text not null,
  schedule text not null default '',
  status text not null default 'active',
  notify_json text not null default '{}',
  summary_json text not null default '{}',
  created_at text not null,
  updated_at text not null
);

create index if not exists idx_agent_tasks_status_updated
  on agent_tasks(status, updated_at, id);

create table if not exists agent_task_runs (
  id text primary key,
  task_id text not null,
  status text not null,
  command text not null default '',
  started_at text,
  finished_at text,
  duration_ms integer not null default 0,
  exit_code integer not null default 0,
  output text not null default '',
  error text not null default '',
  summary_json text not null default '{}',
  created_at text not null,
  foreign key (task_id) references agent_tasks(id) on delete cascade
);

create index if not exists idx_agent_task_runs_task_started
  on agent_task_runs(task_id, started_at desc, id);

create index if not exists idx_agent_task_runs_status_created
  on agent_task_runs(status, created_at, id);`,
	},
}
