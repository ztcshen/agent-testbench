# Visual Workflow Studio

Open `/workflow-studio.html` on an AgentTestBench workbench server. The workflow
catalog also links to the studio and to each workflow's editor.

The page provides a Store-backed scenario list, execution/data-flow canvas,
reorderable step list, request preview, parameter-source selector, output
configuration, and asynchronous run results with Evidence links. It follows the
step editor and data-source interaction described in the
[Apifox scenario guide](https://docs.apifox.com/new-test-scenario) and
[step data guide](https://docs.apifox.com/pass-data-between-test-steps).

Add registered API cases as ordered steps. Select a preceding step, request or
response body, and a dot path such as `$.data.id` or `$.items.0.id` for a template
parameter. The current native request renderer uses `{{override:name}}` tokens.
The editor discovers those parameters from existing requests and preserves their
base configuration. Saving compiles the references into unique export names and
step-scoped request overrides. Shared case definitions remain unchanged.

The canvas distinguishes execution order from parameter references. Dragging
nodes changes layout; reorder steps in the step list to change execution order.
Moving or deleting a producer leaves its invalid references visible and blocks
saving until they are repaired. A missing selected field fails materialization
before the consumer sends its HTTP request.

`GET /api/workflow-studio` returns workflows, steps, safe request previews, cases,
template parameter names, response fields, the selected profile, and catalog
revision. `PUT` accepts `{workflow, steps}` and requires the strong ETag returned
by `GET`. The existing catalog compare-and-swap persists the workflow,
WorkflowBindings, and step TemplateConfigs as one revision. Other catalog
entities and unknown configuration fields are retained. UI conflicts keep the
local draft and do not retry or overwrite the newer revision.

Positions, parameter references, base requests, and generated names live in the
existing step configuration JSON. No additional Store schema or browser-local
configuration source is introduced. Changes made to compiled request settings
through another entrypoint require review before the studio replaces them.

Run uses the existing Store-native `/api/cases/batch-runs` API. The studio checks
that its catalog version is current before starting, then displays persisted
step statuses and Evidence. Step success and environment acceptance are shown
separately; missing real SkyWalking topology does not become successful
environment verification. This does not pin concurrent changes throughout an
entire run; that remains the execution API's existing contract.

Build the full frontend with `npm run build:frontend`. For an isolated studio
build during development, run `node control-plane/frontend/buildWorkflowStudio.mjs`.
The standalone bundle avoids changing shared chunks for other workbench pages.
