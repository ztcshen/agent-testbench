import { buildWorkflowDiscovery } from "./workflowDiscoveryModel.mjs";

export function buildCapabilityCards({ runs = {}, caseRuns = {}, catalog = {} } = {}) {
  const workflowRuns = runs?.workflowRuns || [];
  const latestRun = workflowRuns[0];
  const latestCaseRun = caseRuns?.caseRuns?.[0];
  const cards = [];
  const targetCard = configuredWorkflowTargetCard(catalog);
  if (targetCard) {
    cards.push(targetCard);
  }
  cards.push(
    {
      kind: "workflow-evidence",
      title: "Workflow Evidence",
      detail: "Requests, responses, logs, journal entries, database hints, and trace topology.",
      href: latestRun ? `/workflow-run.html?id=${encodeURIComponent(latestRun.id)}` : "/workflow-run.html",
      meta: latestRun ? `latest ${latestRun.status}` : "no run yet",
    },
    {
      kind: "run-topology",
      title: "Run Topology",
      detail: "Confirmed edges, external exits, unresolved exits, request ids, and trace ids.",
      href: latestRun ? `/trace-topology.html?workflowRunId=${encodeURIComponent(latestRun.id)}` : "/trace-topology.html",
      meta: latestRun ? `run #${latestRun.id}` : "no run yet",
    },
    {
      kind: "api-case-evidence",
      title: "API Case Evidence",
      detail: "Runtime case bundles, request and response snapshots, trace continuity, and failure kind.",
      href: "/case-runs.html",
      meta: latestCaseRun ? `${latestCaseRun.status || "unknown"} · ${latestCaseRun.failureKind || "no failure kind"}` : "no case run yet",
    },
    {
      kind: "service-inventory",
      title: "Service Inventory",
      detail: "Registry-backed services, runtime nodes, containers, ports, and declared dependencies.",
      href: "/service-inventory.html",
      meta: `${catalog?.services?.length || 0} services`,
    },
    {
      kind: "replay-probe",
      title: "Replay And Probe",
      detail: "Replay fixtures, negative probes, capability evidence, and persisted reports.",
      href: "/workflow-detail.html?id=sandbox.replay_probe_observability",
      meta: `${runs?.probeRuns?.length || 0} probes`,
    },
  );
  return cards;
}

function configuredWorkflowTargetCard(catalog = {}) {
  const target = catalog?.presentation?.workflowFinder || catalog?.presentation?.targetWorkflow || {};
  const targetStepCount = Number(target.targetStepCount || target.stepCount || 0);
  const targetInterfaceCount = Number(target.targetInterfaceCount || target.interfaceCount || 0);
  if (!Number.isFinite(targetStepCount) || targetStepCount <= 0 || !Number.isFinite(targetInterfaceCount) || targetInterfaceCount <= 0) {
    return null;
  }
  const label = String(target.targetLabel || target.label || "Configured workflow target").trim();
  const discovery = buildWorkflowDiscovery(catalog?.workflows || [], {
    targetStepCount,
    targetInterfaceCount,
    targetLabel: label,
  });
  const first = discovery.targetWorkflows[0];
  const detail = first
    ? `${first.title || first.id} · ${first.coverageLabel}`
    : `${targetStepCount} configured steps · ${targetInterfaceCount} configured interfaces`;
  return {
    kind: "workflow-target",
    title: label,
    detail,
    href: "/workflows.html",
    meta: `${discovery.summary.targetExact} matching workflow${discovery.summary.targetExact === 1 ? "" : "s"}`,
  };
}
