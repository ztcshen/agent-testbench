# GitHub Actions Integration

This starter workflow keeps adoption deliberately small: run the built-in local
demo, run a scoped release check against a temporary SQLite Store, and upload
Evidence when a job fails.

```yaml
name: agent-testbench

on:
  pull_request:
  push:
    branches: [main]

jobs:
  smoke:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v7

      - uses: actions/setup-go@v6
        with:
          go-version-file: go.mod

      - uses: actions/setup-node@v6
        with:
          node-version: 20
          cache: npm

      - name: Install release gate tools
        run: sudo apt-get update && sudo apt-get install -y ripgrep sqlite3

      - run: npm ci

      - name: Run local AgentTestBench demo
        run: ./bin/agent-testbench.sh demo --output-dir .runtime/demo --json

      - name: Run scoped release check
        env:
          AGENT_TESTBENCH_SMOKE_STORE_DSN: sqlite://${{ github.workspace }}/.runtime/ci-store.sqlite
        run: npm run release-check -- --scope cmd/agent-testbench

      - name: Upload AgentTestBench Evidence
        if: always()
        uses: actions/upload-artifact@v7
        with:
          name: agent-testbench-evidence
          path: |
            .runtime/demo
            .runtime/release-check
          if-no-files-found: ignore
```

## Store Choice

Use SQLite for first CI adoption because it is local to the job and does not
need secrets. Move to PostgreSQL or MySQL when the team wants shared Store
history across jobs or environments:

```yaml
env:
  AGENT_TESTBENCH_SMOKE_STORE_DSN: ${{ secrets.AGENT_TESTBENCH_SMOKE_STORE_DSN }}
```

The smoke Store must be an AgentTestBench metadata Store. Do not point CI at an
application database or any database restored for a target system.

## Promotion Checklist

- Add the demo job first so reviewers can see request/response/assertion
  Evidence without team infrastructure.
- Add `release-check` after the demo is stable; keep `--scope` narrow for early
  PRs and use `--full` for scheduled or release jobs.
- Upload `.runtime/demo` and `.runtime/release-check` artifacts on every run so
  failures are reviewable from the Actions page.
- For real topology sign-off, provide `AGENT_TESTBENCH_TRACE_GRAPHQL_URL`,
  `AGENT_TESTBENCH_SMOKE_EXPECTED_STEPS`, `AGENT_TESTBENCH_SMOKE_TRACE_IDS`,
  and `AGENT_TESTBENCH_REQUIRE_REAL_SKYWALKING=1` from protected CI secrets or
  environment variables.
