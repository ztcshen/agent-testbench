# Open Test Sandbox Agent Guide

Open Test Sandbox is a new open-source-oriented project. Keep the core generic,
API-operated, Store-first, and local-first.

## Core Rules

- Do not hardcode a concrete business domain into core packages.
- Source code and default core assets must not contain source-domain terms.
  Put domain-specific names and language only in private validation/config data.
- Treat test engineers as sandbox users, not external configuration maintainers.
  Day-to-day testing should be possible through sandbox APIs and UI discovery,
  with minimal one-time registration when a runtime or service must be known.
- PostgreSQL Store is the active source of truth for current sandbox configuration,
  runtime facts, workflow catalog, execution state, Evidence indexes, and
  verification results.
- Environment Catalog entries must be Store-first. Test engineers register,
  discover, inspect, bootstrap, verify, and publish verified environments through
  CLI/API/UI surfaces backed by the active Store or an explicit `--store
  NAME_OR_DSN`.
- An environment may enter the verified discovery list only after its acceptance
  workflow has passed and its Evidence plus real SkyWalking topology are
  complete.
- Portable template packages are optional artifacts for import, export, review,
  migration, and sharing. Do not introduce new mandatory file-package-first
  flows for normal testing.
- Prefer Store-first APIs and UI paths for new behavior. Add file-package
  adapters only as compatibility or import/export bridges.
- PostgreSQL is the default product Store for personal and team workflows.
- SQLite is retained only for legacy migration, compatibility, and tests.
- Runtime Evidence, logs, and local databases must not be committed.
- Prefer small, verifiable slices with tests and a commit per slice.
- Use headless/background verification for local browser checks.
- For any moderately large change, first do web research and ground the design
  in mature open-source projects before editing. This is mandatory when the
  change is expected to touch 3 or more files or exceed roughly 200 lines of
  code. Do not rely on pure inference to generate substantial architecture,
  API, persistence, migration, or workflow code.

## Project Shape

- `cmd/otsandbox/`: CLI entrypoint.
- `internal/`: future core packages.
- `docs/`: public docs.
- `tools/guardrails/`: local quality gates and repository checks.

Domain-specific validation data lives outside this core repository. If a
portable template package exists, it is imported into the local Store instead
of becoming the daily maintenance surface.

## Naming

The working product name is **Open Test Sandbox**. Keep names easy to change
until the first public release.
