import assert from "node:assert/strict";
import { describe, it } from "node:test";

import { buildInterfaceNodeDirectoryContext, interfaceNodeDetailHref } from "./interfaceNodesModel.mjs";

describe("buildInterfaceNodeDirectoryContext", () => {
  it("extracts service, workflow, and case context from query parameters", () => {
    const context = buildInterfaceNodeDirectoryContext(new URLSearchParams("serviceId=service.alpha&workflow=workflow.alpha&case=case.alpha"));

    assert.deepEqual(context, {
      serviceId: "service.alpha",
      workflowId: "workflow.alpha",
      caseId: "case.alpha",
      workflowCaseSetHref: "/api-cases.html?workflow=workflow.alpha&case=case.alpha",
    });
  });

  it("keeps node detail links scoped to the workflow case context", () => {
    const context = buildInterfaceNodeDirectoryContext(new URLSearchParams("serviceId=service.alpha&workflow=workflow.alpha&case=case.alpha"));

    assert.equal(
      interfaceNodeDetailHref({ id: "node.alpha" }, context),
      "/interface-node.html?id=node.alpha&workflow=workflow.alpha&case=case.alpha",
    );
  });
});
