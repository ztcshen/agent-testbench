package schema

var assistantSessionChanges = []Change{{Version: 23, Name: "add Store-backed assistant conversations", SQL: `
create table if not exists assistant_sessions (
 id text primary key, owner_id text not null, map_id text not null,
 profile_id text not null, title text not null, status text not null,
 revision integer not null, state_json text not null,
 created_at text not null, updated_at text not null
);
create index if not exists idx_assistant_sessions_owner_map_updated
 on assistant_sessions(owner_id, map_id, updated_at);`}}
