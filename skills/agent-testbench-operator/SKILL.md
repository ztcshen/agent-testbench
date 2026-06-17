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

SQLite, PostgreSQL, and MySQL are Store engines for the same logical SQL Store
model. SQLite is the local single-file engine; do not treat it as a separate
legacy table model when inspecting status, running workflows, or collecting
Store evidence.

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

Build and inspect a Store-backed workflow map without running target services:

```bash
./skills/agent-testbench-operator/scripts/atb.sh map import-workflows --store STORE_NAME --json
./skills/agent-testbench-operator/scripts/atb.sh map workflows --store STORE_NAME --map MAP_ID --filter TEXT --json
./skills/agent-testbench-operator/scripts/atb.sh map explain --store STORE_NAME --map MAP_ID --case CASE_ID --json
./skills/agent-testbench-operator/scripts/atb.sh map explain --store STORE_NAME --map MAP_ID --scope all --environment ENV_ID --save --json
./skills/agent-testbench-operator/scripts/atb.sh map run --store STORE_NAME --map MAP_ID --scope all --environment ENV_ID --json
./skills/agent-testbench-operator/scripts/atb.sh map run explain --store STORE_NAME --plan PLAN_ID --json
./skills/agent-testbench-operator/scripts/atb.sh map review-html --store STORE_NAME --map MAP_ID --filter TEXT --output /tmp/map-review.html --json
```

Create a new map for one coherent capability or acceptance surface when related
workflows share setup, fixtures, state transitions, or interface cases. Do not
create one map per workflow or one map per negative case. Use a separate map
only when the workflows belong to a different profile, runtime boundary,
environment contract, or independent capability whose preconditions should not
be planned together.

Build the map from Store catalog assets first: `Workflow` becomes a named path,
`WorkflowBinding` supplies ordered path steps, `APICase` supplies request
nodes, and `Fixture`/`CaseDependency` supply materialized preconditions. Re-run
`map import-workflows --map MAP_ID` after catalog changes to replace that map
projection. Use `map workflows --map MAP_ID --filter TEXT` to find workflow
paths by path id, workflow id, or display name. Use `map explain` as the
SQL-style planner: it emits logical plan, rule trace, candidate plans,
physical task DAG, task edges, cost, properties, and compatibility `operations`
for single-case replay. Use `--scope all|workflows|cases` for map-level
planning, `--case`/`--node` for a single target, and `--save` to persist the
planner instance into `test_map_plan_instances`, `test_map_plan_tasks`, and
`test_map_plan_task_edges`. Use `map run --map MAP_ID --scope all` only after
the explain output is reviewable: it creates a `mode=run` planner instance,
executes the physical task DAG serially, writes each task status and child
workflow/API case run id back to Store, and links child runs through
test-plan metadata. Use `map run explain --plan PLAN_ID` to inspect a run plan
without re-running it. Use `map review-html --map MAP_ID --filter TEXT
--output PATH` when a human needs to review the Store-backed map visually: the
generated HTML embeds the current map facts, can be narrowed to matching
workflows/cases, supports workflow filtering/search, and shows clickable case
details including request template, patch/expected JSON, reuse paths, and
planner replay operations.

For compose-backed sandbox services, `sandbox start --json` reports
`recoveryCommand`, `readiness`, and `warning` on each service result when
applicable. If a registered `docker start CONTAINER` command fails because the
container is missing and the service has a recorded Compose service,
AgentTestBench retries with `docker compose up -d SERVICE`. After a Compose
startup command, the CLI checks `docker compose ps -a --format json SERVICE`;
without `healthUrl`, treat `readiness=compose-service-running` as container
evidence only, not full application readiness. Add a `healthUrl` to turn a
start pass into an application-readiness check.

For long Docker-backed startup or restore execution, prefer the agent event
stream so the CLI emits one machine-readable event per line while work is still
running:

```bash
./skills/agent-testbench-operator/scripts/atb.sh environment restore ENV_ID --store STORE_NAME --workspace WORKSPACE --execute --output-format stream-json
./skills/agent-testbench-operator/scripts/atb.sh environment restore ENV_ID --store STORE_NAME --workspace WORKSPACE --execute --run-workflow --server-url SERVER_URL --output-format stream-json
./skills/agent-testbench-operator/scripts/atb.sh environment status ENV_ID --store STORE_NAME --workspace WORKSPACE --json
./skills/agent-testbench-operator/scripts/atb.sh environment stop ENV_ID --store STORE_NAME --workspace WORKSPACE --json
./skills/agent-testbench-operator/scripts/atb.sh environment service restart ENV_ID --store STORE_NAME --workspace WORKSPACE --service SERVICE_OR_COMPONENT --json
./skills/agent-testbench-operator/scripts/atb.sh environment migration apply ENV_ID --store STORE_NAME --edge OWNER:PROVIDER --database DB_NAME --workspace WORKSPACE --execute --output-format stream-json
```

For Store-backed API cases, prefer updating the config selected by the current
runner. `case config upsert --case CASE_ID` now updates that selected config
when present; use `--config-id` only when intentionally targeting another
template config. Add signed or gateway-specific request metadata with repeated
`--header KEY=VALUE`, `--headers-json`, `--auth-json`, `--signed`, and
`--trace-endpoint`. Before running a suite, `case suite inspect --json` includes
`serviceId`, `serviceReady`, and `serviceIssues`; a case can be runnable but
blocked when its node's service has no startup command or health URL. Workflow
batch start fails fast if any selected workflow binding cannot be converted into
a runnable case plan, instead of silently running only the later steps. When a
failed batch has no indexed case Evidence, use `case diagnose --run RUN_ID
--case-id CASE_ID --json`; the report points back to the batch report instead
of crashing or hiding the missing Evidence.

During `environment restore --output-format stream-json`, watch the
`environment.restore.plan`, `docker.prepare`, `docker.compose.validate`,
`docker.cleanup`, `docker.native-assets`, `docker.compose.execute`,
`docker.edge-assets`, `docker.health`, and `workflow.acceptance` phases. If
Docker restore or health does not pass, `--run-workflow` must remain skipped
instead of invoking the acceptance workflow. While restore report construction
or Docker preflight is still running before the first Docker action, the stream
emits `environment.restore.plan` waiting observations and returns a structured
timeout report if that phase exceeds its bounded watchdog. While Docker Compose
startup commands are still running, the stream emits `docker.compose.execute`
waiting observations for the active command. While Docker health probes are
still waiting, the stream emits `docker.health` waiting observations with the
current probe target and remaining time. While an acceptance run is still
running, the stream emits `workflow.acceptance` waiting observations with the
acceptance run id.
During `environment migration apply|baseline --output-format stream-json`, the
stream emits `environment.migration` waiting observations for the active
migration asset while MySQL execution is still running.

`environment status` is a read-only Compose inspection path: it can materialize
Store-backed compose/env files, then uses `docker compose ps` without
pull/build/up/down. `environment stop` defaults to `docker compose stop
SERVICE...`; both commands require recorded or discoverable Compose services
for their default service-scoped behavior. Use `environment service restart
ENV_ID --service SERVICE_OR_COMPONENT --workspace WORKSPACE` when a running app
only needs a cache/config refresh; `--service` accepts either a recorded Compose
service or a component id that maps to one through the Store component graph,
then waits for the scoped service health evidence. `environment stop --down` is
a destructive Compose operation and is blocked unless the same Store-to-Compose
linkage proof used by restore cleanup passes.
Destructive restore cleanup still requires more than
`--allow-destructive-docker-cleanup`: the cleanup linkage proof must show a
recorded Compose project name, Store component graph, required component
services, Store-backed Compose env injection, and a complete `fileProjection`
report for compose/env/native Compose file references before `docker compose
down` can run. Inspect `docker.cleanup.linkage.envInjection` to confirm
Store-backed `compose.env` keys and generated env files used by the Compose
command. When cleanup is blocked, read `docker.cleanup.linkage.repairPlan`;
each item names the missing Store-backed fact and a command hint for repairing
the Store metadata or file projection before retrying.

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
report. A referenced compose file, Compose `env_file`, config file, or secret
file is not repair-complete until it is backed by structured
`environment_files`, a component config asset, generated Compose env metadata,
legacy `compose.generatedFiles`, or an explicit environment package projection;
summary-only `startupFiles` entries are repair hints, not durable file content.
An `environment_files` row is repair-complete only when it carries materialized
inline content. Explicitly stored empty files are valid, but reference-only rows
without materialized content must still be treated as projection gaps.
New environment registration stores service repositories and health checks as
structured `environment_services` and `environment_health_checks`; legacy
`services_json`, `repos_json`, and `health_checks_json` are compatibility views.
Restore consumes structured environment files, services, and health checks
first. During migration or Store copy, structured rows replace matching legacy
entries by path, service id, health-check id, or equivalent health probe; legacy
entries that are not represented structurally remain compatibility inputs for older
rows and imports.
Dynamic Compose file paths, including nested Compose defaults such as
`${A:-${B:-file.env}}`, are resolved only from Store-backed `compose.env` and
Store-backed `compose.envFiles`. Absolute or
home-directory paths remain blocking projection gaps even when they come from
Store-backed env values, because they depend on a host-local file instead of
Store-backed projection. `extends.file` scans only the named `extends.service`
instead of every service in the referenced file. When
`fileProjection.ok=false`, read `fileProjection.repairPlan`; it groups blocking
Store repairs for summary-only startup files, unresolved Compose variables, and
unprojected Compose env/config/secret/include/extends file references.

## Report Back

Always report:

- Store name/backend from `store current --json`.
- Runtime freshness: whether the active shell command points at the repo
  runtime.
- Sandbox runtime evidence: after any `sandbox start --json` pass, inspect the
  report's `runtime.activeMatchesRuntime` and `runtime.fresh` fields; treat the
  validation as incomplete when either is false until the runtime is rebuilt or
  the wrapper points at `.runtime/bin/agent-testbench`.
- Sandbox service readiness evidence: inspect each service's `readiness`,
  `recoveryCommand`, and `warning`; when a service lacks `healthUrl`, a Compose
  running-state pass is not the same as end-to-end application readiness.
- Workflow, suite, or task counts: total, passed, failed, not-run when
  available.
- Exact report paths for HTML, JSON, JUnit, task logs, or notification JSONL.
- Whether failures are workflow failures, assertion mismatches, dependency
  health failures, notification delivery failures, or CLI/service errors.

## References

Read `references/operator-runbook.md` for the full black-box runbook, command
patterns, environment-neutral defaults, task/watch/notify guidance, and when
source inspection is allowed.
