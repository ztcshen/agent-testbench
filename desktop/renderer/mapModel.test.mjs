import test from "node:test";
import assert from "node:assert/strict";
import { mapIndex, filterMap, mapLayout, planNodeStatus } from "./mapModel.mjs";

const document = {
  graph: {
    map: { id: "map.generic" },
    nodes: [
      {
        id: "create",
        caseId: "case.create",
        interfaceNodeId: "api.create",
        role: "primary",
      },
      { id: "read", caseId: "case.read", role: "primary" },
      {
        id: "empty",
        caseId: "case.empty",
        role: "validation",
        anchorNodeId: "create",
      },
    ],
    edges: [],
    paths: [{ id: "path.one" }],
    pathSteps: [
      { pathId: "path.one", nodeId: "read", stepIndex: 2 },
      { pathId: "path.one", nodeId: "create", stepIndex: 1 },
    ],
  },
  context: {
    cases: [
      { id: "case.create", displayName: "Create item" },
      { id: "case.empty", displayName: "Empty name" },
    ],
    interfaces: [{ id: "api.create", method: "POST", path: "/items" }],
  },
};
test("map projection derives display relations while keeping the Store graph unchanged", () => {
  const before = JSON.stringify(document);
  const index = mapIndex(document);
  assert.equal(index.byID.get("create").name, "Create item");
  assert.equal(index.edges.length, 2);
  assert(index.edges.every((edge) => edge.derived));
  assert.equal(JSON.stringify(document), before);
  const primary = filterMap(index, { pathId: "path.one" });
  assert.equal(primary.nodes.length, 2);
  assert.equal(primary.nodes[0].hiddenCount, 1);
  assert.equal(
    filterMap(index, { pathId: "path.one", expanded: ["create"] }).nodes.length,
    3,
  );
  const searched = filterMap(index, {
    pathId: "path.one",
    query: "Empty name",
  });
  assert.equal(searched.matches, 1);
  assert(searched.nodes.some((node) => node.id === "empty"));
  const layout = mapLayout(primary);
  assert(layout.nodes[1].position.x > layout.nodes[0].position.x);
});
test("a partial plan never becomes a passed node overlay", () => {
  assert.equal(planNodeStatus([]), "");
  assert.equal(
    planNodeStatus([{ status: "passed" }, { status: "planned" }]),
    "planned",
  );
  assert.equal(
    planNodeStatus([{ status: "passed" }, { status: "failed" }]),
    "failed",
  );
});
