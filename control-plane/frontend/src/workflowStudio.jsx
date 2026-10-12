import React, { useEffect, useMemo, useState } from "react";
import { createRoot } from "react-dom/client";
import { Background, Controls, Handle, MiniMap, Position, ReactFlow, ReactFlowProvider, applyNodeChanges, useReactFlow } from "@xyflow/react";
import { ArrowDown, ArrowUp, Check, ChevronRight, Code2, Database, FileJson, GitBranch, Layers, ListTree, Play, Plus, RefreshCw, Save, Search, Trash2, X } from "lucide-react";
import "@xyflow/react/dist/style.css";
import "./workflowStudio.css";
import { fetchStudio, fetchStudioRun, saveStudio, startStudioRun } from "./workflowStudioApi.mjs";
import { addStudioStep, emptyStudioDraft, moveStudioStep, removeStudioStep, setStudioBinding, studioCanvas, studioRunnable, studioSourceSteps, updateStudioStep, validateStudioDraft } from "./workflowStudioModel.mjs";

const statusLabels = { passed: "通过", failed: "失败", running: "运行中", pending: "等待", skipped: "跳过" };

function StudioStepNode({ data, selected }) {
  const { step, index, caseDef, result } = data;
  return <div className={`studio-node ${selected ? "selected" : ""} ${result?.status || ""}`}>
    <Handle id="previous" type="target" position={Position.Top} isConnectable={false} />
    <Handle id="next" type="source" position={Position.Bottom} isConnectable={false} />
    <Handle id="input" type="target" position={Position.Left} isConnectable={false} className="studio-data-handle" />
    <Handle id="output" type="source" position={Position.Right} isConnectable={false} className="studio-data-handle" />
    <div className="studio-node-head"><span className="studio-step-number">{String(index + 1).padStart(2, "0")}</span><strong>{step.title || caseDef?.displayName || step.id}</strong><span className={`studio-method ${caseDef?.method?.toLowerCase()}`}>{caseDef?.method || "API"}</span></div>
    <code className="studio-node-path">{caseDef?.path || step.caseId}</code>
    <div className="studio-node-foot"><span>{(step.bindings || []).length ? `${step.bindings.length} 个参数引用` : "沿用用例参数"}</span><span className={`studio-status ${result?.status || ""}`}>{result ? statusLabels[result.status] || result.status : step.required ? "必需步骤" : "可选步骤"}</span></div>
  </div>;
}
const nodeTypes = { studioStep: StudioStepNode };

function StudioField({ label, children }) {
  return <label className="studio-field"><span>{label}</span>{children}</label>;
}

function CaseLibrary({ cases, onSelect, onClose }) {
  const [query, setQuery] = useState("");
  const visible = cases.filter(item => `${item.displayName} ${item.id} ${item.method} ${item.path}`.toLowerCase().includes(query.toLowerCase()));
  return <div className="studio-dialog-backdrop"><section className="studio-dialog" role="dialog" aria-modal="true" aria-label="添加接口步骤">
    <header><div><h2>添加接口步骤</h2><p>从当前用例目录选择，步骤配置保存在场景中。</p></div><button type="button" aria-label="关闭接口选择" onClick={onClose}><X size={18} /></button></header>
    <div className="studio-search"><Search size={16} /><input autoFocus placeholder="搜索接口、用例或路径" value={query} onChange={event => setQuery(event.target.value)} /></div>
    <div className="studio-case-library">{visible.length ? visible.map(item => <button type="button" key={item.id} onClick={() => onSelect(item)}><span className={`studio-method ${item.method?.toLowerCase()}`}>{item.method || "API"}</span><span><strong>{item.displayName}</strong><code>{item.path || item.id}</code></span><small>{item.executionReady ? "可执行" : "未就绪"}</small><Plus size={16} /></button>) : <div className="studio-empty">没有匹配用例。<a href="/api-cases.html">维护接口用例</a></div>}</div>
  </section></div>;
}

function ParameterBindings({ draft, step, cases, onChange }) {
  const caseDef = cases.find(item => item.id === step.caseId);
  const sources = studioSourceSteps(draft, step.id);
  const variables = [...new Set([...(step.variables || caseDef?.variables || []), ...(step.bindings || []).map(item => item.parameter)])];
  const bindingFor = name => (step.bindings || []).find(item => item.parameter === name);
  function patch(name, changes) {
    onChange(setStudioBinding(draft, step.id, name, { ...bindingFor(name), ...changes }));
  }
  return <div className="studio-inspector-body">
    <div className="studio-note"><GitBranch size={16} /><span>选择前序步骤的字段作为模板参数。引用只作用于当前场景步骤。</span></div>
    {variables.length ? variables.map(name => {
      const binding = bindingFor(name);
      const source = sources.find(item => item.id === binding?.sourceStepId);
      const fields = cases.find(item => item.id === source?.caseId)?.responseFields || [];
      const listId = `source-${step.id}-${name}`;
      return <section className="studio-binding" key={name}>
        <h3><Code2 size={15} />{name}<span>{binding ? "步骤引用" : "用例默认 / 运行输入"}</span></h3>
        <StudioField label="数据来源"><select aria-label={`${name} 来源步骤`} value={binding?.sourceStepId || ""} onChange={event => event.target.value ? patch(name, { sourceStepId: event.target.value, from: "responsebody", path: "" }) : onChange(setStudioBinding(draft, step.id, name, null))}><option value="">沿用用例配置</option>{sources.map((item, index) => <option key={item.id} value={item.id}>{`${index + 1}. ${item.title || item.id}`}</option>)}</select></StudioField>
        {binding ? <><div className="studio-field-pair"><StudioField label="提取位置"><select aria-label={`${name} 提取位置`} value={binding.from} onChange={event => patch(name, { from: event.target.value })}><option value="responsebody">响应体</option><option value="requestbody">请求体</option></select></StudioField><StudioField label="字段路径"><input aria-label={`${name} 字段路径`} list={listId} value={binding.path} placeholder="$.data.id" onChange={event => patch(name, { path: event.target.value })} /><datalist id={listId}>{fields.map(path => <option key={path} value={path} />)}</datalist></StudioField></div><div className="studio-binding-preview"><span>绑定预览</span><strong>{source?.title || binding.sourceStepId}</strong><ChevronRight size={13} /><code>{binding.path || "选择字段"}</code><ChevronRight size={13} /><code>{name}</code></div></> : null}
      </section>;
    }) : <div className="studio-empty"><Code2 size={25} /><strong>没有可绑定的模板参数</strong><p>请求使用当前用例的既有配置。</p></div>}
  </div>;
}

function OutputFields({ step, onPatch }) {
  const outputs = step.outputs || [];
  function update(index, patch) { onPatch({ outputs: outputs.map((item, i) => i === index ? { ...item, ...patch } : item) }); }
  return <div className="studio-inspector-body"><div className="studio-note"><FileJson size={16} /><span>命名输出可供既有流程使用。参数引用所需输出会随场景一起保存。</span></div>
    {outputs.map((item, index) => <section className="studio-binding" key={index}><div className="studio-row"><input aria-label={`输出变量 ${index + 1}`} placeholder="变量名称" value={item.name} onChange={event => update(index, { name: event.target.value })} /><button type="button" aria-label={`删除输出 ${index + 1}`} onClick={() => onPatch({ outputs: outputs.filter((_, i) => i !== index) })}><Trash2 size={14} /></button></div><div className="studio-field-pair"><StudioField label="提取位置"><select value={item.from} onChange={event => update(index, { from: event.target.value })}><option value="responsebody">响应体</option><option value="requestbody">请求体</option></select></StudioField><StudioField label="字段路径"><input aria-label={`输出路径 ${index + 1}`} placeholder="$.data.id" value={item.path} onChange={event => update(index, { path: event.target.value })} /></StudioField></div></section>)}
    <button className="studio-outline" type="button" onClick={() => onPatch({ outputs: [...outputs, { name: "", from: "responsebody", path: "" }] })}><Plus size={15} />添加输出变量</button>
  </div>;
}

function Inspector({ draft, step, cases, result, onChange, onRemove, tab, setTab }) {
  const caseDef = cases.find(item => item.id === step?.caseId);
  if (!step) return <aside className="studio-inspector"><div className="studio-empty"><Layers size={30} /><strong>选择一个步骤</strong><p>配置参数来源、查看请求与运行结果。</p></div></aside>;
  const onPatch = patch => onChange(updateStudioStep(draft, step.id, patch));
  return <aside className="studio-inspector"><header className="studio-inspector-head"><div><small>步骤配置</small><strong>{step.title}</strong></div><button type="button" aria-label="删除当前步骤" onClick={onRemove}><Trash2 size={16} /></button></header>
    <div className="studio-inspector-meta"><StudioField label="步骤名称"><input aria-label="步骤名称" value={step.title || ""} onChange={event => onPatch({ title: event.target.value })} /></StudioField><div className="studio-reference"><Database size={14} /><span>引用用例</span><a href={`/api-cases.html?case=${encodeURIComponent(step.caseId)}`}>{caseDef?.displayName || step.caseId}</a></div><label className="studio-checkbox"><input type="checkbox" checked={step.required} onChange={event => onPatch({ required: event.target.checked })} />作为必需验收步骤</label></div>
    <div className="studio-tabs" role="tablist" aria-label="步骤详情">{[["request", "请求"], ["bindings", "参数"], ["outputs", "输出"], ["result", "结果"]].map(([id, label]) => <button type="button" role="tab" aria-selected={tab === id} className={tab === id ? "active" : ""} key={id} onClick={() => setTab(id)}>{label}{id === "bindings" && step.bindings?.length ? <em>{step.bindings.length}</em> : null}</button>)}</div>
    {tab === "bindings" ? <ParameterBindings draft={draft} step={step} cases={cases} onChange={onChange} /> : null}
    {tab === "outputs" ? <OutputFields step={step} onPatch={onPatch} /> : null}
    {tab === "request" ? <div className="studio-inspector-body"><div className="studio-request-title"><span className={`studio-method ${caseDef?.method?.toLowerCase()}`}>{caseDef?.method || "API"}</span><code>{caseDef?.path || "请求由执行器维护"}</code></div><p className="studio-muted">基础请求来自关联用例；场景参数引用会在执行时填充。</p><pre>{JSON.stringify(step.request || caseDef?.request || {}, null, 2)}</pre></div> : null}
    {tab === "result" ? <div className="studio-inspector-body">{result ? <><span className={`studio-status ${result.status}`}>{statusLabels[result.status] || result.status}</span><p>{result.elapsedMs != null ? `${result.elapsedMs} ms` : ""}</p>{result.error ? <p className="studio-error">{result.error}</p> : null}<div className="studio-reference">运行：<code>{result.runId}</code></div>{result.viewerUrl ? <a className="studio-outline" href={result.viewerUrl} target="_blank" rel="noreferrer">查看请求、响应与断言证据</a> : null}</> : <div className="studio-empty"><Play size={25} /><strong>尚未运行</strong><p>保存场景后开始执行。</p></div>}</div> : null}
  </aside>;
}

function RunPanel({ run, onSelect }) {
  const [open, setOpen] = useState(true);
  if (!run) return null;
  return <section className={`studio-run-panel ${open ? "open" : ""}`}><header><button type="button" onClick={() => setOpen(!open)}><ListTree size={16} /><strong>运行结果</strong><span className={`studio-status ${run.status}`}>{statusLabels[run.status] || run.status}</span></button><span>{run.completed || 0}/{run.total || 0} 步骤 · {run.passed || 0} 通过 · {run.failed || 0} 失败</span></header>{open ? <><div className="studio-run-list">{(run.cases || []).map(item => <button type="button" key={item.stepId || item.caseId} onClick={() => onSelect(item.stepId)}><span className={`studio-status ${item.status}`}>{statusLabels[item.status] || item.status}</span><strong>{item.displayName || item.stepId}</strong><code>{item.method} {item.path}</code><small>{item.elapsedMs != null ? `${item.elapsedMs} ms` : ""}</small></button>)}</div>{run.status !== "running" && run.acceptance ? <div className={`studio-acceptance ${run.acceptance.ok ? "passed" : ""}`}>{run.acceptance.ok ? "验收证据齐备" : "业务步骤结果已记录；拓扑或验收证据尚未齐备"}{run.htmlReportUrl ? <a href={run.htmlReportUrl} target="_blank" rel="noreferrer">完整报告</a> : null}</div> : null}</> : null}</section>;
}

function WorkflowStudio() {
  const { fitView } = useReactFlow();
  const [snapshot, setSnapshot] = useState(null);
  const [etag, setETag] = useState("");
  const [draft, setDraft] = useState(null);
  const [selectedId, setSelectedId] = useState("");
  const [tab, setTab] = useState("bindings");
  const [dirty, setDirty] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [message, setMessage] = useState("正在读取场景目录");
  const [library, setLibrary] = useState(false);
  const [query, setQuery] = useState("");
  const [run, setRun] = useState(null);
  const [view, setView] = useState("canvas");
  const cases = snapshot?.cases || [];
  const selected = draft?.steps.find(step => step.id === selectedId);
  const validation = useMemo(() => draft ? validateStudioDraft(draft, cases) : [], [draft, snapshot]);
  const graph = useMemo(() => draft ? studioCanvas(draft, cases, run?.cases || [], selectedId) : { nodes: [], edges: [] }, [draft, snapshot, run, selectedId]);

  function loadDraft(value) {
    setDraft(structuredClone(value));
    setSelectedId(value.steps[0]?.id || "");
    setDirty(false);
    setRun(null);
    setError("");
  }
  async function refresh(preferredId = draft?.workflow.id, discard = false) {
    if (dirty && !discard && !window.confirm("重新读取将放弃未保存的场景修改，继续？")) return;
    setBusy(true);
    try {
      const next = await fetchStudio();
      setSnapshot(next.payload); setETag(next.etag);
      const requested = preferredId || new URLSearchParams(window.location.search).get("workflow");
      const found = next.payload.workflows.find(item => item.workflow.id === requested) || next.payload.workflows[0];
      if (found) loadDraft(found); else { setDraft(null); setDirty(false); }
      setMessage("目录已同步"); setError("");
    } catch (problem) { setError(problem.message); setMessage("目录读取失败"); } finally { setBusy(false); }
  }
  useEffect(() => { refresh(); }, []);
  useEffect(() => {
    if (!draft?.steps.length || view !== "canvas") return;
    const timer = setTimeout(() => fitView({ padding: 0.24, maxZoom: 1, duration: 160 }), 120);
    return () => clearTimeout(timer);
  }, [draft?.workflow.id, draft?.steps.length, Boolean(run), view, fitView]);
  useEffect(() => {
    const guard = event => { if (dirty) { event.preventDefault(); event.returnValue = ""; } };
    window.addEventListener("beforeunload", guard);
    return () => window.removeEventListener("beforeunload", guard);
  }, [dirty]);
  useEffect(() => {
    if (run?.status !== "running") return;
    let live = true;
    const timer = setTimeout(async () => {
      try { const next = await fetchStudioRun(run.batchRunId); if (live) setRun(next); }
      catch (problem) { if (live) { setError(problem.message); setRun(current => ({ ...current, status: "unknown" })); } }
    }, 700);
    return () => { live = false; clearTimeout(timer); };
  }, [run]);
  function edit(next) { setDraft(next); setDirty(true); setError(""); }
  function selectWorkflow(value) {
    if (dirty && !window.confirm("切换场景将放弃未保存修改，继续？")) return;
    loadDraft(value);
  }
  function create() {
    if (dirty && !window.confirm("新建场景将放弃未保存修改，继续？")) return;
    const value = emptyStudioDraft(); setDraft(value); setSelectedId(""); setDirty(true); setRun(null); setError("");
  }
  async function save() {
    if (validation.length) return;
    setBusy(true); setError("");
    try {
      const next = await saveStudio(draft, etag);
      setSnapshot(next.payload); setETag(next.etag);
      const saved = next.payload.workflows.find(item => item.workflow.id === draft.workflow.id);
      setDraft(saved); setDirty(false); setMessage("场景已保存");
    } catch (problem) {
      setError(problem.status === 412 ? "目录已被其他维护者更新，当前草稿已保留。请重新读取后核对修改。" : problem.message);
    } finally { setBusy(false); }
  }
  async function execute() {
    setBusy(true); setError("");
    try {
      const current = await fetchStudio();
      if (current.etag !== etag) throw new Error("场景目录已有新版本，请重新读取并核对后运行。");
      setRun(await startStudioRun(draft.workflow.id)); setMessage("场景开始执行");
    } catch (problem) { setError(problem.message); } finally { setBusy(false); }
  }
  function selectStep(id, showResult = false) { setSelectedId(id); if (showResult) setTab("result"); }
  const running = run?.status === "running";
  const readonly = busy || running;
  const workflows = (snapshot?.workflows || []).filter(item => `${item.workflow.displayName} ${item.workflow.id}`.toLowerCase().includes(query.toLowerCase()));
  const runDisabled = readonly || dirty || !draft || validation.length > 0 || !studioRunnable(draft, cases);

  return <div className="studio-app"><header className="studio-topbar"><a className="studio-brand" href="/"><span><GitBranch size={20} /></span>AgentTestBench</a><nav><a href="/workflows.html">场景目录</a><a href="/api-cases.html">接口用例</a><strong>可视化工作台</strong></nav><span className="studio-store"><Database size={14} />{snapshot?.profileId || "读取目录"}{snapshot ? <small>r{snapshot.revision}</small> : null}</span></header>
    <div className="studio-workspace"><aside className="studio-sidebar"><header><strong><Layers size={16} />测试场景</strong><button type="button" title="新建场景" aria-label="新建场景" disabled={!snapshot || readonly} onClick={create}><Plus size={17} /></button></header><div className="studio-search"><Search size={14} /><input aria-label="搜索场景" placeholder="搜索场景" value={query} onChange={event => setQuery(event.target.value)} /></div><div className="studio-scene-list">{workflows.map(item => <button type="button" className={draft?.workflow.id === item.workflow.id ? "active" : ""} key={item.workflow.id} disabled={readonly} onClick={() => selectWorkflow(item)}><GitBranch size={16} /><span><strong>{item.workflow.displayName}</strong><small>{item.steps.length} 个步骤</small></span></button>)}{!workflows.length ? <p className="studio-muted">当前目录没有场景。</p> : null}</div>
      <div className="studio-sidebar-bottom"><button type="button" onClick={() => refresh()} disabled={readonly}><RefreshCw size={14} />重新读取</button><a href="/api-cases.html">维护接口用例 <ChevronRight size={14} /></a></div></aside>
      <main className="studio-main"><header className="studio-toolbar"><div><small>自动化测试 / 场景编排</small>{draft ? <input aria-label="场景名称" value={draft.workflow.displayName} disabled={readonly} onChange={event => edit({ ...draft, workflow: { ...draft.workflow, displayName: event.target.value } })} /> : <h1>场景工作台</h1>}</div><div className="studio-toolbar-actions"><span className={dirty ? "studio-dirty" : "studio-muted"}>{dirty ? "有未保存修改" : message}</span><button type="button" disabled={!draft || readonly || validation.length > 0} onClick={save}><Save size={15} />保存场景</button><button type="button" className="studio-primary" disabled={runDisabled} title={dirty ? "保存场景后运行" : "运行已保存场景"} onClick={execute}><Play size={15} />{running ? "运行中" : "运行场景"}</button></div></header>
        {error ? <div className="studio-error" role="alert">{error}<button type="button" aria-label="关闭提示" onClick={() => setError("")}><X size={14} /></button></div> : null}
        {validation.length ? <div className="studio-validation" role="status">{validation.slice(0, 3).join("；")}</div> : null}
        {draft ? <><div className="studio-canvas-toolbar"><div className="studio-view-switch"><button type="button" className={view === "canvas" ? "active" : ""} onClick={() => setView("canvas")}><GitBranch size={14} />流程图</button><button type="button" className={view === "list" ? "active" : ""} onClick={() => setView("list")}><ListTree size={14} />步骤列表</button></div><div className="studio-legend"><span>● 执行顺序</span><span className="data">● 参数引用</span></div><button type="button" className="studio-outline" disabled={readonly} onClick={() => setLibrary(true)}><Plus size={15} />添加步骤</button></div><div className="studio-editor-area">
          <div className="studio-canvas">{draft.steps.length ? view === "canvas" ? <ReactFlow nodes={graph.nodes} edges={graph.edges} nodeTypes={nodeTypes} fitView fitViewOptions={{ padding: 0.22, maxZoom: 1 }} nodesDraggable={!readonly} nodesConnectable={false} deleteKeyCode={null} onNodeClick={(_event, node) => selectStep(node.id)} onNodesChange={changes => { if (readonly) return; const moved = changes.filter(item => item.type === "position" && item.position); if (!moved.length) return; const positions = new Map(applyNodeChanges(moved, graph.nodes).map(node => [node.id, node.position])); edit({ ...draft, steps: draft.steps.map(step => ({ ...step, position: positions.get(step.id) || step.position })) }); }}><Background gap={22} size={1} color="#d7dfe9" /><Controls showInteractive={false} /><MiniMap nodeColor="#c9ddff" maskColor="rgba(247,249,253,.65)" pannable zoomable /></ReactFlow> : <div className="studio-step-list">{draft.steps.map((step, index) => <article key={step.id} className={selectedId === step.id ? "selected" : ""} draggable={!readonly} onDragStart={event => event.dataTransfer.setData("text/plain", step.id)} onDragOver={event => event.preventDefault()} onDrop={event => { event.preventDefault(); if (!readonly) edit(moveStudioStep(draft, event.dataTransfer.getData("text/plain"), index)); }}><button type="button" onClick={() => selectStep(step.id)}><span className="studio-step-number">{index + 1}</span><strong>{step.title}</strong><small>{step.caseId}</small></button><button type="button" title="上移" disabled={readonly || index === 0} onClick={() => edit(moveStudioStep(draft, step.id, index - 1))}><ArrowUp size={14} /></button><button type="button" title="下移" disabled={readonly || index === draft.steps.length - 1} onClick={() => edit(moveStudioStep(draft, step.id, index + 1))}><ArrowDown size={14} /></button></article>)}</div> : <div className="studio-empty studio-canvas-empty"><span><GitBranch size={35} /></span><h2>把接口组装成测试场景</h2><p>添加已登记用例，安排步骤顺序，再选择上游参数来源。</p><button type="button" className="studio-primary" onClick={() => setLibrary(true)} disabled={readonly}><Plus size={16} />添加第一个步骤</button></div>}</div>
          <fieldset className="studio-inspector-wrap" disabled={readonly}><Inspector draft={draft} step={selected} cases={cases} result={run?.cases?.find(item => item.stepId === selectedId)} onChange={edit} onRemove={() => { edit(removeStudioStep(draft, selectedId)); setSelectedId(""); }} tab={tab} setTab={setTab} /></fieldset>
        </div><RunPanel run={run} onSelect={id => selectStep(id, true)} /></> : <div className="studio-empty studio-start"><span><GitBranch size={40} /></span><h1>搭建可复用的测试场景</h1><p>接口用例、步骤配置、运行记录使用同一份目录。</p><button type="button" className="studio-primary" disabled={!snapshot || readonly} onClick={create}><Plus size={17} />新建场景</button></div>}
      </main></div>{library ? <CaseLibrary cases={cases} onClose={() => setLibrary(false)} onSelect={item => { const next = addStudioStep(draft, item); edit(next); setSelectedId(next.steps.at(-1).id); setTab("bindings"); setLibrary(false); }} /> : null}
  </div>;
}

createRoot(document.getElementById("react-workflow-studio-root")).render(<ReactFlowProvider><WorkflowStudio /></ReactFlowProvider>);
