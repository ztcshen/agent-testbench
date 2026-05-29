# MySQL Edge Migrations

## Decision

AgentTestBench manages consumer-owned MySQL schema changes as Store-first
component edge migration assets. The project keeps the source of truth in
`component_config_assets` and records execution state in the target MySQL
database, not in a separate migration directory.

This follows the core pattern used by mature migration tools: ordered versions,
checksums, a target-side history table, dry-run planning, and explicit baseline.
The first implementation supports MySQL edge assets only; online DDL executors
such as pt-online-schema-change or gh-ost are future policy extensions.

## Shape

- Migration assets use `assetKind=mysql-migration-sql`.
- `summaryJson.migration` stores version, description, database, checksum, and
  optional preconditions.
- The dependency edge's `profileJson.assetIds` links the migration asset to the
  consumer-to-provider edge.
- The target database owns `agent_testbench_schema_history`, keyed by
  environment, owner component, provider component, database, and version.
- `environment migration add/list/plan/apply/baseline` is the operator surface.
- `environment restore --execute` applies versioned migration assets through
  the same history/checksum/precondition path.

## Safety Rules

- Already-applied versions are skipped only when their checksum matches.
- Checksum drift fails instead of re-running changed SQL.
- `apply` and `baseline` are dry-run by default and require `--execute` for
  target writes.
- `column-not-exists:TABLE.COLUMN` preconditions are checked through
  `information_schema.COLUMNS` before running ALTER SQL.
- Baseline records existing schemas without running their SQL.
