import test from "node:test";
import assert from "node:assert/strict";
import {
  captureStepBlock,
  addStepBlock,
  updateStepGroup,
  upgradeStepGroup,
  sequenceUnits,
  validateStepGroups,
  inferBlockParameters,
} from "./stepBlockModel.mjs";
import {
  moveStudioStep,
  removeStudioStep,
  buildStudioSavePayload,
} from "../../control-plane/frontend/src/workflowStudioModel.mjs";

const cases = [
  { id: "case.create", variables: ["quantity"] },
  { id: "case.read", variables: ["itemId"] },
];
const source = {
  workflow: { id: "workflow.source", displayName: "Source" },
  steps: [
    {
      id: "prepare",
      caseId: "case.create",
      title: "Create",
      bindings: [],
      outputs: [],
    },
    {
      id: "read",
      caseId: "case.read",
      title: "Read",
      bindings: [
        {
          parameter: "itemId",
          sourceStepId: "prepare",
          from: "responsebody",
          path: "$.id",
        },
      ],
      outputs: [],
    },
  ],
};

test("captured blocks retain internal bindings and promote external dependencies", () => {
  const all = captureStepBlock(source, cases, 0, 1, "block.full");
  assert.equal(all.steps[1].bindings[0].sourceStepId, "step.1");
  assert.deepEqual(
    all.parameters.map((parameter) => parameter.name),
    ["quantity"],
  );
  const tail = captureStepBlock(source, cases, 1, 1, "block.tail");
  assert.deepEqual(tail.steps[0].bindings, []);
  assert.deepEqual(tail.parameters[0].targets, [
    { stepId: "step.1", parameter: "itemId" },
  ]);
});

test("repeated references namespace step identities and move or remove as a unit", () => {
  const definition = {
    ...captureStepBlock(source, cases, 0, 1, "block.full"),
    revision: "version-one",
  };
  let draft = addStepBlock({ ...source, steps: [] }, definition, "first");
  draft = addStepBlock(draft, definition, "second");
  assert.equal(new Set(draft.steps.map((step) => step.id)).size, 4);
  assert.equal(draft.steps[3].bindings[0].sourceStepId, "second.step.1");
  const moved = moveStudioStep(draft, "first.step.1", 2);
  assert.deepEqual(
    sequenceUnits(moved).map((unit) => unit.id),
    ["second", "first"],
  );
  assert.equal(removeStudioStep(draft, "first.step.2").steps.length, 2);
  const payload = buildStudioSavePayload(draft);
  assert.equal(payload.steps[0].group.revision, "version-one");
});

test("zero and false remain explicit values and invalid parameters block saving", () => {
  const definition = {
    ...captureStepBlock(source, cases, 0, 1, "block.full"),
    revision: "one",
  };
  definition.parameters[0].dataType = "integer";
  let draft = addStepBlock({ ...source, steps: [] }, definition, "first");
  assert(validateStepGroups(draft).some((error) => error.includes("quantity")));
  draft = updateStepGroup(draft, "first", { values: { quantity: 0 } });
  assert.deepEqual(validateStepGroups(draft), []);
  assert.equal(draft.steps[1].group.values.quantity, 0);
  const invalid = updateStepGroup(draft, "first", {
    values: { quantity: "0" },
  });
  assert(
    validateStepGroups(invalid).some((error) => error.includes("integer")),
  );
});

test("a deliberate revision upgrade retains matching input values", () => {
  const definition = {
    ...captureStepBlock(source, cases, 0, 1, "block.full"),
    revision: "one",
  };
  let draft = addStepBlock({ ...source, steps: [] }, definition, "first");
  draft = updateStepGroup(draft, "first", { values: { quantity: "7" } });
  const updated = upgradeStepGroup(draft, "first", {
    ...definition,
    revision: "two",
  });
  assert.equal(updated.steps[0].group.revision, "two");
  assert.equal(updated.steps[0].group.values.quantity, "7");
  assert.equal(draft.steps[0].group.revision, "one");
});

test("parameter aliases survive definition editing and removed targets disappear", () => {
  const parameters = [
    {
      name: "count",
      dataType: "integer",
      required: true,
      hasDefault: true,
      defaultValue: 0,
      targets: [{ stepId: "prepare", parameter: "quantity" }],
    },
  ];
  assert.equal(
    inferBlockParameters(source.steps, cases, parameters)[0].name,
    "count",
  );
  assert.deepEqual(
    inferBlockParameters(source.steps.slice(1), cases, parameters),
    [],
  );
});
