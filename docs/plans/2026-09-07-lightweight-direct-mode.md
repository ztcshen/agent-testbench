# Testing workbench direction and lightweight direct execution

Date: 2026-09-07

This document consolidates the current review and the first implementation
slice. It records proposed changes separately from implemented behavior. Private
test projects, environment addresses, credentials, and runtime evidence remain
outside public source.

## Product decision

**Lightweight direct mode is an explicit operating mode.** It runs an existing
local test executor against an already-running application. Selecting it never
depends on the availability of Docker. TestBench owns registration, execution
identity, outcome recording, and evidence; it does not own the target's startup
or shutdown in this mode.

Three independent concerns must stay separate:

| Concern | Direct mode | Managed environment mode |
| --- | --- | --- |
| Control-plane persistence | Selected SQLite, PostgreSQL, or MySQL Store | Same Store contract |
| Test executor | Local process in its existing language runtime | Local or container executor as configured |
| Target application | Existing endpoint with explicit target configuration | Environment lifecycle may prepare repositories, start services, and restore Docker Compose |

Container-based restoration remains a supported capability. The direct path has
its own discovery, planning, execution, and result contract; it does not require
environment restoration, a component graph, or a topology publication gate.
An ordinary test pass never implies that an entire environment is verified.

## Consolidated change inventory

| ID | Change | Rationale | Delivery |
| --- | --- | --- | --- |
| D01 | Add explicit lightweight direct execution of existing local test projects | External executor descriptors currently plan but do not execute native tests | First slice |
| D02 | Register attached target configuration independently of target startup | Existing services should be usable without inventing a Compose or start-command lifecycle | First slice, binding-level target |
| D03 | Normalize native results and index run evidence | Process exit, assertion failure, interrupted execution, and a dry-run are different outcomes | First slice |
| D04 | Isolate execution state directories | Cache, session, timing, and output state must not leak between independent runs | First slice |
| D05 | Preserve source identity and detect changed test inputs | A case ID alone does not identify the code and data actually executed | First slice |
| D06 | Keep external case ownership in its original project | Avoid duplicating signing, request builders, assertions, and workflow logic inside the core | First slice, references in Store |
| D07 | Extract reusable execution responsibilities from CLI and HTTP packages | The current CLI and control-plane packages hold too much orchestration | Start with the new slice; broader extraction remains open |
| D08 | Expose Map discovery, explain, execution, and gates consistently through APIs | Existing Map functionality is concentrated in the CLI | Follow-up |
| D09 | Provide Store-case dry-runs and temporary drafts | Discovery and exploration should not require complete production-style catalog preparation | Follow-up; direct plans included first |
| D10 | Add shared artifact retrieval and durable artifact ownership | Shared SQL indexes currently refer to execution-host-local files | Follow-up |
| D11 | Separate topology requirements from provider implementations | Real evidence remains required where appropriate; provider choice should not be a universal workflow constraint | Follow-up; existing verified publication policy unchanged |
| D12 | Audit capability-specific diagnostics | A local test executor should require its own dependencies; development and managed-environment prerequisites should be separate | Follow-up |
| D13 | Make fixture setup, sharing, and cleanup explicit | Repeatability depends on equivalent preconditions and controlled side effects | Direct state directories first; general fixture model follow-up |
| D14 | Distinguish submit retry, test retry, new execution, and replay | Request idempotency does not make every tested operation idempotent | First slice reserves run identity; general recovery policy follow-up |
| D15 | Track changes to expected behavior and test strength | Automatic repair must not silently erase failures, skip coverage, or weaken assertions | Follow-up |
| D16 | Reduce procedural instructions as model capability improves | Agents can own test design, code generation, and investigation; the runtime owns facts and boundaries | Ongoing, measured with historical defects |

The 2026-09-07 working-tree review ran Go and frontend tests. It observed 1,237
passing Go test items and 87 skipped external-database/real-Docker items, 56
passing frontend tests, and a report-only quality result of 3 blocking and 33
warning findings. The Go duplicate detector found no clones at its configured
thresholds. These are baseline observations, not validation of this new slice.

## Maintenance and execution boundaries

During maintenance an agent may discover cases, propose scenarios, change
scripts, refine fixtures, and investigate failures. Stable regression runs bind
their test definition and target before execution. Changes create a new
definition or source snapshot rather than changing the meaning of an existing
run.

- Submission idempotency: repeating the same request identity must not launch
  a second process. Reusing it with different inputs is a conflict.
- Repeatable execution: a new execution gets its own state directory; sharing
  within an existing external workflow remains that workflow's responsibility.
- Failure reproduction: preserve source digests, resolved inputs, target,
  executor identity, and original outcomes. This does not claim that external
  services or all dependencies are hermetically reproducible.
- Assertion quality: distinguish expected behavior from observed output.
  Updating a baseline, changing an assertion, and repairing a locator are
  identifiable maintenance actions.
- Result integrity: preserve failed attempts, skipped outcomes, malformed
  reports, cancellations, and unknown outcomes. A process exit code of zero
  alone does not prove a test passed.

## Reference evidence and adaptation

| Reference | Observed approach | Adaptation |
| --- | --- | --- |
| [Playwright Test Agents](https://playwright.dev/docs/test-agents) | Agents generate and maintain executable test files | Let agents maintain test assets while a runner executes selected definitions |
| [Playwright fixtures](https://playwright.dev/docs/test-fixtures) | Scoped setup/teardown and fixture dependencies | Give each execution explicit state, preserving intentional workflow sharing |
| [Playwright web server](https://playwright.dev/docs/test-webserver) | Start a local command or reuse an existing server | Treat attached targets and managed startup as separate lifecycle choices |
| [TestSprite CLI v0.10.0](https://github.com/TestSprite/testsprite-cli/tree/v0.10.0) | Versioned code updates, run IDs, structured failure bundles; hosted execution | Reuse result and identity concepts, without introducing a hosted dependency |
| [Testkube workflows](https://docs.testkube.io/articles/test-workflows) | Invoke existing tools with explicit code sources and execution configuration | Preserve external runner ownership; keep container orchestration optional |
| [Keploy test recording](https://keploy.io/docs/running-keploy/cli-commands/) | Recorded requests and dependencies, explicit re-recording and normalization | Treat a recorded baseline as a versioned test input |
| [Schemathesis replay](https://schemathesis.readthedocs.io/en/stable/guides/crash-reproduction/) | Preserve failing requests and stateful call chains | Keep generation separate from replay; preserve dependency context |
| [Allure report integration](https://allurereport.org/docs/v2/generate-report/) | Original test frameworks emit results consumed by a common report layer | Normalize external reports without translating every test into the native HTTP format |
| [Test-strength research, August 2026 revision](https://arxiv.org/abs/2604.01518v2) | Validate generated tests against behavioral variants | Evaluate defect detection and false passes, not pass rate alone |

TestSprite's public CLI is not its hosted execution engine. Testkube's open
agent and commercial control plane have different scopes. Tracetest remains a
useful conceptual reference, but its last observed repository push was in
June 2025. None of these observations establish comparative runtime quality.

## First slice contract

The implementation will provide an explicitly named `direct` CLI/API surface.
A one-time registration records an existing local executor, attached target,
case references, source identity, and evidence location in the active Store.
Daily discovery and runs use that Store registration.

The generic executor adapter uses JSON for discovery and execution. Its test
implementation remains outside the core. Public examples use only synthetic
HTTP targets and test data. An existing external workflow initially remains a
single execution unit; its own runner controls its internal dependency order.

The native process boundary must support explicit executable/argv/working
directory, bounded output, timeout and cancellation of owned child processes,
and per-run state. The result contract must distinguish real pass/fail/skip
from planning and protocol failure. Source drift must be visible and require
an explicit registration refresh.

The direct HTTP surface is an explicitly selected local execution mode. It
accepts registered identities and execution inputs, not a shell command hidden
in a run request. The first slice does not expose a remote general-purpose
command service.

## Acceptance and validation

1. Register and discover an external synthetic test project in a fresh local
   SQLite Store using direct mode.
2. Execute a real native process against an already-running local HTTP target.
   Docker availability is irrelevant: direct mode invokes no Docker command.
3. Verify pass, assertion failure, malformed output, process startup failure,
   timeout, and duplicate request handling.
4. Verify that a reported dry-run cannot become a passed test.
5. Preserve source identity, per-run directories, original output, and indexed
   evidence across repeated runs; prevent source drift from silently changing
   a registered case.
6. Exercise CLI and API through the same application service.
7. Demonstrate the private existing project's adapter without calling shared
   business environments. Any live target validation needs its own authorized
   target and case selection.
8. Run focused tests, `go test ./...`, `make quality`, and relevant repository
   guards. Report baseline debt separately from newly introduced findings.

## Status

Public surface impact of the first slice:

- Adds five `direct` CLI commands and expands default discovery from 35 to at
  most 40 commands; existing workbench commands retain their meanings.
- Adds `serve --mode direct` and an explicitly separate local API route set.
  The default remains the existing workbench server.
- Adds runner packages for native process lifecycle and direct registration/run
  orchestration. They reuse versioned catalog storage and Run/CaseRun/Evidence
  interfaces; SQL schemas, migrations, and existing HTTP-case contracts do not
  change.
- Direct bindings are catalog template-config records. They have their own
  discovery surface and are not yet native HTTP Case Catalog or Map entries.
- A stable `caseKey` qualifies the external case identity by its binding, so
  equally named cases in unrelated projects do not share history accidentally.

- [x] Consolidated session decisions and research evidence.
- [x] Implement first direct-mode slice.
- [x] Verify local native execution and API/CLI behavior.
- [x] Add private existing-project adapter and validate it locally.
- [x] Record checks for the local review checkpoint.

First-slice verification on 2026-09-07:

- `go test ./...`: 1,268 test items passed; 87 external database and real-Docker
  checks skipped. No shared service was used for this validation.
- Native process and direct runner race tests passed.
- Go lint for the CLI, control plane, direct service, and native process packages
  reported zero issues.
- `make quality`: 3 existing package-size blocking findings and 34 warnings;
  the added warning identifies the new public surface documented above. No
  duplicate clone blocks were found at the repository's configured thresholds.
- Store-first, secret, whitespace, and required keyword guards passed.
- An actual native adapter called a synthetic HTTP target, recorded pass and
  assertion failure, and reused an identical request identity across CLI/API
  without another target call. An available Docker stub was never invoked.
  The local direct server shut down cleanly.
- The existing private adapter's discovery and plan were registered and checked
  in a local SQLite Store. Its four adapter tests passed. Actual business cases
  were not executed.
- Existing unrelated working-tree changes were preserved.

Broader Map APIs, remote artifact storage, provider-general topology acceptance,
and automatic test-strength gates remain follow-up work.
