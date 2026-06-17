# Test Plan Maps / 测试计划地图

Test Plan Maps group related workflows into one Store-backed graph. A workflow
is stored as a named path in the map, and each case node stays a single HTTP/MQ
request. The planner uses paths, fixtures, and case dependencies to explain the
precondition replay needed before running a target case.

Test Plan Map 把相关 workflow 收敛成一张 Store-backed 图。workflow 在图里是
named path，case node 仍然表示一次 HTTP/MQ 请求。planner 通过 path、fixture
和 case dependency 解释运行目标 case 前需要回放哪段前置路径。

## When To Create A New Map / 什么时候建新 Map

Create a new map for one coherent product capability or acceptance surface when
multiple workflows share setup, state transitions, fixtures, or interface cases.
Do not create one map per workflow or one map per negative case; that keeps the
old duplication problem.

当多个 workflow 共享前置状态、状态流转、fixture 或接口 case 时，为同一个能力域或
验收面建一张 map。不要为每条 workflow 或每个负向 case 单独建 map，否则仍然会产生
重复资产。

Use a separate map when the workflows belong to a different profile, runtime
boundary, environment contract, or independent capability whose data setup should
not be planned together.

当 workflow 属于不同 profile、不同 runtime 边界、不同环境契约，或者数据前置不应
一起规划的独立能力时，再拆成另一张 map。

## Build A Map / 构建 Map

Keep the Store catalog as the source of truth first:

- `Workflow`: the named end-to-end path to preserve.
- `WorkflowBinding`: ordered steps that bind workflow steps to case nodes.
- `APICase`: the single HTTP/MQ request case, including patch-based validation
  cases.
- `Fixture` and `CaseDependency`: materialized preconditions for cases that need
  replayed state.

Then import the graph into the active Store:

```bash
agent-testbench map import-workflows --store STORE_NAME --json
```

Use `--map ID` only when intentionally naming a separate map:

```bash
agent-testbench map import-workflows --store STORE_NAME --map map.capability --display-name "Capability Map" --json
```

The import replaces the same map id with the current catalog projection. It
writes `test_maps`, `test_map_nodes`, `test_map_edges`, `test_map_paths`,
`test_map_path_steps`, and `test_plan_materializations`.

导入同一个 map id 时会用当前 catalog 投影替换旧图，写入上述 Store 表。

## Find Workflows In A Map / 通过 Map 搜 Workflow

List all workflow paths in a map:

```bash
agent-testbench map workflows --store STORE_NAME --map MAP_ID --json
```

Search by path id, workflow id, or display name:

```bash
agent-testbench map workflows --store STORE_NAME --map MAP_ID --filter cancel --json
```

Each row reports the path id, workflow id, display name, step count, first node,
and last node. Use the path id when discussing the workflow as part of the map.

每条记录会返回 path id、workflow id、名称、step 数、首尾节点。讨论 map 中的
workflow 时优先使用 path id。

## Review A Map / 评审 Map

Generate a self-contained HTML review page from the Store-backed map:

```bash
agent-testbench map review-html --store STORE_NAME --map MAP_ID --filter TEXT --output /tmp/map-review.html --json
```

The review page is built from Store facts, not model inference. It embeds the
map nodes, edges, workflow paths, materializations, catalog case metadata, and
planner explanations. `--filter` narrows the generated review to matching path
ids, workflow ids, display names, node ids, or case ids. Reviewers can search
cases, filter by workflow path, click case nodes, inspect request templates,
patch/expected JSON, workflow reuse, and the replay operations selected by
`map explain`.

这个页面用于人工评审 agent 产出的 map：图上的每个节点是一个 case，颜色来自
workflow path，右侧详情展示 case、请求模板、patch、expected、复用路径以及 planner
选择的前置回放操作。

## Explain A Target Case / 解释目标 Case

Use `map explain` as the map-level SQL-style `EXPLAIN`: it turns the
Store-backed graph into a deterministic planner result with logical operators,
optimizer rule trace, candidate plans, selected physical tasks, task DAG edges,
cost, and required/provided properties.

```bash
agent-testbench map explain --store STORE_NAME --map MAP_ID --case CASE_ID --json
```

Explain the whole map instead of one target case:

```bash
agent-testbench map explain --store STORE_NAME --map MAP_ID --scope all --json
```

Persist the planner instance and task DAG for later execution, gate, or audit:

```bash
agent-testbench map explain --store STORE_NAME --map MAP_ID --scope all --environment ENV_ID --save --json
```

The JSON output includes:

- `logicalPlan`: deterministic logical map operators.
- `rulesApplied`: optimizer/planner rule trace.
- `candidatePlans` and `rejectedPlans`: selected and rejected path/case plans.
- `physicalTasks`: executable tasks such as `run_path`, `run_path_prefix`, and
  `run_case`.
- `taskEdges`: task DAG dependencies; do not infer order only from task index.
- `operations`: compatibility view for single-case replay operations.

For patch-based validation cases, the target node should carry
`baseCaseId`, `patchJson`, and `stateEffect=unchanged` so the planner can reuse
the base case precondition and run only the patched request.

When `--save` is used, the Store writes `test_map_plan_instances`,
`test_map_plan_tasks`, and `test_map_plan_task_edges`. Existing workflow and API
case run tables remain the child execution records through
`runs.test_plan_map_id`, `runs.test_plan_path_id`,
`api_case_runs.test_plan_node_id`, and planner summary JSON fields.

## Run A Map Plan / 执行 Map

Use `map run` after `map explain` looks reasonable. By default, `map run`
creates a fresh `mode=run` planner instance, persists it, executes the physical
task DAG, and writes task status plus child run ids back to the Store.

```bash
agent-testbench map run --store STORE_NAME --map MAP_ID --scope all --environment ENV_ID --json
```

Run a narrower target when reviewing or debugging one path/case:

```bash
agent-testbench map run --store STORE_NAME --map MAP_ID --path PATH_ID --json
agent-testbench map run --store STORE_NAME --map MAP_ID --case CASE_ID --json
```

Run an existing planner instance:

```bash
agent-testbench map run --store STORE_NAME --plan PLAN_ID --json
```

By default, running an existing plan resets all non-skipped tasks and performs a
full rerun. Use resume controls when a large map already has useful child run
evidence:

```bash
agent-testbench map run --store STORE_NAME --plan PLAN_ID --resume --json
agent-testbench map run --store STORE_NAME --plan PLAN_ID --retry-failed --json
agent-testbench map run --store STORE_NAME --plan PLAN_ID --rerun-task TASK_ID --json
```

`--resume` keeps passed/skipped tasks and reruns incomplete, failed, blocked, or
previously running tasks. `--retry-failed` selects only failed or blocked tasks.
`--rerun-task` can be repeated to reset specific task ids while keeping every
other task and child run reference intact. `--skip-passed` is a lighter modifier
for preserving passed/skipped tasks when rerunning an existing plan.

The v1 executor is deliberately deterministic and serial. `run_path` and
`run_path_prefix` execute their mapped path steps as Store catalog API cases and
record an aggregate workflow run for the plan task. `run_case` executes the
target case as a single Store catalog API case. Every child run is linked back
through the test-plan metadata fields, while `test_map_plan_tasks` stores the
task status, `workflow_run_id`, `api_case_run_id`, evidence root, timestamps,
and summary.

Use `map run explain` to inspect a completed or failed run plan without
re-running it:

```bash
agent-testbench map run explain --store STORE_NAME --plan PLAN_ID --json
```

Gate a persisted map run before accepting it as map-level evidence:

```bash
agent-testbench map gate --store STORE_NAME --plan PLAN_ID --require-passed --require-tasks --require-evidence --json
```

`map gate` does not execute target services. It reads the saved planner
instance, checks aggregate plan status, task status, child workflow/API case run
links, and Store Evidence indexes, then reports failed tasks, missing Evidence,
and next actions such as `--retry-failed` or `--rerun-task`.
