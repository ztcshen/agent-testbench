const workflowCatalogState = {
  catalog: null,
  dashboard: null,
};

const workflowCatalogEl = (id) => document.getElementById(id);

function setWorkflowCatalogMessage(value) {
  workflowCatalogEl("workflowCatalogMessage").textContent = value;
}

async function workflowCatalogRequest(path) {
  const response = await fetch(path);
  const body = await response.json();
  if (!response.ok) {
    throw new Error(body.error || response.statusText);
  }
  return body;
}

function dashboardStatusById() {
  const byId = new Map();
  (workflowCatalogState.dashboard?.groups || []).forEach((group) => {
    (group.items || []).forEach((item) => byId.set(item.id, item));
  });
  return byId;
}

function workflowStepHref(workflowId, stepId) {
  return `/workflow-step.html?workflow=${encodeURIComponent(workflowId || "")}&step=${encodeURIComponent(stepId || "")}`;
}

function workflowCatalogServiceHref(service) {
  if (service?.role === "external") {
    return `/service-inventory.html#service-${encodeURIComponent(service.id || "external")}`;
  }
  return `/environment-node.html?id=${encodeURIComponent(service?.id || "")}`;
}

function workflowServiceSearchText(workflow, services) {
  const serviceById = new Map((services || []).map((service) => [service.id, service]));
  return (workflow.steps || [])
    .map((step) => {
      if (!step.serviceId) return "";
      const service = serviceById.get(step.serviceId);
      return service
        ? [service.id, service.displayName, service.role].filter(Boolean).join(" ")
        : `${step.serviceId} 未建模`;
    })
    .join(" ");
}

function workflowRuntimeImpact(workflow, statusById) {
  const serviceIds = [...new Set((workflow.steps || []).map((step) => step.serviceId).filter(Boolean))];
  const runtimeItems = serviceIds.map((serviceId) => statusById.get(serviceId)).filter(Boolean);
  const badCount = runtimeItems.filter((item) => !item.ok).length;
  if (!runtimeItems.length) {
    return { text: "运行态未覆盖", tone: "unknown" };
  }
  return badCount ? { text: `${badCount} 异常服务`, tone: "bad" } : { text: "服务正常", tone: "ok" };
}

function filterWorkflowCatalog(workflows, services = [], statusById = new Map()) {
  const query = workflowCatalogEl("workflowFilter")?.value.trim().toLowerCase() || "";
  if (!query) return workflows;
  return workflows.filter((workflow) => {
    const stepText = (workflow.steps || [])
      .map((step) => [step.id, step.displayName, step.caseId, step.serviceId, step.action, ...(step.evidenceKinds || [])].filter(Boolean).join(" "))
      .join(" ");
    const serviceText = workflowServiceSearchText(workflow, services);
    const impactText = workflowRuntimeImpact(workflow, statusById).text;
    const text = [workflow.id, workflow.displayName, workflow.description, stepText, serviceText, impactText].filter(Boolean).join(" ");
    return text.toLowerCase().includes(query);
  });
}

function renderCatalogSource(source, schemaVersion, warnings = []) {
  const target = workflowCatalogEl("catalogSourceStatus");
  const warningCount = warnings.length;
  const warningDetails = warnings.join("\n");
  const ok = source?.ok !== false;
  target.className = `catalog-source-status ${ok && !warningCount ? "ok" : "warning"}`;
  const name = source?.kind === "manifest" ? "Manifest" : "Catalog";
  const path = source?.path ? source.path.split("/").slice(-2).join("/") : "unknown";
  const version = schemaVersion ? ` v${schemaVersion}` : "";
  const warningText = warningCount ? ` · ${warningCount} 警告` : "";
  target.textContent = ok ? `${name}${version}${warningText}` : `${name}${version}: fallback${warningText}`;
  target.title = [ok ? source?.path || "" : source?.error || "Catalog manifest unavailable", warningDetails].filter(Boolean).join("\n");
}

function renderWorkflowServiceSummary(workflow, services) {
  const serviceById = new Map((services || []).map((service) => [service.id, service]));
  const serviceIds = [...new Set((workflow.steps || []).map((step) => step.serviceId).filter(Boolean))];
  const summary = document.createElement("div");
  summary.className = "workflow-service-summary";

  const label = document.createElement("span");
  label.textContent = "服务";
  summary.appendChild(label);

  if (!serviceIds.length) {
    const empty = document.createElement("code");
    empty.textContent = "未声明服务";
    summary.appendChild(empty);
    return summary;
  }

  serviceIds.slice(0, 6).forEach((serviceId) => {
    const service = serviceById.get(serviceId);
    if (!service) {
      const chip = document.createElement("code");
      chip.className = "unknown";
      chip.textContent = `${serviceId} · 未建模`;
      summary.appendChild(chip);
    } else {
      const chip = document.createElement("a");
      chip.className = "workflow-service-link";
      chip.href = workflowCatalogServiceHref(service);
      chip.textContent = service.displayName || service.id;
      summary.appendChild(chip);
    }
  });

  if (serviceIds.length > 6) {
    const more = document.createElement("code");
    more.textContent = `+${serviceIds.length - 6}`;
    summary.appendChild(more);
  }
  return summary;
}

function updateWorkflowFilterControls() {
  const filter = workflowCatalogEl("workflowFilter");
  workflowCatalogEl("clearWorkflowFilterBtn").disabled = !(filter?.value.trim());
}

function applyWorkflowCatalogFilter(value) {
  const filter = workflowCatalogEl("workflowFilter");
  filter.value = value;
  filter.focus();
  refreshWorkflowCatalogFilter();
}

function clearWorkflowFilter() {
  applyWorkflowCatalogFilter("");
}

function renderWorkflowCatalog(workflows, totalCount = workflows.length) {
  const target = workflowCatalogEl("workflowCatalog");
  target.innerHTML = "";
  const catalog = workflowCatalogState.catalog || {};
  const services = catalog.services || [];
  const statusById = dashboardStatusById();
  const query = workflowCatalogEl("workflowFilter")?.value.trim();
  updateWorkflowFilterControls();
  workflowCatalogEl("workflowSummary").textContent = query ? `${workflows.length}/${totalCount} 个 Workflow` : totalCount ? `${totalCount} 个 Workflow` : "暂无 Workflow";
  workflowCatalogEl("workflowCatalogSummary").textContent = `${totalCount || 0} 个模板化 Workflow · ${services.length || 0} 个服务`;
  if (!workflows.length) {
    const empty = document.createElement("div");
    empty.className = "dashboard-empty";
    empty.textContent = query ? "没有匹配的 Workflow。" : "Catalog 暂未返回 Workflow 定义。";
    target.appendChild(empty);
    return;
  }

  workflows.forEach((workflow) => {
    const card = document.createElement("article");
    card.className = "workflow-catalog-card";

    const top = document.createElement("div");
    top.className = "workflow-catalog-top";
    const title = document.createElement("strong");
    title.textContent = workflow.displayName || workflow.id;
    const count = document.createElement("code");
    count.textContent = `${workflow.steps?.length || 0} steps`;
    const impact = workflowRuntimeImpact(workflow, statusById);
    const impactBadge = document.createElement("button");
    impactBadge.type = "button";
    impactBadge.className = `workflow-impact workflow-impact-button ${impact.tone}`;
    impactBadge.textContent = impact.text;
    impactBadge.addEventListener("click", () => applyWorkflowCatalogFilter(impact.text));
    top.appendChild(title);
    top.appendChild(count);
    top.appendChild(impactBadge);

    const desc = document.createElement("p");
    desc.textContent = workflow.description || "-";
    const serviceSummary = renderWorkflowServiceSummary(workflow, services);

    const steps = document.createElement("div");
    steps.className = "workflow-step-strip";
    (workflow.steps || []).slice(0, 12).forEach((step) => {
      const chip = document.createElement("a");
      chip.href = workflowStepHref(workflow.id, step.id);
      chip.textContent = step.displayName || step.id;
      steps.appendChild(chip);
    });

    const actions = document.createElement("div");
    actions.className = "dashboard-card-actions";
    const detailLink = document.createElement("a");
    detailLink.className = "button-link";
    detailLink.href = `/workflow-detail.html?id=${encodeURIComponent(workflow.id || "")}`;
    detailLink.textContent = "查看定义";
    const openLink = document.createElement("a");
    openLink.className = "button-link";
    openLink.href = `/workflow-detail.html?id=${encodeURIComponent(workflow.id || "")}`;
    openLink.textContent = "打开 Workflow";
    actions.appendChild(detailLink);
    actions.appendChild(openLink);

    card.appendChild(top);
    card.appendChild(desc);
    card.appendChild(serviceSummary);
    card.appendChild(steps);
    card.appendChild(actions);
    target.appendChild(card);
  });
}

function refreshWorkflowCatalogFilter() {
  const catalog = workflowCatalogState.catalog || {};
  const workflows = catalog.workflows || [];
  renderWorkflowCatalog(filterWorkflowCatalog(workflows, catalog.services || [], dashboardStatusById()), workflows.length);
}

function seedWorkflowFilterFromUrl() {
  const searchParams = new URLSearchParams(window.location.search);
  const value = searchParams.get("workflowFilter");
  if (!value) return;
  workflowCatalogEl("workflowFilter").value = value;
  updateWorkflowFilterControls();
}

async function refreshWorkflowCatalog() {
  setWorkflowCatalogMessage("refreshing...");
  const [catalog, dashboard] = await Promise.all([
    workflowCatalogRequest("/api/catalog"),
    workflowCatalogRequest("/api/dashboard"),
  ]);
  workflowCatalogState.catalog = catalog;
  workflowCatalogState.dashboard = dashboard;
  renderCatalogSource(catalog.source || {}, catalog.schemaVersion, catalog.warnings || []);
  refreshWorkflowCatalogFilter();
  setWorkflowCatalogMessage("ready");
}

workflowCatalogEl("refreshWorkflowCatalogBtn").addEventListener("click", () => refreshWorkflowCatalog().catch((error) => setWorkflowCatalogMessage(error.message)));
workflowCatalogEl("workflowFilter").addEventListener("input", refreshWorkflowCatalogFilter);
workflowCatalogEl("clearWorkflowFilterBtn").addEventListener("click", clearWorkflowFilter);
seedWorkflowFilterFromUrl();
refreshWorkflowCatalog().catch((error) => setWorkflowCatalogMessage(error.message));
