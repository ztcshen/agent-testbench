# AgentTestBench Black-Box Operator Runbook

## Defaults

- Project source, only when needed: `the current AgentTestBench checkout`
- Canonical operator skill:
  `./skills/agent-testbench-operator`
- Runtime/report root: `.runtime/operator-reports`
- Default helper Store: `local`

This public runbook is intentionally product-neutral. Do not add private
environment IDs, company Store names, internal service names, customer workflow
IDs, private DSNs, or business-domain examples. Use placeholders such as
`STORE_NAME`, `ENV_ID`, `WORKFLOW_ID`, `NODE_ID`, `OWNER:KIND`, and
`WORKSPACE`.

For shared validation, pass the desired Store explicitly:

```bash
./skills/agent-testbench-operator/scripts/atb.sh store status --store STORE_NAME --json
```

## CLI Freshness And Onboarding

Use:

```bash
./skills/agent-testbench-operator/scripts/atb.sh status --json
./skills/agent-testbench-operator/scripts/atb.sh doctor --json
```

The wrapper prefers `ATB_BIN`, then
`./.runtime/bin/agent-testbench`, then
`agent-testbench` on `PATH`, then the repo wrapper. If the runtime is missing
or stale, rebuild it with the current checkout:

```bash
./skills/agent-testbench-operator/scripts/atb.sh onboard \
  --repo "$(pwd)" \
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

Before stopping on a repeated or unrecoverable CLI-diagnosed blocker, preserve
it as durable feedback. Match it to an existing feedback item or register a new
one with the blocking command, workflow run id, failed step or case id, service
id, Store state, exact error, and next repair command. A chat-only summary is
not enough evidence for an AgentTestBench operator handoff.

## Environment Acceptance

Use explicit environment and workflow identifiers supplied by the user, Store,
or command discovery. Do not bake a private default into the public skill.

Discover environments and workflows:

```bash
./skills/agent-testbench-operator/scripts/atb.sh environment discover --store STORE_NAME --json
./skills/agent-testbench-operator/scripts/atb.sh workflow discover --store STORE_NAME --json
```

Plan a workflow:

```bash
./skills/agent-testbench-operator/scripts/atb.sh workflow plan --store STORE_NAME --workflow WORKFLOW_ID --json
```

Run environment acceptance only after the environment is explicit:

```bash
./skills/agent-testbench-operator/scripts/atb.sh serve --store STORE_NAME --host 127.0.0.1 --port PORT
./skills/agent-testbench-operator/scripts/atb.sh environment acceptance start ENV_ID --server-url http://127.0.0.1:PORT --request-id REQUEST_ID --evidence-dir OUTPUT_DIR --json
./skills/agent-testbench-operator/scripts/atb.sh environment acceptance report ENV_ID --server-url http://127.0.0.1:PORT --run RUN_ID --json
```

Report both the workflow result and extra health failures separately.

## API Case Suite

Use:

```bash
ATB_STORE=STORE_NAME ./skills/agent-testbench-operator/scripts/run-case-suite.sh
```

Useful environment overrides:

```bash
ATB_TIMEOUT_SECONDS=45 ATB_STORE=STORE_NAME ./skills/agent-testbench-operator/scripts/run-case-suite.sh
ATB_OUTPUT_DIR=/tmp/atb-suite ATB_STORE=STORE_NAME ./skills/agent-testbench-operator/scripts/run-case-suite.sh --node NODE_ID
```

The script runs `case suite report --status active`, writes HTML/JSON/JUnit
reports, and prints failure aggregation by node and error.

## Store-Backed Tasks, Watch, And Notify

Use `task run` for one-shot repeatable CLI actions:

```bash
./skills/agent-testbench-operator/scripts/atb.sh task run catalog-smoke \
  --store STORE_NAME \
  --command "commands --filter task --json" \
  --json
```

Use `task watch` for bounded polling. Prefer a finite `--limit` unless the user
explicitly asks for a long-running watch:

```bash
./skills/agent-testbench-operator/scripts/atb.sh task watch catalog-smoke \
  --store STORE_NAME \
  --command "commands --filter task --json" \
  --interval 30s \
  --limit 3 \
  --until success \
  --json
```

Inspect task state and logs from the Store:

```bash
./skills/agent-testbench-operator/scripts/atb.sh task list --store STORE_NAME --json
./skills/agent-testbench-operator/scripts/atb.sh task status catalog-smoke --store STORE_NAME --json
./skills/agent-testbench-operator/scripts/atb.sh task logs catalog-smoke --store STORE_NAME --json
```

Stop a long watch from another shell:

```bash
./skills/agent-testbench-operator/scripts/atb.sh task stop catalog-smoke --store STORE_NAME --json
```

Validate notification targets before wiring them to task runs:

```bash
./skills/agent-testbench-operator/scripts/atb.sh notify test \
  --file /tmp/agent-testbench-notify.jsonl \
  --message "AgentTestBench notification check" \
  --json
```

When a task uses `--notify-file` or `--notify-webhook`, treat notification
delivery failure as a task operation failure and report the failed channel.

## Manual CLI Patterns

Discover active cases:

```bash
./skills/agent-testbench-operator/scripts/atb.sh case discover --store STORE_NAME --status active --json
```

Check suite coverage after a run:

```bash
./skills/agent-testbench-operator/scripts/atb.sh case suite coverage --store STORE_NAME --status active --json
```

Inspect registered sandbox services without executing startup commands:

```bash
./skills/agent-testbench-operator/scripts/atb.sh sandbox service list --store STORE_NAME --json
./skills/agent-testbench-operator/scripts/atb.sh sandbox start --store STORE_NAME --dry-run --json
./skills/agent-testbench-operator/scripts/atb.sh sandbox start --store STORE_NAME --workflow WORKFLOW_ID --dry-run --json
```

Inspect or stop an Environment Catalog target without rerunning heavy restore:

```bash
./skills/agent-testbench-operator/scripts/atb.sh environment status ENV_ID --store STORE_NAME --workspace WORKSPACE --json
./skills/agent-testbench-operator/scripts/atb.sh environment stop ENV_ID --store STORE_NAME --workspace WORKSPACE --json
```

`environment status` materializes Store-backed compose/env files when needed and
uses `docker compose ps` only, with per-service state and an aggregate health
summary. It must not be treated as a restore or workflow verification run.
For long restore execution, prefer `environment restore --output-format
stream-json`; phase events identify `docker.prepare`,
`docker.compose.validate`, `docker.cleanup`, `docker.native-assets`,
`docker.compose.execute`, `docker.edge-assets`, `docker.health`, and
`workflow.acceptance`. If Docker restore or health does not pass, the workflow
acceptance phase must be skipped rather than started. While acceptance is still
running, watch `workflow.acceptance` waiting observations for the run id and
remaining timeout.
For `environment migration apply|baseline --output-format stream-json`, watch
`environment.migration` waiting observations to identify the active migration
asset while MySQL execution is still running.
If no recorded or discoverable Compose services are available, status fails
instead of reporting an empty success. `environment stop` defaults to `docker
compose stop SERVICE...` and preserves containers, volumes, and images; the
default path also requires recorded or discoverable services so it cannot widen
into an accidental whole-project stop. Use `--down --remove-orphans` only after
explicitly deciding to remove Compose-managed containers.
For restore cleanup, `--allow-destructive-docker-cleanup` is only the operator
approval step. Restore still blocks `docker compose down` when the cleanup
linkage proof is incomplete: project name, Store component graph, required
component services, and the full `fileProjection` report for compose/env/native
Compose file references must all line up.
When inspecting or bootstrapping an environment, use `fileProjection` to verify
that Compose `env_file`, config file, and secret file references discovered
inside Store-backed compose files also have Store-backed projection sources.

After any `sandbox start --json` pass, inspect the report's `runtime` block.
If `runtime.activeMatchesRuntime=false` or `runtime.fresh=false`, treat the
sandbox result as incomplete validation until `.runtime/bin/agent-testbench` is
rebuilt or the wrapper resolves to it. Report the active path, expected runtime
path, and repair command with the workflow result.

When a workflow-scoped sandbox start reports a service with an empty startup
command, treat the workflow as blocked. Repair the Store service entry with
`sandbox service register --id SERVICE_ID --startup-command ...`. If the
startup metadata is already recorded on an Environment Catalog component, copy
it back into the service registry with
`sandbox service register --id SERVICE_ID --from-environment ENV_ID`. Rerun the
environment restore path that owns the generated startup files and component
assets before claiming the workflow is startable.

Add a Store-first SQL edge migration for incremental DDL/ALTER work:

```bash
./skills/agent-testbench-operator/scripts/atb.sh environment migration add ENV_ID \
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
./skills/agent-testbench-operator/scripts/atb.sh environment migration plan ENV_ID --store STORE_NAME --edge OWNER:KIND --database DB_NAME --json
./skills/agent-testbench-operator/scripts/atb.sh environment migration apply ENV_ID --store STORE_NAME --edge OWNER:KIND --database DB_NAME --workspace WORKSPACE --execute --json
```

Use `environment migration baseline` instead of `apply` when the target
database already contains the schema and only needs history rows recorded.
When `environment restore --use-existing-containers` adopts an already-running
database, plain MySQL SQL bootstrap assets are skipped rather than reapplied.
Clean Docker restore projects bootstrap SQL from the Store into MySQL initdb
files before Compose starts. Convert repeatable changes to migration assets,
baseline existing schema, or rerun a clean restore when bootstrap SQL must be
replayed.
For Docker-native config/secret/env projections, Store asset `summary_json` may
carry projection metadata such as `{"dockerNative":{"fileMode":"0600"}}`; do not
patch generated workspace files by hand as the durable configuration.
Use `environment inspect --json` or `environment bootstrap --json` to review
`fileProjection` before restore. Missing entries mean a referenced Compose file
or env file is still local/summary-only and must be repaired with
`environment startup-file put`, a component config asset, or an explicit
environment package projection before claiming the environment is reproducible.

## Maintaining This Skill

- Keep this repository copy as the source of truth.
- Keep local `~/.codex/skills/agent-testbench-operator` as a symlink to this
  folder.
- Do not commit runtime evidence, generated reports, local databases,
  temporary runtime repair binaries, private environment IDs, private Store
  names, internal services, or customer workflow examples.
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
- Runtime evidence, reports, local databases, and temporary runtime repair
  binaries should not be committed.
- `environment migration apply` and `baseline` are dry-run unless `--execute`
  is supplied. Do not edit old bootstrap DDL for already-applied schemas; add a
  new migration version and let the target history table/checksum guard repeats.
