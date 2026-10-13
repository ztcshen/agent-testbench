import { useEffect, useState } from "react";
import {
  Library,
  Plus,
  RefreshCw,
  GitBranch,
  Save,
  ArrowLeft,
  ArrowUpRight,
  Trash2,
  Copy,
} from "lucide-react";
import { api } from "./api.mjs";
import { Breadcrumb, Empty, Modal, SearchInput } from "./components.jsx";
import ScenarioEditor from "./ScenarioEditor.jsx";
import { StepBlockParameterDefinitions } from "./StepBlockInputs.jsx";
import {
  captureStepBlock,
  newStepBlock,
  inferBlockParameters,
  parameterValueValid,
} from "./stepBlockModel.mjs";
import {
  buildStudioSavePayload,
  validateStudioDraft,
} from "../../control-plane/frontend/src/workflowStudioModel.mjs";

export default function StepLibrary({
  workspace,
  openWorkflow,
  onSaved,
  notify,
  initialID = "",
  draftState,
  onDraft,
}) {
  const [data, setData] = useState({ blocks: [], etag: "" });
  const [localDraft, setLocalDraft] = useState(null);
  const draft = draftState === undefined ? localDraft : draftState;
  const setDraft = (value) => {
    const next = typeof value === "function" ? value(draft) : value;
    if (onDraft) onDraft(next);
    else setLocalDraft(next);
  };
  const [query, setQuery] = useState("");
  const [busy, setBusy] = useState(false);
  const [importing, setImporting] = useState(false);
  const [sourceID, setSourceID] = useState("");
  const [deleting, setDeleting] = useState(null);
  const [discarding, setDiscarding] = useState(false);
  const [error, setError] = useState("");
  const cases = workspace.studio.cases || [];
  async function load() {
    const response = await api("/api/step-library");
    const next = { ...response.data, etag: response.etag };
    setData(next);
    return next;
  }
  useEffect(() => {
    load()
      .then((next) => {
        if (initialID && !draft) {
          const record = next.blocks.find(
            (item) => item.definition.id === initialID,
          );
          if (record)
            setDraft({ ...structuredClone(record.definition), dirty: false });
        }
      })
      .catch((error) => setError(error.message));
  }, [initialID, workspace.library?.revision]);
  const scene = draft
    ? {
        workflow: {
          id: draft.id,
          displayName: draft.displayName,
          description: draft.description,
        },
        steps: draft.steps,
      }
    : null;
  const errors = scene ? validateStudioDraft(scene, cases) : [];
  if (draft)
    for (const parameter of draft.parameters) {
      if (!/^[A-Za-z0-9][A-Za-z0-9._-]*$/.test(parameter.name))
        errors.push("填写有效的参数名称");
      if (
        parameter.hasDefault &&
        parameter.defaultValue !== "[REDACTED]" &&
        !parameterValueValid(parameter, parameter.defaultValue)
      )
        errors.push(`${parameter.name} 的默认值应为 ${parameter.dataType}`);
    }
  async function save() {
    if (errors.length) {
      setError(errors[0]);
      return;
    }
    setBusy(true);
    setError("");
    try {
      const definition = {
        ...draft,
        steps: buildStudioSavePayload(scene).steps,
      };
      const response = await api("/api/step-library", {
        method: "PUT",
        etag: data.etag,
        body: { definition },
      });
      setData({ ...response.data, etag: response.etag });
      setDraft(null);
      await onSaved();
      notify("步骤组已保存，已有场景继续使用各自引用的版本");
    } catch (error) {
      setError(error.message);
    } finally {
      setBusy(false);
    }
  }
  async function remove() {
    setBusy(true);
    setError("");
    try {
      const response = await api("/api/step-library", {
        method: "DELETE",
        etag: data.etag,
        body: { id: deleting.definition.id },
      });
      setData({ ...response.data, etag: response.etag });
      setDeleting(null);
      await onSaved();
      notify("步骤组已删除");
    } catch (error) {
      setError(error.message);
      setDeleting(null);
    } finally {
      setBusy(false);
    }
  }
  const matching = data.blocks.filter((item) =>
    `${item.definition.displayName} ${item.definition.description} ${item.definition.id}`
      .toLowerCase()
      .includes(query.toLowerCase()),
  );
  const record = data.blocks.find((item) => item.definition.id === draft?.id);
  if (draft)
    return (
      <div className="step-library-editor">
        <div className="library-edit-toolbar">
          <button
            className="text-button"
            disabled={busy}
            onClick={() => {
              if (draft.dirty) setDiscarding(true);
              else setDraft(null);
            }}
          >
            <ArrowLeft size={14} />
            步骤库
          </button>
          <span className="soft-tag">
            {draft.revision ? `版本 ${draft.revision.slice(0, 8)}` : "新定义"}
          </span>
          <span className="muted">{record?.usages.length || 0} 处引用</span>
        </div>
        {error && <div className="notice error">{error}</div>}
        <StepBlockParameterDefinitions
          parameters={draft.parameters}
          steps={draft.steps}
          disabled={busy}
          patch={(parameters) =>
            setDraft({ ...draft, parameters, dirty: true })
          }
        />
        <ScenarioEditor
          draft={scene}
          cases={cases}
          busy={busy}
          libraryMode
          save={save}
          run={() => {}}
          patch={(next) =>
            setDraft((current) => ({
              ...current,
              displayName: next.workflow.displayName,
              description: next.workflow.description,
              steps: next.steps,
              parameters: inferBlockParameters(
                next.steps,
                cases,
                current.parameters,
              ),
              dirty: true,
            }))
          }
          extraErrors={errors}
        />
        {record?.usages.length > 0 && (
          <section className="library-usages">
            <h3>被引用的场景</h3>
            {record.usages.map((usage) => (
              <button
                className="usage-row"
                key={usage.workflowId + usage.instanceId}
                onClick={() => openWorkflow(usage.workflowId)}
              >
                <GitBranch size={14} />
                <strong>{usage.workflowName}</strong>
                <code>{usage.revision.slice(0, 8)}</code>
                {usage.outdated && <span className="soft-tag">可更新</span>}
                <ArrowUpRight size={14} />
              </button>
            ))}
          </section>
        )}
        {discarding && (
          <Modal title="放弃步骤组修改？" onClose={() => setDiscarding(false)}>
            <div className="modal-body">
              <p>当前定义还没有保存。可返回继续编辑，或放弃本次修改。</p>
              <button
                className="button primary"
                onClick={() => {
                  setDiscarding(false);
                  setDraft(null);
                }}
              >
                放弃修改
              </button>
            </div>
          </Modal>
        )}
      </div>
    );
  return (
    <div className="step-library">
      <div className="editor-heading">
        <div>
          <Breadcrumb items={["自动化测试", "步骤库"]} />
          <div className="editor-title">
            <Library size={21} className="accent" />
            <h2>复用步骤库</h2>
            <span className="soft-tag">{data.blocks.length} 个步骤组</span>
          </div>
          <p className="muted">
            把公共流程整理成步骤组，在不同场景中引用并独立填写参数。
          </p>
        </div>
        <div className="heading-actions">
          <button
            className="button"
            disabled={busy}
            onClick={() => load().catch((error) => setError(error.message))}
          >
            <RefreshCw size={14} />
            刷新
          </button>
          <button
            className="button"
            disabled={busy || !workspace.studio.workflows?.length}
            onClick={() => setImporting(true)}
          >
            <Copy size={14} />
            从已有场景创建
          </button>
          <button
            className="button primary"
            onClick={() => setDraft(newStepBlock())}
          >
            <Plus size={14} />
            新建步骤组
          </button>
        </div>
      </div>
      {error && <div className="notice error">{error}</div>}
      <div className="library-search">
        <SearchInput
          value={query}
          onChange={setQuery}
          placeholder="搜索步骤组名称、说明或 ID"
        />
      </div>
      {!matching.length ? (
        <Empty
          icon={Library}
          title={query ? "没有匹配的步骤组" : "建立第一个复用步骤组"}
        >
          从已有场景创建，或在场景编排中选取连续步骤保存到步骤库。
        </Empty>
      ) : (
        <div className="library-grid">
          {matching.map((item) => (
            <article className="library-card" key={item.definition.id}>
              <header>
                <Library size={18} />
                <strong>{item.definition.displayName}</strong>
                <code>{item.definition.revision.slice(0, 8)}</code>
              </header>
              <p>{item.definition.description || "公共测试流程"}</p>
              <div className="library-metrics">
                <span>{item.definition.steps.length} 个步骤</span>
                <span>{item.definition.parameters.length} 个参数</span>
                <span>{item.usages.length} 处引用</span>
              </div>
              <div className="library-step-preview">
                {item.definition.steps.slice(0, 3).map((step) => (
                  <span key={step.id}>{step.title || step.caseId}</span>
                ))}
                {item.definition.steps.length > 3 && <span>…</span>}
              </div>
              {item.usages.some((usage) => usage.outdated) && (
                <div className="notice">
                  {item.usages.filter((usage) => usage.outdated).length}{" "}
                  处引用可升级
                </div>
              )}
              <footer>
                <button
                  className="button"
                  onClick={() =>
                    setDraft({
                      ...structuredClone(item.definition),
                      dirty: false,
                    })
                  }
                >
                  <Save size={13} />
                  编辑定义与引用
                </button>
                <button
                  className="text-button danger"
                  disabled={busy || item.usages.length > 0}
                  title={
                    item.usages.length
                      ? "仍被场景引用，需先移除引用"
                      : "删除步骤组"
                  }
                  onClick={() => setDeleting(item)}
                >
                  <Trash2 size={13} />
                  删除
                </button>
              </footer>
            </article>
          ))}
        </div>
      )}
      {importing && (
        <Modal title="从已有场景创建步骤组" onClose={() => setImporting(false)}>
          <div className="modal-body">
            <p>选择已保存的场景，公共参数会自动汇总。</p>
            <label className="field">
              来源场景
              <select
                aria-label="步骤组来源场景"
                value={sourceID}
                onChange={(event) => setSourceID(event.target.value)}
              >
                <option value="">选择场景</option>
                {workspace.studio.workflows.map((workflow) => (
                  <option
                    key={workflow.workflow.id}
                    value={workflow.workflow.id}
                  >
                    {workflow.workflow.displayName} · {workflow.steps.length} 步
                  </option>
                ))}
              </select>
            </label>
            <button
              className="button primary"
              disabled={!sourceID}
              onClick={() => {
                try {
                  const source = workspace.studio.workflows.find(
                    (item) => item.workflow.id === sourceID,
                  );
                  setDraft(captureStepBlock(source, cases));
                  setImporting(false);
                } catch (error) {
                  setError(error.message);
                  setImporting(false);
                }
              }}
            >
              <Plus size={14} />
              创建草稿
            </button>
          </div>
        </Modal>
      )}
      {deleting && (
        <Modal title="删除步骤组" onClose={() => setDeleting(null)}>
          <div className="modal-body">
            <p>
              删除「{deleting.definition.displayName}」及其未被引用的历史版本。
            </p>
            <button className="button primary" disabled={busy} onClick={remove}>
              确认删除
            </button>
          </div>
        </Modal>
      )}
    </div>
  );
}
