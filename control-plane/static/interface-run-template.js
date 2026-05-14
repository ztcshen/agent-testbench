(function () {
  function text(value, fallback = "-") {
    const out = String(value ?? "").trim();
    return out || fallback;
  }

  function prettyJSON(value) {
    if (value === undefined || value === null || value === "") {
      return "-";
    }
    if (typeof value === "string") {
      try {
        return JSON.stringify(JSON.parse(value), null, 2);
      } catch {
        return value;
      }
    }
    return JSON.stringify(value, null, 2);
  }

  function statusTone(value) {
    const status = String(value || "").toLowerCase();
    if (["pass", "passed", "success", "ok", "complete"].includes(status)) return "passed";
    if (["fail", "failed", "error", "partial"].includes(status)) return "failed";
    if (["running", "started"].includes(status)) return "running";
    return "warning";
  }

  function renderKV(label, value, tone = "") {
    const card = document.createElement("article");
    card.className = ["interface-run-kv", tone].filter(Boolean).join(" ");
    const key = document.createElement("span");
    key.textContent = label;
    const textEl = document.createElement("strong");
    textEl.textContent = text(value);
    card.appendChild(key);
    card.appendChild(textEl);
    return card;
  }

  function renderSummary(rows) {
    const grid = document.createElement("div");
    grid.className = "interface-run-summary";
    (rows || []).forEach(([label, value, tone]) => {
      grid.appendChild(renderKV(label, value, tone || ""));
    });
    return grid;
  }

  function renderJSONBlock(titleText, value) {
    const block = document.createElement("article");
    block.className = "interface-run-json-block";
    const title = document.createElement("strong");
    title.textContent = titleText;
    const pre = document.createElement("pre");
    pre.textContent = prettyJSON(value);
    block.appendChild(title);
    block.appendChild(pre);
    return block;
  }

  function renderRequestResponse({ request, response, emptyText = "这一次运行没有请求/响应明细。" } = {}) {
    const grid = document.createElement("div");
    grid.className = "interface-run-request-response";
    if (request === undefined && response === undefined) {
      const empty = document.createElement("div");
      empty.className = "empty-note";
      empty.textContent = emptyText;
      grid.appendChild(empty);
      return grid;
    }
    grid.appendChild(renderJSONBlock("request", request || {}));
    grid.appendChild(renderJSONBlock("response", response || {}));
    return grid;
  }

  window.InterfaceRunTemplate = {
    prettyJSON,
    renderJSONBlock,
    renderKV,
    renderRequestResponse,
    renderSummary,
    statusTone,
    text,
  };
})();
