import assert from "node:assert/strict";
import { describe, it } from "node:test";

import { buildWorkflowDiscovery } from "./workflowDiscoveryModel.mjs";

describe("buildWorkflowDiscovery", () => {
  const targetWorkflow = {
    id: "workflow.target",
    displayName: "Configured target workflow",
    steps: Array.from({ length: 4 }, (_, index) => ({
      id: `step.${index + 1}`,
      serviceId: `service.${index + 1}`,
      caseId: `case.${index + 1}`,
    })),
  };
  const smallWorkflow = {
    id: "workflow.small",
    displayName: "Small workflow",
    steps: [
      { id: "step.a", serviceId: "service.a", caseId: "case.a" },
      { id: "step.b", serviceId: "service.a", caseId: "case.b" },
    ],
  };

  it("surfaces configured workflow targets with matching interface targets", () => {
    const discovery = buildWorkflowDiscovery([targetWorkflow, smallWorkflow], { targetStepCount: 4, targetInterfaceCount: 4 });

    assert.equal(discovery.targetWorkflows.length, 1);
    assert.equal(discovery.targetWorkflows[0].id, "workflow.target");
    assert.equal(discovery.targetWorkflows[0].stepCount, 4);
    assert.equal(discovery.targetWorkflows[0].interfaceCount, 4);
    assert.equal(discovery.targetWorkflows[0].caseCount, 4);
    assert.equal(discovery.summary.targetExact, 1);
  });

  it("filters workflow discovery by configured interface or case text", () => {
    const discovery = buildWorkflowDiscovery([targetWorkflow, smallWorkflow], { query: "service.4", targetStepCount: 4, targetInterfaceCount: 4 });

    assert.deepEqual(discovery.visibleWorkflows.map((item) => item.id), ["workflow.target"]);
  });

  it("builds an auditable target checklist for matching workflow steps", () => {
    const incompleteTarget = {
      id: "workflow.incomplete",
      displayName: "Incomplete target workflow",
      steps: [
        { id: "step.ready", serviceId: "service.ready", caseId: "case.ready" },
        { id: "step.missing-interface", caseId: "case.only" },
        { id: "step.missing-case", serviceId: "service.only" },
        { id: "step.ready.again", serviceId: "service.ready.again", caseId: "case.ready.again" },
      ],
    };

    const discovery = buildWorkflowDiscovery([incompleteTarget], { targetStepCount: 4, targetInterfaceCount: 3 });

    assert.equal(discovery.targetWorkflows.length, 1);
    assert.equal(discovery.targetChecklist.length, 1);
    assert.equal(discovery.targetChecklist[0].workflowId, "workflow.incomplete");
    assert.equal(discovery.targetChecklist[0].summary.ready, 2);
    assert.equal(discovery.targetChecklist[0].summary.missingInterface, 1);
    assert.equal(discovery.targetChecklist[0].summary.missingCase, 1);
    assert.deepEqual(discovery.targetChecklist[0].rows.map((row) => row.status), ["ready", "missing-interface", "missing-case", "ready"]);
    assert.equal(discovery.targetChecklist[0].rows[0].stepHref, "/workflow-step.html?workflow=workflow.incomplete&step=step.ready");
    assert.equal(discovery.targetChecklist[0].rows[0].interfaceHref, "/interface-node.html?id=service.ready");
    assert.equal(discovery.targetChecklist[0].rows[0].caseHref, "/api-cases.html?workflow=workflow.incomplete&case=case.ready");
    assert.equal(discovery.targetChecklist[0].rows[0].runsHref, "/case-runs.html?case=case.ready");
  });
});
