import { useEffect, useMemo, useState } from "react";
import { createRoot } from "react-dom/client";
import { RefreshCw } from "lucide-react";

function queryParam(name) {
  return new URLSearchParams(window.location.search).get(name) || "";
}

function pageMode() {
  const path = window.location.pathname;
  if (path.includes("history")) return "history";
  if (path.includes("fields")) return "fields";
  return "main";
}

function text(value, fallback = "-") {
  const out = String(value ?? "").trim();
  return out || fallback;
}

function tail(value, length = 12) {
  const out = text(value);
  return out.length <= length ? out : `...${out.slice(-length)}`;
}

function prettyJSON(value) {
  if (value === undefined || value === null || value === "") return "-";
  if (typeof value === "string") {
    try {
      return JSON.stringify(JSON.parse(value), null, 2);
    } catch {
      return value;
    }
  }
  return JSON.stringify(value, null, 2);
}

function duration(ms) {
  const value = Number(ms || 0);
  if (!Number.isFinite(value) || value <= 0) return "-";
  if (value < 1000) return `${Math.round(value)}ms`;
  return `${(value / 1000).toFixed(1)}s`;
}

function runElapsedMs(run) {
  if (!run) return 0;
  const direct = Number(run.elapsedMs || 0);
  if (Number.isFinite(direct) && direct > 0) return direct;
  try {
    const summary = JSON.parse(run.summaryJson || "{}");
    const parsed = Number(summary.elapsedMs || summary.elapsed_ms || 0);
    return Number.isFinite(parsed) ? parsed : 0;
  } catch {
    return 0;
  }
}

function caseGroupKey(item) {
  const type = String(item?.caseType || "").trim().toLowerCase();
  return ["success", "pass", "positive"].includes(type) ? "success" : "failure";
}

function caseNumber(cases, item) {
  if (!item) return "";
  const groupKey = caseGroupKey(item);
  const group = cases.filter((candidate) => caseGroupKey(candidate) === groupKey);
  const index = group.findIndex((candidate) => candidate.id && item.id ? candidate.id === item.id : candidate === item);
  return `${groupKey === "success" ? "S" : "F"}${String(Math.max(index, 0) + 1).padStart(2, "0")}`;
}

function outcomeLabel(item) {
  const run = item?.latestRun;
  if (!run) return "no run";
  const status = String(run.status || "").trim().toLowerCase();
  const passed = ["pass", "passed", "success", "succeeded"].includes(status);
  if (caseGroupKey(item) === "failure") {
    return passed ? "命中预期失败" : "未命中预期失败";
  }
  return run.status || "unknown";
}

function RunBadge({ item }) {
  const run = item?.latestRun;
  const tone = run?.status === "pass" || run?.status === "passed" ? "good" : run ? "bad" : "warn";
  return <span className={`react-pill ${tone}`}>{outcomeLabel(item)}</span>;
}

function Stat({ label, value, title }) {
  return (
    <div title={text(title || value)}>
      <span>{label}</span>
      <strong>{text(value)}</strong>
    </div>
  );
}

function Panel({ title, subtitle, className = "", children }) {
  return (
    <section className={`environment-node-detail-panel interface-node-panel ${className}`.trim()}>
      <div className="dashboard-section-head">
        <h2>{title}</h2>
        <p>{subtitle}</p>
      </div>
      {children}
    </section>
  );
}

function Summary({ rows }) {
  return (
    <div className="interface-run-summary interface-node-case-run-summary">
      {rows.map(([label, value, tone]) => (
        <article className={["interface-run-kv", tone].filter(Boolean).join(" ")} key={label}>
          <span>{label}</span>
          <strong>{text(value)}</strong>
        </article>
      ))}
    </div>
  );
}

function RequestTemplatePanel({ payload }) {
  const templates = Array.isArray(payload.requestTemplates) ? payload.requestTemplates : [];
  const requestFields = payload.fields?.request || [];
  return (
    <Panel
      title="公共模板参数"
      subtitle={templates.length ? "来自 interface_node_request_template，Case 只维护差异 Patch" : "尚未登记公共请求模板，先按接口字段契约展示公共参数骨架"}
      className="interface-node-request-template-panel"
    >
      <div className={`interface-node-request-template-body ${templates.length ? "" : "no-template"}`.trim()}>
        <div className="interface-node-request-template-fields">
          <span>公共参数</span>
          {requestFields.length ? requestFields.map((field) => (
            <div className="interface-node-request-template-field" key={field.id || field.fieldPath}>
              <strong>{field.displayName || field.fieldPath || field.id || "-"}</strong>
              <code>{field.fieldPath || "-"}</code>
              <span>{[field.dataType || "unknown", field.required ? "required" : "optional", field.bindable ? "bindable" : ""].filter(Boolean).join(" · ")}</span>
            </div>
          )) : <p className="dashboard-empty">当前接口节点还没有登记请求字段。</p>}
        </div>
        <div className="interface-node-request-template-list">
          <span>模板 JSON</span>
          {templates.length ? templates.map((template) => (
            <article className="interface-node-request-template-card" key={template.id}>
              <div className="interface-node-request-template-card-top">
                <strong>{template.name || template.id || "公共请求模板"}</strong>
                <code>{[template.id || "", template.version || "", template.status || ""].filter(Boolean).join(" · ") || "-"}</code>
              </div>
              <pre>{prettyJSON(template.templateJson || template.template_json || "{}")}</pre>
            </article>
          )) : <p className="dashboard-empty">未找到 interface_node_request_template 记录。新增必填字段时，应优先补公共请求模板，再让 Case Patch 表达差异。</p>}
        </div>
      </div>
    </Panel>
  );
}

function Dependencies({ item }) {
  const dependencies = item?.dependencies || [];
  if (!dependencies.length) return null;
  return (
    <div className="interface-node-case-dependencies">
      <span>前置数据</span>
      {dependencies.map((dependency) => (
        <div className="interface-node-case-dependency" key={dependency.id || dependency.fixtureProfileId}>
          <strong>{dependency.profile?.name || dependency.fixtureProfileId || dependency.id}</strong>
          <code>{[dependency.fixtureProfileId, dependency.required ? "required" : "optional", (dependency.tableBindings || []).map((binding) => `${binding.schemaName}.${binding.tableName}`).join(", ")].filter(Boolean).join(" · ")}</code>
          <pre>{prettyJSON(dependency.mappingsJson || "[]")}</pre>
        </div>
      ))}
    </div>
  );
}

function CaseDetail({ item, cases, onRunCase }) {
  if (!item) {
    return (
      <article className="interface-node-case-detail">
        <p className="dashboard-empty">当前接口节点还没有配置测试用例。</p>
      </article>
    );
  }
  const run = item.latestRun || {};
  return (
    <article className="interface-node-case-detail">
      <div className="interface-node-case-detail-top">
        <div>
          <h3>{`${caseNumber(cases, item)} · ${item.title || item.id}`}</h3>
          <code>{item.id || "-"}</code>
        </div>
        <RunBadge item={item} />
      </div>
      <p className="interface-node-case-detail-meta">
        {[caseGroupKey(item) === "success" ? "成功" : "失败", item.caseType || "case", outcomeLabel(item), `最近耗时 ${duration(runElapsedMs(item.latestRun))}`, item.latestRun?.failureReason || "", item.scenario || "", item.requiredForAdmission ? "required_for_admission" : "optional", item.blocked ? "暂不可运行" : ""].filter(Boolean).join(" · ")}
      </p>
      <Summary rows={[
        ["case", item.caseType || "case"],
        ["required", item.requiredForAdmission ? "yes" : "no"],
        ["latest run", run.runId ? tail(run.runId) : "no run"],
        ["elapsed", duration(runElapsedMs(run))],
      ]} />
      {item.latestRun?.runId ? <a className="button-link interface-node-evidence-link" href={`/evidence-viewer.html?caseRun=${encodeURIComponent(item.latestRun.runId)}`}>查看运行证据</a> : null}
      <div className="interface-node-case-actions">
        <button className="button-link interface-node-case-run-button" type="button" disabled={Boolean(item.blocked)} onClick={() => onRunCase(item.id)}>
          {item.blocked ? "等待前置数据" : "运行此用例"}
        </button>
      </div>
      <Dependencies item={item} />
    </article>
  );
}

function CasesPanel({ payload, onRunCase, onRunAll }) {
  const cases = payload.cases || [];
  const [selectedID, setSelectedID] = useState(cases[0]?.id || "");
  useEffect(() => {
    if (cases.length && !cases.some((item) => item.id === selectedID)) {
      setSelectedID(cases[0].id || "");
    }
  }, [cases, selectedID]);
  const selected = cases.find((item) => item.id === selectedID) || cases[0] || null;
  const groups = [
    { key: "success", title: "成功用例", items: cases.filter((item) => caseGroupKey(item) === "success") },
    { key: "failure", title: "失败用例", items: cases.filter((item) => caseGroupKey(item) === "failure") },
  ];

  return (
    <Panel title="测试用例" subtitle="接口准入用例与最近运行耗时" className="interface-node-cases-panel">
      {cases.length ? (
        <>
          <div className="interface-node-case-toolbar">
            <span className="interface-node-case-total">最近总耗时 {duration(cases.reduce((sum, item) => sum + runElapsedMs(item.latestRun), 0))}</span>
            <button type="button" className="button-link interface-node-case-run-all" onClick={() => onRunAll(cases)}>全部运行</button>
          </div>
          <div className="interface-node-case-browser">
            <div className="interface-node-case-list">
              {groups.map((group) => (
                <section className="interface-node-case-group" data-case-group={group.key} key={group.key}>
                  <div className="interface-node-case-group-head">
                    <strong>{group.title}</strong>
                    <span>{group.items.length}</span>
                  </div>
                  {group.items.length ? group.items.map((item) => (
                    <button className={`interface-node-case-list-item ${item.id === selectedID ? "selected" : ""}`} type="button" data-case-id={item.id || ""} onClick={() => setSelectedID(item.id || "")} key={item.id}>
                      <span className="interface-node-case-number">{caseNumber(cases, item)}</span>
                      <strong>{item.title || item.id || "case"}</strong>
                      <span>{[item.id, `耗时 ${duration(runElapsedMs(item.latestRun))}`, outcomeLabel(item), item.requiredForAdmission ? "required" : "optional", item.blocked ? "暂不可运行" : "", item.scenario || ""].filter(Boolean).join(" · ")}</span>
                      <RunBadge item={item} />
                    </button>
                  )) : <p className="dashboard-empty">暂无</p>}
                </section>
              ))}
            </div>
            <div className="interface-node-case-detail-wrap">
              <CaseDetail item={selected} cases={cases} onRunCase={onRunCase} />
            </div>
          </div>
        </>
      ) : <p className="dashboard-empty">当前接口节点还没有配置测试用例。</p>}
    </Panel>
  );
}

function AdmissionPanel({ admission }) {
  const blockers = admission.blockers || [];
  if (!blockers.length) return null;
  return (
    <Panel title="准入阻塞" subtitle="required_for_admission Case 的当前阻塞项" className="interface-node-admission">
      <div className="interface-node-admission-blockers">
        {blockers.map((blocker) => (
          <article className="interface-node-admission-blocker" key={blocker.caseId || blocker.title}>
            <div>
              <strong>{blocker.title || blocker.caseId || "required case"}</strong>
              <span className={`react-pill ${blocker.status === "failed" ? "bad" : "warn"}`}>{blocker.status || "blocked"}</span>
            </div>
            <code>{[blocker.caseId, blocker.runId, blocker.failureKind].filter(Boolean).join(" · ") || "-"}</code>
            <p>{blocker.failureReason || "required case is not admitted"}</p>
            {blocker.evidenceHref ? <a className="button-link interface-node-admission-blocker-link" href={blocker.evidenceHref}>打开证据</a> : null}
          </article>
        ))}
      </div>
    </Panel>
  );
}

function HistoryPanel({ payload }) {
  const history = payload.history || {};
  const perCase = Array.isArray(history.perCase) ? history.perCase : [];
  return (
    <Panel title="运行历史" subtitle="来自 interface_node_case_run 的最近运行聚合" className="interface-node-history-panel">
      <div className="interface-node-history-grid">
        <Stat label="最近运行" value={tail(history.latestRunId || "-")} title={history.latestRunId || "-"} />
        <Stat label="通过/失败" value={`${history.passCount || 0}/${history.failCount || 0}`} />
        <Stat label="运行总数" value={history.runCount || 0} />
        <Stat label="最近失败" value={text(history.latestFailureReason || "-", "-")} title={history.latestFailureReason || "-"} />
        <Stat label="累计耗时" value={duration(history.totalElapsedMs || 0)} />
      </div>
      <div className="interface-node-history-case-list">
        {perCase.length ? perCase.slice(0, 8).map((item) => (
          <div className="interface-node-history-case" key={item.caseId}>
            <strong>{item.caseId || "-"}</strong>
            <span>{[`${item.passCount || 0}/${item.failCount || 0}`, item.latestStatus || "-", duration(item.latestElapsedMs || 0), item.latestFailureReason || ""].filter(Boolean).join(" · ")}</span>
          </div>
        )) : <p className="dashboard-empty">还没有接口级运行历史。</p>}
      </div>
    </Panel>
  );
}

function RunsPanel({ payload }) {
  const runs = payload.runs || [];
  return (
    <Panel title="运行证据索引" subtitle="只保留 Evidence 路径和摘要索引，证据正文仍在 Case bundle 中" className="interface-node-runs-panel">
      <div className="interface-node-run-list">
        {runs.length ? runs.slice(0, 8).map((run) => (
          <a className="environment-node-peer interface-node-run-item" href={run?.runId ? `/evidence-viewer.html?caseRun=${encodeURIComponent(run.runId)}` : "#"} key={run.runId || run.caseId}>
            <strong>{run?.runId || "-"}</strong>
            <span>{`${run?.caseId || "-"} · ${run?.status || "-"}`}</span>
          </a>
        )) : <p className="dashboard-empty">还没有接口级 Case run 证据。</p>}
      </div>
    </Panel>
  );
}

function FieldCard({ field }) {
  return (
    <article className="interface-node-field-card">
      <strong>{field.displayName || field.fieldPath || field.id}</strong>
      <code>{field.fieldPath || "-"}</code>
      <span>{[field.dataType || "unknown", field.required ? "required" : "optional", field.bindable ? "bindable" : ""].filter(Boolean).join(" · ")}</span>
    </article>
  );
}

function FieldsPanel({ payload, direction, title, subtitle }) {
  const fields = payload.fields?.[direction] || [];
  return (
    <Panel title={title} subtitle={subtitle} className={`interface-node-${direction}-fields`}>
      <div className="interface-node-field-grid">
        {fields.length ? fields.map((field) => <FieldCard field={field} key={field.id || field.fieldPath} />) : <p className="dashboard-empty">当前接口节点还没有配置字段。</p>}
      </div>
    </Panel>
  );
}

function FieldContract({ payload }) {
  const requestFields = payload.fields?.request || [];
  const responseFields = payload.fields?.response || [];
  const rows = [
    ["request required", `${requestFields.filter((field) => field.required).length}/${requestFields.length}`],
    ["response required", `${responseFields.filter((field) => field.required).length}/${responseFields.length}`],
    ["bindable response", responseFields.filter((field) => field.bindable).length],
  ];
  return (
    <Panel title="字段契约" subtitle="只汇总已登记字段配置，不从业务样例推断字段" className="interface-node-field-contract">
      <div className="interface-node-field-contract-grid">
        {rows.map(([label, value]) => (
          <div key={label}>
            <span>{label}</span>
            <strong>{value}</strong>
          </div>
        ))}
      </div>
    </Panel>
  );
}

function MissingNode({ payload, requested }) {
  const available = payload.available || [];
  return (
    <section className="interface-node-layout" aria-label="接口节点测试用例">
      <Panel title="可选接口节点" subtitle="当前 template-config SQLite 中已登记的接口节点" className="interface-node-missing-panel">
        <div className="environment-node-peer-list">
          {available.length ? available.map((item) => (
            <a className="environment-node-peer" href={item.href} key={item.id}>
              <strong>{item.displayName || item.id}</strong>
              <span>{`${item.serviceId || "-"} · ${item.operation || "-"}`}</span>
            </a>
          )) : <p className="dashboard-empty">{requested ? "还没有登记接口节点配置。" : "缺少 id 参数。"}</p>}
        </div>
      </Panel>
    </section>
  );
}

function InterfaceNodeApp() {
  const mode = pageMode();
  const [payload, setPayload] = useState(null);
  const [message, setMessage] = useState("loading");
  const requestedID = queryParam("id");

  async function getJSON(path) {
    const response = await fetch(path, { headers: { Accept: "application/json" } });
    const data = await response.json().catch(() => ({}));
    if (!response.ok || data.ok === false) {
      const error = new Error(data.error || response.statusText);
      error.payload = data;
      throw error;
    }
    return data;
  }

  async function load(options = {}) {
    if (!options.silent) setMessage("refreshing...");
    try {
      setPayload(await getJSON(`/api/interface-node?id=${encodeURIComponent(requestedID)}`));
      if (!options.preserveStatus) setMessage("ready");
    } catch (error) {
      setPayload(error.payload || { ok: false, requested: requestedID, available: [] });
      setMessage("missing");
    }
  }

  async function postJSON(path, body) {
    const response = await fetch(path, {
      method: "POST",
      headers: { "content-type": "application/json", Accept: "application/json" },
      body: JSON.stringify(body || {}),
    });
    const data = await response.json().catch(() => ({}));
    if (!response.ok || data.ok === false) throw new Error(data.error || data.stderr || response.statusText);
    return data;
  }

  async function runCase(caseId) {
    if (!caseId) return;
    setMessage(`running ${caseId}`);
    try {
      const result = await postJSON("/api/test-kit/run", { caseId, dryRun: false, skipTraceTopology: false, timeoutSeconds: 90 });
      const finalStatus = `${result.ok ? "case run passed" : "case run failed"} · ${duration(result.elapsedMs || 0)}`;
      setMessage(finalStatus);
      await load({ silent: true, preserveStatus: true });
      setMessage(finalStatus);
    } catch (error) {
      setMessage(error.message);
    }
  }

  async function runAll(cases) {
    const runnable = (cases || []).filter((item) => item.id && !item.blocked);
    if (!runnable.length) return;
    setMessage(`running ${runnable.length} cases concurrently`);
    try {
      const result = await postJSON("/api/test-kit/run-batch", {
        caseIds: runnable.map((item) => item.id),
        dryRun: false,
        skipTraceTopology: false,
        timeoutSeconds: 90,
        concurrency: runnable.length,
      });
      const summary = result.summary || {};
      const finalStatus = `all cases finished · ${summary.passed || 0}/${summary.caseCount || runnable.length} passed · ${duration(result.elapsedMs || 0)}`;
      setMessage(finalStatus);
      await load({ silent: true, preserveStatus: true });
      setMessage(finalStatus);
    } catch (error) {
      setMessage(error.message);
    }
  }

  useEffect(() => {
    load();
  }, []);

  const node = payload?.node || {};
  const admission = payload?.admission || {};
  const nodeID = node.id || requestedID;
  const stats = useMemo(() => [
    ["准入", admission.status || "pending"],
    ["必需 Case", admission.requiredCaseCount ?? 0],
    ["已通过", admission.passedCaseCount ?? 0],
    ["最新运行", tail(admission.latestRunId || "-"), admission.latestRunId || "-"],
  ], [admission]);
  const missing = payload?.ok === false || message === "missing";
  const pageClass = mode === "history" ? "interface-node-history-page" : mode === "fields" ? "interface-node-field-page" : "interface-node-main-page";
  const pageTitle = missing
    ? "未找到接口节点"
    : mode === "history"
      ? "接口节点运行历史"
      : mode === "fields"
        ? "接口节点字段契约"
        : node.displayName || node.id || "接口节点";
  const contentClass = mode === "history" ? "interface-node-layout interface-node-history-layout" : mode === "fields" ? "interface-node-layout interface-node-field-layout" : "interface-node-layout";

  return (
    <main className={`app interface-node-page ${pageClass}`} data-template-id={mode === "history" ? "TPL-INTERFACE-NODE-RUN-HISTORY-V1" : mode === "fields" ? "TPL-INTERFACE-NODE-FIELD-CONTRACT-V1" : "TPL-INTERFACE-NODE-CASE-LIST-V1"} data-interface-node-mode={mode}>
      <div className="template-watermark" aria-label="模板编号">{mode === "history" ? "TPL-INTERFACE-NODE-RUN-HISTORY-V1" : mode === "fields" ? "TPL-INTERFACE-NODE-FIELD-CONTRACT-V1" : "TPL-INTERFACE-NODE-CASE-LIST-V1"}</div>
      <section className="topbar interface-node-topbar">
        <div>
          <p className="viewer-eyebrow">{mode === "history" ? "Interface Node History" : mode === "fields" ? "Interface Node Fields" : "Interface Node"}</p>
          <h1>{pageTitle}</h1>
          <p>{missing ? payload?.requested || requestedID || "缺少 id" : `${text(node.serviceId)} · ${text(node.operation)} · ${text(node.method)} ${text(node.path)}`}</p>
        </div>
        <div className="dashboard-top-stats" aria-label="接口节点摘要">
          {missing ? (
            <>
              <Stat label="状态" value="missing" />
              <Stat label="可选节点" value={(payload?.available || []).length} />
            </>
          ) : stats.map(([label, value, title]) => <Stat label={label} value={value} title={title} key={label} />)}
        </div>
        <div className="actions">
          <span className="environment-status-pill" role="status">{message}</span>
          <a className="button-link" href={node.serviceId ? `/environment-node.html?id=${encodeURIComponent(node.serviceId)}` : "/environment-nodes.html"}>{node.serviceId ? "服务节点" : "环境节点"}</a>
          <a className={`button-link ${mode === "main" ? "disabled-link" : ""}`} href={nodeID ? `/interface-node.html?id=${encodeURIComponent(nodeID)}` : "/interface-node.html"}>用例概览</a>
          <a className={`button-link ${mode === "history" ? "disabled-link" : ""}`} href={nodeID ? `/interface-node-history.html?id=${encodeURIComponent(nodeID)}` : "/interface-node-history.html"}>运行历史</a>
          <a className={`button-link ${mode === "fields" ? "disabled-link" : ""}`} href={nodeID ? `/interface-node-fields.html?id=${encodeURIComponent(nodeID)}` : "/interface-node-fields.html"}>字段契约</a>
          <button type="button" title="刷新状态" onClick={() => load()}>
            <RefreshCw size={15} aria-hidden="true" />
          </button>
        </div>
      </section>

      {missing ? <MissingNode payload={payload || {}} requested={requestedID} /> : (
        <section className={contentClass} aria-label="接口节点测试用例">
          {mode === "history" ? (
            <>
              <HistoryPanel payload={payload || {}} />
              <RunsPanel payload={payload || {}} />
            </>
          ) : mode === "fields" ? (
            <>
              <FieldContract payload={payload || {}} />
              <FieldsPanel payload={payload || {}} direction="request" title="标准请求参数" subtitle="接口入参字段，可用于后续模板确认" />
              <FieldsPanel payload={payload || {}} direction="response" title="标准返回参数" subtitle="可连线字段应在配置中标记为 bindable" />
            </>
          ) : (
            <>
              <RequestTemplatePanel payload={payload || {}} />
              <CasesPanel payload={payload || {}} onRunCase={runCase} onRunAll={runAll} />
              <AdmissionPanel admission={admission} />
            </>
          )}
        </section>
      )}
    </main>
  );
}

createRoot(document.getElementById("react-interface-node-root")).render(<InterfaceNodeApp />);
