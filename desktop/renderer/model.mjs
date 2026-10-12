export const httpMethods = [
  "GET",
  "POST",
  "PUT",
  "PATCH",
  "DELETE",
  "HEAD",
  "OPTIONS",
];
export const uid = (prefix) => `${prefix}.${crypto.randomUUID()}`;
export const pretty = (value) =>
  typeof value === "string"
    ? tryPretty(value)
    : JSON.stringify(value ?? {}, null, 2);
function tryPretty(value) {
  try {
    return JSON.stringify(JSON.parse(value), null, 2);
  } catch {
    return value;
  }
}
export const methodTone = (method) =>
  ({
    GET: "green",
    POST: "orange",
    PUT: "blue",
    PATCH: "purple",
    DELETE: "red",
  })[method] || "muted";
export const statusLabel = (status) =>
  ({
    passed: "通过",
    failed: "失败",
    running: "执行中",
    queued: "排队中",
    skipped: "跳过",
    pending: "等待",
  })[status] ||
  status ||
  "未运行";
export const dateLabel = (value) =>
  value ? new Date(value).toLocaleString("zh-CN", { hour12: false }) : "—";

function rowValue(value) {
  return typeof value === "object"
    ? JSON.stringify(value)
    : String(value ?? "");
}
export function objectRows(value = {}) {
  return Object.entries(value || {}).map(([key, original]) => ({
    id: uid("row"),
    key,
    value: rowValue(original),
    enabled: true,
    original,
  }));
}
export function rowsObject(rows = []) {
  return Object.fromEntries(
    rows
      .filter((row) => row.enabled && row.key.trim())
      .map((row) => [
        row.key.trim(),
        rowValue(row.original) === row.value ? row.original : row.value,
      ]),
  );
}
export function requestDraft(record) {
  const execution = record.execution || {};
  const path = String(execution.path || "/")
    .replace(/%7B/gi, "{")
    .replace(/%7D/gi, "}");
  return {
    ...record,
    url: `${(record.baseUrl || "").replace(/\/$/, "")}${path}`,
    method: execution.method || "GET",
    params: objectRows(execution.query),
    headers: objectRows(execution.headers),
    bodyText:
      execution.body == null ? "" : JSON.stringify(execution.body, null, 2),
    codesText: (execution.expectedHttpCodes || [200]).join(", "),
    containsText: (execution.expectedResponseContains || []).join("\n"),
    absentText: (execution.expectedResponseNotContains || []).join("\n"),
    dirty: false,
  };
}
export function newRequest(services = []) {
  return requestDraft({
    id: uid("case"),
    nodeId: uid("node"),
    serviceId: services[0]?.id || "service.http",
    displayName: "新建接口",
    description: "",
    baseUrl: "",
    status: "active",
    timeoutSeconds: 30,
    editable: true,
    variables: [],
    execution: { method: "GET", path: "/", expectedHttpCodes: [200] },
  });
}
export function compileRequest(draft) {
  if (!draft.displayName?.trim()) throw new Error("请填写接口名称");
  let url;
  try {
    url = new URL(draft.url);
  } catch {
    throw new Error("请输入完整的 HTTP 或 HTTPS 请求地址");
  }
  if (
    !["http:", "https:"].includes(url.protocol) ||
    url.username ||
    url.password ||
    url.hash
  )
    throw new Error("请求地址不支持账号信息或锚点");
  if (!httpMethods.includes(draft.method))
    throw new Error("不支持该 HTTP 方法");
  const query = {
    ...Object.fromEntries(url.searchParams),
    ...rowsObject(draft.params),
  };
  let body = null;
  if (draft.bodyText.trim()) {
    try {
      body = JSON.parse(draft.bodyText);
    } catch {
      throw new Error("请求体不是有效的 JSON");
    }
  }
  const codes = draft.codesText
    .split(/[,，\s]+/)
    .filter(Boolean)
    .map(Number);
  if (
    !codes.length ||
    codes.some((code) => !Number.isInteger(code) || code < 100 || code > 599)
  )
    throw new Error("请填写有效的 HTTP 状态码，例如 200, 201");
  const lines = (value) =>
    value
      .split("\n")
      .map((line) => line.trim())
      .filter(Boolean);
  return {
    id: draft.id,
    nodeId: draft.nodeId,
    serviceId: draft.serviceId,
    displayName: draft.displayName.trim(),
    description: draft.description,
    status: draft.status,
    baseUrl: url.origin,
    timeoutSeconds: Number(draft.timeoutSeconds) || 0,
    execution: {
      ...draft.execution,
      method: draft.method,
      path: url.pathname.replace(/%7B/gi, "{").replace(/%7D/gi, "}"),
      query,
      headers: rowsObject(draft.headers),
      body,
      expectedHttpCodes: codes,
      expectedResponseContains: lines(draft.containsText),
      expectedResponseNotContains: lines(draft.absentText),
    },
  };
}

// Keep edits across tabs and refreshes. A changed baseline keeps its original
// ETag, so a save cannot silently overwrite an edit from another client.
export function reconcileDrafts(
  previous,
  records,
  etag,
  createDraft = requestDraft,
  getID = (item) => item.id,
) {
  const next = { ...previous };
  for (const record of records) {
    const id = getID(record);
    const baseline = JSON.stringify(record);
    const old = previous[id];
    if (old?.draft.dirty) {
      next[id] = { ...old, etag: old.baseline === baseline ? etag : old.etag };
    } else next[id] = { draft: createDraft(record), baseline, etag };
  }
  return next;
}
export function filterRequests(items, query) {
  const needle = query.trim().toLowerCase();
  return items.filter((item) =>
    [item.displayName, item.id, item.execution?.method, item.execution?.path]
      .join(" ")
      .toLowerCase()
      .includes(needle),
  );
}
export function responseFromRun(run) {
  return (
    run?.result?.response || run?.evidence?.response || run?.response || null
  );
}
export function variablesInDraft(draft) {
  const text = JSON.stringify([
    draft.url,
    draft.params,
    draft.headers,
    draft.bodyText,
  ]);
  return [
    ...new Set(
      [...text.matchAll(/\{\{override:([^{}|]+)(?:\|[^{}]*)?\}\}/g)].map(
        (match) => match[1],
      ),
    ),
  ];
}
