import assert from "node:assert/strict";
import { describe, it } from "node:test";

import {
  assertTerminalWorkflowBatchReport,
  exportedValues,
  parseEvidenceBody,
  storeNativeWorkflowBatchRequest,
  workflowBatchPollTimeoutMs,
  workflowBatchRunnerState,
  workflowStepResultOK,
} from "./workflowDetailModel.mjs";

describe("workflow detail context exports", () => {
  it("keeps long numeric response identifiers as strings", () => {
    const body = "{\"ext_params\":{\"payout_results\":[{\"payout_id\":9161018236071807}]}}";

    assert.deepEqual(parseEvidenceBody(body), {
      ext_params: {
        payout_results: [{ payout_id: "9161018236071807" }],
      },
    });
  });

  it("does not export scientific notation for long payout ids", () => {
    const values = exportedValues({
      exports: [{ from: "responseBody", name: "payout_id", path: "ext_params.payout_results.0.payout_id" }],
    }, {
      result: {
        response: {
          body: "{\"ext_params\":{\"payout_results\":[{\"payout_id\":9161018236071807}]}}",
        },
      },
    });

    assert.deepEqual(values, { payout_id: "9161018236071807" });
  });
});

describe("workflow detail Store-native batch execution", () => {
  it("builds a catalog-owned workflow request without trusted step fields", () => {
    const request = storeNativeWorkflowBatchRequest("workflow.alpha", "workflow-ui-001");

    assert.deepEqual(request, {
      requestId: "workflow-ui-001",
      workflowId: "workflow.alpha",
    });
    for (const field of ["baseUrl", "evidenceDir", "environmentId", "stepId", "caseIds", "overrides"]) {
      assert.equal(Object.hasOwn(request, field), false, field);
    }
  });

  it("projects only completed batch cases into runner progress", () => {
    const state = workflowBatchRunnerState({
      batchRunId: "batch.workflow-ui-001.20260717T000000Z",
      status: "running",
      total: 3,
      completed: 1,
      cases: [
        { caseId: "case.one", stepId: "step-one", status: "passed", elapsedMs: 12 },
        { caseId: "case.two", stepId: "step-two", status: "running", elapsedMs: 0 },
        { caseId: "case.three", stepId: "step-three", status: "running", elapsedMs: 0 },
      ],
    }, { startedAt: 1000, elapsedMs: 25 });

    assert.equal(state.status, "running");
    assert.equal(state.runId, "batch.workflow-ui-001.20260717T000000Z");
    assert.equal(state.message, "completed 1/3");
    assert.equal(state.startedAt, 1000);
    assert.equal(state.elapsedMs, 25);
    assert.deepEqual(state.steps.map((step) => step.stepId), ["step-one"]);
    assert.equal(workflowStepResultOK(state.steps[0]), true);
  });

  it("uses terminal batch status and case errors for the final state", () => {
    const report = {
      batchRunId: "batch.workflow-ui-002.20260717T000000Z",
      status: "failed",
      total: 2,
      completed: 2,
      passed: 1,
      failed: 1,
      cases: [
        { caseId: "case.one", stepId: "step-one", status: "passed", elapsedMs: 12 },
        { caseId: "case.two", stepId: "step-two", status: "failed", error: "response assertion failed", elapsedMs: 8 },
      ],
    };
    const state = workflowBatchRunnerState(assertTerminalWorkflowBatchReport(report), { startedAt: 1000, elapsedMs: 40 });

    assert.equal(state.status, "failed");
    assert.equal(state.message, "response assertion failed");
    assert.equal(state.startedAt, undefined);
    assert.equal(state.steps.length, 2);
    assert.equal(workflowStepResultOK(state.steps[0]), true);
    assert.equal(workflowStepResultOK(state.steps[1]), false);
    assert.equal(workflowStepResultOK({ status: "passed" }), true, "cached batch summary");
  });

  it("waits for the Store execution budget instead of the shorter catalog display budget", () => {
    assert.equal(workflowBatchPollTimeoutMs({
      total: 3,
      cases: [
        { timeoutSeconds: 90 },
        { timeoutSeconds: 90 },
        { timeoutSeconds: 90 },
      ],
    }, 9000), 375000);
    assert.equal(workflowBatchPollTimeoutMs({ total: 3, cases: [{}, {}, {}] }, 9000), 375000);
  });

  it("rejects a terminal batch report that still contains running work", () => {
    assert.throws(() => assertTerminalWorkflowBatchReport({
      status: "passed",
      total: 2,
      completed: 1,
      passed: 1,
      cases: [
        { stepId: "step-one", status: "passed" },
        { stepId: "step-two", status: "running" },
      ],
    }), /incomplete terminal report/);
  });

  it("rejects a passed batch report with failed case counts", () => {
    assert.throws(() => assertTerminalWorkflowBatchReport({
      status: "passed",
      total: 2,
      completed: 2,
      passed: 1,
      failed: 1,
      cases: [
        { stepId: "step-one", status: "passed" },
        { stepId: "step-two", status: "failed" },
      ],
    }), /inconsistent terminal report/);
  });
});
