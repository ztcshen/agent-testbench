import { useEffect, useMemo, useState } from "react";

import {
  buildCaseEditorDraft,
  buildNewCaseEditorDraft,
  fetchCaseCatalog,
  fetchCaseCatalogHistory,
  patchCaseCatalog,
  rollbackCaseCatalog,
} from "./apiCasesApi.mjs";

const lifecycleStatuses = ["draft", "review", "active", "quarantined", "deprecated"];

function TextField({ label, value, onChange, list = "", type = "text", readOnly = false }) {
  return (
    <label className="workflow-filter">
      <span>{label}</span>
      <input
        type={type}
        list={list || undefined}
        readOnly={readOnly}
        spellCheck="false"
        value={value}
        onChange={(event) => onChange(event.target.value)}
      />
    </label>
  );
}

function SelectField({ label, value, onChange, children }) {
  return (
    <label className="workflow-filter">
      <span>{label}</span>
      <select className="workflow-filter-clear" value={value} onChange={(event) => onChange(event.target.value)}>
        {children}
      </select>
    </label>
  );
}

function CatalogConflict({ currentRevision, onRefresh, disabled }) {
  return (
    <div className="api-case-result failed" role="alert">
      <strong>Catalog 已被其他维护者更新</strong>
      <p>{`Store 当前版本为 r${currentRevision || "?"}。本地草稿尚未覆盖远端，请刷新后重新确认修改。`}</p>
      <button type="button" disabled={disabled} onClick={onRefresh}>刷新 Store 版本（丢弃当前草稿）</button>
    </div>
  );
}

function CatalogHistory({ items, currentRevision, busy, onRollback }) {
  return (
    <div className="api-case-workflow-sequence-list" aria-label="Case catalog history">
      {items.length ? items.map((item) => {
        const summary = item.summary || {};
        const detail = [item.operation || "update", summary.caseId, `${item.caseCount || 0} cases`].filter(Boolean).join(" · ");
        return (
          <article className={`api-case-workflow-step ${item.revision === currentRevision ? "selected" : ""}`.trim()} key={item.revision}>
            <div>
              <span>{`r${item.revision}`}</span>
              <strong>{detail}</strong>
            </div>
            <div>
              <code>{formatTimestamp(item.createdAt)}</code>
              <small>{String(item.sha256 || "").slice(0, 12) || "no digest"}</small>
            </div>
            <div className="api-case-workflow-step-actions">
              <button
                type="button"
                disabled={busy || item.revision === currentRevision}
                onClick={() => onRollback(item.revision)}
              >
                {item.revision === currentRevision ? "当前版本" : "回滚"}
              </button>
            </div>
          </article>
        );
      }) : <p className="api-case-muted">暂无可展示的 Catalog 历史。</p>}
    </div>
  );
}

export function CaseCatalogMaintenance({ selectedCaseID = "", onCatalogChanged }) {
  const [snapshot, setSnapshot] = useState(null);
  const [etag, setETag] = useState("");
  const [draft, setDraft] = useState(null);
  const [message, setMessage] = useState("loading...");
  const [busy, setBusy] = useState(false);
  const [conflictRevision, setConflictRevision] = useState(0);
  const [historyOpen, setHistoryOpen] = useState(false);
  const [history, setHistory] = useState([]);
  const [creating, setCreating] = useState(false);
  const [editingCaseID, setEditingCaseID] = useState(selectedCaseID);

  const selectedCatalogCase = useMemo(
    () => (snapshot?.cases || []).find((item) => item.id === editingCaseID) || null,
    [snapshot, editingCaseID],
  );

  async function refreshCatalog(nextMessage = "ready", preferredCaseID = editingCaseID || selectedCaseID) {
    setBusy(true);
    setMessage("loading catalog...");
    try {
      const next = await fetchCaseCatalog();
      const nextCase = (next.payload.cases || []).find((item) => item.id === preferredCaseID) || null;
      setSnapshot(next.payload);
      setETag(next.etag);
      setEditingCaseID(preferredCaseID);
      setDraft(nextCase ? buildCaseEditorDraft(nextCase) : null);
      setConflictRevision(0);
      setMessage(nextMessage);
    } catch (error) {
      setMessage(error.message);
    } finally {
      setBusy(false);
    }
  }

  async function loadHistory() {
    setBusy(true);
    setMessage("loading history...");
    try {
      const payload = await fetchCaseCatalogHistory();
      setHistory(payload.items || []);
      setHistoryOpen(true);
      setMessage("history ready");
    } catch (error) {
      setMessage(error.message);
    } finally {
      setBusy(false);
    }
  }

  useEffect(() => {
    setCreating(false);
    setEditingCaseID(selectedCaseID);
    refreshCatalog("ready", selectedCaseID);
  }, [selectedCaseID]);

  function updateDraft(field, value) {
    setDraft((current) => ({ ...current, [field]: value }));
  }

  function handleMutationError(error) {
    if (error.status === 412) {
      setConflictRevision(Number(error.payload?.currentRevision || 0));
      setMessage("revision conflict");
      return;
    }
    setMessage(error.message);
  }

  function startCreate() {
    setCreating(true);
    setEditingCaseID("");
    setDraft(buildNewCaseEditorDraft());
    setConflictRevision(0);
    setMessage("new draft");
  }

  function discardDraft() {
    const selected = (snapshot?.cases || []).find((item) => item.id === selectedCaseID) || null;
    setCreating(false);
    setEditingCaseID(selectedCaseID);
    setDraft(selected ? buildCaseEditorDraft(selected) : null);
    refreshCatalog("refreshed", selectedCaseID);
  }

  async function saveCase(event) {
    event.preventDefault();
    if (!draft) return;
    setBusy(true);
    setMessage("saving...");
    try {
      const mutation = await patchCaseCatalog(draft, etag);
      const savedCaseID = mutation.payload.case?.id || draft.id;
      setEditingCaseID(savedCaseID);
      await refreshCatalog(`saved r${mutation.payload.revision}`, savedCaseID);
      setCreating(false);
      if (historyOpen) await loadHistory();
      await onCatalogChanged?.(savedCaseID);
    } catch (error) {
      handleMutationError(error);
    } finally {
      setBusy(false);
    }
  }

  async function rollback(revision) {
    if (!window.confirm(`确认将整个 Case Catalog 回滚到 r${revision}？系统会创建一个新的修订版本。`)) return;
    setBusy(true);
    setMessage(`rolling back to r${revision}...`);
    try {
      const mutation = await rollbackCaseCatalog(revision, etag);
      await refreshCatalog(`rolled back to r${revision} as r${mutation.payload.revision}`);
      await loadHistory();
      await onCatalogChanged?.();
    } catch (error) {
      handleMutationError(error);
    } finally {
      setBusy(false);
    }
  }

  const revision = Number(snapshot?.revision || 0);
  return (
    <section className="api-case-management-panel" aria-label="Case catalog maintenance">
      <div className="section-head compact-head">
        <div>
          <h3>Case Catalog 维护</h3>
          <p>{`Store revision r${revision || "-"} · ${message}`}</p>
        </div>
        <div className="actions">
          <button type="button" disabled={busy || creating} onClick={() => refreshCatalog("refreshed", editingCaseID || selectedCaseID)}>刷新</button>
          <button type="button" disabled={busy || creating} onClick={startCreate}>新建草稿</button>
          <button type="button" disabled={busy} onClick={() => historyOpen ? setHistoryOpen(false) : loadHistory()}>
            {historyOpen ? "收起历史" : "查看历史"}
          </button>
        </div>
      </div>

      {conflictRevision ? <CatalogConflict currentRevision={conflictRevision} disabled={busy} onRefresh={discardDraft} /> : null}

      {draft && (selectedCatalogCase || creating) ? (
        <form className="api-case-trigger" onSubmit={saveCase}>
          <p>仅维护通用元数据与生命周期；执行源、请求覆盖值和密钥不会在此表单展示或修改。</p>
          <div className="api-case-management-toolbar">
            <TextField label={creating ? "Case ID" : "Case ID（只读）"} value={draft.id} readOnly={!creating} onChange={(value) => updateDraft("id", value)} />
            <TextField label="显示名称" value={draft.displayName} onChange={(value) => updateDraft("displayName", value)} />
            <TextField label="描述" value={draft.description} onChange={(value) => updateDraft("description", value)} />
            <TextField label="Owner" value={draft.owner} onChange={(value) => updateDraft("owner", value)} />
            <TextField label="Priority" value={draft.priority} onChange={(value) => updateDraft("priority", value)} />
            <TextField label="Tags（逗号分隔）" value={draft.tagsText} onChange={(value) => updateDraft("tagsText", value)} />
            <TextField label="Interface Node" list="case-catalog-node-options" value={draft.nodeId} onChange={(value) => updateDraft("nodeId", value)} />
            <TextField label="Request Template" list="case-catalog-template-options" value={draft.requestTemplateId} onChange={(value) => updateDraft("requestTemplateId", value)} />
            <TextField label="Case Type" value={draft.caseType} onChange={(value) => updateDraft("caseType", value)} />
            <TextField label="Scenario" value={draft.scenario} onChange={(value) => updateDraft("scenario", value)} />
            <TextField label="Render Mode" value={draft.renderMode} onChange={(value) => updateDraft("renderMode", value)} />
            <TextField label="Sort Order" type="number" value={draft.sortOrder} onChange={(value) => updateDraft("sortOrder", value)} />
            <SelectField label="Lifecycle" value={draft.status} onChange={(value) => updateDraft("status", value)}>
              {lifecycleStatuses.map((status) => <option value={status} key={status}>{status}</option>)}
            </SelectField>
            <SelectField label="Admission 必需" value={String(draft.requiredForAdmission)} onChange={(value) => updateDraft("requiredForAdmission", value === "true")}>
              <option value="false">false</option>
              <option value="true">true</option>
            </SelectField>
          </div>
          <datalist id="case-catalog-node-options">
            {(snapshot.interfaceNodes || []).map((item) => <option value={item.id} key={item.id}>{item.displayName || item.id}</option>)}
          </datalist>
          <datalist id="case-catalog-template-options">
            {(snapshot.requestTemplates || []).map((item) => <option value={item.id} key={item.id}>{item.displayName || item.id}</option>)}
          </datalist>
          <div className="actions">
            <button className="primary-action" type="submit" disabled={busy || Boolean(conflictRevision) || !String(draft.id || "").trim()}>
              {creating ? "创建 Draft" : "保存为新修订"}
            </button>
            {creating ? <button type="button" disabled={busy} onClick={discardDraft}>取消新建</button> : null}
            <span className="api-case-readiness">{selectedCatalogCase?.sourceKind || "source unconfigured"}</span>
          </div>
        </form>
      ) : (
        <p className="api-case-muted">当前所选用例不在可维护的 Store Catalog 中，请刷新或选择其他用例。</p>
      )}

      {historyOpen ? <CatalogHistory items={history} currentRevision={revision} busy={busy} onRollback={rollback} /> : null}
    </section>
  );
}

function formatTimestamp(value) {
  const parsed = new Date(value || 0);
  return Number.isNaN(parsed.getTime()) ? "unknown time" : parsed.toLocaleString();
}
