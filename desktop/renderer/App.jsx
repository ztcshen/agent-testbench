import { useEffect, useRef, useState } from "react";
import {
  Boxes,
  Braces,
  GitBranch,
  FileText,
  Server,
  Settings,
  Plus,
  ChevronDown,
  ChevronRight,
  Folder,
  FolderOpen,
  Search,
  RefreshCw,
  PanelLeftClose,
  PanelLeftOpen,
  X,
  Cable,
  Download,
  CheckCircle2,
  AlertCircle,
  LoaderCircle,
  House,
  ArrowDownToLine,
} from "lucide-react";
import {
  emptyStudioDraft,
  buildStudioSavePayload,
  validateStudioDraft,
} from "../../control-plane/frontend/src/workflowStudioModel.mjs";
import {
  api,
  loadWorkspace,
  saveRequest,
  runRequest,
  runScenario,
} from "./api.mjs";
import {
  compileRequest,
  newRequest,
  requestDraft,
  reconcileDrafts,
  filterRequests,
} from "./model.mjs";
import {
  IconButton,
  Method,
  Modal,
  SearchInput,
  Empty,
} from "./components.jsx";
import { Sidebar } from "./Navigation.jsx";
import RequestEditor from "./RequestEditor.jsx";
import ScenarioEditor from "./ScenarioEditor.jsx";
import {
  Overview,
  Reports,
  Evidence,
  Environments,
  Settings as SettingsPanel,
} from "./WorkspacePanels.jsx";

const emptyWorkspace = {
  requests: [],
  studio: { workflows: [], cases: [] },
  catalog: { services: [] },
  runs: [],
  environments: [],
  store: {},
  issues: [],
};
const navigation = [
  { id: "apis", label: "接口管理", Icon: Braces },
  { id: "scenarios", label: "自动化测试", Icon: GitBranch },
  { id: "reports", label: "测试报告", Icon: FileText },
  { id: "environments", label: "运行环境", Icon: Server },
];
const sceneDraft = (record) => ({ ...record, dirty: false });
const sceneID = (record) => record.workflow.id;
const tabKey = (tab) => `${tab.kind}:${tab.id}`;

function ProductMark() {
  return (
    <svg width="28" height="28" viewBox="0 0 32 32" aria-hidden="true">
      <rect width="32" height="32" rx="9" fill="#f4665a" />
      <path d="M10 9h12v4h-4v10h-4V13h-4Z" fill="white" />
      <circle cx="23" cy="23" r="3" fill="#ffd8cb" />
    </svg>
  );
}

export default function App() {
  const [workspace, setWorkspace] = useState(emptyWorkspace);
  const [connection, setConnection] = useState({ connected: false });
  const [requests, setRequests] = useState({});
  const [scenes, setScenes] = useState({});
  const [tabs, setTabs] = useState([]);
  const [active, setActive] = useState("");
  const [module, setModule] = useState("apis");
  const [sidebar, setSidebar] = useState(true);
  const [loading, setLoading] = useState(true);
  const [busy, setBusy] = useState(new Set());
  const [results, setResults] = useState({});
  const [sceneResults, setSceneResults] = useState({});
  const [fatal, setFatal] = useState("");
  const [toast, setToast] = useState(null);
  const [confirmation, setConfirmation] = useState(null);
  const [importing, setImporting] = useState(null);
  const activeRef = useRef(false);
  const runAbort = useRef(new AbortController());
  const requestList = Object.values(requests).map((item) => item.draft);
  const sceneList = Object.values(scenes).map((item) => item.draft);
  const anyDirty =
    requestList.some((item) => item.dirty) ||
    sceneList.some((item) => item.dirty);
  useEffect(() => {
    const guard = (event) => {
      if (anyDirty) {
        event.preventDefault();
        event.returnValue = "";
      }
    };
    window.addEventListener("beforeunload", guard);
    return () => window.removeEventListener("beforeunload", guard);
  }, [anyDirty]);
  function notify(message, error = false) {
    setToast({ message, error });
    setTimeout(
      () =>
        setToast((current) => (current?.message === message ? null : current)),
      4500,
    );
  }
  function open(tab) {
    const key = tabKey(tab);
    setTabs((current) =>
      current.some((item) => tabKey(item) === key)
        ? current
        : [...current, tab],
    );
    setActive(key);
    setModule(
      tab.kind === "request" || tab.kind === "overview"
        ? "apis"
        : tab.kind === "scenario"
          ? "scenarios"
          : tab.kind === "evidence"
            ? "reports"
            : tab.kind,
    );
  }
  async function refresh() {
    const next = await loadWorkspace();
    setWorkspace(next);
    setRequests((current) =>
      reconcileDrafts(current, next.requests, next.requestETag),
    );
    setScenes((current) =>
      reconcileDrafts(
        current,
        next.studio.workflows || [],
        next.studioETag,
        sceneDraft,
        sceneID,
      ),
    );
    setFatal("");
    return next;
  }
  useEffect(() => {
    activeRef.current = true;
    (async () => {
      try {
        const current = await window.testbench.connection();
        setConnection(current);
        if (!current.connected)
          throw new Error(current.error || "尚未连接执行引擎");
        const next = await refresh();
        if (next.requests.length)
          open({
            kind: "request",
            id: next.requests[0].id,
            name: next.requests[0].displayName,
          });
        else open({ kind: "overview", id: "home", name: "项目概览" });
      } catch (error) {
        setFatal(error.message);
        open({ kind: "settings", id: "project", name: "项目设置" });
      } finally {
        setLoading(false);
      }
    })();
    return () => {
      activeRef.current = false;
      runAbort.current.abort();
    };
  }, []);

  async function job(key, action) {
    setBusy((current) => new Set([...current, key]));
    try {
      return await action();
    } catch (error) {
      notify(error.message, true);
      return null;
    } finally {
      setBusy((current) => {
        const next = new Set(current);
        next.delete(key);
        return next;
      });
    }
  }
  function create(type) {
    if (type === "apis") {
      const draft = { ...newRequest(workspace.catalog.services), dirty: true };
      setRequests((current) => ({
        ...current,
        [draft.id]: { draft, etag: workspace.requestETag, baseline: "" },
      }));
      open({ kind: "request", id: draft.id, name: draft.displayName });
    } else {
      const draft = { ...emptyStudioDraft(), dirty: true };
      setScenes((current) => ({
        ...current,
        [draft.workflow.id]: {
          draft,
          etag: workspace.studioETag,
          baseline: "",
        },
      }));
      open({
        kind: "scenario",
        id: draft.workflow.id,
        name: draft.workflow.displayName,
      });
    }
  }
  function patchRequest(id, patch) {
    setRequests((current) => ({
      ...current,
      [id]: {
        ...current[id],
        draft: { ...current[id].draft, ...patch, dirty: true },
      },
    }));
  }
  function patchScene(id, draft) {
    setScenes((current) => ({
      ...current,
      [id]: { ...current[id], draft: { ...draft, dirty: true } },
    }));
  }
  async function persistRequest(id) {
    const current = requests[id];
    const record = compileRequest(current.draft);
    const response = await saveRequest(record, current.etag);
    setRequests((previous) => {
      const cleared = { ...previous };
      delete cleared[id];
      return reconcileDrafts(cleared, response.data.requests, response.etag);
    });
    await refresh();
    return record;
  }
  async function persistScene(id) {
    const current = scenes[id];
    const errors = validateStudioDraft(current.draft, workspace.studio.cases);
    if (errors.length) throw new Error(errors[0]);
    const response = await api("/api/workflow-studio", {
      method: "PUT",
      etag: current.etag,
      body: buildStudioSavePayload(current.draft),
    });
    setScenes((previous) => {
      const cleared = { ...previous };
      delete cleared[id];
      return reconcileDrafts(
        cleared,
        response.data.workflows,
        response.etag,
        sceneDraft,
        sceneID,
      );
    });
    await refresh();
  }
  function save(id, scene = false) {
    return job(id, async () => {
      await (scene ? persistScene(id) : persistRequest(id));
      notify("已保存至 Store");
    });
  }
  function send(id, overrides) {
    return job(id, async () => {
      if (requests[id].draft.dirty) await persistRequest(id);
      const response = await runRequest(id, overrides);
      setResults((current) => ({ ...current, [id]: response.data }));
      await refresh();
      notify(
        response.data.status === "passed"
          ? "请求执行通过"
          : "请求已完成，请查看断言结果",
      );
    });
  }
  function run(id) {
    return job(id, async () => {
      if (scenes[id].draft.dirty) await persistScene(id);
      await runScenario(
        id,
        (report) => {
          if (activeRef.current)
            setSceneResults((current) => ({ ...current, [id]: report }));
        },
        runAbort.current.signal,
      );
      await refresh();
      notify("场景执行完成");
    });
  }
  function removeTab(tab, discard = false) {
    const dirty =
      tab.kind === "request"
        ? requests[tab.id]?.draft.dirty
        : tab.kind === "scenario"
          ? scenes[tab.id]?.draft.dirty
          : false;
    if (dirty && !discard) {
      setConfirmation(tab);
      return;
    }
    if (discard) {
      if (tab.kind === "request")
        setRequests((current) => {
          const next = { ...current };
          const saved = workspace.requests.find((item) => item.id === tab.id);
          if (saved)
            next[tab.id] = {
              draft: requestDraft(saved),
              baseline: JSON.stringify(saved),
              etag: workspace.requestETag,
            };
          else delete next[tab.id];
          return next;
        });
      if (tab.kind === "scenario")
        setScenes((current) => {
          const next = { ...current };
          const saved = workspace.studio.workflows.find(
            (item) => item.workflow.id === tab.id,
          );
          if (saved)
            next[tab.id] = {
              draft: sceneDraft(saved),
              baseline: JSON.stringify(saved),
              etag: workspace.studioETag,
            };
          else delete next[tab.id];
          return next;
        });
    }
    const remaining = tabs.filter((item) => tabKey(item) !== tabKey(tab));
    setTabs(remaining);
    if (active === tabKey(tab)) {
      if (remaining.length) open(remaining.at(-1));
      else open({ kind: "overview", id: "home", name: "项目概览" });
    }
    setConfirmation(null);
  }
  async function connect(settings) {
    if (anyDirty || busy.size)
      throw new Error("请先保存当前编辑并等待运行结束");
    const response = await window.testbench.connect(settings);
    if (!response.ok) {
      setConnection({ ...response, connected: false });
      setFatal(response.error);
      throw new Error(response.error);
    }
    setConnection(response);
    setRequests({});
    setScenes({});
    setResults({});
    setSceneResults({});
    setTabs([]);
    await refresh();
    open({ kind: "overview", id: "home", name: "项目概览" });
    notify("工作区已连接");
  }
  async function importRequests() {
    try {
      const file = await window.testbench.importJSON();
      if (file.canceled) return;
      const records =
        file.value.requests || (file.value.id ? [file.value] : []);
      if (!records.length) throw new Error("文件中没有 TestBench 接口配置");
      setImporting(records);
    } catch (error) {
      notify(error.message, true);
    }
  }
  async function applyImport() {
    await job("import", async () => {
      let etag = workspace.requestETag;
      for (const record of importing) {
        const draft = requestDraft({
          ...record,
          serviceId: record.serviceId || workspace.catalog.services[0]?.id,
          nodeId: record.nodeId || newRequest().nodeId,
          displayName: record.displayName || "导入接口",
        });
        const response = await saveRequest(compileRequest(draft), etag);
        etag = response.etag;
      }
      setImporting(null);
      await refresh();
      notify("接口已导入 Store");
    });
  }
  const tab = tabs.find((item) => tabKey(item) === active);
  function title(item) {
    return item.kind === "request"
      ? requests[item.id]?.draft.displayName || item.name
      : item.kind === "scenario"
        ? scenes[item.id]?.draft.workflow.displayName || item.name
        : item.name;
  }
  function view() {
    if (!tab) return <Empty title="正在打开工作区" />;
    if (tab.kind === "request" && requests[tab.id])
      return (
        <RequestEditor
          key={tab.id}
          draft={requests[tab.id].draft}
          patch={(patch) => patchRequest(tab.id, patch)}
          save={() => save(tab.id)}
          send={(overrides) => send(tab.id, overrides)}
          result={results[tab.id]}
          busy={busy.has(tab.id)}
          services={workspace.catalog.services}
        />
      );
    if (tab.kind === "scenario" && scenes[tab.id])
      return (
        <ScenarioEditor
          key={tab.id}
          draft={scenes[tab.id].draft}
          cases={workspace.studio.cases}
          patch={(draft) => patchScene(tab.id, draft)}
          save={() => save(tab.id, true)}
          run={() => run(tab.id)}
          report={sceneResults[tab.id]}
          busy={busy.has(tab.id)}
        />
      );
    if (tab.kind === "scenarios")
      return (
        <div className="overview">
          <h2>场景用例</h2>
          <Empty icon={GitBranch} title="创建第一个测试场景">
            从接口导入步骤，设置参数来源，按顺序运行并查看结果。
          </Empty>
          <button
            className="button primary"
            onClick={() => create("scenarios")}
          >
            <Plus size={14} />
            新建场景用例
          </button>
        </div>
      );
    if (tab.kind === "reports")
      return (
        <Reports
          key={tab.id}
          runs={workspace.runs}
          requests={workspace.requests}
          filterID={tab.id === "all" ? "" : tab.id}
          open={open}
          refresh={() => job("refresh", refresh)}
        />
      );
    if (tab.kind === "evidence") return <Evidence run={tab.run} />;
    if (tab.kind === "environments")
      return (
        <Environments
          items={workspace.environments}
          workflows={workspace.studio.workflows}
          selectedID={tab.id}
          refresh={refresh}
          notify={notify}
          activity={(value) =>
            setBusy((current) => {
              const next = new Set(current);
              if (value) next.add("environment");
              else next.delete("environment");
              return next;
            })
          }
        />
      );
    if (tab.kind === "settings")
      return (
        <SettingsPanel
          connection={connection}
          connect={connect}
          locked={anyDirty || busy.size > 0}
          workspace={workspace}
          importRequests={importRequests}
          notify={notify}
        />
      );
    return <Overview workspace={workspace} open={open} create={create} />;
  }
  function navigate(id) {
    setModule(id);
    if (id === "apis") {
      const first = requestList[0];
      open(
        first
          ? { kind: "request", id: first.id, name: first.displayName }
          : { kind: "overview", id: "home", name: "项目概览" },
      );
    } else if (id === "scenarios") {
      const first = sceneList[0];
      open(
        first
          ? {
              kind: "scenario",
              id: first.workflow.id,
              name: first.workflow.displayName,
            }
          : { kind: "scenarios", id: "all", name: "场景用例" },
      );
    } else
      open({
        kind: id,
        id: id === "settings" ? "project" : "all",
        name:
          id === "settings"
            ? "项目设置"
            : navigation.find((item) => item.id === id)?.label,
      });
  }

  return (
    <div className="desktop-app">
      <header className="titlebar">
        <div className="traffic-space" />
        <button
          className="workspace-chip"
          onClick={() =>
            open({ kind: "overview", id: "home", name: "项目概览" })
          }
        >
          <Boxes size={15} />
          <span>本机工作区</span>
          <ChevronDown size={12} />
        </button>
        <span className="window-title">AgentTestBench</span>
        <IconButton
          icon={RefreshCw}
          title="刷新项目"
          onClick={() => job("refresh", refresh)}
        />
        <IconButton
          icon={Settings}
          title="项目设置"
          onClick={() => navigate("settings")}
        />
        <span className="local-user">L</span>
      </header>
      <div className="desktop-body">
        <nav className="activity-rail">
          <div className="rail-logo">
            <ProductMark />
          </div>
          {navigation.map(({ id, label, Icon }) => (
            <button
              key={id}
              aria-label={label}
              className={module === id ? "active" : ""}
              onClick={() => navigate(id)}
            >
              <Icon size={20} strokeWidth={1.65} />
              <span>{label}</span>
            </button>
          ))}
          <div className="rail-spacer" />
          <button
            aria-label="项目设置导航"
            className={module === "settings" ? "active" : ""}
            onClick={() => navigate("settings")}
          >
            <Settings size={20} strokeWidth={1.65} />
            <span>项目设置</span>
          </button>
          <div className="rail-local">
            <span>本机</span>
          </div>
        </nav>
        {sidebar && (
          <Sidebar
            module={module}
            workspace={workspace}
            requests={requestList}
            scenes={sceneList}
            active={active}
            open={open}
            create={create}
            importRequests={importRequests}
            refresh={() => job("refresh", refresh)}
          />
        )}
        <main className="main-workspace">
          <div className="editor-tabs" role="tablist" aria-label="打开的编辑器">
            {tabs.map((item) => {
              const dirty =
                item.kind === "request"
                  ? requests[item.id]?.draft.dirty
                  : item.kind === "scenario"
                    ? scenes[item.id]?.draft.dirty
                    : false;
              return (
                <div
                  key={tabKey(item)}
                  className={`editor-tab ${active === tabKey(item) ? "active" : ""}`}
                >
                  <button
                    role="tab"
                    aria-selected={active === tabKey(item)}
                    onClick={() => open(item)}
                  >
                    {item.kind === "request" ? (
                      <Method value={requests[item.id]?.draft.method} />
                    ) : item.kind === "scenario" ? (
                      <GitBranch size={13} />
                    ) : item.kind === "settings" ? (
                      <Settings size={13} />
                    ) : (
                      <FileText size={13} />
                    )}
                    <span>{title(item)}</span>
                    {dirty && <i className="dirty-dot" />}
                  </button>
                  <button
                    className="close-tab"
                    title={`关闭 ${title(item)}`}
                    onClick={() => removeTab(item)}
                  >
                    <X size={12} />
                  </button>
                </div>
              );
            })}
            <IconButton
              icon={Plus}
              title={module === "scenarios" ? "新建场景标签" : "新建接口标签"}
              onClick={() =>
                create(module === "scenarios" ? "scenarios" : "apis")
              }
            />
            <span className="tabs-spacer" />
            <button
              className="environment-selector"
              aria-label="管理运行环境"
              onClick={() => navigate("environments")}
            >
              <Server size={13} />
              <span>运行环境</span>
              <ChevronDown size={12} />
            </button>
          </div>
          {fatal && (
            <div className="notice error connection-error">
              <AlertCircle size={15} />
              {fatal}
            </div>
          )}
          <div className="workspace-content">
            {loading ? (
              <Empty icon={LoaderCircle} title="正在连接 TestBench" />
            ) : (
              view()
            )}
          </div>
        </main>
      </div>
      <footer className="statusbar">
        <IconButton
          icon={sidebar ? PanelLeftClose : PanelLeftOpen}
          title={sidebar ? "折叠目录" : "展开目录"}
          onClick={() => setSidebar(!sidebar)}
        />
        <span
          className={`connection-dot ${connection.connected ? "online" : ""}`}
        />
        <span>
          {connection.connected ? "执行引擎已连接" : "执行引擎未连接"}
        </span>
        <span className="statusbar-divider" />
        <DatabaseLabel store={workspace.store} />
        <span className="statusbar-spacer" />
        {busy.size > 0 && (
          <span>
            <LoaderCircle size={12} className="spin" />
            处理中
          </span>
        )}
        <span>目录版本 {workspace.studio.revision ?? "—"}</span>
        <span className="statusbar-divider" />
        <span>Electron 桌面端</span>
      </footer>
      {toast && (
        <div role="status" className={`toast ${toast.error ? "error" : ""}`}>
          {toast.error ? <AlertCircle size={16} /> : <CheckCircle2 size={16} />}
          <span>{toast.message}</span>
          <IconButton
            icon={X}
            title="关闭提示"
            onClick={() => setToast(null)}
          />
        </div>
      )}
      {confirmation && (
        <Modal title="保留未保存的编辑？" onClose={() => setConfirmation(null)}>
          <div className="modal-body">
            <p>「{title(confirmation)}」还有未保存的内容。</p>
          </div>
          <footer>
            <button className="button" onClick={() => setConfirmation(null)}>
              继续编辑
            </button>
            <button
              className="button danger"
              onClick={() => removeTab(confirmation, true)}
            >
              放弃并关闭
            </button>
          </footer>
        </Modal>
      )}
      {importing && (
        <Modal
          title="导入接口配置"
          onClose={() => {
            if (!busy.has("import")) setImporting(null);
          }}
        >
          <div className="modal-body">
            <p>
              将 {importing.length} 个接口保存到当前 Store。同 ID
              的已有接口会更新。
            </p>
            {importing.slice(0, 8).map((item) => (
              <p className="import-preview" key={item.id}>
                <Method value={item.execution?.method} />
                {item.displayName || item.id}
              </p>
            ))}
          </div>
          <footer>
            <button
              className="button"
              disabled={busy.has("import")}
              onClick={() => setImporting(null)}
            >
              取消
            </button>
            <button
              className="button primary"
              disabled={anyDirty || busy.size > 0}
              onClick={applyImport}
            >
              导入接口
            </button>
          </footer>
        </Modal>
      )}
    </div>
  );
}
function DatabaseLabel({ store }) {
  return (
    <span className="store-label">
      <Boxes size={12} />
      {String(
        store.backend || store.storeInfo?.backend || "SQL Store",
      ).toUpperCase()}{" "}
      Store
    </span>
  );
}
