import { useState } from "react";
import {
  Play,
  Save,
  FileText,
  Code2,
  SlidersHorizontal,
  Braces,
  LoaderCircle,
  Download,
  CheckCircle2,
  Send,
  ShieldCheck,
} from "lucide-react";
import {
  Tabs,
  IconButton,
  Method,
  Status,
  Empty,
  Code,
  KeyValueEditor,
  Breadcrumb,
} from "./components.jsx";
import {
  httpMethods,
  methodTone,
  responseFromRun,
  variablesInDraft,
  compileRequest,
} from "./model.mjs";

export default function RequestEditor({
  draft,
  patch,
  save,
  send,
  result,
  busy,
  services,
}) {
  const [mode, setMode] = useState("run");
  const [tab, setTab] = useState("body");
  const [resultTab, setResultTab] = useState("body");
  const [overrides, setOverrides] = useState({});
  const variables = variablesInDraft(draft);
  const response = responseFromRun(result);
  const mutable = draft.editable !== false;
  const requestTabs = [
    { id: "params", label: "Params", count: draft.params.length || undefined },
    { id: "body", label: "Body" },
    {
      id: "headers",
      label: "Headers",
      count: draft.headers.length || undefined,
    },
    {
      id: "variables",
      label: "请求变量",
      count: variables.length || undefined,
    },
    { id: "assertions", label: "断言" },
    { id: "settings", label: "设置" },
  ];
  return (
    <div className="request-editor">
      <div className="editor-heading">
        <div>
          <Breadcrumb
            items={[
              "接口管理",
              services.find((item) => item.id === draft.serviceId)
                ?.displayName || "HTTP API",
            ]}
          />
          <div className="editor-title">
            <Method value={draft.method} />
            <input
              aria-label="接口名称"
              value={draft.displayName}
              disabled={!mutable}
              onChange={(event) => patch({ displayName: event.target.value })}
            />
            <span className="soft-tag">
              {draft.status === "draft" ? "开发中" : "已启用"}
            </span>
            {draft.dirty && <span className="unsaved-label">未保存</span>}
          </div>
        </div>
        <div className="heading-actions">
          <IconButton
            icon={Download}
            title="导出接口配置"
            disabled={draft.dirty}
            onClick={() =>
              window.testbench.exportJSON(`${draft.displayName}.json`, {
                id: draft.id,
                nodeId: draft.nodeId,
                serviceId: draft.serviceId,
                displayName: draft.displayName,
                description: draft.description,
                execution: draft.execution,
                baseUrl: draft.baseUrl,
                timeoutSeconds: draft.timeoutSeconds,
              })
            }
          />
          <button className="button" disabled={busy || !mutable} onClick={save}>
            <Save size={14} />
            保存
          </button>
        </div>
      </div>
      <Tabs
        items={[
          { id: "document", label: "文档" },
          { id: "run", label: "运行" },
        ]}
        value={mode}
        onChange={setMode}
      />
      {mode === "document" ? (
        <div className="document-pane">
          <div className="document-section">
            <h2>{draft.displayName}</h2>
            <div className="doc-address">
              <Method value={draft.method} />
              <code>{draft.url}</code>
            </div>
            <label className="field">
              接口说明
              <textarea
                aria-label="接口说明"
                rows={4}
                value={draft.description || ""}
                disabled={!mutable}
                placeholder="描述接口用途、请求参数和返回数据"
                onChange={(event) => patch({ description: event.target.value })}
              />
            </label>
          </div>
          <div className="document-section">
            <h3>请求体</h3>
            {draft.bodyText ? (
              <Code value={draft.bodyText} />
            ) : (
              <p className="muted">该接口没有请求体</p>
            )}
          </div>
          <div className="document-section">
            <h3>成功响应</h3>
            <span className="success-dot" /> HTTP {draft.codesText}
          </div>
        </div>
      ) : (
        <>
          <div className="request-address">
            <select
              aria-label="HTTP 方法"
              className={`method-select ${methodTone(draft.method)}`}
              value={draft.method}
              disabled={!mutable}
              onChange={(event) => patch({ method: event.target.value })}
            >
              {httpMethods.map((value) => (
                <option key={value}>{value}</option>
              ))}
            </select>
            <input
              aria-label="请求地址"
              value={draft.url}
              disabled={!mutable}
              placeholder="输入请求地址，例如 https://api.example.com/items"
              onChange={(event) => patch({ url: event.target.value })}
            />
            <button
              className="button primary send-button"
              disabled={busy}
              onClick={() => send(overrides)}
            >
              {busy ? (
                <LoaderCircle size={15} className="spin" />
              ) : (
                <Send size={15} />
              )}
              发送
            </button>
          </div>
          {!mutable && (
            <div className="notice">
              该接口由外部执行适配器维护，可直接运行并查看结果。
            </div>
          )}
          <div className="request-config">
            <Tabs compact items={requestTabs} value={tab} onChange={setTab} />
            <div className="request-tab-content">
              {tab === "params" && (
                <>
                  <p className="table-note">
                    查询参数 · 可使用 {"{{override:变量名}}"} 引用请求变量
                  </p>
                  <KeyValueEditor
                    rows={draft.params}
                    kind="查询参数"
                    disabled={!mutable}
                    onChange={(params) => patch({ params })}
                  />
                </>
              )}
              {tab === "headers" && (
                <>
                  <p className="table-note">
                    已保存的敏感字段以 [REDACTED] 显示，直接保存会保留原值。
                  </p>
                  <KeyValueEditor
                    rows={draft.headers}
                    kind="请求头"
                    disabled={!mutable}
                    onChange={(headers) => patch({ headers })}
                  />
                </>
              )}
              {tab === "body" && (
                <>
                  <div className="body-toolbar">
                    <span className="radio-selected">
                      <span />
                      JSON
                    </span>
                    <span className="muted">application/json</span>
                    <button
                      className="text-button"
                      disabled={!mutable}
                      onClick={() => {
                        try {
                          patch({
                            bodyText: JSON.stringify(
                              JSON.parse(draft.bodyText),
                              null,
                              2,
                            ),
                          });
                        } catch {}
                      }}
                    >
                      <Code2 size={13} />
                      格式化
                    </button>
                  </div>
                  <div className="body-editor">
                    <div className="editor-lines">
                      {Array.from(
                        {
                          length: Math.max(
                            8,
                            draft.bodyText.split("\n").length,
                          ),
                        },
                        (_, i) => (
                          <span key={i}>{i + 1}</span>
                        ),
                      )}
                    </div>
                    <textarea
                      aria-label="JSON 请求体"
                      spellCheck={false}
                      value={draft.bodyText}
                      disabled={!mutable}
                      placeholder={'{\n  "name": "example"\n}'}
                      onChange={(event) =>
                        patch({ bodyText: event.target.value })
                      }
                    />
                  </div>
                </>
              )}
              {tab === "variables" && (
                <div className="variables-editor">
                  {variables.length ? (
                    variables.map((name) => (
                      <label className="field horizontal" key={name}>
                        <code>{name}</code>
                        <input
                          aria-label={`请求变量 ${name}`}
                          value={overrides[name] || ""}
                          placeholder="本次请求的变量值"
                          onChange={(event) =>
                            setOverrides({
                              ...overrides,
                              [name]: event.target.value,
                            })
                          }
                        />
                      </label>
                    ))
                  ) : (
                    <Empty icon={Braces} title="没有请求变量">
                      在地址、参数或请求体中使用 {"{{override:变量名}}"}{" "}
                      后，可在这里填写本次请求值。
                    </Empty>
                  )}
                </div>
              )}
              {tab === "assertions" && (
                <div className="assertion-editor">
                  <label className="field">
                    <span>
                      <ShieldCheck size={14} />
                      预期 HTTP 状态码
                    </span>
                    <input
                      aria-label="预期状态码"
                      value={draft.codesText}
                      disabled={!mutable}
                      onChange={(event) =>
                        patch({ codesText: event.target.value })
                      }
                    />
                  </label>
                  <label className="field">
                    响应包含（每行一项）
                    <textarea
                      aria-label="响应包含"
                      rows={3}
                      value={draft.containsText}
                      disabled={!mutable}
                      onChange={(event) =>
                        patch({ containsText: event.target.value })
                      }
                    />
                  </label>
                  <label className="field">
                    响应不包含（每行一项）
                    <textarea
                      aria-label="响应不包含"
                      rows={2}
                      value={draft.absentText}
                      disabled={!mutable}
                      onChange={(event) =>
                        patch({ absentText: event.target.value })
                      }
                    />
                  </label>
                </div>
              )}
              {tab === "settings" && (
                <div className="assertion-editor">
                  <label className="field">
                    所属服务
                    <select
                      aria-label="所属服务"
                      value={draft.serviceId}
                      disabled={!mutable}
                      onChange={(event) =>
                        patch({ serviceId: event.target.value })
                      }
                    >
                      {services.map((service) => (
                        <option value={service.id} key={service.id}>
                          {service.displayName || service.id}
                        </option>
                      ))}
                    </select>
                  </label>
                  <label className="field">
                    请求超时（秒）
                    <input
                      aria-label="请求超时"
                      type="number"
                      min={1}
                      max={300}
                      value={draft.timeoutSeconds || 30}
                      disabled={!mutable}
                      onChange={(event) =>
                        patch({ timeoutSeconds: Number(event.target.value) })
                      }
                    />
                  </label>
                  <div className="detail-grid">
                    <span>接口 ID</span>
                    <code>{draft.nodeId}</code>
                    <span>用例 ID</span>
                    <code>{draft.id}</code>
                  </div>
                </div>
              )}
            </div>
          </div>
          <section className="response-pane">
            <div className="response-heading">
              <strong>返回响应</strong>
              {result && (
                <div className="response-metrics">
                  <Status
                    value={result.status || (result.ok ? "passed" : "failed")}
                  />
                  {response && (
                    <>
                      <span>
                        HTTP{" "}
                        <b>
                          {response.statusCode ||
                            result.summary?.httpCode ||
                            "—"}
                        </b>
                      </span>
                      <span>
                        <b>{response.elapsedMs ?? "—"}</b> ms
                      </span>
                      <span>
                        <b>
                          {
                            new TextEncoder().encode(
                              String(response.body || ""),
                            ).length
                          }
                        </b>{" "}
                        B
                      </span>
                    </>
                  )}
                </div>
              )}
            </div>
            <Tabs
              compact
              items={[
                { id: "body", label: "Body" },
                { id: "headers", label: "Headers" },
                { id: "request", label: "实际请求" },
                { id: "assertions", label: "断言结果" },
                { id: "evidence", label: "Evidence" },
              ]}
              value={resultTab}
              onChange={setResultTab}
            />
            {!result ? (
              <Empty icon={Send} title="点击发送，查看响应">
                请求将在 TestBench 执行引擎中运行，并保存运行记录与 Evidence。
              </Empty>
            ) : resultTab === "body" ? (
              response ? (
                <Code value={response.body} />
              ) : (
                <div className="failure-detail">
                  {result.failureReason ||
                    result.error ||
                    result.summary?.failureReason ||
                    "没有收到 HTTP 响应"}
                  <Code value={result} />
                </div>
              )
            ) : resultTab === "headers" ? (
              <Code value={response?.headers || {}} />
            ) : resultTab === "request" ? (
              <Code
                value={result.result?.request || result.evidence?.request || {}}
              />
            ) : resultTab === "assertions" ? (
              <div className="assertion-result">
                <Status
                  value={result.status || (result.ok ? "passed" : "failed")}
                />
                <p>
                  {result.failureReason ||
                    result.summary?.failureReason ||
                    `预期状态码：${draft.codesText}`}
                </p>
              </div>
            ) : (
              <Code
                value={{
                  runId: result.runId,
                  caseRunId: result.caseRunId,
                  evidence:
                    result.evidence ||
                    result.evidenceHandles ||
                    result.summary ||
                    {},
                }}
              />
            )}
          </section>
        </>
      )}
    </div>
  );
}
