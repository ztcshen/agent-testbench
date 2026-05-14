import { createRoot } from "react-dom/client";
import { useEffect, useMemo, useState } from "react";
import { fetchJSON } from "./api.js";
import "./workflowBlueprintDemo.css";

const blankWorkflow = {
  id: "draft.workflow",
  name: "新建工作流",
};

function workflowFromCatalog(workflow) {
  const steps = (workflow?.steps || []).map((step, index) => ({
    id: step.id || `step-${index + 1}`,
    name: step.displayName || step.id || `Step ${index + 1}`,
    service: step.serviceId || "",
    caseId: step.caseId || "",
    x: 120 + index * 250,
    y: 110 + (index % 2) * 90,
  }));
  return {
    id: workflow?.id || blankWorkflow.id,
    name: workflow?.displayName || workflow?.id || blankWorkflow.name,
    steps,
  };
}

function workflowJSON(workflow) {
  return {
    schemaVersion: "workflow-blueprint/v1",
    workflowId: workflow.id,
    workflowName: workflow.name,
    nodes: workflow.steps.map((step) => ({
      id: step.id,
      name: step.name,
      service: step.service,
      caseId: step.caseId,
      position: { x: step.x, y: step.y },
    })),
    edges: workflow.steps.slice(1).map((step, index) => ({
      id: `edge-${workflow.steps[index].id}-${step.id}`,
      from: workflow.steps[index].id,
      to: step.id,
      kind: "exec",
    })),
  };
}

function BlueprintApp() {
  const root = document.getElementById("react-workflow-blueprint-demo-root");
  const isNew = root?.dataset?.blueprintMode === "new";
  const templateId = root?.dataset?.templateId || "TPL-WORKFLOW-BLUEPRINT-DEMO-V1";
  const [workflow, setWorkflow] = useState({ ...blankWorkflow, steps: [] });
  const [status, setStatus] = useState(isNew ? "blank draft" : "loading");
  const [selectedId, setSelectedId] = useState("");

  async function loadCatalogWorkflow() {
    if (isNew) {
      setStatus("blank draft");
      return;
    }
    try {
      const catalog = await fetchJSON("/api/catalog");
      const requested = new URLSearchParams(window.location.search).get("workflow") || "";
      const source = (catalog.workflows || []).find((item) => item.id === requested) || (catalog.workflows || [])[0];
      const next = workflowFromCatalog(source);
      setWorkflow(next);
      setSelectedId(next.steps[0]?.id || "");
      setStatus(source ? "catalog loaded" : "blank draft");
    } catch (error) {
      setStatus(error.message);
    }
  }

  useEffect(() => {
    loadCatalogWorkflow();
  }, []);

  const selected = workflow.steps.find((step) => step.id === selectedId) || null;
  const preview = useMemo(() => workflowJSON(workflow), [workflow]);

  function addPlaceholder() {
    const index = workflow.steps.length + 1;
    const step = {
      id: `step-${index}`,
      name: `接口节点 ${index}`,
      service: "",
      caseId: "",
      x: 120 + (index - 1) * 230,
      y: 120 + ((index - 1) % 2) * 90,
    };
    setWorkflow((current) => ({ ...current, steps: [...current.steps, step] }));
    setSelectedId(step.id);
    setStatus("draft updated");
  }

  function removeSelected() {
    if (!selectedId) return;
    setWorkflow((current) => ({ ...current, steps: current.steps.filter((step) => step.id !== selectedId) }));
    setSelectedId("");
    setStatus("draft updated");
  }

  function updateWorkflow(field, value) {
    setWorkflow((current) => ({ ...current, [field]: value }));
  }

  function updateSelected(field, value) {
    setWorkflow((current) => ({
      ...current,
      steps: current.steps.map((step) => (step.id === selectedId ? { ...step, [field]: value } : step)),
    }));
  }

  return (
    <main className={`blueprint-demo-shell ${isNew ? "blueprint-new-shell" : ""}`}>
      <div className="template-watermark" aria-label="模板编号">
        {templateId}
      </div>
      <header className="blueprint-demo-topbar">
        <div>
          <span>api workflow blueprint</span>
          <h1>{isNew ? "新建接口工作流" : "接口工作流蓝图"}</h1>
          <p>{workflow.name}</p>
        </div>
        <nav>
          <button type="button" onClick={addPlaceholder}>
            新增接口节点
          </button>
          <button type="button" onClick={removeSelected} disabled={!selectedId}>
            删除选中
          </button>
          {!isNew ? (
            <button type="button" onClick={loadCatalogWorkflow}>
              载入 Catalog
            </button>
          ) : null}
          <a href="/">控制台</a>
          <a href="/workflows.html">Workflow 目录</a>
        </nav>
      </header>

      <section className="blueprint-workflow-fields" aria-label="工作流元数据">
        <label>
          <span>workflowId</span>
          <input value={workflow.id} onChange={(event) => updateWorkflow("id", event.target.value)} />
        </label>
        <label>
          <span>workflowName</span>
          <input value={workflow.name} onChange={(event) => updateWorkflow("name", event.target.value)} />
        </label>
        <strong>{status}</strong>
      </section>

      <section className="blueprint-demo-grid">
        <aside className="blueprint-template-list" aria-label="接口节点库">
          <div className="blueprint-panel-head">
            <span>节点库</span>
            <strong>{workflow.steps.length}</strong>
          </div>
          <button className="blueprint-template-button blueprint-placeholder-button" type="button" onClick={addPlaceholder}>
            <span>SKETCH</span>
            <strong>接口占位</strong>
            <em>先画出步骤，再绑定服务和 Case。</em>
          </button>
          {workflow.steps.map((step) => (
            <button
              className={`blueprint-template-button ${selectedId === step.id ? "selected" : ""}`}
              key={step.id}
              type="button"
              onClick={() => setSelectedId(step.id)}
            >
              <span>{step.id}</span>
              <strong>{step.name}</strong>
              <em>{step.service || "未绑定服务"}</em>
            </button>
          ))}
        </aside>

        <section className="blueprint-canvas-shell" aria-label="工作流画布">
          {!workflow.steps.length ? (
            <div className="blueprint-empty-canvas">
              <strong>空白画布</strong>
              <span>从节点库添加接口节点，或载入已登记 Workflow。</span>
            </div>
          ) : (
            <div className="blueprint-node-map">
              {workflow.steps.map((step, index) => (
                <article className={`blueprint-node ${selectedId === step.id ? "selected" : ""}`} key={step.id}>
                  <span>{`#${index + 1}`}</span>
                  <strong>{step.name}</strong>
                  <em>{step.service || "service pending"}</em>
                </article>
              ))}
            </div>
          )}
        </section>

        <aside className={`blueprint-config-panel ${selected ? "is-open" : ""}`} aria-label="配置面板">
          <div className="blueprint-panel-head">
            <span>接口配置</span>
            <strong>{selected?.id || "none"}</strong>
          </div>
          {selected ? (
            <section className="blueprint-form">
              {["name", "service", "caseId"].map((field) => (
                <label key={field}>
                  <span>{field}</span>
                  <input value={selected[field] || ""} onChange={(event) => updateSelected(field, event.target.value)} />
                </label>
              ))}
            </section>
          ) : (
            <div className="blueprint-empty">No selection</div>
          )}
        </aside>
      </section>

      <section className={`blueprint-bottom ${workflow.steps.length ? "has-nodes" : ""}`}>
        <div className="blueprint-validation">
          <div className="blueprint-panel-head">
            <span>连线校验</span>
            <strong>{workflow.steps.length ? "valid" : "draft"}</strong>
          </div>
          <ul>
            <li>{workflow.steps.length ? "exec order is represented by node order" : "add at least one interface node"}</li>
          </ul>
        </div>
        <div className="blueprint-json-preview">
          <div className="blueprint-panel-head">
            <span>工作流 JSON</span>
            <strong>{`${preview.nodes.length} nodes · ${preview.edges.length} edges`}</strong>
          </div>
          <pre>{JSON.stringify(preview, null, 2)}</pre>
        </div>
      </section>
    </main>
  );
}

createRoot(document.getElementById("react-workflow-blueprint-demo-root")).render(<BlueprintApp />);
