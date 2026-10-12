import { useEffect, useState } from "react";
import {
  Sparkles,
  X,
  Settings2,
  Send,
  Square,
  Plus,
  LoaderCircle,
  GitBranch,
  SearchCheck,
  ClipboardList,
} from "lucide-react";
import { IconButton } from "./components.jsx";
import AssistantModelSettings from "./AssistantModelSettings.jsx";

const statusLabels = {
  idle: "待提问",
  running: "分析中",
  completed: "分析完成",
  failed: "分析失败",
  cancelled: "已取消",
  interrupted: "已中断",
};
const toolLabels = {
  "map.read": "读取图谱",
  "node.explain": "解释依赖",
  "plan.explain": "生成测试计划",
  "runs.list": "读取历史运行",
  "evidence.read": "读取失败证据",
};
const presets = [
  [
    GitBranch,
    "分析依赖",
    "分析当前所选节点或路径的前置条件、步骤依赖和校验覆盖，引用实际读取的图谱节点。",
  ],
  [
    ClipboardList,
    "生成计划",
    "为当前所选节点或路径生成测试计划，调用现有 Planner，说明要验证的内容和执行前需要满足的条件。",
  ],
  [
    SearchCheck,
    "诊断失败",
    "查找当前所选节点或路径的已有失败记录，读取相关断言和 Evidence，区分已确认的事实、原因假设和下一步检查。没有失败记录时明确说明。",
  ],
];

export default function AssistantPanel({
  mapId,
  mapName,
  node,
  pathId,
  selectNode,
  selectPath,
  openEvidence,
  close,
}) {
  const [model, setModel] = useState(null);
  const [settingsOpen, setSettingsOpen] = useState(false);
  const [sessions, setSessions] = useState([]);
  const [session, setSession] = useState(null);
  const [prompt, setPrompt] = useState("");
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(true);
  const [starting, setStarting] = useState(false);
  const running = session?.status === "running";
  useEffect(() => {
    let current = true;
    Promise.all([
      window.testbench.assistantSettings(),
      window.testbench.assistantSessions(mapId),
    ])
      .then(async ([config, items]) => {
        if (!current) return;
        setModel(config);
        setSessions(items);
        if (items[0]) {
          const latest = await window.testbench.assistantSession(items[0].id);
          if (current) setSession(latest);
        }
      })
      .catch((error) => {
        if (current) setError(error.message);
      })
      .finally(() => {
        if (current) setLoading(false);
      });
    const unsubscribe = window.testbench.onAssistantEvent((item) => {
      if (item.mapId !== mapId) return;
      setSession(item);
      setSessions((current) =>
        [item, ...current.filter((value) => value.id !== item.id)].slice(0, 50),
      );
    });
    return () => {
      current = false;
      unsubscribe();
    };
  }, [mapId]);
  async function submit(value = prompt) {
    if (!value.trim() || running || starting) return;
    if (!model?.configured) {
      setSettingsOpen(true);
      return;
    }
    setStarting(true);
    setError("");
    try {
      const item = await window.testbench.assistantStart({
        mapId,
        nodeId: node?.id || "",
        pathId,
        sessionId: session?.id,
        prompt: value,
      });
      setSession(item);
      setPrompt("");
    } catch (error) {
      setError(error.message);
    } finally {
      setStarting(false);
    }
  }
  async function history(id) {
    if (!id || running) return;
    setError("");
    try {
      setSession(await window.testbench.assistantSession(id));
    } catch (error) {
      setError(error.message);
    }
  }
  function reference(ref) {
    if (ref.startsWith("node:")) selectNode(ref.slice(5));
    if (ref.startsWith("path:")) selectPath(ref.slice(5));
    if (ref.startsWith("run:"))
      openEvidence({
        apiCaseRunId: ref.slice(4),
        caseId: "Evidence",
        status: "unknown",
      });
  }
  const state = session?.state || {};
  const tools = (state.events || []).filter(
    (item) => item.type === "tool_started" || item.type === "tool_completed",
  );
  const plan =
    (state.receipts || []).findLast(
      (item) => item.action === "plan.explain" && item.ok,
    )?.data ||
    tools.findLast(
      (item) =>
        item.type === "tool_completed" &&
        item.action === "plan.explain" &&
        item.ok,
    )?.data;
  return (
    <aside
      className="assistant-panel"
      data-assistant-status={session?.status || "idle"}
    >
      <header>
        <div>
          <Sparkles size={17} />
          <strong>AI 图谱助手</strong>
        </div>
        <span>
          <IconButton
            icon={Settings2}
            title="模型设置"
            onClick={() => setSettingsOpen(true)}
          />
          <IconButton icon={X} title="关闭 AI 图谱助手" onClick={close} />
        </span>
      </header>
      <div className="assistant-context">
        <span>{model?.configured ? model.model : "尚未配置模型"}</span>
        <strong>{node?.name || mapName}</strong>
        <small>
          {pathId ? `路径：${pathId}` : "当前图谱"} · 图谱、计划与技术证据
        </small>
      </div>
      <div className="assistant-history">
        <select
          aria-label="助手历史会话"
          value={session?.id || ""}
          disabled={running}
          onChange={(event) => history(event.target.value)}
        >
          <option value="">新会话</option>
          {sessions.map((item) => (
            <option key={item.id} value={item.id}>
              {item.title} · {statusLabels[item.status] || item.status}
            </option>
          ))}
        </select>
        <IconButton
          icon={Plus}
          title="新建助手会话"
          disabled={running}
          onClick={() => {
            setSession(null);
            setError("");
          }}
        />
      </div>
      <div className="assistant-content">
        {loading && (
          <div className="assistant-empty">
            <LoaderCircle size={22} className="spin" />
            正在读取助手记录
          </div>
        )}
        {!loading && !session && (
          <div className="assistant-empty">
            <Sparkles size={30} />
            <h3>从这张图谱开始</h3>
            <p>分析路径依赖、生成现有 Planner 的计划，或解释已有失败证据。</p>
            {!model?.configured && (
              <button className="button" onClick={() => setSettingsOpen(true)}>
                配置 OpenAI 兼容模型
              </button>
            )}
          </div>
        )}
        {(state.messages || [])
          .filter((item) => item.role === "user")
          .slice(-1)
          .map((item) => (
            <div className="assistant-question" key={item.id || item.content}>
              {item.content}
            </div>
          ))}
        {session && (
          <div className={`assistant-state ${session.status}`}>
            {running && <LoaderCircle size={12} className="spin" />}
            {statusLabels[session.status] || session.status}
            <small>会话保存在 Store</small>
          </div>
        )}
        {tools.length > 0 && (
          <details className="assistant-tools" open={running}>
            <summary>
              工具调用 ·{" "}
              {tools.filter((item) => item.type === "tool_started").length}
            </summary>
            {tools.map((item, index) => (
              <div key={index}>
                <i
                  className={
                    item.type === "tool_completed"
                      ? item.ok
                        ? "done"
                        : "error"
                      : ""
                  }
                />
                <span>
                  {toolLabels[item.action] || item.name}
                  {item.type === "tool_completed"
                    ? item.ok
                      ? " · 完成"
                      : " · 失败"
                    : " · 开始"}
                </span>
                {item.durationMs !== undefined && (
                  <small>{item.durationMs} ms</small>
                )}
                {item.error && <p>{item.error}</p>}
              </div>
            ))}
          </details>
        )}
        {state.result && (
          <div className="assistant-result">
            <p>{state.result.summary}</p>
            {state.result.findings.map((item, index) => (
              <section key={index}>
                <div>
                  <span className={item.kind}>
                    {item.kind === "observation" ? "有据结论" : "原因假设"}
                  </span>
                  <strong>{item.title}</strong>
                </div>
                <p>{item.detail}</p>
                <div className="assistant-refs">
                  {item.evidenceRefs.map((ref) => (
                    <button key={ref} onClick={() => reference(ref)}>
                      {ref === "plan:preview" ? "计划预览" : ref}
                    </button>
                  ))}
                </div>
                {item.nodeIds.length > 0 && (
                  <div className="assistant-node-links">
                    {item.nodeIds.map((id) => (
                      <button key={id} onClick={() => selectNode(id)}>
                        定位 {id}
                      </button>
                    ))}
                  </div>
                )}
              </section>
            ))}
            {state.result.nextActions.length > 0 && (
              <div className="assistant-next">
                <strong>下一步检查</strong>
                <ul>
                  {state.result.nextActions.map((value, index) => (
                    <li key={index}>{value}</li>
                  ))}
                </ul>
              </div>
            )}
          </div>
        )}
        {plan && (
          <details className="assistant-plan" open>
            <summary>
              Planner 计划预览 · {plan.physicalTasks?.length || 0}
              {plan.truncated ? ` / ${plan.totalTasks || "更多"}` : ""} 项任务
            </summary>
            <p>{plan.truncated ? "当前上下文只显示部分计划，请缩小到路径或节点查看。" : "此处显示计划，执行结果另行验证。"}</p>
            <ol>
              {(plan.physicalTasks || []).slice(0, 16).map((item) => (
                <li key={item.id}>
                  <strong>{item.kind}</strong>
                  <span>
                    {item.caseId ||
                      item.workflowId ||
                      item.pathId ||
                      item.nodeId ||
                      item.id}
                  </span>
                  {item.nodeId && (
                    <button onClick={() => selectNode(item.nodeId)}>
                      定位
                    </button>
                  )}
                </li>
              ))}
            </ol>
          </details>
        )}
        {(error || state.error) && (
          <div className="notice error">{error || state.error}</div>
        )}
      </div>
      <div className="assistant-compose">
        <div className="assistant-presets">
          {presets.map(([Icon, label, value]) => (
            <button
              key={label}
              disabled={running || starting}
              onClick={() => {
                setPrompt(value);
                submit(value);
              }}
            >
              <Icon size={12} />
              {label}
            </button>
          ))}
        </div>
        <textarea
          aria-label="助手问题"
          placeholder="针对当前节点或路径提问…"
          value={prompt}
          maxLength={8000}
          onChange={(event) => setPrompt(event.target.value)}
          onKeyDown={(event) => {
            if ((event.metaKey || event.ctrlKey) && event.key === "Enter") {
              event.preventDefault();
              submit();
            }
          }}
        />
        <footer>
          <small>LangChain · 只读分析与计划</small>
          {running ? (
            <button
              className="button"
              onClick={() =>
                window.testbench
                  .assistantCancel(session.id)
                  .catch((error) => setError(error.message))
              }
            >
              <Square size={12} />
              取消
            </button>
          ) : (
            <button
              className="button primary"
              disabled={starting || !prompt.trim()}
              onClick={() => submit()}
            >
              {starting ? (
                <LoaderCircle size={13} className="spin" />
              ) : (
                <Send size={13} />
              )}
              发送
            </button>
          )}
        </footer>
      </div>
      {settingsOpen && model && (
        <AssistantModelSettings
          value={model}
          saved={setModel}
          close={() => setSettingsOpen(false)}
        />
      )}
    </aside>
  );
}
