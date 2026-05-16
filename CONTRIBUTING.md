# Contributing

Thanks for helping improve Open Test Sandbox. The project is local-first and
profile-driven: the core should stay generic, while team-specific test assets
belong in external profile bundles.

## Ground Rules

- Keep business or team language out of core source and default assets.
- Do not add a root `profiles/` directory to this repository.
- Do not commit runtime databases, Evidence bundles, logs, coverage, or local
  browser output.
- Keep changes small enough to verify, but large enough to finish a complete
  user-facing slice.
- Prefer profile/config additions over hardcoded behavior.

## Local Setup

```sh
npm ci
go test ./...
npm run build:frontend
```

Run the full release gate before opening a pull request:

```sh
npm run release-check
```

The gate runs formatting hygiene, generated-state checks, source-domain
guardrails, Go tests, the React build, and browser smoke tests.

## Pull Request Checklist

- The change has tests or a clear reason tests are not needed.
- Public CLI, API, profile, or report changes are documented.
- The source-domain guardrail passes.
- Runtime output remains ignored and untracked.
- The README or docs still let a new user complete the quick start.

## Profile Work

Profile bundles are source assets owned outside the core repository. When a
change needs new services, workflows, interface nodes, cases, fixtures, or
templates, create or update an external bundle and publish it through the CLI
or Control plane. Core code may read profile data through Store/read-models,
but it should not bake a specific organization or workflow into package logic.

See [docs/profile-authoring.md](docs/profile-authoring.md) for the authoring
workflow.
