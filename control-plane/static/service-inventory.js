const inventoryEl = (id) => document.getElementById(id);

const roleLabels = {
  app: "App",
  platform: "Platform",
  support: "Support",
  middleware: "Middleware",
  external: "External",
};

async function serviceInventoryRequest(path) {
  const response = await fetch(path);
  const body = await response.json();
  if (!response.ok) {
    throw new Error(body.error || response.statusText);
  }
  return body;
}

function setServiceInventoryStatus(value) {
  inventoryEl("serviceInventoryStatus").textContent = value;
}

function serviceInventoryRuntimeById(snapshot) {
  const byId = new Map();
  (snapshot?.groups || []).forEach((group) => {
    (group.items || []).forEach((item) => byId.set(item.id, item));
  });
  return byId;
}

function serviceInventoryStatusText(runtime) {
  if (!runtime) {
    return "未纳入运行快照";
  }
  if (runtime.state === "missing") {
    return "离线";
  }
  if (runtime.health && runtime.health !== "unknown") {
    return runtime.health;
  }
  return runtime.state || "unknown";
}

function renderServiceInventorySource(catalog) {
  const target = inventoryEl("serviceInventorySource");
  const warnings = catalog.warnings || [];
  const ok = catalog.source?.ok !== false;
  target.className = `catalog-source-status ${ok && !warnings.length ? "ok" : "warning"}`;
  const source = catalog.source?.kind === "manifest" ? "Manifest" : "Catalog";
  const version = catalog.schemaVersion ? ` v${catalog.schemaVersion}` : "";
  const warningText = warnings.length ? ` · ${warnings.length} warnings` : "";
  target.textContent = ok ? `${source}${version}${warningText}` : `${source}${version}: fallback${warningText}`;
  target.title = [catalog.source?.path || catalog.source?.error || "", warnings.join("\n")].filter(Boolean).join("\n");
}

function renderServiceInventoryStats(services) {
  const stats = inventoryEl("serviceInventoryStats");
  stats.innerHTML = "";
  const counts = services.reduce((acc, service) => {
    const role = service.role || "unknown";
    acc[role] = (acc[role] || 0) + 1;
    return acc;
  }, {});

  ["app", "support", "middleware", "platform", "external"].forEach((role) => {
    const item = document.createElement("div");
    const label = document.createElement("span");
    label.textContent = roleLabels[role] || role;
    const value = document.createElement("strong");
    value.textContent = counts[role] || 0;
    item.appendChild(label);
    item.appendChild(value);
    stats.appendChild(item);
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

function serviceInventoryDetailHref(service) {
  if (service.role === "external") {
    return `#service-${encodeURIComponent(service.id || "external")}`;
  }
  return `/environment-node.html?id=${encodeURIComponent(service.id || "")}`;
}

function renderServiceCard(service, usageCount, runtime) {
  const card = document.createElement("article");
  card.className = `service-inventory-card ${runtime?.ok ? "ok" : runtime?.state === "missing" ? "missing" : "unknown"}`;
  card.id = `service-${service.id || "unknown"}`;

  const top = document.createElement("div");
  top.className = "service-inventory-card-top";
  const title = document.createElement("a");
  title.className = "service-inventory-service-link";
  title.href = serviceInventoryDetailHref(service);
  title.textContent = service.displayName || service.id;
  const role = document.createElement("span");
  role.textContent = serviceInventoryStatusText(runtime);
  top.appendChild(title);
  top.appendChild(role);

  const meta = document.createElement("dl");
  meta.className = "service-inventory-meta";
  [
    ["id", service.id || "-"],
    ["port", service.port ? `:${service.port}` : "-"],
    ["runtime", serviceInventoryStatusText(runtime)],
    ["container", runtime?.container || "-"],
    ["health", runtime?.health || "-"],
    ["repo", service.repoEnv || "-"],
    ["mock", service.mockable ? "yes" : "no"],
    ["downstream", service.dependencies?.length ? service.dependencies.join(", ") : "-"],
    ["workflows", String(usageCount || 0)],
  ].forEach(([key, value]) => {
    const dt = document.createElement("dt");
    dt.textContent = key;
    const dd = document.createElement("dd");
    dd.textContent = value;
    meta.appendChild(dt);
    meta.appendChild(dd);
  });

  card.appendChild(top);
  card.appendChild(meta);
  return card;
}

function renderServiceInventoryGroups(catalog, snapshot) {
  const target = inventoryEl("serviceInventoryGroups");
  target.innerHTML = "";
  const services = catalog.services || [];
  const usage = workflowUsageByService(catalog.workflows || []);
  const runtimeById = serviceInventoryRuntimeById(snapshot);
  const roles = ["app", "platform", "support", "external"];

  roles.forEach((role) => {
    const items = services.filter((service) => service.role === role);
    const group = document.createElement("section");
    group.className = "service-inventory-group";
    const head = document.createElement("div");
    head.className = "service-inventory-group-head";
    const title = document.createElement("h3");
    title.textContent = roleLabels[role] || role;
    const count = document.createElement("span");
    count.textContent = `${items.length} services`;
    head.appendChild(title);
    head.appendChild(count);
    group.appendChild(head);

    const grid = document.createElement("div");
    grid.className = "service-inventory-card-grid";
    if (!items.length) {
      const empty = document.createElement("p");
      empty.className = "dashboard-empty";
      empty.textContent = "No catalog service in this role.";
      grid.appendChild(empty);
    } else {
      items.forEach((service) => grid.appendChild(renderServiceCard(service, usage.get(service.id), runtimeById.get(service.id))));
    }
    group.appendChild(grid);
    target.appendChild(group);
  });
}

function renderServiceInventoryTopology(catalog) {
  const target = inventoryEl("serviceInventoryTopology");
  target.innerHTML = "";
  const edges = catalog.topology?.edges || [];
  inventoryEl("serviceInventoryTopologySummary").textContent = `${edges.length} edges · ${catalog.topology?.nodes?.length || 0} nodes`;

  if (!edges.length) {
    const empty = document.createElement("p");
    empty.className = "dashboard-empty";
    empty.textContent = "No topology edges declared.";
    target.appendChild(empty);
    return;
  }

  target.appendChild(renderServiceInventoryDirectedGraph(catalog.services || [], edges));

  edges.forEach((edge) => {
    const item = document.createElement("div");
    item.className = "service-inventory-edge";
    const from = document.createElement("strong");
    from.textContent = edge.from || "-";
    const arrow = document.createElement("span");
    arrow.textContent = "->";
    const to = document.createElement("strong");
    to.textContent = edge.to || "-";
    item.appendChild(from);
    item.appendChild(arrow);
    item.appendChild(to);
    target.appendChild(item);
  });
}

function renderServiceInventoryTemplate(catalog, snapshot) {
  const services = catalog.services || [];
  const summary = snapshot?.summary || {};
  inventoryEl("serviceInventorySummary").textContent = `${services.length} services · ${summary.healthy || 0}/${summary.total || 0} online`;
  renderServiceInventoryStats(services);
  renderServiceInventorySource(catalog);
  renderServiceInventoryGroups(catalog, snapshot);
  renderServiceInventoryTopology(catalog);
}

async function loadServiceInventory() {
  setServiceInventoryStatus("refreshing...");
  const [catalog, dashboard] = await Promise.all([
    serviceInventoryRequest("/api/catalog"),
    serviceInventoryRequest("/api/dashboard"),
  ]);
  renderServiceInventoryTemplate(catalog, dashboard);
  setServiceInventoryStatus("ready");
}

inventoryEl("refreshServiceInventoryBtn")?.addEventListener("click", () => {
  loadServiceInventory().catch((error) => setServiceInventoryStatus(`failed: ${error.message}`));
});

loadServiceInventory().catch((error) => setServiceInventoryStatus(`failed: ${error.message}`));
