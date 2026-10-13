import { buildStudioSavePayload } from "../../control-plane/frontend/src/workflowStudioModel.mjs";

const clone = (value) => structuredClone(value);
export const parameterTypes = [
  "string",
  "number",
  "integer",
  "boolean",
  "object",
  "array",
];

export function parseParameterValue(type, text) {
  if (type === "string") return text;
  if (!text.trim()) return text;
  try {
    return JSON.parse(text);
  } catch {
    return text;
  }
}

export function parameterValueValid(parameter, value) {
  if (value === null) return Boolean(parameter.nullable);
  switch (parameter.dataType) {
    case "string":
      return typeof value === "string";
    case "boolean":
      return typeof value === "boolean";
    case "number":
      return typeof value === "number" && Number.isFinite(value);
    case "integer":
      return Number.isInteger(value);
    case "object":
      return (
        Boolean(value) && typeof value === "object" && !Array.isArray(value)
      );
    case "array":
      return Array.isArray(value);
    default:
      return false;
  }
}

export function newStepBlock(id = `block.${crypto.randomUUID()}`) {
  return {
    id,
    displayName: "新建步骤组",
    description: "",
    revision: "",
    steps: [],
    parameters: [],
    dirty: true,
  };
}

export function captureStepBlock(
  draft,
  cases,
  start = 0,
  end = draft.steps.length - 1,
  id,
) {
  const selected = draft.steps.slice(start, end + 1);
  if (!selected.length) throw new Error("请选择至少一个步骤");
  if (selected.some((step) => step.group))
    throw new Error("选择独立步骤创建步骤组，避免嵌套引用");
  const ids = new Map(selected.map((step, i) => [step.id, `step.${i + 1}`]));
  const parameters = new Map();
  const steps = selected.map((step) => {
    const copy = buildStudioSavePayload({
      workflow: draft.workflow,
      steps: [step],
    }).steps[0];
    copy.id = ids.get(step.id);
    copy.position = { x: 80, y: (Number(copy.id.split(".").at(-1)) - 1) * 190 };
    copy.bindings = copy.bindings
      .filter((binding) => ids.has(binding.sourceStepId))
      .map((binding) => ({
        ...binding,
        sourceStepId: ids.get(binding.sourceStepId),
      }));
    const caseDef = cases.find((item) => item.id === step.caseId);
    for (const name of step.variables || caseDef?.variables || []) {
      if (copy.bindings.some((binding) => binding.parameter === name)) continue;
      const parameter = parameters.get(name) || {
        name,
        dataType: "string",
        required: true,
        nullable: false,
        hasDefault: false,
        defaultValue: null,
        targets: [],
      };
      parameter.targets.push({ stepId: copy.id, parameter: name });
      if (Object.hasOwn(step.values || {}, name)) {
        parameter.hasDefault = true;
        parameter.defaultValue = step.values[name];
        parameter.dataType = Array.isArray(parameter.defaultValue)
          ? "array"
          : parameter.defaultValue === null
            ? "string"
            : typeof parameter.defaultValue;
        parameter.nullable = parameter.defaultValue === null;
      }
      parameters.set(name, parameter);
    }
    delete copy.values;
    return copy;
  });
  return {
    ...newStepBlock(id),
    displayName: `${draft.workflow.displayName} · 步骤组`,
    steps,
    parameters: [...parameters.values()],
  };
}

export function addStepBlock(
  draft,
  definition,
  instanceId = `group.${crypto.randomUUID()}`,
  previous = null,
) {
  const group = {
    id: instanceId,
    blockId: definition.id,
    revision: definition.revision,
    title: previous?.title || definition.displayName,
    values: Object.fromEntries(
      Object.entries(previous?.values || {}).filter(([key]) =>
        definition.parameters.some((p) => p.name === key),
      ),
    ),
    bindings: (previous?.bindings || []).filter((b) =>
      definition.parameters.some((p) => p.name === b.parameter),
    ),
    parameters: clone(definition.parameters),
  };
  const steps = definition.steps.map((original, index) => ({
    ...clone(original),
    id: `${instanceId}.${original.id}`,
    group: clone(group),
    position: { x: 80, y: (draft.steps.length + index) * 190 },
    bindings: (original.bindings || []).map((binding) => ({
      ...binding,
      sourceStepId: `${instanceId}.${binding.sourceStepId}`,
    })),
    outputs: (original.outputs || []).map((output) => ({
      ...output,
      name: `${instanceId}.${output.name}`,
    })),
  }));
  return { ...draft, steps: [...draft.steps, ...steps] };
}

export function updateStepGroup(draft, groupId, patch) {
  return {
    ...draft,
    steps: draft.steps.map((step) =>
      step.group?.id === groupId
        ? { ...step, group: { ...step.group, ...clone(patch) } }
        : step,
    ),
  };
}

export function upgradeStepGroup(draft, groupId, definition) {
  const start = draft.steps.findIndex((step) => step.group?.id === groupId);
  if (start < 0) return draft;
  const current = draft.steps[start].group;
  const members = addStepBlock(
    { ...draft, steps: [] },
    definition,
    groupId,
    current,
  ).steps;
  const steps = [
    ...draft.steps.slice(0, start),
    ...members,
    ...draft.steps.slice(start).filter((step) => step.group?.id !== groupId),
  ];
  return {
    ...draft,
    steps: steps.map((step, index) => ({
      ...step,
      position: { x: 80, y: index * 190 },
    })),
  };
}

export function sequenceUnits(draft) {
  const units = [];
  for (const step of draft.steps) {
    const last = units.at(-1);
    if (step.group && last?.group?.id === step.group.id) last.steps.push(step);
    else
      units.push({
        id: step.group?.id || step.id,
        group: step.group,
        steps: [step],
      });
  }
  return units;
}

export function validateStepGroups(draft) {
  const errors = [];
  const seen = new Set();
  for (const unit of sequenceUnits(draft)) {
    const group = unit.group;
    if (!group) continue;
    if (seen.has(group.id)) errors.push("步骤组必须保持连续");
    seen.add(group.id);
    const start = draft.steps.findIndex((step) => step.group?.id === group.id);
    const bindings = group.bindings || [];
    const names = new Set((group.parameters || []).map((p) => p.name));
    for (const parameter of group.parameters || []) {
      const bound = bindings.find((b) => b.parameter === parameter.name);
      if (bound) {
        if (
          !draft.steps
            .slice(0, start)
            .some((step) => step.id === bound.sourceStepId) ||
          !bound.path?.trim()
        )
          errors.push(
            `${group.title}：${parameter.name} 的来源必须在步骤组之前`,
          );
        continue;
      }
      const exists = Object.hasOwn(group.values || {}, parameter.name);
      if (!exists && !parameter.hasDefault) {
        if (parameter.required)
          errors.push(`${group.title}：填写参数 ${parameter.name}`);
        continue;
      }
      const value = exists
        ? group.values[parameter.name]
        : parameter.defaultValue;
      if (value !== "[REDACTED]" && !parameterValueValid(parameter, value))
        errors.push(
          `${group.title}：${parameter.name} 应为 ${parameter.dataType}`,
        );
    }
    if (bindings.some((b) => !names.has(b.parameter)))
      errors.push(`${group.title}：绑定参数已被删除`);
  }
  return errors;
}

export function inferBlockParameters(steps, cases, previous = []) {
  const open = new Map();
  for (const step of steps) {
    const variables =
      step.variables ||
      cases.find((item) => item.id === step.caseId)?.variables ||
      [];
    for (const parameter of variables) {
      if (
        !(step.bindings || []).some(
          (binding) => binding.parameter === parameter,
        )
      )
        open.set(`${step.id}:${parameter}`, { stepId: step.id, parameter });
    }
  }
  const result = previous
    .map((parameter) => ({
      ...parameter,
      targets: parameter.targets.filter((target) =>
        open.has(`${target.stepId}:${target.parameter}`),
      ),
    }))
    .filter((parameter) => parameter.targets.length);
  for (const parameter of result)
    for (const target of parameter.targets)
      open.delete(`${target.stepId}:${target.parameter}`);
  for (const target of open.values()) {
    let parameter = result.find((item) => item.name === target.parameter);
    if (!parameter) {
      parameter = {
        name: target.parameter,
        dataType: "string",
        required: true,
        nullable: false,
        hasDefault: false,
        defaultValue: null,
        targets: [],
      };
      result.push(parameter);
    }
    parameter.targets.push(target);
  }
  return result;
}
