import assert from "node:assert/strict";
import { describe, it } from "node:test";

import { buildRunAnalysis } from "./caseRunsModel.mjs";

describe("buildRunAnalysis", () => {
  const runs = [
    {
      runId: "run-1",
      caseId: "case.create",
      operation: "Create item",
      status: "failed",
      failureCategory: "Product errors",
      failureKind: "assertion-mismatch",
      failureReason: "status code 500 was not expected",
      durationMs: 1250,
      updatedAt: "2026-05-18T01:00:00Z",
    },
    {
      runId: "run-2",
      caseId: "case.search",
      operation: "Search item",
      status: "passed",
      durationMs: 320,
      updatedAt: "2026-05-18T01:01:00Z",
    },
    {
      runId: "run-3",
      caseId: "case.timeout",
      operation: "Timeout path",
      status: "failed",
      failureCategory: "timeout",
      failureKind: "timeout",
      failureReason: "context deadline exceeded",
      durationMs: 5000,
      updatedAt: "2026-05-18T01:02:00Z",
    },
  ];

  it("groups runs by status and failure category for triage facets", () => {
    const analysis = buildRunAnalysis(runs);

    assert.deepEqual(analysis.statusFacets, [
      { key: "failed", label: "failed", count: 2, field: "status" },
      { key: "passed", label: "passed", count: 1, field: "status" },
    ]);
    assert.deepEqual(analysis.failureCategoryFacets, [
      { key: "Product errors", label: "Product errors", count: 1, field: "failureCategory" },
      { key: "timeout", label: "timeout", count: 1, field: "failureCategory" },
    ]);
    assert.equal(analysis.summary.total, 3);
    assert.equal(analysis.summary.failed, 2);
    assert.equal(analysis.summary.passed, 1);
    assert.equal(analysis.slowest[0].runId, "run-3");
  });

  it("filters by text, status, and failure category", () => {
    const analysis = buildRunAnalysis(runs, {
      query: "create",
      status: "failed",
      failureCategory: "Product errors",
    });

    assert.equal(analysis.visibleRuns.length, 1);
    assert.equal(analysis.visibleRuns[0].runId, "run-1");
    assert.equal(analysis.activeFilters.query, "create");
    assert.equal(analysis.activeFilters.status, "failed");
    assert.equal(analysis.activeFilters.failureCategory, "Product errors");
  });

  it("focuses the report grid on an exact case id when linked from case management", () => {
    const analysis = buildRunAnalysis([
      ...runs,
      {
        runId: "run-4",
        caseId: "case.create",
        operation: "Create item",
        status: "passed",
        durationMs: 900,
        updatedAt: "2026-05-18T01:03:00Z",
      },
    ], { caseId: "case.create" });

    assert.deepEqual(analysis.visibleRuns.map((run) => run.runId), ["run-1", "run-4"]);
    assert.deepEqual(analysis.grid.rows.map((row) => row.id), ["run-4", "run-1"]);
    assert.equal(analysis.activeFilters.caseId, "case.create");
    assert.equal(analysis.summary.total, 4);
    assert.equal(analysis.summary.visible, 2);
    assert.deepEqual(analysis.caseFocus, {
      caseId: "case.create",
      total: 2,
      passed: 1,
      failed: 1,
      latestRunId: "run-4",
      latestStatus: "passed",
      latestUpdatedAt: "2026-05-18T01:03:00Z",
      latestEvidenceHref: "/evidence-viewer.html?caseRun=run-4&caseId=case.create",
      longestDurationMs: 1250,
    });
  });

  it("preserves workflow context for case-run links opened from workflow case sequence", () => {
    const analysis = buildRunAnalysis(runs, {
      workflowId: "workflow.checkout",
      caseId: "case.create",
    });

    assert.equal(analysis.activeFilters.workflowId, "workflow.checkout");
    assert.equal(analysis.workflowContext.workflowId, "workflow.checkout");
    assert.equal(analysis.workflowContext.caseId, "case.create");
    assert.equal(analysis.workflowContext.caseSetHref, "/api-cases.html?workflow=workflow.checkout&case=case.create");
    assert.equal(analysis.grid.rows[0].evidenceHref, "/evidence-viewer.html?caseRun=run-1&caseId=case.create&workflow=workflow.checkout");
    assert.equal(analysis.caseFocus.latestEvidenceHref, "/evidence-viewer.html?caseRun=run-1&caseId=case.create&workflow=workflow.checkout");
  });

  it("builds a dense report grid with stable columns and sortable rows", () => {
    const analysis = buildRunAnalysis(runs, { sort: "duration_desc" });

    assert.deepEqual(analysis.grid.columns.map((column) => column.id), [
      "status",
      "case",
      "operation",
      "failureCategory",
      "duration",
      "updated",
      "evidence",
    ]);
    assert.deepEqual(analysis.grid.rows.map((row) => row.id), ["run-3", "run-1", "run-2"]);
    assert.equal(analysis.grid.rows[0].durationMs, 5000);
    assert.equal(analysis.grid.rows[0].durationRank, 1);
    assert.equal(analysis.grid.rows[0].evidenceHref, "/evidence-viewer.html?caseRun=run-3&caseId=case.timeout");
  });

  it("summarizes failed rows by failure group for report navigation", () => {
    const analysis = buildRunAnalysis(runs);

    assert.deepEqual(analysis.failureGroups, [
      {
        key: "Product errors",
        label: "Product errors",
        count: 1,
        longestRunId: "run-1",
        longestDurationMs: 1250,
      },
      {
        key: "timeout",
        label: "timeout",
        count: 1,
        longestRunId: "run-3",
        longestDurationMs: 5000,
      },
    ]);
  });

  it("builds first-match failure triage buckets with evidence handoffs", () => {
    const analysis = buildRunAnalysis([
      {
        runId: "run-assertion",
        caseId: "case.assertion",
        status: "failed",
        failureCategory: "assertion-mismatch",
        failureReason: "status code 500 was not expected",
        durationMs: 1800,
        updatedAt: "2026-05-18T01:05:00Z",
      },
      {
        runId: "run-transport",
        caseId: "case.transport",
        status: "failed",
        failureCategory: "transport-error",
        failureReason: "send request: connection refused",
        durationMs: 700,
        updatedAt: "2026-05-18T01:06:00Z",
      },
    ], {
      workflowId: "workflow.checkout",
      failureCategoryRules: [
        {
          name: "Product errors",
          matchers: {
            statuses: ["failed"],
            failureCategories: ["assertion-mismatch"],
            messageContains: ["not expected"],
          },
        },
        {
          name: "Catch-all failed",
          matchers: {
            statuses: ["failed"],
          },
        },
      ],
    });

    assert.deepEqual(analysis.failureTriage.map((group) => ({
      key: group.key,
      label: group.label,
      count: group.count,
      matchedBy: group.matchedBy,
      sampleEvidenceHref: group.sampleEvidenceHref,
      sampleReason: group.sampleReason,
    })), [
      {
        key: "Product errors",
        label: "Product errors",
        count: 1,
        matchedBy: "rule 1",
        sampleEvidenceHref: "/evidence-viewer.html?caseRun=run-assertion&caseId=case.assertion&workflow=workflow.checkout",
        sampleReason: "status code 500 was not expected",
      },
      {
        key: "Catch-all failed",
        label: "Catch-all failed",
        count: 1,
        matchedBy: "rule 2",
        sampleEvidenceHref: "/evidence-viewer.html?caseRun=run-transport&caseId=case.transport&workflow=workflow.checkout",
        sampleReason: "send request: connection refused",
      },
    ]);
  });

  it("detects flaky candidates from mixed pass and fail case history", () => {
    const analysis = buildRunAnalysis([
      {
        runId: "run-a1",
        caseId: "case.checkout",
        operation: "Checkout",
        status: "failed",
        failureReason: "assertion mismatch",
        durationMs: 1000,
        updatedAt: "2026-05-18T01:00:00Z",
      },
      {
        runId: "run-a2",
        caseId: "case.checkout",
        operation: "Checkout",
        status: "passed",
        durationMs: 800,
        updatedAt: "2026-05-18T01:10:00Z",
      },
      {
        runId: "run-a3",
        caseId: "case.checkout",
        operation: "Checkout",
        status: "failed",
        failureReason: "timeout",
        durationMs: 1600,
        updatedAt: "2026-05-18T01:20:00Z",
      },
      {
        runId: "run-b1",
        caseId: "case.search",
        operation: "Search",
        status: "passed",
        durationMs: 300,
        updatedAt: "2026-05-18T01:05:00Z",
      },
    ], { workflowId: "workflow.checkout" });

    assert.deepEqual(analysis.flakyCandidates, [
      {
        caseId: "case.checkout",
        operation: "Checkout",
        total: 3,
        passed: 1,
        failed: 2,
        latestStatus: "failed",
        latestRunId: "run-a3",
        latestEvidenceHref: "/evidence-viewer.html?caseRun=run-a3&caseId=case.checkout&workflow=workflow.checkout",
        caseRunsHref: "/case-runs.html?case=case.checkout&workflow=workflow.checkout",
        failureReasons: ["assertion mismatch", "timeout"],
        flakeScore: 67,
      },
    ]);
  });
});
