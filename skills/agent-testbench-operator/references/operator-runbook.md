# AgentTestBench Black-Box Operator Runbook

## Defaults

- Project source, only when needed: `/Users/zlh/codes/agent-testbench`
- Canonical operator skill:
  `/Users/zlh/codes/agent-testbench/skills/agent-testbench-operator`
- Runtime/report root: `.runtime/operator-reports`
- Default helper Store: `local`

This public runbook is intentionally product-neutral. Do not add private
environment IDs, company Store names, internal service names, customer workflow
IDs, private DSNs, or business-domain examples. Use placeholders such as
`STORE_NAME`, `ENV_ID`, `WORKFLOW_ID`, `NODE_ID`, `OWNER:KIND`, and
`WORKSPACE`.

For shared validation, pass the desired Store explicitly:

```bash
ATB_STORE=STORE_NAME /Users/zlh/codes/agent-testbench/skills/agent-testbench-operator/scripts/atb.sh store current --json
```

## CLI Freshness And Onboarding

Use:

```bash
/Users/zlh/codes/agent-testbench/skills/agent-testbench-operator/scripts/atb.sh status --json
/Users/zlh/codes/agent-testbench/skills/agent-testbench-operator/scripts/atb.sh doctor --json
```

The wrapper prefers `ATB_BIN`, then
`/Users/zlh/codes/agent-testbench/.runtime/bin/agent-testbench`, then
`agent-testbench` on `PATH`, then the repo wrapper. If the runtime is missing
or stale, rebuild it with the current checkout:

```bash
/Users/zlh/codes/agent-testbench/skills/agent-testbench-operator/scripts/atb.sh onboard \
  --repo /Users/zlh/codes/agent-testbench \
  --store local \
  --build-runtime \
  --install-shell \
  --smoke commands \
  --json
```

For a clean machine, start with `--store local`. Switch to a team Store only
when the user gives a Store name or DSN.

## Source-Light Rule

Start with CLI help, JSON reports, generated HTML reports, Docker status, and
service logs. Inspect source only for one of these reasons:

- The CLI command itself crashes or lacks a needed capability.
- The user asks to change AgentTestBench code.
- Reports contradict the observed runtime behavior.
- A bug cannot be explained from CLI output, Store metadata, evidence, or logs.

When source inspection is needed, keep it narrow and explain why.

## Environment Acceptance

Use explicit environment and workflow identifiers supplied by the user, Store,
or command discovery. Do not bake a private default into the public skill.

Discover environments and workflows:

```bash
/Users/zlh/codes/agent-testbench/skills/agent-testbench-operator/scripts/atb.sh environment discover --store STORE_NAME --json
/Users/zlh/codes/agent-testbench/skills/agent-testbench-operator/scripts/atb.sh workflow discover --store STORE_NAME --json
```

Plan a workflow:

```bash
/Users/zlh/codes/agent-testbench/skills/agent-testbench-operator/scripts/atb.sh workflow plan --store STORE_NAME --workflow WORKFLOW_ID --json
```

Run environment acceptance only after the environment is explicit:

```bash
/Users/zlh/codes/agent-testbench/skills/agent-testbench-operator/scripts/atb.sh serve --store STORE_NAME --host 127.0.0.1 --port PORT
/Users/zlh/codes/agent-testbench/skills/agent-testbench-operator/scripts/atb.sh environment acceptance start ENV_ID --server-url http://127.0.0.1:PORT --request-id REQUEST_ID --evidence-dir OUTPUT_DIR --json
/Users/zlh/codes/agent-testbench/skills/agent-testbench-operator/scripts/atb.sh environment acceptance report ENV_ID --server-url http://127.0.0.1:PORT --run RUN_ID --json
```

Report both the workflow result and extra health failures separately.

## API Case Suite

Use:

```bash
ATB_STORE=STORE_NAME /Users/zlh/codes/agent-testbench/skills/agent-testbench-operator/scripts/run-case-suite.sh
```

Useful environment overrides:

```bash
ATB_TIMEOUT_SECONDS=45 ATB_STORE=STORE_NAME /Users/zlh/codes/agent-testbench/skills/agent-testbench-operator/scripts/run-case-suite.sh
ATB_OUTPUT_DIR=/tmp/atb-suite ATB_STORE=STORE_NAME /Users/zlh/codes/agent-testbench/skills/agent-testbench-operator/scripts/run-case-suite.sh --node NODE_ID
```

The script runs `case suite report --status active`, writes HTML/JSON/JUnit
reports, and prints failure aggregation by node and error.

## Store-Backed Tasks, Watch, And Notify

Use `task run` for one-shot repeatable CLI actions:

```bash
/Users/zlh/codes/agent-testbench/skills/agent-testbench-operator/scripts/atb.sh task run catalog-smoke \
  --store STORE_NAME \
  --command "commands --filter task --json" \
  --json
```

Use `task watch` for bounded polling. Prefer a finite `--limit` unless the user
explicitly asks for a long-running watch:

```bash
/Users/zlh/codes/agent-testbench/skills/agent-testbench-operator/scripts/atb.sh task watch catalog-smoke \
  --store STORE_NAME \
  --command "commands --filter task --json" \
  --interval 30s \
  --limit 3 \
  --until success \
  --json
```

Inspect task state and logs from the Store:

```bash
/Users/zlh/codes/agent-testbench/skills/agent-testbench-operator/scripts/atb.sh task list --store STORE_NAME --json
/Users/zlh/codes/agent-testbench/skills/agent-testbench-operator/scripts/atb.sh task status catalog-smoke --store STORE_NAME --json
/Users/zlh/codes/agent-testbench/skills/agent-testbench-operator/scripts/atb.sh task logs catalog-smoke --store STORE_NAME --json
```

Stop a long watch from another shell:

```bash
/Users/zlh/codes/agent-testbench/skills/agent-testbench-operator/scripts/atb.sh task stop catalog-smoke --store STORE_NAME --json
```

Validate notification targets before wiring them to task runs:

```bash
/Users/zlh/codes/agent-testbench/skills/agent-testbench-operator/scripts/atb.sh notify test \
  --file /tmp/agent-testbench-notify.jsonl \
  --message "AgentTestBench notification check" \
  --json
```

When a task uses `--notify-file` or `--notify-webhook`, treat notification
delivery failure as a task operation failure and report the failed channel.

## Manual CLI Patterns

Discover active cases:

```bash
/Users/zlh/codes/agent-testbench/skills/agent-testbench-operator/scripts/atb.sh case discover --store STORE_NAME --status active --json
```

Check suite coverage after a run:

```bash
/Users/zlh/codes/agent-testbench/skills/agent-testbench-operator/scripts/atb.sh case suite coverage --store STORE_NAME --status active --json
```

Inspect registered sandbox services without executing startup commands:

```bash
/Users/zlh/codes/agent-testbench/skills/agent-testbench-operator/scripts/atb.sh sandbox service list --store STORE_NAME --json
/Users/zlh/codes/agent-testbench/skills/agent-testbench-operator/scripts/atb.sh sandbox start --store STORE_NAME --dry-run --json
```

Add a Store-first SQL edge migration for incremental DDL/ALTER work:

```bash
/Users/zlh/codes/agent-testbench/skills/agent-testbench-operator/scripts/atb.sh environment migration add ENV_ID \
  --store STORE_NAME \
  --edge OWNER:KIND \
  --database DB_NAME \
  --version 0011 \
  --description add-column-name \
  --precondition column-not-exists:TABLE.COLUMN \
  --file ./V0011__add_column_name.sql \
  --json
```

Review before writes, then execute against a restored workspace:

```bash
/Users/zlh/codes/agent-testbench/skills/agent-testbench-operator/scripts/atb.sh environment migration plan ENV_ID --store STORE_NAME --edge OWNER:KIND --database DB_NAME --json
/Users/zlh/codes/agent-testbench/skills/agent-testbench-operator/scripts/atb.sh environment migration apply ENV_ID --store STORE_NAME --edge OWNER:KIND --database DB_NAME --workspace WORKSPACE --execute --json
```

Use `environment migration baseline` instead of `apply` when the target
database already contains the schema and only needs history rows recorded.

## Maintaining This Skill

- Keep this repository copy as the source of truth.
- Keep local `~/.codex/skills/agent-testbench-operator` as a symlink to this
  folder.
- Do not commit runtime evidence, generated reports, local databases, fallback
  binaries, private environment IDs, private Store names, internal services, or
  customer workflow examples.
- Update `SKILL.md`, this runbook, and helper scripts whenever CLI/operator
  behavior changes.
- Keep helper script JSON output and report paths stable enough for another
  Codex session to consume.

## Common Pitfalls

- Do not pass a single `--base-url` for workflows with callback steps unless the
  user explicitly wants that. Store-defined per-case URLs are needed because
  different steps can use different services.
- `workflow report` has a short synchronous per-step timeout and can fail on
  slow target paths. Prefer acceptance commands with explicit timeouts.
- Docker must be running before restore or live case execution.
- Runtime evidence, reports, local databases, and fallback binaries should not
  be committed.
- `environment migration apply` and `baseline` are dry-run unless `--execute`
  is supplied. Do not edit old bootstrap DDL for already-applied schemas; add a
  new migration version and let the target history table/checksum guard repeats.
