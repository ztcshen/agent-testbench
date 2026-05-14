import { useEffect, useMemo, useState } from "react";
import { createRoot } from "react-dom/client";
import { fetchJSON, queryParam, serviceName, statusTone } from "./workflowPagesCommon.jsx";

function shortTime(value) {
  if (!value) return "-";
  const date = new Date(value);
  return Number.isNaN(date.getTime()) ? "-" : date.toLocaleString("zh-CN", { hour12: false });
}

function parseSummary(run) {
  if (run?.summary && typeof run.summary === "object") return run.summary;
  if (typeof run?.summaryJson === "string") {
    try {
      return JSON.parse(run.summaryJson);
    } catch {
      return {};
    }
  }
  return {};
}

function RunCard({ run, selected }) {
  return (
    <a className={`workflow-run-step ${selected ? "active" : ""} ${statusTone(run.status)}`} href={`/workflow-run.html?id=${encodeURIComponent(run.id || run.runId || "")}`}>
      <div>
        <strong>{run.workflowId || run.id || "-"}</strong>
        <span>{shortTime(run.updatedAt || run.createdAt)}</span>
      </div>
      <code>{run.status || "-"}</code>
    </a>
  );
}

function stepID(step, index) {
  return step.stepId || step.id || `step-${index + 1}`;
}

function stepStatus(step) {
  if (step.status) return step.status;
  if (step.stepOk === false || step.ok === false) return "failed";
  if (step.stepOk === true || step.ok === true) return "passed";
  return "unknown";
}

function traceTopologyHref(runID, step) {
  const params = new URLSearchParams();
  params.set("workflowRunId", runID || "");
  const traceFilter = step.stepId || step.summary?.requestId || "";
  if (traceFilter) params.set("traceFilter", traceFilter);
  return `/trace-topology.html?${params.toString()}`;
}

function workflowStepHref(run, step) {
  const params = new URLSearchParams();
  params.set("workflow", run?.workflowId || "");
  params.set("step", step.stepId || step.id || "");
  if (run?.id) params.set("runId", run.id);
  return `/workflow-step.html?${params.toString()}`;
}

function catalogStepFor(run, step, catalog) {
  const workflow = (catalog?.workflows || []).find((item) => item.id === run?.workflowId);
  return (workflow?.steps || []).find((item) => item.id === (step.stepId || step.id)) || null;
}

function stepServiceID(run, step, catalog) {
  return step.serviceId || step.summary?.serviceId || step.summary?.targetServiceId || catalogStepFor(run, step, catalog)?.serviceId || "";
}

function serviceHref(serviceID, catalog) {
  const service = (catalog?.services || []).find((item) => item.id === serviceID);
  if (service?.role === "external") return "/service-inventory.html";
  return `/environment-node.html?id=${encodeURIComponent(serviceID)}`;
}

function StepBodyHealth({ step }) {
  const bodyHealth = step.bodyHealth || {};
  const message = bodyHealth.message || "";
  const level = bodyHealth.level || (bodyHealth.ok === false ? "failed" : "ok");
  if (bodyHealth.ok !== false && !message) return null;
  return (
    <div className={`workflow-run-step-body-health ${bodyHealth.ok === false ? "failed" : "passed"}`}>
      <span>body health</span>
      <strong>{`${level}${message ? ` · ${message}` : ""}`}</strong>
    </div>
  );
}

function StepCard({ run, step, index, catalog }) {
  const id = stepID(step, index);
  const status = stepStatus(step);
  const serviceID = stepServiceID(run, step, catalog);
  const summary = step.summary || {};
  const line = [
    id,
    summary.httpCode ? `http ${summary.httpCode}` : "",
    summary.requestId || "",
    step.elapsedMs || summary.elapsedMs ? `${step.elapsedMs || summary.elapsedMs} ms` : "",
  ].filter(Boolean).join(" · ");
  return (
    <article className={`workflow-run-step-card ${statusTone(status)}`}>
      <div>
        <strong>{`${String(index + 1).padStart(2, "0")} ${step.title || id}`}</strong>
        <span className={`status-pill ${statusTone(status)}`}>{status}</span>
      </div>
      <p>{line || "-"}</p>
      <StepBodyHealth step={step} />
      <div className="workflow-run-step-service-links">
        {serviceID ? <a href={serviceHref(serviceID, catalog)}>{serviceName(catalog?.services, serviceID)}</a> : null}
        <a href={workflowStepHref(run, step)}>接口明细</a>
        {run?.id ? <a href={traceTopologyHref(run.id, step)}>过滤拓扑</a> : null}
      </div>
    </article>
  );
}

function WorkflowRunApp() {
  const [runs, setRuns] = useState(null);
  const [detail, setDetail] = useState(null);
  const [catalog, setCatalog] = useState(null);
  const [message, setMessage] = useState("loading");
  const requestedID = queryParam("id") || queryParam("runId");

  async function refresh() {
    setMessage("loading");
    try {
      const [payload, catalogPayload] = await Promise.all([fetchJSON("/api/runs"), fetchJSON("/api/catalog")]);
      setRuns(payload);
      setCatalog(catalogPayload);
      const selected = requestedID || payload.workflowRuns?.[0]?.id || "";
      if (selected) {
        try {
          setDetail(await fetchJSON(`/api/workflow-runs/${encodeURIComponent(selected)}`));
        } catch {
          setDetail(payload.workflowRuns?.find((run) => run.id === selected) || null);
        }
      }
      setMessage("ready");
    } catch (error) {
      setMessage(error.message);
    }
  }

  useEffect(() => {
    refresh();
  }, []);

  const workflowRuns = runs?.workflowRuns || [];
  const selectedRun = detail?.run || detail || workflowRuns[0] || null;
  const selectedID = selectedRun?.id || selectedRun?.runId || requestedID;
  const summary = useMemo(() => detail?.summary || parseSummary(selectedRun), [detail, selectedRun]);
  const steps = summary.steps || detail?.steps || [];
  const identifiers = Object.entries(summary.identifiers || summary.ids || {});

  return (
    <main className="app workflow-run-page" data-template-id="TPL-WORKFLOW-RUN-EVIDENCE-V1">
      <div className="template-watermark" aria-label="模板编号">TPL-WORKFLOW-RUN-EVIDENCE-V1</div>
      <section className="topbar">
        <div>
          <h1>Workflow run</h1>
          <p>{selectedRun ? `${selectedRun.workflowId || "-"} · ${selectedID}` : "run loading"}</p>
        </div>
        <div className="actions">
          <span className="workflow-step-status-pill" role="status">{message}</span>
          <a className="button-link" href="/">控制台</a>
          <a className="button-link" href="/dashboard.html">环境大盘</a>
        </div>
      </section>
      <section className="workflow-run-summary" aria-label="Workflow run summary">
        <article><span>Status</span><strong>{selectedRun?.status || "-"}</strong></article>
        <article><span>Workflow</span><strong>{selectedRun?.workflowId || "-"}</strong></article>
        <article><span>Runs</span><strong>{workflowRuns.length}</strong></article>
        <article><span>Updated</span><strong>{shortTime(selectedRun?.updatedAt || selectedRun?.createdAt)}</strong></article>
      </section>
      <section className="workflow-run-shell">
        <section className="workflow-run-panel workflow-run-steps-panel">
          <div className="section-head"><div><h2>Runs</h2><p>{`${workflowRuns.length} stored workflow runs`}</p></div></div>
          <div className="workflow-run-steps">
            {workflowRuns.length ? workflowRuns.map((run) => <RunCard run={run} selected={(run.id || run.runId) === selectedID} key={run.id || run.runId} />) : <div className="run-history-empty">暂无 Workflow run</div>}
          </div>
        </section>
        <section className="workflow-run-panel">
          <div className="section-head"><div><h2>Steps</h2><p>{`${steps.length || 0} step records`}</p></div></div>
          <div className="workflow-run-trace-topologies">
            {steps.length ? steps.map((step, index) => (
              <StepCard run={selectedRun} step={step} index={index} catalog={catalog} key={stepID(step, index)} />
            )) : <p className="dashboard-empty">当前 run 没有 step 摘要。</p>}
          </div>
        </section>
        <section className="workflow-run-panel">
          <div className="section-head"><div><h2>Identifiers</h2><p>{`${identifiers.length} identifiers`}</p></div></div>
          <div className="workflow-run-identifiers">
            {identifiers.length ? identifiers.map(([key, value]) => <code key={key}>{`${key}: ${value}`}</code>) : <p className="dashboard-empty">当前 run 没有关联标识。</p>}
          </div>
        </section>
      </section>
    </main>
  );
}

createRoot(document.getElementById("react-workflow-run-root")).render(<WorkflowRunApp />);
