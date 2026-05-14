const el = (id) => document.getElementById(id);

function setEnvironmentMessage(value) {
  el("environmentMessage").textContent = value;
}

async function environmentRequest(path) {
  const response = await fetch(path);
  const body = await response.json();
  if (!response.ok) {
    throw new Error(body.error || response.statusText);
  }
  return body;
}

function statusText(item) {
  if (item.state === "missing") {
    return "未运行";
  }
  if (item.health && item.health !== "unknown") {
    return item.health;
  }
  return item.state || "unknown";
}

function environmentNodeDetailHref(item) {
  return `/environment-node.html?id=${encodeURIComponent(item.id)}`;
}

function renderEnvironmentNodeListTemplate(snapshot) {
  const summary = snapshot.summary || {};
  el("environmentSummary").textContent = `${summary.healthy || 0}/${summary.total || 0} healthy · ${summary.missing || 0} missing`;
  el("healthyCount").textContent = summary.healthy || 0;
  el("missingCount").textContent = summary.missing || 0;
  el("unhealthyCount").textContent = summary.unhealthy || 0;

  const business = (snapshot.groups || []).find((group) => group.id === "business");
  el("businessCount").textContent = business?.items?.length || 0;

  const grid = el("environmentGrid");
  grid.innerHTML = "";
  (snapshot.groups || []).forEach((group) => {
    const section = document.createElement("section");
    section.className = "dashboard-group";

    const head = document.createElement("div");
    head.className = "dashboard-group-head";
    const title = document.createElement("h2");
    title.textContent = group.label;
    const count = document.createElement("code");
    count.textContent = `${(group.items || []).filter((item) => item.ok).length}/${(group.items || []).length}`;
    head.appendChild(title);
    head.appendChild(count);
    section.appendChild(head);

    const list = document.createElement("div");
    list.className = "dashboard-service-list";
    (group.items || []).forEach((item) => {
      const card = document.createElement("a");
      card.className = `dashboard-card environment-node-card-button ${item.ok ? "ok" : item.state === "missing" ? "missing" : "bad"}`;
      card.href = environmentNodeDetailHref(item);
      card.setAttribute("aria-label", `查看 ${item.name} 服务详情`);

      const top = document.createElement("div");
      top.className = "dashboard-card-top";
      const name = document.createElement("strong");
      name.textContent = item.name;
      const status = document.createElement("span");
      status.textContent = statusText(item);
      top.appendChild(name);
      top.appendChild(status);

      const meta = document.createElement("div");
      meta.className = "dashboard-card-meta";
      const bits = [
        item.container,
        item.version ? `版本 ${item.version}` : "",
        item.port ? `:${item.port}` : "",
        item.managementPort ? `mgmt:${item.managementPort}` : "",
      ].filter(Boolean);
      meta.textContent = bits.join(" · ");

      const message = document.createElement("p");
      message.textContent = item.message || item.image || "-";

      const actions = document.createElement("div");
      actions.className = "dashboard-card-actions";
      const detailAction = document.createElement("span");
      detailAction.className = "button-link";
      detailAction.textContent = "查看详情";
      actions.appendChild(detailAction);

      card.appendChild(top);
      card.appendChild(meta);
      card.appendChild(message);
      card.appendChild(actions);
      list.appendChild(card);
    });
    section.appendChild(list);
    grid.appendChild(section);
  });
}

async function refreshEnvironmentNodes() {
  setEnvironmentMessage("refreshing...");
  const snapshot = await environmentRequest("/api/dashboard");
  renderEnvironmentNodeListTemplate(snapshot);
  setEnvironmentMessage("ready");
}

el("refreshEnvironmentBtn").addEventListener("click", () => refreshEnvironmentNodes().catch((error) => setEnvironmentMessage(error.message)));
refreshEnvironmentNodes().catch((error) => setEnvironmentMessage(error.message));
