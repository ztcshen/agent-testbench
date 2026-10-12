import { useEffect, useState } from "react";
import { X, Braces, FileText, GitBranch, ArrowUpRight } from "lucide-react";
import { api } from "./api.mjs";
import { IconButton, Method, Status, Tabs, Code } from "./components.jsx";
import { parseMapJSON, validationNode } from "./mapModel.mjs";

export default function MapInspector({
  node,
  index,
  plan,
  pathId,
  selectPath,
  openCase,
  openWorkflow,
  openEvidence,
  close,
}) {
  const [tab, setTab] = useState("details");
  const [explanation, setExplanation] = useState(null);
  const [error, setError] = useState("");
  useEffect(() => {
    setExplanation(null);
    setError("");
  }, [node?.id]);
  if (!node)
    return (
      <aside className="map-inspector">
        <header>
          <strong>图谱详情</strong>
        </header>
        <div className="map-inspector-content">
          <h3>{index.graph.map.displayName || index.graph.map.id}</h3>
          <p>
            {index.graph.map.description ||
              "选择图中的节点，查看接口、用例、所属路径及运行信息。"}
          </p>
          <div className="detail-grid">
            <span>接口节点</span>
            <strong>{index.nodes.length}</strong>
            <span>执行路径</span>
            <strong>{index.graph.paths?.length || 0}</strong>
            <span>校验节点</span>
            <strong>{index.nodes.filter(validationNode).length}</strong>
            <span>目录</span>
            <code>{index.graph.map.profileId}</code>
          </div>
          <h4>图谱说明</h4>
          <p>
            实线表示 Store
            中登记的连接。虚线表示路径顺序或校验锚点推导的展示关系。
          </p>
        </div>
      </aside>
    );
  const memberships = (index.graph.paths || []).filter((path) =>
    node.memberships.includes(path.id),
  );
  const tasks = (plan?.tasks || []).filter(
    (task) => task.nodeId === node.id || task.caseId === node.caseId,
  );
  async function explain() {
    setTab("explain");
    setError("");
    try {
      setExplanation(
        (
          await api(
            `/api/map/explanation?${new URLSearchParams({ id: index.graph.map.id, nodeId: node.id })}`,
          )
        ).data.explanation,
      );
    } catch (error) {
      setError(error.message);
    }
  }
  return (
    <aside className="map-inspector">
      <header>
        <strong>节点详情</strong>
        <IconButton icon={X} title="取消选择节点" onClick={close} />
      </header>
      <div className="map-node-title">
        <span
          className={`map-role ${validationNode(node) ? "validation" : ""}`}
        >
          {validationNode(node) ? "校验用例" : "主流程节点"}
        </span>
        <h3>{node.name}</h3>
        <div>
          <Method value={node.method} />
          <code>{node.address || node.interfaceNodeId || node.id}</code>
        </div>
      </div>
      <Tabs
        compact
        items={[
          { id: "details", label: "详情" },
          { id: "paths", label: "路径", count: memberships.length },
          { id: "runs", label: "运行", count: tasks.length },
          { id: "explain", label: "依赖说明" },
        ]}
        value={tab}
        onChange={(value) => (value === "explain" ? explain() : setTab(value))}
      />
      <div className="map-inspector-content">
        {tab === "details" && (
          <>
            <p>
              {node.description ||
                node.caseDef.scenario ||
                "该节点从 Store 图谱中读取。"}
            </p>
            <div className="detail-grid">
              <span>节点 ID</span>
              <code>{node.id}</code>
              <span>用例 ID</span>
              <code>{node.caseId || "—"}</code>
              <span>接口</span>
              <code>{node.interfaceName || node.interfaceNodeId || "—"}</code>
              <span>状态作用</span>
              <span>
                {node.stateEffect === "advance"
                  ? "推进流程状态"
                  : node.stateEffect === "unchanged"
                    ? "保持状态"
                    : "未登记"}
              </span>
              <span>锚点</span>
              <code>{node.anchor || "—"}</code>
            </div>
            {node.caseId && (
              <button className="button" onClick={() => openCase(node.caseId)}>
                <ArrowUpRight size={13} />
                查看接口与用例
              </button>
            )}
            <h4>
              <Braces size={13} />
              输入与输出约束
            </h4>
            <details>
              <summary>所需属性</summary>
              <Code value={parseMapJSON(node.requiredPropertyJson)} />
            </details>
            <details>
              <summary>提供属性</summary>
              <Code value={parseMapJSON(node.providedPropertyJson)} />
            </details>
            {validationNode(node) && (
              <>
                <h4>校验配置</h4>
                <details>
                  <summary>请求变更</summary>
                  <Code value={parseMapJSON(node.patchJson)} />
                </details>
                <details>
                  <summary>预期响应</summary>
                  <Code value={parseMapJSON(node.expectedJson)} />
                </details>
              </>
            )}
          </>
        )}
        {tab === "paths" && (
          <div className="map-memberships">
            {memberships.length ? (
              memberships.map((path) => (
                <section key={path.id}>
                  <strong>{path.displayName || path.id}</strong>
                  <code>{path.id}</code>
                  <p>
                    {(index.steps.get(path.id) || []).find(
                      (step) => step.nodeId === node.id,
                    )?.stepIndex || "—"}{" "}
                    / {index.steps.get(path.id)?.length || 0} 步
                  </p>
                  <div>
                    <button
                      className="button"
                      onClick={() => selectPath(path.id)}
                    >
                      <GitBranch size={12} />
                      定位路径
                    </button>
                    {path.workflowId && (
                      <button
                        className="text-button"
                        onClick={() => openWorkflow(path.workflowId)}
                      >
                        关联场景
                        <ArrowUpRight size={12} />
                      </button>
                    )}
                  </div>
                </section>
              ))
            ) : (
              <p>该节点尚未登记到路径中。</p>
            )}
          </div>
        )}
        {tab === "runs" && (
          <div className="map-task-list">
            {tasks.length ? (
              tasks.map((task) => (
                <section key={task.id}>
                  <div>
                    <Status value={task.status} />
                    <span>{task.kind}</span>
                  </div>
                  <strong>{task.id}</strong>
                  <p>{task.reason || "—"}</p>
                  {task.apiCaseRunId && (
                    <button
                      className="button"
                      onClick={() => openEvidence(task)}
                    >
                      <FileText size={12} />
                      查看 Evidence
                    </button>
                  )}
                  <details>
                    <summary>任务记录</summary>
                    <Code value={task} />
                  </details>
                </section>
              ))
            ) : (
              <p>
                当前选择的计划没有该节点的执行记录。选择历史计划后可查看实际状态。
              </p>
            )}
          </div>
        )}
        {tab === "explain" &&
          (error ? (
            <div className="notice error">{error}</div>
          ) : explanation ? (
            <>
              <p>从现有 map 推导该节点的依赖与准备路径；此操作只读取图谱。</p>
              <Code value={explanation} />
            </>
          ) : (
            <p>正在读取依赖说明…</p>
          ))}
      </div>
    </aside>
  );
}
