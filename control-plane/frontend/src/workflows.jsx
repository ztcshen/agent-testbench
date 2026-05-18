import React, { useEffect, useMemo, useState } from "react";
import { createRoot } from "react-dom/client";
import "./control-plane-react.css";
import { fetchJSON } from "./api.js";
import { buildWorkflowDiscovery, workflowDiscoveryRow } from "./workflowDiscoveryModel.mjs";
import { dashboardStatusById, filterWorkflows, workflowKind, workflowRuntimeImpact } from "./workflowModel.js";
import { ButtonLink, Hero, IconButton, Icons, Panel, Shell, WorkflowCard } from "./components.jsx";

function targetConfig(catalog) {
  const params = new URLSearchParams(window.location.search);
  const configuredTarget = catalog?.presentation?.workflowFinder || catalog?.presentation?.targetWorkflow || {};
  return {
    targetStepCount: Number(params.get("targetStepCount") || configuredTarget.targetStepCount || configuredTarget.stepCount || 0),
    targetInterfaceCount: Number(params.get("targetInterfaceCount") || configuredTarget.targetInterfaceCount || configuredTarget.interfaceCount || 0),
    targetLabel: params.get("targetLabel") || configuredTarget.targetLabel || configuredTarget.label || "Configured workflow target",
  };
}

function WorkflowDiscoveryBadge({ workflow }) {
  const row = workflowDiscoveryRow(workflow);
  return (
    <div className="workflow-discovery-badge">
      <span>{`${row.stepCount} steps`}</span>
      <span>{`${row.interfaceCount} interfaces`}</span>
      <span>{`${row.caseCount} cases`}</span>
    </div>
  );
}

function WorkflowSection({ title, summary, workflows, services, statusById, onRuntimeImpactClick }) {
  if (!workflows.length) return null;
  return (
    <section className="react-section">
      <div className="react-section-head">
        <div>
          <h3>{title}</h3>
          <p>{summary}</p>
        </div>
        <span className="react-pill">{workflows.length} entries</span>
      </div>
      <div className="react-workflow-list">
        {workflows.map((workflow) => (
          <div className="workflow-discovery-card" key={workflow.id}>
            <WorkflowDiscoveryBadge workflow={workflow} />
            <WorkflowCard
              workflow={workflow}
              services={services}
              runtimeImpact={workflowRuntimeImpact(workflow, statusById)}
              onRuntimeImpactClick={onRuntimeImpactClick}
            />
          </div>
        ))}
      </div>
    </section>
  );
}

function TargetWorkflowFocus({ discovery, onFocus }) {
  const { target, targetWorkflows, targetChecklist } = discovery;
  const checklistByWorkflow = new Map((targetChecklist || []).map((item) => [item.workflowId, item]));
  return (
    <Panel
      title={target.label}
      label="Configured finder"
      summary={target.enabled ? `${target.stepCount} configured steps · ${target.interfaceCount} configured interfaces` : "Set targetStepCount and targetInterfaceCount in Store presentation or URL parameters."}
      action={target.enabled ? <IconButton icon={Icons.Search} title="Show configured target" onClick={() => onFocus(String(target.stepCount))}>Show target</IconButton> : null}
    >
      <div className="target-workflow-list">
        {targetWorkflows.length ? targetWorkflows.map((row) => (
          <article className="target-workflow-item" key={row.id}>
            <div className="target-workflow-head">
              <div>
                <strong>{row.title || row.id}</strong>
                <p>{row.coverageLabel}</p>
              </div>
              <TargetChecklistSummary checklist={checklistByWorkflow.get(row.id)} />
            </div>
            <div className="target-interface-strip">
              {row.stepLabels.map((step) => (
                <a href={`/workflow-step.html?workflow=${encodeURIComponent(row.id)}&step=${encodeURIComponent(step.id)}`} key={step.id}>
                  <span>{String(step.index).padStart(2, "0")}</span>
                  <strong>{step.interfaceId || step.id}</strong>
                  <code>{step.caseId || "no case"}</code>
                </a>
              ))}
            </div>
            <TargetChecklistTable checklist={checklistByWorkflow.get(row.id)} />
            <div className="react-card-actions">
              <ButtonLink href={`/workflow-detail.html?id=${encodeURIComponent(row.id)}`}>View workflow detail</ButtonLink>
              <ButtonLink href={`/api-cases.html?workflow=${encodeURIComponent(row.id)}`} primary icon={Icons.ArrowUpRight}>查看接口用例</ButtonLink>
            </div>
          </article>
        )) : (
          <div className="react-empty">No workflow matches the configured step/interface target.</div>
        )}
      </div>
    </Panel>
  );
}

function TargetChecklistSummary({ checklist }) {
  if (!checklist) return null;
  const { summary } = checklist;
  const blocked = summary.missingInterface + summary.missingCase;
  return (
    <div className="target-checklist-summary">
      <span>{`${summary.ready}/${summary.total} ready`}</span>
      <span className={blocked ? "warn" : "good"}>{blocked ? `${blocked} gaps` : "complete"}</span>
    </div>
  );
}

function TargetChecklistTable({ checklist }) {
  if (!checklist?.rows?.length) return null;
  return (
    <div className="target-checklist-table" role="table" aria-label="Configured workflow target checklist">
      <div className="target-checklist-row target-checklist-head" role="row">
        <span role="columnheader">Step</span>
        <span role="columnheader">Interface</span>
        <span role="columnheader">Case</span>
        <span role="columnheader">State</span>
        <span role="columnheader">Actions</span>
      </div>
      {checklist.rows.map((row) => (
        <div className={`target-checklist-row ${row.status}`} role="row" key={`${row.sequence}-${row.stepId}`}>
          <a href={row.stepHref} role="cell">{`${String(row.sequence).padStart(2, "0")} ${row.title || row.stepId}`}</a>
          <span role="cell">{row.interfaceHref ? <a href={row.interfaceHref}>{row.interfaceId}</a> : "missing"}</span>
          <span role="cell">{row.caseHref ? <a href={row.caseHref}>{row.caseId}</a> : "missing"}</span>
          <span role="cell"><span className={`target-state-pill ${row.status}`}>{row.status}</span></span>
          <span role="cell">{row.runsHref ? <a href={row.runsHref}>Runs</a> : "-"}</span>
        </div>
      ))}
    </div>
  );
}

function WorkflowCatalogStudio() {
  const [catalog, setCatalog] = useState(null);
  const [dashboard, setDashboard] = useState(null);
  const [query, setQuery] = useState(new URLSearchParams(window.location.search).get("workflowFilter") || "");
  const [message, setMessage] = useState("loading");
  const [error, setError] = useState("");

  async function refresh() {
    setMessage("loading");
    setError("");
    try {
      const [nextCatalog, nextDashboard] = await Promise.all([
        fetchJSON("/api/catalog"),
        fetchJSON("/api/dashboard"),
      ]);
      setCatalog(nextCatalog);
      setDashboard(nextDashboard);
      setMessage("ready");
    } catch (refreshError) {
      setError(refreshError.message);
      setMessage("failed");
    }
  }

  useEffect(() => {
    refresh();
  }, []);

  const workflows = catalog?.workflows || [];
  const services = catalog?.services || [];
  const statusById = useMemo(() => dashboardStatusById(dashboard), [dashboard]);
  const finderTarget = useMemo(() => targetConfig(catalog), [catalog]);
  const discovery = useMemo(() => buildWorkflowDiscovery(workflows, { query, ...finderTarget }), [workflows, query, finderTarget]);
  const visible = useMemo(() => filterWorkflows(workflows, services, query, statusById), [workflows, services, query, statusById]);
  const businessFlows = visible.filter((workflow) => workflowKind(workflow) === "businessFlow");
  const toolEntries = visible.filter((workflow) => workflowKind(workflow) !== "businessFlow");
  const applyFilter = (value) => setQuery(value || "");

  return (
    <Shell>
      <Hero
        kicker="React Catalog Studio"
        title="Workflow 清单"
        summary={query ? `${visible.length}/${workflows.length} 个匹配入口` : `${businessFlows.length} 个业务流 · ${toolEntries.length} 个观测/工具入口`}
        actions={
          <>
            <span className="react-status">{message}</span>
            <ButtonLink href="/" icon={Icons.LayoutDashboard}>控制台</ButtonLink>
            <ButtonLink href="/dashboard.html" icon={Icons.Gauge}>环境大盘</ButtonLink>
            <ButtonLink href="/service-inventory.html" icon={Icons.Boxes}>服务清单</ButtonLink>
          </>
        }
        stats={[
          { label: "Business", value: businessFlows.length },
          { label: "Target", value: discovery.summary.targetExact },
          { label: "Max steps", value: discovery.summary.maxStepCount },
          { label: "Interfaces", value: discovery.summary.maxInterfaceCount },
        ]}
      />

      {error ? <div className="react-error">{error}</div> : null}

      <TargetWorkflowFocus discovery={discovery} onFocus={applyFilter} />

      <Panel
        title="Catalog routing"
        label="Workflow map"
        summary="业务流使用 Workflow Studio；平台配置、服务健康、Replay/Probe 保留为控制面工具入口。"
        action={
          <div className="react-toolbar">
            <Icons.Search size={16} aria-hidden="true" />
            <input className="react-input" value={query} onChange={(event) => setQuery(event.target.value)} placeholder="搜索 Workflow / 服务 / Step" />
            <IconButton icon={Icons.X} title="清除筛选" onClick={() => applyFilter("")}>
              清除
            </IconButton>
          </div>
        }
      >
        {visible.length ? (
          <>
            <WorkflowSection
              title="业务流 Workflow"
              summary="可运行的端到端业务链路，适合进入 Workflow Studio。"
              workflows={businessFlows}
              services={services}
              statusById={statusById}
              onRuntimeImpactClick={applyFilter}
            />
            <WorkflowSection
              title="观测/工具入口"
              summary="平台配置、服务健康和 Replay/Probe 等控制面入口，不作为业务流模版展示。"
              workflows={toolEntries}
              services={services}
              statusById={statusById}
              onRuntimeImpactClick={applyFilter}
            />
          </>
        ) : (
          <div className="react-empty">没有匹配的 Workflow。</div>
        )}
      </Panel>
    </Shell>
  );
}

createRoot(document.getElementById("react-workflows-root")).render(<WorkflowCatalogStudio />);
