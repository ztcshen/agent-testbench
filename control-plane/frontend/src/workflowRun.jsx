import { useEffect, useMemo, useState } from "react";
import { createRoot } from "react-dom/client";
import { fetchJSON, queryParam, statusTone } from "./workflowPagesCommon.jsx";

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

function WorkflowRunApp() {
  const [runs, setRuns] = useState(null);
  const [detail, setDetail] = useState(null);
  const [message, setMessage] = useState("loading");
  const requestedID = queryParam("id") || queryParam("runId");

  async function refresh() {
    setMessage("loading");
    try {
      const payload = await fetchJSON("/api/runs");
      setRuns(payload);
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
  const summary = useMemo(() => parseSummary(selectedRun), [selectedRun]);
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
              <article className="workflow-run-trace-card" key={step.stepId || step.id || index}>
                <strong>{step.stepId || step.id || `step-${index + 1}`}</strong>
                <span>{step.status || (step.ok ? "passed" : "unknown")}</span>
              </article>
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
