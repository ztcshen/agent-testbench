# Import Bundle Authoring Guide

Import Bundles make Open Test Sandbox useful without turning the core repository into
a business-specific codebase. A import bundle is a reviewable configuration bundle
owned outside this repository and published into the local Store when a user or
agent needs to run it.

## What Belongs in a Import Bundle

- Services and runtime endpoints for a team environment.
- Workflows and step order.
- Interface nodes and the cases attached to them.
- Request templates and small fixture data.
- Case dependencies and workflow bindings.
- Local defaults such as request timeout budgets.

## What Stays in Core

- Generic Store interfaces, PostgreSQL implementation, and compatibility backends.
- Import Bundle loading, auditing, installation, packing, and publishing.
- API case execution and Evidence indexing.
- Control plane APIs and React workbench pages.
- Generic report templates and stable CLI contracts.

## Recommended Layout

```text
import bundle.json
services/
workflows/
interface-nodes/
cases/
request-templates/
case-dependencies/
workflow-bindings/
fixtures/
```

The manifest can contain all assets inline for small bundles. Larger bundles
should use the split directories above. Files are loaded in sorted path order,
which makes review and generated diffs predictable.

## Authoring Flow

1. Create a bundle outside the core repository:

   ```sh
   otsandbox import bundle init --output /path/to/team-import bundle --id team-alpha
   ```

2. Add services, workflows, interface nodes, cases, fixtures, and bindings.

3. Audit locally:

   ```sh
   otsandbox import bundle audit --import bundle /path/to/team-import bundle --json
   ```

4. Publish and verify against a local Store:

   ```sh
   otsandbox import bundle verify \
     --import bundle /path/to/team-import bundle \
     --store local-personal \
     --require-case-runs \
     --require-workflow-runs
   ```

5. Pack the reviewed bundle for handoff:

   ```sh
   otsandbox import bundle pack \
     --import bundle /path/to/team-import bundle \
     --output team-alpha-import bundle.tar.gz
   ```

## Agent-Friendly Discovery

Agents should not rely on hardcoded case or workflow identifiers. The expected
flow is:

1. Run `interface-node discover` or `workflow discover` with an optional filter.
2. Choose the target from the returned identifiers.
3. Run the matching report command with the selected identifier.
4. Use the report links to inspect failed case Evidence when needed.

This keeps prompts generic and lets each team evolve import bundle ids without
changing core code.

## Review Rules

- Keep import bundle changes file-first and reviewable.
- Keep generated Store rows, Evidence bundles, reports, and logs out of source
  control.
- Use synthetic data for public examples.
- Put secret or private values behind local environment configuration, not in a
  shared import bundle bundle.
