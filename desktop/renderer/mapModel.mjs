import dagre from "@dagrejs/dagre";

export function parseMapJSON(raw, otherwise = {}) {
  if (typeof raw === "object" && raw !== null) return raw;
  try {
    return JSON.parse(raw || "") ?? otherwise;
  } catch {
    return otherwise;
  }
}
export const validationNode = (node) =>
  ["validation", "negative"].includes(node.role) ||
  node.stateEffect === "unchanged";

export function mapIndex(document) {
  const graph = document.graph || {
    map: {},
    nodes: [],
    edges: [],
    paths: [],
    pathSteps: [],
    materializations: [],
  };
  const cases = new Map(
    (document.context?.cases || []).map((item) => [item.id, item]),
  );
  const interfaces = new Map(
    (document.context?.interfaces || []).map((item) => [item.id, item]),
  );
  const steps = new Map((graph.paths || []).map((path) => [path.id, []]));
  for (const item of graph.pathSteps || []) {
    if (!steps.has(item.pathId)) steps.set(item.pathId, []);
    steps.get(item.pathId).push(item);
  }
  for (const items of steps.values())
    items.sort((a, b) => a.stepIndex - b.stepIndex);
  const byCase = new Map(
    (graph.nodes || [])
      .filter((node) => !validationNode(node))
      .map((node) => [node.caseId, node.id]),
  );
  const byInterface = new Map(
    (graph.nodes || [])
      .filter((node) => !validationNode(node))
      .map((node) => [node.interfaceNodeId, node.id]),
  );
  const nodes = (graph.nodes || []).map((node) => {
    const item = cases.get(node.caseId) || {};
    const endpoint = interfaces.get(node.interfaceNodeId || item.nodeId) || {};
    const summary = parseMapJSON(node.summaryJson);
    const memberships = [...steps.entries()]
      .filter(([, items]) => items.some((step) => step.nodeId === node.id))
      .map(([id]) => id);
    const anchor =
      node.anchorNodeId ||
      byCase.get(node.baseCaseId) ||
      (validationNode(node)
        ? byInterface.get(node.interfaceNodeId || item.nodeId)
        : "");
    return {
      ...node,
      name: item.displayName || summary.displayName || node.caseId || node.id,
      description: item.description || summary.description || "",
      method: endpoint.method || "",
      address: endpoint.path || "",
      serviceId: endpoint.serviceId || "",
      interfaceName: endpoint.displayName || "",
      caseDef: item,
      memberships,
      anchor,
    };
  });
  const byID = new Map(nodes.map((node) => [node.id, node]));
  const edges = [...(graph.edges || [])].map((edge) => ({
    ...edge,
    derived: false,
  }));
  const known = new Set(
    edges.map(
      (edge) => `${edge.fromNodeId}\0${edge.toNodeId}\0${edge.pathId || ""}`,
    ),
  );
  for (const [pathId, items] of steps) {
    for (let i = 1; i < items.length; i++) {
      const from = items[i - 1].nodeId,
        to = items[i].nodeId;
      const key = `${from}\0${to}\0${pathId}`;
      if (from === to || known.has(key) || !byID.has(from) || !byID.has(to))
        continue;
      known.add(key);
      edges.push({
        id: `path:${pathId}:${i}`,
        fromNodeId: from,
        toNodeId: to,
        pathId,
        kind: "path",
        derived: true,
      });
    }
  }
  for (const node of nodes) {
    if (
      !validationNode(node) ||
      !node.anchor ||
      !byID.has(node.anchor) ||
      node.anchor === node.id
    )
      continue;
    const key = `${node.anchor}\0${node.id}\0`;
    if (!known.has(key)) {
      known.add(key);
      edges.push({
        id: `validation:${node.id}`,
        fromNodeId: node.anchor,
        toNodeID: node.id,
        toNodeId: node.id,
        kind: "validation",
        derived: true,
      });
    }
  }
  return {
    graph,
    nodes,
    byID,
    edges: edges.filter(
      (edge) => byID.has(edge.fromNodeId) && byID.has(edge.toNodeId),
    ),
    steps,
  };
}

export function filterMap(
  index,
  { pathId = "", query = "", expanded = [], allValidations = false } = {},
) {
  const needle = query.trim().toLowerCase();
  const matching = new Set(
    index.nodes
      .filter((node) =>
        [node.name, node.id, node.caseId, node.method, node.address]
          .join(" ")
          .toLowerCase()
          .includes(needle),
      )
      .map((node) => node.id),
  );
  const inPath = new Set(
    pathId
      ? (index.steps.get(pathId) || []).map((step) => step.nodeId)
      : index.nodes
          .filter((node) => !validationNode(node))
          .map((node) => node.id),
  );
  const visible = new Set();
  for (const node of index.nodes) {
    const belongs =
      inPath.has(node.id) || (node.anchor && inPath.has(node.anchor));
    if (!belongs) continue;
    if (
      !validationNode(node) ||
      allValidations ||
      expanded.includes(node.anchor) ||
      (needle && matching.has(node.id))
    )
      visible.add(node.id);
  }
  if (needle)
    for (const id of matching) {
      const node = index.byID.get(id);
      if (node && (!pathId || inPath.has(id) || inPath.has(node.anchor))) {
        visible.add(id);
        if (node.anchor) visible.add(node.anchor);
      }
    }
  const hiddenByAnchor = new Map();
  for (const node of index.nodes)
    if (validationNode(node) && node.anchor && !visible.has(node.id))
      hiddenByAnchor.set(
        node.anchor,
        (hiddenByAnchor.get(node.anchor) || 0) + 1,
      );
  const nodes = index.nodes
    .filter((node) => visible.has(node.id))
    .map((node) => ({
      ...node,
      hiddenCount: hiddenByAnchor.get(node.id) || 0,
      matched: Boolean(needle && matching.has(node.id)),
    }));
  const edges = index.edges.filter(
    (edge) =>
      visible.has(edge.fromNodeId) &&
      visible.has(edge.toNodeId) &&
      (!pathId || !edge.pathId || edge.pathId === pathId),
  );
  return {
    nodes,
    edges,
    hidden: index.nodes.length - nodes.length,
    matches: needle ? nodes.filter((node) => node.matched).length : 0,
  };
}

export function mapLayout(
  view,
  { direction = "LR", selected = "", plan } = {},
) {
  const graph = new dagre.graphlib.Graph({ multigraph: true })
    .setGraph({
      rankdir: direction,
      nodesep: 32,
      ranksep: 70,
      marginx: 32,
      marginy: 32,
    })
    .setDefaultEdgeLabel(() => ({}));
  for (const node of view.nodes)
    graph.setNode(node.id, { width: 236, height: 108 });
  for (const edge of view.edges)
    graph.setEdge(edge.fromNodeId, edge.toNodeId, {}, edge.id);
  dagre.layout(graph);
  const tasksByNode = new Map();
  for (const task of plan?.tasks || []) {
    if (task.nodeId) {
      if (!tasksByNode.has(task.nodeId)) tasksByNode.set(task.nodeId, []);
      tasksByNode.get(task.nodeId).push(task);
    }
  }
  const nodes = view.nodes.map((node) => {
    const point = graph.node(node.id);
    return {
      id: node.id,
      type: "mapNode",
      position: { x: point.x - 118, y: point.y - 54 },
      selected: node.id === selected,
      data: { ...node, tasks: tasksByNode.get(node.id) || [] },
      sourcePosition: direction === "LR" ? "right" : "bottom",
      targetPosition: direction === "LR" ? "left" : "top",
    };
  });
  const edges = view.edges.map((edge) => ({
    id: edge.id,
    source: edge.fromNodeId,
    target: edge.toNodeId,
    type: "smoothstep",
    data: edge,
    style: {
      stroke:
        edge.kind === "validation"
          ? "#aa93c5"
          : edge.kind === "fixture"
            ? "#5ca795"
            : "#bcc6d5",
      strokeWidth: edge.kind === "validation" ? 1 : 1.4,
      strokeDasharray: edge.derived ? "5 4" : undefined,
    },
    markerEnd: { type: "arrowclosed", color: "#b4bfce" },
  }));
  return { nodes, edges };
}

export function planNodeStatus(tasks = []) {
  if (!tasks.length) return "";
  if (tasks.some((item) => item.status === "failed")) return "failed";
  if (tasks.some((item) => ["running", "queued"].includes(item.status)))
    return "running";
  if (tasks.every((item) => ["passed", "skipped"].includes(item.status)))
    return tasks.every((item) => item.status === "skipped")
      ? "skipped"
      : "passed";
  return (
    tasks.find((item) => !["passed", "skipped"].includes(item.status))
      ?.status || ""
  );
}
