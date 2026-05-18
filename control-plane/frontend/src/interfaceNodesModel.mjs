export function buildInterfaceNodeDirectoryContext(params = new URLSearchParams()) {
  const serviceId = String(params.get("serviceId") || "").trim();
  const workflowId = String(params.get("workflow") || params.get("workflowId") || "").trim();
  const caseId = String(params.get("case") || params.get("caseId") || "").trim();
  return {
    serviceId,
    workflowId,
    caseId,
    workflowCaseSetHref: workflowCaseSetHref(workflowId, caseId),
  };
}

export function interfaceNodeDetailHref(item = {}, context = {}) {
  const nodeId = String(item.id || "").trim();
  if (!nodeId) return "";
  const params = new URLSearchParams({ id: nodeId });
  if (context.workflowId) params.set("workflow", context.workflowId);
  if (context.caseId) params.set("case", context.caseId);
  return `/interface-node.html?${params.toString()}`;
}

function workflowCaseSetHref(workflowId, caseId) {
  if (!workflowId) return "";
  const params = new URLSearchParams({ workflow: workflowId });
  if (caseId) params.set("case", caseId);
  return `/api-cases.html?${params.toString()}`;
}
