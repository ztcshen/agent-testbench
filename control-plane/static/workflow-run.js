const workflowRunEl = (id) => document.getElementById(id);

function setWorkflowRunStatus(value) {
  workflowRunEl("workflowRunStatus").textContent = value;
}

async function workflowRunRequest(path) {
  const response = await fetch(path);
  const body = await response.json();
  if (!response.ok) {
    throw new Error(body.error || response.statusText);
  }
  return body;
}

function formatWorkflowRunTime(value) {
  if (!value) return "-";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  return new Intl.DateTimeFormat("zh-CN", {
    month: "numeric",
    day: "numeric",
    hour: "2-digit",
    minute: "2-digit",
    second: "2-digit",
  }).format(date);
}

function workflowRunKV(label, value) {
  const card = document.createElement("article");
  const key = document.createElement("span");
  key.textContent = label;
  const text = document.createElement("strong");
  text.textContent = value || "-";
  card.appendChild(key);
  card.appendChild(text);
  return card;
}

function workflowRunServiceLinkHref(serviceId, catalog) {
  const service = (catalog.services || []).find((candidate) => candidate.id === serviceId);
  if (service?.role === "external") {
    return "/service-inventory.html";
  }
  return `/environment-node.html?id=${encodeURIComponent(serviceId)}`;
}

function workflowRunStepService(step, run, catalog) {
  const directServiceId = step.serviceId || step.summary?.serviceId || step.summary?.targetServiceId;
  if (directServiceId) {
    return (catalog.services || []).find((service) => service.id === directServiceId) || { id: directServiceId };
  }
  const workflow = (catalog.workflows || []).find((candidate) => candidate.id === run.workflowId);
  const catalogStep = (workflow?.steps || []).find((candidate) => candidate.id === step.stepId);
  if (!catalogStep?.serviceId) {
    return null;
  }
  return (catalog.services || []).find((service) => service.id === catalogStep.serviceId) || { id: catalogStep.serviceId };
}

function traceTopologyHref(runId, traceFilter = "", exitKind = "") {
  const params = new URLSearchParams();
  params.set("workflowRunId", runId || "");
  if (traceFilter) {
    params.set("traceFilter", traceFilter);
  }
  if (exitKind) {
    params.set("exitKind", exitKind);
  }
  return `/trace-topology.html?${params.toString()}`;
}

function workflowStepDetailHref(run, step) {
  const params = new URLSearchParams();
  params.set("workflow", run.workflowId || "");
  params.set("step", step.stepId || "");
  if (run.id) {
    params.set("runId", run.id);
  }
  return `/workflow-step.html?${params.toString()}`;
}

function latestWorkflowRunTopologyHref(runsPayload) {
  const latest = (runsPayload.workflowRuns || []).find((run) => run.id);
  return latest ? traceTopologyHref(latest.id) : "/trace-topology.html";
}

function renderWorkflowRunStepServiceLinks(card, step, run, catalog) {
  const service = workflowRunStepService(step, run, catalog);
  const links = document.createElement("div");
  links.className = "workflow-run-step-service-links";
  if (service?.id) {
    const serviceLink = document.createElement("a");
    serviceLink.href = workflowRunServiceLinkHref(service.id, catalog);
    serviceLink.textContent = service.displayName || service.id;
    links.appendChild(serviceLink);
  }
  if (run.id) {
    const stepLink = document.createElement("a");
    stepLink.href = workflowStepDetailHref(run, step);
    stepLink.textContent = "接口明细";
    links.appendChild(stepLink);
    const topologyLink = document.createElement("a");
    topologyLink.href = traceTopologyHref(run.id, step.stepId || step.summary?.requestId || "");
    topologyLink.textContent = "过滤拓扑";
    links.appendChild(topologyLink);
  }
  card.appendChild(links);
}

function workflowRunStepAnchor(stepId) {
  return `workflow-step-${encodeURIComponent(stepId || "unknown")}`;
}

function renderWorkflowRunStepBodyHealth(card, step) {
  const bodyHealth = step.bodyHealth || {};
  const message = bodyHealth.message || "";
  const level = bodyHealth.level || (bodyHealth.ok === false ? "failed" : "ok");
  if (bodyHealth.ok !== false && !message) {
    return;
  }
  const row = document.createElement("div");
  row.className = `workflow-run-step-body-health ${bodyHealth.ok === false ? "failed" : "passed"}`;
  const label = document.createElement("span");
  label.textContent = "body health";
  const text = document.createElement("strong");
  text.textContent = `${level}${message ? ` · ${message}` : ""}`;
  row.appendChild(label);
  row.appendChild(text);
  card.appendChild(row);
}

function renderWorkflowRunLink(label, href) {
  const link = document.createElement("a");
  link.className = "button-link";
  link.href = href;
  link.textContent = label;
  return link;
}

function renderWorkflowRunObservationBoard(run, summary, catalog) {
  const target = workflowRunEl("workflowRunObservationBoard");
  target.innerHTML = "";
  const workflow = (catalog.workflows || []).find((candidate) => candidate.id === run.workflowId);
  target.hidden = !workflow;
  if (!workflow) return;

  const panels = workflow.observability?.panels || [];
  const head = document.createElement("div");
  head.className = "section-head";
  const titleWrap = document.createElement("div");
  const title = document.createElement("h2");
  title.textContent = "配置化观测";
  const text = document.createElement("p");
  text.textContent = `${panels.length || 0} panels · ${workflow.graph?.nodes?.length || 0} graph nodes · ${summary.steps?.length || 0} runtime steps`;
  titleWrap.appendChild(title);
  titleWrap.appendChild(text);
  head.appendChild(titleWrap);

  const actions = document.createElement("div");
  actions.className = "workflow-run-step-service-links";
  actions.appendChild(renderWorkflowRunLink("Workflow 定义", `/workflow-detail.html?id=${encodeURIComponent(workflow.id || "")}`));
  const firstStep = (workflow.steps || [])[0];
  if (firstStep) {
    actions.appendChild(renderWorkflowRunLink("首个步骤", `/workflow-step.html?workflow=${encodeURIComponent(workflow.id || "")}&step=${encodeURIComponent(firstStep.id || "")}`));
  }
  head.appendChild(actions);

  const grid = document.createElement("div");
  grid.className = "workflow-run-observation-grid";
  panels.forEach((panel) => {
    const card = document.createElement("article");
    card.className = "workflow-run-observation-card";
    const titleLine = document.createElement("strong");
    titleLine.textContent = panel.title || panel.id || panel.type || "-";
    const meta = document.createElement("code");
    meta.textContent = [panel.type || "-", panel.scope || "workflow"].join(" · ");
    card.appendChild(titleLine);
    card.appendChild(meta);
    grid.appendChild(card);
  });

  target.appendChild(head);
  target.appendChild(grid);
}

function renderWorkflowRunSummary(run, summary) {
  workflowRunEl("workflowRunTitle").textContent = `${run.workflowId || "-"} · #${run.id || "-"}`;
  const target = workflowRunEl("workflowRunSummary");
  target.innerHTML = "";
  const finalSummary = summary.summary || {};
  const steps = summary.steps || [];
  [
    ["status", run.status || summary.status],
    ["steps", `${finalSummary.stepCount || steps.length || 0}/${finalSummary.expectedStepCount || steps.length || "-"}`],
    ["elapsed", summary.elapsedMs ? `${summary.elapsedMs} ms` : "-"],
    ["created", formatWorkflowRunTime(run.createdAt)],
  ].forEach(([label, value]) => target.appendChild(workflowRunKV(label, value)));
}

function renderWorkflowRunSteps(run, summary, catalog) {
  const target = workflowRunEl("workflowRunSteps");
  target.innerHTML = "";
  target.classList.remove("workflow-run-missing-grid");
  const steps = summary.steps || [];
  workflowRunEl("workflowRunStepsMeta").textContent = `${steps.length} steps · ${run.workflowId || "-"}`;
  if (!steps.length) {
    const empty = document.createElement("div");
    empty.className = "empty-note";
    empty.textContent = "此 run 没有持久化 step 明细。";
    target.appendChild(empty);
    return;
  }
  steps.forEach((step, index) => {
    const card = document.createElement("article");
    card.className = `workflow-run-step-card ${step.stepOk === false || step.ok === false ? "failed" : "passed"}`;
    card.id = workflowRunStepAnchor(step.stepId || step.caseId || String(index + 1));
    const head = document.createElement("div");
    const title = document.createElement("strong");
    title.textContent = `${String(index + 1).padStart(2, "0")} ${step.title || step.stepId || "-"}`;
    const status = document.createElement("span");
    status.className = `status-pill ${step.stepOk === false || step.ok === false ? "failed" : "passed"}`;
    status.textContent = step.stepOk === false || step.ok === false ? "fail" : "pass";
    head.appendChild(title);
    head.appendChild(status);
    const summaryRow = document.createElement("p");
    const stepSummary = step.summary || {};
    summaryRow.textContent = `${step.stepId || "-"} · http ${stepSummary.httpCode || "-"} · ${stepSummary.requestId || "-"} · ${step.elapsedMs || stepSummary.elapsedMs || "-"} ms`;
    card.appendChild(head);
    card.appendChild(summaryRow);
    renderWorkflowRunStepBodyHealth(card, step);
    renderWorkflowRunStepServiceLinks(card, step, run, catalog || {});
    target.appendChild(card);
  });
}

function renderWorkflowRunTraceTopologies(payload) {
  const target = workflowRunEl("workflowRunTraceTopologies");
  target.innerHTML = "";
  const topologies = payload.traceTopologies || [];
  const runId = payload.run?.id;
  const topologyLink = workflowRunEl("workflowRunTraceTopologyWorkbench");
  if (topologyLink && runId) {
    topologyLink.href = traceTopologyHref(runId);
  }
  workflowRunEl("workflowRunTraceTopologiesMeta").textContent = `${topologies.length} persisted records`;
  if (!topologies.length) {
    const empty = document.createElement("div");
    empty.className = "empty-note";
    empty.textContent = "暂无 SkyWalking 拓扑记录。";
    target.appendChild(empty);
    return;
  }
  topologies.forEach((topology) => {
    const card = document.createElement("article");
    card.className = `workflow-run-trace-card ${topology.status === "complete" ? "complete" : "partial"}`;
    const head = document.createElement("div");
    const title = document.createElement("strong");
    title.textContent = topology.stepId || topology.caseId || "trace";
    const status = document.createElement("span");
    status.className = `status-pill ${topology.status === "complete" ? "passed" : "failed"}`;
    status.textContent = topology.status || "unknown";
    head.appendChild(title);
    head.appendChild(status);
    const meta = document.createElement("p");
    meta.textContent = `${topology.requestId || "-"} · ${topology.traceId || "-"} · ${formatWorkflowRunTime(topology.createdAt)}`;
    const pre = document.createElement("pre");
    pre.textContent = topology.textTopology || "No text topology captured.";
    const actions = document.createElement("div");
    actions.className = "workflow-run-step-service-links";
    actions.appendChild(renderWorkflowRunLink("过滤此 step", traceTopologyHref(runId, topology.stepId || topology.caseId || "")));
    actions.appendChild(renderWorkflowRunLink("只看 exits", traceTopologyHref(runId, topology.stepId || topology.caseId || "", "external")));
    card.appendChild(head);
    card.appendChild(meta);
    card.appendChild(actions);
    card.appendChild(pre);
    target.appendChild(card);
  });
}

function renderWorkflowRunCandidates(runsPayload) {
  const target = workflowRunEl("workflowRunIdentifiers");
  target.innerHTML = "";
  target.className = "workflow-run-identifiers workflow-run-candidate-list";
  const runs = (runsPayload.workflowRuns || []).slice(0, 8);
  workflowRunEl("workflowRunIdentifiersMeta").textContent = `${runs.length} recent runs`;
  if (!runs.length) {
    const empty = document.createElement("div");
    empty.className = "empty-note";
    empty.textContent = "暂无最近 Workflow run。";
    target.appendChild(empty);
    return;
  }
  runs.forEach((run) => {
    const item = document.createElement("article");
    const link = document.createElement("a");
    link.href = `/workflow-run.html?id=${encodeURIComponent(run.id)}`;
    const title = document.createElement("strong");
    title.textContent = `#${run.id} · ${run.status || "-"}`;
    const meta = document.createElement("span");
    meta.textContent = `${run.workflowId || "-"} · ${run.stepCount || 0} steps`;
    link.appendChild(title);
    link.appendChild(meta);
    const topologyLink = document.createElement("a");
    topologyLink.className = "workflow-run-candidate-topology-link";
    topologyLink.href = traceTopologyHref(run.id);
    topologyLink.textContent = "Topology";
    item.appendChild(link);
    item.appendChild(topologyLink);
    target.appendChild(item);
  });
}

function renderWorkflowRunIdentifiers(summary) {
  const target = workflowRunEl("workflowRunIdentifiers");
  target.innerHTML = "";
  target.className = "workflow-run-identifiers";
  const identifiers = summary.identifiers || {};
  const entries = Object.entries(identifiers);
  workflowRunEl("workflowRunIdentifiersMeta").textContent = `${entries.length} ids`;
  if (!entries.length) {
    const empty = document.createElement("div");
    empty.className = "empty-note";
    empty.textContent = "暂无业务标识。";
    target.appendChild(empty);
    return;
  }
  entries.forEach(([key, value]) => target.appendChild(workflowRunKV(key, String(value))));
}

function renderWorkflowRunMissing(error, runsPayload, requestedId) {
  workflowRunEl("workflowRunTitle").textContent = requestedId ? `未找到 run · id=${requestedId}` : "缺少 run id";
  const summary = workflowRunEl("workflowRunSummary");
  summary.innerHTML = "";
  [
    ["requested", requestedId || "-"],
    ["status", "missing"],
    ["reason", error.message || String(error)],
    ["recent", String((runsPayload.workflowRuns || []).length)],
  ].forEach(([label, value]) => summary.appendChild(workflowRunKV(label, value)));

  workflowRunEl("workflowRunStepsMeta").textContent = "恢复入口";
  const steps = workflowRunEl("workflowRunSteps");
  steps.innerHTML = "";
  steps.classList.add("workflow-run-missing-grid");
  const panel = document.createElement("section");
  panel.className = "workflow-run-step-card";
  const head = document.createElement("div");
  const title = document.createElement("strong");
  title.textContent = "请求信息";
  const status = document.createElement("span");
  status.className = "status-pill failed";
  status.textContent = "missing";
  head.appendChild(title);
  head.appendChild(status);
  const body = document.createElement("p");
  body.textContent = `当前 URL 没有匹配到已持久化的 Workflow run：${requestedId || "id 参数为空"}`;
  const actions = document.createElement("div");
  actions.className = "workflow-run-step-service-links";
  actions.appendChild(renderWorkflowRunLink("返回控制台", "/"));
  actions.appendChild(renderWorkflowRunLink("Workflow 目录", "/workflows.html"));
  actions.appendChild(renderWorkflowRunLink("最近拓扑", latestWorkflowRunTopologyHref(runsPayload)));
  panel.appendChild(head);
  panel.appendChild(body);
  panel.appendChild(actions);
  steps.appendChild(panel);
  renderWorkflowRunCandidates(runsPayload);
  workflowRunEl("workflowRunTraceTopologiesMeta").textContent = "0 persisted records";
  workflowRunEl("workflowRunTraceTopologies").innerHTML = "";
}

function renderWorkflowRun(payload, catalog) {
  renderWorkflowRunSummary(payload.run || {}, payload.summary || {});
  renderWorkflowRunObservationBoard(payload.run || {}, payload.summary || {}, catalog || {});
  renderWorkflowRunSteps(payload.run || {}, payload.summary || {}, catalog || {});
  renderWorkflowRunTraceTopologies(payload || {});
  renderWorkflowRunIdentifiers(payload.summary || {});
}

async function refreshWorkflowRun() {
  const id = new URLSearchParams(window.location.search).get("id") || "";
  if (!id) {
    throw new Error("workflow run id is required");
  }
  setWorkflowRunStatus("refreshing...");
  const [payload, catalog] = await Promise.all([
    workflowRunRequest(`/api/workflow-runs/${encodeURIComponent(id)}`),
    workflowRunRequest("/api/catalog"),
  ]);
  renderWorkflowRun(payload, catalog);
  setWorkflowRunStatus("ready");
}

refreshWorkflowRun().catch(async (error) => {
  const id = new URLSearchParams(window.location.search).get("id") || "";
  let runsPayload = {};
  try {
    runsPayload = await workflowRunRequest("/api/runs");
  } catch (_) {
    runsPayload = {};
  }
  renderWorkflowRunMissing(error, runsPayload, id);
  setWorkflowRunStatus("failed");
});
