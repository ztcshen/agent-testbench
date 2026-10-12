import { useState } from "react";
import { Network, GitBranch, ArrowUpRight, RefreshCw } from "lucide-react";
import { Breadcrumb, SearchInput, Empty } from "./components.jsx";

export default function MapLibrary({ maps, open, refresh }) {
  const [query, setQuery] = useState("");
  const [historical, setHistorical] = useState(false);
  const filtered = maps.filter(
    (item) =>
      (historical || item.status !== "deprecated") &&
      `${item.displayName} ${item.id} ${item.description}`
        .toLowerCase()
        .includes(query.toLowerCase()),
  );
  return (
    <div className="map-library">
      <div className="panel-heading">
        <div>
          <Breadcrumb items={["测试工作区", "测试图谱"]} />
          <h2>测试图谱</h2>
          <p className="muted">
            从当前 Store 读取共享节点、业务路径、校验用例与运行记录。
          </p>
        </div>
        <button className="button" onClick={refresh}>
          <RefreshCw size={13} />
          刷新目录
        </button>
      </div>
      <div className="list-toolbar">
        <SearchInput
          value={query}
          onChange={setQuery}
          placeholder="搜索 map 名称、ID 或说明"
        />
        <label className="check-field">
          <input
            type="checkbox"
            checked={historical}
            onChange={(event) => setHistorical(event.target.checked)}
          />
          显示已废弃图谱
        </label>
        <span className="muted">
          {maps.length} 个图谱 ·{" "}
          {maps.filter((item) => item.status !== "deprecated").length} 个在用
        </span>
      </div>
      <div className="map-library-grid">
        {filtered.map((item) => (
          <button
            key={item.id}
            className={`map-library-card ${item.status === "deprecated" ? "deprecated" : ""}`}
            onClick={() =>
              open({
                kind: "map",
                id: item.id,
                name: item.displayName || item.id,
              })
            }
          >
            <div>
              <span className="map-library-symbol">
                <Network size={24} />
              </span>
              <span className="soft-tag">
                {item.status === "deprecated" ? "已废弃" : "在用"}
              </span>
              <ArrowUpRight size={16} />
            </div>
            <h3>{item.displayName || item.id}</h3>
            <code>{item.id}</code>
            <p>{item.description || "该图谱已登记在当前 Store。"}</p>
            <footer>
              <span>
                <Network size={13} />
                {item.nodeCount} 节点
              </span>
              <span>
                <GitBranch size={13} />
                {item.pathCount} 路径
              </span>
            </footer>
          </button>
        ))}
      </div>
      {!filtered.length && (
        <Empty
          icon={Network}
          title={maps.length ? "没有匹配的图谱" : "当前 Store 还没有 map"}
        >
          可在项目设置中连接已有 Store。已登记的 map 会直接出现在这里。
        </Empty>
      )}
    </div>
  );
}
