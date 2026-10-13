import { _electron } from "playwright";
import { createRequire } from "node:module";
import { createServer } from "node:http";
import { mkdtemp, mkdir, writeFile, rm } from "node:fs/promises";
import { dirname, resolve, join } from "node:path";
import { fileURLToPath } from "node:url";
import { tmpdir } from "node:os";
import assert from "node:assert/strict";
import { verifyStepLibrary } from "./step-library-smoke.mjs";

const root = resolve(dirname(fileURLToPath(import.meta.url)), "..");
const require = createRequire(import.meta.url);
const option = (key) =>
  process.argv
    .find((value) => value.startsWith(`--${key}=`))
    ?.slice(key.length + 3);
const fixtureDir = await mkdtemp(join(tmpdir(), "agent-testbench-desktop-"));
const screenshots = option("screenshots");
if (screenshots) await mkdir(screenshots, { recursive: true });
const target = createServer((request, response) => {
  request.resume();
  request.on("end", () => {
    response.setHeader("Content-Type", "application/json");
    if (
      !["/items", "/items/item-42", "/items/item-42/status"].includes(
        request.url,
      )
    ) {
      response.writeHead(400);
      response.end(JSON.stringify({ error: "item id was not bound" }));
      return;
    }
    response.writeHead(request.method === "POST" ? 201 : 200);
    response.end(
      JSON.stringify({
        id: "item-42",
        name: "Desktop sample",
        status: "ready",
      }),
    );
  });
});
await new Promise((yes) => target.listen(0, "127.0.0.1", yes));
const targetURL = `http://127.0.0.1:${target.address().port}`;
const executable = option("app") || require("electron");
const args = option("app") ? [] : [join(root, "desktop", "main.cjs")];
const checks = [];
const summary = {
  checks,
  pageErrors: [],
  consoleErrors: [],
  packaged: Boolean(option("app")),
};
let application;
let page;
let childPID;
const check = (text) => {
  checks.push(text);
  process.stdout.write(`${text}\n`);
};

async function call(path, options = {}) {
  const value = await page.evaluate(
    async ({ path, options }) => window.testbench.request({ path, ...options }),
    { path, options },
  );
  assert(value.ok, `${options.method || "GET"} ${path}: ${value.error}`);
  return value;
}
async function screenshot(name) {
  if (!screenshots) return;
  if (await page.locator(".toast").isVisible())
    await page.locator(".toast").waitFor({ state: "hidden" });
  await page.screenshot({ path: join(screenshots, `${name}.png`) });
}

try {
  application = await _electron.launch({
    executablePath: executable,
    args: [...args, `--user-data=${join(fixtureDir, "user-data")}`],
    env: { ...process.env, AGENT_TESTBENCH_DESKTOP_HEADLESS: "1" },
    timeout: 45000,
  });
  page = await application.firstWindow();
  page.setDefaultTimeout(15000);
  page.on("pageerror", (error) => summary.pageErrors.push(error.message));
  page.on("console", (message) => {
    if (message.type() === "error") summary.consoleErrors.push(message.text());
  });
  await page.locator(".overview").waitFor({ timeout: 30000 });
  const connection = await page.evaluate(() => window.testbench.connection());
  assert(connection.connected && connection.mode === "local" && connection.pid);
  childPID = connection.pid;
  const isolation = await page.evaluate(() => ({
    node: typeof window.require,
    process: typeof window.process,
  }));
  assert.deepEqual(isolation, { node: "undefined", process: "undefined" });
  check("Electron boots its own engine and an isolated local Store");

  let etag = (await call("/api/request-editor")).etag;
  for (const [id, name, method, path, status] of [
    ["create", "Create item", "POST", "/items", 201],
    ["read", "Read item", "GET", "/items/{{override:itemId}}", 200],
    [
      "inspect",
      "Inspect item",
      "GET",
      "/items/{{override:itemId}}/status",
      200,
    ],
  ]) {
    etag = (
      await call("/api/request-editor", {
        method: "PUT",
        etag,
        body: {
          request: {
            id: `case.${id}`,
            nodeId: `node.${id}`,
            serviceId: "service.http",
            displayName: name,
            baseUrl: targetURL,
            execution: {
              method,
              path,
              body: method === "POST" ? { name: "Desktop sample" } : null,
              expectedHttpCodes: [status],
            },
          },
        },
      })
    ).etag;
  }
  const definition = {
    workflow: {
      id: "workflow.desktop",
      displayName: "Desktop sample",
      description: "Local desktop smoke",
    },
    steps: [
      {
        id: "create",
        title: "Create item",
        caseId: "case.create",
        required: true,
        position: { x: 80, y: 0 },
        bindings: [],
        outputs: [],
      },
      {
        id: "read",
        title: "Read item",
        caseId: "case.read",
        required: true,
        position: { x: 80, y: 190 },
        bindings: [
          {
            parameter: "itemId",
            sourceStepId: "create",
            from: "responsebody",
            path: "$.id",
          },
        ],
        outputs: [],
      },
      {
        id: "inspect",
        title: "Inspect item",
        caseId: "case.inspect",
        required: true,
        position: { x: 80, y: 380 },
        bindings: [
          {
            parameter: "itemId",
            sourceStepId: "read",
            from: "responsebody",
            path: "$.id",
          },
        ],
        outputs: [],
      },
    ],
  };
  await call("/api/workflow-studio", { method: "PUT", etag, body: definition });
  await page.reload();
  await page.getByRole("button", { name: "发送", exact: true }).click();
  await page.locator(".response-metrics .status.passed").waitFor();
  await screenshot("request");
  check("A Store request executes HTTP and displays its response");

  await page.locator(".sidebar-create").filter({ hasText: "新建接口" }).click();
  await page.getByLabel("接口名称", { exact: true }).fill("Desktop read");
  await page
    .getByLabel("请求地址", { exact: true })
    .fill(`${targetURL}/items/item-42`);
  await page.getByRole("button", { name: "保存", exact: true }).click();
  await page.locator(".unsaved-label").waitFor({ state: "hidden" });
  await page.getByText("已保存至 Store", { exact: true }).waitFor();
  await page.getByLabel("JSON 请求体").fill('{"probe":true}');
  await page.locator(".tree-item").filter({ hasText: "Create item" }).click();
  await page.locator(".tree-item").filter({ hasText: "Desktop read" }).click();
  assert.equal(
    await page.getByLabel("JSON 请求体").inputValue(),
    '{"probe":true}',
  );
  await page.getByRole("button", { name: "保存", exact: true }).click();
  await page.locator(".unsaved-label").waitFor({ state: "hidden" });
  await page.getByText("已保存至 Store", { exact: true }).waitFor();
  await page.reload();
  await page.locator(".tree-item").filter({ hasText: "Desktop read" }).click();
  assert.deepEqual(
    JSON.parse(await page.getByLabel("JSON 请求体").inputValue()),
    { probe: true },
  );
  check("Request creation, tab drafts and Store round trips work");

  await page.getByRole("button", { name: "自动化测试", exact: true }).click();
  await page
    .locator(".sequence-card")
    .filter({ has: page.locator("strong", { hasText: "Read item" }) })
    .click();
  assert.equal(
    await page.getByLabel("参数 itemId 的字段路径").inputValue(),
    "$.id",
  );
  await page.getByRole("button", { name: "运行场景", exact: true }).click();
  await page
    .locator(".scenario-inline-result .status.passed")
    .waitFor({ timeout: 30000 });
  const run = (await call("/api/catalog")).data.workflows[0].latestRun;
  summary.run = (
    await call(`/api/cases/batch-runs/${encodeURIComponent(run.id)}`)
  ).data;
  assert.equal(summary.run.status, "passed");
  assert.equal(summary.run.acceptance.ok, false);
  assert.equal(
    summary.run.steps?.length ||
      summary.run.cases?.length ||
      summary.run.summary?.total,
    3,
  );
  await screenshot("scenario");
  check("Three dependent steps pass, with acceptance kept separate");

  await verifyStepLibrary({ page, call, screenshot, check, summary });

  await page.getByRole("button", { name: "测试报告", exact: true }).click();
  await page.locator(".report-table-row").first().click();
  await page.locator(".evidence-page .code-view").waitFor();
  check("The desktop opens persisted reports and Evidence");

  await page
    .locator(".activity-rail")
    .getByRole("button", { name: "运行环境", exact: true })
    .click();
  await page.getByRole("button", { name: "新建环境", exact: true }).click();
  await page
    .getByLabel("环境名称", { exact: true })
    .fill("Desktop environment");
  await page.getByRole("button", { name: "保存环境", exact: true }).click();
  await page.getByText("环境已保存至 Store", { exact: true }).waitFor();
  const environment = (await call("/api/environments?all=true")).data.items[0];
  assert.equal(environment.verified, false);
  check("Environment registration remains unverified until actual acceptance");
  await page.getByRole("button", { name: "自动化测试", exact: true }).click();
  await page
    .getByRole("button", { name: "查看环境验收", exact: true })
    .waitFor();
  assert.equal(
    await page.getByRole("button", { name: "运行场景", exact: true }).count(),
    0,
  );
  check(
    "Environment verification scenes direct users to their acceptance entrypoint",
  );
  assert.equal(
    (await fetch(connection.backendURL + "/workflow-studio.html")).status,
    410,
  );
  assert.deepEqual(summary.pageErrors, []);
  assert.deepEqual(summary.consoleErrors, []);
  summary.ok = true;
} catch (error) {
  summary.ok = false;
  summary.error = error.stack;
  if (page && screenshots) {
    await page
      .screenshot({ path: join(screenshots, "failure.png") })
      .catch(() => {});
    await writeFile(
      join(screenshots, "failure.txt"),
      await page
        .locator("body")
        .innerText()
        .catch(() => ""),
    );
  }
  throw error;
} finally {
  if (application) {
    await application
      .evaluate(({ BrowserWindow }) => {
        for (const window of BrowserWindow.getAllWindows()) {
          window.webContents.removeAllListeners("will-prevent-unload");
          window.webContents.on("will-prevent-unload", (event) =>
            event.preventDefault(),
          );
        }
      })
      .catch(() => {});
    await application.close();
  }
  if (childPID) {
    const deadline = Date.now() + 5000;
    while (Date.now() < deadline) {
      try {
        process.kill(childPID, 0);
        await new Promise((yes) => setTimeout(yes, 100));
      } catch {
        break;
      }
    }
    try {
      process.kill(childPID, 0);
      summary.sidecarStopped = false;
    } catch {
      summary.sidecarStopped = true;
    }
  }
  await new Promise((yes) => target.close(yes));
  if (screenshots)
    await writeFile(
      join(screenshots, "check.json"),
      JSON.stringify(summary, null, 2),
    );
  await rm(fixtureDir, { recursive: true, force: true });
  if (childPID)
    assert.equal(
      summary.sidecarStopped,
      true,
      "The desktop left its owned engine running",
    );
}
