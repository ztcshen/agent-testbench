import { useEffect, useState } from "react";
import { createRoot } from "react-dom/client";
import { fetchJSON } from "./api.js";

function queryParam(name) {
  return new URLSearchParams(window.location.search).get(name) || "";
}

function text(value, defaultValue = "-") {
  const out = String(value ?? "").trim();
  return out || defaultValue;
}

function formatTime(value) {
  if (!value) return "-";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  return new Intl.DateTimeFormat("zh-CN", {
    month: "numeric",
    day: "numeric",
    hour: "2-digit",
    minute: "2-digit",
    second: "2-digit",
  }).format(date);
}

function safeJSON(value) {
  if (!value) return {};
  if (typeof value === "object") return value;
  try {
    return JSON.parse(value);
  } catch {
    return {};
  }
}

function KV({ label, value }) {
  return (
    <article>
      <span>{label}</span>
      <strong>{text(value)}</strong>
    </article>
  );
}

function Panel({ title, summary, className, children }) {
  return (
    <section className={`replay-evidence-panel ${className || ""}`.trim()}>
      <div className="section-head">
        <div>
          <h2>{title}</h2>
          <p>{summary}</p>
        </div>
      </div>
      {children}
    </section>
  );
}

function RunPanel({ payload }) {
  const run = payload.run || {};
  const evidence = payload.evidence || {};
  const summary = safeJSON(run.summaryJson);
  return (
    <Panel title="Run" summary={`${run.httpStatus || summary.httpStatus || "-"} · ${formatTime(run.createdAt)}`} className="replay-evidence-run-panel">
      <div className="replay-evidence-run">
        <KV label="trace" value={run.traceId || evidence.traceId} />
        <KV label="target" value={run.targetUrl || evidence.request?.targetUrl || summary.targetUrl} />
        <KV label="scenario" value={run.scenario || summary.scenario || "-"} />
        <KV label="evidence" value={run.evidencePath || "-"} />
      </div>
    </Panel>
  );
}

function RequestPanel({ evidence }) {
  const request = evidence.request || {};
  const response = evidence.response || {};
  return (
    <Panel title="Request / response" summary={`${request.method || "-"} · http ${response.httpStatus || "-"}`} className="replay-evidence-request-panel">
      <div className="replay-evidence-grid">
        <KV label="method" value={request.method} />
        <KV label="url" value={request.targetUrl} />
        <KV label="http status" value={String(response.httpStatus || "-")} />
        <KV label="body" value={response.bodySummary || "-"} />
      </div>
    </Panel>
  );
}

function SystemsPanel({ evidence }) {
  const systems = evidence.systems || [];
  const matched = systems.filter((system) => system.found).length;
  return (
    <Panel title="Systems" summary={`${matched}/${systems.length} matched`} className="replay-evidence-systems-panel">
      <div className="replay-evidence-systems">
        {systems.length ? systems.map((system) => (
          <article className="replay-evidence-system-card" key={system.id || system.name}>
            <div>
              <strong>{system.name || system.id || "-"}</strong>
              <span className={`status-pill ${system.found ? "passed" : ""}`}>{system.found ? "matched" : "empty"}</span>
            </div>
            <pre>{(system.coreLogs || []).slice(0, 4).join("\n") || system.note || "No matching logs"}</pre>
          </article>
        )) : <div className="empty-note">暂无系统证据。</div>}
      </div>
    </Panel>
  );
}

function ReplayEvidenceApp() {
  const [payload, setPayload] = useState(null);
  const [message, setMessage] = useState("loading");
  const traceID = queryParam("traceId");

  async function refresh() {
    if (!traceID) {
      setMessage("failed");
      setPayload({ error: "traceId is required" });
      return;
    }
    setMessage("refreshing...");
    try {
      setPayload(await fetchJSON(`/api/replay/evidence?traceId=${encodeURIComponent(traceID)}`));
      setMessage("ready");
    } catch (error) {
      setPayload({ error: error.message });
      setMessage("failed");
    }
  }

  useEffect(() => {
    refresh();
  }, []);

  const run = payload?.run || {};
  const evidence = payload?.evidence || {};
  const title = payload?.error || run.traceId || evidence.traceId || traceID || "trace loading";

  return (
    <main className="app replay-evidence-page">
      <section className="topbar">
        <div>
          <h1>Replay evidence</h1>
          <p>{title}</p>
        </div>
        <div className="actions">
          <span className="workflow-step-status-pill" role="status">{message}</span>
          <a className="button-link" href="/workflow-detail.html?id=sandbox.replay_probe_observability">Replay / Probe</a>
          <a className="button-link" href="/">控制台</a>
          <a className="button-link" href="/dashboard.html">环境大盘</a>
        </div>
      </section>
      <section className="replay-evidence-shell">
        <RunPanel payload={payload || {}} />
        <RequestPanel evidence={evidence} />
        <SystemsPanel evidence={evidence} />
      </section>
    </main>
  );
}

createRoot(document.getElementById("react-replay-evidence-root")).render(<ReplayEvidenceApp />);
