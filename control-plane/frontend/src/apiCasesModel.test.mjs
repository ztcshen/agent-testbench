import assert from "node:assert/strict";
import { describe, it } from "node:test";

import { buildCaseCoverageBoard, buildCaseManagement, buildWorkflowCaseContext } from "./apiCasesModel.mjs";

describe("buildCaseManagement", () => {
  const cases = [
    {
      id: "case.create",
      title: "Create item",
      operation: "POST /items",
      status: "active",
      owner: "team-a",
      priority: "p0",
      tags: ["smoke", "regression"],
      casePath: "cases/create.json",
      sourceKind: "karate",
      executorId: "executor.karate",
      runCount: 4,
      latestRun: { status: "failed", runId: "run-1", elapsedMs: 1300, failureReason: "assertion mismatch" },
    },
    {
      id: "case.search",
      title: "Search item",
      operation: "GET /items",
      status: "review",
      owner: "team-b",
      priority: "p1",
      tags: ["regression"],
      runCount: 0,
    },
    {
      id: "case.cleanup",
      title: "Cleanup item",
      operation: "DELETE /items",
      status: "active",
      owner: "team-a",
      priority: "p2",
      tags: ["cleanup"],
      casePath: "cases/cleanup.json",
      sourceKind: "http",
      executorId: "executor.http",
      runCount: 1,
      latestRun: { status: "passed", runId: "run-2", elapsedMs: 450 },
    },
  ];

  it("builds management facets and readiness metrics for case search", () => {
    const management = buildCaseManagement(cases);

    assert.deepEqual(management.summary, {
      total: 3,
      visible: 3,
      ready: 2,
      needsReview: 1,
      failedLatest: 1,
      neverRun: 1,
    });
    assert.deepEqual(management.facets.owner.map(({ key, count }) => ({ key, count })), [
      { key: "team-a", count: 2 },
      { key: "team-b", count: 1 },
    ]);
    assert.deepEqual(management.facets.priority.map(({ key, count }) => ({ key, count })), [
      { key: "p0", count: 1 },
      { key: "p1", count: 1 },
      { key: "p2", count: 1 },
    ]);
  });

  it("filters cases by text, lifecycle, owner, priority, tag, and latest run state", () => {
    const management = buildCaseManagement(cases, {
      query: "create",
      status: "active",
      owner: "team-a",
      priority: "p0",
      tag: "smoke",
      runState: "failed",
    });

    assert.deepEqual(management.rows.map((row) => row.id), ["case.create"]);
    assert.equal(management.rows[0].readiness, "ready");
    assert.equal(management.rows[0].latestEvidenceHref, "/evidence-viewer.html?caseRun=run-1&caseId=case.create");
  });

  it("sorts management rows by readiness, priority, and latest result", () => {
    const management = buildCaseManagement(cases, { sort: "priority_desc" });

    assert.deepEqual(management.rows.map((row) => row.id), ["case.create", "case.search", "case.cleanup"]);
    assert.deepEqual(management.readinessGroups.map(({ key, count }) => ({ key, count })), [
      { key: "ready", count: 2 },
      { key: "needs-review", count: 1 },
    ]);
  });

  it("focuses case management on the cases mapped by a workflow catalog", () => {
    const catalog = {
      workflows: [
        {
          id: "workflow.alpha",
          displayName: "Workflow Alpha",
          steps: [
            { id: "step.create", serviceId: "service.create", caseId: "case.create" },
            { id: "step.cleanup", serviceId: "service.cleanup", caseId: "case.cleanup" },
            { id: "step.missing", serviceId: "service.missing" },
          ],
        },
      ],
    };

    const context = buildWorkflowCaseContext(catalog, "workflow.alpha");
    const management = buildCaseManagement(cases, { caseIds: context.caseIds, caseIdsFilterEnabled: context.enabled });

    assert.equal(context.enabled, true);
    assert.equal(context.title, "Workflow Alpha");
    assert.deepEqual(context.caseIds, ["case.create", "case.cleanup"]);
    assert.deepEqual(context.interfaceIds, ["service.create", "service.cleanup", "service.missing"]);
    assert.equal(context.summary.steps, 3);
    assert.equal(context.summary.cases, 2);
    assert.deepEqual(management.rows.map((row) => row.id), ["case.create", "case.cleanup"]);
    assert.equal(management.summary.total, 3);
    assert.equal(management.summary.visible, 2);
  });

  it("keeps an empty workflow case set empty instead of falling back to all cases", () => {
    const context = buildWorkflowCaseContext({
      workflows: [{ id: "workflow.empty", displayName: "Empty Workflow", steps: [{ id: "step.only", serviceId: "service.only" }] }],
    }, "workflow.empty");
    const management = buildCaseManagement(cases, { caseIds: context.caseIds, caseIdsFilterEnabled: context.enabled });

    assert.equal(context.enabled, true);
    assert.deepEqual(context.caseIds, []);
    assert.deepEqual(management.rows, []);
    assert.equal(management.summary.total, 3);
    assert.equal(management.summary.visible, 0);
  });

  it("enriches workflow steps with case readiness and latest result for execution tracking", () => {
    const catalog = {
      workflows: [
        {
          id: "workflow.alpha",
          displayName: "Workflow Alpha",
          steps: [
            { id: "step.create", serviceId: "service.create", caseId: "case.create" },
            { id: "step.cleanup", serviceId: "service.cleanup", caseId: "case.cleanup" },
            { id: "step.unmapped", serviceId: "service.only" },
          ],
        },
      ],
    };

    const context = buildWorkflowCaseContext(catalog, "workflow.alpha", cases);

    assert.deepEqual(context.steps.map((step) => ({
      id: step.id,
      caseTitle: step.caseTitle,
      readiness: step.readiness,
      latestStatus: step.latestStatus,
      state: step.state,
      interfaceHref: step.interfaceHref,
    })), [
      { id: "step.create", caseTitle: "Create item", readiness: "ready", latestStatus: "failed", state: "latest-failed", interfaceHref: "/interface-nodes.html?serviceId=service.create&workflow=workflow.alpha&case=case.create" },
      { id: "step.cleanup", caseTitle: "Cleanup item", readiness: "ready", latestStatus: "passed", state: "ready", interfaceHref: "/interface-nodes.html?serviceId=service.cleanup&workflow=workflow.alpha&case=case.cleanup" },
      { id: "step.unmapped", caseTitle: "", readiness: "missing", latestStatus: "not-run", state: "no-case", interfaceHref: "/interface-nodes.html?serviceId=service.only&workflow=workflow.alpha" },
    ]);
    assert.equal(context.summary.latestFailed, 1);
    assert.equal(context.summary.sequenceIssues, 2);
    assert.equal(context.steps[0].latestEvidenceHref, "/evidence-viewer.html?caseRun=run-1&caseId=case.create");
  });
});

describe("buildCaseCoverageBoard", () => {
  const coverage = {
    counts: { total: 4, passed: 1, failed: 1, notRun: 2 },
    items: [
      { caseId: "case.create", title: "Create item", nodeId: "node.items", nodeName: "Items API", latestStatus: "passed", latestRunId: "run-1", caseRunId: "case-run-1", elapsedMs: 480, hasPassed: true },
      { caseId: "case.update", title: "Update item", nodeId: "node.items", nodeName: "Items API", latestStatus: "failed", latestRunId: "run-2", caseRunId: "case-run-2", reason: "assertion mismatch", elapsedMs: 980 },
      { caseId: "case.audit", title: "Audit item", nodeId: "node.audit", nodeName: "Audit API", latestStatus: "not-run", reason: "no run recorded in Store" },
      { caseId: "case.archive", title: "Archive item", nodeId: "node.archive", nodeName: "Archive API", latestStatus: "not-run" },
    ],
  };

  it("builds an interface coverage matrix with gaps and evidence handoffs", () => {
    const board = buildCaseCoverageBoard(coverage);

    assert.deepEqual(board.summary, {
      total: 4,
      passed: 1,
      failed: 1,
      notRun: 2,
      covered: 1,
      gaps: 3,
      passRate: 25,
    });
    assert.deepEqual(board.groups.map((group) => ({
      nodeId: group.nodeId,
      nodeName: group.nodeName,
      total: group.total,
      passed: group.passed,
      failed: group.failed,
      notRun: group.notRun,
      gapCount: group.gapCount,
    })), [
      { nodeId: "node.items", nodeName: "Items API", total: 2, passed: 1, failed: 1, notRun: 0, gapCount: 1 },
      { nodeId: "node.archive", nodeName: "Archive API", total: 1, passed: 0, failed: 0, notRun: 1, gapCount: 1 },
      { nodeId: "node.audit", nodeName: "Audit API", total: 1, passed: 0, failed: 0, notRun: 1, gapCount: 1 },
    ]);
    assert.equal(board.rows[1].caseRunsHref, "/case-runs.html?case=case.update");
    assert.equal(board.rows[1].latestEvidenceHref, "/evidence-viewer.html?caseRun=run-2&caseId=case.update");
  });

  it("can scope coverage to a workflow case set while preserving workflow links", () => {
    const board = buildCaseCoverageBoard(coverage, {
      workflowId: "workflow.alpha",
      caseIds: ["case.create", "case.audit"],
    });

    assert.deepEqual(board.rows.map((row) => row.caseId), ["case.create", "case.audit"]);
    assert.deepEqual(board.summary, {
      total: 2,
      passed: 1,
      failed: 0,
      notRun: 1,
      covered: 1,
      gaps: 1,
      passRate: 50,
    });
    assert.equal(board.rows[0].caseRunsHref, "/case-runs.html?case=case.create&workflow=workflow.alpha");
    assert.equal(board.rows[0].latestEvidenceHref, "/evidence-viewer.html?caseRun=run-1&caseId=case.create&workflow=workflow.alpha");
  });
});
