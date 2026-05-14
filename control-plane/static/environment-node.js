const environmentNodeEl = (id) => document.getElementById(id);

async function environmentNodeRequest(path) {
  const response = await fetch(path);
  const body = await response.json();
  if (!response.ok) {
    throw new Error(body.error || response.statusText);
  }
  return body;
}

function setEnvironmentNodeStatus(value) {
  environmentNodeEl("environmentNodeStatus").textContent = value;
}

function environmentNodeStatusText(item) {
  if (item.state === "missing") {
    return "未运行";
  }
  if (item.health && item.health !== "unknown") {
    return item.health;
  }
  return item.state || "unknown";
}

function requestedEnvironmentNodeId() {
  return new URLSearchParams(window.location.search).get("id") || "";
}

function flattenEnvironmentNodes(snapshot) {
  return (snapshot.groups || []).flatMap((group) =>
    (group.items || []).map((item) => ({
      ...item,
      groupLabel: group.label,
      groupId: group.id,
    })),
  );
}

function environmentNodeRuntimeById(snapshot) {
  return new Map((snapshot.serviceRuntime || []).map((item) => [item.serviceId, item]));
}

function environmentNodeRuntimeRows(item, runtime) {
  const role = runtime?.nodeRole || item.group || "";
  const showRepo = Boolean(runtime?.branchName || runtime?.commitId) && !["middleware", "platform", "observability", "external"].includes(role);
  if (showRepo) {
    return [
      ["branch_name", runtime?.branchName || "-"],
      ["commit_id", runtime?.commitId || "-"],
    ];
  }
  return [
    ["image_version", runtime?.imageVersion || item.version || "-"],
    ["image", runtime?.image || item.image || "-"],
    ["container_state", runtime?.containerState || item.state || "-"],
    ["health", runtime?.health || item.health || "-"],
  ];
}

function statBox(label, value) {
  const box = document.createElement("div");
  const span = document.createElement("span");
  span.textContent = label;
  const strong = document.createElement("strong");
  strong.textContent = value;
  box.appendChild(span);
  box.appendChild(strong);
  return box;
}

function renderEnvironmentNodeStats(item) {
  const target = environmentNodeEl("environmentNodeStats");
  target.innerHTML = "";
  target.appendChild(statBox("状态", environmentNodeStatusText(item)));
  target.appendChild(statBox("端口", item.port ? `:${item.port}` : "-"));
  target.appendChild(statBox("Mgmt", item.managementPort ? `:${item.managementPort}` : "-"));
  target.appendChild(statBox("分组", item.groupLabel || item.group || "-"));
}

function detailItem(label, value) {
  const dt = document.createElement("dt");
  dt.textContent = label;
  const dd = document.createElement("dd");
  dd.textContent = value || "-";
  return [dt, dd];
}

function appendDetails(list, rows) {
  rows.forEach(([label, value]) => {
    const [dt, dd] = detailItem(label, value);
    list.appendChild(dt);
    list.appendChild(dd);
  });
}

function renderActionLink(label, href) {
  const link = document.createElement("a");
  link.className = "button-link";
  link.href = href;
  link.textContent = label;
  return link;
}

function renderEnvironmentNodePeers(item, snapshot) {
  const panel = document.createElement("section");
  panel.className = "environment-node-detail-panel environment-node-peers-panel";
  const head = document.createElement("div");
  head.className = "dashboard-section-head";
  head.innerHTML = "<h2>同组节点</h2><p>同一环境分组里的当前服务状态</p>";
  const list = document.createElement("div");
  list.className = "environment-node-peer-list";
  flattenEnvironmentNodes(snapshot)
    .filter((candidate) => candidate.groupId === item.groupId)
    .forEach((candidate) => {
      const link = document.createElement("a");
      link.className = `environment-node-peer ${candidate.id === item.id ? "active" : ""}`;
      link.href = `/environment-node.html?id=${encodeURIComponent(candidate.id)}`;
      const name = document.createElement("strong");
      name.textContent = candidate.name || candidate.id;
      const status = document.createElement("span");
      status.textContent = environmentNodeStatusText(candidate);
      link.appendChild(name);
      link.appendChild(status);
      list.appendChild(link);
    });
  panel.appendChild(head);
  panel.appendChild(list);
  return panel;
}

function renderEnvironmentNodeSnapshotSummary(snapshot) {
  const summary = snapshot.summary || {};
  const panel = document.createElement("section");
  panel.className = "environment-node-detail-panel environment-node-summary-panel";
  const head = document.createElement("div");
  head.className = "dashboard-section-head";
  head.innerHTML = "<h2>环境快照</h2><p>当前 Control plane 看到的全局健康计数</p>";
  const list = document.createElement("dl");
  list.className = "environment-node-detail-list";
  appendDetails(list, [
    ["total", String(summary.total || 0)],
    ["healthy", String(summary.healthy || 0)],
    ["missing", String(summary.missing || 0)],
    ["unhealthy", String(summary.unhealthy || 0)],
  ]);
  panel.appendChild(head);
  panel.appendChild(list);
  return panel;
}

function renderEnvironmentNodeMissingRequest(nodeId) {
  const panel = document.createElement("section");
  panel.className = "environment-node-detail-panel";
  const head = document.createElement("div");
  head.className = "dashboard-section-head";
  head.innerHTML = "<h2>请求信息</h2><p>当前 URL 没有匹配到环境快照中的节点。</p>";
  const list = document.createElement("dl");
  list.className = "environment-node-detail-list";
  appendDetails(list, [
    ["requested id", nodeId || "missing query parameter"],
    ["reason", "not found in /api/dashboard groups"],
  ]);
  const marker = document.createElement("code");
  marker.className = "environment-node-missing-id";
  marker.textContent = nodeId || "id 参数为空";
  const actions = document.createElement("div");
  actions.className = "environment-node-detail-actions";
  actions.appendChild(renderActionLink("返回环境节点", "/environment-nodes.html"));
  panel.appendChild(head);
  panel.appendChild(marker);
  panel.appendChild(list);
  panel.appendChild(actions);
  return panel;
}

function renderEnvironmentNodeMissingCandidates(snapshot) {
  const panel = document.createElement("section");
  panel.className = "environment-node-detail-panel";
  const head = document.createElement("div");
  head.className = "dashboard-section-head";
  head.innerHTML = "<h2>可选节点</h2><p>从当前环境快照选择一个真实存在的服务。</p>";
  const list = document.createElement("div");
  list.className = "environment-node-candidate-list";
  flattenEnvironmentNodes(snapshot).forEach((candidate) => {
    const link = document.createElement("a");
    link.className = "environment-node-candidate";
    link.href = `/environment-node.html?id=${encodeURIComponent(candidate.id)}`;
    const name = document.createElement("strong");
    name.textContent = candidate.name || candidate.id;
    const meta = document.createElement("span");
    meta.textContent = `${candidate.groupLabel || candidate.group || "节点"} · ${environmentNodeStatusText(candidate)}`;
    link.appendChild(name);
    link.appendChild(meta);
    list.appendChild(link);
  });
  panel.appendChild(head);
  panel.appendChild(list);
  return panel;
}

function renderEnvironmentNodeMissingSnapshotIndex(snapshot) {
  const panel = document.createElement("section");
  panel.className = "environment-node-detail-panel environment-node-missing-snapshot";
  const head = document.createElement("div");
  head.className = "dashboard-section-head";
  head.innerHTML = "<h2>快照索引</h2><p>用于确认当前可恢复节点来自同一次 /api/dashboard 快照。</p>";
  const pre = document.createElement("pre");
  pre.textContent = JSON.stringify(
    {
      summary: snapshot.summary || {},
      groups: (snapshot.groups || []).map((group) => ({
        id: group.id,
        label: group.label,
        count: (group.items || []).length,
        nodes: (group.items || []).map((item) => item.id),
      })),
    },
    null,
    2,
  );
  panel.appendChild(head);
  panel.appendChild(pre);
  return panel;
}

function renderEnvironmentNodeRawSnapshot(item) {
  const panel = document.createElement("section");
  panel.className = "environment-node-detail-panel environment-node-raw-snapshot";
  const details = document.createElement("details");
  details.open = true;
  const head = document.createElement("summary");
  head.textContent = "原始快照字段";
  const pre = document.createElement("pre");
  pre.textContent = JSON.stringify(item, null, 2);
  details.appendChild(head);
  details.appendChild(pre);
  panel.appendChild(details);
  return panel;
}

function renderEnvironmentNodeRuntimeMetadata(item, snapshot) {
  const runtime = environmentNodeRuntimeById(snapshot).get(item.id) || {};
  const panel = document.createElement("section");
  panel.className = "environment-node-detail-panel environment-node-runtime-panel";
  const head = document.createElement("div");
  head.className = "dashboard-section-head";
  head.innerHTML = "<h2>运行态索引</h2><p>来自 runtime SQLite 的 service_runtime 表</p>";
  const list = document.createElement("dl");
  list.className = "environment-node-detail-list";
  appendDetails(list, environmentNodeRuntimeRows(item, runtime));
  panel.appendChild(head);
  panel.appendChild(list);
  return panel;
}

function renderEnvironmentNodeInterfaceLinks(item) {
  const panel = document.createElement("section");
  panel.className = "environment-node-detail-panel environment-node-interfaces-panel";
  const head = document.createElement("div");
  head.className = "dashboard-section-head";
  head.innerHTML = "<h2>接口节点</h2><p>从服务节点跳转到接口级测试用例模板</p>";
  const list = document.createElement("div");
  list.className = "environment-node-peer-list";
  const loading = document.createElement("p");
  loading.className = "dashboard-empty";
  loading.textContent = "loading";
  list.appendChild(loading);
  panel.appendChild(head);
  panel.appendChild(list);

  environmentNodeRequest(`/api/interface-nodes?serviceId=${encodeURIComponent(item.id)}`)
    .then((payload) => {
      list.innerHTML = "";
      (payload.items || []).forEach((node) => {
        const link = document.createElement("a");
        link.className = "environment-node-peer";
        link.href = node.href;
        const name = document.createElement("strong");
        name.textContent = node.displayName || node.id;
        const status = document.createElement("span");
        status.textContent = node.admissionStatus || "pending";
        link.appendChild(name);
        link.appendChild(status);
        list.appendChild(link);
      });
      if (!list.children.length) {
        const empty = document.createElement("p");
        empty.className = "dashboard-empty";
        empty.textContent = "当前服务还没有登记接口节点。";
        list.appendChild(empty);
      }
    })
    .catch((error) => {
      list.innerHTML = "";
      const empty = document.createElement("p");
      empty.className = "dashboard-empty";
      empty.textContent = `接口节点读取失败：${error.message}`;
      list.appendChild(empty);
    });
  return panel;
}

function renderEnvironmentNodeDetailTemplate(item, snapshot) {
  environmentNodeEl("environmentNodeTitle").textContent = item.name || item.id;
  environmentNodeEl("environmentNodeSummary").textContent = `${item.groupLabel || item.group || "环境节点"} · ${item.container || item.id}`;
  renderEnvironmentNodeStats(item);

  const detail = environmentNodeEl("environmentNodeDetail");
  detail.innerHTML = "";
  detail.classList.remove("environment-node-missing-grid");

  const primary = document.createElement("section");
  primary.className = "environment-node-detail-panel environment-node-detail-primary";
  const primaryHead = document.createElement("div");
  primaryHead.className = "dashboard-section-head";
  primaryHead.innerHTML = "<h2>运行证据</h2><p>来自 /api/dashboard 的当前快照</p>";
  const primaryList = document.createElement("dl");
  primaryList.className = "environment-node-detail-list";
  appendDetails(primaryList, [
    ["id", item.id],
    ["container", item.container],
    ["state", item.state],
    ["health", item.health],
    ["message", item.message],
    ["image", item.image],
    ["version", item.version],
  ]);
  primary.appendChild(primaryHead);
  primary.appendChild(primaryList);

  const runtimePanel = renderEnvironmentNodeRuntimeMetadata(item, snapshot);

  const side = document.createElement("aside");
  side.className = "environment-node-detail-panel environment-node-connection-panel";
  const sideHead = document.createElement("div");
  sideHead.className = "dashboard-section-head";
  sideHead.innerHTML = "<h2>连接入口</h2><p>只展示当前快照能证明的端口与页面</p>";
  const sideList = document.createElement("dl");
  sideList.className = "environment-node-detail-list";
  appendDetails(sideList, [
    ["service port", item.port ? `127.0.0.1:${item.port}` : "-"],
    ["management", item.managementPort ? `127.0.0.1:${item.managementPort}` : "-"],
    ["group", item.groupLabel || item.group],
  ]);
  const actions = document.createElement("div");
  actions.className = "environment-node-detail-actions";
  if (item.port) {
    const portLink = renderActionLink("打开服务端口", `http://127.0.0.1:${item.port}`);
    portLink.target = "_blank";
    portLink.rel = "noreferrer";
    actions.appendChild(portLink);
  }
  if (item.managementPort) {
    const managementLink = renderActionLink("打开管理端口", `http://127.0.0.1:${item.managementPort}`);
    managementLink.target = "_blank";
    managementLink.rel = "noreferrer";
    actions.appendChild(managementLink);
  }
  if (!actions.children.length) {
    const empty = document.createElement("p");
    empty.className = "dashboard-empty";
    empty.textContent = "当前快照没有可打开入口。";
    actions.appendChild(empty);
  }
  side.appendChild(sideHead);
  side.appendChild(sideList);
  side.appendChild(actions);

  detail.appendChild(primary);
  detail.appendChild(runtimePanel);
  detail.appendChild(side);
  detail.appendChild(renderEnvironmentNodeInterfaceLinks(item));
  detail.appendChild(renderEnvironmentNodePeers(item, snapshot));
  detail.appendChild(renderEnvironmentNodeSnapshotSummary(snapshot));
  detail.appendChild(renderEnvironmentNodeRawSnapshot(item));
}

function renderEnvironmentNodeMissing(nodeId, snapshot) {
  environmentNodeEl("environmentNodeTitle").textContent = "未找到环境节点";
  environmentNodeEl("environmentNodeSummary").textContent = nodeId ? `id=${nodeId}` : "缺少 id 参数";
  const target = environmentNodeEl("environmentNodeStats");
  target.innerHTML = "";
  target.appendChild(statBox("节点总数", String(flattenEnvironmentNodes(snapshot).length)));
  target.appendChild(statBox("状态", "missing"));
  const detail = environmentNodeEl("environmentNodeDetail");
  detail.innerHTML = "";
  detail.classList.add("environment-node-missing-grid");
  detail.appendChild(renderEnvironmentNodeMissingRequest(nodeId));
  detail.appendChild(renderEnvironmentNodeMissingCandidates(snapshot));
  detail.appendChild(renderEnvironmentNodeSnapshotSummary(snapshot));
  detail.appendChild(renderEnvironmentNodeMissingSnapshotIndex(snapshot));
}

async function loadEnvironmentNodeDetail() {
  setEnvironmentNodeStatus("refreshing...");
  const nodeId = requestedEnvironmentNodeId();
  const snapshot = await environmentNodeRequest("/api/dashboard");
  const item = flattenEnvironmentNodes(snapshot).find((candidate) => candidate.id === nodeId);
  if (!item) {
    renderEnvironmentNodeMissing(nodeId, snapshot);
    setEnvironmentNodeStatus("missing");
    return;
  }
  renderEnvironmentNodeDetailTemplate(item, snapshot);
  setEnvironmentNodeStatus("ready");
}

environmentNodeEl("refreshEnvironmentNodeBtn")?.addEventListener("click", () => {
  loadEnvironmentNodeDetail().catch((error) => setEnvironmentNodeStatus(`failed: ${error.message}`));
});

loadEnvironmentNodeDetail().catch((error) => setEnvironmentNodeStatus(`failed: ${error.message}`));
