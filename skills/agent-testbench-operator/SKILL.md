---
name: agent-testbench-operator
description: Operate AgentTestBench as a mostly black-box local CLI/service for clean-machine onboarding, workflow runs, API case suites, Store inspection, Store-backed task/watch/notify operations, Docker-backed sandbox checks, Store-first SQL migrations, sandbox service discovery/dry-run, environment acceptance verification, and report collection. Use when the user asks to run AgentTestBench, test workflows, run interface cases, verify a sandbox, configure a local or team Store, schedule/watch CLI tasks, send notifications, add/plan/apply SQL migrations on component edges, inspect sandbox service registration, or hand another Codex session a source-light operating path. If source changes, refactors, GitHub PRs, or quality gates are requested, also use the AgentTestBench refactor guardrails skill.
---

# AgentTestBench Operator

## Source Of Truth

This skill is maintained in the AgentTestBench repository:

```bash
/Users/zlh/codes/agent-testbench/skills/agent-testbench-operator
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

Use the repository-maintained operator wrapper:

```bash
/Users/zlh/codes/agent-testbench/skills/agent-testbench-operator/scripts/atb.sh ...
```

The wrapper leaves `AGENT_TESTBENCH_CONFIG_HOME` to the CLI/default environment
and exports `ATB_STORE=local` only for helper scripts that need a Store name.
It prefers, in order: `ATB_BIN`, the latest repo runtime at
`/Users/zlh/codes/agent-testbench/.runtime/bin/agent-testbench`, the
`agent-testbench` binary on `PATH`, and the repo wrapper at
`/Users/zlh/codes/agent-testbench/bin/agent-testbench.sh`.

## Freshness Check

Before real work, confirm the wrapper is on the latest repo runtime:

```bash
/Users/zlh/codes/agent-testbench/skills/agent-testbench-operator/scripts/atb.sh status --json
/Users/zlh/codes/agent-testbench/skills/agent-testbench-operator/scripts/atb.sh doctor --json
```

If the runtime binary is missing or stale, rebuild and optionally install the
bare shell command from the current checkout:

```bash
/Users/zlh/codes/agent-testbench/skills/agent-testbench-operator/scripts/atb.sh onboard --repo /Users/zlh/codes/agent-testbench --store local --build-runtime --install-shell --smoke commands --json
```

For team/shared validation, use the Store name or DSN explicitly:

```bash
/Users/zlh/codes/agent-testbench/skills/agent-testbench-operator/scripts/atb.sh store use STORE_NAME
```

## Quick Commands

Confirm Store and Docker before running real tests:

```bash
/Users/zlh/codes/agent-testbench/skills/agent-testbench-operator/scripts/atb.sh store current --json
docker info
```

Run all active interface cases in the selected Store:

```bash
ATB_STORE=STORE_NAME /Users/zlh/codes/agent-testbench/skills/agent-testbench-operator/scripts/run-case-suite.sh
```

Run or watch repeatable Store-backed CLI tasks:

```bash
/Users/zlh/codes/agent-testbench/skills/agent-testbench-operator/scripts/atb.sh task run catalog-smoke --store STORE_NAME --command "commands --filter task --json" --json
/Users/zlh/codes/agent-testbench/skills/agent-testbench-operator/scripts/atb.sh task watch catalog-smoke --store STORE_NAME --command "commands --filter task --json" --interval 30s --limit 3 --until success --json
/Users/zlh/codes/agent-testbench/skills/agent-testbench-operator/scripts/atb.sh task logs catalog-smoke --store STORE_NAME --json
```

Send a notification test before wiring a task to notifications:

```bash
/Users/zlh/codes/agent-testbench/skills/agent-testbench-operator/scripts/atb.sh notify test --file /tmp/agent-testbench-notify.jsonl --message "AgentTestBench notification check" --json
```

Inspect sandbox service registration without starting services:

```bash
/Users/zlh/codes/agent-testbench/skills/agent-testbench-operator/scripts/atb.sh sandbox service list --store STORE_NAME --json
/Users/zlh/codes/agent-testbench/skills/agent-testbench-operator/scripts/atb.sh sandbox start --store STORE_NAME --dry-run --json
```

Plan or apply Store-first SQL edge migrations:

```bash
/Users/zlh/codes/agent-testbench/skills/agent-testbench-operator/scripts/atb.sh environment migration plan ENV_ID --store STORE_NAME --edge OWNER:KIND --database DB_NAME --json
/Users/zlh/codes/agent-testbench/skills/agent-testbench-operator/scripts/atb.sh environment migration apply ENV_ID --store STORE_NAME --edge OWNER:KIND --database DB_NAME --workspace WORKSPACE --execute --json
```

## Report Back

Always report:

- Store name/backend from `store current --json`.
- Runtime freshness: whether the active shell command points at the repo
  runtime.
- Workflow, suite, or task counts: total, passed, failed, not-run when
  available.
- Exact report paths for HTML, JSON, JUnit, task logs, or notification JSONL.
- Whether failures are workflow failures, assertion mismatches, dependency
  health failures, notification delivery failures, or CLI/service errors.

## References

Read `references/operator-runbook.md` for the full black-box runbook, command
patterns, environment-neutral defaults, task/watch/notify guidance, and when
source inspection is allowed.
