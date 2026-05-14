(function () {
  function svgEl(name, attributes = {}) {
    const element = document.createElementNS("http://www.w3.org/2000/svg", name);
    Object.entries(attributes).forEach(([key, value]) => element.setAttribute(key, String(value)));
    return element;
  }

  function shortLabel(value, maxLength = 24) {
    const text = String(value || "-");
    return text.length > maxLength ? `${text.slice(0, maxLength - 1)}...` : text;
  }

  function edges(topology = {}) {
    return [
      ...(topology.confirmedEdges || []).map((edge) => ({ ...edge, kind: "confirmed" })),
      ...(topology.externalExits || []).map((edge) => ({ ...edge, kind: "external" })),
      ...(topology.unresolvedExits || []).map((edge) => ({ ...edge, kind: "unresolved" })),
    ];
  }

  function nodes(topology = {}, topologyEdges = []) {
    const nodeSet = new Set(topology.observedNodes || []);
    topologyEdges.forEach((edge) => {
      if (edge.source) nodeSet.add(edge.source);
      if (edge.target) nodeSet.add(edge.target);
    });
    return [...nodeSet].filter(Boolean);
  }

  function ranks(nodesToRank, topologyEdges) {
    const rankMap = new Map(nodesToRank.map((node) => [node, 0]));
    for (let pass = 0; pass < nodesToRank.length; pass += 1) {
      let changed = false;
      topologyEdges.forEach((edge) => {
        if (!edge.source || !edge.target || !rankMap.has(edge.source) || !rankMap.has(edge.target)) return;
        const nextRank = (rankMap.get(edge.source) || 0) + 1;
        if (nextRank > (rankMap.get(edge.target) || 0)) {
          rankMap.set(edge.target, nextRank);
          changed = true;
        }
      });
      if (!changed) break;
    }
    const maxRank = Math.max(0, ...rankMap.values());
    nodesToRank.forEach((node, index) => {
      if (!Number.isFinite(rankMap.get(node))) rankMap.set(node, Math.min(index, maxRank));
    });
    return rankMap;
  }

  function renderDiagram(nodesToRender, topologyEdges, options = {}) {
    const wrap = document.createElement("div");
    wrap.className = "workflow-step-topology-diagram";
    if (!nodesToRender.length) {
      const empty = document.createElement("div");
      empty.className = "empty-note";
      empty.textContent = options.emptyText || "SkyWalking 返回了记录，但没有可绘制节点。";
      wrap.appendChild(empty);
      return wrap;
    }

    const rankMap = ranks(nodesToRender, topologyEdges);
    const byRank = new Map();
    nodesToRender.forEach((node) => {
      const rank = rankMap.get(node) || 0;
      if (!byRank.has(rank)) byRank.set(rank, []);
      byRank.get(rank).push(node);
    });
    byRank.forEach((rankNodes) => rankNodes.sort((left, right) => left.localeCompare(right)));

    const nodeWidth = 148;
    const nodeHeight = 46;
    const xGap = 205;
    const yGap = 82;
    const marginX = 42;
    const marginY = 36;
    const maxRank = Math.max(0, ...byRank.keys());
    const maxRows = Math.max(1, ...[...byRank.values()].map((rankNodes) => rankNodes.length));
    const width = Math.max(760, marginX * 2 + nodeWidth + maxRank * xGap);
    const height = Math.max(170, marginY * 2 + nodeHeight + (maxRows - 1) * yGap);
    const positions = new Map();
    byRank.forEach((rankNodes, rank) => {
      const columnHeight = (rankNodes.length - 1) * yGap;
      const startY = (height - columnHeight - nodeHeight) / 2;
      rankNodes.forEach((node, index) => {
        positions.set(node, {
          x: marginX + rank * xGap,
          y: startY + index * yGap,
        });
      });
    });

    const markerPrefix = options.markerPrefix || "skywalking-topology-arrow";
    const svg = svgEl("svg", {
      viewBox: `0 0 ${width} ${height}`,
      role: "img",
      "aria-label": "SkyWalking service topology graph",
    });
    const defs = svgEl("defs");
    ["confirmed", "external", "unresolved"].forEach((kind) => {
      const marker = svgEl("marker", {
        id: `${markerPrefix}-${kind}`,
        viewBox: "0 0 10 10",
        refX: "9",
        refY: "5",
        markerWidth: "8",
        markerHeight: "8",
        orient: "auto-start-reverse",
      });
      marker.appendChild(svgEl("path", { d: "M 0 0 L 10 5 L 0 10 z", class: `workflow-step-topology-arrow ${kind}` }));
      defs.appendChild(marker);
    });
    svg.appendChild(defs);

    topologyEdges.forEach((edge) => {
      const source = positions.get(edge.source);
      const target = positions.get(edge.target);
      if (!source || !target) return;
      const startX = source.x + nodeWidth;
      const startY = source.y + nodeHeight / 2;
      const endX = target.x;
      const endY = target.y + nodeHeight / 2;
      const control = Math.max(44, Math.abs(endX - startX) / 2);
      svg.appendChild(svgEl("path", {
        d: `M ${startX} ${startY} C ${startX + control} ${startY}, ${endX - control} ${endY}, ${endX} ${endY}`,
        class: `workflow-step-topology-path ${edge.kind}`,
        "marker-end": `url(#${markerPrefix}-${edge.kind})`,
      }));
      const label = svgEl("text", {
        x: (startX + endX) / 2,
        y: (startY + endY) / 2 - 8,
        class: `workflow-step-topology-path-label ${edge.kind}`,
        "text-anchor": "middle",
      });
      label.textContent = edge.component || edge.sourceComponent || edge.kind;
      svg.appendChild(label);
    });

    positions.forEach((position, node) => {
      const group = svgEl("g", { class: "workflow-step-topology-svg-node" });
      group.appendChild(svgEl("rect", {
        x: position.x,
        y: position.y,
        width: nodeWidth,
        height: nodeHeight,
        rx: "8",
      }));
      const title = svgEl("text", {
        x: position.x + 12,
        y: position.y + 20,
        class: "workflow-step-topology-svg-node-title",
      });
      title.textContent = shortLabel(node, 22);
      const meta = svgEl("text", {
        x: position.x + 12,
        y: position.y + 36,
        class: "workflow-step-topology-svg-node-meta",
      });
      meta.textContent = `rank ${rankMap.get(node) || 0}`;
      group.appendChild(title);
      group.appendChild(meta);
      svg.appendChild(group);
    });

    wrap.appendChild(svg);
    return wrap;
  }

  function renderEdgeList(topologyEdges, options = {}) {
    const list = document.createElement("div");
    list.className = "workflow-step-topology-edges";
    if (!topologyEdges.length) {
      const empty = document.createElement("div");
      empty.className = "empty-note";
      empty.textContent = options.emptyText || "SkyWalking 没有确认调用边；保留当前 trace 状态。";
      list.appendChild(empty);
      return list;
    }
    topologyEdges.forEach((edge) => {
      const item = document.createElement("article");
      item.className = `workflow-step-topology-edge ${edge.kind}`;
      const endpoint = edge.endpoint || edge.targetComponent || edge.component || "";
      item.innerHTML = `<strong>${edge.source || "-"}</strong><span>-></span><strong>${edge.target || "-"}</strong><code>${edge.kind}${endpoint ? ` · ${endpoint}` : ""}</code>`;
      list.appendChild(item);
    });
    return list;
  }

  window.SandboxTopologyRenderer = {
    edges,
    nodes,
    renderDiagram,
    renderEdgeList,
  };
})();
