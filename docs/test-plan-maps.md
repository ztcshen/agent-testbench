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
agent-testbench map import-workflows --store STORE_NAME --map map.checkout --display-name "Checkout Map" --json
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

## Explain A Target Case / 解释目标 Case

Use `map explain` to see which path prefix should be replayed and which single
case operation should run:

```bash
agent-testbench map explain --store STORE_NAME --map MAP_ID --case CASE_ID --json
```

The JSON output includes:

- `logicalPath`: the selected replay prefix.
- `candidatePaths`: paths considered by the planner.
- `rejectedReasons`: why other paths were not selected.
- `operations`: physical steps such as `run_path_prefix` and `run_case`.

For patch-based validation cases, the target node should carry
`baseCaseId`, `patchJson`, and `stateEffect=unchanged` so the planner can reuse
the base case precondition and run only the patched request.
