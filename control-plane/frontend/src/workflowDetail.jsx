import { useEffect, useMemo, useState } from "react";
import { createRoot } from "react-dom/client";
import { Chip, fetchJSON, selectedWorkflow, serviceName, workflowIdFromURL } from "./workflowPagesCommon.jsx";

function serviceIds(workflow) {
  return [...new Set((workflow?.steps || []).map((step) => step.serviceId).filter(Boolean))];
}

async function postJSON(path, payload) {
  const response = await fetch(path, {
    method: "POST",
    headers: { "Content-Type": "application/json", Accept: "application/json" },
    body: JSON.stringify(payload),
  });
  const body = await response.json();
  if (!response.ok) throw new Error(body.error || response.statusText);
  return body;
}

function runStatusTone(status) {
  const value = String(status || "").toLowerCase();
  if (["passed", "success", "ok"].includes(value)) return "passed";
  if (["failed", "error"].includes(value)) return "failed";
  return "idle";
}

function resultOK(result) {
  return Boolean(result?.ok) && result?.bodyHealth?.ok !== false;
}

function resultBodyHealth(result) {
  if (result?.bodyHealth) return result.bodyHealth;
  const reason = result?.error || result?.summary?.failureReason || "";
  return { ok: Boolean(result?.ok), level: result?.ok ? "ok" : "failed", message: result?.ok ? "" : reason || "case failed" };
}

function unsupportedStepResult(step, dryRun) {
  const ok = Boolean(dryRun);
  return {
    ok,
    stepOk: ok,
    status: ok ? "passed" : "failed",
    dryRun,
    caseId: step.caseId || "",
    stepId: step.id,
    title: step.displayName || step.id,
    elapsedMs: 0,
    summary: { dryRun, failureReason: ok ? "" : "caseId is required" },
    bodyHealth: { ok, level: ok ? "ok" : "failed", message: ok ? "" : "caseId is required" },
  };
}

function workflowRunSnapshot(workflow, steps, startedAt, done, dryRun) {
  const passed = steps.filter(resultOK).length;
  const ok = done && steps.length === (workflow?.steps || []).length && passed === steps.length;
  return {
    workflowId: workflow?.id || "",
    status: done ? (ok ? "passed" : "failed") : "running",
    ok,
    dryRun,
    elapsedMs: Date.now() - startedAt,
    summary: { expectedStepCount: workflow?.steps?.length || 0, stepCount: steps.length, passed },
    steps,
  };
}

function coverageNumber(summary, key) {
  const value = summary?.[key];
  return Number.isFinite(value) ? value : 0;
}

function CoverageCard({ title, value, detail }) {
  return (
    <article className="workflow-coverage-card">
      <strong>{title}</strong>
      <code>{value}</code>
      <span>{detail}</span>
    </article>
  );
}

function InterfaceCoverageRow({ row }) {
  return (
    <article className={`workflow-interface-coverage-row ${row.mapped ? "mapped" : "unmapped"}`}>
      <div className="workflow-interface-coverage-title">
        <strong>{row.stepId || "-"}</strong>
        <code>{row.caseDisplayName || row.caseId || "no case"}</code>
      </div>
      <div className="workflow-interface-coverage-state">
        <span className={`status-pill ${row.mapped ? "passed" : "failed"}`}>{row.mapped ? "mapped" : "gap"}</span>
        <code>{row.admissionStatus || "pending"}</code>
      </div>
      <div className="workflow-interface-coverage-target">
        {row.href ? <a className="button-link" href={row.href}>{row.nodeDisplayName || row.nodeId}</a> : <span>未映射接口节点</span>}
      </div>
    </article>
  );
}

function WorkflowCoverage({ workflow, coverage }) {
  const summary = coverage?.summary || {};
  const rows = coverage?.rows || [];
  return (
    <section className="workflow-coverage-panel">
      <div className="section-head">
        <div>
          <h2>接口覆盖</h2>
          <p>{workflow ? `${workflow.id} · ${coverageNumber(summary, "mappedSteps")}/${coverageNumber(summary, "totalSteps")} mapped` : "loading"}</p>
        </div>
        {workflow?.id ? <a className="button-link" href={`/api/interface-node/coverage-gaps?workflow=${encodeURIComponent(workflow.id)}`}>覆盖缺口 JSON</a> : null}
      </div>
      <div className="workflow-coverage-grid">
        <CoverageCard title="total steps" value={coverageNumber(summary, "totalSteps")} detail="workflow bindings" />
        <CoverageCard title="mapped" value={coverageNumber(summary, "mappedSteps")} detail="interface nodes" />
        <CoverageCard title="unmapped" value={coverageNumber(summary, "unmappedSteps")} detail="coverage gaps" />
        <CoverageCard title="pending" value={coverageNumber(summary, "pendingNodes")} detail="admission state" />
      </div>
      <section className="workflow-interface-coverage">
        <h3>Step interface map</h3>
        <div className="workflow-interface-coverage-list">
          {rows.length ? rows.map((row) => <InterfaceCoverageRow row={row} key={`${row.workflowId}-${row.stepId}`} />) : <p className="dashboard-empty">当前 Workflow 没有接口覆盖记录。</p>}
        </div>
      </section>
    </section>
  );
}

function WorkflowGraph({ workflow, services }) {
  const steps = workflow?.steps || [];
  return (
    <div className="workflow-graph-panel" aria-label="Workflow 链路">
      <div className="workflow-graph-nodes">
        {steps.length ? steps.map((step, index) => (
          <a className="workflow-graph-node service" href={`/workflow-step.html?workflow=${encodeURIComponent(workflow.id)}&step=${encodeURIComponent(step.id)}`} key={step.id}>
            <strong>{step.displayName || step.id}</strong>
            <span>{serviceName(services, step.serviceId)}</span>
            <code>{index + 1}</code>
          </a>
        )) : <p className="dashboard-empty">当前 Workflow 还没有声明步骤。</p>}
      </div>
      <div className="workflow-graph-edges">
        {steps.length > 1 ? steps.slice(1).map((step, index) => (
          <article className="workflow-graph-edge" key={`${steps[index].id}-${step.id}`}>
            <strong>{steps[index].displayName || steps[index].id}</strong>
            <span>{"->"}</span>
            <strong>{step.displayName || step.id}</strong>
          </article>
        )) : <p className="dashboard-empty">需要两个以上步骤才会生成链路边。</p>}
      </div>
    </div>
  );
}

function StepList({ workflow, services }) {
  const steps = workflow?.steps || [];
  return (
    <div className="workflow-detail-steps">
      {steps.length ? steps.map((step, index) => (
        <article className="workflow-detail-step" key={step.id}>
          <div className="workflow-detail-step-top">
            <span>{String(index + 1).padStart(2, "0")}</span>
            <strong>{step.displayName || step.id}</strong>
            <code>{step.required ? "required" : "optional"}</code>
          </div>
          <p>{[serviceName(services, step.serviceId), step.action, step.caseId].filter(Boolean).join(" · ")}</p>
          <div className="workflow-detail-chips">
            <Chip>{step.id}</Chip>
            {step.caseId ? <Chip>{step.caseId}</Chip> : null}
            {step.serviceId ? <Chip>{step.serviceId}</Chip> : null}
          </div>
          <a className="button-link" href={`/workflow-step.html?workflow=${encodeURIComponent(workflow.id)}&step=${encodeURIComponent(step.id)}`}>查看 Step</a>
        </article>
      )) : <p className="dashboard-empty">当前 Workflow 还没有可查看的 Step。</p>}
    </div>
  );
}

function WorkflowRunner({ workflow, state, dryRun, onDryRunChange, onRun }) {
  const steps = workflow?.steps || [];
  const results = state.steps || [];
  const resultByStep = new Map(results.map((item) => [item.stepId, item]));
  const percent = steps.length ? Math.round((results.length / steps.length) * 100) : 0;
  const running = state.status === "running";
  return (
    <section className="workflow-progress" aria-label="Workflow runner">
      <div className="workflow-progress-head">
        <span>{`${results.length} / ${steps.length || 0}`}</span>
        <strong>{state.message || "等待运行"}</strong>
      </div>
      <div className="workflow-progress-track" aria-hidden="true"><div className="workflow-progress-fill" style={{ width: `${percent}%` }} /></div>
      <div className="workflow-progress-steps">
        {steps.length ? steps.map((step, index) => {
          const result = resultByStep.get(step.id);
          const tone = result ? (resultOK(result) ? "passed" : "failed") : running && results.length === index ? "running" : "";
          return (
            <article className={`workflow-progress-step ${tone}`} key={step.id}>
              <span className="workflow-progress-index">{String(index + 1).padStart(2, "0")}</span>
              <strong className="workflow-progress-title">{step.displayName || step.id}</strong>
            </article>
          );
        }) : <p className="dashboard-empty">当前 Workflow 没有可运行步骤。</p>}
      </div>
      <div className="actions">
        <label className="workflow-detail-selector">
          <span>Dry run</span>
          <input type="checkbox" checked={dryRun} onChange={(event) => onDryRunChange(event.target.checked)} />
        </label>
        <button className="primary-action" type="button" disabled={running || !steps.length} onClick={onRun}>{running ? "运行中" : "运行 Workflow"}</button>
        {state.runId ? <a className="button-link" href={`/workflow-run.html?id=${encodeURIComponent(state.runId)}`}>查看运行记录</a> : null}
      </div>
    </section>
  );
}

function WorkflowDetailApp() {
  const [catalog, setCatalog] = useState(null);
  const [coverage, setCoverage] = useState(null);
  const [dryRun, setDryRun] = useState(true);
  const [runner, setRunner] = useState({ status: "idle", steps: [], message: "等待运行" });
  const [message, setMessage] = useState("loading");
  const [workflowID, setWorkflowID] = useState(workflowIdFromURL());

  async function refresh() {
    setMessage("loading");
    try {
      setCatalog(await fetchJSON("/api/catalog"));
      setMessage("ready");
    } catch (error) {
      setMessage(error.message);
    }
  }

  useEffect(() => {
    refresh();
  }, []);

  const workflows = catalog?.workflows || [];
  const workflow = selectedWorkflow(catalog, workflowID);
  const services = catalog?.services || [];
  const covered = useMemo(() => serviceIds(workflow), [workflow]);
  const warnings = catalog?.warnings || [];
  const latestRun = workflow?.latestRun || null;
  const latestStatus = latestRun?.status || (workflow?.runCount ? "unknown" : "no run");

  useEffect(() => {
    if (!workflow?.id) {
      setCoverage(null);
      return;
    }
    let cancelled = false;
    fetchJSON(`/api/interface-node/coverage?workflow=${encodeURIComponent(workflow.id)}`)
      .then((payload) => {
        if (!cancelled) setCoverage(payload);
      })
      .catch((error) => {
        if (!cancelled) setCoverage({ ok: false, error: error.message, rows: [], summary: {} });
      });
    return () => {
      cancelled = true;
    };
  }, [workflow?.id]);

  async function runWorkflow() {
    if (!workflow?.id || runner.status === "running") return;
    const startedAt = Date.now();
    const results = [];
    setRunner({ status: "running", steps: [], message: "starting" });
    try {
      for (const step of workflow.steps || []) {
        setRunner({ status: "running", steps: [...results], message: `running ${step.displayName || step.id}` });
        const result = step.caseId
          ? await postJSON("/api/test-kit/run", { caseId: step.caseId, workflowId: workflow.id, stepId: step.id, dryRun, timeoutSeconds: 120 })
          : unsupportedStepResult(step, dryRun);
        const withStep = {
          ...result,
          stepId: step.id,
          title: step.displayName || step.id,
          bodyHealth: resultBodyHealth(result),
          stepOk: resultOK({ ...result, bodyHealth: resultBodyHealth(result) }),
        };
        results.push(withStep);
        setRunner({ status: "running", steps: [...results], message: `completed ${results.length}/${workflow.steps?.length || 0}` });
        if (!resultOK(withStep)) break;
      }
      const snapshot = workflowRunSnapshot(workflow, results, startedAt, true, dryRun);
      const saved = results.length ? await postJSON("/api/workflow-runs", snapshot) : {};
      setRunner({ status: snapshot.status, steps: results, runId: saved.workflowRunId || "", message: snapshot.ok ? "workflow completed" : "workflow failed" });
      refresh();
    } catch (error) {
      setRunner({ status: "failed", steps: results, message: error.message });
    }
  }

  return (
    <main className="app workflow-detail-page workflow-detail-compact-density" data-template-id="TPL-WORKFLOW-LONG-CHAIN-V1">
      <div className="template-watermark" aria-label="模板编号">TPL-WORKFLOW-LONG-CHAIN-V1</div>
      <section className="topbar">
        <div>
          <h1>{workflow?.displayName || workflow?.id || "Workflow 定义"}</h1>
          <p>{workflow ? `${workflow.steps?.length || 0} steps · ${covered.length} services` : "loading"}</p>
        </div>
        <div className="actions">
          <span className="workflow-detail-status-pill" role="status">{message}</span>
          <a className="button-link" href="/">控制台</a>
          <a className="button-link" href="/workflows.html">Workflow 目录</a>
          <a className="button-link" href="/dashboard.html">环境大盘</a>
          <a className="primary-action" href={`/workflow-run.html?workflow=${encodeURIComponent(workflow?.id || "")}`}>运行记录</a>
        </div>
      </section>

      <section className="workflow-run-template" aria-label="Workflow run template">
        <div className="workflow-run-template-head">
          <article><span>workflow</span><strong>{workflow?.id || "-"}</strong></article>
          <article><span>steps</span><strong>{workflow?.steps?.length || 0}</strong></article>
          <article><span>runs</span><strong>{workflow?.runCount || 0}</strong></article>
          <article><span>status</span><strong className={`status-pill ${runStatusTone(latestStatus)}`}>{latestStatus}</strong></article>
          <article><span>source</span><strong>{catalog?.source?.kind || "-"}</strong></article>
        </div>
        <WorkflowRunner workflow={workflow} state={runner} dryRun={dryRun} onDryRunChange={setDryRun} onRun={runWorkflow} />
      </section>

      <section className="workflow-detail-layout">
        <aside className="workflow-detail-side">
          <h2>定义来源</h2>
          <p>{catalog?.source?.kind || "loading"}</p>
          {warnings.length ? <div className="workflow-detail-warning">{warnings.join(" · ")}</div> : null}
          <h2>Workflow</h2>
          <label className="workflow-detail-selector">
            <span>切换 Workflow</span>
            <select value={workflow?.id || ""} onChange={(event) => setWorkflowID(event.target.value)}>
              {workflows.map((item) => <option value={item.id} key={item.id}>{item.displayName || item.id}</option>)}
            </select>
          </label>
          <code>{workflow?.id || "-"}</code>
          <h2>模板入口</h2>
          <code>{workflow?.entrypoint || "-"}</code>
          <h2>服务覆盖</h2>
          <div className="workflow-service-summary">
            {covered.map((serviceId) => <Chip key={serviceId}>{serviceName(services, serviceId)}</Chip>)}
          </div>
        </aside>
        <section className="workflow-detail-main">
          <WorkflowGraph workflow={workflow} services={services} />
          <WorkflowCoverage workflow={workflow} coverage={coverage} />
          <div className="section-head">
            <div>
              <h2>步骤</h2>
              <p>{workflow ? `${workflow.steps?.length || 0} steps` : "loading"}</p>
            </div>
          </div>
          <StepList workflow={workflow} services={services} />
        </section>
      </section>
    </main>
  );
}

createRoot(document.getElementById("react-workflow-detail-root")).render(<WorkflowDetailApp />);
