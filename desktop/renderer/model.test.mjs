import test from "node:test";
import assert from "node:assert/strict";
import {
  requestDraft,
  compileRequest,
  reconcileDrafts,
  filterRequests,
} from "./model.mjs";

const record = {
  id: "case.one",
  nodeId: "node.one",
  serviceId: "service.http",
  displayName: "Read item",
  baseUrl: "https://api.example.com/v1",
  execution: {
    method: "GET",
    path: "/items/%7B%7Boverride:id%7D%7D",
    query: { limit: 3, ids: ["one", "two"] },
    headers: { Authorization: "[REDACTED]" },
    body: { count: 1 },
    expectedHttpCodes: [200],
  },
};

test("request edits retain typed native values and literal template parameters", () => {
  const draft = requestDraft(record);
  draft.url += "?search=a%26b";
  const compiled = compileRequest(draft);
  assert.equal(compiled.baseUrl, "https://api.example.com");
  assert.equal(compiled.execution.path, "/v1/items/{{override:id}}");
  assert.deepEqual(compiled.execution.query, {
    search: "a&b",
    limit: 3,
    ids: ["one", "two"],
  });
  assert.deepEqual(compiled.execution.body, { count: 1 });
  assert.equal(compiled.execution.headers.Authorization, "[REDACTED]");
  assert.throws(() => compileRequest({ ...draft, bodyText: "{bad" }), /JSON/);
  assert.throws(
    () => compileRequest({ ...draft, url: "file:///tmp/a" }),
    /地址/,
  );
  assert.throws(() => compileRequest({ ...draft, codesText: "999" }), /状态码/);
});

test("catalog refresh preserves drafts and exposes concurrent changes through the old ETag", () => {
  const first = reconcileDrafts({}, [record], "revision-1");
  first[record.id].draft.displayName = "Unsaved";
  first[record.id].draft.dirty = true;
  const unrelated = reconcileDrafts(first, [record], "revision-2");
  assert.equal(unrelated[record.id].etag, "revision-2");
  const conflict = reconcileDrafts(
    unrelated,
    [{ ...record, displayName: "Changed elsewhere" }],
    "revision-3",
  );
  assert.equal(conflict[record.id].etag, "revision-2");
  assert.equal(conflict[record.id].draft.displayName, "Unsaved");
  assert.equal(filterRequests([record], "GET items").length, 0);
  assert.equal(filterRequests([record], "items").length, 1);
});
