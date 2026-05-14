const caseRunsState = {
  payload: null,
  timing: null,
  incompleteBatches: null,
};

const caseRunsEl = (id) => document.getElementById(id);

function setCaseRunMessage(value) {
  caseRunsEl("caseRunMessage").textContent = value;
}

async function caseRunsRequest(path) {
  const response = await fetch(path, { headers: { Accept: "application/json" } });
  const payload = await response.json().catch(() => ({}));
  if (!response.ok || payload.ok === false) {
    throw new Error(payload.error || `HTTP ${response.status}`);
  }
  return payload;
}

function caseRunEvidenceHref(run) {
  return `/evidence-viewer.html?caseRun=${encodeURIComponent(run.runId || "")}`;
}

function caseRunStatusTone(status) {
  const value = String(status || "").toLowerCase();
  if (["pass", "passed", "success", "ok"].includes(value)) return "passed";
  if (["fail", "failed", "error"].includes(value)) return "failed";
  return "";
}

function caseRunShortTime(value) {
  if (!value) return "-";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return "-";
  return date.toLocaleString("zh-CN", { hour12: false });
}

function caseRunFormatDuration(ms) {
  const value = Number(ms || 0);
  if (!Number.isFinite(value) || value <= 0) return "-";
  if (value < 1000) return `${Math.round(value)} ms`;
  return `${(value / 1000).toFixed(value >= 10000 ? 0 : 1)} s`;
}

function caseRunFormatSpeedup(value) {
  const parsed = Number(value || 0);
  if (!Number.isFinite(parsed) || parsed <= 0) return "-";
  return `${parsed.toFixed(parsed >= 10 ? 0 : 1)}x`;
}

function renderCaseRunRow(run) {
  const link = document.createElement("a");
  link.className = `run-history-item ${caseRunStatusTone(run.status)}`;
  link.href = caseRunEvidenceHref(run);

  const top = document.createElement("div");
  top.className = "run-history-top";
  const title = document.createElement("strong");
  title.textContent = run.caseId || run.runId || "-";
  const status = document.createElement("code");
  status.textContent = run.status || "-";
  top.appendChild(title);
  top.appendChild(status);

  const detail = document.createElement("p");
  const failureKind = run.failureKind ? ` · failureKind ${run.failureKind}` : "";
  detail.textContent = `${run.operation || "-"} · ${caseRunShortTime(run.updatedAt)}${failureKind}`;

  const reason = document.createElement("p");
  reason.className = "agent-run-detail-note";
  reason.textContent = run.failureReason || run.traceId || run.evidencePath || "open evidence bundle";

  link.appendChild(top);
  link.appendChild(detail);
  link.appendChild(reason);
  return link;
}

function caseRunTimingMetric(label, value) {
  const metric = document.createElement("div");
  metric.className = "case-timing-metric";
  const labelEl = document.createElement("span");
  labelEl.textContent = label;
  const valueEl = document.createElement("strong");
  valueEl.textContent = value;
  metric.appendChild(labelEl);
  metric.appendChild(valueEl);
  return metric;
}

function caseRunTimingSlowestText(row) {
  if (!row || !row.id) return "slowest row: -";
  const caseId = row.caseId ? ` · ${row.caseId}` : "";
  const wallTime = row.wallTimeProxyMs ? ` · wall ${caseRunFormatDuration(row.wallTimeProxyMs)}` : "";
  return `slowest row: ${row.kind || "-"} · ${row.status || "-"} · ${caseRunFormatDuration(row.durationMs)} · ${row.id}${caseId}${wallTime}`;
}

function caseTimingCommandText() {
  const parts = ["otsandbox", "case", "timing", "--kind", caseTimingKindFilter()];
  const freshness = caseTimingFreshnessFilter();
  if (freshness) {
    parts.push("--max-age-minutes", freshness);
  }
  return parts.join(" ");
}

function caseTimingExportCommandText() {
  return `${caseTimingCommandText()} --export jsonl`;
}

function caseTimingSummaryOnlyCommandText() {
  return `${caseTimingCommandText()} --summary-only`;
}

function renderCaseTimingCommand() {
  const target = caseRunsEl("caseTimingCommand");
  target.innerHTML = "";
  const command = document.createElement("code");
  command.textContent = caseTimingCommandText();
  target.appendChild(command);
  const exportCommand = document.createElement("code");
  exportCommand.textContent = caseTimingExportCommandText();
  target.appendChild(exportCommand);
  const summaryCommand = document.createElement("code");
  summaryCommand.textContent = caseTimingSummaryOnlyCommandText();
  target.appendChild(summaryCommand);
}

function renderCaseTimingSlowestHandoff(row) {
  const target = caseRunsEl("caseTimingSlowestHandoff");
  target.innerHTML = "";
  if (!row || !row.id || !row.source) return;
  const title = document.createElement("strong");
  title.textContent = `slowest: ${row.id}`;
  const source = document.createElement("code");
  source.textContent = row.source;
  target.appendChild(title);
  target.appendChild(source);
}

function renderCaseRunTimingSummary() {
  const summaryTarget = caseRunsEl("caseTimingSummary");
  const slowestTarget = caseRunsEl("caseTimingSlowest");
  summaryTarget.innerHTML = "";
  renderCaseTimingCommand();
  const timing = caseRunsState.timing;
  if (!timing) {
    summaryTarget.appendChild(caseRunTimingMetric("timing", "loading"));
    slowestTarget.textContent = "";
    renderCaseTimingSlowestHandoff(null);
    return;
  }
  const summary = timing.summary || {};
  summaryTarget.appendChild(caseRunTimingMetric("case runs", String(summary.caseRunCount || 0)));
  summaryTarget.appendChild(caseRunTimingMetric("candidate batches", String(summary.candidateBatchCount || 0)));
  summaryTarget.appendChild(caseRunTimingMetric("measured durations", String(summary.durationMeasuredCount || 0)));
  summaryTarget.appendChild(caseRunTimingMetric("max duration", caseRunFormatDuration(summary.maxDurationMs)));
  const speedup = summary.speedup || {};
  if (speedup.available) {
    summaryTarget.appendChild(caseRunTimingMetric("avg speedup", caseRunFormatSpeedup(speedup.averageEstimatedSpeedup)));
    summaryTarget.appendChild(caseRunTimingMetric("max speedup", caseRunFormatSpeedup(speedup.maxEstimatedSpeedup)));
    const wallCount = Number(speedup.wallTimeProxyMeasuredCount || 0);
    summaryTarget.appendChild(caseRunTimingMetric("wall proxy", `${wallCount} · ${caseRunFormatDuration(speedup.totalWallTimeProxyMs)}`));
  }
  const slowestRows = summary.slowestRows || {};
  const slowestRow = slowestRows.overall || slowestRows.caseRun || slowestRows.candidateBatch;
  slowestTarget.textContent = caseRunTimingSlowestText(slowestRow);
  renderCaseTimingSlowestHandoff(slowestRow);
  renderCaseRunTimingWarningSummary();
  renderCaseRunTimingWarnings();
}

function renderCaseRunTimingWarningSummary() {
  const target = caseRunsEl("caseTimingWarningSummary");
  target.innerHTML = "";
  const details = caseRunsState.timing?.warningDetails || [];
  if (!details.length) return;
  const counts = details.reduce((acc, detail) => {
    const kind = detail.kind || "unknown";
    acc[kind] = (acc[kind] || 0) + 1;
    return acc;
  }, {});
  Object.entries(counts)
    .sort(([left], [right]) => left.localeCompare(right))
    .forEach(([kind, count]) => {
      const item = document.createElement("span");
      item.textContent = `${kind}: ${count}`;
      target.appendChild(item);
    });
}

function renderCaseRunTimingWarnings() {
  const target = caseRunsEl("caseTimingWarnings");
  target.innerHTML = "";
  const warnings = caseRunsState.timing?.warnings || [];
  if (!warnings.length) return;
  warnings.slice(0, 3).forEach((warning) => {
    const item = document.createElement("code");
    item.textContent = warning;
    target.appendChild(item);
  });
}

function renderCaseIncompleteBatches() {
  const summaryTarget = caseRunsEl("caseIncompleteBatchSummary");
  const listTarget = caseRunsEl("caseIncompleteBatchList");
  summaryTarget.innerHTML = "";
  listTarget.innerHTML = "";
  const report = caseRunsState.incompleteBatches;
  if (!report) {
    summaryTarget.textContent = "incomplete batches: loading";
    return;
  }
  const items = Array.isArray(report.items) ? report.items : [];
  const warnings = Array.isArray(report.warnings) ? report.warnings : [];
  const label = document.createElement("span");
  label.textContent = `incomplete batches: ${items.length}`;
  summaryTarget.appendChild(label);
  const command = document.createElement("code");
  command.textContent = "dry-run: otsandbox case incomplete-batches";
  summaryTarget.appendChild(command);
  if (!items.length) {
    warnings.slice(0, 2).forEach((warning) => {
      const item = document.createElement("code");
      item.textContent = warning;
      listTarget.appendChild(item);
    });
    return;
  }
  items.slice(0, 5).forEach((item) => {
    const row = document.createElement("div");
    row.className = "case-incomplete-batch-item";
    const title = document.createElement("strong");
    title.textContent = `${item.id || "-"} · ${item.reason || "unknown"}`;
    const source = document.createElement("span");
    source.textContent = item.source || item.message || "";
    const suggestedCommand = document.createElement("code");
    suggestedCommand.textContent = item.suggestedCommand ? `cleanup: ${item.suggestedCommand}` : "cleanup command unavailable";
    row.appendChild(title);
    row.appendChild(source);
    row.appendChild(suggestedCommand);
    listTarget.appendChild(row);
  });
  if (items.length > 5) {
    const extra = document.createElement("code");
    extra.textContent = `+${items.length - 5} more`;
    listTarget.appendChild(extra);
  }
}

function caseTimingKindFilter() {
  return caseRunsEl("caseTimingKindFilter").value || "all";
}

function caseTimingFreshnessFilter() {
  return caseRunsEl("caseTimingFreshnessFilter").value || "";
}

async function loadCaseRunTiming() {
  const params = new URLSearchParams();
  params.set("kind", caseTimingKindFilter());
  const freshness = caseTimingFreshnessFilter();
  if (freshness) {
    params.set("maxAgeMinutes", freshness);
  }
  return caseRunsRequest(`/api/case/timing?${params.toString()}`).catch((error) => ({
    ok: false,
    summary: {},
    warnings: [error.message],
  }));
}

async function loadCaseIncompleteBatches() {
  return caseRunsRequest("/api/case/incomplete-batches").catch((error) => ({
    ok: false,
    dryRun: true,
    count: 0,
    items: [],
    warnings: [error.message],
  }));
}

async function refreshCaseRunTiming() {
  setCaseRunMessage("refreshing timing...");
  caseRunsState.timing = await loadCaseRunTiming();
  renderCaseRunTimingSummary();
  setCaseRunMessage("ready");
}

function filterCaseRuns(caseRuns) {
  const query = (caseRunsEl("caseRunFilter").value || "").trim().toLowerCase();
  const status = caseRunsEl("caseRunStatusFilter").value;
  return caseRuns.filter((run) => {
    const statusOk = !status || String(run.status || "").toLowerCase() === status;
    const text = [run.runId, run.caseId, run.operation, run.traceId, run.status, run.failureKind, run.failureReason, run.evidencePath]
      .filter(Boolean)
      .join(" ")
      .toLowerCase();
    return statusOk && (!query || text.includes(query));
  });
}

function renderCaseRunFacets(caseRuns, visibleRuns) {
  const target = caseRunsEl("caseRunFacets");
  target.innerHTML = "";
  const statusCounts = caseRuns.reduce((acc, run) => {
    const status = run.status || "unknown";
    acc[status] = (acc[status] || 0) + 1;
    return acc;
  }, {});
  const failureKindCounts = caseRuns.reduce((acc, run) => {
    const kind = run.failureKind || "no failureKind";
    acc[kind] = (acc[kind] || 0) + 1;
    return acc;
  }, {});
  target.appendChild(caseRunFacet(`${visibleRuns.length}/${caseRuns.length} visible`, "reset", ""));
  Object.entries(statusCounts).forEach(([status, count]) => {
    target.appendChild(caseRunFacet(`${status}: ${count}`, "status", status));
  });
  Object.entries(failureKindCounts)
    .slice(0, 4)
    .forEach(([kind, count]) => {
      target.appendChild(caseRunFacet(`failureKind ${kind}: ${count}`, "failureKind", kind));
    });
}

function caseRunFacet(label, kind, value) {
  const chip = document.createElement("button");
  chip.type = "button";
  chip.className = "agent-chip case-run-facet";
  chip.textContent = label;
  chip.addEventListener("click", () => applyCaseRunFacet(kind, value));
  return chip;
}

function applyCaseRunFacet(kind, value) {
  if (kind === "reset") {
    caseRunsEl("caseRunFilter").value = "";
    caseRunsEl("caseRunStatusFilter").value = "";
  } else if (kind === "status") {
    caseRunsEl("caseRunStatusFilter").value = value;
  } else if (kind === "failureKind") {
    caseRunsEl("caseRunFilter").value = value === "no failureKind" ? "" : value;
  }
  renderCaseRunsWorkbench();
}

function renderCaseRunsWorkbench() {
  const caseRuns = caseRunsState.payload?.caseRuns || [];
  const visibleRuns = filterCaseRuns(caseRuns);
  const warnings = caseRunsState.payload?.warnings || [];
  const latest = caseRuns[0];
  caseRunsEl("caseRunSummary").textContent = latest
    ? `${visibleRuns.length}/${caseRuns.length} case runs · latest ${latest.status || "unknown"} · ${latest.caseId || latest.runId}`
    : "0 case runs";
  caseRunsEl("caseRunMessage").title = warnings.join("\n");
  renderCaseRunFacets(caseRuns, visibleRuns);
  renderCaseRunTimingSummary();
  renderCaseIncompleteBatches();

  const target = caseRunsEl("caseRunList");
  target.innerHTML = "";
  if (!visibleRuns.length) {
    const empty = document.createElement("div");
    empty.className = "run-history-empty";
    empty.textContent = caseRuns.length ? "没有匹配的 API Case evidence" : warnings[0] || "暂无 API Case evidence";
    target.appendChild(empty);
    return;
  }
  visibleRuns.slice(0, 24).forEach((run) => target.appendChild(renderCaseRunRow(run)));
}

async function refreshCaseRuns() {
  setCaseRunMessage("refreshing...");
  const [payload, timing, incompleteBatches] = await Promise.all([
    caseRunsRequest("/api/case/runs"),
    loadCaseRunTiming(),
    loadCaseIncompleteBatches(),
  ]);
  caseRunsState.payload = payload;
  caseRunsState.timing = timing;
  caseRunsState.incompleteBatches = incompleteBatches;
  renderCaseRunsWorkbench();
  setCaseRunMessage("ready");
}

caseRunsEl("refreshCaseRunsBtn").addEventListener("click", () => refreshCaseRuns().catch((error) => setCaseRunMessage(error.message)));
caseRunsEl("caseRunFilter").addEventListener("input", renderCaseRunsWorkbench);
caseRunsEl("caseRunStatusFilter").addEventListener("change", renderCaseRunsWorkbench);
caseRunsEl("caseTimingKindFilter").addEventListener("change", () => refreshCaseRunTiming().catch((error) => setCaseRunMessage(error.message)));
caseRunsEl("caseTimingFreshnessFilter").addEventListener("change", () => refreshCaseRunTiming().catch((error) => setCaseRunMessage(error.message)));
refreshCaseRuns().catch((error) => setCaseRunMessage(error.message));
