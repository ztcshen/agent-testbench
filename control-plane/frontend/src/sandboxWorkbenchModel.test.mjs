import assert from "node:assert/strict";
import { describe, it } from "node:test";

import { buildCapabilityCards } from "./sandboxWorkbenchModel.mjs";

describe("buildCapabilityCards", () => {
  it("adds a configured workflow target card from catalog presentation data", () => {
    const cards = buildCapabilityCards({
      runs: { workflowRuns: [{ id: "run-1", status: "passed" }], probeRuns: [] },
      caseRuns: { caseRuns: [{ runId: "case-run-1", status: "failed", failureKind: "assertion" }] },
      catalog: {
        services: [{ id: "service.alpha" }],
        presentation: {
          workflowFinder: {
            targetStepCount: 4,
            targetInterfaceCount: 3,
            targetLabel: "Configured workflow target",
          },
        },
        workflows: [
          {
            id: "workflow.match",
            displayName: "Target workflow",
            steps: [
              { id: "step.1", serviceId: "service.a" },
              { id: "step.2", serviceId: "service.b" },
              { id: "step.3", serviceId: "service.c" },
              { id: "step.4", serviceId: "service.c" },
            ],
          },
        ],
      },
    });

    const target = cards.find((card) => card.kind === "workflow-target");

    assert.deepEqual(target, {
      kind: "workflow-target",
      title: "Configured workflow target",
      detail: "Target workflow · 4 steps / 3 interfaces",
      href: "/workflows.html",
      meta: "1 matching workflow",
    });
  });

  it("omits the target card when workflow finder config is absent", () => {
    const cards = buildCapabilityCards({ catalog: { workflows: [], services: [] } });

    assert.equal(cards.some((card) => card.kind === "workflow-target"), false);
  });
});
