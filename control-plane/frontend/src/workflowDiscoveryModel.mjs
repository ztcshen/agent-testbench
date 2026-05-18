export function buildWorkflowDiscovery(workflows = [], filters = {}) {
  const rows = (Array.isArray(workflows) ? workflows : []).map(workflowDiscoveryRow);
  const query = String(filters.query || "").trim().toLowerCase();
  const target = normalizeTarget(filters);
  const visibleWorkflows = query ? rows.filter((row) => workflowMatchesQuery(row, query)) : rows;
  const targetWorkflows = target.enabled ? rows.filter((row) => row.stepCount === target.stepCount && row.interfaceCount === target.interfaceCount) : [];
  const targetChecklist = targetWorkflows.map(targetWorkflowChecklist);
  return {
    rows,
    visibleWorkflows,
    target,
    targetWorkflows,
    targetChecklist,
    summary: {
      total: rows.length,
      visible: visibleWorkflows.length,
      targetExact: targetWorkflows.length,
      maxStepCount: rows.reduce((max, row) => Math.max(max, row.stepCount), 0),
      maxInterfaceCount: rows.reduce((max, row) => Math.max(max, row.interfaceCount), 0),
    },
  };
}

export function targetWorkflowChecklist(row = {}) {
  const rows = (Array.isArray(row.stepLabels) ? row.stepLabels : []).map((step) => {
    const status = !step.interfaceId ? "missing-interface" : !step.caseId ? "missing-case" : "ready";
    return {
      sequence: step.index,
      stepId: step.id,
      title: step.title,
      interfaceId: step.interfaceId,
      caseId: step.caseId,
      status,
      stepHref: `/workflow-step.html?workflow=${encodeURIComponent(row.id || "")}&step=${encodeURIComponent(step.id || "")}`,
      interfaceHref: step.interfaceId ? `/interface-node.html?id=${encodeURIComponent(step.interfaceId)}` : "",
      caseHref: step.caseId ? caseManagementHref(row.id, step.caseId) : "",
      runsHref: step.caseId ? `/case-runs.html?case=${encodeURIComponent(step.caseId)}` : "",
    };
  });
  return {
    workflowId: row.id || "",
    title: row.title || row.id || "",
    rows,
    summary: {
      total: rows.length,
      ready: rows.filter((item) => item.status === "ready").length,
      missingInterface: rows.filter((item) => item.status === "missing-interface").length,
      missingCase: rows.filter((item) => item.status === "missing-case").length,
    },
  };
}

function caseManagementHref(workflowId, caseId) {
  const params = new URLSearchParams();
  if (workflowId) params.set("workflow", workflowId);
  params.set("case", caseId || "");
  return `/api-cases.html?${params.toString()}`;
}

export function workflowDiscoveryRow(workflow = {}) {
  const steps = Array.isArray(workflow.steps) ? workflow.steps : [];
  const interfaces = unique(steps.map((step) => step.nodeId || step.interfaceNodeId || step.serviceId).filter(Boolean));
  const cases = unique(steps.map((step) => step.caseId).filter(Boolean));
  const stepLabels = steps.map((step, index) => ({
    index: index + 1,
    id: step.id || step.stepId || `step-${index + 1}`,
    title: step.displayName || step.title || step.id || step.stepId || `Step ${index + 1}`,
    interfaceId: step.nodeId || step.interfaceNodeId || step.serviceId || "",
    caseId: step.caseId || "",
  }));
  return {
    id: workflow.id || "",
    title: workflow.displayName || workflow.title || workflow.id || "",
    description: workflow.description || "",
    stepCount: steps.length,
    interfaceCount: interfaces.length,
    caseCount: cases.length,
    interfaces,
    cases,
    stepLabels,
    workflow,
    coverageLabel: `${steps.length} steps / ${interfaces.length} interfaces`,
  };
}

function workflowMatchesQuery(row, query) {
  return [
    row.id,
    row.title,
    row.description,
    row.coverageLabel,
    `${row.stepCount} steps`,
    `${row.interfaceCount} interfaces`,
    ...row.interfaces,
    ...row.cases,
    ...row.stepLabels.flatMap((step) => [step.id, step.title, step.interfaceId, step.caseId]),
  ]
    .filter(Boolean)
    .join(" ")
    .toLowerCase()
    .includes(query);
}

function normalizeTarget(filters = {}) {
  const stepCount = Number(filters.targetStepCount || filters.stepCount || 0);
  const interfaceCount = Number(filters.targetInterfaceCount || filters.interfaceCount || 0);
  return {
    enabled: Number.isFinite(stepCount) && stepCount > 0 && Number.isFinite(interfaceCount) && interfaceCount > 0,
    stepCount: Number.isFinite(stepCount) ? stepCount : 0,
    interfaceCount: Number.isFinite(interfaceCount) ? interfaceCount : 0,
    label: String(filters.targetLabel || "Configured workflow target").trim(),
  };
}

function unique(values) {
  return [...new Set(values)];
}
