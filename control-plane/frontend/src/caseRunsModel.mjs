export function buildRunAnalysis(runs = [], filters = {}) {
  const normalizedRuns = Array.isArray(runs) ? runs : [];
  const activeFilters = normalizeFilters(filters);
  const visibleRuns = normalizedRuns.filter((run) => runMatchesFilters(run, activeFilters));
  const gridRows = buildGridRows(visibleRuns, activeFilters.sort, activeFilters);
  const failedRuns = normalizedRuns.filter(isFailedRun);
  return {
    activeFilters,
    visibleRuns,
    caseFocus: buildCaseFocus(visibleRuns, activeFilters.caseId, activeFilters),
    workflowContext: buildWorkflowContext(activeFilters),
    grid: {
      columns: gridColumns(),
      rows: gridRows,
    },
    statusFacets: buildFacets(normalizedRuns, statusKey, "status"),
    failureCategoryFacets: buildFacets(failedRuns, (run) => failureCategoryKey(run, activeFilters.failureCategoryRules), "failureCategory"),
    failureGroups: buildFailureGroups(normalizedRuns, activeFilters),
    failureTriage: buildFailureTriage(failedRuns, activeFilters),
    flakyCandidates: buildFlakyCandidates(normalizedRuns, activeFilters),
    slowest: [...normalizedRuns].sort((left, right) => durationMs(right) - durationMs(left)).slice(0, 5),
    summary: {
      total: normalizedRuns.length,
      visible: visibleRuns.length,
      passed: normalizedRuns.filter((run) => statusKey(run) === "passed").length,
      failed: normalizedRuns.filter((run) => statusKey(run) === "failed").length,
    },
  };
}

function buildCaseFocus(runs, caseId, filters = {}) {
  if (!caseId) return null;
  const ordered = [...runs].sort((left, right) => updatedTime(right) - updatedTime(left));
  const latest = ordered[0] || {};
  return {
    caseId,
    total: ordered.length,
    passed: ordered.filter((run) => statusKey(run) === "passed").length,
    failed: ordered.filter((run) => statusKey(run) === "failed").length,
    latestRunId: latest.runId || latest.id || "",
    latestStatus: latest.status || "not-run",
    latestUpdatedAt: latest.updatedAt || latest.createdAt || "",
    latestEvidenceHref: latest.runId || latest.id ? evidenceHref(latest, filters) : "",
    longestDurationMs: ordered.reduce((max, run) => Math.max(max, durationMs(run)), 0),
  };
}

function buildWorkflowContext(filters) {
  if (!filters.workflowId) {
    return null;
  }
  const params = new URLSearchParams({ workflow: filters.workflowId });
  if (filters.caseId) {
    params.set("case", filters.caseId);
  }
  return {
    workflowId: filters.workflowId,
    caseId: filters.caseId,
    caseSetHref: `/api-cases.html?${params.toString()}`,
  };
}

export function caseRunSearchText(run = {}) {
  return [
    run.runId,
    run.caseId,
    run.operation,
    run.traceId,
    run.status,
    run.failureCategory,
    run.failureKind,
    run.failureReason,
    run.evidencePath,
  ]
    .filter(Boolean)
    .join(" ")
    .toLowerCase();
}

function normalizeFilters(filters = {}) {
  return {
    query: String(filters.query || "").trim(),
    caseId: String(filters.caseId || "").trim(),
    workflowId: String(filters.workflowId || "").trim(),
    status: String(filters.status || "").trim().toLowerCase(),
    failureCategory: String(filters.failureCategory || "").trim(),
    failureCategoryRules: Array.isArray(filters.failureCategoryRules) ? filters.failureCategoryRules : [],
    sort: String(filters.sort || "updated_desc").trim(),
  };
}

function runMatchesFilters(run, filters) {
  if (filters.caseId && String(run.caseId || "") !== filters.caseId) {
    return false;
  }
  if (filters.status && statusKey(run) !== filters.status) {
    return false;
  }
  if (filters.failureCategory && failureCategoryKey(run, filters.failureCategoryRules) !== filters.failureCategory) {
    return false;
  }
  if (filters.query && !caseRunSearchText(run).includes(filters.query.toLowerCase())) {
    return false;
  }
  return true;
}

function buildFacets(runs, keyFn, field) {
  const counts = new Map();
  for (const run of runs) {
    const key = keyFn(run);
    if (!key) {
      continue;
    }
    counts.set(key, (counts.get(key) || 0) + 1);
  }
  return [...counts.entries()]
    .map(([key, count]) => ({ key, label: key, count, field }))
    .sort((left, right) => right.count - left.count || left.label.localeCompare(right.label));
}

function statusKey(run = {}) {
  const value = String(run.status || "unknown").toLowerCase();
  if (["pass", "passed", "success", "ok"].includes(value)) return "passed";
  if (["fail", "failed", "error"].includes(value)) return "failed";
  return value || "unknown";
}

function failureCategoryKey(run = {}, rules = []) {
  return failureTriageCategory(run, rules).label;
}

function isFailedRun(run) {
  return statusKey(run) === "failed";
}

function durationMs(run = {}) {
  const value = Number(run.durationMs || run.elapsedMs || 0);
  return Number.isFinite(value) ? value : 0;
}

function gridColumns() {
  return [
    { id: "status", label: "Status", sortable: true },
    { id: "case", label: "Case", sortable: true },
    { id: "operation", label: "Operation", searchable: true },
    { id: "failureCategory", label: "Failure", sortable: true, searchable: true },
    { id: "duration", label: "Duration", sortable: true, align: "right" },
    { id: "updated", label: "Updated", sortable: true },
    { id: "evidence", label: "Evidence" },
  ];
}

function buildGridRows(runs, sort, filters = {}) {
  return [...runs]
    .sort(sortRunComparator(sort))
    .map((run, index) => ({
      id: run.runId || run.id || `${run.caseId || "case"}-${index}`,
      run,
      caseId: run.caseId || "",
      operation: run.operation || "",
      status: run.status || "unknown",
      statusKey: statusKey(run),
      failureCategory: failureCategoryKey(run, filters.failureCategoryRules),
      defaultFailureCategory: defaultFailureCategory(run),
      failureReason: run.failureReason || "",
      traceId: run.traceId || "",
      durationMs: durationMs(run),
      durationRank: index + 1,
      updatedAt: run.updatedAt || run.createdAt || "",
      evidenceHref: evidenceHref(run, filters),
    }));
}

function sortRunComparator(sort) {
  switch (sort) {
    case "duration_desc":
      return (left, right) => durationMs(right) - durationMs(left) || updatedTime(right) - updatedTime(left);
    case "duration_asc":
      return (left, right) => durationMs(left) - durationMs(right) || updatedTime(right) - updatedTime(left);
    case "case_asc":
      return (left, right) => String(left.caseId || "").localeCompare(String(right.caseId || "")) || updatedTime(right) - updatedTime(left);
    case "status_asc":
      return (left, right) => statusKey(left).localeCompare(statusKey(right)) || updatedTime(right) - updatedTime(left);
    case "updated_asc":
      return (left, right) => updatedTime(left) - updatedTime(right);
    case "updated_desc":
    default:
      return (left, right) => updatedTime(right) - updatedTime(left);
  }
}

function updatedTime(run = {}) {
  const value = Date.parse(run.updatedAt || run.createdAt || "");
  return Number.isFinite(value) ? value : 0;
}

function evidenceHref(run = {}, filters = {}) {
  const params = new URLSearchParams({ caseRun: run.runId || "" });
  if (run.caseId) {
    params.set("caseId", run.caseId);
  }
  if (filters.workflowId) {
    params.set("workflow", filters.workflowId);
  }
  return `/evidence-viewer.html?${params.toString()}`;
}

function caseRunsHref(caseId = "", filters = {}) {
  const params = new URLSearchParams({ case: caseId });
  if (filters.workflowId) {
    params.set("workflow", filters.workflowId);
  }
  return `/case-runs.html?${params.toString()}`;
}

function buildFlakyCandidates(runs, filters = {}) {
  const groups = new Map();
  for (const run of runs) {
    const caseId = String(run.caseId || "").trim();
    if (!caseId) continue;
    const current = groups.get(caseId) || {
      caseId,
      operation: run.operation || "",
      items: [],
      passed: 0,
      failed: 0,
      failureReasons: [],
    };
    current.items.push(run);
    if (!current.operation && run.operation) current.operation = run.operation;
    if (statusKey(run) === "passed") current.passed += 1;
    if (statusKey(run) === "failed") {
      current.failed += 1;
      const reason = String(run.failureReason || run.failureKind || defaultFailureCategory(run) || "").trim();
      if (reason && !current.failureReasons.includes(reason)) {
        current.failureReasons.push(reason);
      }
    }
    groups.set(caseId, current);
  }
  return [...groups.values()]
    .filter((group) => group.passed > 0 && group.failed > 0)
    .map((group) => {
      const ordered = [...group.items].sort((left, right) => updatedTime(right) - updatedTime(left));
      const latest = ordered[0] || {};
      const total = group.items.length;
      return {
        caseId: group.caseId,
        operation: group.operation,
        total,
        passed: group.passed,
        failed: group.failed,
        latestStatus: statusKey(latest),
        latestRunId: latest.runId || latest.id || "",
        latestEvidenceHref: evidenceHref(latest, filters),
        caseRunsHref: caseRunsHref(group.caseId, filters),
        failureReasons: group.failureReasons.slice(0, 3),
        flakeScore: total ? Math.round((Math.min(group.passed, group.failed) / total) * 100 * 2) : 0,
      };
    })
    .sort((left, right) => right.flakeScore - left.flakeScore || right.failed - left.failed || left.caseId.localeCompare(right.caseId));
}

function buildFailureGroups(runs, filters = {}) {
  const groups = new Map();
  for (const run of runs.filter(isFailedRun)) {
    const key = failureCategoryKey(run, filters.failureCategoryRules);
    const current = groups.get(key) || { key, label: key, count: 0, longestRunId: "", longestDurationMs: 0 };
    const runDuration = durationMs(run);
    current.count += 1;
    if (runDuration >= current.longestDurationMs) {
      current.longestRunId = run.runId || "";
      current.longestDurationMs = runDuration;
    }
    groups.set(key, current);
  }
  return [...groups.values()].sort((left, right) => right.count - left.count || left.label.localeCompare(right.label));
}

function buildFailureTriage(runs, filters = {}) {
  const groups = new Map();
  for (const run of runs) {
    const category = failureTriageCategory(run, filters.failureCategoryRules);
    const current = groups.get(category.key) || {
      key: category.key,
      label: category.label,
      matchedBy: category.matchedBy,
      ruleRank: category.ruleRank,
      count: 0,
      longestRunId: "",
      longestDurationMs: 0,
      sampleRunId: "",
      sampleCaseId: "",
      sampleReason: "",
      sampleEvidenceHref: "",
    };
    const runDuration = durationMs(run);
    current.count += 1;
    if (!current.sampleRunId) {
      current.sampleRunId = run.runId || run.id || "";
      current.sampleCaseId = run.caseId || "";
      current.sampleReason = run.failureReason || run.failureKind || defaultFailureCategory(run);
      current.sampleEvidenceHref = evidenceHref(run, filters);
    }
    if (runDuration >= current.longestDurationMs) {
      current.longestRunId = run.runId || run.id || "";
      current.longestDurationMs = runDuration;
    }
    groups.set(category.key, current);
  }
  return [...groups.values()].sort((left, right) => left.ruleRank - right.ruleRank || right.count - left.count || left.label.localeCompare(right.label));
}

function failureTriageCategory(run = {}, rules = []) {
  const baseCategory = defaultFailureCategory(run);
  for (let index = 0; index < rules.length; index += 1) {
    const rule = rules[index] || {};
    if (failureCategoryRuleMatches(rule, run, baseCategory)) {
      const label = String(rule.category || rule.name || baseCategory || "uncategorized").trim();
      return {
        key: label,
        label,
        matchedBy: `rule ${index + 1}`,
        ruleRank: index + 1,
      };
    }
  }
  return {
    key: baseCategory,
    label: baseCategory,
    matchedBy: "default",
    ruleRank: Number.MAX_SAFE_INTEGER,
  };
}

function failureCategoryRuleMatches(rule = {}, run = {}, baseCategory = "") {
  const matchers = rule.matchers || {};
  if (Array.isArray(matchers.statuses) && matchers.statuses.length && !containsFold(matchers.statuses, statusKey(run))) {
    return false;
  }
  if (Array.isArray(matchers.failureCategories) && matchers.failureCategories.length && !containsFold(matchers.failureCategories, baseCategory)) {
    return false;
  }
  if (Array.isArray(matchers.messageContains) && matchers.messageContains.length && !containsMessageFragment(matchers.messageContains, run.failureReason || run.failureKind || "")) {
    return false;
  }
  return Boolean(
    (Array.isArray(matchers.statuses) && matchers.statuses.length) ||
    (Array.isArray(matchers.failureCategories) && matchers.failureCategories.length) ||
    (Array.isArray(matchers.messageContains) && matchers.messageContains.length),
  );
}

function defaultFailureCategory(run = {}) {
  return String(run.defaultFailureCategory || run.failureCategory || run.failureKind || "uncategorized").trim() || "uncategorized";
}

function containsFold(values = [], want = "") {
  const normalized = String(want || "").trim().toLowerCase();
  return values.some((value) => String(value || "").trim().toLowerCase() === normalized);
}

function containsMessageFragment(fragments = [], message = "") {
  const normalized = String(message || "").toLowerCase();
  return fragments.some((fragment) => {
    const value = String(fragment || "").trim().toLowerCase();
    return value && normalized.includes(value);
  });
}
