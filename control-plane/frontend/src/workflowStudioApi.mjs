import { requestJSON } from "./api.js";
import { readJSONResponse } from "./apiCasesApi.mjs";
import { buildStudioSavePayload } from "./workflowStudioModel.mjs";

export async function fetchStudio(fetchImpl = fetch) {
  const response = await fetchImpl("/api/workflow-studio", { cache: "no-store", headers: { Accept: "application/json" } });
  const payload = await readJSONResponse(response);
  const etag = response.headers.get("etag");
  if (!etag) throw new Error("场景目录没有版本信息");
  return { payload, etag };
}

export async function saveStudio(draft, etag, fetchImpl = fetch) {
  if (!etag) throw new Error("先读取场景目录版本");
  const response = await fetchImpl("/api/workflow-studio", { method: "PUT", headers: { Accept: "application/json", "content-type": "application/json", "If-Match": etag }, body: JSON.stringify(buildStudioSavePayload(draft)) });
  return { payload: await readJSONResponse(response), etag: response.headers.get("etag") || "" };
}

export function startStudioRun(workflowId) {
  return requestJSON("/api/cases/batch-runs", { method: "POST", body: JSON.stringify({ workflowId, requestId: crypto.randomUUID() }) });
}

export function fetchStudioRun(runId) {
  return requestJSON(`/api/cases/batch-runs/${encodeURIComponent(runId)}`);
}
