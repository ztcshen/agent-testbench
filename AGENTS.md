# Open Test Sandbox Agent Guide

Open Test Sandbox is a new open-source-oriented project. Keep the core generic,
profile-driven, and local-first.

## Core Rules

- Migration-first, not greenfield-first: before implementing or expanding a
  core capability, inspect the corresponding source capability named in
  `docs/migration/source-map.md`.
- Every migrated capability must be classified as one of:
  `ported-and-scrubbed`, `reimplemented-with-rationale`, `new-core-only`, or
  `profile-only`.
- A from-scratch implementation is allowed only when the source implementation
  is too domain-specific or coupled to runtime state; record the reason in the
  source map before or in the same commit as the implementation.
- Prefer extracting behavior, contracts, tests, and data models from the source
  repository over inventing replacement behavior.
- Do not hardcode a concrete business domain into core packages.
- Source code and default core assets must not contain source-domain terms.
  Put domain-specific names and language only in profile/config bundles or
  migration-only documents.
- Keep template configuration as reviewable files first; databases are indexes
  and runtime stores.
- SQLite is the default local Store.
- PostgreSQL is optional for team or hosted mode.
- Runtime Evidence, logs, and local databases must not be committed.
- Prefer small, verifiable slices with tests and a commit per slice.
- Use headless/background verification for local browser checks.

## Project Shape

- `cmd/otsandbox/`: CLI entrypoint.
- `internal/`: future core packages.
- `profiles/`: future profile bundles.
- `docs/`: public docs and migration notes.
- `tools/migration/`: one-time and repeatable migration helpers.

## Naming

The working product name is **Open Test Sandbox**. Keep names easy to change
until the first public release.

## Migration Workflow

For each non-trivial core slice:

1. Identify the source feature, source files, and source tests in
   the source repository named by the migration source map.
2. Update `docs/migration/source-map.md` with the chosen migration
   classification and rationale.
3. Implement the smallest neutral slice.
4. Run `go test ./...` and the source-domain scan.
5. Commit the code and source-map update together.

If no source feature exists, mark the slice `new-core-only` and explain why the
new behavior belongs in the open core.
