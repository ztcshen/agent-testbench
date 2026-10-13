import { Fragment, useEffect, useState } from "react";
import {
  Play,
  Save,
  Plus,
  GripVertical,
  ChevronRight,
  GitBranch,
  ArrowUp,
  ArrowDown,
  Trash2,
  X,
  Braces,
  ArrowRight,
  LoaderCircle,
  CheckCircle2,
  ShieldCheck,
  Library,
  ChevronDown,
} from "lucide-react";
import {
  addStudioStep,
  updateStudioStep,
  removeStudioStep,
  moveStudioStep,
  studioSourceSteps,
  setStudioBinding,
  validateStudioDraft,
} from "../../control-plane/frontend/src/workflowStudioModel.mjs";
import {
  IconButton,
  Method,
  Tabs,
  Status,
  Empty,
  Code,
  Modal,
  SearchInput,
  Breadcrumb,
} from "./components.jsx";
import { filterRequests, statusLabel } from "./model.mjs";
import StepBlockInputs from "./StepBlockInputs.jsx";
import ScenarioRunStep from "./ScenarioRunStep.jsx";
import {
  addStepBlock,
  captureStepBlock,
  validateStepGroups,
} from "./stepBlockModel.mjs";

function StepInspector({ draft, step, cases, patch, onClose }) {
  const caseDef = cases.find((item) => item.id === step.caseId) || {};
  const sources = studioSourceSteps(draft, step.id);
  const variables = step.variables || caseDef.variables || [];
  const update = (value) => patch(updateStudioStep(draft, step.id, value));
  function bind(parameter, value) {
    patch(setStudioBinding(draft, step.id, parameter, value));
  }
  return (
    <aside className="step-inspector">
      <header>
        <strong>步骤设置</strong>
        <IconButton icon={X} title="关闭步骤设置" onClick={onClose} />
      </header>
      <div className="inspector-content">
        <label className="field">
          步骤名称
          <input
            aria-label="步骤名称"
            value={step.title}
            onChange={(event) => update({ title: event.target.value })}
          />
        </label>
        <div className="inspector-request">
          <Method value={caseDef.method} />
          <code>{caseDef.path}</code>
        </div>
        <label className="check-field">
          <input
            type="checkbox"
            checked={step.required}
            onChange={(event) => update({ required: event.target.checked })}
          />
          该步骤必须通过
        </label>
        <h4>
          <Braces size={14} />
          请求参数与数据来源
        </h4>
        {variables.length ? (
          variables.map((parameter) => {
            const binding = (step.bindings || []).find(
              (item) => item.parameter === parameter,
            );
            const invalid =
              binding &&
              !sources.some((item) => item.id === binding.sourceStepId);
            return (
              <div
                className={`binding-editor ${invalid ? "invalid" : ""}`}
                key={parameter}
              >
                <div className="binding-name">
                  <code>{parameter}</code>
                  <span>{binding ? "前序步骤" : "接口默认值"}</span>
                </div>
                <label className="field">
                  数据来源
                  <select
                    aria-label={`参数 ${parameter} 的来源`}
                    value={binding?.sourceStepId || ""}
                    onChange={(event) =>
                      bind(parameter, {
                        ...binding,
                        sourceStepId: event.target.value,
                        path: binding?.path || "$.id",
                      })
                    }
                  >
                    <option value="">使用接口默认值</option>
                    {invalid && (
                      <option value={binding.sourceStepId}>
                        来源已移除或顺序无效
                      </option>
                    )}
                    {sources.map((source, index) => (
                      <option key={source.id} value={source.id}>
                        {index + 1}. {source.title}
                      </option>
                    ))}
                  </select>
                </label>
                {binding && (
                  <>
                    <label className="field">
                      读取内容
                      <select
                        aria-label={`参数 ${parameter} 的内容`}
                        value={binding.from}
                        onChange={(event) =>
                          bind(parameter, {
                            ...binding,
                            from: event.target.value,
                          })
                        }
                      >
                        <option value="responsebody">响应体 Body</option>
                        <option value="requestbody">请求体 Body</option>
                      </select>
                    </label>
                    <label className="field">
                      字段路径
                      <input
                        aria-label={`参数 ${parameter} 的字段路径`}
                        value={binding.path}
                        placeholder="$.data.id"
                        onChange={(event) =>
                          bind(parameter, {
                            ...binding,
                            path: event.target.value,
                          })
                        }
                      />
                    </label>
                    <div className="binding-preview">
                      <span>前序输出</span>
                      <ArrowRight size={12} />
                      <code>{parameter}</code>
                    </div>
                  </>
                )}
              </div>
            );
          })
        ) : (
          <p className="muted inspector-note">
            该请求没有模板参数。可在接口编辑器中使用 {"{{override:变量名}}"}{" "}
            定义需要传入的字段。
          </p>
        )}
        <h4>
          <GitBranch size={14} />
          提取输出变量
        </h4>
        {(step.outputs || []).map((output, index) => (
          <div className="output-row" key={index}>
            <input
              aria-label={`输出变量名 ${index + 1}`}
              placeholder="变量名"
              value={output.name}
              onChange={(event) =>
                update({
                  outputs: step.outputs.map((item, i) =>
                    i === index ? { ...item, name: event.target.value } : item,
                  ),
                })
              }
            />
            <input
              aria-label={`输出字段路径 ${index + 1}`}
              placeholder="$.data.id"
              value={output.path}
              onChange={(event) =>
                update({
                  outputs: step.outputs.map((item, i) =>
                    i === index ? { ...item, path: event.target.value } : item,
                  ),
                })
              }
            />
            <IconButton
              icon={Trash2}
              title={`移除输出变量 ${index + 1}`}
              onClick={() =>
                update({ outputs: step.outputs.filter((_, i) => i !== index) })
              }
            />
          </div>
        ))}
        <button
          className="text-button"
          onClick={() =>
            update({
              outputs: [
                ...(step.outputs || []),
                { name: "", from: "responsebody", path: "$.id" },
              ],
            })
          }
        >
          <Plus size={13} />
          添加提取变量
        </button>
        <details className="request-preview">
          <summary>请求配置预览</summary>
          <Code value={step.request || caseDef.request || {}} />
        </details>
      </div>
    </aside>
  );
}

export default function ScenarioEditor({
  draft,
  cases,
  patch,
  save,
  run,
  report,
  busy,
  verificationEnvironments = [],
  inspectEnvironment,
  blocks = [],
  saveBlock,
  openLibrary,
  libraryMode = false,
  extraErrors = [],
  openEvidence,
}) {
  const [selected, setSelected] = useState(draft.steps[0]?.id || "");
  const [tab, setTab] = useState("steps");
  const [adding, setAdding] = useState(false);
  const [query, setQuery] = useState("");
  const [dragging, setDragging] = useState("");
  const [collapsed, setCollapsed] = useState({});
  const [capture, setCapture] = useState(null);
  const [capturing, setCapturing] = useState(false);
  const [captureError, setCaptureError] = useState("");
  useEffect(() => {
    setSelected(draft.steps[0]?.id || "");
  }, [draft.workflow.id]);
  const step = draft.steps.find((item) => item.id === selected);
  const errors = [
    ...validateStudioDraft(draft, cases),
    ...validateStepGroups(draft),
    ...extraErrors,
  ];
  const results = report?.steps || report?.cases || [];
  const resultByID = new Map(results.map((item) => [item.stepId, item]));
  const available = cases.filter((item) =>
    `${item.displayName} ${item.method} ${item.path}`
      .toLowerCase()
      .includes(query.toLowerCase()),
  );
  function add(item) {
    const next = addStudioStep(draft, item);
    patch(next);
    setSelected(next.steps.at(-1).id);
    setAdding(false);
  }
  function addBlock(definition) {
    const next = addStepBlock(draft, definition);
    patch(next);
    setSelected(next.steps.at(-1).id);
    setAdding(false);
  }
  async function captureBlock() {
    setCapturing(true);
    setCaptureError("");
    try {
      const definition = captureStepBlock(
        draft,
        cases,
        capture.start,
        capture.end,
      );
      definition.displayName = capture.name;
      await saveBlock(definition);
      setCapture(null);
    } catch (error) {
      setCaptureError(error.message);
    } finally {
      setCapturing(false);
    }
  }
  return (
    <div className="scenario-editor">
      <div className="editor-heading">
        <div>
          <Breadcrumb
            items={["自动化测试", libraryMode ? "步骤组定义" : "场景用例"]}
          />
          <div className="editor-title">
            <GitBranch size={20} className="accent" />
            <input
              aria-label="场景名称"
              value={draft.workflow.displayName}
              onChange={(event) =>
                patch({
                  ...draft,
                  workflow: {
                    ...draft.workflow,
                    displayName: event.target.value,
                  },
                })
              }
            />
            {draft.dirty && <span className="unsaved-label">未保存</span>}
          </div>
        </div>
        <div className="heading-actions">
          <button
            className="button"
            disabled={busy || errors.length > 0}
            onClick={save}
          >
            <Save size={14} />
            保存
          </button>
          {libraryMode ? null : verificationEnvironments.length > 0 ? (
            <button
              className="button primary"
              disabled={busy || draft.dirty}
              onClick={inspectEnvironment}
            >
              <ShieldCheck size={14} />
              查看环境验收
            </button>
          ) : (
            <button
              className="button primary"
              disabled={busy || !draft.steps.length || errors.length > 0}
              onClick={run}
            >
              {busy ? (
                <LoaderCircle size={14} className="spin" />
              ) : (
                <Play size={14} />
              )}
              运行场景
            </button>
          )}
        </div>
      </div>
      <Tabs
        items={[
          { id: "steps", label: "步骤编排", count: draft.steps.length },
          { id: "results", label: "运行结果" },
          { id: "description", label: "场景说明" },
        ]}
        value={tab}
        onChange={setTab}
      />
      {errors.length > 0 && <div className="notice error">{errors[0]}</div>}
      {verificationEnvironments.length > 0 && (
        <div className="notice">
          <ShieldCheck size={14} />
          该场景用于环境验收：
          {verificationEnvironments
            .map((item) => item.displayName || item.id)
            .join("、")}
          。请先保存修改，再到运行环境中执行验收。
        </div>
      )}
      {tab === "steps" ? (
        <div className="scenario-workspace">
          <div className="scenario-step-area">
            <div className="scenario-toolbar">
              <span>
                <GitBranch size={14} />
                按顺序执行 <b>{draft.steps.length}</b> 个步骤
              </span>
              {!libraryMode && saveBlock && (
                <button
                  className="button"
                  disabled={busy || !draft.steps.length}
                  onClick={() => {
                    setCaptureError("");
                    setCapture({
                      name: `${draft.workflow.displayName} · 公共步骤`,
                      start: 0,
                      end: draft.steps.length - 1,
                    });
                  }}
                >
                  <Library size={14} />
                  保存为步骤组
                </button>
              )}
              <button
                className="button"
                disabled={busy}
                onClick={() => setAdding(true)}
              >
                <Plus size={14} />
                添加步骤
              </button>
            </div>
            <div className="step-sequence">
              <div className="sequence-cap">
                <span className="sequence-dot" />
                开始
              </div>
              {draft.steps.map((item, index) => {
                const caseDef =
                  cases.find((value) => value.id === item.caseId) || {};
                const result = resultByID.get(item.id);
                const group = item.group;
                const first =
                  group && draft.steps[index - 1]?.group?.id !== group.id;
                const members = group
                  ? draft.steps.filter((step) => step.group?.id === group.id)
                  : [];
                const folded = group && collapsed[group.id] !== false;
                return (
                  <Fragment key={item.id}>
                    {first && (
                      <div
                        className={`sequence-row group-sequence ${step?.group?.id === group.id ? "selected" : ""}`}
                        onDragOver={(event) => event.preventDefault()}
                        onDrop={(event) => {
                          event.preventDefault();
                          if (dragging)
                            patch(moveStudioStep(draft, dragging, index));
                          setDragging("");
                        }}
                      >
                        <div className="sequence-index">{index + 1}</div>
                        <div
                          className="sequence-card group-sequence-card"
                          onClick={() => setSelected(item.id)}
                        >
                          <div className="step-card-heading">
                            <span
                              className="drag-handle"
                              draggable={!busy}
                              onDragStart={() => setDragging(item.id)}
                              onDragEnd={() => setDragging("")}
                            >
                              <GripVertical size={16} />
                            </span>
                            <Library size={16} className="accent" />
                            <strong>{group.title}</strong>
                            <span className="soft-tag">
                              {members.length} 步
                            </span>
                            <code className="group-revision">
                              {group.revision.slice(0, 8)}
                            </code>
                            <div className="step-actions">
                              <IconButton
                                icon={ArrowUp}
                                title={`上移步骤组 ${group.title}`}
                                disabled={busy || index === 0}
                                onClick={(event) => {
                                  event.stopPropagation();
                                  patch(
                                    moveStudioStep(draft, item.id, index - 1),
                                  );
                                }}
                              />
                              <IconButton
                                icon={ArrowDown}
                                title={`下移步骤组 ${group.title}`}
                                disabled={
                                  busy ||
                                  index + members.length === draft.steps.length
                                }
                                onClick={(event) => {
                                  event.stopPropagation();
                                  patch(
                                    moveStudioStep(
                                      draft,
                                      item.id,
                                      index + members.length,
                                    ),
                                  );
                                }}
                              />
                              <IconButton
                                icon={Trash2}
                                title={`移除步骤组 ${group.title}`}
                                disabled={busy}
                                onClick={(event) => {
                                  event.stopPropagation();
                                  patch(removeStudioStep(draft, item.id));
                                }}
                              />
                              <IconButton
                                icon={folded ? ChevronRight : ChevronDown}
                                title={`${folded ? "展开" : "折叠"}步骤组 ${group.title}`}
                                onClick={(event) => {
                                  event.stopPropagation();
                                  setCollapsed({
                                    ...collapsed,
                                    [group.id]: !folded,
                                  });
                                }}
                              />
                            </div>
                          </div>
                          <div className="group-parameter-chips">
                            {(group.parameters || []).map((parameter) => (
                              <span className="soft-tag" key={parameter.name}>
                                <Braces size={11} />
                                {parameter.name}
                                <span>
                                  {(group.bindings || []).some(
                                    (b) => b.parameter === parameter.name,
                                  )
                                    ? "前序输出"
                                    : Object.hasOwn(
                                          group.values || {},
                                          parameter.name,
                                        )
                                      ? "固定值"
                                      : parameter.hasDefault
                                        ? "默认值"
                                        : "待填写"}
                                </span>
                              </span>
                            ))}
                          </div>
                        </div>
                      </div>
                    )}
                    <div
                      hidden={Boolean(folded)}
                      className={`sequence-row ${group ? "group-member" : ""} ${item.id === selected ? "selected" : ""}`}
                      key={item.id}
                      onDragOver={(event) => event.preventDefault()}
                      onDrop={(event) => {
                        event.preventDefault();
                        if (dragging)
                          patch(moveStudioStep(draft, dragging, index));
                        setDragging("");
                      }}
                    >
                      <div className="sequence-index">{index + 1}</div>
                      <div
                        className="sequence-card"
                        onClick={() => setSelected(item.id)}
                      >
                        <div className="step-card-heading">
                          <span
                            className="drag-handle"
                            draggable={!busy}
                            onDragStart={() => setDragging(item.id)}
                            onDragEnd={() => setDragging("")}
                            title="拖动调整步骤顺序"
                          >
                            <GripVertical size={16} />
                          </span>
                          <Method value={caseDef.method} />
                          <strong>{item.title}</strong>
                          {result ? (
                            <Status value={result.status} />
                          ) : (
                            <span className="soft-tag">
                              {item.required ? "必选" : "可选"}
                            </span>
                          )}
                          <div className="step-actions">
                            <IconButton
                              icon={ArrowUp}
                              title={`上移步骤 ${index + 1}`}
                              disabled={busy || Boolean(group) || index === 0}
                              onClick={(event) => {
                                event.stopPropagation();
                                patch(
                                  moveStudioStep(draft, item.id, index - 1),
                                );
                              }}
                            />
                            <IconButton
                              icon={ArrowDown}
                              title={`下移步骤 ${index + 1}`}
                              disabled={
                                busy ||
                                Boolean(group) ||
                                index === draft.steps.length - 1
                              }
                              onClick={(event) => {
                                event.stopPropagation();
                                patch(
                                  moveStudioStep(draft, item.id, index + 1),
                                );
                              }}
                            />
                            <IconButton
                              icon={Trash2}
                              title={`删除步骤 ${index + 1}`}
                              disabled={busy || Boolean(group)}
                              onClick={(event) => {
                                event.stopPropagation();
                                patch(removeStudioStep(draft, item.id));
                              }}
                            />
                          </div>
                          <ChevronRight size={14} />
                        </div>
                        <code className="step-path">
                          {caseDef.path || item.caseId}
                        </code>
                        {(item.bindings || []).map((binding) => (
                          <div className="step-binding" key={binding.parameter}>
                            <Braces size={12} />
                            <code>{binding.parameter}</code>
                            <span>
                              ←{" "}
                              {draft.steps.find(
                                (source) => source.id === binding.sourceStepId,
                              )?.title || "来源已移除"}
                            </span>
                            <code>{binding.path}</code>
                          </div>
                        ))}
                      </div>
                    </div>
                  </Fragment>
                );
              })}
              <button
                className="add-sequence"
                disabled={busy}
                onClick={() => setAdding(true)}
              >
                <Plus size={16} />
                添加测试步骤
              </button>
              <div className="sequence-cap end">
                <span className="sequence-dot" />
                结束
              </div>
            </div>
            {report && (
              <div className="scenario-inline-result">
                <Status value={report.status} />
                <span>
                  {report.summary?.passed ??
                    results.filter((item) => item.status === "passed")
                      .length}{" "}
                  个步骤通过
                </span>
                <button
                  className="text-button"
                  onClick={() => setTab("results")}
                >
                  查看运行结果
                  <ChevronRight size={12} />
                </button>
              </div>
            )}
          </div>
          {step?.group ? (
            <StepBlockInputs
              draft={draft}
              group={step.group}
              blocks={blocks}
              patch={patch}
              close={() => setSelected("")}
              openLibrary={openLibrary}
              busy={busy}
            />
          ) : (
            step && (
              <StepInspector
                draft={draft}
                step={step}
                cases={cases}
                patch={patch}
                onClose={() => setSelected("")}
              />
            )
          )}
        </div>
      ) : tab === "description" ? (
        <div className="description-editor">
          <label className="field">
            场景说明
            <textarea
              aria-label="场景说明"
              rows={8}
              placeholder="说明此场景的前置条件、执行目的与预期结果"
              value={draft.workflow.description}
              onChange={(event) =>
                patch({
                  ...draft,
                  workflow: {
                    ...draft.workflow,
                    description: event.target.value,
                  },
                })
              }
            />
          </label>
        </div>
      ) : !report ? (
        <Empty icon={Play} title="还没有运行结果">
          保存场景后点击「运行场景」，执行结果会在这里展示。
        </Empty>
      ) : (
        <div className="scenario-results">
          <div className="run-summary">
            <Status value={report.status} />
            <span>{results.length} 个步骤</span>
            <span className="muted">{report.batchRunId}</span>
          </div>
          <div className="notice">
            <ShieldCheck size={15} />
            环境验收：{report.acceptance?.ok ? "通过" : "未通过"}
            。步骤执行结果与环境验收分别记录。
          </div>
          <div className="report-steps">
            {results.map((item, index) => (
              <ScenarioRunStep
                key={item.caseRunId || item.stepId || index}
                item={item}
                index={index}
                step={draft.steps.find((step) => step.id === item.stepId)}
                openEvidence={openEvidence}
                locate={() => {
                  const found = draft.steps.find(
                    (step) => step.id === item.stepId,
                  );
                  if (found) {
                    setSelected(found.id);
                    if (found.group)
                      setCollapsed((current) => ({
                        ...current,
                        [found.group.id]: false,
                      }));
                    setTab("steps");
                  }
                }}
              />
            ))}
          </div>
          <details>
            <summary>完整报告与 Evidence</summary>
            <Code value={report} />
          </details>
        </div>
      )}
      {adding && (
        <Modal title="从接口添加测试步骤" wide onClose={() => setAdding(false)}>
          <div className="modal-body">
            <SearchInput
              value={query}
              onChange={setQuery}
              placeholder="搜索接口名称或路径"
            />
            {!libraryMode && blocks.length > 0 && (
              <div className="add-block-list">
                <h4>引用步骤组</h4>
                {blocks
                  .filter((item) =>
                    item.definition.displayName
                      .toLowerCase()
                      .includes(query.toLowerCase()),
                  )
                  .map((item) => (
                    <button
                      className="add-block-item"
                      key={item.definition.id}
                      onClick={() => addBlock(item.definition)}
                    >
                      <Library size={17} />
                      <span>
                        <strong>{item.definition.displayName}</strong>
                        <small>
                          {item.definition.steps.length} 步 ·{" "}
                          {item.definition.parameters.length} 个参数 ·{" "}
                          {item.definition.revision.slice(0, 8)}
                        </small>
                      </span>
                      <Plus size={14} />
                    </button>
                  ))}
                <h4>独立接口步骤</h4>
              </div>
            )}
            <div className="add-case-list">
              {available.map((item) => (
                <button key={item.id} onClick={() => add(item)}>
                  <Method value={item.method} />
                  <span>
                    <strong>{item.displayName}</strong>
                    <code>{item.path}</code>
                  </span>
                  <Plus size={15} />
                </button>
              ))}
              {!available.length && (
                <Empty title="没有可添加的接口">
                  先在接口管理中保存一个接口。
                </Empty>
              )}
            </div>
          </div>
        </Modal>
      )}
      {capture && (
        <Modal
          title="保存连续步骤到步骤库"
          onClose={() => {
            if (!capturing) setCapture(null);
          }}
        >
          <div className="modal-body">
            <label className="field">
              步骤组名称
              <input
                aria-label="新步骤组名称"
                value={capture.name}
                onChange={(event) =>
                  setCapture({ ...capture, name: event.target.value })
                }
              />
            </label>
            <div className="capture-range">
              <label className="field">
                开始步骤
                <select
                  aria-label="步骤组开始步骤"
                  value={capture.start}
                  onChange={(event) =>
                    setCapture({
                      ...capture,
                      start: Number(event.target.value),
                      end: Math.max(Number(event.target.value), capture.end),
                    })
                  }
                >
                  {draft.steps.map((step, index) => (
                    <option key={step.id} value={index}>
                      {index + 1}. {step.title}
                    </option>
                  ))}
                </select>
              </label>
              <label className="field">
                结束步骤
                <select
                  aria-label="步骤组结束步骤"
                  value={capture.end}
                  onChange={(event) =>
                    setCapture({ ...capture, end: Number(event.target.value) })
                  }
                >
                  {draft.steps.map(
                    (step, index) =>
                      index >= capture.start && (
                        <option key={step.id} value={index}>
                          {index + 1}. {step.title}
                        </option>
                      ),
                  )}
                </select>
              </label>
            </div>
            <p className="muted">
              组内数据绑定会保留；外部来源会转换成步骤组输入参数。
            </p>
            {captureError && <div className="notice error">{captureError}</div>}
            <button
              className="button primary"
              disabled={capturing || !capture.name.trim()}
              onClick={captureBlock}
            >
              <Save size={14} />
              {capturing ? "正在保存" : "保存到步骤库"}
            </button>
          </div>
        </Modal>
      )}
    </div>
  );
}
