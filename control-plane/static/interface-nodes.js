const interfaceNodeDirectoryState = {
  items: [],
  source: null,
};

const interfaceNodeDirectoryEl = (id) => document.getElementById(id);

function setInterfaceNodeDirectoryMessage(value) {
  interfaceNodeDirectoryEl("interfaceNodeDirectoryMessage").textContent = value;
}

async function interfaceNodeDirectoryRequest(path) {
  const response = await fetch(path, { headers: { Accept: "application/json" } });
  const body = await response.json().catch(() => ({}));
  if (!response.ok || body.ok === false) {
    throw new Error(body.error || response.statusText);
  }
  return body;
}

function interfaceNodeDirectoryText(value, fallback = "-") {
  const text = String(value ?? "").trim();
  return text || fallback;
}

function interfaceNodeDirectoryCounts(items) {
  return items.reduce((acc, item) => {
    acc.total += 1;
    const admission = item.admissionStatus || "pending";
    acc[admission] = (acc[admission] || 0) + 1;
    if (item.validationStatus === "invalid") acc.invalid += 1;
    if (item.serviceId) acc.services.add(item.serviceId);
    return acc;
  }, { total: 0, passed: 0, failed: 0, pending: 0, invalid: 0, services: new Set() });
}

function renderInterfaceNodeDirectoryStats(items) {
  const stats = interfaceNodeDirectoryEl("interfaceNodeDirectoryStats");
  if (!stats) return;
  const counts = interfaceNodeDirectoryCounts(items);
  const rows = [
    ["nodes", counts.total],
    ["services", counts.services.size],
    ["passed", counts.passed || 0],
    ["attention", (counts.failed || 0) + (counts.pending || 0) + counts.invalid],
  ];
  stats.innerHTML = "";
  rows.forEach(([label, value]) => {
    const card = document.createElement("div");
    card.className = "interface-node-directory-summary-card";
    const valueEl = document.createElement("strong");
    valueEl.textContent = value;
    const labelEl = document.createElement("span");
    labelEl.textContent = label;
    card.appendChild(valueEl);
    card.appendChild(labelEl);
    stats.appendChild(card);
  });
}

function renderInterfaceNodeDirectoryAttention(items) {
  const list = interfaceNodeDirectoryEl("interfaceNodeDirectoryAttentionList");
  if (!list) return;
  const attention = items
    .filter((item) => item.admissionStatus !== "passed" || item.validationStatus === "invalid")
    .slice(0, 8);
  list.innerHTML = "";
  if (!attention.length) {
    const empty = document.createElement("p");
    empty.className = "dashboard-empty compact";
    empty.textContent = "当前没有待处理接口。";
    list.appendChild(empty);
    return;
  }
  attention.forEach((item) => {
    const link = document.createElement("a");
    link.className = "interface-node-directory-attention-item";
    link.href = item.href || `/interface-node.html?id=${encodeURIComponent(item.id || "")}`;
    const title = document.createElement("strong");
    title.textContent = item.displayName || item.id || "接口节点";
    const meta = document.createElement("span");
    meta.textContent = [
      item.admissionStatus || "pending",
      item.validationStatus === "invalid" ? `${item.validationIssueCount ?? 0} validation` : "",
      item.serviceId,
    ].filter(Boolean).join(" · ");
    link.appendChild(title);
    link.appendChild(meta);
    list.appendChild(link);
  });
}

function renderInterfaceNodeServiceOptions(items) {
  const select = interfaceNodeDirectoryEl("interfaceNodeServiceFilter");
  const selected = select.value;
  select.innerHTML = "";
  const all = document.createElement("option");
  all.value = "";
  all.textContent = "全部服务";
  select.appendChild(all);
  [...new Set(items.map((item) => item.serviceId).filter(Boolean))].sort().forEach((serviceId) => {
    const option = document.createElement("option");
    option.value = serviceId;
    option.textContent = serviceId;
    option.selected = serviceId === selected;
    select.appendChild(option);
  });
}

function filteredInterfaceNodes() {
  const query = interfaceNodeDirectoryEl("interfaceNodeDirectoryFilter").value.trim().toLowerCase();
  const serviceID = interfaceNodeDirectoryEl("interfaceNodeServiceFilter").value;
  return interfaceNodeDirectoryState.items.filter((item) => {
    if (serviceID && item.serviceId !== serviceID) return false;
    if (!query) return true;
    const text = [
      item.id,
      item.displayName,
      item.serviceId,
      item.operation,
      item.method,
      item.path,
      item.status,
      item.admissionStatus,
      item.validationStatus,
    ].filter(Boolean).join(" ").toLowerCase();
    return text.includes(query);
  });
}

function interfaceNodeCard(item) {
  const card = document.createElement("a");
  card.className = "interface-node-directory-card";
  card.href = item.href || `/interface-node.html?id=${encodeURIComponent(item.id || "")}`;
  const top = document.createElement("div");
  top.className = "interface-node-directory-card-top";
  const title = document.createElement("strong");
  title.textContent = item.displayName || item.id || "接口节点";
  const status = document.createElement("span");
  status.className = `react-pill ${item.admissionStatus === "passed" ? "good" : item.admissionStatus === "failed" ? "bad" : "warn"}`;
  status.textContent = item.admissionStatus || "pending";
  top.appendChild(title);
  top.appendChild(status);

  const meta = document.createElement("code");
  meta.textContent = [item.id, item.serviceId, item.operation].filter(Boolean).join(" · ") || "-";
  const path = document.createElement("p");
  path.textContent = `${interfaceNodeDirectoryText(item.method)} ${interfaceNodeDirectoryText(item.path)}`;
  const details = document.createElement("div");
  details.className = "interface-node-directory-card-details";
  const cases = document.createElement("span");
  cases.textContent = `${item.passedCaseCount ?? 0}/${item.requiredCaseCount ?? 0} required cases`;
  const validation = document.createElement("span");
  validation.textContent = item.validationStatus === "invalid"
    ? `validation issues ${item.validationIssueCount ?? 0}`
    : "validation ok";
  details.appendChild(cases);
  details.appendChild(validation);

  card.appendChild(top);
  card.appendChild(meta);
  card.appendChild(path);
  card.appendChild(details);
  return card;
}

function renderInterfaceNodeDirectory() {
  const items = filteredInterfaceNodes();
  interfaceNodeDirectoryEl("interfaceNodeDirectorySummary").textContent =
    `${items.length}/${interfaceNodeDirectoryState.items.length} interface nodes`;
  const source = interfaceNodeDirectoryState.source || {};
  interfaceNodeDirectoryEl("interfaceNodeDirectorySource").textContent =
    `${source.kind || "sqlite"}${source.path ? ` · ${source.path}` : ""}`;
  renderInterfaceNodeDirectoryStats(interfaceNodeDirectoryState.items);
  renderInterfaceNodeDirectoryAttention(interfaceNodeDirectoryState.items);

  const list = interfaceNodeDirectoryEl("interfaceNodeDirectoryList");
  list.innerHTML = "";
  if (!items.length) {
    const empty = document.createElement("p");
    empty.className = "dashboard-empty";
    empty.textContent = "没有匹配的接口节点。";
    list.appendChild(empty);
    return;
  }
  items.forEach((item) => list.appendChild(interfaceNodeCard(item)));
}

async function refreshInterfaceNodeDirectory() {
  setInterfaceNodeDirectoryMessage("refreshing...");
  const payload = await interfaceNodeDirectoryRequest("/api/interface-nodes");
  interfaceNodeDirectoryState.items = payload.items || [];
  interfaceNodeDirectoryState.source = payload.source || {};
  renderInterfaceNodeServiceOptions(interfaceNodeDirectoryState.items);
  renderInterfaceNodeDirectory();
  setInterfaceNodeDirectoryMessage("ready");
}

interfaceNodeDirectoryEl("refreshInterfaceNodeDirectoryBtn")?.addEventListener("click", () => {
  refreshInterfaceNodeDirectory().catch((error) => setInterfaceNodeDirectoryMessage(error.message));
});
interfaceNodeDirectoryEl("interfaceNodeDirectoryFilter")?.addEventListener("input", renderInterfaceNodeDirectory);
interfaceNodeDirectoryEl("interfaceNodeServiceFilter")?.addEventListener("change", renderInterfaceNodeDirectory);

refreshInterfaceNodeDirectory().catch((error) => setInterfaceNodeDirectoryMessage(error.message));
