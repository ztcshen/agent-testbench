const dashboardState = {
  snapshot: null,
  catalog: null,
};

const knownPlatformNodes = ["apollo", "xxl-job"];
const topologyRoleLabels = {
  app: "App",
  platform: "Platform",
  support: "Support",
  middleware: "Middleware",
  external: "External",
};
const el = (id) => document.getElementById(id);

function setDashboardMessage(value) {
  el("dashboardMessage").textContent = value;
}

async function dashboardRequest(path) {
  const response = await fetch(path);
  const body = await response.json();
  if (!response.ok) {
    throw new Error(body.error || response.statusText);
  }
  return body;
}

function statusText(item) {
  if (item.state === "missing") {
    return "未运行";
  }
  if (item.health && item.health !== "unknown") {
    return item.health;
  }
  return item.state || "unknown";
}

function renderEnvironmentOverviewTemplate() {
  const snapshot = dashboardState.snapshot;
  const catalog = dashboardState.catalog || {};
  const summary = snapshot.summary || {};
  el("dashboardSummary").textContent = `${summary.healthy || 0}/${summary.total || 0} healthy · ${summary.missing || 0} missing`;
  el("healthyCount").textContent = summary.healthy || 0;
  el("missingCount").textContent = summary.missing || 0;
  el("unhealthyCount").textContent = summary.unhealthy || 0;

  const business = (snapshot.groups || []).find((group) => group.id === "business");
  el("businessCount").textContent = business?.items?.length || 0;

  renderTopology(catalog.services || [], catalog.topology || {}, catalog.workflows || []);
  renderCatalogSource(catalog.source || {}, catalog.schemaVersion, catalog.warnings || []);

  if (!knownPlatformNodes.every((id) => JSON.stringify(snapshot).includes(id))) {
    setDashboardMessage("平台组件列表不完整：apollo / xxl-job 未返回");
  }
}

function renderCatalogSource(source, schemaVersion, warnings = []) {
  const target = el("catalogSourceStatus");
  if (!target) return;
  const warningCount = warnings.length;
  const warningDetails = warnings.join("\n");
  const ok = source.ok !== false;
  target.className = `catalog-source-status ${ok && !warningCount ? "ok" : "warning"}`;
  const name = source.kind === "manifest" ? "Manifest" : "Catalog";
  const path = source.path ? source.path.split("/").slice(-2).join("/") : "unknown";
  const version = schemaVersion ? ` v${schemaVersion}` : "";
  const warningText = warningCount ? ` · ${warningCount} 警告` : "";
  target.textContent = ok ? `${name}${version}${warningText}` : `${name}${version}: 使用 fallback${warningText}`;
  target.title = [ok ? source.path || "" : source.error || "Catalog manifest unavailable", warningDetails].filter(Boolean).join("\n");
}

function dashboardStatusById() {
  const byId = new Map();
  (dashboardState.snapshot?.groups || []).forEach((group) => {
    (group.items || []).forEach((item) => byId.set(item.id, item));
  });
  return byId;
}

function renderConfiguredActions(targetId, actions = []) {
  const target = el(targetId);
  if (!target) return;
  target.innerHTML = "";
  actions.forEach((action) => {
    const link = document.createElement("a");
    link.className = `button-link ${action.variant === "primary" ? "primary-link" : ""}`.trim();
    link.href = action.href || "#";
    link.textContent = action.label || action.id || "-";
    target.appendChild(link);
  });
}

function workflowUsageByService(workflows) {
  const usage = new Map();
  (workflows || []).forEach((workflow) => {
    const serviceIds = [...new Set((workflow.steps || []).map((step) => step.serviceId).filter(Boolean))];
    serviceIds.forEach((serviceId) => {
      const workflowIds = usage.get(serviceId) || new Set();
      workflowIds.add(workflow.id || "");
      usage.set(serviceId, workflowIds);
    });
  });
  return new Map([...usage.entries()].map(([serviceId, workflowIds]) => [serviceId, workflowIds.size]));
}

function unmodeledWorkflowServices(services, workflows) {
  const modeled = new Set((services || []).map((service) => service.id));
  const referenced = new Set();
  (workflows || []).forEach((workflow) => {
    (workflow.steps || []).forEach((step) => {
      if (step.serviceId && !modeled.has(step.serviceId)) {
        referenced.add(step.serviceId);
      }
    });
  });
  return [...referenced].sort();
}

function renderTopology(services, topology, workflows = []) {
  const rail = el("topologyRail");
  rail.innerHTML = "";
  const edgeCount = topology.edges?.length || 0;
  el("topologySummary").textContent = services.length ? `${services.length} 个服务节点 · ${edgeCount} 条真实边` : "暂无服务定义";
  if (!services.length) {
    const empty = document.createElement("div");
    empty.className = "dashboard-empty";
    empty.textContent = "Catalog 暂未返回服务拓扑。";
    rail.appendChild(empty);
    return;
  }

  const statusById = dashboardStatusById();
  const usageByService = workflowUsageByService(workflows);
  const unmodeledServices = unmodeledWorkflowServices(services, workflows);
  const byId = new Map(services.map((service) => [service.id, service]));
  const orderedServices = (topology.nodes || []).map((id) => byId.get(id)).filter(Boolean);
  services.forEach((service) => {
    if (!orderedServices.includes(service)) {
      orderedServices.push(service);
    }
  });

  rail.appendChild(renderTopologyDirectedGraph(orderedServices, topology.edges || [], statusById, usageByService));
  rail.appendChild(renderTopologyEdgeList(topology.edges || []));

  if (unmodeledServices.length) {
    const gap = document.createElement("div");
    gap.className = "topology-gap";
    gap.textContent = `未建模服务: ${unmodeledServices.join(", ")}`;
    rail.appendChild(gap);
  }
}

function topologyAdjacency(edges) {
  const graph = new Map();
  (edges || []).forEach((edge) => {
    const from = edge.from || "";
    const to = edge.to || "";
    if (!from || !to) return;
    if (!graph.has(from)) graph.set(from, { in: [], out: [] });
    if (!graph.has(to)) graph.set(to, { in: [], out: [] });
    graph.get(from).out.push(to);
    graph.get(to).in.push(from);
  });
  graph.forEach((value) => {
    value.in = [...new Set(value.in)].sort();
    value.out = [...new Set(value.out)].sort();
  });
  return graph;
}

function renderTopologyDirectedGraph(services, edges, statusById, usageByService) {
  const graph = document.createElement("section");
  graph.className = "topology-directed-graph";
  graph.setAttribute("aria-label", "有向服务拓扑图");

  const adjacency = topologyAdjacency(edges);
  const roles = ["app", "support", "middleware", "platform", "external"];
  roles.forEach((role) => {
    const roleServices = services.filter((service) => service.role === role);
    if (!roleServices.length) return;

    const group = document.createElement("section");
    group.className = "topology-graph-group";
    const head = document.createElement("div");
    head.className = "topology-role-head";
    const title = document.createElement("strong");
    title.textContent = topologyRoleLabels[role] || role;
    const count = document.createElement("span");
    count.textContent = `${roleServices.length} nodes`;
    head.appendChild(title);
    head.appendChild(count);
    group.appendChild(head);

    const grid = document.createElement("div");
    grid.className = "topology-graph-grid";
    roleServices.forEach((service) => {
      grid.appendChild(renderTopologyGraphNode(
        service,
        statusById.get(service.id),
        usageByService.get(service.id) || 0,
        adjacency.get(service.id) || { in: [], out: [] }
      ));
    });
    group.appendChild(grid);
    graph.appendChild(group);
  });
  return graph;
}

function renderTopologyGraphNode(service, runtime, usageCount, neighbors) {
  const node = renderTopologyNode(service, runtime, usageCount);
  node.classList.add("topology-graph-node");
  node.appendChild(renderTopologyNeighborList("入边", neighbors.in));
  node.appendChild(renderTopologyNeighborList("出边", neighbors.out));
  return node;
}

function renderTopologyNeighborList(label, items) {
  const row = document.createElement("div");
  row.className = "topology-neighbor-list";
  const title = document.createElement("span");
  title.textContent = label;
  row.appendChild(title);
  const list = document.createElement("div");
  list.className = "topology-neighbor-chips";
  if (!items.length) {
    const empty = document.createElement("em");
    empty.textContent = "-";
    list.appendChild(empty);
  } else {
    items.forEach((item) => {
      const chip = document.createElement("a");
      chip.href = `/environment-node.html?id=${encodeURIComponent(item)}`;
      chip.textContent = item;
      list.appendChild(chip);
    });
  }
  row.appendChild(list);
  return row;
}

function topologyNodeHref(service) {
  if (service.role === "external") {
    return "/service-inventory.html";
  }
  return `/environment-node.html?id=${encodeURIComponent(service.id || "")}`;
}

function renderTopologyNode(service, runtime, usageCount) {
  const node = document.createElement("article");
  node.className = `topology-node ${runtime?.ok ? "ok" : runtime?.state === "missing" ? "missing" : "unknown"}`;

  const title = document.createElement("a");
  title.className = "topology-node-link";
  title.href = topologyNodeHref(service);
  title.textContent = service.displayName || service.id;

  const meta = document.createElement("span");
  const bits = [service.role, service.port ? `:${service.port}` : "", runtime ? statusText(runtime) : "catalog"].filter(Boolean);
  meta.textContent = bits.join(" · ");

  const deps = document.createElement("p");
  deps.textContent = service.dependencies?.length ? `下游: ${service.dependencies.join(", ")}` : "下游: -";
  const usage = document.createElement("p");
  usage.className = "workflow-usage";
  const usageButton = document.createElement("button");
  usageButton.type = "button";
  usageButton.className = "workflow-usage-button";
  usageButton.textContent = `Workflow 使用: ${usageCount}`;
  usageButton.addEventListener("click", () => applyTopologyServiceFilter(service));
  usage.appendChild(usageButton);

  node.appendChild(title);
  node.appendChild(meta);
  node.appendChild(deps);
  node.appendChild(usage);
  return node;
}

function renderTopologyEdgeList(edges) {
  const panel = document.createElement("section");
  panel.className = "topology-edge-list";
  const head = document.createElement("div");
  head.className = "topology-role-head";
  const title = document.createElement("strong");
  title.textContent = "真实边";
  const count = document.createElement("span");
  count.textContent = `${edges.length} edges`;
  head.appendChild(title);
  head.appendChild(count);
  panel.appendChild(head);

  if (!edges.length) {
    const empty = document.createElement("p");
    empty.className = "dashboard-empty";
    empty.textContent = "Catalog 未声明 topology edge。";
    panel.appendChild(empty);
    return panel;
  }

  edges.forEach((edge) => {
    const item = document.createElement("div");
    item.className = "topology-edge-item";
    const from = document.createElement("strong");
    from.textContent = edge.from || "-";
    const arrow = document.createElement("span");
    arrow.textContent = "->";
    const to = document.createElement("strong");
    to.textContent = edge.to || "-";
    item.appendChild(from);
    item.appendChild(arrow);
    item.appendChild(to);
    panel.appendChild(item);
  });
  return panel;
}

function applyTopologyServiceFilter(service) {
  window.location.href = `/workflows.html?workflowFilter=${encodeURIComponent(service.displayName || service.id || "")}`;
}

async function refreshDashboard() {
  setDashboardMessage("refreshing...");
  const [snapshot, catalog] = await Promise.all([
    dashboardRequest("/api/dashboard"),
    dashboardRequest("/api/catalog"),
  ]);
  dashboardState.snapshot = snapshot;
  dashboardState.catalog = catalog;
  renderEnvironmentOverviewTemplate();
  setDashboardMessage("ready");
}

el("refreshDashboardBtn").addEventListener("click", () => refreshDashboard().catch((error) => setDashboardMessage(error.message)));
refreshDashboard().catch((error) => setDashboardMessage(error.message));
