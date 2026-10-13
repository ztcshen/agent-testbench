const { spawn } = require("node:child_process");
const { createServer } = require("node:net");
const { join } = require("node:path");
const { mkdirSync, existsSync } = require("node:fs");
const { setTimeout: delay } = require("node:timers/promises");
const { backendURL, apiRequest } = require("./trust.cjs");

async function availablePort() {
  const server = createServer();
  await new Promise((resolve, reject) => {
    server.once("error", reject);
    server.listen(0, "127.0.0.1", resolve);
  });
  const port = server.address().port;
  await new Promise((resolve) => server.close(resolve));
  return port;
}

class Backend {
  constructor({ binary, dataDir }) {
    this.binary = binary;
    this.dataDir = dataDir;
    this.child = null;
    this.url = "";
    this.mode = "local";
    this.error = "";
    this.store = null;
  }

  async start(settings) {
    await this.stop();
    this.error = "";
    this.mode = settings.mode || "local";
    if (this.mode === "external") {
      this.url = backendURL(settings.backendURL);
    } else {
      if (!existsSync(this.binary))
        throw new Error("执行引擎尚未构建，请运行 npm run desktop:build");
      mkdirSync(this.dataDir, { recursive: true });
      const port = await availablePort();
      this.url = `http://127.0.0.1:${port}`;
      const storeRef =
        settings.store || `sqlite://${join(this.dataDir, "testbench.sqlite")}`;
      let output = "";
      this.child = spawn(
        this.binary,
        [
          "serve",
          "--host",
          "127.0.0.1",
          "--port",
          String(port),
          "--store",
          storeRef,
        ],
        {
          cwd: this.dataDir,
          windowsHide: true,
          stdio: ["pipe", "pipe", "pipe"],
          env: {
            ...process.env,
            AGENT_TESTBENCH_TRACE_GRAPHQL_URL: settings.traceURL || "",
            AGENT_TESTBENCH_PARENT_STDIN_CLOSE: "1",
          },
        },
      );
      this.child.stdout.on("data", (data) => {
        output = (output + data).slice(-4000);
      });
      this.child.stderr.on("data", (data) => {
        output = (output + data).slice(-4000);
      });
      this.child.once("error", (error) => {
        this.error = error.message;
      });
      this.child.once("exit", (code) => {
        this.error = `执行引擎已退出 (${code ?? "signal"})`;
      });
      const deadline = Date.now() + 20000;
      while (Date.now() < deadline) {
        if (this.error) throw new Error(this.error);
        try {
          const response = await fetch(`${this.url}/api/store/current`, {
            signal: AbortSignal.timeout(800),
          });
          if (response.ok) break;
        } catch {}
        await delay(100);
      }
      if (Date.now() >= deadline)
        throw new Error("本机执行引擎启动超时，请检查 Store 配置");
      const catalog = await this.request({ path: "/api/case-catalog" });
      if (
        catalog.status === 404 &&
        catalog.data?.error === "profile catalog was not found"
      ) {
        const seed = await this.request({
          path: "/api/sandbox/services",
          method: "POST",
          body: { id: "service.http", displayName: "HTTP API", kind: "http" },
        });
        if (!seed.ok) throw new Error(seed.error);
      }
    }
    const current = await this.request({ path: "/api/store/current" });
    if (!current.ok) throw new Error(current.error || "无法连接执行引擎");
    this.store = current.data;
    return this.status();
  }

  status() {
    return {
      mode: this.mode,
      backendURL: this.url,
      connected: Boolean(this.url && !this.error),
      error: this.error,
      store: this.store,
      pid: this.child?.pid || null,
    };
  }

  async request(input) {
    if (!this.url) return { ok: false, status: 503, error: "执行引擎尚未连接" };
    const request = apiRequest(input);
    const headers = { Accept: "application/json" };
    if (request.body !== undefined)
      headers["Content-Type"] = "application/json";
    if (request.etag) headers["If-Match"] = request.etag;
    try {
      const response = await fetch(this.url + request.path, {
        method: request.method,
        headers,
        body: request.body,
        redirect: "error",
        signal: AbortSignal.timeout(120000),
      });
      const raw = await response.text();
      if (raw.length > 24 * 1024 * 1024) throw new Error("服务响应超过 24 MiB");
      let data;
      try {
        data = JSON.parse(raw);
      } catch {
        throw new Error("执行引擎没有返回 JSON；请检查地址和版本");
      }
      return {
        ok: response.ok,
        status: response.status,
        etag: response.headers.get("etag") || "",
        data,
        error: response.ok ? "" : data.error || response.statusText,
      };
    } catch (error) {
      return { ok: false, status: 503, error: error.message };
    }
  }

  async stop() {
    const child = this.child;
    this.child = null;
    this.url = "";
    this.store = null;
    if (!child || child.exitCode !== null || child.signalCode) return;
    await new Promise((resolve) => {
      const graceful = setTimeout(() => child.kill("SIGTERM"), 1500);
      const timer = setTimeout(() => {
        child.kill("SIGKILL");
      }, 3000);
      child.once("exit", () => {
        clearTimeout(graceful);
        clearTimeout(timer);
        resolve();
      });
      child.stdin.end();
    });
  }
}

module.exports = { Backend };
