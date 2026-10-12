import test from "node:test";
import assert from "node:assert/strict";
import { addStudioStep, buildStudioSavePayload, emptyStudioDraft, moveStudioStep, removeStudioStep, setStudioBinding, studioCanvas, studioSourceSteps, validateStudioDraft } from "./workflowStudioModel.mjs";
import { saveStudio } from "./workflowStudioApi.mjs";

const cases = [{ id: "case.create", displayName: "Create", variables: [] }, { id: "case.read", displayName: "Read", variables: ["itemId"] }];
function flow() {
  let draft = addStudioStep(emptyStudioDraft("workflow.visual"), cases[0], "create");
  draft = addStudioStep(draft, cases[1], "read");
  return setStudioBinding(draft, "read", "itemId", { sourceStepId: "create", from: "responsebody", path: "$.id" });
}

test("reordering or deleting a producer keeps its dependency visible and blocks saving", () => {
  const draft = flow();
  assert.deepEqual(validateStudioDraft(draft, cases), []);
  assert.equal(studioSourceSteps(draft, "read")[0].id, "create");
  assert.match(validateStudioDraft(moveStudioStep(draft, "create", 1), cases).join(" "), /前序步骤/);
  const removed = removeStudioStep(draft, "create");
  assert.equal(removed.steps[0].bindings[0].sourceStepId, "create");
  assert.match(validateStudioDraft(removed, cases).join(" "), /前序步骤/);
});

test("save uses declared scope and graph distinguishes execution from data", () => {
  const draft = flow();
  draft.steps[1].request = { headers: { Authorization: "not-a-save-field" } };
  const payload = buildStudioSavePayload(draft);
  assert.equal(payload.steps[1].request, undefined);
  assert.equal(payload.steps[1].bindings[0].parameter, "itemId");
  const graph = studioCanvas(draft, cases);
  assert.equal(graph.edges.length, 2);
  assert.equal(graph.edges[1].sourceHandle, "output");
  assert.equal(graph.edges[0].sourceHandle, "next");
});

test("revision conflict propagates without retrying or mutating a draft", async () => {
  const draft = flow();
  const before = JSON.stringify(draft);
  let count = 0;
  await assert.rejects(saveStudio(draft, '"case-catalog-r1"', async (_path, options) => {
    count++;
    assert.equal(options.headers["If-Match"], '"case-catalog-r1"');
    return new Response(JSON.stringify({ error: "changed", currentRevision: 2 }), { status: 412 });
  }), error => error.status === 412 && error.payload.currentRevision === 2);
  assert.equal(count, 1);
  assert.equal(JSON.stringify(draft), before);
});
