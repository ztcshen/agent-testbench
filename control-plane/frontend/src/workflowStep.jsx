import { useEffect, useMemo, useState } from "react";
import { createRoot } from "react-dom/client";
import { Chip, fetchJSON, queryParam, selectedStep, selectedWorkflow, serviceName, workflowIdFromURL } from "./workflowPagesCommon.jsx";

function WorkflowStepApp() {
  const [catalog, setCatalog] = useState(null);
  const [dashboard, setDashboard] = useState(null);
  const [message, setMessage] = useState("loading");
  const [workflowID, setWorkflowID] = useState(workflowIdFromURL());
  const [stepID, setStepID] = useState(queryParam("step"));
  const runID = queryParam("runId");

  async function refresh() {
    setMessage("loading");
    try {
      const [nextCatalog, nextDashboard] = await Promise.all([fetchJSON("/api/catalog"), fetchJSON("/api/dashboard")]);
      setCatalog(nextCatalog);
      setDashboard(nextDashboard);
      setMessage("ready");
    } catch (error) {
      setMessage(error.message);
    }
  }

  useEffect(() => {
    refresh();
  }, []);

  const workflow = selectedWorkflow(catalog, workflowID);
  const step = selectedStep(workflow, stepID);
  const steps = workflow?.steps || [];
  const foundIndex = steps.findIndex((item) => item.id === step?.id);
  const position = foundIndex >= 0 ? foundIndex : 0;
  const positionText = steps.length ? `${position + 1}/${steps.length}` : "0/0";
  const previous = steps[position - 1];
  const next = steps[position + 1];
  const services = catalog?.services || [];
  const runtime = useMemo(() => {
    const items = (dashboard?.groups || []).flatMap((group) => group.items || []);
    return items.find((item) => item.id === step?.serviceId);
  }, [dashboard, step]);

  return (
    <main className="app workflow-step-page workflow-step-compact-density" data-template-id="TPL-INTERFACE-STEP-DETAIL-V1">
      <div className="template-watermark" aria-label="模板编号">TPL-INTERFACE-STEP-DETAIL-V1</div>
      <section className="topbar workflow-step-topbar">
        <div>
          <h1>{step?.displayName || step?.id || "Workflow Step 详情"}</h1>
          <p>{workflow ? `${workflow.displayName || workflow.id} · ${positionText}` : "loading"}</p>
        </div>
        <div className="actions">
          <span className="workflow-step-status-pill" role="status">{message}</span>
          <a className="button-link" href="/">控制台</a>
          <a className="button-link" href="/workflows.html">Workflow 目录</a>
          <a className="button-link" href="/dashboard.html">环境大盘</a>
          <a className="button-link" href={`/workflow-detail.html?id=${encodeURIComponent(workflow?.id || "")}`}>返回 Workflow 定义</a>
        </div>
      </section>

      <section className="workflow-step-load-progress" aria-label="Workflow Step 加载进度" aria-live="polite">
        <div className="workflow-step-load-progress-head">
          <strong>{message === "ready" ? "已加载" : "准备加载"}</strong>
          <span>{message === "ready" ? "100%" : "0%"}</span>
        </div>
        <div className="workflow-step-load-progress-track" role="progressbar" aria-valuemin="0" aria-valuemax="100" aria-valuenow={message === "ready" ? 100 : 0}>
          <div className="workflow-step-load-progress-fill" style={{ width: message === "ready" ? "100%" : "0%" }} />
        </div>
      </section>

      <section className="workflow-step-layout">
        <aside className="workflow-step-side">
          <h2>定位</h2>
          <label className="workflow-detail-selector">
            <span>切换步骤</span>
            <select value={step?.id || ""} onChange={(event) => setStepID(event.target.value)}>
              {steps.map((item) => <option value={item.id} key={item.id}>{item.displayName || item.id}</option>)}
            </select>
          </label>
          <code>{step?.id || "-"}</code>
          <h2>Workflow</h2>
          <select value={workflow?.id || ""} onChange={(event) => setWorkflowID(event.target.value)}>
            {(catalog?.workflows || []).map((item) => <option value={item.id} key={item.id}>{item.displayName || item.id}</option>)}
          </select>
          <h2>运行证据</h2>
          <code>{runID || "未绑定 run"}</code>
          <h2>前后步骤</h2>
          <div className="workflow-step-nav">
            <a className={`button-link ${previous ? "" : "disabled-link"}`} href={previous ? `/workflow-step.html?workflow=${encodeURIComponent(workflow.id)}&step=${encodeURIComponent(previous.id)}` : "#"}>上一步</a>
            <a className={`button-link ${next ? "" : "disabled-link"}`} href={next ? `/workflow-step.html?workflow=${encodeURIComponent(workflow.id)}&step=${encodeURIComponent(next.id)}` : "#"}>下一步</a>
          </div>
        </aside>

        <section className="workflow-step-main">
          <section className="workflow-step-hero">
            <div>
              <span className="detail-phase">{serviceName(services, step?.serviceId)}</span>
              <h2>{step?.displayName || step?.id || "-"}</h2>
              <p>{[step?.action, runtime?.state, runtime?.health].filter(Boolean).join(" · ") || "-"}</p>
            </div>
            <code>{step?.caseId || "case"}</code>
          </section>
          <section className="workflow-step-grid">
            <article className="workflow-step-card"><span>Action</span><strong>{step?.action || "-"}</strong></article>
            <article className="workflow-step-card"><span>Evidence</span><div className="workflow-detail-chips"><Chip>{runtime?.message || "catalog"}</Chip></div></article>
            <article className="workflow-step-card"><span>Service</span><div className="workflow-detail-chips"><Chip>{step?.serviceId || "-"}</Chip></div></article>
          </section>
          <section className="workflow-step-detail-card">
            <div className="section-head">
              <h2>全步骤导航</h2>
              <span className="evidence-count">{positionText}</span>
            </div>
            <div className="workflow-step-sequence">
              {steps.length ? steps.map((item, index) => (
                <a className={item.id === step?.id ? "active" : ""} href={`/workflow-step.html?workflow=${encodeURIComponent(workflow?.id || "")}&step=${encodeURIComponent(item.id)}`} key={item.id}>
                  <span>{index + 1}</span>
                  <strong>{item.displayName || item.id}</strong>
                </a>
              )) : <p className="dashboard-empty">当前 Workflow 还没有声明步骤。</p>}
            </div>
          </section>
        </section>
      </section>
    </main>
  );
}

createRoot(document.getElementById("react-workflow-step-root")).render(<WorkflowStepApp />);
