const STORAGE_PREFIX = "open-test-sandbox-evidence:";

const el = (id) => document.getElementById(id);

function parseQuery() {
  const params = new URLSearchParams(window.location.search);
  return {
    key: params.get("key") || "",
    caseRun: params.get("caseRun") || params.get("runId") || "",
  };
}

async function evidenceViewerRequest(path) {
  const response = await fetch(path, { headers: { Accept: "application/json" } });
  const payload = await response.json().catch(() => ({}));
  if (!response.ok || payload.ok === false) {
    throw new Error(payload.error || `HTTP ${response.status}`);
  }
  return payload;
}

function normalizeCaseEvidence(payload) {
  const evidence = payload.evidence || {};
  const summary = evidence.summary || {};
  const trace = evidence.trace || {};
  const request = evidence.request || {};
  const response = evidence.response || {};
  const topology = evidence.topology || {};
  const systems = Array.isArray(evidence.logs) ? evidence.logs : Array.isArray(trace.systems) ? trace.systems : [];
  const continuity = trace.trace_continuity || trace.traceContinuity || {};
  const continuityStatus =
    continuity.status || (continuity.ok === true ? "passed" : continuity.ok === false ? "failed" : "unknown");
  const normalizedSystems = systems.map((system) => ({
    id: system.id,
    name: system.name || system.id,
    found: Boolean(system.found),
    coreLogs: system.coreLogs || system.lines || [],
    error: system.message || system.error || "",
  }));

  return {
    step: {
      title: summary.case_id || "Case run evidence",
      goal: summary.operation || "Case run evidence",
      stageTitle: "API Case",
      caseId: summary.case_id || "-",
      path: (trace.required_systems || trace.requiredSystems || []).join(" -> "),
      correlators: trace.correlators || [],
      systems: normalizedSystems,
      traceContinuity: {
        status: continuityStatus,
        reason: continuity.reason || "",
        requestId: summary.request_id || trace.requestId || summary.trace_id || "",
        matchedSystems: continuity.matched_systems || continuity.matchedSystems || [],
        missingSystems: continuity.missing_systems || continuity.missingSystems || [],
      },
      meta: `${request.method || request.sdk_operation || request.sdkOperation || "request"} / ${response.http_code || "-"}`,
      topology,
    },
    caseDiagnostics: {
      summary,
      request,
      response,
      assertions: evidence.assertions || {},
      services: Array.isArray(evidence.services) ? evidence.services : [],
      mysql: evidence.mysql || {},
      fixture: evidence.fixture || emptyFixtureEvidence(),
      topology,
    },
  };
}

async function loadPayload() {
  const { key, caseRun } = parseQuery();
  if (caseRun) {
    const payload = await evidenceViewerRequest(`/api/case/evidence?runId=${encodeURIComponent(caseRun)}`);
    return normalizeCaseEvidence(payload);
  }
  if (!key.startsWith(STORAGE_PREFIX)) {
    return null;
  }
  try {
    const raw = localStorage.getItem(key);
    return raw ? JSON.parse(raw) : null;
  } catch (error) {
    return null;
  }
}

function renderEmpty() {
  el("viewerTitle").textContent = "日志不可用";
  el("viewerSubtitle").textContent = "没有找到当前步骤的日志快照，请回到主页面重新打开。";
  el("viewerContinuity").textContent = "-";
  el("viewerCodeHints").textContent = "-";
  el("viewerGrid").innerHTML = '<section class="viewer-card"><pre class="viewer-pre">没有找到日志快照。</pre></section>';
}

function summarizeLogLine(line) {
  let summary = String(line || "").trim();
  summary = summary.replace(/^\[?\d{4}-\d{2}-\d{2}[^\]]*\]?\s*/, "");
  summary = summary.replace(/^(\[[^\]]+\]\s*)+/, "");
  summary = summary.replace(/\s+/g, " ").trim();
  if (!summary) {
    return "日志详情";
  }
  return summary.length > 140 ? `${summary.slice(0, 140)}…` : summary;
}

function extractCodeHints(systems = []) {
  const hints = [];
  const seen = new Set();
  const javaRefPattern = /\[([A-Za-z0-9_$.]+\.java:[A-Za-z0-9_$<>]+:\d+)\]/g;
  const classLinePattern = /\]\s+\[[A-Z]+\s+\]\s+([A-Za-z0-9_$.]+)\s+(\d+)\s+--/;

  systems.forEach((system) => {
    (system.coreLogs || []).forEach((line) => {
      let match;
      while ((match = javaRefPattern.exec(line)) !== null) {
        const ref = match[1];
        const key = `${system.id}:${ref}`;
        if (seen.has(key)) {
          continue;
        }
        seen.add(key);
        hints.push({ systemId: system.id, systemName: system.name, ref, sample: summarizeLogLine(line) });
      }

      const classMatch = line.match(classLinePattern);
      if (!classMatch) {
        return;
      }
      const ref = `${classMatch[1]}:${classMatch[2]}`;
      const key = `${system.id}:${ref}`;
      if (seen.has(key)) {
        return;
      }
      seen.add(key);
      hints.push({ systemId: system.id, systemName: system.name, ref, sample: summarizeLogLine(line) });
    });
  });

  return hints.slice(0, 12);
}

function normalizeEvidenceStep(step) {
  const trace = step.trace || {};
  const traceCorrelators = Array.isArray(trace.correlators) ? trace.correlators : [];
  const stepCorrelators = Array.isArray(step.correlators) ? step.correlators : [];
  const traceSystems = Array.isArray(trace.systems) ? trace.systems : [];
  const stepSystems = Array.isArray(step.systems) ? step.systems : [];
  const visited = Array.isArray(trace.visited) ? trace.visited : [];

  return {
    ...step,
    path: step.path || (visited.length ? visited.join(" -> ") : ""),
    requestId: step.requestId || trace.requestId || "",
    traceContinuity: step.traceContinuity || trace.traceContinuity || null,
    correlators: stepCorrelators.length ? stepCorrelators : traceCorrelators,
    systems: stepSystems.length ? stepSystems : traceSystems,
  };
}

function deriveTraceContinuity(step) {
  if (step.traceContinuity) {
    return step.traceContinuity;
  }
  const requestId = step.requestId || (step.correlators || [])[0] || "";
  if (!requestId) {
    return null;
  }
  const matchedSystems = [];
  const missingSystems = [];
  (step.systems || []).filter((system) => system.found).forEach((system) => {
    const lines = system.coreLogs || [];
    const matched = lines.some((line) => line.includes(requestId));
    if (matched) {
      matchedSystems.push(system.id);
    } else {
      missingSystems.push(system.id);
    }
  });
  return {
    status: missingSystems.length ? "partial" : matchedSystems.length ? "passed" : "failed",
    reason: missingSystems.length ? `缺失系统: ${missingSystems.join(", ")}` : "当前已加载日志都包含 trace id",
    requestId,
    matchedSystems,
    missingSystems,
  };
}

function renderSignalCard(step, codeHints) {
  const section = document.createElement("section");
  section.className = "viewer-card viewer-signal-card";

  const head = document.createElement("div");
  head.className = "viewer-card-head";
  head.innerHTML = "<h2>排障信号</h2><span>Trace / code focus</span>";
  section.appendChild(head);

  const list = document.createElement("div");
  list.className = "viewer-signal-list";
  const continuity = deriveTraceContinuity(step) || {};
  const signals = [
    {
      label: "TRACE CONTINUITY",
      value: continuity.status || "unknown",
      detail: continuity.reason || "没有 continuity 结论",
    },
    {
      label: "REQUEST ID",
      value: step.requestId || continuity.requestId || "-",
      detail: (step.correlators || []).join(" · ") || "没有关联字段",
    },
    {
      label: "MATCHED SYSTEMS",
      value: (continuity.matchedSystems || []).join(", ") || "-",
      detail: (continuity.missingSystems || []).length ? `缺失: ${continuity.missingSystems.join(", ")}` : "当前匹配系统都带有 trace id",
    },
  ];

  signals.forEach((signal) => {
    const item = document.createElement("article");
    item.className = "viewer-signal-item";
    item.innerHTML = `
      <span>${signal.label}</span>
      <strong>${signal.value}</strong>
      <p>${signal.detail}</p>
    `;
    list.appendChild(item);
  });
  section.appendChild(list);

  const hintBlock = document.createElement("div");
  hintBlock.className = "viewer-code-hints";
  const hintTitle = document.createElement("h3");
  hintTitle.textContent = "疑似代码入口";
  hintBlock.appendChild(hintTitle);

  if (!codeHints.length) {
    const empty = document.createElement("p");
    empty.className = "viewer-code-hint-empty";
    empty.textContent = "当前日志里没有提取到稳定的类 / 方法位点。";
    hintBlock.appendChild(empty);
  } else {
    const hintList = document.createElement("div");
    hintList.className = "viewer-code-hint-list";
    codeHints.forEach((hint) => {
      const row = document.createElement("article");
      row.className = "viewer-code-hint";
      row.innerHTML = `
        <strong>${hint.systemName || hint.systemId}</strong>
        <code>${hint.ref}</code>
        <p>${hint.sample}</p>
      `;
      hintList.appendChild(row);
    });
    hintBlock.appendChild(hintList);
  }
  section.appendChild(hintBlock);

  return section;
}

function diagnosticCard(label, value, detail) {
  const item = document.createElement("article");
  item.className = "viewer-diagnostic-item";

  const labelEl = document.createElement("span");
  labelEl.textContent = label;
  const valueEl = document.createElement("strong");
  valueEl.textContent = value || "-";
  const detailEl = document.createElement("p");
  detailEl.textContent = detail || "-";

  item.appendChild(labelEl);
  item.appendChild(valueEl);
  item.appendChild(detailEl);
  return item;
}

function failedAssertionKeys(assertions = {}) {
  return Object.entries(assertions)
    .filter(([key, value]) => (key.endsWith("_ok") || key === "passed") && value === false)
    .map(([key]) => key);
}

function emptyFixtureEvidence() {
  return { status: "empty", applyRuns: [], summary: { applyCount: 0, restoreCount: 0, failedCount: 0 } };
}

function fixtureEvidenceSummary(fixtureEvidence = {}) {
  const applyRuns = Array.isArray(fixtureEvidence.applyRuns) ? fixtureEvidence.applyRuns : [];
  const summary = fixtureEvidence.summary || {};
  return {
    status: fixtureEvidence.status || (applyRuns.length ? applyRuns[applyRuns.length - 1]?.status : "empty"),
    applyCount: Number(summary.applyCount || applyRuns.filter((run) => run.status === "applied").length || 0),
    restoreCount: Number(summary.restoreCount || applyRuns.filter((run) => run.status === "restored").length || 0),
    failedCount: Number(summary.failedCount || applyRuns.filter((run) => String(run.status || "").includes("failed")).length || 0),
    applyRuns,
  };
}

function renderFixtureEvidence(fixtureEvidence = emptyFixtureEvidence()) {
  const summary = fixtureEvidenceSummary(fixtureEvidence);
  const section = document.createElement("section");
  section.className = "viewer-card viewer-fixture-evidence";

  const head = document.createElement("div");
  head.className = "viewer-card-head";
  head.innerHTML = `<h2>前置证据</h2><span>${summary.status || "empty"} · ${summary.applyRuns.length} runs</span>`;
  section.appendChild(head);

  if (!summary.applyRuns.length) {
    const empty = document.createElement("p");
    empty.className = "viewer-code-hint-empty";
    empty.textContent = "当前 Case 不需要前置证据，或本次运行没有应用前置数据。";
    section.appendChild(empty);
    return section;
  }

  const grid = document.createElement("div");
  grid.className = "viewer-diagnostic-grid";
  grid.appendChild(diagnosticCard("FIXTURE STATUS", summary.status, `${summary.applyCount} apply · ${summary.restoreCount} restore · ${summary.failedCount} failed`));
  grid.appendChild(diagnosticCard("FIXTURE INSTANCE", summary.applyRuns[0]?.fixtureInstanceId || "-", "来自运行前自动选取的前置数据包"));
  grid.appendChild(diagnosticCard("CLEANUP", summary.failedCount ? "needs attention" : "restored", "执行后按运行前快照恢复现场"));
  section.appendChild(grid);

  const list = document.createElement("div");
  list.className = "viewer-fixture-run-list";
  summary.applyRuns.forEach((run) => {
    const item = document.createElement("article");
    item.className = "viewer-fixture-run";
    const cleanupSql = Array.isArray(run.cleanupSql) ? run.cleanupSql : [];
    item.innerHTML = `
      <div class="viewer-card-head">
        <h3>${run.status || "-"}</h3>
        <span>${run.fixtureInstanceId || "-"}</span>
      </div>
      <pre class="viewer-pre">${JSON.stringify({ appliedRows: run.appliedRows || {}, cleanupSql, failureReason: run.failureReason || "" }, null, 2)}</pre>
    `;
    list.appendChild(item);
  });
  section.appendChild(list);
  return section;
}

function renderEvidenceTopology(topology = {}) {
  if (!topology || (!topology.status && !topology.traceId && !topology.requestId)) {
    return null;
  }
  const renderer = window.SandboxTopologyRenderer;
  const edges = renderer.edges(topology);
  const nodes = renderer.nodes(topology, edges);
  const section = document.createElement("section");
  section.className = "viewer-card viewer-case-topology";
  const head = document.createElement("div");
  head.className = "viewer-card-head";
  head.innerHTML = `<h2>SkyWalking 拓扑</h2><span>${topology.status || "-"} · ${topology.requestId || "-"} · ${topology.traceId || "-"}</span>`;
  section.appendChild(head);
  section.appendChild(renderer.renderDiagram(nodes, edges, { markerPrefix: "evidence-arrow" }));
  section.appendChild(renderer.renderEdgeList(edges, { emptyText: "SkyWalking 没有确认调用边；保留当前 trace 状态。" }));
  if (topology.textTopology) {
    const raw = document.createElement("pre");
    raw.className = "viewer-pre";
    raw.textContent = topology.textTopology;
    section.appendChild(raw);
  }
  return section;
}

function renderCaseDiagnostics(caseDiagnostics) {
  if (!caseDiagnostics) return null;
  const { summary = {}, request = {}, response = {}, assertions = {}, services = [], mysql = {}, fixture = emptyFixtureEvidence() } = caseDiagnostics;
  const section = document.createElement("section");
  section.className = "viewer-card viewer-case-diagnostics";

  const head = document.createElement("div");
  head.className = "viewer-card-head";
  head.innerHTML = "<h2>API Case Diagnostics</h2><span>HTTP / ASSERTIONS / MYSQL</span>";
  section.appendChild(head);

  const failedAssertions = failedAssertionKeys(assertions);
  const okServices = services.filter((service) => service.ok).length;
  const queryCount = Array.isArray(mysql.queries) ? mysql.queries.length : 0;
  const sqlRows = Array.isArray(mysql.queries) ? mysql.queries.reduce((total, query) => total + Number(query.row_count || query.rowCount || 0), 0) : 0;
  const expectedCodes = assertions.expected_http_codes || summary.expected_http_codes || [];

  const grid = document.createElement("div");
  grid.className = "viewer-diagnostic-grid";
  grid.appendChild(diagnosticCard("HTTP STATUS", String(response.http_code || assertions.actual_http_code || summary.actual_http_code || "-"), `expected ${expectedCodes.join(", ") || "-"} · request ${response.request_id || "-"}`));
  grid.appendChild(diagnosticCard("FAILURE KIND", summary.failure_kind || assertions.failure_kind || "none", summary.failure_reason || assertions.failure_reason || "no failure reason"));
  grid.appendChild(diagnosticCard("ASSERTIONS", failedAssertions.length ? `${failedAssertions.length} failed` : "passed", failedAssertions.join(", ") || "all tracked assertions passed"));
  grid.appendChild(diagnosticCard("MYSQL", mysql.ok === false ? "failed" : "ok", `${queryCount} queries · ${sqlRows} rows`));
  const fixtureSummary = fixtureEvidenceSummary(fixture);
  grid.appendChild(diagnosticCard("FIXTURE", fixtureSummary.status || "empty", `${fixtureSummary.applyCount} apply · ${fixtureSummary.restoreCount} restore`));
  grid.appendChild(diagnosticCard("SERVICES", `${okServices}/${services.length || 0} ok`, services.map((service) => `${service.id}:${service.health || service.state || "-"}`).join(" · ") || "no service snapshot"));
  grid.appendChild(diagnosticCard("REQUEST", request.sdk_operation || request.sdkOperation || request.method || "-", summary.evidence_path || "runtime case bundle"));
  section.appendChild(grid);

  const rawTitle = document.createElement("h3");
  rawTitle.textContent = "RAW CASE BUNDLE";
  rawTitle.className = "viewer-raw-title";
  const raw = document.createElement("pre");
  raw.className = "viewer-pre viewer-raw-case-bundle";
  raw.textContent = JSON.stringify(caseDiagnostics, null, 2);
  section.appendChild(rawTitle);
  section.appendChild(raw);
  return section;
}

function renderViewer(payload) {
  if (payload?.error) {
    renderEmpty();
    el("viewerSubtitle").textContent = payload.error;
    return;
  }
  if (!payload || !payload.step) {
    renderEmpty();
    return;
  }

  const step = normalizeEvidenceStep(payload.step);
  el("viewerTitle").textContent = step.title || "日志查看页";
  el("viewerSubtitle").textContent = step.goal || "查看当前步骤的完整系统日志。";
  el("viewerStage").textContent = step.stageTitle || "阶段";
  el("viewerCase").textContent = step.caseId || "-";
  el("viewerPath").textContent = step.path || "-";
  el("viewerCorrelators").textContent = (step.correlators || []).join(" · ") || "-";
  el("viewerMeta").textContent = step.meta || "-";
  const continuity = deriveTraceContinuity(step) || {};
  const matchedSystems = continuity.matchedSystems || [];
  const codeHints = extractCodeHints(step.systems || []);
  el("viewerContinuity").textContent = continuity.status ? `${continuity.status} · ${matchedSystems.length} systems` : "-";
  el("viewerCodeHints").textContent = codeHints.length ? `${codeHints.length} 个定位提示` : "0 个定位提示";

  const grid = el("viewerGrid");
  grid.innerHTML = "";

  const systems = (step.systems || []).filter((system) => system.found);
  grid.appendChild(renderSignalCard(step, codeHints));
  grid.appendChild(renderFixtureEvidence(payload.caseDiagnostics?.fixture || emptyFixtureEvidence()));
  const topologyCard = renderEvidenceTopology(step.topology || payload.caseDiagnostics?.topology);
  if (topologyCard) {
    grid.appendChild(topologyCard);
  }
  const caseDiagnostics = renderCaseDiagnostics(payload.caseDiagnostics);
  if (caseDiagnostics) {
    grid.appendChild(caseDiagnostics);
  }
  if (!systems.length) {
    grid.innerHTML += '<section class="viewer-card"><pre class="viewer-pre">当前步骤没有采集到可展示的日志。</pre></section>';
    return;
  }

  systems.forEach((system) => {
    const section = document.createElement("section");
    section.className = "viewer-card";
    const logs = system.coreLogs?.length ? system.coreLogs.join("\n\n") : system.error || "未匹配到核心日志";
    section.innerHTML = `
      <div class="viewer-card-head">
        <h2>${system.name}</h2>
        <span>${system.coreLogs?.length || 0} 条核心日志</span>
      </div>
      <pre class="viewer-pre">${logs}</pre>
    `;
    grid.appendChild(section);
  });
}

async function initViewer() {
  try {
    renderViewer(await loadPayload());
  } catch (error) {
    renderEmpty();
    el("viewerSubtitle").textContent = error.message || "Evidence 加载失败。";
  }
}

initViewer();
