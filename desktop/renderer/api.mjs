export async function api(path, options = {}) {
  const response = await window.testbench.request({ path, ...options });
  if (!response.ok) {
    const error = new Error(
      response.status === 412
        ? "目录已被其他操作修改。请刷新后核对当前内容，再保存。"
        : response.error || "请求失败",
    );
    error.status = response.status;
    error.payload = response.data;
    throw error;
  }
  return { data: response.data, etag: response.etag };
}
export async function loadWorkspace() {
  const responses = await Promise.allSettled([
    api("/api/request-editor"),
    api("/api/workflow-studio"),
    api("/api/catalog"),
    api("/api/case/runs"),
    api("/api/environments?all=true"),
    api("/api/store/current"),
    api("/api/maps"),
  ]);
  const [requests, studio, catalog, runs, environments, store, maps] =
    responses;
  const required = [requests, studio, catalog].find(
    (item) => item.status === "rejected",
  );
  if (required) throw required.reason;
  return {
    requests: requests.value.data.requests || [],
    requestETag: requests.value.etag,
    studio: studio.value.data,
    studioETag: studio.value.etag,
    catalog: catalog.value.data,
    runs: runs.status === "fulfilled" ? runs.value.data.caseRuns || [] : [],
    environments:
      environments.status === "fulfilled"
        ? environments.value.data.items || []
        : [],
    store: store.status === "fulfilled" ? store.value.data : {},
    maps: maps.status === "fulfilled" ? maps.value.data.maps || [] : [],
    issues: responses
      .filter((item) => item.status === "rejected")
      .map((item) => item.reason.message),
  };
}
export function saveRequest(draft, etag) {
  return api("/api/request-editor", {
    method: "PUT",
    etag,
    body: { request: draft },
  });
}
export function runRequest(caseId, overrides = {}) {
  return api("/api/test-kit/run", {
    method: "POST",
    body: { caseId, overrides },
  });
}
export function evidenceFor(run) {
  return api(
    `/api/case/evidence?${new URLSearchParams({ caseRunId: run.id })}`,
  );
}

export async function runScenario(workflowId, onProgress, signal) {
  const { data: started } = await api("/api/cases/batch-runs", {
    method: "POST",
    body: { workflowId, requestId: crypto.randomUUID() },
  });
  let report = started;
  onProgress(report);
  while (["running", "queued", "pending"].includes(report.status)) {
    if (signal?.aborted) return report;
    await new Promise((resolve) => setTimeout(resolve, 300));
    if (signal?.aborted) return report;
    report = (
      await api(
        `/api/cases/batch-runs/${encodeURIComponent(report.batchRunId)}`,
      )
    ).data;
    onProgress(report);
  }
  return report;
}
