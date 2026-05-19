# Store Backends

Open Test Sandbox treats the Store as a pluggable database backend. Users pick
one backend for a workspace or team, and daily CLI/API/UI commands operate
against the selected Store without changing command shape. The Store holds
environment catalogs, service and interface registrations, workflows, maintained
API cases, run records, Evidence indexes, trace topology indexes, baseline
gates, timing data, and post-process task state. Evidence files and local logs
may still live on disk, but their indexes belong in the selected Store.

## Backend Selection

The code path for opening a Store is centralized in `internal/store/open`, and
database-specific SQL differences live behind `internal/store/sqlstore`
dialects. Command handlers and the control-plane should depend on the
`store.Store` contract instead of importing a concrete database package
directly. This keeps PostgreSQL, MySQL, and SQLite compatibility from leaking
into daily workflow commands.

Supported backend families:

- PostgreSQL: active product path for personal and team Stores.
- MySQL: recognized backend family, reserved for organizations that require it;
  implementation is pending.
- SQLite: compatibility backend for migration, old local runs, and tests.

Dialect responsibilities:

- driver name and DSN handoff;
- bind variables, for example `$1` for PostgreSQL and `?` for MySQL/SQLite;
- identifier quoting;
- JSON/time/bool column types;
- upsert syntax and migration DDL differences.

The first shared DDL builder is `sqlstore.CoreSchemaSQL`. It generates the
core workflow tables for runs, API case runs, Evidence indexes, trace topology,
post-process tasks, and baseline gates with dialect-specific JSON/time/bool
types. New Store tables should follow this pattern instead of adding SQLite-only
SQL to the daily Store path.

## PostgreSQL First

Use one PostgreSQL database per isolation boundary:

- `local-personal`: a private database for unverified local work.
- `team-verified`: a shared database for verified environments and reusable
  cases.

Configure named Stores with:

```sh
otsandbox store config set local-personal --url postgres://user:pass@host:5432/otsandbox_local?sslmode=disable
otsandbox store config set team-verified --url postgres://user:pass@host:5432/otsandbox_team?sslmode=disable
otsandbox store use local-personal
otsandbox store current
```

Commands may also use `--store NAME_OR_DSN` for a one-off override. Legacy
`--store-url` remains accepted during migration.

The command shape is location-agnostic. A local PostgreSQL database and a
remote team PostgreSQL database use the same daily commands; only the selected
Store changes:

```sh
otsandbox store use local-personal
otsandbox case discover --filter refund

otsandbox store use team-verified
otsandbox case discover --filter refund

otsandbox case discover --store team-verified --filter refund
otsandbox workflow discover --store postgres://user:pass@host:5432/team_verified --filter checkout
```

## SQLite Compatibility

SQLite is no longer the product target for new daily workflows. It remains a
compatibility path for old local runs, legacy Evidence import, and tests that
exercise historical behavior while the PostgreSQL Store is being rolled in.

Do not add new daily testing behavior that only works with SQLite.

## PostgreSQL-only Validation

Release and environment verification can hard-disable SQLite Store usage:

```sh
OTSANDBOX_DISABLE_SQLITE_STORE=1 \
OTSANDBOX_SMOKE_STORE_DSN="postgres://user:pass@host:5432/otsandbox_smoke?sslmode=disable" \
npm run smoke:frontend
```

When this flag is set, any accidental SQLite Store open fails immediately.
This is the repeatable equivalent of taking the local SQLite path offline before
running the core workflow. The smoke must still complete through the configured
PostgreSQL Store.
