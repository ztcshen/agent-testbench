# Visual Workflow Studio

Open AgentTestBench Desktop and select **自动化测试**. Launch with
`npm run desktop:start`; see the [desktop guide](desktop.md). The former browser
page is retired. The Store contract and native binding compiler remain shared.

The desktop provides a Store-backed scenario list, visual ordered steps, request
preview, parameter-source selector, output configuration, and asynchronous run
results with Evidence. It follows the
step editor and data-source interaction described in the
[Apifox scenario guide](https://docs.apifox.com/new-test-scenario) and
[step data guide](https://docs.apifox.com/pass-data-between-test-steps).

Add registered API cases as ordered steps. Select a preceding step, request or
response body, and a dot path such as `$.data.id` or `$.items.0.id` for a template
parameter. The current native request renderer uses `{{override:name}}` tokens.
The editor discovers those parameters from existing requests and preserves their
base configuration. Saving compiles the references into unique export names and
step-scoped request overrides. Shared case definitions remain unchanged.

The step sequence displays execution order and parameter references. Drag steps
or use the reorder controls to change execution order.
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
saved changes before starting, then displays persisted step statuses and
Evidence. Store compare-and-swap protects saves; a run follows the catalog version
chosen by the existing executor. Step success and environment acceptance are shown
separately; missing real SkyWalking topology does not become successful
environment verification. Studio input checks use the request configuration
snapshot selected by the executor rather than rereading a newer catalog between
steps.

Select **复用步骤库** to create a versioned step block, or select a continuous
range in a scenario and use **保存为步骤组**. Internal bindings are retained;
inputs from outside the selected range become block parameters. A block can be
inserted repeatedly and into other scenarios. Each invocation has its own step
IDs, fixed values or preceding-step bindings, and pinned definition revision.
Blocks move and delete as a unit; their internal steps can be expanded for review.

The parameter panel supports string, integer, number, boolean, object, array and
explicitly nullable values. Fixed values are checked when saved. Values exported
by preceding steps are converted and checked before sending the consumer request.
Zero, false and literal template-like strings are preserved. Sensitive parameter
values are masked in discovery and run snapshots, and masked round trips retain
the stored value.

`GET /api/step-library` returns active definitions and their workflow references.
`PUT` accepts `{definition}` and `DELETE` accepts `{id}`; both require the current
catalog ETag. Definitions use reserved `step-block` TemplateConfig records inside
the existing transactional catalog. Older revisions remain available while
referenced. Deleting a referenced block returns 409 with its affected workflows.
Updating a definition shows outdated references; each scenario explicitly chooses
whether to upgrade, so editing the library does not silently change a saved scene.

The run detail expands from step to request, response, assertions and the persisted
block revision and actual parameter values. These values come from the recorded
case metadata rather than from the latest editable definition. Click **定位编排步骤**
to return to the selected step, or **查看运行证据** to open its indexed Evidence.
The first implementation supports one level of blocks; nested block definitions
are rejected. The existing Apifox visual style is retained. The parameterized reuse
and namespaced steps also follow established patterns in
[Robot Framework user keywords](https://robotframework.org/robotframework/latest/RobotFrameworkUserGuide.html#creating-user-keywords)
and [Airflow TaskGroups](https://airflow.apache.org/docs/apache-airflow/stable/core-concepts/dags.html#taskgroups).

Build the Electron frontend with `npm run build:frontend`, or the frontend and
execution engine with `npm run desktop:build`. The new renderer imports only the
existing pure scenario draft model and uses no retired browser components or CSS.
