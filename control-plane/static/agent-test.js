const agentTestState = {
  snapshot: null,
  caseRuns: null,
};

const agentCapabilityOrder = [
  "Evidence Diagnosis Index",
  "Config Mutable",
  "Capability Gap",
  "Subagent Acceptance",
];

const agentEl = (id) => document.getElementById(id);

function setAgentTestMessage(value) {
  agentEl("agentTestMessage").textContent = value;
}

async function agentTestRequest(path) {
  const response = await fetch(path);
  const body = await response.json();
  if (!response.ok) {
    throw new Error(body.error || response.statusText);
  }
  return body;
}

function renderAgentTestWorkbench() {
  const data = agentTestState.snapshot || {};
  const summary = data.summary || {};
  agentEl("agentTestSummary").textContent = `${summary.capabilityCount || 0} capabilities · ${summary.profileCount || 0} profiles · ${summary.runCount || 0} runs`;
  agentEl("agentProfileCount").textContent = summary.profileCount || 0;
  agentEl("agentRunCount").textContent = summary.runCount || 0;
  agentEl("agentConfigCount").textContent = summary.configEventCount || 0;
  agentEl("agentEscalationCount").textContent = summary.escalationEventCount || 0;
  agentEl("agentAcceptanceVerdict").textContent = summary.latestAcceptanceVerdict || "-";

  renderAgentCapabilities(data.capabilities || []);
  renderAgentProfiles(data.profiles || []);
  renderAgentRuns(data.agentRuns || []);
  renderAgentRunMatrix(data.profiles || [], data.agentRuns || []);
  renderAgentCaseEvidence(agentTestState.caseRuns?.caseRuns || []);
  renderAgentConfigEvents(data.configEvents || []);
  renderAgentCapabilityGaps(data.escalationEvents || [], data.agentRuns || []);
  renderAgentAcceptanceReports(data.acceptanceReports || []);
  renderAgentWarnings(data.warnings || []);
}

function renderAgentCapabilities(capabilities) {
  agentEl("agentCapabilitySummary").textContent = capabilities.length ? `${capabilities.length} 项后端能力已接入` : "暂无能力定义";
  const grid = agentEl("agentCapabilityGrid");
  grid.innerHTML = "";
  if (!capabilities.length) {
    grid.appendChild(emptyAgentState("未读取到 capability map。"));
    return;
  }
  [...capabilities].sort(compareAgentCapability).forEach((capability) => {
    const card = document.createElement("article");
    card.className = "agent-capability-card";
    const top = document.createElement("div");
    top.className = "agent-card-top";
    const title = document.createElement("strong");
    title.textContent = capability.title || capability.id || "-";
    const status = document.createElement("span");
    status.className = `agent-status ${capability.status || "unknown"}`;
    status.textContent = capability.status || "unknown";
    top.appendChild(title);
    top.appendChild(status);

    const desc = document.createElement("p");
    desc.textContent = capability.description || "";
    const evidence = document.createElement("div");
    evidence.className = "agent-chip-row";
    (capability.evidence || []).forEach((item) => evidence.appendChild(agentChip(item)));

    card.appendChild(top);
    card.appendChild(desc);
    card.appendChild(evidence);
    grid.appendChild(card);
  });
}

function compareAgentCapability(left, right) {
  const leftIndex = agentCapabilityOrder.findIndex((title) => (left.title || "").includes(title));
  const rightIndex = agentCapabilityOrder.findIndex((title) => (right.title || "").includes(title));
  return normalizeOrderIndex(leftIndex) - normalizeOrderIndex(rightIndex);
}

function normalizeOrderIndex(index) {
  return index === -1 ? agentCapabilityOrder.length : index;
}

function renderAgentProfiles(profiles) {
  agentEl("agentProfileSummary").textContent = profiles.length ? `${profiles.length} 个 profile` : "暂无 profile";
  const list = agentEl("agentProfileList");
  list.innerHTML = "";
  if (!profiles.length) {
    list.appendChild(emptyAgentState("configs/agent-test-profiles.json 暂无可显示 profile。"));
    return;
  }
  profiles.forEach((profile) => {
    const item = document.createElement("article");
    item.className = "agent-profile-item";
    const top = document.createElement("div");
    top.className = "agent-card-top";
    const title = document.createElement("strong");
    title.textContent = profile.title || profile.id;
    const code = document.createElement("code");
    code.textContent = profile.id || "-";
    top.appendChild(title);
    top.appendChild(code);

    const meta = document.createElement("p");
    meta.textContent = `${profile.stepCount || 0} steps · ${profile.mysqlProbeCount || 0} MySQL probes · ${(profile.allowedChanges || []).length} allowed config changes`;
    const chips = document.createElement("div");
    chips.className = "agent-chip-row";
    (profile.requiredConfig || []).forEach((cfg) => chips.appendChild(agentChip(`${cfg.kind}:${cfg.key}`)));
    (profile.evidenceKinds || []).forEach((kind) => chips.appendChild(agentChip(kind)));

    item.appendChild(top);
    item.appendChild(meta);
    item.appendChild(chips);
    list.appendChild(item);
  });
}

function renderAgentRuns(runs) {
  agentEl("agentRunSummary").textContent = runs.length ? `${runs.length} 条最近记录` : "暂无 run record";
  const list = agentEl("agentRunList");
  list.innerHTML = "";
  if (!runs.length) {
    list.appendChild(emptyAgentState("SQLite 里还没有 agent_runs。"));
    return;
  }
  runs.slice(0, 8).forEach((run) => {
    const item = document.createElement("article");
    item.className = `agent-run-item ${run.status || "unknown"}`;
    const top = document.createElement("div");
    top.className = "agent-card-top";
    const title = document.createElement("a");
    title.className = "agent-run-link";
    title.href = `/agent-run.html?runId=${encodeURIComponent(run.runId || "")}`;
    title.textContent = run.runId || "-";
    const status = document.createElement("span");
    status.className = `agent-status ${run.status || "unknown"}`;
    status.textContent = run.status || "unknown";
    top.appendChild(title);
    top.appendChild(status);

    const meta = document.createElement("p");
    meta.textContent = [run.resolvedServiceId, run.profileId, run.failureKind || "no failure_kind"].filter(Boolean).join(" · ");
    const diagnosis = document.createElement("p");
    diagnosis.className = "agent-diagnosis";
    diagnosis.textContent = run.diagnosis?.nextStep || run.diagnosis?.reason || run.evidenceRoot || "";
    item.appendChild(top);
    item.appendChild(meta);
    item.appendChild(diagnosis);
    list.appendChild(item);
  });
}

function renderAgentRunMatrix(profiles, runs) {
  const target = agentEl("agentRunMatrix");
  target.innerHTML = "";
  const profileIds = profiles.map((profile) => profile.id).filter(Boolean);
  const runProfiles = [...new Set(runs.map((run) => run.profileId).filter(Boolean))];
  const matrixProfileIds = [...new Set([...profileIds, ...runProfiles])];
  agentEl("agentRunMatrixSummary").textContent = matrixProfileIds.length
    ? `${matrixProfileIds.length} profiles · ${runs.length} runs`
    : "暂无 profile/run matrix";
  if (!matrixProfileIds.length) {
    target.appendChild(emptyAgentState("SQLite 里还没有可交叉查看的 Agent run。"));
    return;
  }
  const profileById = new Map(profiles.map((profile) => [profile.id, profile]));
  matrixProfileIds.forEach((profileId) => {
    const profile = profileById.get(profileId) || { id: profileId, title: profileId };
    const profileRuns = runs.filter((run) => run.profileId === profileId);
    const latest = profileRuns[0];
    const passed = profileRuns.filter((run) => run.status === "passed").length;
    const failed = profileRuns.filter((run) => run.status === "failed").length;
    const failureKinds = countAgentFailureKinds(profileRuns);

    const card = document.createElement("article");
    card.className = `profile-run-card ${latest?.status || "empty"}`;
    const top = document.createElement("div");
    top.className = "agent-card-top";
    const title = document.createElement("strong");
    title.textContent = profile.title || profile.id;
    const status = document.createElement("span");
    status.className = `agent-status ${latest?.status || "unknown"}`;
    status.textContent = latest?.status || "no run";
    top.appendChild(title);
    top.appendChild(status);

    const meta = document.createElement("p");
    meta.textContent = `${profileId} · ${profile.stepCount || 0} steps · ${profileRuns.length} runs`;
    const chips = document.createElement("div");
    chips.className = "agent-chip-row";
    chips.appendChild(agentChip(`${passed} passed`));
    chips.appendChild(agentChip(`${failed} failed`));
    Object.entries(failureKinds)
      .slice(0, 3)
      .forEach(([kind, count]) => chips.appendChild(agentChip(`${kind}: ${count}`)));
    const foot = document.createElement("code");
    foot.textContent = latest?.evidenceRoot || latest?.diagnosis?.nextStep || "no evidence yet";
    let detailLink = null;
    if (latest?.runId) {
      detailLink = document.createElement("a");
      detailLink.className = "agent-run-detail-link";
      detailLink.href = `/agent-run.html?runId=${encodeURIComponent(latest.runId)}`;
      detailLink.textContent = "查看 run evidence";
    }

    card.appendChild(top);
    card.appendChild(meta);
    card.appendChild(chips);
    card.appendChild(foot);
    if (detailLink) {
      card.appendChild(detailLink);
    }
    target.appendChild(card);
  });
}

function countAgentFailureKinds(runs) {
  return runs.reduce((acc, run) => {
    const kind = run.failureKind || "no failure_kind";
    acc[kind] = (acc[kind] || 0) + 1;
    return acc;
  }, {});
}

function renderAgentCaseEvidence(caseRuns) {
  const target = agentEl("agentCaseEvidenceList");
  agentEl("agentCaseEvidenceSummary").textContent = caseRuns.length
    ? `${caseRuns.length} case runs · latest ${caseRuns[0].status || "unknown"}`
    : "暂无 API Case evidence";
  target.innerHTML = "";
  if (!caseRuns.length) {
    target.appendChild(emptyAgentState("没有 .runtime/cases evidence bundle。"));
    return;
  }
  caseRuns.slice(0, 6).forEach((run) => {
    const item = document.createElement("a");
    item.className = `agent-case-evidence-item ${caseStatusTone(run.status)}`;
    item.href = `/evidence-viewer.html?caseRun=${encodeURIComponent(run.runId || "")}`;

    const top = document.createElement("div");
    top.className = "agent-card-top";
    const title = document.createElement("strong");
    title.textContent = run.caseId || run.runId || "-";
    const status = document.createElement("span");
    status.className = `agent-status ${caseStatusTone(run.status)}`;
    status.textContent = run.status || "unknown";
    top.appendChild(title);
    top.appendChild(status);

    const meta = document.createElement("p");
    meta.textContent = [run.operation, run.failureKind ? `failureKind ${run.failureKind}` : "", run.traceId].filter(Boolean).join(" · ");
    const foot = document.createElement("code");
    foot.textContent = run.failureReason || run.evidencePath || "open evidence viewer";

    item.appendChild(top);
    item.appendChild(meta);
    item.appendChild(foot);
    target.appendChild(item);
  });
}

function caseStatusTone(status) {
  const value = String(status || "").toLowerCase();
  if (["pass", "passed", "success", "ok"].includes(value)) return "passed";
  if (["fail", "failed", "error"].includes(value)) return "failed";
  return value || "unknown";
}

function renderAgentConfigEvents(events) {
  agentEl("agentConfigSummary").textContent = events.length ? `${events.length} 条配置事件` : "暂无配置事件";
  renderAgentEventList(agentEl("agentConfigEvents"), events, (event) => ({
    title: event.eventId,
    badge: event.status || event.kind,
    body: `${event.profileId || "-"} · ${event.kind}:${event.key}`,
    foot: `${blankDash(event.beforeValue)} -> ${blankDash(event.afterValue)}`,
  }));
}

function renderAgentCapabilityGaps(events, runs) {
  const blockedRuns = runs.filter((run) => run.blockedReport);
  agentEl("agentEscalationSummary").textContent =
    blockedRuns.length || events.length ? `${blockedRuns.length} blocked reports · ${events.length} escalations` : "暂无 capability gap 留证";
  const target = agentEl("agentEscalationEvents");
  target.innerHTML = "";
  if (!blockedRuns.length && !events.length) {
    target.appendChild(emptyAgentState("暂无 blocked report 或 escalation event。"));
    return;
  }

  blockedRuns.slice(0, 5).forEach((run) => {
    const report = run.blockedReport || {};
    const violations = Array.isArray(report.rule_violations) ? report.rule_violations : [];
    const article = document.createElement("article");
    article.className = "agent-event-item agent-blocked-report-item";

    const top = document.createElement("div");
    top.className = "agent-card-top";
    const title = document.createElement("a");
    title.className = "agent-run-link";
    title.href = `/agent-run.html?runId=${encodeURIComponent(run.runId || "")}`;
    title.textContent = run.runId || "-";
    const badge = document.createElement("span");
    badge.className = "agent-status failed";
    badge.textContent = "Blocked Report";
    top.appendChild(title);
    top.appendChild(badge);

    const body = document.createElement("p");
    body.textContent = report.reason || run.failureKind || "blocked report recorded";
    const foot = document.createElement("code");
    foot.textContent = violations.map((item) => item.rule || item.reason).filter(Boolean).join(" · ") || run.evidenceRoot || "";

    article.appendChild(top);
    article.appendChild(body);
    article.appendChild(foot);
    target.appendChild(article);
  });

  events.slice(0, Math.max(0, 8 - blockedRuns.length)).forEach((event) => {
    const article = document.createElement("article");
    article.className = "agent-event-item";
    const top = document.createElement("div");
    top.className = "agent-card-top";
    const title = agentEscalationRunLink(event);
    const badge = document.createElement("span");
    badge.className = "agent-status";
    badge.textContent = event.kind || event.status || "-";
    top.appendChild(title);
    top.appendChild(badge);
    const body = document.createElement("p");
    body.textContent = event.reason || event.runId || "-";
    const foot = document.createElement("code");
    foot.textContent = event.scope || event.evidenceRoot || "";
    article.appendChild(top);
    article.appendChild(body);
    article.appendChild(foot);
    target.appendChild(article);
  });
}

function agentEscalationRunLink(event) {
  if (event.runId) {
    const link = document.createElement("a");
    link.className = "agent-run-link agent-escalation-run-link";
    link.href = `/agent-run.html?runId=${encodeURIComponent(event.runId)}`;
    link.textContent = event.eventId || event.runId;
    return link;
  }
  const title = document.createElement("strong");
  title.textContent = event.eventId || "-";
  return title;
}

function renderAgentAcceptanceReports(reports) {
  agentEl("agentAcceptanceSummary").textContent = reports.length ? `${reports.length} 份诊断报告` : "暂无验收报告";
  renderAgentEventList(agentEl("agentAcceptanceReports"), reports, (report) => ({
    title: report.acceptanceId,
    badge: report.verdict || report.status,
    body: `${(report.caseResults || []).filter((item) => item.status === "passed").length}/${(report.caseResults || []).length} cases passed`,
    foot: report.reportPath || report.root || "",
  }));
}

function renderAgentEventList(target, items, mapItem) {
  target.innerHTML = "";
  if (!items.length) {
    target.appendChild(emptyAgentState("暂无记录。"));
    return;
  }
  items.slice(0, 8).forEach((item) => {
    const view = mapItem(item);
    const article = document.createElement("article");
    article.className = "agent-event-item";
    const top = document.createElement("div");
    top.className = "agent-card-top";
    const title = document.createElement("strong");
    title.textContent = view.title || "-";
    const badge = document.createElement("span");
    badge.className = "agent-status";
    badge.textContent = view.badge || "-";
    top.appendChild(title);
    top.appendChild(badge);
    const body = document.createElement("p");
    body.textContent = view.body || "";
    const foot = document.createElement("code");
    foot.textContent = view.foot || "";
    article.appendChild(top);
    article.appendChild(body);
    article.appendChild(foot);
    target.appendChild(article);
  });
}

function renderAgentWarnings(warnings) {
  if (warnings.length) {
    setAgentTestMessage(`${warnings.length} warning`);
  }
}

function agentChip(value) {
  const chip = document.createElement("span");
  chip.className = "agent-chip";
  chip.textContent = value;
  return chip;
}

function emptyAgentState(text) {
  const item = document.createElement("div");
  item.className = "agent-empty";
  item.textContent = text;
  return item;
}

function blankDash(value) {
  return value === "" || value == null ? "-" : value;
}

async function loadAgentTestWorkbench() {
  setAgentTestMessage("loading");
  try {
    const [snapshot, caseRuns] = await Promise.all([
      agentTestRequest("/api/agent-test"),
      agentTestRequest("/api/case/runs").catch((error) => ({ ok: false, caseRuns: [], warnings: [error.message] })),
    ]);
    agentTestState.snapshot = snapshot;
    agentTestState.caseRuns = caseRuns;
    renderAgentTestWorkbench();
    const warnings = [...(agentTestState.snapshot.warnings || []), ...(agentTestState.caseRuns.warnings || [])];
    if (!warnings.length) {
      setAgentTestMessage("ready");
    } else {
      setAgentTestMessage(`${warnings.length} warning`);
    }
  } catch (error) {
    setAgentTestMessage(error.message);
  }
}

agentEl("refreshAgentTestBtn").addEventListener("click", loadAgentTestWorkbench);
loadAgentTestWorkbench();
