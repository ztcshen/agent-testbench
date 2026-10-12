import { useEffect, useState } from "react";
import {
  Search,
  RefreshCw,
  FileText,
  Download,
  Server,
  ShieldCheck,
  Database,
  Plus,
  Cable,
  Settings2,
  Boxes,
  Send,
  Play,
  CheckCircle2,
  ChevronRight,
  LoaderCircle,
} from "lucide-react";
import { api, evidenceFor } from "./api.mjs";
import {
  Code,
  Empty,
  Status,
  Method,
  SearchInput,
  Modal,
  Tabs,
  Breadcrumb,
} from "./components.jsx";
import { dateLabel, pretty, uid } from "./model.mjs";

export function Overview({ workspace, open, create }) {
  return (
    <div className="overview">
      <Breadcrumb items={["本机工作区", "项目概览"]} />
      <h1>AgentTestBench</h1>
      <p className="overview-subtitle">设计接口，编排场景，验证每一次运行。</p>
      <div className="overview-stats">
        {[
          [workspace.requests.length, "个接口", Send],
          [workspace.studio?.workflows?.length || 0, "个测试场景", Play],
          [workspace.runs.length, "条运行记录", FileText],
          [workspace.environments.length, "个环境", Server],
        ].map(([count, label, Icon]) => (
          <div key={label}>
            <Icon size={20} />
            <strong>{count}</strong>
            <span>{label}</span>
          </div>
        ))}
      </div>
      <h3>开始测试</h3>
      <div className="start-actions">
        <button onClick={() => create("apis")}>
          <span className="action-icon">
            <Plus size={22} />
          </span>
          <strong>新建接口</strong>
          <p>设置请求地址、参数和预期响应</p>
          <ChevronRight size={16} />
        </button>
        <button onClick={() => create("scenarios")}>
          <span className="action-icon">
            <GitBranchIcon />
          </span>
          <strong>新建测试场景</strong>
          <p>组合请求，传递前序步骤的数据</p>
          <ChevronRight size={16} />
        </button>
      </div>
      <h3>最近运行</h3>
      <div className="recent-runs">
        {workspace.runs.slice(0, 6).map((item) => (
          <button
            key={item.id}
            onClick={() =>
              open({
                kind: "evidence",
                id: item.id,
                name: item.caseId,
                run: item,
              })
            }
          >
            <Status value={item.status} />
            <strong>
              {workspace.requests.find((request) => request.id === item.caseId)
                ?.displayName || item.caseId}
            </strong>
            <code>{item.operation}</code>
            <span>{dateLabel(item.updatedAt)}</span>
          </button>
        ))}
        {!workspace.runs.length && (
          <Empty title="还没有运行记录">
            发送接口请求或运行场景后，记录会在这里出现。
          </Empty>
        )}
      </div>
    </div>
  );
}
function GitBranchIcon() {
  return <Boxes size={22} />;
}

export function Reports({ runs, requests, open, refresh, filterID = "" }) {
  const [query, setQuery] = useState("");
  const [status, setStatus] = useState("all");
  const items = runs.filter(
    (item) =>
      (!filterID || item.caseId === filterID) &&
      (status === "all" || item.status === status) &&
      `${item.caseId} ${item.operation} ${item.runId}`
        .toLowerCase()
        .includes(query.toLowerCase()),
  );
  return (
    <div className="reports-page">
      <div className="panel-heading">
        <div>
          <Breadcrumb items={["自动化测试", "运行报告"]} />
          <h2>测试报告</h2>
        </div>
        <button className="button" onClick={refresh}>
          <RefreshCw size={14} />
          刷新
        </button>
      </div>
      <div className="list-toolbar">
        <SearchInput
          value={query}
          onChange={setQuery}
          placeholder="搜索接口、运行 ID 或请求路径"
        />
        <select
          aria-label="报告状态筛选"
          value={status}
          onChange={(event) => setStatus(event.target.value)}
        >
          <option value="all">全部状态</option>
          <option value="passed">通过</option>
          <option value="failed">失败</option>
          <option value="running">执行中</option>
        </select>
        <span className="muted">{items.length} 条记录</span>
      </div>
      <div className="report-table">
        <div className="report-table-head">
          <span>结果</span>
          <span>接口 / 请求</span>
          <span>Evidence</span>
          <span>执行时间</span>
          <span />
        </div>
        {items.map((item) => (
          <button
            key={item.id}
            className="report-table-row"
            onClick={() =>
              open({
                kind: "evidence",
                id: item.id,
                name:
                  requests.find((request) => request.id === item.caseId)
                    ?.displayName || item.caseId,
                run: item,
              })
            }
          >
            <Status value={item.status} />
            <span>
              <strong>
                {requests.find((request) => request.id === item.caseId)
                  ?.displayName || item.caseId}
              </strong>
              <code>{item.operation}</code>
            </span>
            <span className="evidence-count">
              <FileText size={13} />
              {item.evidenceCount || 0} 项
            </span>
            <span>{dateLabel(item.updatedAt)}</span>
            <ChevronRight size={15} />
          </button>
        ))}
      </div>
      {!items.length && (
        <Empty icon={FileText} title="没有匹配的运行记录">
          调整筛选条件，或先运行一个接口。
        </Empty>
      )}
    </div>
  );
}

export function Evidence({ run }) {
  const [payload, setPayload] = useState(null);
  const [error, setError] = useState("");
  const [tab, setTab] = useState("response");
  useEffect(() => {
    let active = true;
    setPayload(null);
    setError("");
    evidenceFor(run)
      .then((result) => {
        if (active) setPayload(result.data);
      })
      .catch((error) => {
        if (active) setError(error.message);
      });
    return () => {
      active = false;
    };
  }, [run.id]);
  const evidence = payload?.evidence || {};
  return (
    <div className="evidence-page">
      <div className="panel-heading">
        <div>
          <Breadcrumb items={["测试报告", "运行明细"]} />
          <h2>{run.caseId}</h2>
          <p className="muted">{run.operation}</p>
        </div>
        <Status value={run.status} />
      </div>
      <div className="evidence-summary">
        <span>
          运行 ID<code>{run.runId}</code>
        </span>
        <span>
          执行时间<strong>{dateLabel(run.updatedAt)}</strong>
        </span>
        <button
          className="button"
          disabled={!payload}
          onClick={() =>
            window.testbench.exportJSON("testbench-evidence.json", payload)
          }
        >
          <Download size={14} />
          导出 Evidence
        </button>
      </div>
      <Tabs
        items={[
          { id: "response", label: "响应" },
          { id: "request", label: "请求" },
          { id: "assertions", label: "断言" },
          { id: "topology", label: "链路拓扑" },
          { id: "all", label: "完整 Evidence" },
        ]}
        value={tab}
        onChange={setTab}
      />
      {error ? (
        <div className="notice error">{error}</div>
      ) : !payload ? (
        <Empty icon={LoaderCircle} title="正在读取 Evidence" />
      ) : (
        <Code value={tab === "all" ? payload : evidence[tab] || {}} />
      )}
    </div>
  );
}

export function Environments({
  items,
  workflows,
  refresh,
  selectedID = "",
  notify,
  activity,
}) {
  const [editing, setEditing] = useState(null);
  const [busy, setBusy] = useState("");
  const [query, setQuery] = useState("");
  const [report, setReport] = useState(null);
  function edit(item = {}) {
    setEditing({
      ...item,
      id: item.id || uid("environment"),
      displayName: item.displayName || "",
      description: item.description || "",
      verificationWorkflowId:
        item.verificationWorkflowId || workflows[0]?.workflow.id || "",
      composeText: JSON.stringify(item.compose || {}, null, 2),
      servicesText: JSON.stringify(item.services || [], null, 2),
      healthChecksText: JSON.stringify(item.healthChecks || [], null, 2),
    });
  }
  async function save() {
    setBusy("save");
    activity(true);
    try {
      const body = {
        ...editing,
        compose: JSON.parse(editing.composeText),
        services: JSON.parse(editing.servicesText),
        healthChecks: JSON.parse(editing.healthChecksText),
      };
      if (!body.displayName.trim() || !body.verificationWorkflowId)
        throw new Error("请填写环境名称并选择验收场景");
      await api("/api/environments", { method: "POST", body });
      setEditing(null);
      await refresh();
      notify("环境已保存至 Store");
    } catch (error) {
      notify(error.message, true);
    } finally {
      setBusy("");
      activity(false);
    }
  }
  async function verify(item) {
    setBusy(item.id);
    activity(true);
    try {
      let value = (
        await api(
          `/api/environments/${encodeURIComponent(item.id)}/acceptance-runs`,
          { method: "POST", body: { requestId: crypto.randomUUID() } },
        )
      ).data;
      setReport(value);
      while (["running", "queued"].includes(value.status)) {
        await new Promise((resolve) => setTimeout(resolve, 300));
        value = (await api(value.reportUrl)).data;
        setReport(value);
      }
      await refresh();
      notify(`验收${value.acceptance?.ok ? "通过" : "未通过"}`);
    } catch (error) {
      notify(error.message, true);
    } finally {
      setBusy("");
      activity(false);
    }
  }
  const filtered = items.filter((item) =>
    `${item.displayName} ${item.id}`
      .toLowerCase()
      .includes(query.toLowerCase()),
  );
  return (
    <div className="environments-page">
      <div className="panel-heading">
        <div>
          <Breadcrumb items={["项目设置", "运行环境"]} />
          <h2>运行环境</h2>
          <p className="muted">
            配置从 Store 读取，验收结果包含真实 Evidence 与拓扑。
          </p>
        </div>
        <button className="button primary" onClick={() => edit()}>
          <Plus size={14} />
          新建环境
        </button>
      </div>
      <div className="list-toolbar">
        <SearchInput value={query} onChange={setQuery} />
        <span className="muted">{items.length} 个环境</span>
      </div>
      <div className="environment-list">
        {filtered.map((item) => (
          <div
            className={`environment-row ${selectedID === item.id ? "selected" : ""}`}
            key={item.id}
          >
            <div className="environment-icon">
              <Server size={21} />
            </div>
            <div className="environment-info">
              <strong>
                {item.displayName || item.id}
                <span className={`soft-tag ${item.verified ? "green" : ""}`}>
                  {item.verified ? "已验证" : "待验证"}
                </span>
              </strong>
              <p>{item.description || item.id}</p>
              <div className="environment-checks">
                <span>
                  Evidence {item.evidenceComplete ? "完整" : "未完成"}
                </span>
                <span>拓扑 {item.topologyComplete ? "完整" : "未完成"}</span>
                {item.lastVerificationStatus && (
                  <Status value={item.lastVerificationStatus} />
                )}
              </div>
            </div>
            <button
              className="button"
              disabled={Boolean(busy)}
              onClick={() => edit(item)}
            >
              编辑
            </button>
            <button
              className="button"
              disabled={Boolean(busy) || !item.verificationWorkflowId}
              onClick={() => verify(item)}
            >
              {busy === item.id ? (
                <LoaderCircle size={14} className="spin" />
              ) : (
                <ShieldCheck size={14} />
              )}
              执行验收
            </button>
          </div>
        ))}
      </div>
      {!items.length && (
        <Empty icon={Server} title="还没有登记运行环境">
          新建环境并选择一个验收场景，随后可查看 Evidence 与拓扑完整性。
        </Empty>
      )}
      {report && (
        <details className="environment-report" open>
          <summary>
            <Status value={report.status} />
            最近验收结果
          </summary>
          <Code value={report} />
        </details>
      )}
      {editing && (
        <Modal
          title="配置运行环境"
          wide
          onClose={() => {
            if (!busy) setEditing(null);
          }}
        >
          <div className="modal-body environment-form">
            <label className="field">
              环境名称
              <input
                aria-label="环境名称"
                value={editing.displayName}
                onChange={(event) =>
                  setEditing({ ...editing, displayName: event.target.value })
                }
              />
            </label>
            <label className="field">
              环境说明
              <input
                value={editing.description}
                onChange={(event) =>
                  setEditing({ ...editing, description: event.target.value })
                }
              />
            </label>
            <label className="field">
              验收场景
              <select
                aria-label="验收场景"
                value={editing.verificationWorkflowId}
                onChange={(event) =>
                  setEditing({
                    ...editing,
                    verificationWorkflowId: event.target.value,
                  })
                }
              >
                <option value="">选择验收场景</option>
                {workflows.map((item) => (
                  <option key={item.workflow.id} value={item.workflow.id}>
                    {item.workflow.displayName}
                  </option>
                ))}
              </select>
            </label>
            <details>
              <summary>服务、健康检查和启动配置</summary>
              {[
                ["servicesText", "服务列表 JSON"],
                ["healthChecksText", "健康检查 JSON"],
                ["composeText", "启动配置 JSON"],
              ].map(([key, label]) => (
                <label className="field" key={key}>
                  {label}
                  <textarea
                    rows={4}
                    spellCheck={false}
                    value={editing[key]}
                    onChange={(event) =>
                      setEditing({ ...editing, [key]: event.target.value })
                    }
                  />
                </label>
              ))}
            </details>
          </div>
          <footer>
            <button
              className="button"
              onClick={() => setEditing(null)}
              disabled={Boolean(busy)}
            >
              取消
            </button>
            <button
              className="button primary"
              onClick={save}
              disabled={Boolean(busy)}
            >
              保存环境
            </button>
          </footer>
        </Modal>
      )}
    </div>
  );
}

export function Settings({
  connection,
  connect,
  locked,
  workspace,
  importRequests,
  notify,
}) {
  const [draft, setDraft] = useState(null);
  const [pending, setPending] = useState(false);
  useEffect(() => {
    window.testbench.settings().then(setDraft);
  }, []);
  async function save() {
    setPending(true);
    try {
      await connect(draft);
    } catch (error) {
      notify(error.message, true);
    } finally {
      setPending(false);
    }
  }
  if (!draft) return <Empty title="正在读取桌面设置" />;
  return (
    <div className="settings-page">
      <Breadcrumb items={["本机工作区", "项目设置"]} />
      <h2>项目设置</h2>
      <section className="settings-section">
        <h3>
          <Cable size={17} />
          工作区连接
        </h3>
        <p className="muted">
          选择由桌面应用启动本机引擎，或连接已经运行的 TestBench API 服务。
        </p>
        <div className="connection-options">
          <button
            className={draft.mode === "local" ? "active" : ""}
            onClick={() => setDraft({ ...draft, mode: "local" })}
          >
            <Database size={18} />
            <strong>本机引擎</strong>
            <span>Electron 管理启动与退出</span>
          </button>
          <button
            className={draft.mode === "external" ? "active" : ""}
            onClick={() => setDraft({ ...draft, mode: "external" })}
          >
            <Cable size={18} />
            <strong>已有服务</strong>
            <span>连接现有 Store 与执行引擎</span>
          </button>
        </div>
        {draft.mode === "external" ? (
          <label className="field">
            API 服务地址
            <input
              aria-label="API 服务地址"
              placeholder="http://127.0.0.1:PORT"
              value={draft.backendURL}
              onChange={(event) =>
                setDraft({ ...draft, backendURL: event.target.value })
              }
            />
          </label>
        ) : (
          <>
            <label className="field">
              Store 名称或 DSN
              <input
                aria-label="Store 配置"
                placeholder="留空使用桌面应用的本机 SQLite Store"
                value={draft.store}
                onChange={(event) =>
                  setDraft({ ...draft, store: event.target.value })
                }
              />
            </label>
            <label className="field">
              SkyWalking GraphQL 地址
              <input
                aria-label="SkyWalking 地址"
                placeholder="按需填写"
                value={draft.traceURL}
                onChange={(event) =>
                  setDraft({ ...draft, traceURL: event.target.value })
                }
              />
            </label>
          </>
        )}
        <button
          className="button primary"
          disabled={pending || locked}
          onClick={save}
        >
          {pending ? (
            <LoaderCircle className="spin" size={14} />
          ) : (
            <CheckCircle2 size={14} />
          )}
          保存并连接
        </button>
        {locked && (
          <p className="muted">
            请先保存编辑并等待当前运行结束，再切换工作区。
          </p>
        )}
      </section>
      <section className="settings-section">
        <h3>
          <Download size={17} />
          接口导入与导出
        </h3>
        <p className="muted">
          通过本机 JSON 文件交换接口配置。导入后可在接口目录中继续编辑。
        </p>
        <div className="inline-actions">
          <button className="button" onClick={importRequests} disabled={locked}>
            <Plus size={14} />
            导入接口 JSON
          </button>
          <button
            className="button"
            onClick={() =>
              window.testbench.exportJSON("testbench-requests.json", {
                schemaVersion: "atb.desktop.v1",
                requests: workspace.requests,
              })
            }
          >
            <Download size={14} />
            导出接口配置
          </button>
        </div>
      </section>
      <section className="settings-section">
        <h3>
          <Settings2 size={17} />
          桌面信息
        </h3>
        <div className="detail-grid">
          <span>产品</span>
          <strong>AgentTestBench</strong>
          <span>版本</span>
          <code>{draft.version}</code>
          <span>界面</span>
          <strong>Electron 桌面工作区</strong>
          <span>执行引擎</span>
          <code>{connection.backendURL || "未连接"}</code>
          <span>Store</span>
          <code>
            {workspace.store.backend ||
              workspace.store.storeInfo?.backend ||
              "未连接"}
          </code>
        </div>
      </section>
    </div>
  );
}
