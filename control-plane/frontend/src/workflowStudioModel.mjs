export function emptyStudioDraft(id = `workflow.${crypto.randomUUID()}`) {
  return {
    workflow: { id, displayName: "新建场景", description: "" },
    steps: [],
  };
}

export function addStudioStep(
  draft,
  caseDef,
  id = `step.${crypto.randomUUID()}`,
) {
  const step = {
    id,
    title: caseDef.displayName || caseDef.id,
    caseId: caseDef.id,
    required: true,
    position: { x: 80, y: draft.steps.length * 190 },
    bindings: [],
    outputs: [],
  };
  return { ...draft, steps: [...draft.steps, step] };
}

export function updateStudioStep(draft, id, patch) {
  return {
    ...draft,
    steps: draft.steps.map((step) =>
      step.id === id ? { ...step, ...patch } : step,
    ),
  };
}

export function moveStudioStep(draft, id, targetIndex) {
  const steps = [...draft.steps];
  const index = steps.findIndex((step) => step.id === id);
  if (index < 0 || targetIndex < 0 || targetIndex >= steps.length) return draft;
  const groupID = steps[index].group?.id;
  if (groupID) {
    const members = steps.filter((step) => step.group?.id === groupID);
    const remaining = steps.filter((step) => step.group?.id !== groupID);
    let target = steps[targetIndex];
    const targetGroup = target.group?.id;
    const targets = targetGroup
      ? remaining.filter((step) => step.group?.id === targetGroup)
      : [target];
    let insert = remaining.findIndex((step) => step.id === targets[0]?.id);
    if (targetIndex > index) insert += targets.length;
    if (insert < 0) return draft;
    remaining.splice(insert, 0, ...members);
    return {
      ...draft,
      steps: remaining.map((step, i) => ({
        ...step,
        position: { x: 80, y: i * 190 },
      })),
    };
  }
  const targetGroup = steps[targetIndex].group?.id;
  if (targetGroup)
    targetIndex =
      targetIndex < index
        ? steps.findIndex((step) => step.group?.id === targetGroup)
        : steps.findLastIndex((step) => step.group?.id === targetGroup);
  const [step] = steps.splice(index, 1);
  steps.splice(targetIndex, 0, step);
  return {
    ...draft,
    steps: steps.map((item, i) => ({
      ...item,
      position: { x: 80, y: i * 190 },
    })),
  };
}

export function removeStudioStep(draft, id) {
  const groupID = draft.steps.find((step) => step.id === id)?.group?.id;
  return {
    ...draft,
    steps: draft.steps.filter((step) =>
      groupID ? step.group?.id !== groupID : step.id !== id,
    ),
  };
}

export function studioSourceSteps(draft, stepId) {
  const index = draft.steps.findIndex((step) => step.id === stepId);
  return index < 0 ? [] : draft.steps.slice(0, index);
}

export function setStudioBinding(draft, stepId, parameter, binding) {
  const step = draft.steps.find((item) => item.id === stepId);
  if (!step) return draft;
  const bindings = (step.bindings || []).filter(
    (item) => item.parameter !== parameter,
  );
  if (binding?.sourceStepId)
    bindings.push({
      parameter,
      sourceStepId: binding.sourceStepId,
      from: binding.from || "responsebody",
      path: binding.path || "",
    });
  return updateStudioStep(draft, stepId, { bindings });
}

export function validateStudioDraft(draft, cases = []) {
  const errors = [];
  if (!draft.workflow?.id?.trim() || !draft.workflow?.displayName?.trim())
    errors.push("填写场景名称和 ID");
  const available = new Map(cases.map((item) => [item.id, item]));
  const seen = new Set();
  for (const step of draft.steps || []) {
    if (!step.id || seen.has(step.id)) errors.push("步骤 ID 必须唯一");
    seen.add(step.id);
    if (!available.has(step.caseId))
      errors.push(`${step.title || step.id}：关联用例不存在`);
    const sources = studioSourceSteps(draft, step.id);
    const variables = new Set(
      step.variables || available.get(step.caseId)?.variables || [],
    );
    const bound = new Set();
    for (const binding of step.bindings || []) {
      if (!sources.some((source) => source.id === binding.sourceStepId))
        errors.push(`${step.title || step.id}：数据来源必须是前序步骤`);
      if (bound.has(binding.parameter))
        errors.push(`${step.title || step.id}：参数重复绑定`);
      bound.add(binding.parameter);
      if (!variables.has(binding.parameter))
        errors.push(
          `${step.title || step.id}：模板参数 ${binding.parameter} 不存在`,
        );
      if (!validStudioPath(binding.path))
        errors.push(`${step.title || step.id}：填写点路径，如 $.data.id`);
    }
    const outputs = new Set();
    for (const output of step.outputs || []) {
      if (
        !output.name?.trim() ||
        outputs.has(output.name) ||
        output.name.startsWith("studio_")
      )
        errors.push(`${step.title || step.id}：输出变量名称无效或重复`);
      outputs.add(output.name);
      if (!validStudioPath(output.path))
        errors.push(`${step.title || step.id}：填写输出字段路径`);
    }
  }
  return [...new Set(errors)];
}

export function validStudioPath(path) {
  return Boolean(String(path || "").trim()) && !/[\[\]*{}]/.test(path);
}

export function buildStudioSavePayload(draft) {
  return {
    workflow: {
      id: draft.workflow.id,
      displayName: draft.workflow.displayName,
      description: draft.workflow.description || "",
    },
    steps: draft.steps.map((step) => ({
      id: step.id,
      title: step.title,
      caseId: step.caseId,
      required: Boolean(step.required),
      position: { x: step.position?.x ?? 80, y: step.position?.y ?? 0 },
      bindings: (step.bindings || []).map(
        ({ parameter, sourceStepId, from, path }) => ({
          parameter,
          sourceStepId,
          from,
          path,
        }),
      ),
      outputs: (step.outputs || []).map(({ name, from, path }) => ({
        name,
        from,
        path,
      })),
      ...(step.values ? { values: step.values } : {}),
      ...(step.group ? { group: step.group } : {}),
    })),
  };
}

export function studioCanvas(draft, cases = [], results = [], selectedId = "") {
  const byCase = new Map(cases.map((item) => [item.id, item]));
  const byStep = new Map(results.map((item) => [item.stepId, item]));
  const nodes = draft.steps.map((step, index) => ({
    id: step.id,
    type: "studioStep",
    position: step.position || { x: 80, y: index * 190 },
    selected: step.id === selectedId,
    data: {
      step,
      index,
      caseDef: byCase.get(step.caseId),
      result: byStep.get(step.id),
    },
  }));
  const edges = [];
  draft.steps.forEach((step, index) => {
    if (index)
      edges.push({
        id: `sequence:${step.id}`,
        source: draft.steps[index - 1].id,
        target: step.id,
        sourceHandle: "next",
        targetHandle: "previous",
        type: "smoothstep",
        animated: byStep.get(step.id)?.status === "running",
        style: { stroke: "#8295b0", strokeWidth: 2 },
        markerEnd: { type: "arrowclosed", color: "#8295b0" },
      });
    for (const binding of step.bindings || []) {
      if (!draft.steps.some((item) => item.id === binding.sourceStepId))
        continue;
      edges.push({
        id: `data:${step.id}:${binding.parameter}`,
        source: binding.sourceStepId,
        target: step.id,
        sourceHandle: "output",
        targetHandle: "input",
        type: "smoothstep",
        label: `${binding.path} → ${binding.parameter}`,
        style: { stroke: "#18a687", strokeWidth: 2, strokeDasharray: "5 4" },
        labelStyle: { fill: "#168670", fontSize: 11 },
        labelBgStyle: { fill: "#f2fcf8" },
        markerEnd: { type: "arrowclosed", color: "#18a687" },
      });
    }
  });
  return { nodes, edges };
}

export function studioRunnable(draft, cases = []) {
  return (
    draft.steps.length > 0 &&
    draft.steps.every((step) =>
      cases.some((item) => item.id === step.caseId && item.executionReady),
    )
  );
}
