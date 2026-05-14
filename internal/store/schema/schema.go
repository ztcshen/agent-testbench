package schema

type Change struct {
	Version int
	Name    string
	SQL     string
}

const CurrentVersion = 1

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
	}
}
