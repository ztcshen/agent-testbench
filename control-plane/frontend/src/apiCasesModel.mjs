export function buildCaseManagement(cases = [], filters = {}) {
  const normalized = Array.isArray(cases) ? cases : [];
  const activeFilters = normalizeFilters(filters);
  const rows = normalized.map(caseRow);
  const visibleRows = rows.filter((row) => rowMatchesFilters(row, activeFilters)).sort(rowComparator(activeFilters.sort));
  return {
    activeFilters,
    rows: visibleRows,
    facets: {
      status: buildFacet(rows, (row) => row.status),
      owner: buildFacet(rows, (row) => row.owner),
      priority: buildFacet(rows, (row) => row.priority),
      tag: buildFacet(rows.flatMap((row) => row.tags.map((tag) => ({ tag }))), (row) => row.tag),
      runState: buildFacet(rows, (row) => row.latestStatus),
    },
    readinessGroups: buildReadinessGroups(rows),
    summary: {
      total: rows.length,
      visible: visibleRows.length,
      ready: rows.filter((row) => row.readiness === "ready").length,
      needsReview: rows.filter((row) => row.readiness !== "ready").length,
      failedLatest: rows.filter((row) => row.latestStatus === "failed").length,
      neverRun: rows.filter((row) => row.latestStatus === "not-run").length,
    },
  };
}

export function buildWorkflowCaseContext(catalog = {}, workflowId = "", cases = []) {
  const id = String(workflowId || "").trim();
  const workflow = (Array.isArray(catalog?.workflows) ? catalog.workflows : []).find((item) => item.id === id);
  if (!id || !workflow) {
    return {
      enabled: false,
      workflowId: id,
      title: "",
      caseIds: [],
      interfaceIds: [],
      steps: [],
      summary: { steps: 0, cases: 0, interfaces: 0 },
    };
  }
  const caseRows = new Map((Array.isArray(cases) ? cases : []).map((item) => {
    const row = caseRow(item);
    return [row.id, row];
  }));
  const steps = (Array.isArray(workflow.steps) ? workflow.steps : []).map((step, index) => ({
    ...workflowStepCaseState({
      id: step.id || step.stepId || `step-${index + 1}`,
      title: step.displayName || step.title || step.id || step.stepId || `Step ${index + 1}`,
      interfaceId: step.nodeId || step.interfaceNodeId || step.serviceId || "",
      interfaceHref: interfaceHref({
        nodeId: step.nodeId || step.interfaceNodeId || "",
        serviceId: step.serviceId || "",
        workflowId: id,
        caseId: step.caseId || "",
      }),
      caseId: step.caseId || "",
    }, caseRows),
    sequence: index + 1,
  }));
  const caseIds = unique(steps.map((step) => step.caseId).filter(Boolean));
  const interfaceIds = unique(steps.map((step) => step.interfaceId).filter(Boolean));
  return {
    enabled: true,
    workflowId: id,
    title: workflow.displayName || workflow.title || workflow.id || id,
    caseIds,
    interfaceIds,
    steps,
    summary: {
      steps: steps.length,
      cases: caseIds.length,
      interfaces: interfaceIds.length,
      latestFailed: steps.filter((step) => step.latestStatus === "failed").length,
      sequenceIssues: steps.filter((step) => step.state !== "ready").length,
    },
  };
}

export function buildCaseCoverageBoard(report = {}, context = {}) {
  const workflowId = String(context.workflowId || "").trim();
  const scopedCaseIds = Array.isArray(context.caseIds) ? context.caseIds.filter(Boolean) : [];
  const scoped = scopedCaseIds.length > 0;
  const rows = (Array.isArray(report?.items) ? report.items : [])
    .filter((item) => !scoped || scopedCaseIds.includes(item.caseId))
    .map((item) => coverageRow(item, workflowId));
  const summary = coverageSummary(rows);
  return {
    summary,
    rows,
    groups: coverageGroups(rows),
  };
}

function interfaceHref({ nodeId = "", serviceId = "", workflowId = "", caseId = "" } = {}) {
  const target = nodeId || serviceId;
  if (!target) return "";
  const params = new URLSearchParams(nodeId ? { id: nodeId } : { serviceId });
  if (workflowId) params.set("workflow", workflowId);
  if (caseId) params.set("case", caseId);
  return `${nodeId ? "/interface-node.html" : "/interface-nodes.html"}?${params.toString()}`;
}

function coverageRow(item = {}, workflowId = "") {
  const caseId = item.caseId || "";
  const status = statusKey(item.latestStatus || "not-run");
  return {
    caseId,
    title: item.title || caseId,
    nodeId: item.nodeId || "",
    nodeName: item.nodeName || item.nodeId || "Unmapped interface",
    latestStatus: status,
    hasPassed: Boolean(item.hasPassed),
    reason: item.reason || "",
    elapsedMs: Number(item.elapsedMs || 0),
    gap: status !== "passed",
    caseRunsHref: caseRunsHref(caseId, workflowId),
    latestEvidenceHref: item.latestRunId ? evidenceHref(item.latestRunId, caseId, workflowId) : "",
  };
}

function caseRunsHref(caseId = "", workflowId = "") {
  const params = new URLSearchParams({ case: caseId });
  if (workflowId) params.set("workflow", workflowId);
  return `/case-runs.html?${params.toString()}`;
}

function evidenceHref(runId = "", caseId = "", workflowId = "") {
  const params = new URLSearchParams({ caseRun: runId });
  if (caseId) params.set("caseId", caseId);
  if (workflowId) params.set("workflow", workflowId);
  return `/evidence-viewer.html?${params.toString()}`;
}

function coverageSummary(rows = []) {
  const total = rows.length;
  const passed = rows.filter((row) => row.latestStatus === "passed").length;
  const failed = rows.filter((row) => row.latestStatus === "failed").length;
  const notRun = rows.filter((row) => row.latestStatus === "not-run").length;
  const covered = rows.filter((row) => row.hasPassed || row.latestStatus === "passed").length;
  const gaps = rows.filter((row) => row.gap).length;
  return {
    total,
    passed,
    failed,
    notRun,
    covered,
    gaps,
    passRate: total ? Math.round((passed / total) * 100) : 0,
  };
}

function coverageGroups(rows = []) {
  const groups = new Map();
  for (const row of rows) {
    const current = groups.get(row.nodeId) || {
      nodeId: row.nodeId,
      nodeName: row.nodeName,
      total: 0,
      passed: 0,
      failed: 0,
      notRun: 0,
      gapCount: 0,
      rows: [],
    };
    current.total += 1;
    if (row.latestStatus === "passed") current.passed += 1;
    if (row.latestStatus === "failed") current.failed += 1;
    if (row.latestStatus === "not-run") current.notRun += 1;
    if (row.gap) current.gapCount += 1;
    current.rows.push(row);
    groups.set(row.nodeId, current);
  }
  return [...groups.values()].sort((left, right) =>
    right.gapCount - left.gapCount ||
    right.failed - left.failed ||
    right.total - left.total ||
    left.nodeName.localeCompare(right.nodeName)
  );
}

function workflowStepCaseState(step, caseRows) {
  if (!step.caseId) {
    return {
      ...step,
      caseTitle: "",
      readiness: "missing",
      latestStatus: "not-run",
      latestEvidenceHref: "",
      state: "no-case",
    };
  }
  const row = caseRows.get(step.caseId);
  if (!row) {
    return {
      ...step,
      caseTitle: "",
      readiness: "missing",
      latestStatus: "not-run",
      latestEvidenceHref: "",
      state: "missing-case",
    };
  }
  return {
    ...step,
    caseTitle: row.title,
    readiness: row.readiness,
    latestStatus: row.latestStatus,
    latestEvidenceHref: row.latestEvidenceHref,
    state: workflowStepState(row),
  };
}

function workflowStepState(row) {
  if (row.latestStatus === "failed") return "latest-failed";
  if (row.readiness !== "ready") return "needs-review";
  return "ready";
}

export function caseManagementSearchText(row = {}) {
  return [
    row.id,
    row.title,
    row.operation,
    row.status,
    row.owner,
    row.priority,
    row.tags?.join(" "),
    row.sourceKind,
    row.executorId,
    row.latestStatus,
    row.latestFailureReason,
  ]
    .filter(Boolean)
    .join(" ")
    .toLowerCase();
}

function normalizeFilters(filters = {}) {
  return {
    query: String(filters.query || "").trim().toLowerCase(),
    status: String(filters.status || "").trim(),
    owner: String(filters.owner || "").trim(),
    priority: String(filters.priority || "").trim(),
    tag: String(filters.tag || "").trim(),
    runState: String(filters.runState || "").trim(),
    caseIds: Array.isArray(filters.caseIds) ? filters.caseIds.filter(Boolean) : [],
    caseIdsFilterEnabled: Boolean(filters.caseIdsFilterEnabled),
    sort: String(filters.sort || "readiness").trim(),
  };
}

function caseRow(item = {}) {
  const latestRun = item.latestRun || {};
  const latestStatus = statusKey(latestRun.status || item.latestStatus || (item.runCount ? "unknown" : "not-run"));
  const tags = Array.isArray(item.tags) ? item.tags.filter(Boolean) : [];
  const row = {
    id: item.id || "",
    title: item.title || item.displayName || item.id || "",
    operation: item.operation || "",
    status: item.status || "active",
    owner: item.owner || "unassigned",
    priority: item.priority || "unset",
    tags,
    casePath: item.casePath || "",
    sourceKind: item.sourceKind || "",
    sourcePath: item.sourcePath || "",
    executorId: item.executorId || "",
    baseUrl: item.baseUrl || "",
    evidenceDir: item.evidenceDir || "",
    timeoutSeconds: item.timeoutSeconds || 0,
    runCount: Number(item.runCount || 0),
    latestStatus,
    latestRunId: latestRun.runId || latestRun.id || item.latestRunId || "",
    latestElapsedMs: Number(latestRun.elapsedMs || item.latestElapsedMs || 0),
    latestFailureReason: latestRun.failureReason || item.failureReason || "",
    caseDef: item,
  };
  row.readiness = readiness(row);
  row.latestEvidenceHref = row.latestRunId ? `/evidence-viewer.html?${new URLSearchParams({ caseRun: row.latestRunId, caseId: row.id }).toString()}` : "";
  return row;
}

function readiness(row) {
  if (row.status !== "active") {
    return "needs-review";
  }
  if (!row.casePath || !row.sourceKind || !row.executorId) {
    return "needs-review";
  }
  return "ready";
}

function rowMatchesFilters(row, filters) {
  if (filters.caseIdsFilterEnabled && !filters.caseIds.includes(row.id)) return false;
  if (filters.status && row.status !== filters.status) return false;
  if (filters.owner && row.owner !== filters.owner) return false;
  if (filters.priority && row.priority !== filters.priority) return false;
  if (filters.tag && !row.tags.includes(filters.tag)) return false;
  if (filters.runState && row.latestStatus !== filters.runState) return false;
  if (filters.query && !caseManagementSearchText(row).includes(filters.query)) return false;
  return true;
}

function unique(values) {
  return [...new Set(values)];
}

function buildFacet(rows, keyFn) {
  const counts = new Map();
  for (const row of rows) {
    const key = String(keyFn(row) || "").trim();
    if (!key) continue;
    counts.set(key, (counts.get(key) || 0) + 1);
  }
  return [...counts.entries()]
    .map(([key, count]) => ({ key, label: key, count }))
    .sort((left, right) => right.count - left.count || left.label.localeCompare(right.label));
}

function buildReadinessGroups(rows) {
  return buildFacet(rows, (row) => row.readiness);
}

function rowComparator(sort) {
  switch (sort) {
    case "priority_desc":
      return (left, right) => priorityWeight(left.priority) - priorityWeight(right.priority) || left.id.localeCompare(right.id);
    case "latest_failed":
      return (left, right) => latestWeight(left.latestStatus) - latestWeight(right.latestStatus) || left.id.localeCompare(right.id);
    case "owner_asc":
      return (left, right) => left.owner.localeCompare(right.owner) || left.id.localeCompare(right.id);
    case "case_asc":
      return (left, right) => left.id.localeCompare(right.id);
    case "readiness":
    default:
      return (left, right) => readinessWeight(left.readiness) - readinessWeight(right.readiness) || priorityWeight(left.priority) - priorityWeight(right.priority) || left.id.localeCompare(right.id);
  }
}

function statusKey(value) {
  const normalized = String(value || "").toLowerCase();
  if (["pass", "passed", "success", "ok"].includes(normalized)) return "passed";
  if (["fail", "failed", "error"].includes(normalized)) return "failed";
  return normalized || "unknown";
}

function priorityWeight(priority) {
  const normalized = String(priority || "").toLowerCase();
  if (normalized === "p0") return 0;
  if (normalized === "p1") return 1;
  if (normalized === "p2") return 2;
  if (normalized === "p3") return 3;
  return 9;
}

function readinessWeight(value) {
  return value === "needs-review" ? 0 : 1;
}

function latestWeight(value) {
  if (value === "failed") return 0;
  if (value === "not-run") return 1;
  if (value === "unknown") return 2;
  return 3;
}
