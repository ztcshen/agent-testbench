const detailEl = (id) => document.getElementById(id);
let workflowRunState = emptyWorkflowRunState();

function setDetailMessage(value) {
  detailEl("workflowDetailMessage").textContent = value;
}

async function detailRequest(path) {
  const response = await fetch(path);
  const body = await response.json();
  if (!response.ok) {
    throw new Error(body.error || response.statusText);
  }
  return body;
}

async function detailPost(path, payload) {
  const response = await fetch(path, {
    method: "POST",
    headers: { "content-type": "application/json" },
    body: JSON.stringify(payload || {}),
  });
  const body = await response.json();
  if (!response.ok && response.status >= 500) {
    throw new Error(body.error || response.statusText);
  }
  return body;
}

function selectedWorkflowId() {
  const params = new URLSearchParams(window.location.search);
  return params.get("id") || "";
}

function workflowDetailHref(id) {
  return `/workflow-detail.html?id=${encodeURIComponent(id || "")}`;
}

function workflowStepHref(workflowId, stepId) {
  const params = new URLSearchParams();
  params.set("workflow", workflowId || "");
  params.set("step", stepId || "");
  if (workflowRunState.workflowRunId) {
    params.set("runId", workflowRunState.workflowRunId);
  }
  return `/workflow-step.html?${params.toString()}`;
}

function emptyWorkflowRunState(workflow = null) {
  return {
    workflowId: workflow?.id || "",
    workflowRunId: 0,
    ok: false,
    status: "idle",
    elapsedMs: 0,
    summary: { stepCount: 0, expectedStepCount: workflow?.steps?.length || 0 },
    steps: [],
  };
}

function workflowRunEventsHref(workflow) {
  void workflow;
  return "";
}

function workflowRunSupported(workflow) {
  const steps = workflow?.steps || [];
  return Boolean(steps.length) && steps.every((step) => step.executable !== false);
}

function consumeWorkflowEventStream(url, eventNames, onEvent) {
  return new Promise((resolve, reject) => {
    const source = new EventSource(url);
    let settled = false;
    const finish = () => {
      if (!settled) {
        settled = true;
        source.close();
        resolve();
      }
    };
    const fail = () => {
      if (!settled) {
        settled = true;
        source.close();
        reject(new Error("event stream disconnected"));
      }
    };
    eventNames.forEach((eventName) => {
      source.addEventListener(eventName, (event) => {
        const data = JSON.parse(event.data);
        onEvent({ event: eventName, data });
        if (eventName === "workflow-completed" || eventName === "workflow-failed") {
          finish();
        }
      });
    });
    source.onerror = fail;
  });
}

function runStepId(step) {
  return step?.stepId || step?.id || "";
}

function runStepOK(step) {
  if (!step) return false;
  if (step.stepOk !== undefined) return Boolean(step.stepOk);
  return Boolean(step.ok);
}

function parseMaybeJSON(value) {
  if (!value) return {};
  if (typeof value === "object") return value;
  if (typeof value !== "string") return {};
  try {
    return JSON.parse(value);
  } catch (_error) {
    return {};
  }
}

function workflowValue(value) {
  if (value === undefined || value === null) return "";
  if (typeof value === "string") return value.trim();
  return String(value).trim();
}

function genericStepBodyHealth(result) {
  const parsed = result?.result || {};
  const response = parsed.response || {};
  const body = parseMaybeJSON(response.body);
  const extParams = body.ext_params || {};
  const resultStatus = workflowValue(body.result_status || extParams.result_status).toUpperCase();
  if (["F", "FAIL", "FAILED"].includes(resultStatus)) {
    return { ok: false, level: "failed", message: workflowValue(body.failed_msg || body.fail_msg || body.message || "result_status=F") };
  }
  const code = workflowValue(body.code);
  if (code && code !== "0") {
    return { ok: false, level: "failed", message: workflowValue(body.message || `code=${code}`) };
  }
  return { ok: true, level: "ok", message: "" };
}

function valueAtPath(root, path) {
  const text = workflowValue(path);
  if (!text) return undefined;
  return text.split(".").reduce((current, segment) => {
    if (current === undefined || current === null) return undefined;
    if (Array.isArray(current) && /^\d+$/.test(segment)) return current[Number(segment)];
    if (typeof current === "object") return current[segment];
    return undefined;
  }, root);
}

function absorbConfiguredWorkflowContext(ctx, step, result) {
  const parsed = result?.result || {};
  const request = parsed.request || {};
  const response = parsed.response || {};
  const responseBody = parseMaybeJSON(response.body);
  const remember = (key, value) => {
    const text = workflowValue(value);
    if (text && text !== "<nil>") ctx[key] = text;
  };

  (step.exports || []).forEach((item) => {
    const name = workflowValue(item.name || item.key || item.field);
    const from = workflowValue(item.from || "response");
    const path = item.path || name;
    if (from === "request") {
      remember(name, valueAtPath(request, path) ?? valueAtPath(request.body, path));
      return;
    }
    const root = from === "requestBody" ? request.body : from === "responseBody" ? responseBody : response;
    remember(name, valueAtPath(root, path));
  });
  (step.exportFields || []).forEach((name) => {
    const requestValue = valueAtPath(request, `body.${name}`) ?? valueAtPath(request, name);
    const responseValue = valueAtPath(responseBody, name) ?? valueAtPath(response, name);
    remember(name, responseValue ?? requestValue);
  });
}

function genericWorkflowStepOverrides(step, ctx) {
  const overrides = {};
  const add = (key, value) => {
    const text = workflowValue(value);
    if (text) overrides[key] = text;
  };

  (step.inputs || []).forEach((input) => {
    const name = workflowValue(input.name || input.key || input.field);
    if (!name) return;
    add(name, ctx[name] || input.default || input.value || "");
  });
  if (step.overrides && typeof step.overrides === "object") {
    Object.entries(step.overrides).forEach(([key, value]) => add(key, value));
  }
  return overrides;
}

function genericWorkflowSnapshot(workflow, startedAt, steps, ctx, final = false) {
  const expected = workflow?.steps?.length || 0;
  const ok = final && steps.length === expected && steps.every((step) => runStepOK(step));
  const done = new Set(steps.map((step) => step.stepId));
  return {
    workflowId: workflow?.id || "",
    workflowRunId: 0,
    ok,
    status: final ? (ok ? "passed" : "failed") : "running",
    elapsedMs: Date.now() - startedAt,
    identifiers: ctx,
    steps,
    missingSteps: (workflow?.steps || []).filter((step) => !done.has(step.id)).map((step) => step.displayName || step.id),
    warnings: [],
    summary: { passed: ok, stepCount: steps.length, expectedStepCount: expected },
  };
}

function unsupportedGenericStepResult(step, dryRun, reason = "") {
  const ok = Boolean(dryRun);
  return {
    ok,
    stepOk: ok,
    code: ok ? 0 : 1,
    dryRun,
    caseId: step.caseId || "",
    stepId: step.id,
    title: step.displayName || step.id,
    elapsedMs: 0,
    summary: { passed: ok, failureReason: ok ? "" : reason || `step action ${step.action || "-"} has no executable adapter` },
    bodyHealth: { ok, level: ok ? "ok" : "failed", message: ok ? "" : reason || "runner adapter missing" },
    result: { request: { action: step.action || "-", caseId: step.caseId || "" }, response: { body: "" } },
  };
}

async function runGenericWorkflowInBrowser(workflow) {
  const dryRun = Boolean(detailEl("workflowDryRun")?.checked);
  const timeoutSeconds = 120;
  const startedAt = Date.now();
  const steps = [];
  const ctx = {};
  workflowRunState = genericWorkflowSnapshot(workflow, startedAt, steps, ctx, false);
  renderWorkflowRunner(workflow);

  for (const step of workflow.steps || []) {
    workflowRunState = {
      ...genericWorkflowSnapshot(workflow, startedAt, steps, ctx, false),
      currentStep: { stepId: step.id, title: step.displayName || step.id, caseId: step.caseId || "" },
    };
    renderWorkflowRunner(workflow);
    setDetailMessage(`running ${steps.length}/${workflow.steps.length}: ${step.displayName || step.id}`);

    let result;
    if (!step.caseId) {
      result = unsupportedGenericStepResult(step, dryRun);
    } else {
      const stepStartedAt = Date.now();
      result = await detailPost("/api/test-kit/run", {
        caseId: step.caseId,
        workflowId: workflow.id,
        stepId: step.id,
        dryRun,
        timeoutSeconds,
        skipTraceTopology: false,
        overrides: genericWorkflowStepOverrides(step, ctx),
      });
      result.stepId = step.id;
      result.title = step.displayName || step.id;
      result.elapsedMs = result.elapsedMs || Date.now() - stepStartedAt;
      result.bodyHealth = genericStepBodyHealth(result);
      result.stepOk = Boolean(result.ok) && Boolean(result.bodyHealth.ok);
    }

    steps.push(result);
    absorbConfiguredWorkflowContext(ctx, step, result);
    workflowRunState = genericWorkflowSnapshot(workflow, startedAt, steps, ctx, false);
    renderWorkflowRunner(workflow);
    if (!runStepOK(result)) break;
  }

  workflowRunState = genericWorkflowSnapshot(workflow, startedAt, steps, ctx, true);
  if (workflowRunState.steps.length) {
    const saved = await detailPost("/api/workflow-runs", workflowRunState);
    if (saved.workflowRunId) {
      workflowRunState = { ...workflowRunState, workflowRunId: saved.workflowRunId };
      const params = new URLSearchParams(window.location.search);
      params.set("id", workflow.id || "");
      params.set("runId", saved.workflowRunId);
      window.history.replaceState({}, "", `/workflow-detail.html?${params.toString()}`);
    }
  }
  renderWorkflowRunner(workflow);
  setDetailMessage(workflowRunState.ok ? "workflow completed" : "workflow failed");
}

function workflowProgressState(workflow) {
  const steps = workflow?.steps || [];
  const realById = new Map((workflowRunState.steps || []).map((step) => [runStepId(step), step]));
  const attempted = steps.filter((step) => realById.has(step.id)).length;
  const failedStep = steps.find((step) => {
    const real = realById.get(step.id);
    return real && !runStepOK(real);
  });
  const completed = steps.filter((step) => runStepOK(realById.get(step.id))).length;
  const currentStepId = workflowRunState.currentStep?.stepId || failedStep?.id || steps[Math.min(attempted, Math.max(steps.length - 1, 0))]?.id;
  const current = steps.find((step) => step.id === currentStepId) || failedStep || steps[0];
  const percent = steps.length ? Math.round((attempted / steps.length) * 100) : 0;
  return { steps, realById, attempted, completed, failedStep, current, percent };
}

function renderWorkflowRunner(workflow) {
  const runButton = detailEl("runWorkflowBtn");
  const supported = workflowRunSupported(workflow);
  runButton.disabled = !supported;
  runButton.textContent = supported ? "运行 Workflow" : "运行配置未接入";

  const { steps, realById, attempted, failedStep, current, percent } = workflowProgressState(workflow);
  const total = steps.length || workflowRunState.summary?.expectedStepCount || 0;
  detailEl("workflowRunnerId").textContent = workflow?.id || "-";
  detailEl("workflowRunnerStepCount").textContent = `${attempted} / ${total}`;
  detailEl("workflowRunnerElapsed").textContent = workflowRunState.elapsedMs ? `${workflowRunState.elapsedMs} ms` : "-";
  const status = detailEl("workflowRunnerStatus");
  const statusText = workflowRunState.status === "running" ? "RUNNING" : workflowRunState.ok ? "PASS" : failedStep ? "FAIL" : supported ? "未运行" : "配置未接入";
  status.className = `status-pill ${workflowRunState.status === "running" ? "running" : workflowRunState.ok ? "passed" : failedStep ? "failed" : "idle"}`;
  status.textContent = statusText;

  const progressLabel = workflowRunState.status === "running" ? "运行中" : failedStep ? "停在" : workflowRunState.ok ? "已完成" : supported ? "等待运行" : "等待可执行配置";
  detailEl("workflowProgressText").textContent = `${attempted} / ${total}`;
  detailEl("workflowProgressCurrent").textContent = `${progressLabel}: ${current?.displayName || current?.id || "-"}`;
  detailEl("workflowProgressFill").style.width = `${Math.min(100, Math.max(0, percent))}%`;

  const list = detailEl("workflowProgressSteps");
  list.innerHTML = "";
  steps.forEach((step, index) => {
    const real = realById.get(step.id);
    const item = document.createElement("a");
    item.className = "workflow-progress-step";
    item.href = workflowStepHref(workflow.id, step.id);
    if (runStepOK(real)) {
      item.classList.add(real?.bodyHealth?.level === "warning" ? "warning" : "passed");
    } else if (real) {
      item.classList.add("failed");
    } else if (workflowRunState.status === "running" && index === attempted) {
      item.classList.add("running");
    } else if (step.executable === false) {
      item.classList.add("failed");
    } else {
      item.classList.add("pending");
    }

    const number = document.createElement("span");
    number.className = "workflow-progress-index";
    number.textContent = String(index + 1).padStart(2, "0");
    const title = document.createElement("span");
    title.className = "workflow-progress-title";
    title.textContent = step.displayName || step.id || "-";
    item.appendChild(number);
    item.appendChild(title);
    list.appendChild(item);
  });
}

function workflowCatalogServiceHref(service) {
  if (service?.role === "external") {
    return `/service-inventory.html#service-${encodeURIComponent(service.id || "external")}`;
  }
  return `/environment-node.html?id=${encodeURIComponent(service?.id || "")}`;
}

function serviceById(catalog) {
  return new Map((catalog.services || []).map((service) => [service.id, service]));
}

function serviceDisplay(serviceId, services) {
  const service = services.get(serviceId);
  if (!service) {
    return serviceId ? `${serviceId} · 未在服务拓扑中` : "-";
  }
  const bits = [
    service.displayName || service.id,
    service.role || "",
    service.port ? `:${service.port}` : "",
  ].filter(Boolean);
  return bits.join(" · ");
}

function renderWorkflowServiceCoverage(workflow, services) {
  const target = detailEl("workflowServiceCoverage");
  target.innerHTML = "";
  target.className = "workflow-service-summary";
  target.setAttribute("aria-label", "服务覆盖");

  const serviceIds = [...new Set((workflow?.steps || []).map((step) => step.serviceId).filter(Boolean))];
  if (!serviceIds.length) {
    const empty = document.createElement("code");
    empty.textContent = "未声明服务";
    target.appendChild(empty);
    return;
  }

  serviceIds.forEach((serviceId) => {
    const service = services.get(serviceId);
    if (!service) {
      const chip = document.createElement("code");
      chip.className = "unknown";
      chip.textContent = `${serviceId} · 未建模`;
      target.appendChild(chip);
    } else {
      const chip = document.createElement("a");
      chip.className = "workflow-service-link";
      chip.href = `/workflows.html?workflowFilter=${encodeURIComponent(service.id)}`;
      chip.textContent = service.displayName || service.id;
      target.appendChild(chip);
    }
  });
}

function catalogSourceText(catalog) {
  const version = catalog.schemaVersion ? ` v${catalog.schemaVersion}` : "";
  if (catalog.source?.ok === false) {
    return `Catalog${version} fallback`;
  }
  const path = catalog.source?.path || "Catalog manifest";
  return `Catalog${version}: ${path}`;
}

function chipList(values, emptyText = "-") {
  const wrap = document.createElement("div");
  wrap.className = "workflow-detail-chips";
  if (!values?.length) {
    const empty = document.createElement("span");
    empty.textContent = emptyText;
    wrap.appendChild(empty);
    return wrap;
  }
  values.forEach((value) => {
    const chip = document.createElement("span");
    chip.textContent = value;
    wrap.appendChild(chip);
  });
  return wrap;
}

function renderWorkflowSelector(workflows, selectedId) {
  const selector = detailEl("workflowSelector");
  selector.innerHTML = "";
  if (!workflows?.length) {
    selector.disabled = true;
    const option = document.createElement("option");
    option.textContent = "暂无 Workflow";
    selector.appendChild(option);
    return;
  }
  selector.disabled = false;
  workflows.forEach((workflow) => {
    const option = document.createElement("option");
    option.value = workflow.id || "";
    option.textContent = workflow.displayName || workflow.id || "-";
    option.selected = workflow.id === selectedId;
    selector.appendChild(option);
  });
}

function renderCatalogWarnings(catalog, selectedId = "") {
  const target = detailEl("workflowWarnings");
  const selectedWorkflowWarningPrefix = selectedId ? `workflow "${selectedId}"` : "";
  const warnings = selectedWorkflowWarningPrefix
    ? (catalog.warnings || []).filter((warning) => warning.startsWith(selectedWorkflowWarningPrefix))
    : catalog.warnings || [];
  target.innerHTML = "";
  target.hidden = !warnings.length;
  if (!warnings.length) return;

  const title = document.createElement("strong");
  title.textContent = `${warnings.length} 条 Catalog 警告`;
  target.appendChild(title);
  warnings.forEach((warning) => {
    const item = document.createElement("p");
    item.textContent = warning;
    target.appendChild(item);
  });
}

function summarizeValues(values) {
  const counts = new Map();
  (values || []).filter(Boolean).forEach((value) => {
    counts.set(value, (counts.get(value) || 0) + 1);
  });
  return [...counts.entries()].map(([value, count]) => (count > 1 ? `${value} x${count}` : value));
}

function renderCoverageCard(titleText, values, emptyText) {
  const card = document.createElement("article");
  card.className = "workflow-coverage-card";
  const title = document.createElement("strong");
  title.textContent = titleText;
  card.appendChild(title);
  card.appendChild(chipList(values, emptyText));
  return card;
}

function workflowObservabilityPanelValues(panel, workflow, services) {
  const steps = workflow?.steps || [];
  switch (panel.type) {
    case "workflowGraph":
      return [`${workflow?.graph?.nodes?.length || 0} nodes`, `${workflow?.graph?.edges?.length || 0} edges`];
    case "stepSequence":
      return [`${steps.length || 0} steps`];
    case "serviceEvidence":
      return summarizeValues(steps.map((step) => serviceDisplay(step.serviceId, services)));
    case "evidenceKinds":
      return summarizeValues(steps.flatMap((step) => step.evidenceKinds || []));
    case "mockTargets":
      return summarizeValues(steps.flatMap((step) => step.relatedMockTargets || []));
    case "databaseHints":
      return summarizeValues(steps.flatMap((step) => (step.databaseHints || []).map((hint) => hint.table)));
    case "caseRunner":
      return summarizeValues(steps.map((step) => step.caseId).filter(Boolean));
    case "runHistory":
      return [workflow?.entrypoint || "-"];
    case "configEvidence":
      return summarizeValues(steps.map((step) => step.action).filter(Boolean));
    default:
      return [panel.type || "unknown"];
  }
}

function renderWorkflowObservabilityBoard(workflow, services) {
  const panel = detailEl("workflowObservabilityBoard");
  panel.innerHTML = "";
  panel.hidden = !workflow;
  if (!workflow) return;

  const panels = workflow.observability?.panels || [];
  const head = document.createElement("div");
  head.className = "section-head";
  const titleWrap = document.createElement("div");
  const title = document.createElement("h2");
  title.textContent = "观测看板";
  const summary = document.createElement("p");
  summary.textContent = `${panels.length || 0} configured panels`;
  titleWrap.appendChild(title);
  titleWrap.appendChild(summary);
  head.appendChild(titleWrap);

  const grid = document.createElement("div");
  grid.className = "workflow-observability-grid";
  if (!panels.length) {
    const empty = document.createElement("p");
    empty.className = "empty-note";
    empty.textContent = "该 Workflow 未声明观测看板。";
    grid.appendChild(empty);
  }
  panels.forEach((item) => {
    const card = document.createElement("article");
    card.className = "workflow-observability-card";
    const titleLine = document.createElement("strong");
    titleLine.textContent = item.title || item.id || item.type || "-";
    const meta = document.createElement("code");
    meta.textContent = [item.type || "-", item.scope || "workflow"].join(" · ");
    card.appendChild(titleLine);
    card.appendChild(meta);
    card.appendChild(chipList(workflowObservabilityPanelValues(item, workflow, services), "无配置值"));
    grid.appendChild(card);
  });

  panel.appendChild(head);
  panel.appendChild(grid);
}

function coverageNumber(summary, key) {
  const value = Number(summary?.[key] || 0);
  return Number.isFinite(value) ? value : 0;
}

function renderInterfaceCoverageRow(row) {
  const item = document.createElement("article");
  item.className = `workflow-interface-coverage-row ${row.mapped ? "mapped" : "unmapped"}`;

  const title = document.createElement("div");
  title.className = "workflow-interface-coverage-title";
  const step = document.createElement("strong");
  step.textContent = row.stepTitle || row.stepId || "-";
  const meta = document.createElement("code");
  meta.textContent = [row.stepId, row.caseId, row.serviceId].filter(Boolean).join(" · ") || "-";
  title.appendChild(step);
  title.appendChild(meta);

  const state = document.createElement("div");
  state.className = "workflow-interface-coverage-state";
  const status = document.createElement("span");
  status.className = `workflow-interface-admission ${row.admissionStatus || "unknown"}`;
  status.textContent = row.admissionStatus || "unknown";
  const source = document.createElement("code");
  source.textContent = row.mappingSource || "unmapped";
  state.appendChild(status);
  state.appendChild(source);

  const target = document.createElement("div");
  target.className = "workflow-interface-coverage-target";
  if (row.href) {
    const link = document.createElement("a");
    link.className = "button-link workflow-interface-node-link";
    link.href = row.href;
    link.textContent = row.nodeDisplayName || row.nodeId || "接口节点";
    target.appendChild(link);
  } else {
    const empty = document.createElement("span");
    empty.textContent = "未映射接口节点";
    target.appendChild(empty);
  }

  item.appendChild(title);
  item.appendChild(state);
  item.appendChild(target);
  return item;
}

function renderInterfaceCoverageSection(coverage) {
  const rows = coverage?.rows || [];
  const section = document.createElement("section");
  section.className = "workflow-interface-coverage";
  const title = document.createElement("h3");
  title.textContent = "接口节点覆盖明细";
  section.appendChild(title);

  const list = document.createElement("div");
  list.className = "workflow-interface-coverage-list";
  if (!rows.length) {
    const empty = document.createElement("p");
    empty.className = "empty-note";
    empty.textContent = "当前 Workflow 没有可检查的接口节点映射。";
    list.appendChild(empty);
  }
  rows.forEach((row) => list.appendChild(renderInterfaceCoverageRow(row)));
  section.appendChild(list);
  return section;
}

function renderWorkflowInterfaceGapPreview(payload) {
  const panel = detailEl("workflowCoveragePanel");
  let preview = panel.querySelector(".workflow-interface-gap-preview");
  if (!preview) {
    preview = document.createElement("section");
    preview.className = "workflow-interface-gap-preview";
    const head = document.createElement("div");
    head.className = "section-head";
    const titleWrap = document.createElement("div");
    const title = document.createElement("h3");
    title.textContent = "接口节点缺口 JSON";
    const summary = document.createElement("p");
    summary.className = "workflow-interface-gap-preview-summary";
    titleWrap.appendChild(title);
    titleWrap.appendChild(summary);
    const copy = document.createElement("button");
    copy.className = "button-link workflow-interface-gap-copy-button";
    copy.type = "button";
    copy.textContent = "复制 JSON";
    head.appendChild(titleWrap);
    head.appendChild(copy);
    const pre = document.createElement("pre");
    preview.appendChild(head);
    preview.appendChild(pre);
    panel.appendChild(preview);
  }
  const text = JSON.stringify(payload, null, 2);
  preview.querySelector(".workflow-interface-gap-preview-summary").textContent =
    `${payload?.summary?.gapCount ?? payload?.gaps?.length ?? 0} gaps`;
  preview.querySelector("pre").textContent = text;
  preview.querySelector(".workflow-interface-gap-copy-button").onclick = async () => {
    try {
      await navigator.clipboard.writeText(text);
      setDetailMessage("gap json copied");
    } catch (_error) {
      setDetailMessage("gap json ready to copy");
    }
  };
}

async function previewWorkflowInterfaceGaps(workflowID, button) {
  if (!workflowID) return;
  const originalText = button?.textContent || "预览缺口";
  if (button) {
    button.disabled = true;
    button.textContent = "加载中";
  }
  try {
    const payload = await detailRequest(`/api/interface-node/coverage-gaps?workflow=${encodeURIComponent(workflowID)}`);
    renderWorkflowInterfaceGapPreview(payload);
  } finally {
    if (button) {
      button.disabled = false;
      button.textContent = originalText;
    }
  }
}

function renderWorkflowStepInterfaceHints(workflow, coverage) {
  const rowsByStep = new Map((coverage?.rows || []).map((row) => [row.stepId, row]));
  document.querySelectorAll(".workflow-detail-step").forEach((card) => {
    const stepID = card.getAttribute("data-step-id") || "";
    const target = card.querySelector(".workflow-step-interface-hint");
    if (!target) return;
    target.innerHTML = "";
    const row = rowsByStep.get(stepID);
    if (!row) {
      target.textContent = "接口节点未检查";
      return;
    }
    const status = document.createElement("span");
    status.className = `workflow-interface-admission ${row.admissionStatus || "unknown"}`;
    status.textContent = row.admissionStatus || "unknown";
    target.appendChild(status);
    if (row.href) {
      const link = document.createElement("a");
      link.href = row.href;
      link.textContent = row.nodeDisplayName || row.nodeId || "接口节点";
      target.appendChild(link);
      return;
    }
    const link = document.createElement("a");
    link.href = `/api/interface-node/coverage-gaps?workflow=${encodeURIComponent(workflow?.id || row.workflowId || "")}`;
    link.textContent = "接口节点未映射";
    target.appendChild(link);
  });
}

function renderWorkflowCoveragePanel(workflow, services, interfaceCoverage = null) {
  const panel = detailEl("workflowCoveragePanel");
  panel.innerHTML = "";
  panel.hidden = !workflow;
  if (!workflow) return;

  const steps = workflow.steps || [];
  const serviceIds = [...new Set(steps.map((step) => step.serviceId).filter(Boolean))];
  const serviceNames = serviceIds.map((serviceId) => serviceDisplay(serviceId, services));
  const evidence = summarizeValues(steps.flatMap((step) => step.evidenceKinds || []));
  const mocks = summarizeValues(steps.flatMap((step) => step.relatedMockTargets || []));
  const cases = steps.map((step) => step.caseId).filter(Boolean);
  const interfaceSummary = interfaceCoverage?.summary || {};
  const interfaceTotal = coverageNumber(interfaceSummary, "totalSteps");
  const interfaceMapped = coverageNumber(interfaceSummary, "mappedSteps");
  const interfacePassed = coverageNumber(interfaceSummary, "passedNodes");
  const interfaceFailed = coverageNumber(interfaceSummary, "failedNodes");
  const interfacePending = coverageNumber(interfaceSummary, "pendingNodes");
  const interfaceUnmapped = coverageNumber(interfaceSummary, "unmappedSteps");

  const head = document.createElement("div");
  head.className = "section-head";
  const titleWrap = document.createElement("div");
  const title = document.createElement("h2");
  title.textContent = "覆盖摘要";
  const summary = document.createElement("p");
  summary.textContent = [
    `${serviceIds.length || 0} services`,
    `${evidence.length || 0} evidence kinds`,
    `${cases.length || 0} cases`,
    interfaceCoverage ? `interface mapped ${interfaceMapped}/${interfaceTotal}` : "",
  ].filter(Boolean).join(" · ");
  titleWrap.appendChild(title);
  titleWrap.appendChild(summary);
  head.appendChild(titleWrap);
  if (interfaceCoverage && interfaceUnmapped > 0) {
    const previewButton = document.createElement("button");
    previewButton.className = "button-link workflow-interface-gap-preview-button";
    previewButton.type = "button";
    previewButton.textContent = "预览缺口";
    previewButton.addEventListener("click", () => previewWorkflowInterfaceGaps(workflow.id || "", previewButton));
    head.appendChild(previewButton);
    const gapLink = document.createElement("a");
    gapLink.className = "button-link workflow-interface-gap-export";
    gapLink.href = `/api/interface-node/coverage-gaps?workflow=${encodeURIComponent(workflow.id || "")}`;
    gapLink.textContent = "缺口 JSON";
    head.appendChild(gapLink);
  }

  const grid = document.createElement("div");
  grid.className = "workflow-coverage-grid";
  grid.appendChild(renderCoverageCard("服务", serviceNames, "未声明服务"));
  grid.appendChild(renderCoverageCard("Evidence", evidence, "无 Evidence"));
  grid.appendChild(renderCoverageCard("Mock target", mocks, "无 Mock"));
  grid.appendChild(renderCoverageCard("Case ID", cases, "无 case"));
  if (interfaceCoverage) {
    grid.appendChild(renderCoverageCard("接口节点准入", [
      `mapped ${interfaceMapped}/${interfaceTotal}`,
      `passed ${interfacePassed}`,
      `failed ${interfaceFailed}`,
      `pending ${interfacePending}`,
      `unmapped ${interfaceUnmapped}`,
    ], "暂无接口节点覆盖"));
  }

  panel.appendChild(head);
  panel.appendChild(grid);
  if (interfaceCoverage) {
    panel.appendChild(renderInterfaceCoverageSection(interfaceCoverage));
  }
}

function topologyAdjacency(catalog) {
  const adjacency = new Map();
  (catalog.topology?.edges || []).forEach((edge) => {
    if (!edge.from || !edge.to) return;
    if (!adjacency.has(edge.from)) adjacency.set(edge.from, []);
    adjacency.get(edge.from).push(edge.to);
  });
  return adjacency;
}

function shortestTopologyPath(from, to, adjacency) {
  if (!from || !to) return [];
  if (from === to) return [from];
  const queue = [[from]];
  const seen = new Set([from]);
  while (queue.length) {
    const path = queue.shift();
    const current = path[path.length - 1];
    for (const next of adjacency.get(current) || []) {
      if (seen.has(next)) continue;
      const candidate = [...path, next];
      if (next === to) return candidate;
      seen.add(next);
      queue.push(candidate);
    }
  }
  return [];
}

function addGraphPath(path, nodes, edges) {
  path.filter(Boolean).forEach((node) => nodes.add(node));
  for (let index = 0; index < path.length - 1; index += 1) {
    edges.add(`${path[index]}->${path[index + 1]}`);
  }
}

function buildWorkflowGraph(workflow, catalog) {
  if (workflow?.graph?.edges?.length || workflow?.graph?.nodes?.length) {
    const nodes = new Set(workflow.graph.nodes || []);
    (workflow.graph.edges || []).forEach((edge) => {
      if (edge.from) nodes.add(edge.from);
      if (edge.to) nodes.add(edge.to);
    });
    return {
      nodes: [...nodes],
      edges: (workflow.graph.edges || []).map((edge) => [edge.from, edge.to]).filter(([from, to]) => from && to),
    };
  }
  const adjacency = topologyAdjacency(catalog);
  const nodes = new Set();
  const edges = new Set();
  const primaryServices = (workflow.steps || []).map((step) => step.serviceId).filter(Boolean);

  primaryServices.forEach((serviceId) => nodes.add(serviceId));
  for (let index = 0; index < primaryServices.length - 1; index += 1) {
    const from = primaryServices[index];
    const to = primaryServices[index + 1];
    const path = shortestTopologyPath(from, to, adjacency);
    if (path.length) {
      addGraphPath(path, nodes, edges);
    }
  }

  (workflow.steps || []).forEach((step) => {
    [...(step.relatedServiceIds || []), ...(step.relatedMockTargets || [])].forEach((target) => {
      nodes.add(target);
      const parent = [...nodes].find((node) => (adjacency.get(node) || []).includes(target));
      if (parent) {
        edges.add(`${parent}->${target}`);
      }
    });
  });

  (catalog.topology?.edges || []).forEach((edge) => {
    if (nodes.has(edge.from) && nodes.has(edge.to)) {
      edges.add(`${edge.from}->${edge.to}`);
    }
  });

  return { nodes: [...nodes], edges: [...edges].map((edge) => edge.split("->")) };
}

function renderWorkflowGraphPanel(workflow, catalog, services) {
  const panel = detailEl("workflowGraphPanel");
  panel.innerHTML = "";
  panel.hidden = !workflow;
  if (!workflow) return;

  const graph = buildWorkflowGraph(workflow, catalog);
  const head = document.createElement("div");
  head.className = "section-head";
  const titleWrap = document.createElement("div");
  const title = document.createElement("h2");
  title.textContent = "Workflow 链路";
  const summary = document.createElement("p");
  summary.textContent = `${graph.nodes.length} services · ${graph.edges.length} true catalog edges`;
  titleWrap.appendChild(title);
  titleWrap.appendChild(summary);
  head.appendChild(titleWrap);

  const nodeGrid = document.createElement("div");
  nodeGrid.className = "workflow-graph-nodes";
  graph.nodes.forEach((serviceId) => {
    const service = services.get(serviceId);
    const node = document.createElement(service ? "a" : "article");
    node.className = `workflow-graph-node ${service?.role || "unknown"}`;
    if (service) node.href = workflowCatalogServiceHref(service);
    const name = document.createElement("strong");
    name.textContent = service?.displayName || serviceId;
    const meta = document.createElement("span");
    meta.textContent = [service?.role || "未建模", service?.port ? `:${service.port}` : ""].filter(Boolean).join(" · ");
    node.appendChild(name);
    node.appendChild(meta);
    nodeGrid.appendChild(node);
  });

  const edgeList = document.createElement("div");
  edgeList.className = "workflow-graph-edges";
  if (!graph.edges.length) {
    const empty = document.createElement("p");
    empty.textContent = "该 Workflow 没有可从 Catalog 拓扑推导的边。";
    edgeList.appendChild(empty);
  } else {
    graph.edges.forEach(([from, to]) => {
      const edge = document.createElement("article");
      edge.className = "workflow-graph-edge";
      const fromText = document.createElement("strong");
      fromText.textContent = services.get(from)?.displayName || from;
      const arrow = document.createElement("span");
      arrow.textContent = "->";
      const toText = document.createElement("strong");
      toText.textContent = services.get(to)?.displayName || to;
      edge.appendChild(fromText);
      edge.appendChild(arrow);
      edge.appendChild(toText);
      edgeList.appendChild(edge);
    });
  }

  panel.appendChild(head);
  panel.appendChild(nodeGrid);
  panel.appendChild(edgeList);
}

function renderWorkflowDetail(catalog) {
  const workflows = catalog.workflows || [];
  const requested = selectedWorkflowId();
  let workflow = workflows.find((item) => item.id === requested);
  if (requested && !workflow) {
    renderCatalogWarnings(catalog, requested);
    renderWorkflowSelector(workflows, requested);
    detailEl("workflowDetailTitle").textContent = "Workflow 未找到";
    detailEl("workflowDetailSummary").textContent = `Catalog 中没有 ${requested}`;
    detailEl("workflowDetailId").textContent = requested;
    detailEl("workflowDetailEntrypoint").textContent = "-";
    detailEl("workflowStepSummary").textContent = "0 steps";
    detailEl("workflowSource").textContent = catalogSourceText(catalog);
    renderWorkflowServiceCoverage(null, serviceById(catalog));
    renderWorkflowObservabilityBoard(null, serviceById(catalog));
    renderWorkflowGraphPanel(null, catalog, serviceById(catalog));
    renderWorkflowCoveragePanel(null, serviceById(catalog));
    workflowRunState = emptyWorkflowRunState(null);
    renderWorkflowRunner(null);
    detailEl("workflowStepList").innerHTML = "";
    return null;
  }
  workflow = workflow || workflows[0];
  if (!workflow) {
    renderCatalogWarnings(catalog);
    renderWorkflowSelector(workflows, "");
    detailEl("workflowDetailTitle").textContent = "暂无 Workflow 定义";
    detailEl("workflowDetailSummary").textContent = "Catalog 暂未返回 Workflow。";
    detailEl("workflowStepSummary").textContent = "0 steps";
    renderWorkflowServiceCoverage(null, serviceById(catalog));
    renderWorkflowObservabilityBoard(null, serviceById(catalog));
    renderWorkflowGraphPanel(null, catalog, serviceById(catalog));
    renderWorkflowCoveragePanel(null, serviceById(catalog));
    workflowRunState = emptyWorkflowRunState(null);
    renderWorkflowRunner(null);
    detailEl("workflowStepList").innerHTML = "";
    return null;
  }

  renderCatalogWarnings(catalog, workflow.id);
  renderWorkflowSelector(workflows, workflow.id);
  detailEl("workflowDetailTitle").textContent = workflow.displayName || workflow.id;
  detailEl("workflowDetailSummary").textContent = workflow.description || "-";
  detailEl("workflowDetailId").textContent = workflow.id || "-";
  detailEl("workflowDetailEntrypoint").textContent = workflow.entrypoint || "-";
  detailEl("workflowStepSummary").textContent = `${workflow.steps?.length || 0} steps`;
  detailEl("workflowSource").textContent = catalogSourceText(catalog);
  const services = serviceById(catalog);
  renderWorkflowServiceCoverage(workflow, services);
  renderWorkflowObservabilityBoard(workflow, services);
  renderWorkflowGraphPanel(workflow, catalog, services);
  renderWorkflowCoveragePanel(workflow, services);
  if (workflowRunState.workflowId !== workflow.id || workflowRunState.status === "idle") {
    workflowRunState = emptyWorkflowRunState(workflow);
  }
  renderWorkflowRunner(workflow);

  const list = detailEl("workflowStepList");
  list.innerHTML = "";
  (workflow.steps || []).forEach((step, index) => {
    const card = document.createElement("article");
    card.className = "workflow-detail-step";
    card.setAttribute("data-step-id", step.id || "");

    const top = document.createElement("div");
    top.className = "workflow-detail-step-top";
    const title = document.createElement("strong");
    title.textContent = `${String(index + 1).padStart(2, "0")} ${step.displayName || step.id}`;
    const code = document.createElement("code");
    code.textContent = step.id || "-";
    top.appendChild(title);
    top.appendChild(code);

    const meta = document.createElement("dl");
    meta.className = "workflow-detail-meta";
    [
      ["service", serviceDisplay(step.serviceId, services)],
      ["action", step.action || "-"],
      ["case", step.caseId || "-"],
    ].forEach(([name, value]) => {
      const dt = document.createElement("dt");
      dt.textContent = name;
      const dd = document.createElement("dd");
      dd.textContent = value;
      meta.appendChild(dt);
      meta.appendChild(dd);
    });

    card.appendChild(top);
    card.appendChild(meta);
    const interfaceHint = document.createElement("div");
    interfaceHint.className = "workflow-step-interface-hint";
    interfaceHint.textContent = "接口节点待检查";
    card.appendChild(interfaceHint);
    card.appendChild(chipList(step.evidenceKinds || [], "无 Evidence"));
    if (step.relatedMockTargets?.length) {
      card.appendChild(chipList(step.relatedMockTargets, "无 Mock"));
    }
    const actions = document.createElement("div");
    actions.className = "workflow-detail-step-actions";
    const stepLink = document.createElement("a");
    stepLink.className = "button-link";
    stepLink.href = workflowStepHref(workflow.id, step.id);
    stepLink.textContent = "查看步骤页";
    actions.appendChild(stepLink);
    card.appendChild(actions);
    list.appendChild(card);
  });
  return workflow;
}

async function refreshWorkflowDetail() {
  setDetailMessage("refreshing...");
  const catalog = await detailRequest("/api/catalog");
  const workflow = renderWorkflowDetail(catalog);
  if (workflow?.id) {
    const services = serviceById(catalog);
    try {
      const coverage = await detailRequest(`/api/interface-node/coverage?workflow=${encodeURIComponent(workflow.id)}`);
      renderWorkflowCoveragePanel(workflow, services, coverage);
      renderWorkflowStepInterfaceHints(workflow, coverage);
    } catch (error) {
      renderWorkflowCoveragePanel(workflow, services, { ok: false, error: error.message, summary: {}, rows: [] });
      renderWorkflowStepInterfaceHints(workflow, { rows: [] });
    }
  }
  setDetailMessage("ready");
}

async function runSelectedWorkflow() {
  const workflow = (await detailRequest("/api/catalog")).workflows?.find((item) => item.id === selectedWorkflowId());
  if (!workflow || !workflowRunSupported(workflow)) {
    setDetailMessage("workflow runner not connected");
    return;
  }
  if (!workflowRunEventsHref(workflow)) {
    const button = detailEl("runWorkflowBtn");
    button.disabled = true;
    try {
      await runGenericWorkflowInBrowser(workflow);
    } finally {
      button.disabled = !workflowRunSupported(workflow);
    }
    return;
  }
  const button = detailEl("runWorkflowBtn");
  button.disabled = true;
  workflowRunState = { ...emptyWorkflowRunState(workflow), status: "running" };
  renderWorkflowRunner(workflow);
  setDetailMessage("running...");
  try {
    let finalBody = null;
    await consumeWorkflowEventStream(workflowRunEventsHref(workflow), [
      "workflow-started",
      "step-started",
      "step-completed",
      "workflow-completed",
      "workflow-failed",
    ], ({ event, data }) => {
      workflowRunState = data;
      renderWorkflowRunner(workflow);
      if (event === "workflow-completed" || event === "workflow-failed") {
        finalBody = data;
      } else {
        const done = data.summary?.stepCount || 0;
        const total = data.summary?.expectedStepCount || workflow.steps?.length || 0;
        setDetailMessage(`running ${done}/${total}: ${data.currentStep?.title || data.currentStep?.stepId || "-"}`);
      }
    });
    if (finalBody) {
      workflowRunState = finalBody;
      renderWorkflowRunner(workflow);
      setDetailMessage(finalBody.ok ? "workflow completed" : "workflow failed");
    }
  } finally {
    button.disabled = !workflowRunSupported(workflow);
  }
}

detailEl("workflowSelector").addEventListener("change", (event) => {
  window.location.href = workflowDetailHref(event.target.value);
});
detailEl("runWorkflowBtn").addEventListener("click", () => runSelectedWorkflow().catch((error) => setDetailMessage(error.message)));

refreshWorkflowDetail().catch((error) => setDetailMessage(error.message));
