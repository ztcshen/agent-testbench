# Adoption Playbook

Use this page when evaluating or presenting AgentTestBench outside the core
maintainer group. It keeps the pitch honest: AgentTestBench is strongest when
teams need agent-friendly discovery, Store-backed execution history, auditable
Evidence, and reviewable API workflow gates in one local-first workbench.

## Positioning

**AgentTestBench is a local-first control plane for agent-native API workflow
validation.**

It sits beside mature runners instead of replacing them. Postman/Newman,
Karate, Playwright, Testcontainers, and in-process tests can remain the best
tool for their lane. AgentTestBench adds the shared operating layer around
them: discoverable targets, SQL Store-backed run state, workflow/map planning,
Evidence indexes, and compact reports that a person or agent can inspect after
the command exits.

## Five-Minute Evaluation

Run this path from a fresh source checkout:

```sh
./bin/agent-testbench.sh demo
npm ci
npm run demo:one
npm run smoke:release-archive-serve
```

What this proves:

- `demo` runs one generic API case without any team Store or private target.
- `demo:one` proves the source checkout wrapper can run the same first proof.
- `smoke:release-archive-serve` builds a release archive, extracts it outside
  the source tree, starts `agent-testbench serve` from the extracted directory,
  loads the workbench UI, and confirms the Store API responds.

When presenting the UI, open:

```sh
open control-plane/static/demo-gallery.html
```

Then show the generated Evidence directory printed by `demo` and explain that
the same Store/Evidence contract is used by API cases, workflow runs, map
plans, gates, and the workbench.

## Promotion Assets

Use these assets in order:

- README badges: CI, Release, Apache-2.0 license.
- One-liner: "AgentTestBench is a local-first control plane for agent-native
  API workflow validation."
- Visual tour: `control-plane/static/demo-gallery.html`.
- Comparison: [Comparison and Positioning](comparison.md).
- CI starter: [GitHub Actions Integration](github-actions.md).
- Release confidence: [Release Checklist](release-checklist.md).
- Share copy: [Share Kit](share-kit.md).

Suggested repository topics:

`testing`, `test-automation`, `integration-testing`, `api-testing`,
`local-first`, `developer-tools`, `agent`, `agents`, `evidence`,
`workflow`, `qa-automation`, `golang`, `react`

## Trust Checklist

Before sharing a public tag or asking external users to try the project:

- CI badge is green on the target branch.
- `npm run release-check -- --scope PATH` passes for the release slice, or
  `npm run release-check -- --full` passes for a release sign-off.
- `npm run smoke:release-archive-serve` passes on the intended platform.
- Public docs do not contain private Store names, DSNs, customer workflows,
  internal service names, ports, or business-domain examples.
- Release notes list the minimum Go and Node versions, known limitations, and
  any CLI/API contract changes.
- Example data is synthetic and domain-neutral.

## Honest Limits

Say these upfront:

- AgentTestBench is pre-1.0; CLI/API/report contracts may still tighten.
- A single collection runner is simpler when a team only needs one request
  suite and does not need Store-backed Evidence history.
- Real topology proof requires a team-owned SkyWalking endpoint and trace ids;
  the built-in synthetic provider only proves local wiring.
- Shared PostgreSQL/MySQL Stores need a dedicated AgentTestBench database. Do
  not point demos or release gates at application schemas.

## Adoption Path

1. Run the built-in demo locally.
2. Pick one maintained API case or workflow and publish only generic metadata
   into a dedicated SQLite/PostgreSQL/MySQL Store.
3. Add scoped `release-check` to pull requests that touch that slice.
4. Use the workbench to review Evidence and failure reports.
5. Add a scenario map only after repeated workflows start to duplicate paths or
   validation families.
6. Add real SkyWalking sign-off only when the team has stable trace ids for the
   workflow being certified.
