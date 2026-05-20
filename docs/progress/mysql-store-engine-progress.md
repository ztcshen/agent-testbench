# MySQL Store Engine Progress

This ledger tracks the MySQL Store engine goal for Open Test Sandbox.

## 2026-05-20 Initial MySQL Store Engine Slice

Progress: `[##########----------] 50%`

Implemented:

- Added `internal/store/mysql` with explicit `mysql://` Store URL parsing, driver
  DSN conversion, schema status, schema upgrade, and Store open.
- Added `github.com/go-sql-driver/mysql` as the MySQL `database/sql` driver.
- Extended the shared SQL dialect with MySQL-safe key text columns and index DDL.
- Added MySQL DDL output through `otsandbox store ddl --backend mysql`.
- Allowed named MySQL Stores in `store config set`, `store use`, daily Store
  resolution, and `store status`/`store upgrade`.
- Kept SQLite rejected for daily paths; daily paths now allow PostgreSQL or
  MySQL Stores.
- Added optional external MySQL Store contract coverage behind
  `OTSANDBOX_MYSQL_TEST_DSN`.
- Updated smoke helpers so Store smoke selection accepts `postgres://`,
  `postgresql://`, or `mysql://`.

Validated:

- `go test ./internal/store/... -count=1`
- `node --test tools/smoke/control-plane-smoke.test.mjs tools/examples/api-case-demo.test.mjs`
- Manual CLI check:
  `OTSANDBOX_CONFIG_HOME=$(mktemp -d) go run ./cmd/otsandbox store config set local-mysql --url 'mysql://user:secret@example.com:3306/otsandbox_local?tls=false'`

Known gaps:

- `go mod tidy` was attempted but could not fetch transient test dependencies
  from `proxy.golang.org` in this network window; `go.mod`/`go.sum` already
  contain the MySQL driver entries needed by the current code.
- The heavy `cmd/otsandbox` CLI test subset is slow because each `runCLI`
  invocation rebuilds through `go run`; it timed out before producing useful
  signal. Kept this slice to Store-layer and smoke helper checks.
- No live MySQL database contract was run yet. To run it, provide
  `OTSANDBOX_MYSQL_TEST_DSN=mysql://USER:PASS@HOST:3306/mysql?...`.
