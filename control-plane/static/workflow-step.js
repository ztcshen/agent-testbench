const stepEl = (id) => document.getElementById(id);
let workflowStepBoundRunId = "";

function setStepMessage(value) {
  stepEl("workflowStepMessage").textContent = value;
}

function setStepLoadingProgress(percent, title, active = true) {
  const panel = stepEl("workflowStepLoadProgress");
  const track = panel.querySelector('[role="progressbar"]');
  const clamped = Math.min(100, Math.max(0, percent));
  panel.hidden = !active;
  stepEl("workflowStepLoadProgressTitle").textContent = title;
  stepEl("workflowStepLoadProgressValue").textContent = `${clamped}%`;
  stepEl("workflowStepLoadProgressFill").style.width = `${clamped}%`;
  track.setAttribute("aria-valuenow", String(clamped));
}

async function stepRequest(path) {
  const response = await fetch(path);
  const body = await response.json();
  if (!response.ok) {
    throw new Error(body.error || response.statusText);
  }
  return body;
}

function selectedWorkflowId() {
  const params = new URLSearchParams(window.location.search);
  return params.get("workflow") || params.get("id") || "";
}

function selectedStepId() {
  const params = new URLSearchParams(window.location.search);
  return params.get("step") || "";
}

function selectedRunId() {
  if (workflowStepBoundRunId) return workflowStepBoundRunId;
  return selectedRunIdFromUrl();
}

function selectedRunIdFromUrl() {
  const params = new URLSearchParams(window.location.search);
  return params.get("runId") || "";
}

function workflowStepHref(workflowId, stepId) {
  const params = new URLSearchParams();
  params.set("workflow", workflowId || "");
  params.set("step", stepId || "");
  const runId = selectedRunId();
  if (runId) {
    params.set("runId", runId);
  }
  return `/workflow-step.html?${params.toString()}`;
}

function resolvedWorkflowStep(catalog) {
  const workflows = catalog.workflows || [];
  const requestedWorkflowId = selectedWorkflowId();
  const requestedStepId = selectedStepId();
  const workflow = workflows.find((item) => item.id === requestedWorkflowId) || (!requestedWorkflowId ? workflows[0] : null);
  const steps = workflow?.steps || [];
  const step = steps.find((item) => item.id === requestedStepId) || (!requestedStepId ? steps[0] : null);
  return { workflow, step, requestedWorkflowId, requestedStepId };
}

function replaceWorkflowStepLocation(workflow, step, runId = "") {
  if (!workflow?.id || !step?.id) return;
  const current = new URLSearchParams(window.location.search);
  const currentWorkflowId = current.get("workflow") || current.get("id") || "";
  const currentStepId = current.get("step") || "";
  const currentRunId = current.get("runId") || "";
  const nextRunId = runId || currentRunId;
  if (
    currentWorkflowId === workflow.id &&
    currentStepId === step.id &&
    currentRunId === nextRunId &&
    !current.has("id")
  ) {
    return;
  }

  const next = new URLSearchParams();
  next.set("workflow", workflow.id);
  next.set("step", step.id);
  if (nextRunId) {
    next.set("runId", nextRunId);
  }
  window.history.replaceState(null, "", `/workflow-step.html?${next.toString()}`);
}

function serviceById(catalog) {
  return new Map((catalog.services || []).map((service) => [service.id, service]));
}

function dashboardStatusById(snapshot) {
  const byId = new Map();
  (snapshot?.groups || []).forEach((group) => {
    (group.items || []).forEach((item) => byId.set(item.id, item));
  });
  return byId;
}

function serviceRuntimeStatusText(runtime) {
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

function serviceLabel(serviceId, services) {
  const service = services.get(serviceId);
  if (!service) {
    return serviceId ? `${serviceId} · 未建模` : "-";
  }
  return [service.displayName || service.id, service.role || "", service.port ? `:${service.port}` : ""]
    .filter(Boolean)
    .join(" · ");
}

function renderChipList(target, values, emptyText) {
  target.innerHTML = "";
  if (!values?.length) {
    const empty = document.createElement("span");
    empty.textContent = emptyText;
    target.appendChild(empty);
    return;
  }
  values.forEach((value) => {
    const chip = document.createElement("span");
    chip.textContent = value;
    target.appendChild(chip);
  });
}

function summarizedStepValues(values) {
  const counts = new Map();
  (values || []).filter(Boolean).forEach((value) => {
    counts.set(value, (counts.get(value) || 0) + 1);
  });
  return [...counts.entries()].map(([value, count]) => (count > 1 ? `${value} x${count}` : value));
}

function workflowStepContextCard(titleText, values, emptyText) {
  const card = document.createElement("article");
  card.className = "workflow-step-context-card";
  const title = document.createElement("strong");
  title.textContent = titleText;
  card.appendChild(title);
  const chips = document.createElement("div");
  chips.className = "workflow-detail-chips";
  renderChipList(chips, values, emptyText);
  card.appendChild(chips);
  return card;
}

function workflowStepServiceDetailHref(serviceId) {
  return `/environment-node.html?id=${encodeURIComponent(serviceId || "")}`;
}

function workflowStepDefinitionRow(label, value) {
  const dt = document.createElement("dt");
  dt.textContent = label;
  const dd = document.createElement("dd");
  dd.textContent = value || "-";
  return [dt, dd];
}

function renderWorkflowStepServiceEvidence(step, services, runtimeById = new Map()) {
  const panel = stepEl("workflowStepServiceEvidence");
  panel.innerHTML = "";
  panel.hidden = !step;
  if (!step) return;

  const service = services.get(step.serviceId);
  const runtime = runtimeById.get(step.serviceId);
  const head = document.createElement("div");
  head.className = "section-head";
  const titleWrap = document.createElement("div");
  const title = document.createElement("h2");
  title.textContent = "服务证据";
  const summary = document.createElement("p");
  summary.textContent = service
    ? `${service.displayName || service.id} · ${service.role || "unknown"} · ${serviceRuntimeStatusText(runtime)}${service.port ? ` · :${service.port}` : ""}`
    : `${step.serviceId || "-"} · 未建模`;
  titleWrap.appendChild(title);
  titleWrap.appendChild(summary);
  head.appendChild(titleWrap);

  const actions = document.createElement("div");
  actions.className = "workflow-step-service-actions";
  if (step.serviceId) {
    const environmentLink = document.createElement("a");
    environmentLink.className = "button-link";
    environmentLink.href = workflowStepServiceDetailHref(step.serviceId);
    environmentLink.textContent = "环境节点详情";
    actions.appendChild(environmentLink);
  }
  const inventoryLink = document.createElement("a");
  inventoryLink.className = "button-link";
  inventoryLink.href = "/service-inventory.html";
  inventoryLink.textContent = "服务清单";
  actions.appendChild(inventoryLink);
  head.appendChild(actions);

  const list = document.createElement("dl");
  list.className = "workflow-step-service-meta";
  [
    ["service id", step.serviceId || "-"],
    ["runtime", serviceRuntimeStatusText(runtime)],
    ["container", runtime?.container || "-"],
    ["health", runtime?.health || "-"],
    ["role", service?.role || "-"],
    ["port", service?.port ? `:${service.port}` : "-"],
    ["repo env", service?.repoEnv || "-"],
    ["mockable", service ? (service.mockable ? "yes" : "no") : "-"],
    ["dependencies", service?.dependencies?.length ? service.dependencies.join(", ") : "-"],
  ].forEach(([label, value]) => {
    const [dt, dd] = workflowStepDefinitionRow(label, value);
    list.appendChild(dt);
    list.appendChild(dd);
  });

  panel.appendChild(head);
  panel.appendChild(list);
}

function renderWorkflowStepContext(workflow, step, services) {
  const panel = stepEl("workflowStepContext");
  panel.innerHTML = "";
  panel.hidden = !workflow || !step;
  if (!workflow || !step) return;

  const steps = workflow.steps || [];
  const index = steps.findIndex((item) => item.id === step.id);
  const evidence = summarizedStepValues(steps.flatMap((item) => item.evidenceKinds || []));
  const actions = summarizedStepValues(steps.map((item) => item.action));
  const mocks = summarizedStepValues(steps.flatMap((item) => item.relatedMockTargets || []));

  const head = document.createElement("div");
  head.className = "section-head";
  const titleWrap = document.createElement("div");
  const title = document.createElement("h2");
  title.textContent = "上下文摘要";
  const summary = document.createElement("p");
  summary.textContent = `${index + 1} / ${steps.length || 0} · ${workflow.displayName || workflow.id || "-"}`;
  titleWrap.appendChild(title);
  titleWrap.appendChild(summary);
  head.appendChild(titleWrap);

  const grid = document.createElement("div");
  grid.className = "workflow-step-context-grid";
  grid.appendChild(workflowStepContextCard("当前服务", [serviceLabel(step.serviceId, services)], "未声明服务"));
  grid.appendChild(workflowStepContextCard("Workflow action", actions, "无 action"));
  grid.appendChild(workflowStepContextCard("Workflow evidence", evidence, "无 Evidence"));
  grid.appendChild(workflowStepContextCard("Workflow mock", mocks, "无 Mock"));

  panel.appendChild(head);
  panel.appendChild(grid);
}

function workflowStepObservationValues(panel, workflow, step, services) {
  switch (panel.type) {
    case "workflowGraph":
      return [`${workflow?.graph?.nodes?.length || 0} nodes`, `${workflow?.graph?.edges?.length || 0} edges`];
    case "stepSequence":
      return [`${workflow?.steps?.length || 0} steps`];
    case "serviceEvidence":
      return [serviceLabel(step?.serviceId, services)];
    case "evidenceKinds":
      return step?.evidenceKinds || [];
    case "mockTargets":
      return step?.relatedMockTargets || [];
    case "databaseHints":
      return (step?.databaseHints || []).map((hint) => hint.table || hint.entity).filter(Boolean);
    case "caseRunner":
      return [step?.caseId || "-"];
    case "runHistory":
      return [workflow?.entrypoint || "-"];
    case "configEvidence":
      return [step?.action || "-"];
    default:
      return [panel.type || "unknown"];
  }
}

function renderWorkflowStepObservationBoard(workflow, step, services) {
  const panel = stepEl("workflowStepObservationBoard");
  panel.innerHTML = "";
  panel.hidden = !workflow || !step;
  if (!workflow || !step) return;

  const panels = (workflow.observability?.panels || []).filter((item) => {
    const scope = item.scope || "workflow";
    return scope === "workflow" || scope === "step" || scope === step.id || scope === step.action;
  });

  const head = document.createElement("div");
  head.className = "section-head";
  const titleWrap = document.createElement("div");
  const title = document.createElement("h2");
  title.textContent = "配置化观测";
  const summary = document.createElement("p");
  summary.textContent = `${panels.length || 0} panels · ${workflow.id || "-"}`;
  titleWrap.appendChild(title);
  titleWrap.appendChild(summary);
  head.appendChild(titleWrap);

  const grid = document.createElement("div");
  grid.className = "workflow-step-observation-grid";
  panels.forEach((item) => {
    const card = document.createElement("article");
    card.className = "workflow-step-observation-card";
    const titleLine = document.createElement("strong");
    titleLine.textContent = item.title || item.id || item.type || "-";
    const meta = document.createElement("code");
    meta.textContent = [item.type || "-", item.scope || "workflow"].join(" · ");
    const chips = document.createElement("div");
    chips.className = "workflow-detail-chips";
    renderChipList(chips, workflowStepObservationValues(item, workflow, step, services), "无配置值");
    card.appendChild(titleLine);
    card.appendChild(meta);
    card.appendChild(chips);
    grid.appendChild(card);
  });

  panel.appendChild(head);
  panel.appendChild(grid);
}

function renderStepSelector(workflow, selectedId) {
  const selector = stepEl("workflowStepSelector");
  selector.innerHTML = "";
  (workflow.steps || []).forEach((step, index) => {
    const option = document.createElement("option");
    option.value = step.id || "";
    option.textContent = `${String(index + 1).padStart(2, "0")} ${step.displayName || step.id || "-"}`;
    option.selected = step.id === selectedId;
    selector.appendChild(option);
  });
  selector.disabled = !(workflow.steps || []).length;
}

function renderStepSequence(workflow, selectedId) {
  const target = stepEl("workflowStepSequence");
  const steps = workflow.steps || [];
  target.innerHTML = "";
  stepEl("workflowStepSequenceTitle").textContent = "全步骤导航";
  stepEl("workflowStepPosition").textContent = `${steps.findIndex((item) => item.id === selectedId) + 1} / ${steps.length}`;
  steps.forEach((step, index) => {
    const link = document.createElement("a");
    link.href = workflowStepHref(workflow.id, step.id);
    link.className = "workflow-step-sequence-item";
    if (step.id === selectedId) {
      link.classList.add("selected");
    }
    link.innerHTML = `
      <span>${String(index + 1).padStart(2, "0")}</span>
      <strong>${step.displayName || step.id}</strong>
      <code>${step.serviceId || "-"}</code>
    `;
    target.appendChild(link);
  });
}

function renderWorkflowRecovery(workflows) {
  const target = stepEl("workflowStepSequence");
  target.innerHTML = "";
  stepEl("workflowStepSequenceTitle").textContent = "可用 Workflow";
  stepEl("workflowStepPosition").textContent = `${workflows.length} 个`;
  workflows.forEach((workflow, index) => {
    const firstStep = (workflow.steps || [])[0];
    const link = document.createElement("a");
    link.className = "workflow-step-sequence-item";
    link.href = firstStep ? workflowStepHref(workflow.id, firstStep.id) : `/workflow-detail.html?id=${encodeURIComponent(workflow.id || "")}`;
    link.innerHTML = `
      <span>${String(index + 1).padStart(2, "0")}</span>
      <strong>${workflow.displayName || workflow.id}</strong>
      <code>${workflow.steps?.length || 0} steps</code>
    `;
    target.appendChild(link);
  });
}

function runStatusTone(value) {
  const status = String(value || "").toLowerCase();
  if (["pass", "passed", "success", "ok", "complete"].includes(status)) return "passed";
  if (["fail", "failed", "error", "partial"].includes(status)) return "failed";
  if (["running", "started"].includes(status)) return "running";
  return "warning";
}

function parseStepTopology(row) {
  if (!row?.topologyJson) return {};
  if (typeof row.topologyJson === "object") return row.topologyJson;
  try {
    return JSON.parse(row.topologyJson);
  } catch (_) {
    return {};
  }
}

function findRunStep(runPayload, step) {
  const steps = runPayload?.summary?.steps || [];
  return steps.find((item) => item.stepId === step?.id || item.caseId === step?.caseId) || null;
}

function findStepTopology(runPayload, step) {
  const rows = runPayload?.traceTopologies || [];
  return rows.find((item) => item.stepId === step?.id || item.caseId === step?.caseId) || null;
}

function stepRuntimeSummary(stepRun) {
  const summary = stepRun?.summary || {};
  const bodyHealth = stepRun?.bodyHealth || {};
  return [
    ["status", stepRun ? (stepRun.stepOk === false || stepRun.ok === false ? "failed" : "passed") : "-"],
    ["http", summary.httpCode || "-"],
    ["request", summary.requestId || "-"],
    ["elapsed", stepRun ? `${stepRun.elapsedMs || summary.elapsedMs || "-"} ms` : "-"],
    ["body", bodyHealth.message || bodyHealth.level || "-"],
  ];
}

function renderStepKV(label, value, tone = "") {
  if (window.InterfaceRunTemplate) {
    return window.InterfaceRunTemplate.renderKV(label, value, tone);
  }
  const card = document.createElement("article");
  if (tone) card.className = tone;
  const key = document.createElement("span");
  key.textContent = label;
  const text = document.createElement("strong");
  text.textContent = value || "-";
  card.appendChild(key);
  card.appendChild(text);
  return card;
}

function renderStepJSONBlock(titleText, value) {
  if (window.InterfaceRunTemplate) {
    return window.InterfaceRunTemplate.renderJSONBlock(titleText, value);
  }
  const block = document.createElement("article");
  const title = document.createElement("strong");
  title.textContent = titleText;
  const pre = document.createElement("pre");
  if (value === undefined || value === null || value === "") {
    pre.textContent = "-";
  } else if (typeof value === "string") {
    pre.textContent = value;
  } else {
    pre.textContent = JSON.stringify(value, null, 2);
  }
  block.appendChild(title);
  block.appendChild(pre);
  return block;
}

function topologyEdges(parsed) {
  return [
    ...(parsed.confirmedEdges || []).map((edge) => ({ ...edge, kind: "confirmed" })),
    ...(parsed.externalExits || []).map((edge) => ({ ...edge, kind: "external" })),
    ...(parsed.unresolvedExits || []).map((edge) => ({ ...edge, kind: "unresolved" })),
  ];
}

function mermaidSafeLabel(value, fallback = "") {
  return String(value || fallback)
    .replace(/\\/g, "\\\\")
    .replace(/"/g, '\\"')
    .replace(/\n/g, " ")
    .replace(/\|/g, "\\|")
    .slice(0, 80);
}

function buildMermaidTopologySource(nodes, edges) {
  if (!nodes.length) {
    return "";
  }
  const nodeIndex = new Map();
  nodes.forEach((node, index) => nodeIndex.set(node, `S${index + 1}`));
  const sourceLines = ["flowchart LR"];
  const linkStyles = [];
  let linkIndex = 0;
  nodes.forEach((node) => {
    sourceLines.push(`  ${nodeIndex.get(node)}["${mermaidSafeLabel(node)}"]`);
  });
  edges.forEach((edge) => {
    if (!edge.source || !edge.target) {
      return;
    }
    const sourceId = nodeIndex.get(edge.source);
    const targetId = nodeIndex.get(edge.target);
    if (!sourceId || !targetId) return;
    const rawLabel = edge.component || edge.sourceComponent || edge.endpoint || edge.kind || "call";
    const label = mermaidSafeLabel(rawLabel || edge.kind, edge.kind);
    sourceLines.push(`  ${sourceId} -->|"${label}"| ${targetId}`);
    if (edge.kind === "unresolved") {
      linkStyles.push(`  linkStyle ${linkIndex} stroke:#c0342b,stroke-width:2.8px,stroke-dasharray:4 4,color:#a43a31`);
    } else if (edge.kind === "external") {
      linkStyles.push(`  linkStyle ${linkIndex} stroke:#b87910,stroke-width:2.4px,stroke-dasharray:7 5,color:#9a650d`);
    }
    linkIndex += 1;
  });
  return [...sourceLines, ...linkStyles].join("\n");
}

function ensureMermaidReady() {
  if (typeof window === "undefined" || typeof window.mermaid === "undefined" || typeof window.mermaid.init !== "function") {
    return false;
  }
  try {
    window.mermaid.initialize({
      startOnLoad: false,
      securityLevel: "loose",
      theme: "default",
      flowchart: { htmlLabels: false, useMaxWidth: true },
    });
    return true;
  } catch (_error) {
    return false;
  }
}

function renderMermaidTopology(nodes, edges) {
  const source = buildMermaidTopologySource(nodes, edges);
  if (!source || !ensureMermaidReady()) {
    return null;
  }
  const block = document.createElement("div");
  block.className = "workflow-step-topology-mermaid";
  const pre = document.createElement("div");
  pre.className = "mermaid";
  pre.textContent = source;
  block.appendChild(pre);
  try {
    window.mermaid.init(undefined, pre);
    return block;
  } catch (_error) {
    return null;
  }
}

function renderStepTopology(row, step, runPayload) {
  const target = stepEl("workflowStepTopologyGraph");
  target.innerHTML = "";
  const topologyLink = stepEl("workflowStepTopologyLink");
  topologyLink.classList.add("disabled-link");
  topologyLink.removeAttribute("href");
  if (!row) {
    stepEl("workflowStepTopologyMeta").textContent = "这一次接口请求暂无 SkyWalking 拓扑记录。";
    const empty = document.createElement("div");
    empty.className = "empty-note";
    empty.textContent = "运行父 Workflow 后，接口级子模板会按 stepId / caseId 绑定 SkyWalking 结果。";
    target.appendChild(empty);
    return;
  }

  const parsed = parseStepTopology(row);
  const renderer = window.SandboxTopologyRenderer;
  const edges = renderer.edges(parsed);
  const nodes = renderer.nodes(parsed, edges);
  stepEl("workflowStepTopologyMeta").textContent = `${row.status || parsed.status || "-"} · ${row.requestId || "-"} · ${row.traceId || parsed.traceId || "-"}`;
  if (runPayload?.run?.id) {
    topologyLink.href = `/trace-topology.html?workflowRunId=${encodeURIComponent(runPayload.run.id)}&traceFilter=${encodeURIComponent(step?.id || row.stepId || row.caseId || "")}`;
    topologyLink.classList.remove("disabled-link");
  }

  const mermaidDiagram = renderMermaidTopology(nodes, edges);
  if (mermaidDiagram) {
    target.appendChild(mermaidDiagram);
  }
  const svgDiagram = renderer.renderDiagram(nodes, edges, { markerPrefix: "workflow-step-arrow" });
  svgDiagram.classList.add("workflow-step-topology-synthetic");
  if (mermaidDiagram) {
    svgDiagram.style.display = "none";
  }
  target.appendChild(svgDiagram);

  const graph = document.createElement("div");
  graph.className = "workflow-step-topology-nodes";
  if (!nodes.length) {
    const empty = document.createElement("div");
    empty.className = "empty-note";
    empty.textContent = "SkyWalking 返回了记录，但没有可绘制节点。";
    graph.appendChild(empty);
  } else {
    nodes.forEach((node, index) => {
      const item = document.createElement("article");
      item.className = "workflow-step-topology-node";
      item.innerHTML = `<span>${String(index + 1).padStart(2, "0")}</span><strong>${node}</strong>`;
      graph.appendChild(item);
    });
  }
  target.appendChild(graph);

  target.appendChild(renderer.renderEdgeList(edges, { emptyText: "SkyWalking 没有确认调用边；失败接口会保留 partial / unresolved 证据。" }));

  if (row.textTopology) {
    target.appendChild(renderStepJSONBlock("text topology", row.textTopology));
  }
}

function renderStepRunEvidence(workflow, step, runPayload) {
  const runId = selectedRunId();
  stepEl("workflowStepRunId").textContent = runId ? `#${runId}` : "未绑定 run";
  const runLink = stepEl("workflowStepRunLink");
  runLink.classList.add("disabled-link");
  runLink.removeAttribute("href");
  const summaryTarget = stepEl("workflowStepRunSummary");
  const requestTarget = stepEl("workflowStepRequestResponse");
  summaryTarget.innerHTML = "";
  requestTarget.innerHTML = "";

  if (!runId) {
    stepEl("workflowStepRunEvidenceMeta").textContent = "当前是定义视图；从父 Workflow 运行结果进入可查看单次接口证据。";
    summaryTarget.appendChild(renderStepKV("mode", "definition"));
    renderStepTopology(null, step, null);
    return;
  }
  if (!runPayload?.run) {
    stepEl("workflowStepRunEvidenceMeta").textContent = `未找到 Workflow run #${runId}`;
    summaryTarget.appendChild(renderStepKV("run", "missing", "failed"));
    renderStepTopology(null, step, null);
    return;
  }

  runLink.href = `/workflow-run.html?id=${encodeURIComponent(runPayload.run.id)}`;
  runLink.classList.remove("disabled-link");
  const stepRun = findRunStep(runPayload, step);
  const topology = findStepTopology(runPayload, step);
  stepEl("workflowStepRunEvidenceMeta").textContent = `${runPayload.run.workflowId || workflow?.id || "-"} · run #${runPayload.run.id}`;
  stepRuntimeSummary(stepRun).forEach(([label, value], index) => {
    summaryTarget.appendChild(renderStepKV(label, value, index === 0 ? runStatusTone(value) : ""));
  });

  if (!stepRun) {
    const empty = document.createElement("div");
    empty.className = "empty-note";
    empty.textContent = "这一次 run 没有当前 step 的执行明细。";
    requestTarget.appendChild(empty);
  } else {
    const result = stepRun.result || {};
    if (window.InterfaceRunTemplate) {
      requestTarget.appendChild(window.InterfaceRunTemplate.renderRequestResponse({
        request: result.request || stepRun.request || {},
        response: result.response || stepRun.response || {},
      }));
    } else {
      requestTarget.appendChild(renderStepJSONBlock("request", result.request || stepRun.request || {}));
      requestTarget.appendChild(renderStepJSONBlock("response", result.response || stepRun.response || {}));
    }
  }
  renderStepTopology(topology, step, runPayload);
}

function setNeighborLink(id, workflow, step, label) {
  const link = stepEl(id);
  if (!step) {
    link.removeAttribute("href");
    link.classList.add("disabled-link");
    link.textContent = label;
    return;
  }
  link.href = workflowStepHref(workflow.id, step.id);
  link.classList.remove("disabled-link");
  link.textContent = `${label}: ${step.displayName || step.id}`;
}

function renderWorkflowStep(catalog, runPayload = null, dashboard = null) {
  const workflows = catalog.workflows || [];
  const workflowId = selectedWorkflowId();
  let workflow = workflows.find((item) => item.id === workflowId);
  if (workflowId && !workflow) {
    const selector = stepEl("workflowStepSelector");
    selector.innerHTML = "";
    selector.disabled = true;
    const option = document.createElement("option");
    option.textContent = "Workflow 未找到";
    selector.appendChild(option);
    renderWorkflowRecovery(workflows);
    stepEl("workflowStepTitle").textContent = "Workflow 未找到";
    stepEl("workflowStepSummary").textContent = `Catalog 中没有 workflow=${workflowId}`;
    stepEl("workflowStepId").textContent = selectedStepId() || "-";
    stepEl("workflowStepWorkflowId").textContent = workflowId;
    stepEl("workflowStepService").textContent = "-";
    stepEl("workflowStepName").textContent = "Workflow 未找到";
    stepEl("workflowStepDescription").textContent = "请回到控制台或环境大盘选择已声明的 Workflow。";
    stepEl("workflowStepCase").textContent = "-";
    stepEl("workflowStepAction").textContent = "-";
    stepEl("workflowDefinitionLink").href = "/workflows.html";
    renderChipList(stepEl("workflowStepEvidence"), [], "无 Evidence");
    renderChipList(stepEl("workflowStepMocks"), [], "无 Mock");
    renderWorkflowStepServiceEvidence(null, serviceById(catalog), dashboardStatusById(dashboard));
    renderWorkflowStepObservationBoard(null, null, serviceById(catalog));
    renderWorkflowStepContext(null, null, serviceById(catalog));
    renderStepRunEvidence(null, null, runPayload);
    ["previousStepLink", "nextStepLink"].forEach((id) => {
      const link = stepEl(id);
      link.removeAttribute("href");
      link.classList.add("disabled-link");
      link.textContent = id === "previousStepLink" ? "上一步" : "下一步";
    });
    setStepMessage("workflow not found");
    return;
  }
  workflow = workflow || workflows[0];
  if (!workflow) {
    stepEl("workflowStepTitle").textContent = "暂无 Workflow Step";
    stepEl("workflowStepSummary").textContent = "Catalog 暂未返回 Workflow。";
    setStepMessage("empty catalog");
    return;
  }

  const requestedStep = selectedStepId();
  const steps = workflow.steps || [];
  let step = steps.find((item) => item.id === requestedStep);
  if (requestedStep && !step) {
    renderStepSelector(workflow, "");
    renderStepSequence(workflow, "");
    stepEl("workflowStepTitle").textContent = "Workflow Step 未找到";
    stepEl("workflowStepSummary").textContent = `${workflow.displayName || workflow.id || "-"} 中没有 step=${requestedStep}`;
    stepEl("workflowStepId").textContent = requestedStep;
    stepEl("workflowStepWorkflowId").textContent = workflow.id || "-";
    stepEl("workflowStepService").textContent = "-";
    stepEl("workflowStepName").textContent = "Step 未找到";
    stepEl("workflowStepDescription").textContent = "请从左侧切换到已声明的步骤，或回到 Workflow 定义页核对 catalog。";
    stepEl("workflowStepCase").textContent = "-";
    stepEl("workflowStepAction").textContent = "-";
    stepEl("workflowDefinitionLink").href = `/workflow-detail.html?id=${encodeURIComponent(workflow.id || "")}`;
    renderChipList(stepEl("workflowStepEvidence"), [], "无 Evidence");
    renderChipList(stepEl("workflowStepMocks"), [], "无 Mock");
    renderWorkflowStepServiceEvidence(null, serviceById(catalog), dashboardStatusById(dashboard));
    renderWorkflowStepObservationBoard(workflow, null, serviceById(catalog));
    renderWorkflowStepContext(null, null, serviceById(catalog));
    renderStepRunEvidence(workflow, null, runPayload);
    setNeighborLink("previousStepLink", workflow, null, "上一步");
    setNeighborLink("nextStepLink", workflow, steps[0], "返回第一步");
    setStepMessage("step not found");
    return;
  }
  step = step || steps[0];
  if (!step) {
    stepEl("workflowStepTitle").textContent = workflow.displayName || workflow.id;
    stepEl("workflowStepSummary").textContent = "该 Workflow 暂无步骤。";
    setStepMessage("empty workflow");
    return;
  }

  const services = serviceById(catalog);
  const runtimeById = dashboardStatusById(dashboard);
  const index = steps.findIndex((item) => item.id === step.id);
  renderStepSelector(workflow, step.id);
  renderStepSequence(workflow, step.id);

  stepEl("workflowStepTitle").textContent = step.displayName || step.id;
  stepEl("workflowStepSummary").textContent = workflow.displayName || workflow.id || "-";
  stepEl("workflowStepId").textContent = step.id || "-";
  stepEl("workflowStepWorkflowId").textContent = workflow.id || "-";
  stepEl("workflowStepService").textContent = serviceLabel(step.serviceId, services);
  stepEl("workflowStepName").textContent = `${String(index + 1).padStart(2, "0")} ${step.displayName || step.id}`;
  stepEl("workflowStepDescription").textContent = workflow.description || "-";
  stepEl("workflowStepCase").textContent = step.caseId || "-";
  stepEl("workflowStepAction").textContent = step.action || "-";
  stepEl("workflowDefinitionLink").href = `/workflow-detail.html?id=${encodeURIComponent(workflow.id || "")}`;

  renderChipList(stepEl("workflowStepEvidence"), step.evidenceKinds || [], "无 Evidence");
  renderChipList(stepEl("workflowStepMocks"), step.relatedMockTargets || [], "无 Mock");
  renderWorkflowStepServiceEvidence(step, services, runtimeById);
  renderWorkflowStepObservationBoard(workflow, step, services);
  renderWorkflowStepContext(workflow, step, services);
  renderStepRunEvidence(workflow, step, runPayload);
  setNeighborLink("previousStepLink", workflow, steps[index - 1], "上一步");
  setNeighborLink("nextStepLink", workflow, steps[index + 1], "下一步");
}

async function refreshWorkflowStep() {
  setStepMessage("refreshing...");
  setStepLoadingProgress(8, "读取 Workflow Catalog");
  workflowStepBoundRunId = "";
  const [catalog, dashboard] = await Promise.all([
    stepRequest("/api/catalog"),
    stepRequest("/api/dashboard"),
  ]);
  setStepLoadingProgress(32, "解析 Workflow 和步骤");
  const { workflow, step, requestedWorkflowId, requestedStepId } = resolvedWorkflowStep(catalog);
  if (workflow?.id && step?.id && (!requestedWorkflowId || !requestedStepId || new URLSearchParams(window.location.search).has("id"))) {
    replaceWorkflowStepLocation(workflow, step);
  }

  const runId = selectedRunIdFromUrl();
  let runPayload = null;
  if (runId) {
    setStepLoadingProgress(58, `读取 Workflow run #${runId}`);
    runPayload = await stepRequest(`/api/workflow-runs/step?runId=${encodeURIComponent(runId)}&stepId=${encodeURIComponent(step.id)}`).catch((error) => ({ ok: false, error: error.message }));
  } else if (workflow?.id && step?.id) {
    setStepLoadingProgress(58, "绑定最近一次运行证据");
    const latestRunPath = `/api/workflow-runs/latest-step?workflowId=${encodeURIComponent(workflow.id)}&stepId=${encodeURIComponent(step.id)}`;
    runPayload = await stepRequest(latestRunPath).catch(() => null);
  }
  workflowStepBoundRunId = runPayload?.run?.id ? String(runPayload.run.id) : "";
  if (workflowStepBoundRunId && workflow?.id && step?.id && selectedRunIdFromUrl() !== workflowStepBoundRunId) {
    replaceWorkflowStepLocation(workflow, step, workflowStepBoundRunId);
  }
  setStepLoadingProgress(84, "渲染接口证据和拓扑");
  renderWorkflowStep(catalog, runPayload, dashboard);
  setStepLoadingProgress(100, workflowStepBoundRunId ? `加载完成 · run #${workflowStepBoundRunId}` : "加载完成");
  setStepMessage("ready");
}

stepEl("workflowStepSelector").addEventListener("change", (event) => {
  window.location.href = workflowStepHref(selectedWorkflowId(), event.target.value);
});

refreshWorkflowStep().catch((error) => {
  setStepLoadingProgress(100, error.message);
  setStepMessage(error.message);
});
