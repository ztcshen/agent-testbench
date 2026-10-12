const { randomUUID } = require("node:crypto");
const { AssistantSettings } = require("./assistant-settings.cjs");

class AssistantController {
  constructor({ backend, dataDir, safeStorage, fork, workerPath, notify }) {
    this.backend = backend;
    this.settings = new AssistantSettings({ dataDir, safeStorage });
    this.fork = fork;
    this.workerPath = workerPath;
    this.notify = notify;
    this.running = new Map();
  }
  async call(path, options = {}) {
    const result = await this.backend.request({ path, ...options });
    if (!result.ok) throw new Error(result.error || "助手 Store 请求失败");
    return result.data;
  }
  async sessions(mapId) {
    return (
      await this.call(
        `/api/assistant/sessions?${new URLSearchParams({ mapId, ownerId: this.settings.value.ownerId })}`,
      )
    ).sessions;
  }
  async session(id) {
    const item = (
      await this.call(`/api/assistant/session?${new URLSearchParams({ id })}`)
    ).session;
    if (item.ownerId !== this.settings.value.ownerId)
      throw new Error("该助手会话属于其他桌面实例");
    if (item.status === "running" && !this.running.has(id)) {
      item.status = "interrupted";
      item.state.error = "上次分析已中断，可以继续提问或重新分析。";
      await this.persist(item);
    }
    return item;
  }
  async persist(item) {
    const result = await this.call(
      `/api/assistant/session?${new URLSearchParams({ id: item.id })}`,
      {
        method: "PUT",
        body: {
          revision: item.revision,
          title: item.title,
          status: item.status,
          state: item.state,
        },
      },
    );
    Object.assign(item, result.session);
    this.notify(item);
    return item;
  }
  spawn() {
    return this.fork(this.workerPath, [], {
      serviceName: "AgentTestBench Assistant",
      stdio: "pipe",
      env: {
        ...process.env,
        LANGSMITH_TRACING: "false",
        LANGCHAIN_TRACING_V2: "false",
        LANGSMITH_API_KEY: "",
        LANGCHAIN_API_KEY: "",
      },
    });
  }
  async probe() {
    const config = this.settings.internal();
    if (!config.model) throw new Error("请先填写模型 ID");
    return new Promise((resolve, reject) => {
      const worker = this.spawn();
      const timer = setTimeout(
        () => {
          worker.kill();
          reject(new Error("模型连接测试超时"));
        },
        (config.timeoutSeconds + 5) * 1000,
      );
      let settled = false;
      const finish = (error, value) => {
        if (settled) return;
        settled = true;
        clearTimeout(timer);
        worker.kill();
        error ? reject(error) : resolve(value);
      };
      worker.on("message", (message) => {
        if (message.type === "done") finish(null, message.value);
        if (message.type === "error")
          finish(new Error(this.clean(message.error, config.apiKey)));
      });
      worker.on("exit", (code) => {
        if (!settled) finish(new Error(`模型进程退出 (${code})`));
      });
      worker.postMessage({ type: "probe", config });
    });
  }
  clean(value, key) {
    if (typeof value === "string")
      return key ? value.split(key).join("[REDACTED]") : value;
    if (Array.isArray(value)) return value.map((item) => this.clean(item, key));
    if (value && typeof value === "object")
      return Object.fromEntries(
        Object.entries(value).map(([name, item]) => [
          name,
          this.clean(item, key),
        ]),
      );
    return value;
  }
  async start(input) {
    if (this.running.size || this.startPromise)
      throw new Error("已有分析正在进行，请等待或取消后再开始");
    const pending = this.begin(input);
    this.startPromise = pending;
    try {
      return await pending;
    } finally {
      this.startPromise = null;
    }
  }
  async begin(input) {
    const config = this.settings.internal();
    if (!config.model) throw new Error("请先配置 OpenAI 兼容模型");
    const prompt = String(input.prompt || "").trim();
    if (!prompt || prompt.length > 8000)
      throw new Error("问题应为 1–8000 字符");
    this.settings.persistOwner();
    const item = input.sessionId
      ? await this.session(input.sessionId)
      : (
          await this.call("/api/assistant/sessions", {
            method: "POST",
            body: {
              ownerId: this.settings.value.ownerId,
              mapId: input.mapId,
              title: prompt.slice(0, 80),
            },
          })
        ).session;
    if (item.mapId !== input.mapId) throw new Error("会话与所选图谱不一致");
    item.state = {
      ...item.state,
      focus: {
        nodeId: String(input.nodeId || ""),
        pathId: String(input.pathId || ""),
      },
      model: {
        id: config.model,
        provider: config.provider || "compatible",
        baseURL: config.baseURL,
      },
      events: [],
      result: null,
      receipts: [],
      error: "",
      messages: [
        ...(item.state.messages || []).slice(-24),
        { role: "user", content: prompt, id: randomUUID() },
      ],
    };
    item.status = "running";
    await this.persist(item);
    const worker = this.spawn();
    let queue = Promise.resolve();
    let settled = false;
    const active = { worker, item, cancelled: false };
    const enqueue = (action) => {
      queue = queue.then(action);
      queue.catch((error) => {
        worker.kill();
        finish("failed", { error: error.message });
      });
      return queue;
    };
    const finish = async (status, value = {}) => {
      if (settled) return;
      settled = true;
      clearTimeout(active.timer);
      worker.kill();
      await queue.catch(() => {});
      item.status = active.cancelled ? "cancelled" : status;
      if (status === "completed" && !active.cancelled) {
        item.state.result = this.clean(value.result, config.apiKey);
        item.state.receipts = this.clean(value.receipts, config.apiKey);
        item.state.messages.push({
          role: "assistant",
          content: item.state.result.summary,
          id: randomUUID(),
        });
      } else
        item.state.error = this.clean(
          value.error || "分析已取消",
          config.apiKey,
        );
      try {
        await this.persist(item);
      } catch (error) {
        item.status = "failed";
        item.state.result = null;
        item.state.error = error.message;
        this.notify(item);
      }
      this.running.delete(item.id);
      active.resolve?.();
    };
    active.finished = new Promise((resolve) => {
      active.resolve = resolve;
    });
    active.finish = finish;
    active.timer = setTimeout(
      () => finish("failed", { error: "分析达到总时限，请缩小路径范围后重试" }),
      Math.min(240000, config.timeoutSeconds * 1000 * 3),
    );
    this.running.set(item.id, active);
    worker.on("message", (message) => {
      if (settled) return;
      if (message.type === "event")
        enqueue(async () => {
          const event = this.clean(message.event, config.apiKey);
          item.state.events = [
            ...item.state.events,
            {
              ...event,
              at: new Date().toISOString(),
              seq: item.state.events.length + 1,
            },
          ].slice(-100);
          await this.persist(item);
        });
      if (message.type === "tool")
        enqueue(async () => {
          const response = await this.backend.request({
            path: "/api/assistant/tool",
            method: "POST",
            body: {
              sessionId: item.id,
              action: message.action,
              args: message.args,
            },
          });
          if (!settled)
            worker.postMessage({
              type: "tool_result",
              requestId: message.requestId,
              response: response.ok
                ? { ok: true, data: response.data.data }
                : { ok: false, error: response.error },
            });
        });
      if (message.type === "done") finish("completed", message.value);
      if (message.type === "error")
        finish(message.cancelled ? "cancelled" : "failed", {
          error: message.error,
        });
    });
    worker.on("exit", (code) => {
      if (!settled) finish("interrupted", { error: `助手进程退出 (${code})` });
    });
    worker.postMessage({
      type: "start",
      config,
      focus: item.state.focus,
      messages: item.state.messages,
    });
    return item;
  }
  async cancel(id) {
    const active = this.running.get(id);
    if (!active) return;
    active.cancelled = true;
    active.worker.postMessage({ type: "cancel" });
    await active.finish("cancelled");
    await active.finished;
  }
  async stop() {
    await this.startPromise?.catch(() => {});
    await Promise.all([...this.running.keys()].map((id) => this.cancel(id)));
  }
}

module.exports = { AssistantController };
