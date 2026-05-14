# Continuous Iteration Outline

This outline tracks small verified slices for Open Test Sandbox. Keep tasks
generic, profile-driven, and local-first.

## Completed

- Store interface, SQLite backend, and contract tests.
- Store migrations and CLI status/migrate commands.
- Profile bundle loader, split asset directories, and empty profile.
- Generic Control plane for loaded profiles.
- External profile export fixture and profile import index.
- Runtime Evidence index import from a legacy SQLite source.
- Generic API Case dry-run and HTTP runner.
- API Case Store indexing.
- Local quickstart example.
- API Case format documentation.
- Control Plane profile asset list API.
- API Case Store summaries for requests, assertions, and response Evidence.
- Store backend URL boundary with SQLite as the default.
- Machine-readable Evidence import report.
- Workflow planning command.
- Request template rendering preview.
- Evidence query CLI.
- Baseline gate CLI.
- Release hygiene pass.

## Open Task Queue

### Task 1: Migration Source Audit Before Further Core Expansion

Goal:
- Stop greenfield drift by auditing the next core capability against the source
  repository before adding more behavior.

Acceptance:
- One `needs-audit` row in `docs/migration/source-map.md` is upgraded to
  `ported-and-scrubbed`, `reimplemented-with-rationale`, `new-core-only`, or
  `profile-only`.
- The source files and source tests inspected are named explicitly.
- The next implementation task is derived from source behavior or records why
  no source behavior applies.
- Core/profile separation remains intact.
- `go test ./...` and the source-domain scan pass after each slice.
