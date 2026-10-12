import { _electron } from "playwright";
import { createRequire } from "node:module";
import { mkdtemp, rm, mkdir, writeFile } from "node:fs/promises";
import { execFileSync } from "node:child_process";
import { tmpdir } from "node:os";
import { join, resolve, dirname } from "node:path";
import { fileURLToPath } from "node:url";
import assert from "node:assert/strict";
import { startAssistantFixture } from "./assistant/fixture-server.mjs";

const root = resolve(dirname(fileURLToPath(import.meta.url)), "..");
const require = createRequire(import.meta.url);
const option = (key) =>
  process.argv
    .find((item) => item.startsWith(`--${key}=`))
    ?.slice(key.length + 3);
const fixture = await mkdtemp(join(tmpdir(), "testbench-assistant-"));
const service = await startAssistantFixture();
const report = { checks: [], errors: [], packaged: Boolean(option("app")) };
let app, page, pid;
const userData = join(fixture, "desktop");
const executablePath = option("app") || require("electron");
const args = option("app") ? [] : [join(root, "desktop", "main.cjs")];
const env = {
  ...process.env,
  AGENT_TESTBENCH_DESKTOP_HEADLESS: "1",
  AGENT_TESTBENCH_MODEL_BASE_URL: service.baseURL,
  AGENT_TESTBENCH_MODEL_ID: "fixture-model",
  AGENT_TESTBENCH_MODEL_API_KEY: "fixture-key",
};
async function launch() {
  app = await _electron.launch({
    executablePath,
    args: [...args, `--user-data=${userData}`],
    env,
    timeout: 45000,
  });
  page = await app.firstWindow();
  page.setDefaultTimeout(30000);
  page.on("pageerror", (error) => report.errors.push(error.message));
}
async function call(path, options = {}) {
  const value = await page.evaluate(
    async ({ path, options }) => window.testbench.request({ path, ...options }),
    { path, options },
  );
  assert(value.ok, `${path}: ${value.error}`);
  return value;
}
function checked(value) {
  report.checks.push(value);
  process.stdout.write(`${value}\n`);
}
try {
  await launch();
  await page.locator(".overview").waitFor();
  const connection = await page.evaluate(() => window.testbench.connection());
  pid = connection.pid;
  let etag = (await call("/api/request-editor")).etag;
  etag = (
    await call("/api/request-editor", {
      method: "PUT",
      etag,
      body: {
        request: {
          id: "case.item",
          nodeId: "api.item",
          serviceId: "service.http",
          displayName: "Read item",
          baseUrl: "http://127.0.0.1",
          execution: {
            method: "GET",
            path: "/items",
            expectedHttpCodes: [200],
          },
        },
      },
    })
  ).etag;
  await call("/api/workflow-studio", {
    method: "PUT",
    etag,
    body: {
      workflow: { id: "flow.item", displayName: "Item lifecycle" },
      steps: [
        {
          id: "read",
          title: "Read item",
          caseId: "case.item",
          required: true,
          position: { x: 0, y: 0 },
          bindings: [],
          outputs: [],
        },
      ],
    },
  });
  const engine = option("app")
    ? resolve(dirname(option("app")), "..", "Resources", "agent-testbench")
    : join(root, "dist", "desktop", "bin", "agent-testbench");
  execFileSync(
    engine,
    [
      "map",
      "import-workflows",
      "--store",
      connection.store.url,
      "--map",
      "map.assistant.fixture",
      "--display-name",
      "Item lifecycle",
      "--json",
    ],
    { encoding: "utf8", stdio: ["ignore", "pipe", "pipe"] },
  );
  await page.reload();
  await page.locator(".map-workspace").waitFor();
  await page.getByRole("button", { name: "AI 图谱助手", exact: true }).click();
  await page.locator(".assistant-panel").waitFor();
  await page.getByRole("button", { name: "生成计划", exact: true }).click();
  await page
    .locator('.assistant-panel[data-assistant-status="completed"]')
    .waitFor();
  await page.locator(".assistant-result").waitFor();
  const sessions = await page.evaluate(() =>
    window.testbench.assistantSessions("map.assistant.fixture"),
  );
  assert.equal(sessions.length, 1);
  const id = sessions[0].id;
  const saved = await page.evaluate(
    (id) => window.testbench.assistantSession(id),
    id,
  );
  assert(saved.state.receipts.some((item) => item.action === "map.read"));
  assert(saved.state.receipts.some((item) => item.action === "plan.explain"));
  assert(!JSON.stringify(saved).includes("fixture-key"));
  assert.equal((await call("/api/case/runs")).data.caseRuns.length, 0);
  checked(
    "Actual LangChain utility process queries the Store and builds a Go plan without executing a case",
  );
  if (option("screenshots")) {
    await mkdir(option("screenshots"), { recursive: true });
    await page.screenshot({
      path: join(option("screenshots"), "TestBench-LangChain-助手.png"),
    });
  }
  await app.close();
  app = null;
  await launch();
  await page.locator(".map-workspace").waitFor();
  await page.getByRole("button", { name: "AI 图谱助手", exact: true }).click();
  await page.locator(".assistant-result").waitFor();
  assert.equal(await page.getByLabel("助手历史会话").inputValue(), id);
  checked(
    "Conversation, validated findings and tool receipts survive a full application restart",
  );
  await page.getByRole("button", { name: "模型设置", exact: true }).click();
  await page.getByRole("button", { name: "测试连接", exact: true }).click();
  await page
    .getByText("模型连接成功。图谱分析还要求模型支持工具调用。", {
      exact: true,
    })
    .waitFor();
  checked(
    "Model settings and connection probing use the isolated worker and omit credentials from the renderer",
  );
  await page.getByRole("button", { name: "关闭模型设置", exact: true }).click();
  service.setDelay(500);
  const concurrent = await page.evaluate(
    async ({ id }) => {
      const input = {
        mapId: "map.assistant.fixture",
        sessionId: id,
        prompt: "Generate a test plan",
      };
      const replies = await Promise.allSettled([
        window.testbench.assistantStart(input),
        window.testbench.assistantStart(input),
      ]);
      return replies.map((item) => ({
        status: item.status,
        value: item.status === "fulfilled" ? item.value : item.reason.message,
      }));
    },
    { id },
  );
  assert.equal(
    concurrent.filter((item) => item.status === "fulfilled").length,
    1,
  );
  await page.evaluate((id) => window.testbench.assistantCancel(id), id);
  const cancelled = await page.evaluate(
    (id) => window.testbench.assistantSession(id),
    id,
  );
  assert.equal(cancelled.status, "cancelled");
  assert.equal(cancelled.state.result, null);
  checked(
    "Concurrent starts are serialized and cancellation persists without manufacturing a result",
  );
  assert.deepEqual(report.errors, []);
  report.ok = true;
} catch (error) {
  report.ok = false;
  report.error = error.stack;
  if (page)
    report.body = (
      await page
        .locator("body")
        .innerText()
        .catch(() => "")
    ).slice(-7000);
  throw error;
} finally {
  if (app) await app.close();
  await service.close();
  if (pid) {
    try {
      process.kill(pid, 0);
      report.engineStopped = false;
    } catch {
      report.engineStopped = true;
    }
  }
  if (option("report"))
    await writeFile(option("report"), JSON.stringify(report, null, 2));
  await rm(fixture, { recursive: true, force: true });
}
