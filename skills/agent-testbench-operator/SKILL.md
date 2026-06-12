---
name: agent-testbench-operator
description: Operate AgentTestBench as a mostly black-box local CLI/service for clean-machine onboarding, workflow runs, API case suites, Store inspection, Store-backed task/watch/notify operations, Docker-backed sandbox checks, Store-first SQL migrations, sandbox service discovery/dry-run, environment acceptance verification, and report collection. Use when the user asks to run AgentTestBench, test workflows, run interface cases, verify a sandbox, configure a local or team Store, schedule/watch CLI tasks, send notifications, add/plan/apply SQL migrations on component edges, inspect sandbox service registration, or hand another Codex session a source-light operating path. If source changes, refactors, GitHub PRs, or quality gates are requested, also use the AgentTestBench refactor guardrails skill.
---

# AgentTestBench Operator

## Source Of Truth

This skill is maintained in the AgentTestBench repository:

```bash
./skills/agent-testbench-operator
```

Local Codex installs should symlink to that directory instead of carrying a
separate copy. When CLI behavior, Store handling, task/watch/notify flows,
reports, or helper scripts change, update this skill in the same local slice or
GitHub PR.

Keep this public skill product-neutral. Do not add private environment IDs,
company Store names, customer workflows, internal service names, DSNs, ports, or
business-domain examples. Put private defaults in a separate local-only overlay.

## Operating Stance

Treat AgentTestBench as a CLI/service first. Do not read `cmd/`, `internal/`,
or other source files unless the CLI/service behavior is broken, the user asks
for implementation work, or a command's help/report is insufficient.

### Failure Boundary

For integration-test and sandbox-operation tasks, stay on the AgentTestBench
CLI path. Do not bypass the CLI by manually starting containers, editing
generated runtime files, patching temporary workspaces, querying implementation
databases directly, or reading AgentTestBench source to work around missing
features.

If the CLI cannot express the needed action, returns a misleading success
state, lacks a required workflow/service capability, or cannot recover a broken
sandbox without manual intervention, stop the current test session. Capture the
blocking command, the relevant JSON output, the impact, and the next capability
needed, then register feedback through the local AgentTestBench feedback skill
or its helper script. Report that human intervention or a product change is
needed before continuing.

Do not stop after a repeated or unrecoverable workflow/case failure with only a
chat summary. Before handing off, either match the blocker to an existing
feedback entry or add/update a durable feedback record. Include the workflow
run id, failed step or case id, service id, relevant Store state such as
`hasStartupCommand=false`, the exact CLI error, and the next repair command.
If the product behavior is already fixed but the local Store/runtime still
needs repair, record that operational follow-up instead of treating the
conversation as sufficient evidence.

Only leave this boundary when the user explicitly asks for AgentTestBench
implementation work or gives explicit permission to use a non-CLI recovery path
for the current turn. Even then, keep product feedback separate from the tested
project and avoid presenting manually repaired state as clean CLI evidence.

Use the repository-maintained operator wrapper:

```bash
./skills/agent-testbench-operator/scripts/atb.sh ...
```

The wrapper leaves `AGENT_TESTBENCH_CONFIG_HOME` to the CLI/default environment
and exports `ATB_STORE=local` only for helper scripts that need a Store name.
It prefers, in order: `ATB_BIN`, the latest repo runtime at
`./.runtime/bin/agent-testbench`, the
`agent-testbench` binary on `PATH`, and the repo wrapper at
`./bin/agent-testbench.sh`.

## Freshness Check

Before real work, confirm the wrapper is on the latest repo runtime:

```bash
./skills/agent-testbench-operator/scripts/atb.sh status --json
./skills/agent-testbench-operator/scripts/atb.sh doctor --json
```

If the runtime binary is missing or stale, rebuild and optionally install the
bare shell command from the current checkout:

```bash
./skills/agent-testbench-operator/scripts/atb.sh onboard --repo "$(pwd)" --store local --build-runtime --install-shell --smoke commands --json
```

For team/shared validation, use the Store name or DSN explicitly:

```bash
./skills/agent-testbench-operator/scripts/atb.sh store use STORE_NAME
```

## Quick Commands

Confirm Store and Docker before running real tests:

```bash
./skills/agent-testbench-operator/scripts/atb.sh store current --json
docker info
```

Run all active interface cases in the selected Store:

```bash
ATB_STORE=STORE_NAME ./skills/agent-testbench-operator/scripts/run-case-suite.sh
```

Run or watch repeatable Store-backed CLI tasks:

```bash
./skills/agent-testbench-operator/scripts/atb.sh task run catalog-smoke --store STORE_NAME --command "commands --filter task --json" --json
./skills/agent-testbench-operator/scripts/atb.sh task run sandbox-trigger --store STORE_NAME --shell --command "docker exec CONTAINER kafka-console-producer.sh --bootstrap-server HOST:PORT --topic TOPIC < MESSAGE_FILE" --json
./skills/agent-testbench-operator/scripts/atb.sh task watch catalog-smoke --store STORE_NAME --command "commands --filter task --json" --interval 30s --limit 3 --until success --json
./skills/agent-testbench-operator/scripts/atb.sh task logs catalog-smoke --store STORE_NAME --json
```

Send a notification test before wiring a task to notifications:

```bash
./skills/agent-testbench-operator/scripts/atb.sh notify test --file /tmp/agent-testbench-notify.jsonl --message "AgentTestBench notification check" --json
```

Inspect sandbox service registration without starting services:

```bash
./skills/agent-testbench-operator/scripts/atb.sh sandbox service list --store STORE_NAME --json
./skills/agent-testbench-operator/scripts/atb.sh workflow discover --store STORE_NAME --service SERVICE_ID --json
./skills/agent-testbench-operator/scripts/atb.sh sandbox start --store STORE_NAME --dry-run --json
./skills/agent-testbench-operator/scripts/atb.sh environment discover --store STORE_NAME --all --json
./skills/agent-testbench-operator/scripts/atb.sh environment bootstrap ENV_ID --store STORE_NAME --json
```

For long Docker-backed startup or restore execution, prefer the agent event
stream so the CLI emits one machine-readable event per line while work is still
running:

```bash
./skills/agent-testbench-operator/scripts/atb.sh environment restore ENV_ID --store STORE_NAME --workspace WORKSPACE --execute --output-format stream-json
./skills/agent-testbench-operator/scripts/atb.sh environment restore ENV_ID --store STORE_NAME --workspace WORKSPACE --execute --run-workflow --server-url SERVER_URL --output-format stream-json
./skills/agent-testbench-operator/scripts/atb.sh environment status ENV_ID --store STORE_NAME --workspace WORKSPACE --json
./skills/agent-testbench-operator/scripts/atb.sh environment stop ENV_ID --store STORE_NAME --workspace WORKSPACE --json
./skills/agent-testbench-operator/scripts/atb.sh environment migration apply ENV_ID --store STORE_NAME --edge OWNER:PROVIDER --database DB_NAME --workspace WORKSPACE --execute --output-format stream-json
```

During `environment restore --output-format stream-json`, watch the
`docker.prepare`, `docker.compose.validate`, `docker.cleanup`,
`docker.native-assets`, `docker.compose.execute`, `docker.edge-assets`,
`docker.health`, and `workflow.acceptance` phases. If Docker restore or health
does not pass, `--run-workflow` must remain skipped instead of invoking the
acceptance workflow. While an acceptance run is still running, the stream emits
`workflow.acceptance` waiting observations with the acceptance run id.

`environment status` is a read-only Compose inspection path: it can materialize
Store-backed compose/env files, then uses `docker compose ps` without
pull/build/up/down. `environment stop` defaults to `docker compose stop
SERVICE...`; both commands require recorded or discoverable Compose services
for their default service-scoped behavior.
Destructive restore cleanup still requires more than
`--allow-destructive-docker-cleanup`: the cleanup linkage proof must show a
recorded Compose project name, Store component graph, required component
services, and Store-projected compose/env files before `docker compose down`
can run.

Plan or apply Store-first SQL edge migrations:

```bash
./skills/agent-testbench-operator/scripts/atb.sh environment migration plan ENV_ID --store STORE_NAME --edge OWNER:KIND --database DB_NAME --json
./skills/agent-testbench-operator/scripts/atb.sh environment migration apply ENV_ID --store STORE_NAME --edge OWNER:KIND --database DB_NAME --workspace WORKSPACE --execute --json
```

When adopting already-running containers with `environment restore
--use-existing-containers`, plain MySQL SQL bootstrap assets are intentionally
not reapplied. During clean Docker restore, bootstrap SQL is projected from the
Store into MySQL initdb files before Compose starts. Use `environment migration
add/plan/apply` or `environment migration baseline` for incremental SQL work
against an existing database; use a clean restore when bootstrap SQL must be
replayed from scratch.
For Docker-native config/secret/env projections, Store asset `summary_json` may
carry projection metadata such as `{"dockerNative":{"fileMode":"0600"}}`; do not
patch generated workspace files by hand as the durable configuration.
When inspecting or bootstrapping an environment, check the JSON `fileProjection`
report. A referenced compose or env file is not repair-complete until it is
backed by `compose.generatedFiles`, a component config asset, generated Compose
env metadata, or an explicit environment package projection; summary-only
`startupFiles` entries are repair hints, not durable file content.

## Report Back

Always report:

- Store name/backend from `store current --json`.
- Runtime freshness: whether the active shell command points at the repo
  runtime.
- Sandbox runtime evidence: after any `sandbox start --json` pass, inspect the
  report's `runtime.activeMatchesRuntime` and `runtime.fresh` fields; treat the
  validation as incomplete when either is false until the runtime is rebuilt or
  the wrapper points at `.runtime/bin/agent-testbench`.
- Workflow, suite, or task counts: total, passed, failed, not-run when
  available.
- Exact report paths for HTML, JSON, JUnit, task logs, or notification JSONL.
- Whether failures are workflow failures, assertion mismatches, dependency
  health failures, notification delivery failures, or CLI/service errors.

## References

Read `references/operator-runbook.md` for the full black-box runbook, command
patterns, environment-neutral defaults, task/watch/notify guidance, and when
source inspection is allowed.
