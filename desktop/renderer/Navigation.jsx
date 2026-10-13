import { useEffect, useState } from "react";
import {
  Boxes,
  GitBranch,
  FileText,
  Server,
  Cable,
  House,
  FolderOpen,
  ChevronDown,
  ChevronRight,
  Plus,
  RefreshCw,
  ArrowDownToLine,
  Network,
  Library,
} from "lucide-react";
import { IconButton, Method, SearchInput } from "./components.jsx";
import { filterRequests } from "./model.mjs";

export function Sidebar({
  module,
  workspace,
  requests,
  scenes,
  active,
  open,
  create,
  importRequests,
  refresh,
}) {
  const [query, setQuery] = useState("");
  const [collapsed, setCollapsed] = useState({});
  useEffect(() => {
    setQuery("");
  }, [module]);
  const heading = {
    apis: "接口管理",
    scenarios: "自动化测试",
    reports: "测试报告",
    environments: "运行环境",
    settings: "项目设置",
    maps: "测试图谱",
    library: "复用步骤库",
  }[module];
  const apiItems = filterRequests(requests, query);
  const services = workspace.catalog.services || [];
  const groupIDs = [
    ...new Set(
      apiItems.map((item) => item.serviceId || services[0]?.id || "http"),
    ),
  ];
  return (
    <aside className="sidebar">
      <div className="sidebar-project">
        <span className="project-avatar">
          <Boxes size={17} />
        </span>
        <div>
          <strong>{workspace.catalog.source?.id || "本机项目"}</strong>
          <span>AgentTestBench</span>
        </div>
        <ChevronDown size={14} />
      </div>
      <div className="sidebar-context">
        <span>
          <GitBranch size={13} />
          工作区目录
        </span>
        <span className="local-badge">
          {workspace.store.backend === "mysql"
            ? "MYSQL"
            : workspace.store.backend === "postgres"
              ? "PG"
              : "LOCAL"}
        </span>
      </div>
      <div className="sidebar-search">
        <SearchInput
          value={query}
          onChange={setQuery}
          placeholder={
            module === "scenarios" ? "搜索场景用例" : "搜索名称或路径"
          }
        />
        {["apis", "scenarios"].includes(module) && (
          <IconButton
            icon={Plus}
            title={module === "apis" ? "新建接口" : "新建场景"}
            onClick={() => create(module)}
          />
        )}
      </div>
      <div className="sidebar-heading">
        <span>{heading}</span>
        <IconButton icon={RefreshCw} title="刷新目录" onClick={refresh} />
      </div>
      <div className="tree-content">
        {module === "library" && (
          <>
            <button
              className="tree-overview"
              onClick={() =>
                open({ kind: "library", id: "all", name: "复用步骤库" })
              }
            >
              <Library size={15} />
              全部步骤组
            </button>
            {(workspace.library?.blocks || [])
              .filter((item) =>
                item.definition.displayName
                  .toLowerCase()
                  .includes(query.toLowerCase()),
              )
              .map((item) => (
                <button
                  className={`tree-item ${active === `library:${item.definition.id}` ? "active" : ""}`}
                  key={item.definition.id}
                  onClick={() =>
                    open({
                      kind: "library",
                      id: item.definition.id,
                      name: item.definition.displayName,
                    })
                  }
                >
                  <Library size={14} />
                  <span>{item.definition.displayName}</span>
                  <small>{item.definition.steps.length} 步</small>
                </button>
              ))}
          </>
        )}
        {module === "maps" && (
          <>
            <button
              className={`tree-overview ${active === "maps:all" ? "active" : ""}`}
              onClick={() =>
                open({ kind: "maps", id: "all", name: "测试图谱" })
              }
            >
              <Network size={15} />
              全部图谱<small>{workspace.maps?.length || 0}</small>
            </button>
            {(workspace.maps || [])
              .filter((item) =>
                `${item.displayName} ${item.id}`
                  .toLowerCase()
                  .includes(query.toLowerCase()),
              )
              .map((item) => (
                <button
                  className={`tree-item map-tree-item ${active === `map:${item.id}` ? "active" : ""} ${item.status === "deprecated" ? "deprecated" : ""}`}
                  key={item.id}
                  onClick={() =>
                    open({
                      kind: "map",
                      id: item.id,
                      name: item.displayName || item.id,
                    })
                  }
                >
                  <Network size={14} />
                  <span>
                    <strong>{item.displayName || item.id}</strong>
                    <small>
                      {item.nodeCount} 节点 · {item.pathCount} 路径
                      {item.status === "deprecated" ? " · 已废弃" : ""}
                    </small>
                  </span>
                </button>
              ))}
          </>
        )}

        {module === "apis" && (
          <>
            <button
              className={`tree-overview ${active === "overview:home" ? "active" : ""}`}
              onClick={() =>
                open({ kind: "overview", id: "home", name: "项目概览" })
              }
            >
              <House size={15} />
              项目概览
            </button>
            {groupIDs.map((serviceID) => {
              const group = apiItems.filter(
                (item) =>
                  (item.serviceId || services[0]?.id || "http") === serviceID,
              );
              const folded = collapsed[serviceID];
              return (
                <div className="tree-group" key={serviceID}>
                  <button
                    className="tree-folder"
                    onClick={() =>
                      setCollapsed({ ...collapsed, [serviceID]: !folded })
                    }
                  >
                    {folded ? (
                      <ChevronRight size={12} />
                    ) : (
                      <ChevronDown size={12} />
                    )}
                    <FolderOpen size={14} />
                    <span>
                      {services.find((item) => item.id === serviceID)
                        ?.displayName || "HTTP API"}
                    </span>
                    <small>{group.length}</small>
                  </button>
                  {!folded &&
                    group.map((item) => (
                      <button
                        className={`tree-item ${active === `request:${item.id}` ? "active" : ""}`}
                        key={item.id}
                        onClick={() =>
                          open({
                            kind: "request",
                            id: item.id,
                            name: item.displayName,
                          })
                        }
                      >
                        <Method value={item.method || item.execution?.method} />
                        <span>{item.displayName}</span>
                        {item.dirty && <i className="dirty-dot" />}
                      </button>
                    ))}
                </div>
              );
            })}
            <button className="sidebar-create" onClick={() => create("apis")}>
              <Plus size={14} />
              新建接口
            </button>
            <button className="sidebar-create" onClick={importRequests}>
              <ArrowDownToLine size={14} />
              导入接口 JSON
            </button>
          </>
        )}
        {module === "scenarios" && (
          <>
            <div className="tree-folder">
              <ChevronDown size={12} />
              <FolderOpen size={14} />
              <span>场景用例</span>
              <small>{scenes.length}</small>
            </div>
            {scenes
              .filter((item) =>
                item.workflow.displayName
                  .toLowerCase()
                  .includes(query.toLowerCase()),
              )
              .map((item) => (
                <button
                  className={`tree-item scene-tree-item ${active === `scenario:${item.workflow.id}` ? "active" : ""}`}
                  key={item.workflow.id}
                  onClick={() =>
                    open({
                      kind: "scenario",
                      id: item.workflow.id,
                      name: item.workflow.displayName,
                    })
                  }
                >
                  <GitBranch size={14} />
                  <span>{item.workflow.displayName}</span>
                  {item.dirty && <i className="dirty-dot" />}
                  <small>{item.steps.length}</small>
                </button>
              ))}
            <button
              className="sidebar-create"
              onClick={() => create("scenarios")}
            >
              <Plus size={14} />
              新建场景用例
            </button>
          </>
        )}
        {module === "reports" && (
          <>
            <button
              className="tree-overview active"
              onClick={() =>
                open({ kind: "reports", id: "all", name: "测试报告" })
              }
            >
              <FileText size={15} />
              全部运行记录
            </button>
            <div className="tree-folder">
              <ChevronDown size={12} />
              <FolderOpen size={14} />
              按接口筛选
            </div>
            {requests
              .filter((item) =>
                item.displayName.toLowerCase().includes(query.toLowerCase()),
              )
              .map((item) => (
                <button
                  className="tree-item"
                  key={item.id}
                  onClick={() =>
                    open({
                      kind: "reports",
                      id: item.id,
                      name: `${item.displayName} · 报告`,
                    })
                  }
                >
                  <Method value={item.method || item.execution?.method} />
                  <span>{item.displayName}</span>
                </button>
              ))}
          </>
        )}
        {module === "environments" && (
          <>
            <button
              className="tree-overview active"
              onClick={() =>
                open({ kind: "environments", id: "all", name: "运行环境" })
              }
            >
              <Server size={15} />
              全部环境
            </button>
            {workspace.environments
              .filter((item) =>
                `${item.displayName} ${item.id}`
                  .toLowerCase()
                  .includes(query.toLowerCase()),
              )
              .map((item) => (
                <button
                  className="tree-item"
                  key={item.id}
                  onClick={() =>
                    open({
                      kind: "environments",
                      id: item.id,
                      name: item.displayName || item.id,
                    })
                  }
                >
                  <Server size={14} />
                  <span>{item.displayName || item.id}</span>
                  <i
                    className={`environment-dot ${item.verified ? "verified" : ""}`}
                  />
                </button>
              ))}
          </>
        )}
        {module === "settings" && (
          <button
            className="tree-overview active"
            onClick={() =>
              open({ kind: "settings", id: "project", name: "项目设置" })
            }
          >
            <Cable size={15} />
            工作区与项目配置
          </button>
        )}
      </div>
      <div className="sidebar-bottom">
        <span className="quiet-dot" />
        配置保存至 SQL Store
      </div>
    </aside>
  );
}
