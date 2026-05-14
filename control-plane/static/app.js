const state = {
  snapshot: null,
  catalog: null,
  runs: null,
  agentTest: null,
  caseRuns: null,
};

const el = (id) => document.getElementById(id);

function setMessage(value) {
  el("message").textContent = value;
}

async function request(path, options = {}) {
  const response = await fetch(path, {
    headers: { "Content-Type": "application/json" },
    ...options,
  });
  const body = await response.json();
  if (!response.ok) {
    throw new Error(body.error || response.statusText);
  }
  return body;
}

function render() {
  const services = state.catalog?.services || [];
  const workflows = state.catalog?.workflows || [];
  const workflowRuns = state.runs?.workflowRuns || [];
  const caseRuns = state.caseRuns?.caseRuns || [];
  el("summary").textContent = `${services.length} services · ${workflows.length} workflows · ${workflowRuns.length} workflow runs · ${caseRuns.length} case runs`;
  renderSandboxCapabilities();
  renderSandboxEvidenceLinks();
  renderSandboxTopology();
  renderServiceHealth();
  renderRunHistory();
}

function renderSandboxCapabilities() {
  const target = el("sandboxCapabilities");
  if (!target) return;
  target.innerHTML = "";
  const workflowRuns = state.runs?.workflowRuns || [];
  const latestRun = workflowRuns[0];
  const latestRunHref = latestRun ? `/workflow-run.html?id=${encodeURIComponent(latestRun.id)}` : "/agent-test.html";
  const latestTopologyHref = latestRun ? `/trace-topology.html?workflowRunId=${encodeURIComponent(latestRun.id)}` : "/agent-test.html";
  const agentTestSummary = state.agentTest?.summary || {};
  const latestAgentFailureKind = agentTestSummary.latestFailureKind || "no active failure";
  const capabilityGapCount = agentTestSummary.failureKinds?.sandbox_capability_gap || 0;
  const latestCaseRun = state.caseRuns?.caseRuns?.[0];
  const cards = [
    {
      title: "Agent Test Kit",
      detail: `Docker-only profile runner, Evidence bundle, SQLite run record, capability gaps ${capabilityGapCount}.`,
      href: "/agent-test.html",
      meta: latestAgentFailureKind,
    },
    {
      title: "Workflow Evidence",
      detail: "Requests, responses, logs, WireMock journal, database hints, and trace topology.",
      href: latestRunHref,
      meta: latestRun ? `latest ${latestRun.status}` : "no run yet",
    },
    {
      title: "Run Topology",
      detail: "SkyWalking confirmed edges, external exits, unresolved exits, request ids, and trace ids.",
      href: latestTopologyHref,
      meta: latestRun ? `run #${latestRun.id}` : "no run yet",
    },
    {
      title: "API Case Evidence",
      detail: "Runtime case bundles, request and response snapshots, trace continuity, and failureKind.",
      href: "/case-runs.html",
      meta: latestCaseRun ? `${latestCaseRun.status || "unknown"} · ${latestCaseRun.failureKind || "no failureKind"}` : "no case run yet",
    },
    {
      title: "Service Inventory",
      detail: "Registry-backed services, runtime nodes, containers, ports, and declared dependencies.",
      href: "/service-inventory.html",
      meta: `${state.catalog?.services?.length || 0} services`,
    },
    {
      title: "Replay And Probe",
      detail: "Replay fixtures, negative probes, capability-gap evidence, and persisted reports.",
      href: "/workflow-detail.html?id=sandbox.replay_probe_observability",
      meta: `${state.runs?.probeRuns?.length || 0} probes`,
    },
  ];
  cards.forEach((card) => {
    const link = document.createElement("a");
    link.className = "sandbox-capability-card";
    link.href = card.href;
    link.innerHTML = `
      <span>${card.meta}</span>
      <strong>${card.title}</strong>
      <p>${card.detail}</p>
    `;
    target.appendChild(link);
  });
}

function renderSandboxEvidenceLinks() {
  const workflowRuns = state.runs?.workflowRuns || [];
  const latestRun = workflowRuns[0];
  const latestFailedRun = latestFailedWorkflowRun(workflowRuns);
  const runLink = el("latestWorkflowRunEvidenceLink");
  const failedRunLink = el("latestFailedWorkflowRunEvidenceLink");
  const topologyLink = el("latestWorkflowTopologyLink");
  const failedTopologyLink = el("latestFailedWorkflowTopologyLink");
  if (runLink) {
    runLink.href = latestRun ? `/workflow-run.html?id=${encodeURIComponent(latestRun.id)}` : "/workflow-run.html";
    runLink.textContent = latestRun ? `Latest Workflow Run #${latestRun.id}` : "Latest Workflow Run";
    runLink.title = latestRun ? `${latestRun.workflowId || "-"} · ${latestRun.status || "-"}` : "No persisted Workflow run yet";
  }
  if (failedRunLink) {
    failedRunLink.href = latestFailedRun ? `/workflow-run.html?id=${encodeURIComponent(latestFailedRun.id)}` : "/workflow-run.html";
    failedRunLink.textContent = latestFailedRun ? `Latest Failed Workflow #${latestFailedRun.id}` : "Latest Failed Workflow";
    failedRunLink.title = latestFailedRun
      ? `${latestFailedRun.workflowId || "-"} · ${latestFailedRun.status || "-"}`
      : "No failed Workflow run yet";
  }
  if (topologyLink) {
    topologyLink.href = latestRun ? `/trace-topology.html?workflowRunId=${encodeURIComponent(latestRun.id)}` : "/trace-topology.html";
    topologyLink.textContent = latestRun ? `Latest Run Topology #${latestRun.id}` : "Latest Run Topology";
    topologyLink.title = latestRun ? `${latestRun.workflowId || "-"} · SkyWalking topology` : "No persisted Workflow topology yet";
  }
  if (failedTopologyLink) {
    failedTopologyLink.href = latestFailedWorkflowTopologyHref(latestFailedRun);
    failedTopologyLink.textContent = latestFailedRun ? `Latest Failed Topology #${latestFailedRun.id}` : "Latest Failed Topology";
    failedTopologyLink.title = latestFailedRun
      ? `${latestFailedRun.workflowId || "-"} · failed SkyWalking topology`
      : "No failed Workflow topology yet";
  }
}

function latestFailedWorkflowRun(workflowRuns) {
  return (workflowRuns || []).find((run) => run.status === "failed");
}

function latestFailedWorkflowTopologyHref(latestFailedRun) {
  return latestFailedRun ? `/trace-topology.html?workflowRunId=${encodeURIComponent(latestFailedRun.id)}&exitKind=unresolved` : "/trace-topology.html";
}

function renderSandboxTopology() {
  const target = el("sandboxTopology");
  if (!target) return;
  target.innerHTML = "";
  const services = state.catalog?.services || [];
  const serviceById = new Map(services.map((service) => [service.id, service]));
  const edges = state.catalog?.topology?.edges || [];
  if (!edges.length) {
    const empty = document.createElement("div");
    empty.className = "run-history-empty";
    empty.textContent = "Catalog 未声明拓扑边";
    target.appendChild(empty);
    return;
  }
  edges.forEach((edge) => {
    const row = document.createElement("a");
    row.className = "sandbox-topology-edge";
    row.href = workflowCatalogServiceHref(serviceById.get(edge.from) || { id: edge.from });
    const from = serviceById.get(edge.from)?.displayName || edge.from;
    const to = serviceById.get(edge.to)?.displayName || edge.to;
    row.innerHTML = `<strong>${from}</strong><span>-></span><strong>${to}</strong>`;
    target.appendChild(row);
  });
}

function renderServiceHealth() {
  const target = el("homeServiceHealth");
  if (!target) return;
  target.innerHTML = "";
  const services = state.snapshot?.services || [];
  if (!services.length) {
    const empty = document.createElement("div");
    empty.className = "run-history-empty";
    empty.textContent = "暂无 service health";
    target.appendChild(empty);
    return;
  }
  services.slice(0, 8).forEach((service) => {
    const link = document.createElement("a");
    link.className = `home-service-health-item ${serviceHealthTone(service)}`;
    link.href = service.exists === false ? `/service-inventory.html#service-${encodeURIComponent(service.id || "")}` : `/environment-node.html?id=${encodeURIComponent(service.id || "")}`;

    const top = document.createElement("div");
    top.className = "run-history-top";
    const title = document.createElement("strong");
    title.textContent = service.name || service.id || "-";
    const badge = document.createElement("code");
    badge.textContent = serviceHealthLabel(service);
    top.appendChild(title);
    top.appendChild(badge);

    const detail = document.createElement("p");
    detail.textContent = [service.currentBranch || service.kind || "-", service.currentCommit || service.targetCommit || "-", service.status || service.error || ""]
      .filter(Boolean)
      .join(" · ");

    link.appendChild(top);
    link.appendChild(detail);
    target.appendChild(link);
  });
}

function serviceHealthTone(service) {
  if (service.error || service.exists === false) return "failed";
  if (service.dirty || (service.desiredBranch && service.currentBranch && service.desiredBranch !== service.currentBranch)) return "warning";
  return "passed";
}

function serviceHealthLabel(service) {
  if (service.error) return "error";
  if (service.exists === false) return "external";
  if (service.dirty) return "dirty";
  if (service.desiredBranch && service.currentBranch && service.desiredBranch !== service.currentBranch) return "branch drift";
  return "clean";
}

function workflowCatalogServiceHref(service) {
  if (service?.role === "external") {
    return `/service-inventory.html#service-${encodeURIComponent(service.id || "external")}`;
  }
  return `/environment-node.html?id=${encodeURIComponent(service?.id || "")}`;
}

function shortTime(value) {
  if (!value) return "-";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return "-";
  return date.toLocaleString("zh-CN", { hour12: false });
}

function parseSummary(raw) {
  if (!raw) return {};
  try {
    return JSON.parse(raw);
  } catch {
    return {};
  }
}

function renderRunGroup(title, rows, renderRow) {
  const group = document.createElement("section");
  group.className = "run-history-group";

  const heading = document.createElement("h3");
  heading.textContent = title;
  group.appendChild(heading);

  if (!rows?.length) {
    const empty = document.createElement("div");
    empty.className = "run-history-empty";
    empty.textContent = "暂无记录";
    group.appendChild(empty);
    return group;
  }

  rows.slice(0, 6).forEach((row) => {
    group.appendChild(renderRow(row));
  });
  return group;
}

function runItem(title, meta, detail, tone = "", href = "") {
  const item = document.createElement(href ? "a" : "article");
  item.className = `run-history-item ${tone}`;
  if (href) {
    item.href = href;
  }

  const top = document.createElement("div");
  top.className = "run-history-top";
  const strong = document.createElement("strong");
  strong.textContent = title || "-";
  const code = document.createElement("code");
  code.textContent = meta || "-";
  top.appendChild(strong);
  top.appendChild(code);

  const body = document.createElement("p");
  body.textContent = detail || "-";

  item.appendChild(top);
  item.appendChild(body);
  return item;
}

function runStatusTone(status) {
  const value = String(status || "").toLowerCase();
  if (["pass", "passed", "success", "ok"].includes(value)) return "passed";
  if (["fail", "failed", "error"].includes(value)) return "failed";
  if (["blocked", "warning"].includes(value)) return "warning";
  return value;
}

function renderRunHistory() {
  const target = el("runHistory");
  if (!target || !state.runs) return;
  target.innerHTML = "";

  target.appendChild(
    renderRunGroup("Workflow runs", state.runs.workflowRuns, (row) => {
      const summary = parseSummary(row.summaryJson);
      const stepCount = row.stepCount || summary.summary?.stepCount || summary.steps?.length || "-";
      return runItem(row.workflowId, row.status, `${shortTime(row.createdAt)} · steps ${stepCount}`, row.status, `/workflow-run.html?id=${encodeURIComponent(row.id)}`);
    })
  );
  target.appendChild(
    renderRunGroup("Replay runs", state.runs.replayRuns, (row) =>
      runItem(row.traceId, `${row.httpStatus || "-"} HTTP`, `${shortTime(row.createdAt)} · ${row.targetUrl || "-"}`, "", row.traceId ? `/replay-evidence.html?traceId=${encodeURIComponent(row.traceId)}` : "")
    )
  );
  target.appendChild(
    renderRunGroup("API case runs", state.caseRuns?.caseRuns || [], (row) =>
      runItem(row.caseId || row.runId, row.status || "-", `${shortTime(row.updatedAt)} · ${row.failureKind || row.operation || "-"}`, runStatusTone(row.status), row.runId ? `/evidence-viewer.html?caseRun=${encodeURIComponent(row.runId)}` : "")
    )
  );
  target.appendChild(
    renderRunGroup("Probe reports", state.runs.probeRuns, (row) =>
      runItem(row.service || "probe", row.detected ? "detected" : "not detected", `${shortTime(row.createdAt)} · ${row.traceId || "-"}`, row.detected ? "passed" : "")
    )
  );
}

async function refresh() {
  setMessage("refreshing...");
  const [snapshot, catalog, runs, agentTest, caseRuns] = await Promise.all([
    request("/api/state"),
    request("/api/catalog"),
    request("/api/runs"),
    request("/api/agent-test").catch((error) => ({ ok: false, summary: { latestFailureKind: error.message }, warnings: [error.message] })),
    request("/api/case/runs").catch((error) => ({ ok: false, caseRuns: [], warnings: [error.message] })),
  ]);
  state.snapshot = snapshot;
  state.catalog = catalog;
  state.runs = runs;
  state.agentTest = agentTest;
  state.caseRuns = caseRuns;
  render();
  setMessage("ready");
}

async function refreshRuns(showMessage = true) {
  const [runs, caseRuns] = await Promise.all([
    request("/api/runs"),
    request("/api/case/runs").catch((error) => ({ ok: false, caseRuns: [], warnings: [error.message] })),
  ]);
  state.runs = runs;
  state.caseRuns = caseRuns;
  render();
  if (showMessage) {
    setMessage("run history refreshed");
  }
}

el("refreshBtn").addEventListener("click", () => refresh().catch((error) => setMessage(error.message)));
el("refreshRunsBtn").addEventListener("click", () => refreshRuns().catch((error) => setMessage(error.message)));
refresh().catch((error) => setMessage(error.message));
