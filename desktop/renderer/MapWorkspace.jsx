import { memo, useEffect, useMemo, useState, useCallback } from "react";
import {
  ReactFlow,
  ReactFlowProvider,
  Handle,
  Position,
  Background,
  MiniMap,
  Controls,
  useReactFlow,
} from "@xyflow/react";
import "@xyflow/react/dist/style.css";
import {
  Network,
  GitBranch,
  ChevronDown,
  Search,
  Focus,
  Maximize2,
  List,
  Layers,
  History,
  RefreshCw,
  Plus,
  ArrowUpRight,
  CheckCircle2,
  Sparkles,
} from "lucide-react";
import { api } from "./api.mjs";
import {
  Breadcrumb,
  IconButton,
  Method,
  Status,
  Empty,
  SearchInput,
  Tabs,
  Code,
} from "./components.jsx";
import {
  mapIndex,
  filterMap,
  mapLayout,
  planNodeStatus,
  validationNode,
} from "./mapModel.mjs";
import { dateLabel } from "./model.mjs";
import MapInspector from "./MapInspector.jsx";
import AssistantPanel from "./AssistantPanel.jsx";

const MapNode = memo(({ data, selected }) => {
  const status = planNodeStatus(data.tasks);
  return (
    <div
      className={`map-canvas-node ${validationNode(data) ? "validation" : "primary"} ${selected ? "selected" : ""} ${data.matched ? "match" : ""}`}
    >
      <Handle type="target" position={Position.Left} />
      <div className="map-canvas-node-heading">
        <span className="map-node-symbol">
          {validationNode(data) ? (
            <CheckCircle2 size={13} />
          ) : (
            <Network size={13} />
          )}
        </span>
        <strong>{data.name}</strong>
        {status && (
          <span className={`node-state-dot ${status}`} title={status} />
        )}
      </div>
      <div className="map-node-address">
        <Method value={data.method} />
        <code>{data.address || data.caseId || data.id}</code>
      </div>
      <footer>
        <span>
          {validationNode(data)
            ? "校验"
            : data.memberships.length > 1
              ? `共享 · ${data.memberships.length} 条路径`
              : "流程节点"}
        </span>
        {data.hiddenCount > 0 && (
          <button
            aria-label={`展开 ${data.name} 的 ${data.hiddenCount} 个校验`}
            onClick={(event) => {
              event.stopPropagation();
              data.expand(data.id);
            }}
          >
            <Plus size={10} />
            {data.hiddenCount} 校验
          </button>
        )}
      </footer>
      <Handle type="source" position={Position.Right} />
    </div>
  );
});
const nodeTypes = { mapNode: MapNode };

function Canvas({
  index,
  pathId,
  query,
  selected,
  selectNode,
  expanded,
  expand,
  allValidations,
  plan,
}) {
  const { fitView, setCenter, setViewport, getNode } = useReactFlow();
  const view = useMemo(
    () => filterMap(index, { pathId, query, expanded, allValidations }),
    [index, pathId, query, expanded, allValidations],
  );
  const layout = useMemo(
    () => mapLayout(view, { selected, plan }),
    [view, selected, plan],
  );
  const nodes = useMemo(
    () =>
      layout.nodes.map((node) => ({ ...node, data: { ...node.data, expand } })),
    [layout.nodes, expand],
  );
  useEffect(() => {
    const timer = setTimeout(() => {
      const firstStep = index.steps.get(pathId)?.[0]?.nodeId;
      const node =
        nodes.find((item) => item.id === selected) ||
        nodes.find((item) => item.id === firstStep) ||
        nodes[0];
      if (!node) return;
      if (selected) {
        setCenter(node.position.x + 118, node.position.y + 54, {
          zoom: 0.9,
          duration: 200,
        });
      } else {
        setViewport({
          x: 44 - node.position.x * 0.85,
          y: 80 - node.position.y * 0.85,
          zoom: 0.85,
        });
      }
    }, 80);
    return () => clearTimeout(timer);
  }, [pathId, expanded.length, allValidations, index.graph.map.id]);
  useEffect(() => {
    if (!selected) return;
    const node = nodes.find((item) => item.id === selected);
    if (node)
      setCenter(node.position.x + 118, node.position.y + 54, {
        zoom: 0.9,
        duration: 200,
      });
  }, [selected]);
  useEffect(() => {
    if (!query) return;
    const match = nodes.find((node) => node.data.matched);
    if (match) {
      selectNode(match.id);
      setCenter(match.position.x + 118, match.position.y + 54, {
        zoom: 0.92,
        duration: 300,
      });
    }
  }, [query]);
  function focus() {
    const node = getNode(selected);
    if (node)
      setCenter(node.position.x + 118, node.position.y + 54, {
        zoom: 1,
        duration: 250,
      });
    else fitView({ padding: 0.18, duration: 250 });
  }
  return (
    <div className="map-canvas" data-visible-nodes={nodes.length}>
      <ReactFlow
        nodes={nodes}
        edges={layout.edges}
        nodeTypes={nodeTypes}
        nodesDraggable={false}
        nodesConnectable={false}
        edgesReconnectable={false}
        elementsSelectable
        onNodeClick={(_event, node) => selectNode(node.id)}
        onPaneClick={() => selectNode("")}
        minZoom={0.08}
        maxZoom={1.8}
        proOptions={{ hideAttribution: true }}
      >
        <Background color="#d9dfe8" gap={18} size={1} />
        <MiniMap
          style={{ width: 155, height: 110 }}
          pannable
          zoomable
          nodeColor={(node) =>
            validationNode(node.data) ? "#c4b4d9" : "#ebaaa0"
          }
          maskColor="#f5f7fbad"
        />
        <Controls showInteractive={false} />
      </ReactFlow>
      <div className="canvas-top-status">
        <span>
          {view.nodes.length} / {index.nodes.length} 节点
        </span>
        {view.hidden > 0 && <span>已收拢 {view.hidden}</span>}
        {query && <span>找到 {view.matches}</span>}
      </div>
      <div className="canvas-top-actions">
        <IconButton icon={Focus} title="定位选中节点" onClick={focus} />
        <IconButton
          icon={Maximize2}
          title="适合画布"
          onClick={() => fitView({ padding: 0.18, duration: 250 })}
        />
      </div>
      <div className="map-canvas-legend">
        <span>
          <i />
          流程节点
        </span>
        <span>
          <i className="validation" />
          校验节点
        </span>
        <span className="edge-legend">虚线：路径 / 校验展示关系</span>
      </div>
    </div>
  );
}

export default function MapWorkspace({
  mapId,
  initialPath = "",
  openCase,
  openWorkflow,
  openEvidence,
  refreshMaps,
}) {
  const [document, setDocument] = useState(null);
  const [error, setError] = useState("");
  const [pathId, setPathId] = useState(initialPath);
  const [selected, setSelected] = useState("");
  const [query, setQuery] = useState("");
  const [expanded, setExpanded] = useState([]);
  const [allValidations, setAllValidations] = useState(false);
  const [tab, setTab] = useState("graph");
  const [planId, setPlanId] = useState("");
  const [plan, setPlan] = useState(null);
  const [assistantOpen, setAssistantOpen] = useState(false);
  async function load() {
    setError("");
    try {
      setDocument(
        (await api(`/api/map?${new URLSearchParams({ id: mapId })}`)).data,
      );
    } catch (error) {
      setError(error.message);
    }
  }
  useEffect(() => {
    setDocument(null);
    setPathId(initialPath);
    setSelected("");
    setExpanded([]);
    setPlan(null);
    setPlanId("");
    load();
  }, [mapId]);
  useEffect(() => {
    if (!planId) {
      setPlan(null);
      return;
    }
    let current = true;
    api(`/api/map/plan?${new URLSearchParams({ id: planId, mapId })}`)
      .then((result) => {
        if (current) setPlan(result.data.plan);
      })
      .catch((error) => {
        if (current) setError(error.message);
      });
    return () => {
      current = false;
    };
  }, [planId, mapId]);
  const index = useMemo(
    () => (document ? mapIndex(document) : null),
    [document],
  );
  const expand = useCallback(
    (id) =>
      setExpanded((values) =>
        values.includes(id)
          ? values.filter((value) => value !== id)
          : [...values, id],
      ),
    [],
  );
  if (error && !document) return <div className="notice error">{error}</div>;
  if (!index) return <Empty icon={Network} title="正在读取 Store 图谱" />;
  const paths = index.graph.paths || [];
  const currentPath = paths.find((path) => path.id === pathId);
  const node = index.byID.get(selected);
  return (
    <div className="map-workspace">
      <div className="editor-heading">
        <div>
          <Breadcrumb
            items={["测试图谱", index.graph.map.profileId || "当前目录"]}
          />
          <div className="editor-title">
            <Network size={20} className="accent" />
            <h2>{index.graph.map.displayName || mapId}</h2>
            <span className="soft-tag">
              {index.graph.map.status === "deprecated"
                ? "已废弃"
                : "Store 图谱"}
            </span>
          </div>
        </div>
        <div className="heading-actions">
          <button
            className={`button ${assistantOpen ? "primary" : ""}`}
            onClick={() => {
              setAssistantOpen((value) => !value);
              setTab("graph");
            }}
          >
            <Sparkles size={14} />
            AI 图谱助手
          </button>
          <IconButton icon={RefreshCw} title="刷新图谱" onClick={load} />
          {currentPath?.workflowId && (
            <button
              className="button"
              onClick={() => openWorkflow(currentPath.workflowId)}
            >
              <ArrowUpRight size={13} />
              关联场景
            </button>
          )}
        </div>
      </div>
      <div className="map-summary-strip">
        <span>
          <Network size={13} />
          {index.nodes.length} 节点
        </span>
        <span>
          <GitBranch size={13} />
          {paths.length} 路径
        </span>
        <span>
          <CheckCircle2 size={13} />
          {index.nodes.filter(validationNode).length} 校验
        </span>
        <span>更新 {dateLabel(index.graph.map.updatedAt)}</span>
      </div>
      <div className="map-control-bar">
        <select
          aria-label="图谱路径"
          value={pathId}
          onChange={(event) => {
            setPathId(event.target.value);
            setSelected("");
          }}
        >
          <option value="">全图 · 主流程概览</option>
          {paths.map((path) => (
            <option key={path.id} value={path.id}>
              {path.displayName || path.id} ·{" "}
              {index.steps.get(path.id)?.length || 0} 步
            </option>
          ))}
        </select>
        <SearchInput
          value={query}
          onChange={setQuery}
          placeholder="查找节点、接口或用例"
        />
        <label className="check-field">
          <input
            type="checkbox"
            checked={allValidations}
            onChange={(event) => setAllValidations(event.target.checked)}
          />
          展开全部校验
        </label>
        <select
          aria-label="历史计划"
          value={planId}
          onChange={(event) => setPlanId(event.target.value)}
        >
          <option value="">目录视图 · 不覆盖状态</option>
          {(document.plans || []).map((item) => (
            <option value={item.id} key={item.id}>
              {item.mode === "run" ? "运行" : "计划"} · {item.status} ·{" "}
              {dateLabel(item.createdAt)}
            </option>
          ))}
        </select>
      </div>
      <Tabs
        compact
        items={[
          { id: "graph", label: "图谱" },
          { id: "paths", label: "路径目录", count: paths.length },
          {
            id: "history",
            label: "计划与运行",
            count: document.plans?.length || 0,
          },
        ]}
        value={tab}
        onChange={setTab}
      />
      {error && <div className="notice error">{error}</div>}
      {document.warnings?.length > 0 && (
        <div className="notice">{document.warnings[0]}</div>
      )}
      {tab === "graph" ? (
        <div className="map-stage">
          <ReactFlowProvider>
            <Canvas
              index={index}
              pathId={pathId}
              query={query}
              selected={selected}
              selectNode={setSelected}
              expanded={expanded}
              expand={expand}
              allValidations={allValidations}
              plan={plan}
            />
          </ReactFlowProvider>
          {assistantOpen ? (
            <AssistantPanel
              key={mapId}
              mapId={mapId}
              mapName={index.graph.map.displayName || mapId}
              node={node}
              pathId={pathId}
              selectNode={setSelected}
              selectPath={(id) => {
                setPathId(id);
                setSelected("");
              }}
              openEvidence={openEvidence}
              close={() => setAssistantOpen(false)}
            />
          ) : (
            <MapInspector
              node={node}
              index={index}
              plan={plan}
              pathId={pathId}
              selectPath={(id) => {
                setPathId(id);
                setTab("graph");
              }}
              openCase={openCase}
              openWorkflow={openWorkflow}
              openEvidence={openEvidence}
              close={() => setSelected("")}
            />
          )}
        </div>
      ) : tab === "paths" ? (
        <div className="map-path-table">
          {paths.map((path) => (
            <section key={path.id}>
              <header>
                <GitBranch size={16} />
                <strong>{path.displayName || path.id}</strong>
                <span>{index.steps.get(path.id)?.length || 0} 步</span>
                <button
                  className="button"
                  onClick={() => {
                    setPathId(path.id);
                    setTab("graph");
                  }}
                >
                  查看路径
                </button>
              </header>
              <code>{path.id}</code>
              <div className="map-path-sequence">
                {(index.steps.get(path.id) || []).map((step) => (
                  <button
                    key={`${step.stepIndex}:${step.nodeId}`}
                    onClick={() => {
                      setPathId(path.id);
                      setSelected(step.nodeId);
                      setTab("graph");
                    }}
                  >
                    <span>{step.stepIndex}</span>
                    {index.byID.get(step.nodeId)?.name ||
                      step.caseId ||
                      step.nodeId}
                  </button>
                ))}
              </div>
            </section>
          ))}
        </div>
      ) : (
        <div className="map-history">
          {(document.plans || []).map((item) => (
            <button
              key={item.id}
              className={planId === item.id ? "selected" : ""}
              onClick={() => {
                setPlanId(item.id);
                setTab("graph");
              }}
            >
              <History size={16} />
              <span>
                <strong>
                  {item.mode === "run" ? "执行记录" : "规划记录"} ·{" "}
                  {item.targetId || item.scope}
                </strong>
                <code>{item.id}</code>
              </span>
              <Status value={item.status} />
              <span>{dateLabel(item.createdAt)}</span>
            </button>
          ))}
          {!document.plans?.length && (
            <Empty icon={History} title="还没有持久化计划或运行">
              图谱目录与运行状态分别展示，不把没有执行记录的节点标为通过。
            </Empty>
          )}
        </div>
      )}
    </div>
  );
}
