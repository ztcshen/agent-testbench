# Comparison and Positioning

AgentTestBench is not trying to replace every test tool. Its strongest lane is
a Store-first workbench for agents and test engineers who need discovery,
execution, Evidence, and reviewable quality gates to share one source of truth.

## Quick Position

Use AgentTestBench when the hard part is not just sending one request, but
maintaining runnable targets, workflow maps, run history, Evidence indexes, and
agent-friendly APIs across local and shared SQL Stores.

Keep using focused mature tools when their operating model is already the
source of truth:

| Tool family | Strong fit | AgentTestBench fit |
| --- | --- | --- |
| [Postman / Newman](https://learning.postman.com/docs/collections/using-newman-cli/command-line-integration-with-newman/) | Existing Postman collections and collection-runner CI. | Store-backed catalog, Evidence indexing, map/workflow gates, and agent discovery around API cases. |
| [Karate](https://karatelabs.github.io/karate/) | Code-based API/UI/performance suites with a compact DSL. | Shared SQL Store state, workbench/API discovery, Evidence detail APIs, and reusable scenario maps across runners. |
| [Testcontainers](https://testcontainers.com/) | Test code needs disposable real dependencies during unit/integration tests. | A registered environment lifecycle, target restore, runtime Evidence, and operator-facing gates outside a single test process. |
| [Backstage](https://backstage.io/docs/features/software-catalog/) | Software catalog and developer portal ownership metadata. | Execution control plane for test targets, case/workflow/map runs, Evidence, and release gates. |
| [OpenTelemetry Demo](https://opentelemetry.io/docs/demo/) | Reference application for observability instrumentation and telemetry demos. | Consuming real topology/log/timing evidence from target runs and storing it with verification results. |

## What Makes AgentTestBench Different

- **Store-first daily workflow**: SQLite, PostgreSQL, and MySQL use the same
  CLI/API surfaces. Template packages are import/export artifacts, not the
  normal operating surface.
- **Agent-native discovery**: agents can ask for commands, targets, maps,
  cases, runs, and Evidence before execution instead of guessing IDs from docs.
- **Evidence as a product surface**: request, response, assertions, timing,
  logs, topology, and artifacts are indexed so a failed run is reviewable after
  the command exits.
- **Scenario maps above cases**: maps converge interface nodes, workflows,
  validation families, and materializations into explainable plans and gates.
- **Local-first adoption**: `agent-testbench demo` proves the loop with a
  temporary API target and SQLite Store before any team Store exists.

## Honest Limits

- AgentTestBench is young. Teams that only need a single collection runner
  should start with the existing mature runner they already trust.
- It is most valuable when Store-backed history and Evidence matter. If a test
  only needs in-process assertions, a unit/integration test framework is
  simpler.
- Shared Stores and real topology gates need team-owned setup. The local demo
  proves the mechanics, not a production environment.

## Evaluation Script

For a first technical review:

```sh
./bin/agent-testbench.sh demo
./bin/agent-testbench.sh commands --json
./bin/agent-testbench.sh commands --filter "case gate"
./bin/agent-testbench.sh commands --filter "map run"
```

Then open the generated Evidence directory from the demo output and inspect the
stored run:

```sh
agent-testbench case inspect --view runs --store sqlite:///path/to/store.sqlite --json
```

The key question is whether your team needs this Store + Evidence + discovery
loop around existing tests. If yes, AgentTestBench can sit beside mature
runners instead of forcing a rewrite.
