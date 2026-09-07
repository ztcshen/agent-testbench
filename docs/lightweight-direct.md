# Lightweight direct mode

Direct mode runs an existing local test adapter against an already-running
HTTP(S) application. Select it explicitly through `direct` commands or
`serve --mode direct`. Docker installation and daemon status do not select or
change this mode.

An existing Python, Node, Java, or other test project owns its test code,
request construction, assertions, and workflow sequencing. TestBench records
its binding and source identity in the selected SQL Store, launches the native
process, and records outcomes and evidence.

## Register and run

The adapter contract is JSON over stdin/stdout. The included
[example adapter](../examples/direct/adapter.py) uses Python's standard library
and a [small case catalog](../examples/direct/cases.json).

The following example assumes an initialized named Store and an existing
service with a `/health` endpoint. Supply paths and a URL for the intended setup.

```sh
agent-testbench direct register \
  --id example-tests --project "$TEST_PROJECT" --executable "$PYTHON" \
  --arg adapter.py --target-url "$TARGET_URL" \
  --evidence-dir "$EVIDENCE_DIR" --store "$STORE" --json

agent-testbench direct discover --store "$STORE" --json
agent-testbench direct plan --binding example-tests --case case.health --store "$STORE" --json
agent-testbench direct run --binding example-tests --case case.health --run-id run.example.1 --store "$STORE" --json
agent-testbench direct inspect --run run.example.1 --store "$STORE" --json
```

`--env NAME` forwards a named variable from the execution host. `--target-env
NAME` sets a variable to the registered target URL, for an existing test kit's
endpoint configuration. Basic process variables such as PATH, HOME, temporary
directory locations, and LANG are inherited. Keep credentials in the project's
credential mechanism or explicit environment references; do not put them in
command arguments or target URLs.

Registration invokes the adapter's discovery operation and saves the result
through catalog revision checks. `--expected-revision N` protects a refresh
from concurrent updates. Source files remain owned by their original project.
Bindings use existing versioned `TemplateConfig` storage with scope
`direct-executor`; there is no separate Store schema.

## Execution semantics

- The binding records a native runtime and an attached target. Direct mode
  does not start, stop, restore, or discover Docker services.
- `direct plan` and `direct run --dry-run` inspect source without executing the
  adapter or creating a run. They are not passed tests.
- A real execution records a `running` parent Run before launching a process,
  then appends the final CaseRun and updates the parent outcome.
- `--run-id` is a submission identity. Identical repeated submissions return
  the existing record; different inputs conflict. A new execution needs a new ID.
- `direct run` exits successfully only for a confirmed passed result. A running
  duplicate, skipped result, or failed result is not a successful CI gate.
- Each execution has a separate state/evidence directory. Adapters must direct
  caches, sessions, and temporary databases into that directory.
- Source hashes are checked before and after execution. Changed/missing source
  fails with `source-drift`. Refresh registration to accept intentional changes.
  This detects drift; it is not a hermetic copy of the language environment,
  external SDKs, or target application.
- Outcomes distinguish assertion failure, protocol errors, process exit,
  startup failure, timeout, cancellation, and evidence errors. Exit zero alone
  cannot produce a test pass.
- Output is bounded; timeout and normal cancellation stop owned process groups.
  The initial native implementation supports Unix platforms, including macOS
  and Linux.
- Direct HTTP execution is synchronous. A concurrent duplicate submission may
  return a running record; inspect that run rather than submitting a new ID.
  Abrupt host/process death is not automatically replayed or recovered in this
  slice. A remaining running record is unresolved, not a passed test.

## Adapter protocol, version 1

Discovery receives:

```json
{"protocolVersion":1,"operation":"discover","mode":"direct"}
```

Return one JSON object:

```json
{
  "protocolVersion": 1,
  "cases": [
    {"id":"case.health","title":"Service is healthy","kind":"case","sourcePath":"cases.json"}
  ],
  "sourceFiles": ["adapter.py","cases.json"]
}
```

Case IDs are unique safe identifiers within a binding. Source paths are
relative files within the project, including through symlinks. Include test
definitions, adapter/runtime code, and dependency manifests. The first slice
accepts up to 1,000 cases and 2,000 additional source files, each at most 4 MiB
and 64 MiB in total. Registration metadata, run inputs, and normalized result
summaries are separately bounded to fit the existing SQL Store contract.

Execution receives `operation: run`, the protocol version and mode, `runId`,
`caseId`, `caseKind`, `targetUrl`, absolute `stateDir`, and optional `overrides`.
Execute the original test or workflow and return:

```json
{
  "protocolVersion": 1,
  "caseId": "case.health",
  "status": "passed",
  "executed": true,
  "checks": [{"name":"expected-status","passed":true}],
  "data": {"statusCode":200}
}
```

`status` is `passed`, `failed`, or `skipped`. Passed/failed results require an
executed test and explicit checks consistent with status. `dryRun: true` is
rejected as an execution result. Emit diagnostics on stderr so stdout remains
one JSON object. Optional `data` preserves native step results, request/response
details, and diagnostics; public data uses the existing redactor. Large native
data is represented by an artifact pointer in the Store summary while the full
bounded output remains in `stdout.txt`; its size does not change a valid verdict.

The run directory contains `input.json`, `stdout.txt`, `stderr.txt`,
`snapshot.json`, and `result.json`. Files use owner-only permissions. Raw input
and streams are private indexed artifacts. Native session/cache files may
coexist there. The selected Store holds the Run, CaseRun, and Evidence indexes.

## Local API

```sh
agent-testbench serve --mode direct --store "$STORE"
```

Direct serving requires a loopback host and same-origin JSON requests. Selecting
this mode intentionally enables local execution of registered adapters. It is
not a remote command service. Existing workbench routes use `workbench` mode.

| Method | Path | Behavior |
| --- | --- | --- |
| GET | `/api/direct` | Mode and operations |
| GET | `/api/direct/bindings` | Store bindings and discovered cases |
| POST | `/api/direct/bindings` | Register `binding`, optionally with `expectedRevision` |
| POST | `/api/direct/plan` | Plan `bindingId`, `caseId`, and optional `overrides` |
| POST | `/api/direct/runs` | Run registered IDs, optional `runId` and `overrides` |
| GET | `/api/direct/runs/{runID}` | Inspect a persisted result |

Run requests do not accept executable, directory, environment, or target
overrides. Those belong to registration. HTTP success means a valid execution
report; inspect `status` and `ok` for the test outcome.

## Slice boundary

This slice provides CLI/API discovery and execution, local artifacts, and an
attached target. Direct bindings have their own discovery surface; they are not
automatically converted into native HTTP cases or Map nodes. UI integration,
remote artifacts, richer target health policies, and durable recovery remain
in the [consolidated plan](plans/2026-09-07-lightweight-direct-mode.md).
