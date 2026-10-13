import { Braces, X, ArrowUpRight, RefreshCw } from "lucide-react";
import { IconButton, Code } from "./components.jsx";
import {
  parseParameterValue,
  updateStepGroup,
  upgradeStepGroup,
  parameterTypes,
} from "./stepBlockModel.mjs";

export function ParameterValue({
  parameter,
  value,
  onChange,
  label,
  disabled = false,
}) {
  const masked = value === "[REDACTED]";
  if (parameter.dataType === "boolean")
    return (
      <select
        aria-label={label}
        disabled={disabled}
        value={typeof value === "boolean" ? String(value) : ""}
        onChange={(event) => onChange(event.target.value === "true")}
      >
        <option value="" disabled>
          选择布尔值
        </option>
        <option value="true">true</option>
        <option value="false">false</option>
      </select>
    );
  const text = masked
    ? ""
    : typeof value === "string"
      ? value
      : value == null
        ? ""
        : JSON.stringify(value);
  const input =
    parameter.dataType === "object" || parameter.dataType === "array" ? (
      <textarea
        aria-label={label}
        disabled={disabled}
        rows={2}
        value={text}
        placeholder={parameter.dataType === "array" ? "[]" : "{}"}
        onChange={(event) =>
          onChange(parseParameterValue(parameter.dataType, event.target.value))
        }
      />
    ) : (
      <input
        aria-label={label}
        disabled={disabled || (value === null && parameter.nullable)}
        type={masked ? "password" : "text"}
        value={text}
        placeholder={masked ? "已保存密值，输入可替换" : parameter.dataType}
        onChange={(event) =>
          onChange(parseParameterValue(parameter.dataType, event.target.value))
        }
      />
    );
  return (
    <div className="parameter-value">
      {input}
      {parameter.nullable && (
        <label className="check-field">
          <input
            type="checkbox"
            aria-label={`${label} 使用 null`}
            checked={value === null}
            disabled={disabled}
            onChange={(event) =>
              onChange(
                event.target.checked
                  ? null
                  : parameter.dataType === "string"
                    ? ""
                    : undefined,
              )
            }
          />
          null
        </label>
      )}
    </div>
  );
}

export function StepBlockParameterDefinitions({
  parameters,
  patch,
  steps,
  disabled,
}) {
  const update = (index, value) =>
    patch(
      parameters.map((item, i) => (i === index ? { ...item, ...value } : item)),
    );
  return (
    <section className="block-parameter-definitions">
      <header>
        <Braces size={15} />
        <strong>步骤组参数</strong>
        <span className="muted">未绑定的请求变量自动汇总在这里</span>
      </header>
      {!parameters.length && (
        <p className="muted">当前步骤没有需要传入的参数。</p>
      )}
      {parameters.map((parameter, index) => (
        <div className="block-parameter-definition" key={index}>
          <label className="field">
            参数名
            <input
              aria-label={`步骤组参数名 ${index + 1}`}
              value={parameter.name}
              disabled={disabled}
              onChange={(event) => update(index, { name: event.target.value })}
            />
          </label>
          <label className="field">
            类型
            <select
              aria-label={`参数 ${parameter.name} 类型`}
              value={parameter.dataType}
              disabled={disabled}
              onChange={(event) =>
                update(index, { dataType: event.target.value })
              }
            >
              {parameterTypes.map((type) => (
                <option key={type}>{type}</option>
              ))}
            </select>
          </label>
          <div className="parameter-options">
            <label className="check-field">
              <input
                aria-label={`参数 ${parameter.name} 必填`}
                type="checkbox"
                checked={parameter.required}
                disabled={disabled}
                onChange={(event) =>
                  update(index, { required: event.target.checked })
                }
              />
              必填
            </label>
            <label className="check-field">
              <input
                aria-label={`参数 ${parameter.name} 允许 null`}
                type="checkbox"
                checked={Boolean(parameter.nullable)}
                disabled={disabled}
                onChange={(event) =>
                  update(index, { nullable: event.target.checked })
                }
              />
              允许 null
            </label>
            <label className="check-field">
              <input
                aria-label={`参数 ${parameter.name} 默认值`}
                type="checkbox"
                checked={parameter.hasDefault}
                disabled={disabled}
                onChange={(event) =>
                  update(index, {
                    hasDefault: event.target.checked,
                    defaultValue: event.target.checked
                      ? parameter.dataType === "boolean"
                        ? false
                        : parameter.dataType === "number" ||
                            parameter.dataType === "integer"
                          ? 0
                          : parameter.dataType === "object"
                            ? {}
                            : parameter.dataType === "array"
                              ? []
                              : ""
                      : null,
                  })
                }
              />
              默认值
            </label>
          </div>
          {parameter.hasDefault && (
            <ParameterValue
              parameter={parameter}
              value={parameter.defaultValue}
              disabled={disabled}
              label={`参数 ${parameter.name} 默认内容`}
              onChange={(value) => update(index, { defaultValue: value })}
            />
          )}
          <div className="parameter-targets">
            用于：
            {parameter.targets.map((target) => (
              <span
                className="soft-tag"
                key={`${target.stepId}:${target.parameter}`}
              >
                {steps.find((step) => step.id === target.stepId)?.title ||
                  target.stepId}{" "}
                · {target.parameter}
              </span>
            ))}
          </div>
        </div>
      ))}
    </section>
  );
}

export default function StepBlockInputs({
  draft,
  group,
  blocks,
  patch,
  close,
  openLibrary,
  busy,
}) {
  const start = draft.steps.findIndex((step) => step.group?.id === group.id);
  const sources = draft.steps.slice(0, start);
  const latest = blocks.find(
    (item) => item.definition.id === group.blockId,
  )?.definition;
  const update = (value) => patch(updateStepGroup(draft, group.id, value));
  function change(name, mode, value) {
    const values = { ...group.values };
    let bindings = (group.bindings || []).filter(
      (binding) => binding.parameter !== name,
    );
    delete values[name];
    if (mode === "fixed") values[name] = value;
    if (mode === "previous")
      bindings.push({
        parameter: name,
        sourceStepId: value?.sourceStepId || sources.at(-1)?.id || "",
        from: value?.from || "responsebody",
        path: value?.path || "$.id",
      });
    update({ values, bindings });
  }
  return (
    <aside className="step-inspector block-instance-inspector">
      <header>
        <strong>步骤组参数</strong>
        <IconButton icon={X} title="关闭步骤组参数" onClick={close} />
      </header>
      <div className="inspector-content">
        <label className="field">
          引用名称
          <input
            aria-label="步骤组引用名称"
            disabled={busy}
            value={group.title}
            onChange={(event) => update({ title: event.target.value })}
          />
        </label>
        <div className="block-version">
          <span>引用版本</span>
          <code>{group.revision.slice(0, 8)}</code>
          <button
            className="text-button"
            onClick={() => openLibrary?.(group.blockId)}
          >
            <ArrowUpRight size={12} />
            查看定义与引用
          </button>
        </div>
        {latest && latest.revision !== group.revision && (
          <div className="notice block-update">
            步骤库有新版本
            <button
              className="button"
              disabled={busy}
              onClick={() => patch(upgradeStepGroup(draft, group.id, latest))}
            >
              <RefreshCw size={12} />
              更新此引用
            </button>
          </div>
        )}
        <p className="muted">参数仅影响这次引用；保存后按所选版本执行。</p>
        {(group.parameters || []).map((parameter) => {
          const binding = (group.bindings || []).find(
            (item) => item.parameter === parameter.name,
          );
          const mode = binding
            ? "previous"
            : Object.hasOwn(group.values || {}, parameter.name)
              ? "fixed"
              : "default";
          const value =
            mode === "fixed"
              ? group.values[parameter.name]
              : parameter.defaultValue;
          return (
            <div className="block-call-parameter" key={parameter.name}>
              <div className="binding-name">
                <code>{parameter.name}</code>
                <span>
                  {parameter.dataType}
                  {parameter.required ? " · 必填" : ""}
                </span>
              </div>
              <label className="field">
                数据来源
                <select
                  aria-label={`步骤组参数 ${parameter.name} 来源`}
                  disabled={busy}
                  value={mode}
                  onChange={(event) =>
                    change(
                      parameter.name,
                      event.target.value,
                      parameter.hasDefault
                        ? parameter.defaultValue
                        : parameter.dataType === "boolean"
                          ? false
                          : "",
                    )
                  }
                >
                  <option value="default">
                    {parameter.hasDefault ? "步骤组默认值" : "未填写"}
                  </option>
                  <option value="fixed">固定值</option>
                  <option value="previous" disabled={!sources.length}>
                    前序步骤
                  </option>
                </select>
              </label>
              {mode === "fixed" && (
                <ParameterValue
                  parameter={parameter}
                  value={value}
                  disabled={busy}
                  label={`步骤组参数 ${parameter.name} 内容`}
                  onChange={(next) => change(parameter.name, "fixed", next)}
                />
              )}
              {mode === "default" && parameter.hasDefault && (
                <div className="parameter-preview">
                  <span>默认值</span>
                  <Code value={value} />
                </div>
              )}
              {binding && (
                <>
                  <label className="field">
                    前序步骤
                    <select
                      aria-label={`步骤组参数 ${parameter.name} 前序步骤`}
                      disabled={busy}
                      value={binding.sourceStepId}
                      onChange={(event) =>
                        change(parameter.name, "previous", {
                          ...binding,
                          sourceStepId: event.target.value,
                        })
                      }
                    >
                      <option value="" disabled>
                        选择前序步骤
                      </option>
                      {sources.map((source) => (
                        <option key={source.id} value={source.id}>
                          {source.group ? `${source.group.title} / ` : ""}
                          {source.title}
                        </option>
                      ))}
                    </select>
                  </label>
                  <label className="field">
                    读取内容
                    <select
                      aria-label={`步骤组参数 ${parameter.name} 读取内容`}
                      disabled={busy}
                      value={binding.from}
                      onChange={(event) =>
                        change(parameter.name, "previous", {
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
                      aria-label={`步骤组参数 ${parameter.name} 字段路径`}
                      disabled={busy}
                      value={binding.path}
                      placeholder="$.data.id"
                      onChange={(event) =>
                        change(parameter.name, "previous", {
                          ...binding,
                          path: event.target.value,
                        })
                      }
                    />
                  </label>
                  <div className="binding-preview">
                    <span>前序输出 → {parameter.name}</span>
                    <code>{binding.path}</code>
                  </div>
                </>
              )}
            </div>
          );
        })}
        {!group.parameters?.length && (
          <p className="muted">此步骤组无需参数。</p>
        )}
      </div>
    </aside>
  );
}
