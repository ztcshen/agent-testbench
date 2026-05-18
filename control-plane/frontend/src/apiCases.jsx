import { useEffect, useMemo, useState } from "react";
import { createRoot } from "react-dom/client";
import { buildCaseCoverageBoard, buildCaseManagement, buildWorkflowCaseContext } from "./apiCasesModel.mjs";

async function requestJSON(path, options = undefined) {
  const response = await fetch(path, {
    headers: { Accept: "application/json", ...(options?.headers || {}) },
    ...options,
  });
  const body = await response.json().catch(() => ({}));
  if (!response.ok || body.ok === false) {
    throw new Error(body.error || response.statusText);
  }
  return body;
}

async function requestReportJSON(path) {
  const response = await fetch(path, { headers: { Accept: "application/json" } });
  const body = await response.json().catch(() => ({}));
  if (!response.ok) {
    throw new Error(body.error || response.statusText);
  }
  return body;
}

function runtimeText(runtime = {}) {
  return [runtime.state || "unknown", runtime.health || runtime.message || ""].filter(Boolean).join(" · ");
}

function graphInputs(graph, serviceId) {
  return (graph.edges || []).filter((edge) => edge.to === serviceId).map((edge) => edge.from).sort();
}

function graphOutputs(graph, serviceId) {
  return (graph.edges || []).filter((edge) => edge.from === serviceId).map((edge) => edge.to).sort();
}

function serviceName(caseDef, serviceId) {
  return (caseDef.graph?.nodes || []).find((node) => node.id === serviceId)?.displayName || serviceId;
}

function selectedCaseFromPayload(payload, preferredID = "") {
  const cases = payload?.cases || [];
  const requested = new URLSearchParams(window.location.search).get("case");
  return cases.find((caseDef) => caseDef.id === preferredID) || cases.find((caseDef) => caseDef.id === requested) || cases[0] || null;
}

function selectedCaseForWorkflow(payload, workflowContext, preferredID = "") {
  const cases = payload?.cases || [];
  const requested = new URLSearchParams(window.location.search).get("case");
  return (
    cases.find((caseDef) => caseDef.id === preferredID && (!workflowContext.enabled || workflowContext.caseIds.includes(caseDef.id))) ||
    cases.find((caseDef) => caseDef.id === requested && (!workflowContext.enabled || workflowContext.caseIds.includes(caseDef.id))) ||
    cases.find((caseDef) => workflowContext.enabled && workflowContext.caseIds.includes(caseDef.id)) ||
    selectedCaseFromPayload(payload, preferredID)
  );
}

function caseRunPayload(caseDef) {
  return {
    casePath: caseDef.casePath || "",
    baseUrl: caseDef.baseUrl || "",
    evidenceDir: caseDef.evidenceDir || ".runtime/cases",
    timeoutSeconds: caseDef.timeoutSeconds || 90,
    overrides: caseDef.defaultOverrides || {},
  };
}

function formatDuration(ms) {
  const value = Number(ms || 0);
  if (!Number.isFinite(value) || value <= 0) return "-";
  if (value < 1000) return `${Math.round(value)} ms`;
  return `${(value / 1000).toFixed(value >= 10000 ? 0 : 1)} s`;
}

function KeyValue({ label, value, href }) {
  const body = (
    <>
      <span>{label}</span>
      <strong>{value || "-"}</strong>
    </>
  );
  if (href) {
    return (
      <a className="api-case-kv" href={href}>
        {body}
      </a>
    );
  }
  return <article className="api-case-kv">{body}</article>;
}

function MetricStrip({ management }) {
  const metrics = [
    ["Total", management.summary.total, "catalog cases"],
    ["Ready", management.summary.ready, "executable-ready"],
    ["Review", management.summary.needsReview, "metadata or source gaps"],
    ["Latest failed", management.summary.failedLatest, "needs attention"],
    ["Never run", management.summary.neverRun, "not yet exercised"],
  ];
  return (
    <section className="api-case-management-metrics" aria-label="API case management metrics">
      {metrics.map(([label, value, detail]) => (
        <article className="api-case-management-metric" key={label}>
          <span>{label}</span>
          <strong>{value}</strong>
          <p>{detail}</p>
        </article>
      ))}
    </section>
  );
}

function WorkflowCaseSet({ context }) {
  if (!context.enabled) return null;
  return (
    <section className="api-case-workflow-context" aria-label="Workflow case set">
      <div>
        <span>Workflow case set</span>
        <strong>{context.title || context.workflowId}</strong>
      </div>
      <div className="api-case-workflow-context-stats">
        <span>{`${context.summary.steps} steps`}</span>
        <span>{`${context.summary.interfaces} interfaces`}</span>
        <span>{`${context.summary.cases} cases`}</span>
      </div>
      <a className="button-link" href={`/workflow-detail.html?id=${encodeURIComponent(context.workflowId)}`}>View workflow</a>
    </section>
  );
}

function WorkflowCaseSequence({ context, selectedCase, onSelect }) {
  if (!context.enabled) return null;
  const caseRunsHref = (caseId) => {
    const params = new URLSearchParams({ case: caseId || "" });
    if (context.workflowId) params.set("workflow", context.workflowId);
    return `/case-runs.html?${params.toString()}`;
  };
  return (
    <section className="api-case-management-panel api-case-workflow-sequence">
      <div className="section-head compact-head">
        <h3>Workflow case sequence</h3>
        <p>{`${context.summary.latestFailed} latest failed · ${context.summary.sequenceIssues} issues`}</p>
      </div>
      <div className="api-case-workflow-sequence-list">
        {context.steps.length ? context.steps.map((step) => {
          const selected = selectedCase?.id && selectedCase.id === step.caseId;
          return (
            <article className={`api-case-workflow-step ${step.state} ${selected ? "selected" : ""}`.trim()} key={`${step.sequence}-${step.id}`}>
              <div>
                <span>{String(step.sequence).padStart(2, "0")}</span>
                <strong>{step.title || step.id}</strong>
                {step.interfaceHref ? <a className="api-case-workflow-interface-link" href={step.interfaceHref}>{step.interfaceId}</a> : <code>missing interface</code>}
              </div>
              <div>
                <button type="button" disabled={!step.caseId} onClick={() => onSelect(step.caseId)}>
                  {step.caseTitle || step.caseId || "No case mapped"}
                </button>
                <small>{`${step.readiness} · ${step.latestStatus}`}</small>
              </div>
              <div className="api-case-workflow-step-actions">
                {step.caseId ? <a className="button-link" href={caseRunsHref(step.caseId)}>Runs</a> : null}
                {step.latestEvidenceHref ? <a className="button-link" href={step.latestEvidenceHref}>Evidence</a> : null}
              </div>
            </article>
          );
        }) : <p className="api-case-muted">No workflow steps.</p>}
      </div>
    </section>
  );
}

function FacetBar({ management, filters, onFilter, onReset }) {
  const facetSets = [
    ["status", "Lifecycle"],
    ["owner", "Owner"],
    ["priority", "Priority"],
    ["tag", "Tags"],
    ["runState", "Latest"],
  ];
  return (
    <div className="api-case-facet-bar">
      <button type="button" className="agent-chip" onClick={onReset}>{`${management.summary.visible}/${management.summary.total} visible`}</button>
      {facetSets.flatMap(([field, label]) => management.facets[field].slice(0, 4).map((facet) => (
        <button
          type="button"
          className={`agent-chip api-case-facet ${filters[field] === facet.key ? "active" : ""}`.trim()}
          key={`${field}-${facet.key}`}
          onClick={() => onFilter(field, facet.key)}
        >
          {`${label} ${facet.label}: ${facet.count}`}
        </button>
      )))}
    </div>
  );
}

function CaseResult({ result }) {
  if (!result) {
    return <div className="api-case-result">ready</div>;
  }
  const data = result.report || result.summary || {};
  const title = `${data.status || "fail"} · ${data.run_id || "-"}`;
  const meta = `http ${data.actual_http_code || "-"} · request ${data.request_id || "-"}`;
  return (
    <div className={`api-case-result ${result.ok ? "passed" : "failed"}`}>
      <strong>{title}</strong>
      <p>{meta}</p>
      {result.viewerUrl ? (
        <a className="button-link" href={result.viewerUrl}>
          打开 Evidence
        </a>
      ) : null}
    </div>
  );
}

function LatestRunSummary({ caseDef }) {
  const latestRun = caseDef?.latestRun || null;
  return (
    <div className="api-case-capability-grid">
      <KeyValue label="runs" value={String(caseDef?.runCount || 0)} />
      <KeyValue
        label="latest"
        value={latestRun ? [latestRun.status || "unknown", latestRun.failureReason].filter(Boolean).join(" · ") : "no run"}
        href={latestRun?.runId ? `/evidence-viewer.html?${new URLSearchParams({ caseRun: latestRun.runId, caseId: caseDef.id }).toString()}` : ""}
      />
      <KeyValue label="case run" value={latestRun?.caseRunId || "-"} />
      <KeyValue label="elapsed" value={latestRun?.elapsedMs ? `${latestRun.elapsedMs}ms` : "-"} />
    </div>
  );
}

function ReadinessGroups({ groups, onFilter }) {
  return (
    <section className="api-case-management-panel">
      <div className="section-head compact-head">
        <h3>Readiness groups</h3>
        <p>{`${groups.length} groups`}</p>
      </div>
      <div className="api-case-readiness-list">
        {groups.map((group) => (
          <button type="button" key={group.key} onClick={() => onFilter("readiness", group.key)}>
            <strong>{group.label}</strong>
            <span>{`${group.count} cases`}</span>
          </button>
        ))}
      </div>
    </section>
  );
}

function CoverageMatrix({ board, onSelect }) {
  const metrics = [
    ["Passed", board.summary.passed, `${board.summary.passRate}% pass rate`],
    ["Gaps", board.summary.gaps, "failed or not-run"],
    ["Not run", board.summary.notRun, "missing execution"],
    ["Covered", board.summary.covered, `${board.summary.total} catalog cases`],
  ];
  return (
    <section className="api-case-coverage-matrix" aria-label="Coverage matrix">
      <div className="section-head compact-head">
        <div>
          <h3>Coverage matrix</h3>
          <p>{`${board.groups.length} interfaces · ${board.summary.total} cases`}</p>
        </div>
      </div>
      <div className="api-case-coverage-metrics">
        {metrics.map(([label, value, detail]) => (
          <article key={label}>
            <span>{label}</span>
            <strong>{value}</strong>
            <p>{detail}</p>
          </article>
        ))}
      </div>
      <div className="api-case-coverage-group-list">
        {board.groups.length ? board.groups.map((group) => (
          <article className="api-case-coverage-group" key={group.nodeId || group.nodeName}>
            <div className="api-case-coverage-group-head">
              <div>
                <strong>{group.nodeName || group.nodeId || "Unmapped interface"}</strong>
                <span>{`${group.total} cases · ${group.gapCount} gaps`}</span>
              </div>
              <code>{[group.passed, group.failed, group.notRun].join("/")}</code>
            </div>
            <div className="api-case-coverage-row-list">
              {group.rows.map((row) => (
                <div className={`api-case-coverage-row ${row.latestStatus}`} key={row.caseId}>
                  <button type="button" onClick={() => onSelect(row.caseId)}>
                    <strong>{row.title || row.caseId}</strong>
                    <span>{row.reason || row.caseId}</span>
                  </button>
                  <span className={`agent-status ${row.latestStatus}`}>{row.latestStatus}</span>
                  <a className="button-link" href={row.caseRunsHref}>Runs</a>
                  {row.latestEvidenceHref ? <a className="button-link" href={row.latestEvidenceHref}>Evidence</a> : null}
                </div>
              ))}
            </div>
          </article>
        )) : <p className="api-case-muted">No cases matched coverage scope.</p>}
      </div>
    </section>
  );
}

function CaseManagementTable({ management, selectedCase, onSelect }) {
  return (
    <section className="api-case-management-panel api-case-management-table-panel">
      <div className="section-head compact-head">
        <h3>Case Management Search</h3>
        <p>Lifecycle, owner, priority, latest result, and runnable source</p>
      </div>
      <div className="api-case-management-table-wrap">
        <table className="api-case-management-table">
          <thead>
            <tr>
              <th>Case</th>
              <th>Lifecycle</th>
              <th>Owner</th>
              <th>Priority</th>
              <th>Tags</th>
              <th>Latest</th>
              <th>Readiness</th>
              <th>Evidence</th>
            </tr>
          </thead>
          <tbody>
            {management.rows.length ? management.rows.map((row) => (
              <tr className={row.id === selectedCase?.id ? "selected" : ""} key={row.id}>
                <td>
                  <button type="button" className="api-case-row-select" onClick={() => onSelect(row.caseDef)}>
                    <strong>{row.title || row.id}</strong>
                    <span>{row.operation || row.id}</span>
                  </button>
                </td>
                <td><span className="agent-chip">{row.status}</span></td>
                <td>{row.owner}</td>
                <td>{row.priority}</td>
                <td><div className="api-case-tag-list">{row.tags.length ? row.tags.map((tag) => <code key={tag}>{tag}</code>) : <code>untagged</code>}</div></td>
                <td>
                  <span className={`agent-status ${row.latestStatus}`}>{row.latestStatus}</span>
                  <code>{row.latestElapsedMs ? formatDuration(row.latestElapsedMs) : `${row.runCount} runs`}</code>
                </td>
                <td><span className={`api-case-readiness ${row.readiness}`}>{row.readiness}</span></td>
                <td>{row.latestEvidenceHref ? <a className="button-link api-case-evidence-link" href={row.latestEvidenceHref}>Evidence</a> : <span className="api-case-muted">-</span>}</td>
              </tr>
            )) : (
              <tr>
                <td className="api-case-table-empty" colSpan="8">No matching API cases.</td>
              </tr>
            )}
          </tbody>
        </table>
      </div>
    </section>
  );
}

function CaseServices({ caseDef }) {
  const graph = caseDef?.graph || { nodes: [], edges: [] };
  return (
    <div className="api-case-service-list">
      {(graph.nodes || []).map((service) => (
        <section className="api-case-service-card" key={service.id}>
          <div className="section-head compact-head">
            <h3>{service.displayName || service.id}</h3>
          </div>
          <div className="api-case-capability-grid">
            <KeyValue label="service" value={service.id} href={service.href} />
            <KeyValue label="role" value={service.role} />
            <KeyValue label="port" value={service.port ? `:${service.port}` : "-"} />
            <KeyValue label="runtime" value={runtimeText(service.runtime)} />
            <KeyValue label="in" value={graphInputs(graph, service.id).map((id) => serviceName(caseDef, id)).join(", ") || "-"} />
            <KeyValue label="out" value={graphOutputs(graph, service.id).map((id) => serviceName(caseDef, id)).join(", ") || "-"} />
          </div>
        </section>
      ))}
    </div>
  );
}

function CaseBoundary({ caseDef }) {
  const graph = caseDef?.graph || { nodes: [], edges: [] };
  return (
    <>
      <div className="workflow-graph-nodes">
        {(graph.nodes || []).map((service) => {
          const className = `workflow-graph-node ${service.role || "unknown"}`;
          const body = (
            <>
              <strong>{service.displayName || service.id}</strong>
              <span>{[service.role || "service", service.port ? `:${service.port}` : ""].filter(Boolean).join(" · ")}</span>
            </>
          );
          return service.href ? (
            <a className={className} href={service.href} key={service.id}>
              {body}
            </a>
          ) : (
            <article className={className} key={service.id}>
              {body}
            </article>
          );
        })}
      </div>
      <div className="workflow-graph-edges">
        {(graph.edges || []).map((edge) => (
          <article className="workflow-graph-edge" key={`${edge.from}-${edge.to}`}>
            <strong>{serviceName(caseDef, edge.from)}</strong>
            <span>{"->"}</span>
            <strong>{serviceName(caseDef, edge.to)}</strong>
          </article>
        ))}
      </div>
    </>
  );
}

function ApiCasesApp() {
  const [capabilities, setCapabilities] = useState(null);
  const [catalog, setCatalog] = useState(null);
  const [coverage, setCoverage] = useState(null);
  const [selectedCase, setSelectedCase] = useState(null);
  const [result, setResult] = useState(null);
  const [status, setStatus] = useState("loading");
  const [query, setQuery] = useState("");
  const [filters, setFilters] = useState({ status: "", owner: "", priority: "", tag: "", runState: "", readiness: "" });
  const [sort, setSort] = useState("readiness");

  async function loadCapabilities(preferredCaseID = "", nextStatus = "ready") {
    setStatus("loading...");
    try {
      const [payload, catalogPayload, coveragePayload] = await Promise.all([
        requestJSON("/api/cases/capabilities"),
        requestJSON("/api/catalog"),
        requestReportJSON("/api/case/suite-coverage"),
      ]);
      const workflowContext = buildWorkflowCaseContext(catalogPayload, new URLSearchParams(window.location.search).get("workflow") || "", payload.cases || []);
      setCapabilities(payload);
      setCatalog(catalogPayload);
      setCoverage(coveragePayload);
      setSelectedCase(selectedCaseForWorkflow(payload, workflowContext, preferredCaseID));
      setStatus(nextStatus);
    } catch (error) {
      setStatus(error.message);
    }
  }

  async function runSelectedCase() {
    if (!selectedCase) return;
    setStatus("running...");
    try {
      const payload = await requestJSON("/api/cases/run", {
        method: "POST",
        headers: { "content-type": "application/json" },
        body: JSON.stringify(caseRunPayload(selectedCase)),
      });
      setResult(payload);
      await loadCapabilities(selectedCase.id, payload.ok ? "ready" : "case failed");
    } catch (error) {
      setResult({ ok: false, summary: { status: "fail", failure_reason: error.message } });
      setStatus(error.message);
    }
  }

  useEffect(() => {
    loadCapabilities();
  }, []);

  const cases = capabilities?.cases || [];
  const graph = selectedCase?.graph || { nodes: [], edges: [] };
  const workflowContext = useMemo(() => buildWorkflowCaseContext(catalog, new URLSearchParams(window.location.search).get("workflow") || "", cases), [catalog, cases]);
  const management = useMemo(() => {
    const base = buildCaseManagement(cases, { ...filters, query, sort, caseIds: workflowContext.caseIds, caseIdsFilterEnabled: workflowContext.enabled });
    if (!filters.readiness) return base;
    return { ...base, rows: base.rows.filter((row) => row.readiness === filters.readiness), summary: { ...base.summary, visible: base.rows.filter((row) => row.readiness === filters.readiness).length } };
  }, [cases, filters, query, sort, workflowContext.caseIds]);
  const coverageBoard = useMemo(
    () => buildCaseCoverageBoard(coverage || { items: [] }, workflowContext.enabled ? { workflowId: workflowContext.workflowId, caseIds: workflowContext.caseIds } : {}),
    [coverage, workflowContext.enabled, workflowContext.workflowId, workflowContext.caseIds],
  );
  const caseMeta = useMemo(() => [selectedCase?.id, selectedCase?.operation].filter(Boolean).join(" · "), [selectedCase]);
  const pageSummary = `${management.summary.visible}/${management.summary.total} cases · ${management.summary.ready} ready · ${management.summary.failedLatest} latest failed`;

  function updateFilter(field, value) {
    setFilters((current) => ({ ...current, [field]: current[field] === value ? "" : value }));
  }

  function selectCase(caseDef) {
    setSelectedCase(caseDef);
    setResult(null);
  }

  function selectCaseByID(caseID) {
    const next = cases.find((caseDef) => caseDef.id === caseID);
    if (next) {
      selectCase(next);
    }
  }

  return (
    <main className="app api-case-page">
      <section className="topbar">
        <div>
          <h1>API Case 工作台</h1>
          <p>{pageSummary}</p>
        </div>
        <div className="actions">
          <span className="workflow-step-status-pill" role="status">
            {status}
          </span>
          <a className="button-link" href="/">
            控制台
          </a>
          <a className="button-link" href="/dashboard.html">
            环境大盘
          </a>
          <a className="button-link" href="/service-inventory.html">
            服务清单
          </a>
        </div>
      </section>

      <MetricStrip management={management} />
      <WorkflowCaseSet context={workflowContext} />
      <CoverageMatrix board={coverageBoard} onSelect={selectCaseByID} />

      <section className="api-case-management-toolbar">
        <label className="workflow-filter">
          <span>Search</span>
          <input type="search" placeholder="case / owner / tag / latest" spellCheck="false" value={query} onChange={(event) => setQuery(event.target.value)} />
        </label>
        <label className="workflow-filter">
          <span>Sort</span>
          <select value={sort} onChange={(event) => setSort(event.target.value)}>
            <option value="readiness">Readiness</option>
            <option value="priority_desc">Priority</option>
            <option value="latest_failed">Latest failed</option>
            <option value="owner_asc">Owner</option>
            <option value="case_asc">Case ID</option>
          </select>
        </label>
        <FacetBar
          management={management}
          filters={filters}
          onFilter={updateFilter}
          onReset={() => {
            setQuery("");
            setFilters({ status: "", owner: "", priority: "", tag: "", runState: "", readiness: "" });
          }}
        />
      </section>

      <section className="api-case-management-shell">
        <section className="api-case-management-main">
          <CaseManagementTable management={management} selectedCase={selectedCase} onSelect={selectCase} />
        </section>

        <aside className="api-case-management-side">
          <WorkflowCaseSequence context={workflowContext} selectedCase={selectedCase} onSelect={selectCaseByID} />
          <ReadinessGroups groups={management.readinessGroups} onFilter={updateFilter} />
          <section className="api-case-panel api-case-control-panel">
          <div className="section-head">
            <div>
              <h2>{selectedCase?.title || selectedCase?.id || "API Case"}</h2>
              <p>{caseMeta || "loading"}</p>
            </div>
          </div>
          <LatestRunSummary caseDef={selectedCase} />
          <div className="api-case-trigger">
            <p>使用 Catalog 中声明的 case 文件、网关地址、默认参数和证据目录运行；页面不暴露请求参数。</p>
            <button className="primary-action" type="button" disabled={!selectedCase || status === "running..."} onClick={runSelectedCase}>
              运行 Case
            </button>
          </div>
          <CaseResult result={result} />
          </section>
        </aside>
      </section>

      <section className="api-case-shell">
        <section className="api-case-panel">
          <div className="section-head">
            <div>
              <h2>相关服务</h2>
              <p>由 Catalog 中当前 Case 的 DAG 节点生成。</p>
            </div>
          </div>
          <CaseServices caseDef={selectedCase} />
        </section>

        <section className="api-case-panel api-case-graph-panel">
          <div className="section-head">
            <div>
              <h2>Case 服务边界</h2>
              <p>{`${graph.nodes?.length || 0} services · ${graph.edges?.length || 0} DAG edges`}</p>
            </div>
          </div>
          <CaseBoundary caseDef={selectedCase} />
        </section>
      </section>
    </main>
  );
}

createRoot(document.getElementById("react-api-cases-root")).render(<ApiCasesApp />);
