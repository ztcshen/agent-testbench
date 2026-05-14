const traceTopologyEl = (id) => document.getElementById(id);
const traceTopologyState = {
  payload: null,
};

function setTraceTopologyStatus(value) {
  traceTopologyEl("traceTopologyStatus").textContent = value;
}

async function traceTopologyRequest(path) {
  const response = await fetch(path);
  const body = await response.json();
  if (!response.ok) {
    throw new Error(body.error || response.statusText);
  }
  return body;
}

function parseTraceTopologyPayload(row) {
  if (!row?.topologyJson) return {};
  if (typeof row.topologyJson === "object") return row.topologyJson;
  try {
    return JSON.parse(row.topologyJson);
  } catch (_) {
    return {};
  }
}

function traceTopologyItems(rows, key) {
  return rows.flatMap((row) => {
    const parsed = parseTraceTopologyPayload(row);
    return (parsed[key] || []).map((item) => ({
      ...item,
      stepId: row.stepId,
      caseId: row.caseId,
      workflowRunId: row.workflowRunId,
      requestId: row.requestId,
      traceId: row.traceId,
      status: row.status,
    }));
  });
}

function traceTopologyKV(label, value) {
  const card = document.createElement("button");
  card.type = "button";
  card.className = "trace-topology-summary-action";
  const key = document.createElement("span");
  key.textContent = label;
  const text = document.createElement("strong");
  text.textContent = value || "-";
  card.addEventListener("click", () => applyTraceTopologySummaryFacet(label));
  card.appendChild(key);
  card.appendChild(text);
  return card;
}

function applyTraceTopologySummaryFacet(label) {
  if (label === "external" || label === "unresolved") {
    traceTopologyEl("traceTopologyExitFilter").value = label;
  } else {
    traceTopologyEl("traceTopologyExitFilter").value = "";
  }
  if (traceTopologyState.payload) {
    renderTraceTopologyWorkbench(traceTopologyState.payload);
  }
}

function renderTraceTopologySummary(payload) {
  const run = payload.run || {};
  const rows = payload.traceTopologies || [];
  const confirmedEdges = traceTopologyItems(rows, "confirmedEdges");
  const externalExits = traceTopologyItems(rows, "externalExits");
  const unresolvedExits = traceTopologyItems(rows, "unresolvedExits");
  const observedNodes = new Set(rows.flatMap((row) => parseTraceTopologyPayload(row).observedNodes || []));

  traceTopologyEl("traceTopologyTitle").textContent = `${run.workflowId || "-"} · #${run.id || "-"}`;
  traceTopologyEl("traceTopologyRunLink").href = run.id ? `/workflow-run.html?id=${encodeURIComponent(run.id)}` : "/workflow-run.html";

  const target = traceTopologyEl("traceTopologySummary");
  target.innerHTML = "";
  [
    ["status", run.status || "-"],
    ["records", String(rows.length)],
    ["confirmed", String(confirmedEdges.length)],
    ["external", String(externalExits.length)],
    ["unresolved", String(unresolvedExits.length)],
    ["nodes", String(observedNodes.size)],
  ].forEach(([label, value]) => target.appendChild(traceTopologyKV(label, value)));
}

function renderTraceTopologyWorkbench(payload) {
  const rows = payload.traceTopologies || [];
  const visibleRows = filterTraceTopologyRows(rows);
  renderTraceTopologySummary(payload);
  renderTraceTopologyFilters(rows, visibleRows);
  renderTraceTopologyMatrix(visibleRows, rows.length);
  renderTraceTopologyEdges(visibleRows);
  renderTraceTopologyExits(visibleRows);
}

function filterTraceTopologyRows(rows) {
  const query = (traceTopologyEl("traceTopologyFilter").value || "").trim().toLowerCase();
  const status = traceTopologyEl("traceTopologyStatusFilter").value;
  const hasExactStepMatch = query && rows.some((row) => String(row.stepId || row.caseId || "").toLowerCase() === query);
  return rows.filter((row) => {
    const parsed = parseTraceTopologyPayload(row);
    const statusOk = !status || row.status === status;
    if (hasExactStepMatch) {
      return statusOk && String(row.stepId || row.caseId || "").toLowerCase() === query;
    }
    const text = [
      row.stepId,
      row.caseId,
      row.requestId,
      row.traceId,
      row.status,
      ...(parsed.observedNodes || []),
      ...(parsed.confirmedEdges || []).flatMap((edge) => [edge.source, edge.target, edge.sourceComponent, edge.targetComponent]),
      ...(parsed.externalExits || []).flatMap((exit) => [exit.source, exit.target, exit.endpoint]),
      ...(parsed.unresolvedExits || []).flatMap((exit) => [exit.source, exit.target, exit.endpoint]),
    ]
      .filter(Boolean)
      .join(" ")
      .toLowerCase();
    return statusOk && (!query || text.includes(query));
  });
}

function renderTraceTopologyFilters(rows, visibleRows) {
  const complete = rows.filter((row) => row.status === "complete").length;
  const partial = rows.filter((row) => row.status !== "complete").length;
  traceTopologyEl("traceTopologyFilter").title = `${visibleRows.length}/${rows.length} visible · complete ${complete} · partial ${partial}`;
}

function renderTraceTopologyMatrix(rows, totalRows = rows.length) {
  const target = traceTopologyEl("traceTopologyMatrix");
  target.innerHTML = "";
  traceTopologyEl("traceTopologyMatrixMeta").textContent = rows.length ? `${rows.length}/${totalRows} persisted step traces` : `0/${totalRows} persisted step traces`;
  if (!rows.length) {
    target.appendChild(traceTopologyEmpty(totalRows ? "没有匹配的 SkyWalking topology。" : "此 Workflow run 暂无持久化 SkyWalking topology。"));
    return;
  }
  rows.forEach((row) => {
    const parsed = parseTraceTopologyPayload(row);
    const card = document.createElement("article");
    card.className = `trace-topology-step ${row.status === "complete" ? "complete" : "partial"}`;
    const head = document.createElement("div");
    const title = document.createElement("strong");
    title.textContent = row.stepId || row.caseId || "trace";
    const status = document.createElement("span");
    status.className = `status-pill ${row.status === "complete" ? "passed" : "failed"}`;
    status.textContent = row.status || "unknown";
    head.appendChild(title);
    head.appendChild(status);

    const meta = document.createElement("p");
    meta.textContent = `${row.requestId || "-"} · ${row.traceId || "-"} · spans ${parsed.spanCount || 0}`;
    const chips = document.createElement("div");
    chips.className = "trace-topology-chip-row";
    [
      `${(parsed.confirmedEdges || []).length} edges`,
      `${(parsed.externalExits || []).length} external`,
      `${(parsed.unresolvedExits || []).length} unresolved`,
    ].forEach((value) => chips.appendChild(traceTopologyChip(value)));
    (parsed.observedNodes || []).slice(0, 6).forEach((node) => chips.appendChild(traceTopologyChip(node)));

    card.appendChild(head);
    card.appendChild(meta);
    card.appendChild(chips);
    target.appendChild(card);
  });
}

function renderTraceTopologyEdges(rows) {
  const edges = traceTopologyItems(rows, "confirmedEdges");
  const target = traceTopologyEl("traceTopologyEdges");
  target.innerHTML = "";
  traceTopologyEl("traceTopologyEdgesMeta").textContent = edges.length ? `${edges.length} confirmed edges` : "0 confirmed edges";
  if (!edges.length) {
    target.appendChild(traceTopologyEmpty("SkyWalking 没有返回可确认调用边。"));
    return;
  }
  edges.forEach((edge) => target.appendChild(traceTopologyEdgeItem(edge, "confirmed")));
}

function renderTraceTopologyExits(rows) {
  const externalExits = traceTopologyItems(rows, "externalExits").map((item) => ({ ...item, kind: "external" }));
  const unresolvedExits = traceTopologyItems(rows, "unresolvedExits").map((item) => ({ ...item, kind: "unresolved" }));
  const exits = filterTraceTopologyExits([...unresolvedExits, ...externalExits]);
  const target = traceTopologyEl("traceTopologyExits");
  target.innerHTML = "";
  traceTopologyEl("traceTopologyExitsMeta").textContent = `${exits.length} visible · ${externalExits.length} external · ${unresolvedExits.length} unresolved`;
  if (!exits.length) {
    target.appendChild(traceTopologyEmpty("此 run 没有 external 或 unresolved exit。"));
    return;
  }
  exits.forEach((exit) => target.appendChild(traceTopologyEdgeItem(exit, exit.kind)));
}

function filterTraceTopologyExits(exits) {
  const kind = traceTopologyEl("traceTopologyExitFilter").value;
  const query = (traceTopologyEl("traceTopologyFilter").value || "").trim().toLowerCase();
  const hasExactStepMatch = query && exits.some((exit) => String(exit.stepId || exit.caseId || "").toLowerCase() === query);
  return exits.filter((exit) => {
    const kindOk = !kind || exit.kind === kind;
    if (hasExactStepMatch) {
      return kindOk && String(exit.stepId || exit.caseId || "").toLowerCase() === query;
    }
    const text = [exit.stepId, exit.caseId, exit.source, exit.target, exit.sourceComponent, exit.targetComponent, exit.endpoint, exit.requestId, exit.traceId]
      .filter(Boolean)
      .join(" ")
      .toLowerCase();
    return kindOk && (!query || text.includes(query));
  });
}

function seedTraceTopologyFiltersFromUrl() {
  const params = new URLSearchParams(window.location.search);
  const traceFilter = params.get("traceFilter") || "";
  const status = params.get("status") || "";
  const exitKind = params.get("exitKind") || "";
  if (traceFilter) {
    traceTopologyEl("traceTopologyFilter").value = traceFilter;
  }
  if (status) {
    traceTopologyEl("traceTopologyStatusFilter").value = status;
  }
  if (exitKind) {
    traceTopologyEl("traceTopologyExitFilter").value = exitKind;
  }
}

function traceTopologyEdgeItem(item, kind) {
  const article = document.createElement("article");
  article.className = `trace-topology-edge ${kind}`;
  const top = document.createElement("div");
  const title = document.createElement("strong");
  title.textContent = `${item.source || "-"} -> ${item.target || "-"}`;
  const badge = document.createElement("span");
  badge.className = "agent-status";
  badge.textContent = kind;
  top.appendChild(title);
  top.appendChild(badge);
  const meta = document.createElement("p");
  meta.textContent = `${item.stepId || item.caseId || "-"} · ${item.sourceComponent || item.component || "-"} -> ${item.targetComponent || item.endpoint || "-"}`;
  const ids = document.createElement("code");
  ids.textContent = `${item.requestId || "-"} · ${item.traceId || "-"}`;
  article.appendChild(top);
  article.appendChild(meta);
  article.appendChild(ids);
  if (item.workflowRunId && (item.stepId || item.caseId)) {
    const actions = document.createElement("div");
    actions.className = "workflow-run-step-service-links trace-topology-step-link";
    const link = document.createElement("a");
    link.href = `/workflow-run.html?id=${encodeURIComponent(item.workflowRunId)}#${workflowRunStepAnchor(item.stepId || item.caseId)}`;
    link.textContent = "查看 step";
    actions.appendChild(link);
    article.appendChild(actions);
  }
  return article;
}

function workflowRunStepAnchor(stepId) {
  return `workflow-step-${encodeURIComponent(stepId || "unknown")}`;
}

function traceTopologyChip(value) {
  const chip = document.createElement("span");
  chip.className = "trace-topology-chip";
  chip.textContent = value;
  return chip;
}

function traceTopologyEmpty(text) {
  const empty = document.createElement("div");
  empty.className = "empty-note";
  empty.textContent = text;
  return empty;
}

async function refreshTraceTopologyWorkbench() {
  const workflowRunId = new URLSearchParams(window.location.search).get("workflowRunId") || "";
  if (!workflowRunId) {
    throw new Error("workflowRunId is required");
  }
  setTraceTopologyStatus("refreshing...");
  const payload = await traceTopologyRequest(`/api/workflow-runs/${encodeURIComponent(workflowRunId)}`);
  traceTopologyState.payload = payload;
  renderTraceTopologyWorkbench(payload);
  setTraceTopologyStatus("ready");
}

["traceTopologyFilter", "traceTopologyStatusFilter", "traceTopologyExitFilter"].forEach((id) => {
  traceTopologyEl(id).addEventListener(id === "traceTopologyFilter" ? "input" : "change", () => {
    if (traceTopologyState.payload) {
      renderTraceTopologyWorkbench(traceTopologyState.payload);
    }
  });
});

seedTraceTopologyFiltersFromUrl();
refreshTraceTopologyWorkbench().catch((error) => {
  traceTopologyEl("traceTopologyTitle").textContent = "未找到 topology run";
  traceTopologyEl("traceTopologySummary").innerHTML = "";
  traceTopologyEl("traceTopologySummary").appendChild(traceTopologyKV("status", "failed"));
  traceTopologyEl("traceTopologySummary").appendChild(traceTopologyKV("reason", error.message));
  traceTopologyEl("traceTopologyMatrixMeta").textContent = "恢复入口";
  traceTopologyEl("traceTopologyMatrix").innerHTML = "";
  traceTopologyEl("traceTopologyMatrix").appendChild(traceTopologyEmpty("返回控制台选择一个最近 Workflow run。"));
  traceTopologyEl("traceTopologyEdgesMeta").textContent = "0 confirmed edges";
  traceTopologyEl("traceTopologyEdges").innerHTML = "";
  traceTopologyEl("traceTopologyExitsMeta").textContent = "0 external · 0 unresolved";
  traceTopologyEl("traceTopologyExits").innerHTML = "";
  setTraceTopologyStatus("failed");
});
