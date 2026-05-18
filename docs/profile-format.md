# Import Bundle Bundle Format

A import bundle bundle is a reviewable directory of configuration assets kept outside
the Open Test Sandbox core repository. The minimum bundle contains a
`import bundle.json` manifest.

```json
{
  "id": "empty",
  "displayName": "Empty Import Bundle",
  "description": "A neutral import bundle with no services, workflows, cases, or fixtures.",
  "services": [],
  "workflows": [],
  "interfaceNodes": [],
  "apiCases": [],
  "executors": [],
  "requestTemplates": [],
  "caseDependencies": [],
  "workflowBindings": [],
  "fixtures": [],
  "templateConfigs": []
}
```

## Manifest Fields

- `id`: stable import bundle identifier.
- `displayName`: human-readable import bundle name.
- `description`: optional import bundle summary.
- `services`: systems or dependencies involved in the import bundle.
- `workflows`: template-driven sequences of testable steps.
- `interfaceNodes`: observable interfaces that cases can target.
- `apiCases`: runnable interface tests.
- `executors`: external test tool or script descriptors used for dry-run
  planning before invoking tools outside core.
- `failureCategories`: optional report category rules for local failure
  summaries.
- `requestTemplates`: reusable request rendering assets for API cases.
- `caseDependencies`: fixture requirements and mappings for cases.
- `workflowBindings`: links from workflow steps to interface nodes and cases.
- `fixtures`: input or precondition data for cases and workflows. Fixtures can
  include `dataJson` when a import bundle owns small JSON data needed for request
  template rendering.
- `templateConfigs`: optional presentation and execution configuration for
  generic Control plane templates. Keep import bundle-specific workflow targets here
  instead of hardcoding them into core UI templates.

Configuration remains file-first outside core. Store records are generated
runtime indexes and read-models used by the Control plane; they are not the
source of truth for import bundle assets.

Publish a bundle before serving it through the workbench:

```sh
otsandbox import bundle init --output /path/to/import bundle-bundle --id sample
otsandbox import bundle install --from /path/to/import bundle-bundle
otsandbox import bundle verify --import bundle sample --store-url .runtime/store.sqlite
otsandbox serve --import bundle sample --store-url .runtime/store.sqlite
```

For local bootstrapping, `otsandbox serve --import bundle /path/to/import bundle-bundle`
first publishes the external bundle into the Store/read-model, then serves that
indexed view.

The init command refuses output paths under the core repository's `import bundles/`
directory. This keeps generated bundles external even during local experiments.
It also writes a bundle-local `.gitignore` for generated runtime state such as
`.runtime/`, SQLite files, database sidecar files, and local logs.

## Standard Local Placement

Installed import bundle bundles live outside the core repository. By default the CLI
uses `$HOME/.otsandbox/import bundles`; set `OTSANDBOX_IMPORT_BUNDLE_HOME` or pass
`--import bundle-home PATH` to use a team checkout, mounted volume, or temporary test
directory.

```sh
otsandbox import bundle install --from /path/to/import bundle-bundle
otsandbox import bundle list
otsandbox import bundle pack --import bundle sample --output sample-import bundle.tar.gz
otsandbox import bundle inspect --import bundle sample
otsandbox import bundle verify --import bundle sample --store-url .runtime/store.sqlite
```

`import bundle install` copies the external bundle into the import bundle home under the
bundle's `id`. It accepts either a import bundle directory or a `.tar.gz` / `.tgz`
archive created by `import bundle pack`. The copy is intentionally source-focused:
generated runtime state, local SQLite/database files, logs, and VCS directories
are skipped. Use `--force` to replace an already installed bundle. Commands
that accept import bundle bundles (`inspect`, `audit`, `verify`, `import`, and
`config publish`) accept either a filesystem path or an installed import bundle id.
`serve --import bundle ID --import bundle-home PATH` follows the same resolution rule.

`import bundle list` and `GET /api/import bundle/installed` are tolerant of a mixed import bundle
home. If one installed directory has a malformed manifest, the list still
returns the other import bundles and includes an item with `valid: false` plus an
`error` message for the broken directory. The workbench disables invalid
installed import bundles in the selector instead of hiding the problem.

Use `import bundle pack --import bundle PATH_OR_ID --output bundle.tar.gz` to create a
clean distributable archive for review or handoff. The command accepts either a
filesystem path or an installed import bundle id, uses the same runtime/VCS filtering
as `import bundle install`, and writes import bundle files under an archive root named after
the import bundle id. Archive installation is path-safe: entries that would escape the
archive root are rejected. `import bundle audit`, `import bundle import`, `import bundle verify`,
and `config publish` can also accept a packed archive directly; they install
the archive into the configured import bundle home before auditing or writing
Store/read-model data. Pass `--force` when a matching installed import bundle should
be replaced.

The Control plane exposes the same local placement surface:

- `GET /api/import bundle/installed`: list installed import bundle bundles.
- `POST /api/import bundle/install`: install a bundle from a local path or packed
  archive into the configured import bundle home.
- `POST /api/import bundle/import` and `POST /api/import bundle/verify`: accept either a
  local path, packed archive, or installed import bundle id in the `path` field.
  Archive paths are installed into the configured import bundle home first, then the
  installed bundle is published or verified. Pass `force: true` when a matching
  installed import bundle should be replaced.

The workbench Import Bundle panel lists installed import bundles, can install a bundle from
a local path, and can publish or verify the selected import bundle id.

## Audit

Use `otsandbox import bundle audit --import bundle PATH` to check a bundle before or after
import. The audit verifies basic reference integrity across workflows, API
Cases, request templates, fixtures, case dependencies, and workflow bindings.
For example, it reports a workflow binding that points to a missing workflow,
an API Case that points to a missing interface node, or a case dependency that
points to a missing fixture.

Add `--store-url PATH` to include the local Store import bundle index and API Case run
status in the report. Add `--json` when another tool needs a stable
machine-readable report.

Use `--require-audit-ok` with `import bundle import` or `config publish` when the
publish step must fail before Store/read-model writes if reference integrity
issues are found. The Control plane import API exposes the same behavior with
`requireAuditOk: true`.

Use `otsandbox import bundle verify --import bundle PATH --store-url PATH` as the standard
local acceptance command for an external bundle. It audits the bundle, publishes
it only if the audit is clean, then checks that the import bundle index, active config
version, catalog index, and base Control plane read-models were written for the
same published config version. The Control plane exposes the same flow through
`POST /api/import bundle/verify`, and the workbench Import Bundle panel provides a matching
`验收并发布` action.

Add `--require-case-runs` when acceptance should also prove runtime coverage.
With that gate enabled, `import bundle verify` checks every API Case declared by the
import bundle against the Store's latest API Case run records and fails unless each
case has a latest passed run. The Control plane accepts `requireCaseRuns: true`,
and the workbench exposes the same gate as `要求用例已通过`.

Add `--require-workflow-runs` to apply the same acceptance rule to every
declared Workflow. The Control plane accepts `requireWorkflowRuns: true`, and
the workbench exposes the gate as `要求工作流已通过`.

Verification failures are diagnostic reports, not opaque errors. JSON output
from the CLI and non-2xx Control plane responses both include `ok: false`,
`error`, `summary`, and `checks` so a caller can show the exact missing
acceptance gate without re-running the publish step. The workbench renders the
same failed report inline.

## Split Assets

Large bundles can keep assets in deterministic JSON directories next to
`import bundle.json`:

- `services/*.json`
- `workflows/*.json`
- `interface-nodes/*.json`
- `cases/*.json`
- `executors/*.json`
- `request-templates/*.json`
- `case-dependencies/*.json`
- `workflow-bindings/*.json`
- `fixtures/*.json`

The loader reads files in sorted path order and appends them to any assets
declared directly in the manifest.

## Template Configs

Use `templateConfigs` to tune generic Control plane templates from import bundle
configuration. A workflow directory target can be declared with a default
`workflow-directory` scoped config:

```json
{
  "id": "cfg.workflow-directory.default",
  "templateId": "TPL-WORKFLOW-DIRECTORY-V1",
  "scopeType": "workflow-directory",
  "scopeId": "_default",
  "configJson": "{\"workflowFinder\":{\"targetStepCount\":4,\"targetInterfaceCount\":4,\"targetLabel\":\"Configured workflow target\"}}",
  "status": "active"
}
```

The Control plane exposes this as `GET /api/catalog` under
`presentation.workflowFinder`. UI code should read these values from the
import bundle/catalog payload; concrete workflow targets belong in import bundle
configuration, not in core templates.

## Import Planning

Open Test Sandbox can derive reviewable import bundle asset plans from external API
descriptions or captured HTTP traffic without writing those assets directly
into a bundle. The first planners are small JSON planners inspired by the
reference backlog: Microcks and Schemathesis motivate schema/API asset import,
while Keploy motivates record/replay import from captured traffic.

Use the CLI to inspect the plan before copying any generated assets into a
import bundle bundle:

```sh
otsandbox import bundle import-plan openapi --from /path/to/openapi.json --service-id service.catalog --evidence-dir .runtime/openapi --json
otsandbox import bundle import-plan http-capture --from /path/to/traffic.json --service-id service.catalog --evidence-dir .runtime/replay --json
otsandbox import bundle generation-plan openapi --from /path/to/openapi.json --service-id service.catalog --evidence-dir .runtime/generated --json
```

Add `--output-dir PATH` to write the same plan as reviewable files:

- `import-plan.json`: full source, generated asset, and written-file summary.
- `services/*.json`, `interface-nodes/*.json`, `request-templates/*.json`,
  and `cases/*.json`: import bundle split assets ready for review.
- `api-cases/*.json`: runnable API case files referenced by the generated
  `apiCases[].casePath` values.

The planner deliberately produces `draft` assets. Import Bundle authors or agents must
review and apply the generated assets before they become part of a maintained
suite. This keeps the core import bundle source reviewable and avoids silently
activating generated tests.

Current planner scope:

- JSON OpenAPI 3.x documents.
- HTTP operations under `paths`.
- `operationId`, `summary`, `description`, and `tags`.
- first 2xx response code as the generated positive assertion.
- `application/json` request-body `example` as the draft request body.
- Static HTTP capture JSON with `captures[].request` and
  `captures[].response`.
- Captured request method/path/headers/body, response status, and compact
  response body containment assertions.
- OpenAPI schema generation candidates for missing required request fields,
  emitted as draft negative cases with `generated`, `schema`, and `negative`
  tags.

Generated plans include a service, interface nodes, request templates, draft API
case metadata, and generated API case file bodies. The output directory is a
review surface, not an automatic publish step. YAML, schema expansion, negative
cases beyond missing required fields, stateful workflow generation, eBPF
capture, database/queue virtualization, time freezing, and direct writes into
an existing import bundle bundle are intentionally left for later slices.

## API Case Run Fields

API Case assets can optionally declare local run settings used by the control
plane workbench. They can also carry maintenance metadata so teams can search,
review, and assign case suites without editing the core repository:

```json
{
  "id": "case.alpha",
  "displayName": "Create Item",
  "description": "Creates an item with the default valid payload.",
  "nodeId": "node.alpha",
  "tags": ["smoke", "regression"],
  "priority": "p0",
  "owner": "team-a",
  "status": "active",
  "casePath": "cases/case.alpha.json",
  "baseUrl": "http://127.0.0.1:18080",
  "evidenceDir": ".runtime/cases",
  "timeoutSeconds": 30,
  "defaultOverrides": {
    "itemId": "item-001"
  }
}
```

- `description`: optional case purpose or review note.
- `tags`: optional searchable labels for suites such as smoke, regression, or
  negative.
- `priority`: optional team-defined priority such as p0, p1, or p2.
- `owner`: optional team, service, or person responsible for maintaining the
  case.
- `status`: case lifecycle state. Empty status defaults to `active`. Supported
  values are `draft`, `review`, `active`, `quarantined`, and `deprecated`.
  Only `active` cases are considered executable-ready by suite quality and
  planning checks.
- `casePath`: path to the runnable API Case JSON file.
- `sourceKind`, `sourcePath`, `executorId`: optional external executable source
  reference for cases owned by tools such as Karate, Playwright, pytest, or
  custom import bundle executors. This is a compatibility hook, not a new core DSL.
  Suite quality treats an external source as runnable only when it references an
  active import bundle executor.
- `baseUrl`: default target URL for live runs.
- `evidenceDir`: optional runtime Evidence output directory.
- `timeoutSeconds`: optional request timeout for the control plane run API.
- `defaultOverrides`: optional import bundle-owned defaults passed to the page.

Use `otsandbox case discover` to query this metadata after publishing the
import bundle into a Store.

## Executor Descriptors

Import Bundles can describe external test tools without making Open Test Sandbox own
those tools. This follows the reference-backed executor model: define existing
test tools or scripts as reviewable import bundle assets, then let a caller inspect
readiness before deciding whether to run them.

```json
{
  "id": "executor.karate.api",
  "displayName": "Karate API suite",
  "kind": "karate",
  "sourcePath": "tests/api.feature",
  "status": "active",
  "artifactPaths": ["target/karate-reports"]
}
```

Supported descriptor kinds are `http-case`, `playwright`, `postman`, `k6`,
`pytest`, `karate`, and `custom-command`. Tool-specific descriptors use
`sourcePath`; `custom-command` uses `command` plus optional `args`. The current
surface is a dry-run plan:

```sh
otsandbox executor plan --import bundle /path/to/import bundle --json
```

The plan validates ids, supported kinds, active status, required source paths or
commands, and declared artifact paths. It does not execute external commands.

API cases can reference an executor-owned external source when the runnable
case is not a native Open Test Sandbox JSON case:

```json
{
  "id": "case.karate.create-item",
  "displayName": "Create Item via Karate",
  "nodeId": "node.alpha",
  "status": "active",
  "sourceKind": "karate",
  "sourcePath": "tests/api.feature",
  "executorId": "executor.karate.api"
}
```

This preserves discovery, Store indexing, quality checks, and Evidence/report
surfaces in Open Test Sandbox while leaving execution semantics to the external
tool descriptor.

## Failure Categories

Import Bundles can define local failure category rules for batch reports. This follows
the reference-backed Allure category model: rules are evaluated in order and the
first matching rule wins. The rule only changes the report-facing
`failureCategory`; it does not change case execution status.

```json
{
  "failureCategories": [
    {
      "name": "Product errors",
      "matchers": {
        "statuses": ["failed"],
        "failureCategories": ["assertion-mismatch"],
        "messageContains": ["not expected"]
      }
    }
  ]
}
```

Supported matchers are `statuses`, `failureCategories`, and
`messageContains`. When no rule matches, Open Test Sandbox keeps the built-in
local category such as `assertion-mismatch`, `transport-error`, `timeout`, or
`case-failure`.
