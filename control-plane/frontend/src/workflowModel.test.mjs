import assert from "node:assert/strict";
import { describe, it } from "node:test";

import { workflowRuntimeImpact } from "./workflowModel.js";

describe("workflow runtime impact", () => {
  it("gates workflows on interface availability instead of service runtime", () => {
    const workflow = {
      steps: [
        { id: "entry", serviceId: "retail-gateway", caseId: "case.entry", executable: true },
        { id: "downstream", serviceId: "account-channel", caseId: "case.downstream", executable: true },
        { id: "internal", serviceId: "ledger-core", caseId: "case.internal", executable: true },
      ],
    };
    const statusById = new Map([
      ["retail-gateway", { id: "retail-gateway", ok: true }],
      ["account-channel", { id: "account-channel", ok: true }],
      ["ledger-core", { id: "ledger-core", ok: false }],
    ]);

    assert.deepEqual(workflowRuntimeImpact(workflow, statusById), {
      text: "3/3 接口可用",
      tone: "ok",
    });
  });
});
