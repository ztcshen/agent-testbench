import assert from "node:assert/strict";
import { describe, it } from "node:test";

import {
  buildCaseCatalogPatch,
  buildCaseEditorDraft,
  buildNewCaseEditorDraft,
  buildStoreCaseRunPayload,
  fetchCaseCatalog,
  normalizeStoreCaseRunResult,
  patchCaseCatalog,
  rollbackCaseCatalog,
  runStoreCase,
} from "./apiCasesApi.mjs";

function jsonResponse(body, { status = 200, headers = {} } = {}) {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "content-type": "application/json", ...headers },
  });
}

describe("Store-native case execution", () => {
  it("uses only the case id as the execution contract", () => {
    const payload = buildStoreCaseRunPayload({
      id: " case.alpha ",
      timeoutSeconds: 45,
      casePath: "private/cases/alpha.json",
      baseUrl: "https://example.invalid",
      evidenceDir: ".runtime/private",
      defaultOverrides: { token: "must-not-leave-the-browser" },
    });

    assert.deepEqual(payload, { caseId: "case.alpha" });
    assert.equal("timeoutSeconds" in payload, false);
    assert.equal("casePath" in payload, false);
    assert.equal("baseUrl" in payload, false);
    assert.equal("overrides" in payload, false);
  });

  it("posts the selected case to the Store-native endpoint and preserves failed run reports", async () => {
    let request;
    const result = await runStoreCase({ id: "case.alpha" }, async (path, options) => {
      request = { path, options };
      return jsonResponse({ ok: false, report: { status: "fail", run_id: "run-1" } });
    });

    assert.equal(request.path, "/api/test-kit/run");
    assert.deepEqual(JSON.parse(request.options.body), { caseId: "case.alpha" });
    assert.equal(result.ok, false);
    assert.equal(result.report.run_id, "run-1");
  });

  it("normalizes test-kit top-level run fields for the result component", () => {
    const result = normalizeStoreCaseRunResult({
      ok: true,
      status: "passed",
      runId: "run-store-1",
      caseId: "case.alpha",
      summary: { httpCode: 201 },
      result: { request: { requestId: "request-1" } },
    });

    assert.equal(result.report.status, "passed");
    assert.equal(result.report.run_id, "run-store-1");
    assert.equal(result.report.actual_http_code, 201);
    assert.equal(result.report.request_id, "request-1");
  });
});

describe("case catalog maintenance API", () => {
  it("starts new UI-maintained cases as editable drafts", () => {
    assert.deepEqual(buildNewCaseEditorDraft(), {
      id: "",
      displayName: "",
      description: "",
      nodeId: "",
      caseType: "",
      scenario: "",
      tagsText: "",
      priority: "",
      owner: "",
      requestTemplateId: "",
      renderMode: "",
      requiredForAdmission: false,
      status: "draft",
      sortOrder: 0,
    });
  });

  it("posts a new case id with the draft lifecycle through the guarded patch endpoint", async () => {
    let request;
    const draft = buildNewCaseEditorDraft();
    draft.id = "case.new";
    draft.nodeId = "node.alpha";
    await patchCaseCatalog(draft, '"case-catalog-r3-deadbeef"', async (path, options) => {
      request = { path, options };
      return jsonResponse(
        { ok: true, revision: 4, created: true, case: { id: "case.new", status: "draft" } },
        { headers: { etag: '"case-catalog-r4-feedface"' } },
      );
    });

    const payload = JSON.parse(request.options.body);
    assert.equal(request.path, "/api/case-catalog");
    assert.equal(payload.case.id, "case.new");
    assert.equal(payload.case.status, "draft");
    assert.equal(request.options.headers["If-Match"], '"case-catalog-r3-deadbeef"');
  });

  it("captures the strong ETag returned with the safe catalog projection", async () => {
    const result = await fetchCaseCatalog(async () => jsonResponse(
      { ok: true, revision: 3, cases: [{ id: "case.alpha" }] },
      { headers: { etag: '"case-catalog-r3-deadbeef"' } },
    ));

    assert.equal(result.etag, '"case-catalog-r3-deadbeef"');
    assert.equal(result.payload.revision, 3);
  });

  it("whitelists editable metadata and does not send source paths or secret-shaped values", async () => {
    const draft = buildCaseEditorDraft({
      id: "case.alpha",
      displayName: "Alpha",
      tags: ["smoke", "smoke"],
      status: "review",
      casePath: "private/alpha.json",
      baseUrl: "https://example.invalid",
      token: "secret",
    });
    draft.tagsText = " smoke, regression, smoke ";
    draft.password = "secret";
    draft.casePath = "private/changed.json";
    const patch = buildCaseCatalogPatch(draft);

    assert.deepEqual(patch.tags, ["smoke", "regression"]);
    assert.equal(patch.status, "review");
    assert.equal("token" in patch, false);
    assert.equal("password" in patch, false);
    assert.equal("casePath" in patch, false);
    assert.equal("baseUrl" in patch, false);
  });

  it("sends If-Match on patch and rollback mutations", async () => {
    const requests = [];
    const fakeFetch = async (path, options) => {
      requests.push({ path, options });
      return jsonResponse(
        { ok: true, revision: requests.length + 3 },
        { headers: { etag: `"case-catalog-r${requests.length + 3}-feedface"` } },
      );
    };
    const etag = '"case-catalog-r3-deadbeef"';

    await patchCaseCatalog({ id: "case.alpha", status: "review" }, etag, fakeFetch);
    await rollbackCaseCatalog(1, etag, fakeFetch);

    assert.equal(requests[0].path, "/api/case-catalog");
    assert.equal(requests[0].options.headers["If-Match"], etag);
    assert.equal(requests[1].path, "/api/case-catalog/rollback");
    assert.equal(requests[1].options.headers["If-Match"], etag);
    assert.deepEqual(JSON.parse(requests[1].options.body), { revision: 1 });
  });

  it("surfaces a 412 conflict without retrying or replacing the caller draft", async () => {
    const error = await patchCaseCatalog(
      { id: "case.alpha", displayName: "Local draft" },
      '"case-catalog-r3-deadbeef"',
      async () => jsonResponse(
        { ok: false, code: "catalog_revision_conflict", currentRevision: 4, error: "profile catalog revision changed" },
        { status: 412 },
      ),
    ).then(() => null, (caught) => caught);

    assert.equal(error.status, 412);
    assert.equal(error.payload.currentRevision, 4);
    assert.equal(error.message, "profile catalog revision changed");
  });

  it("renders structured maintenance validation issues as readable API errors", async () => {
    const error = await patchCaseCatalog(
      { id: "case.alpha", status: "active" },
      '"case-catalog-r3-deadbeef"',
      async () => jsonResponse(
        { ok: false, error: "case maintenance validation failed", issues: [{ code: "active-case-not-runnable", message: "active case needs a runnable source" }] },
        { status: 400 },
      ),
    ).then(() => null, (caught) => caught);

    assert.equal(error.message, "case maintenance validation failed: active case needs a runnable source");
  });
});
