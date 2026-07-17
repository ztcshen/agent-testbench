export function parseEvidenceBody(value) {
  if (value === undefined || value === null || value === "") return {};
  if (typeof value === "object") return value;
  if (typeof value !== "string") return {};
  try {
    return JSON.parse(quoteUnsafeJSONIntegers(value));
  } catch {
    return {};
  }
}

export function exportedValues(step, result) {
  const out = {};
  for (const item of step?.exports || []) {
    const name = item?.name;
    const value = valueAtPath(exportRoot(result, item?.from), item?.path);
    if (name && value !== undefined && value !== null && value !== "") {
      out[name] = normalizeExportValue(value);
    }
  }
  return out;
}

export function storeNativeWorkflowBatchRequest(workflowId, requestId) {
  return {
    requestId: String(requestId || "").trim(),
    workflowId: String(workflowId || "").trim(),
  };
}

export function workflowStepResultOK(result) {
  const status = String(result?.status || "").trim().toLowerCase();
  if (["passed", "success", "ok"].includes(status)) {
    return result?.bodyHealth?.ok !== false;
  }
  if (["failed", "error", "skipped", "cancelled"].includes(status)) {
    return false;
  }
  return Boolean(result?.ok) && result?.bodyHealth?.ok !== false;
}

export function workflowBatchRunnerState(report, { startedAt, elapsedMs = 0 } = {}) {
  const status = String(report?.status || "running").trim().toLowerCase() || "running";
  const cases = Array.isArray(report?.cases) ? report.cases : [];
  const steps = cases.filter((item) => ["passed", "failed", "skipped"].includes(String(item?.status || "").trim().toLowerCase()));
  const failed = steps.find((item) => !workflowStepResultOK(item));
  const message = status === "running"
    ? `completed ${Number(report?.completed || steps.length)}/${Number(report?.total || cases.length)}`
    : status === "passed"
      ? "workflow completed"
      : String(report?.error || failed?.error || "workflow failed");
  const state = {
    status,
    steps,
    runId: String(report?.batchRunId || ""),
    message,
    elapsedMs,
  };
  if (status === "running" && startedAt !== undefined) {
    state.startedAt = startedAt;
  }
  return state;
}

export function workflowBatchPollTimeoutMs(report, catalogBudgetMs = 0) {
  const cases = Array.isArray(report?.cases) ? report.cases : [];
  const total = Number.isInteger(Number(report?.total)) && Number(report?.total) > 0
    ? Number(report.total)
    : cases.length;
  const plannedMs = cases.length === total && cases.length > 0
    ? cases.reduce((sum, item) => {
      const seconds = Number(item?.timeoutSeconds);
      return sum + (Number.isFinite(seconds) && seconds > 0 ? seconds : 90) * 1000;
    }, 0)
    : total * 90000;
  const postProcessMs = total * 15000;
  const catalogMs = Number.isFinite(Number(catalogBudgetMs)) && Number(catalogBudgetMs) > 0 ? Number(catalogBudgetMs) : 0;
  return Math.max(75000, plannedMs + postProcessMs + 60000, catalogMs + postProcessMs + 60000);
}

export function assertTerminalWorkflowBatchReport(report) {
  const status = String(report?.status || "").trim().toLowerCase();
  if (status !== "passed" && status !== "failed") {
    throw new Error(`workflow batch is not terminal: ${status || "missing"}`);
  }
  const cases = Array.isArray(report?.cases) ? report.cases : [];
  const total = Number(report?.total);
  const completed = Number(report?.completed);
  const passed = Number(report?.passed || 0);
  const failed = Number(report?.failed || 0);
  const skipped = Number(report?.skipped || 0);
  const counted = passed + failed + skipped;
  const allCasesTerminal = cases.every((item) => ["passed", "failed", "skipped"].includes(String(item?.status || "").trim().toLowerCase()));
  if (!Number.isInteger(total) || total < 1 || completed !== total || counted !== total || cases.length !== total || !allCasesTerminal) {
    throw new Error(`workflow batch returned an incomplete terminal report: ${completed}/${total}`);
  }
  const caseCounts = cases.reduce((counts, item) => {
    const itemStatus = String(item?.status || "").trim().toLowerCase();
    counts[itemStatus] += 1;
    return counts;
  }, { passed: 0, failed: 0, skipped: 0 });
  if (caseCounts.passed !== passed || caseCounts.failed !== failed || caseCounts.skipped !== skipped || (status === "passed" && passed !== total)) {
    throw new Error("workflow batch returned an inconsistent terminal report");
  }
  return report;
}

function exportRoot(result, source) {
  const request = requestEvidence(result);
  const response = responseEvidence(result);
  const responseBody = parseEvidenceBody(response.body);
  switch (source) {
    case "request":
    case "requestBody":
      return request.body || {};
    case "requestQuery":
      return request.query || {};
    case "response":
    case "responseBody":
      return responseBody;
    case "responseHeaders":
      return response.headers || {};
    default:
      return responseBody;
  }
}

function valueAtPath(root, path) {
  if (!path) return undefined;
  return String(path).split(".").reduce((current, part) => {
    if (current === undefined || current === null) return undefined;
    if (Array.isArray(current) && /^\d+$/.test(part)) return current[Number(part)];
    return current[part];
  }, root);
}

function requestEvidence(result) {
  return result?.result?.request || {};
}

function responseEvidence(result) {
  return result?.result?.response || {};
}

function quoteUnsafeJSONIntegers(value) {
  return String(value).replace(/([:[,]\s*)(-?\d{16,})(?=\s*[,}\]])/g, "$1\"$2\"");
}

function normalizeExportValue(value) {
  if (typeof value === "number" && Number.isInteger(value) && Math.abs(value) >= Number.MAX_SAFE_INTEGER) {
    return String(value);
  }
  return value;
}
