const jsonHeaders = { Accept: "application/json" };

export function buildStoreCaseRunPayload(caseDef = {}) {
  const caseId = String(caseDef.id || "").trim();
  if (!caseId) {
    throw new Error("case id is required");
  }
  return { caseId };
}

export async function runStoreCase(caseDef, fetchImpl = fetch) {
  const response = await fetchImpl("/api/test-kit/run", {
    method: "POST",
    headers: { ...jsonHeaders, "content-type": "application/json" },
    body: JSON.stringify(buildStoreCaseRunPayload(caseDef)),
  });
  return normalizeStoreCaseRunResult(await readJSONResponse(response));
}

export function normalizeStoreCaseRunResult(payload = {}) {
  const existing = payload.report && typeof payload.report === "object" ? payload.report : {};
  const summary = payload.summary && typeof payload.summary === "object" ? payload.summary : {};
  const request = payload.result?.request && typeof payload.result.request === "object" ? payload.result.request : {};
  return {
    ...payload,
    report: {
      ...existing,
      status: existing.status ?? payload.status ?? "failed",
      run_id: existing.run_id ?? payload.runId ?? "",
      case_id: existing.case_id ?? payload.caseId ?? "",
      actual_http_code: existing.actual_http_code ?? summary.httpCode ?? 0,
      request_id: existing.request_id ?? request.requestId ?? request.request_id ?? "",
    },
  };
}

export async function fetchCaseCatalog(fetchImpl = fetch) {
  const response = await fetchImpl("/api/case-catalog", { cache: "no-store", headers: jsonHeaders });
  const payload = await readJSONResponse(response);
  const etag = response.headers.get("etag") || "";
  if (!etag) {
    throw new Error("case catalog response is missing an ETag");
  }
  return { payload, etag };
}

export async function fetchCaseCatalogHistory(fetchImpl = fetch) {
  const response = await fetchImpl("/api/case-catalog/history?limit=20", { cache: "no-store", headers: jsonHeaders });
  return readJSONResponse(response);
}

export async function patchCaseCatalog(caseDef, etag, fetchImpl = fetch) {
  return mutateCaseCatalog("/api/case-catalog", "PATCH", { case: buildCaseCatalogPatch(caseDef) }, etag, fetchImpl);
}

export async function rollbackCaseCatalog(revision, etag, fetchImpl = fetch) {
  const targetRevision = Number(revision);
  if (!Number.isInteger(targetRevision) || targetRevision <= 0) {
    throw new Error("catalog revision must be a positive integer");
  }
  return mutateCaseCatalog("/api/case-catalog/rollback", "POST", { revision: targetRevision }, etag, fetchImpl);
}

export function buildCaseEditorDraft(caseDef = {}) {
  return {
    id: String(caseDef.id || ""),
    displayName: String(caseDef.displayName || caseDef.title || ""),
    description: String(caseDef.description || ""),
    nodeId: String(caseDef.nodeId || ""),
    caseType: String(caseDef.caseType || ""),
    scenario: String(caseDef.scenario || ""),
    tagsText: (Array.isArray(caseDef.tags) ? caseDef.tags : []).join(", "),
    priority: String(caseDef.priority || ""),
    owner: String(caseDef.owner || ""),
    requestTemplateId: String(caseDef.requestTemplateId || ""),
    renderMode: String(caseDef.renderMode || ""),
    requiredForAdmission: Boolean(caseDef.requiredForAdmission),
    status: String(caseDef.status || "draft"),
    sortOrder: Number(caseDef.sortOrder || 0),
  };
}

export function buildNewCaseEditorDraft() {
  return buildCaseEditorDraft({ status: "draft" });
}

export function buildCaseCatalogPatch(draft = {}) {
  const id = String(draft.id || "").trim();
  if (!id) {
    throw new Error("case id is required");
  }
  return {
    id,
    displayName: String(draft.displayName || ""),
    description: String(draft.description || ""),
    nodeId: String(draft.nodeId || "").trim(),
    caseType: String(draft.caseType || "").trim(),
    scenario: String(draft.scenario || ""),
    tags: normalizeTags(draft.tagsText ?? draft.tags),
    priority: String(draft.priority || "").trim(),
    owner: String(draft.owner || "").trim(),
    requestTemplateId: String(draft.requestTemplateId || "").trim(),
    renderMode: String(draft.renderMode || "").trim(),
    requiredForAdmission: Boolean(draft.requiredForAdmission),
    status: String(draft.status || "draft").trim(),
    sortOrder: Number.isFinite(Number(draft.sortOrder)) ? Number(draft.sortOrder) : 0,
  };
}

async function mutateCaseCatalog(path, method, payload, etag, fetchImpl) {
  if (!String(etag || "").trim()) {
    throw new Error("case catalog ETag is required");
  }
  const response = await fetchImpl(path, {
    method,
    headers: { ...jsonHeaders, "content-type": "application/json", "If-Match": etag },
    body: JSON.stringify(payload),
  });
  const body = await readJSONResponse(response);
  return { payload: body, etag: response.headers.get("etag") || "" };
}

async function readJSONResponse(response) {
  const body = await response.json().catch(() => ({}));
  if (!response.ok) {
    const issue = Array.isArray(body.issues) && body.issues.length ? `: ${body.issues.map(issueMessage).join("; ")}` : "";
    const error = new Error(`${body.error || response.statusText}${issue}`);
    error.status = response.status;
    error.payload = body;
    throw error;
  }
  return body;
}

function issueMessage(issue) {
  if (typeof issue === "string") return issue;
  return String(issue?.message || issue?.code || "validation issue");
}

function normalizeTags(value) {
  const tags = Array.isArray(value) ? value : String(value || "").split(",");
  return [...new Set(tags.map((tag) => String(tag).trim()).filter(Boolean))];
}
