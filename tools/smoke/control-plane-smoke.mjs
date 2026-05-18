import { spawn, spawnSync } from "node:child_process";
import { mkdir, mkdtemp, rm, writeFile } from "node:fs/promises";
import net from "node:net";
import os from "node:os";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { chromium } from "playwright";

const rootDir = path.resolve(fileURLToPath(new URL("../..", import.meta.url)));

function run(command, args) {
  const result = spawnSync(command, args, { cwd: rootDir, encoding: "utf8", stdio: "pipe" });
  if (result.status !== 0) {
    throw new Error(`${command} ${args.join(" ")} failed\n${result.stdout}\n${result.stderr}`);
  }
}

async function freePort() {
  return new Promise((resolve, reject) => {
    const server = net.createServer();
    server.once("error", reject);
    server.listen(0, "127.0.0.1", () => {
      const address = server.address();
      server.close(() => resolve(address.port));
    });
  });
}

async function waitForJSON(url, timeoutMs = 30000) {
  const deadline = Date.now() + timeoutMs;
  let lastError;
  while (Date.now() < deadline) {
    try {
      const response = await fetch(url, { headers: { Accept: "application/json" } });
      if (response.ok) return response.json();
      lastError = new Error(`${url} returned ${response.status}`);
    } catch (error) {
      lastError = error;
    }
    await new Promise((resolve) => setTimeout(resolve, 250));
  }
  throw lastError || new Error(`timed out waiting for ${url}`);
}

async function postJSON(url, body) {
  const response = await fetch(url, {
    method: "POST",
    headers: { "content-type": "application/json", Accept: "application/json" },
    body: JSON.stringify(body),
  });
  const payload = await response.json();
  if (!response.ok) {
    throw new Error(`${url} returned ${response.status}: ${JSON.stringify(payload)}`);
  }
  return payload;
}

async function writeSmokeProfile(baseDir) {
  const profileDir = path.join(baseDir, "profile");
  await mkdir(profileDir, { recursive: true });
  const profile = {
    id: "smoke",
    displayName: "Smoke Profile",
    description: "Generic profile for local browser smoke checks.",
    services: [{ id: "service.alpha", displayName: "Service Alpha", kind: "http" }],
    workflows: [{ id: "workflow.alpha", displayName: "Workflow Alpha", description: "Checks a generic item flow." }],
    interfaceNodes: [{ id: "node.alpha", displayName: "Node Alpha", serviceId: "service.alpha" }],
    apiCases: [{ id: "case.alpha", displayName: "Case Alpha", nodeId: "node.alpha" }],
    requestTemplates: [
      {
        id: "template.alpha",
        displayName: "Template Alpha",
        nodeId: "node.alpha",
        method: "GET",
        path: "/v1/items",
        templateJson: JSON.stringify({ method: "GET", path: "/v1/items" }),
      },
    ],
    caseDependencies: [{ id: "dependency.alpha", caseId: "case.alpha", fixtureId: "fixture.alpha", mappingsJson: "[]" }],
    workflowBindings: [{ workflowId: "workflow.alpha", stepId: "step.alpha", nodeId: "node.alpha", caseId: "case.alpha", required: true }],
    fixtures: [{ id: "fixture.alpha", displayName: "Fixture Alpha", kind: "json", dataJson: "{}" }],
    templateConfigs: [
      {
        id: "cfg.workflow-directory.default",
        templateId: "TPL-WORKFLOW-DIRECTORY-V1",
        scopeType: "workflow-directory",
        scopeId: "_default",
        configJson: JSON.stringify({
          workflowFinder: {
            targetStepCount: 1,
            targetInterfaceCount: 1,
            targetLabel: "Configured workflow target",
          },
        }),
        status: "active",
      },
    ],
  };
  await writeFile(path.join(profileDir, "profile.json"), JSON.stringify(profile, null, 2));
  return profileDir;
}

async function checkPage(browser, baseURL, pageSpec) {
  const page = await browser.newPage();
  const errors = [];
  page.on("console", (message) => {
    if (message.type() === "error") errors.push(message.text());
  });
  page.on("pageerror", (error) => errors.push(error.message));

  try {
    const response = await page.goto(baseURL + pageSpec.path, { waitUntil: "networkidle" });
    if (!response?.ok()) {
      throw new Error(`${pageSpec.path} returned ${response?.status()}`);
    }
    await page.waitForSelector(pageSpec.root);
    const text = (await page.locator(pageSpec.root).innerText()).trim();
    if (text.length < 20) {
      throw new Error(`${pageSpec.path} rendered too little text: ${JSON.stringify(text)}`);
    }
    for (const presentText of pageSpec.presentText || []) {
      if (!text.includes(presentText)) {
        throw new Error(`${pageSpec.path} missing expected text: ${presentText}`);
      }
    }
    for (const absentText of pageSpec.absentText || []) {
      if (text.includes(absentText)) {
        throw new Error(`${pageSpec.path} still renders removed text: ${absentText}`);
      }
    }
    for (const absentHref of pageSpec.absentHrefs || []) {
      const count = await page.locator(`a[href*="${absentHref}"]`).count();
      if (count > 0) {
        throw new Error(`${pageSpec.path} still links to removed href: ${absentHref}`);
      }
    }
    for (const presentHref of pageSpec.presentHrefs || []) {
      const count = await page.locator(`a[href="${presentHref}"]`).count();
      if (count === 0) {
        throw new Error(`${pageSpec.path} missing expected href: ${presentHref}`);
      }
    }
    if (errors.length > 0) {
      throw new Error(`${pageSpec.path} browser errors:\n${errors.join("\n")}`);
    }
  } finally {
    await page.close();
  }
}

async function checkEvidenceViewerTimeline(browser, baseURL) {
  const page = await browser.newPage();
  const errors = [];
  page.on("console", (message) => {
    if (message.type() === "error") errors.push(message.text());
  });
  page.on("pageerror", (error) => errors.push(error.message));

  try {
    const storageKey = "open-test-sandbox-evidence:smoke-timeline";
    const payload = {
      step: {
        title: "Case Alpha Evidence",
        goal: "POST /items",
        stageTitle: "API Case",
        caseId: "case.alpha",
        path: "service.alpha",
        correlators: ["req-1"],
        systems: [
          {
            id: "service.alpha",
            name: "Service Alpha",
            found: true,
            coreLogs: ["2026-05-18T01:00:00Z request_id=req-1 create item", "2026-05-18T01:00:01Z response 500"],
          },
        ],
        topology: { status: "partial", requestId: "req-1", traceId: "trace-1" },
      },
      caseDiagnostics: {
        summary: { case_id: "case.alpha", operation: "POST /items", evidence_path: ".runtime/evidence/smoke-timeline" },
        request: { method: "POST", path: "/items", request_id: "req-1" },
        response: { http_code: 500, request_id: "req-1" },
        assertions: { status: "failed", passed: false, http_status_ok: false, failure_reason: "unexpected status" },
        fixture: { status: "configured", applyRuns: [{ status: "applied", fixtureInstanceId: "fixture-1" }], dependencies: [{ id: "dependency.alpha" }], summary: { applyCount: 1, dependencyCount: 1 } },
        topology: { status: "partial", requestId: "req-1", traceId: "trace-1" },
        artifacts: [{ label: "case bundle", path: "/api/case/evidence?caseRun=run.alpha&caseId=case.alpha", kind: "json" }],
      },
    };
    await page.goto(`${baseURL}/index.html`, { waitUntil: "networkidle" });
    await page.evaluate(({ key, value }) => localStorage.setItem(key, JSON.stringify(value)), { key: storageKey, value: payload });
    const response = await page.goto(`${baseURL}/evidence-viewer.html?key=${encodeURIComponent(storageKey)}&workflow=workflow.alpha&caseId=case.alpha`, { waitUntil: "networkidle" });
    if (!response?.ok()) {
      throw new Error(`/evidence-viewer.html returned ${response?.status()}`);
    }
    await page.waitForSelector("#react-evidence-viewer-root");
    await page.getByText("Workflow case set").waitFor();
    const workflowContextLink = await page.locator('a[href="/api-cases.html?workflow=workflow.alpha&case=case.alpha"]').count();
    if (workflowContextLink === 0) {
      throw new Error("/evidence-viewer.html missing workflow case set handoff");
    }
    const caseRunsContextLink = await page.locator('a[href="/case-runs.html?case=case.alpha&workflow=workflow.alpha"]').count();
    if (caseRunsContextLink === 0) {
      throw new Error("/evidence-viewer.html missing workflow-scoped case run handoff");
    }
    await page.getByText("Evidence Timeline").waitFor();
    await page.getByText("Evidence Artifacts").waitFor();
    await page.getByText("Reproduction Command").waitFor();
    await page.locator(".viewer-reproduction-card pre", { hasText: "curl -i -X POST" }).waitFor();
    await page.locator(".viewer-artifact-item strong", { hasText: "case bundle" }).waitFor();
    await page.locator(".viewer-artifact-item code", { hasText: ".runtime/evidence/smoke-timeline" }).waitFor();
    await page.getByText("request 1").waitFor();
    await page.getByText("response 1").waitFor();
    await page.getByText("assertions 1").waitFor();
    await page.locator("button.detail-tab", { hasText: "logs 1" }).click();
    await page.locator(".viewer-timeline-detail h3", { hasText: "Service Alpha" }).waitFor();
    await page.getByPlaceholder("request / log / status").fill("response 500");
    await page.locator(".viewer-timeline-detail pre", { hasText: "response 500" }).waitFor();
    if (errors.length > 0) {
      throw new Error(`/evidence-viewer.html timeline browser errors:\n${errors.join("\n")}`);
    }
  } finally {
    await page.close();
  }
}

async function checkWorkbenchVerify(browser, baseURL, profileDir) {
  const page = await browser.newPage();
  const errors = [];
  page.on("console", (message) => {
    if (message.type() === "error") errors.push(message.text());
  });
  page.on("pageerror", (error) => errors.push(error.message));

  try {
    const response = await page.goto(`${baseURL}/index.html`, { waitUntil: "networkidle" });
    if (!response?.ok()) {
      throw new Error(`/index.html returned ${response?.status()}`);
    }
    await page.locator("input[type='text']").first().fill(profileDir);
    await page.getByRole("button", { name: "验收并发布" }).click();
    await page.getByText("all passed").waitFor();
    await page.getByText("profile-index").waitFor();
    await page.getByText("case runs optional").waitFor();
    await page.getByText("workflow runs optional").waitFor();
    await page.getByLabel("要求用例已通过").check();
    await page.getByRole("button", { name: "验收并发布" }).click();
    await page.getByText("1 failed").waitFor();
    await page.getByText("case runs required").waitFor();
    await page.getByText("api-case-run:case.alpha", { exact: true }).waitFor();
    const unexpectedErrors = errors.filter((item) => !item.includes("400 (Bad Request)"));
    if (unexpectedErrors.length > 0) {
      throw new Error(`/index.html verify action browser errors:\n${unexpectedErrors.join("\n")}`);
    }
  } finally {
    await page.close();
  }
}

async function checkWorkbenchInvalidInstalledProfile(browser, baseURL, profileHome) {
  const brokenDir = path.join(profileHome, "broken");
  await mkdir(brokenDir, { recursive: true });
  await writeFile(path.join(brokenDir, "profile.json"), `{"id":`);

  const page = await browser.newPage();
  const errors = [];
  page.on("console", (message) => {
    if (message.type() === "error") errors.push(message.text());
  });
  page.on("pageerror", (error) => errors.push(error.message));

  try {
    const response = await page.goto(`${baseURL}/index.html`, { waitUntil: "networkidle" });
    if (!response?.ok()) {
      throw new Error(`/index.html returned ${response?.status()}`);
    }
    await page.locator("select option").filter({ hasText: "broken · invalid" }).waitFor({ state: "attached" });
    const invalidOption = await page.locator("select").evaluate((select) => {
      const option = Array.from(select.options).find((item) => item.textContent.includes("broken · invalid"));
      return option ? { disabled: option.disabled, text: option.textContent } : null;
    });
    if (!invalidOption?.disabled) {
      throw new Error(`invalid installed profile option should be disabled: ${JSON.stringify(invalidOption)}`);
    }
    if (errors.length > 0) {
      throw new Error(`/index.html invalid profile browser errors:\n${errors.join("\n")}`);
    }
  } finally {
    await page.close();
  }
}

async function stopServer(server) {
  if (server.exitCode !== null || server.signalCode !== null) return;
  const closed = new Promise((resolve) => server.once("close", resolve));
  try {
    process.kill(-server.pid, "SIGTERM");
  } catch {
    server.kill("SIGTERM");
  }
  const stopped = await Promise.race([
    closed.then(() => true),
    new Promise((resolve) => setTimeout(() => resolve(false), 3000)),
  ]);
  if (!stopped) {
    try {
      process.kill(-server.pid, "SIGKILL");
    } catch {
      server.kill("SIGKILL");
    }
    await Promise.race([
      closed,
      new Promise((resolve) => setTimeout(resolve, 3000)),
    ]);
  }
}

async function main() {
  run("node", ["control-plane/frontend/build.mjs"]);

  const tempDir = await mkdtemp(path.join(os.tmpdir(), "otsandbox-smoke-"));
  const profileDir = await writeSmokeProfile(tempDir);
  const profileHome = path.join(tempDir, "profile-home");
  const storePath = path.join(tempDir, "store.sqlite");
  const port = await freePort();
  const baseURL = `http://127.0.0.1:${port}`;
  const server = spawn("go", ["run", "./cmd/otsandbox", "serve", "--profile", profileDir, "--profile-home", profileHome, "--store-url", storePath, "--host", "127.0.0.1", "--port", String(port)], {
    cwd: rootDir,
    detached: true,
    stdio: ["ignore", "pipe", "pipe"],
  });

  let output = "";
  server.stdout.on("data", (chunk) => {
    output += chunk;
  });
  server.stderr.on("data", (chunk) => {
    output += chunk;
  });

  try {
    const profile = await waitForJSON(`${baseURL}/api/profile`);
    if (profile.id !== "smoke") throw new Error(`unexpected profile payload: ${JSON.stringify(profile)}`);

    const imported = await postJSON(`${baseURL}/api/profile/import`, { path: profileDir });
    if (imported.profileId !== "smoke") throw new Error(`unexpected import payload: ${JSON.stringify(imported)}`);

    const index = await waitForJSON(`${baseURL}/api/profile/catalog-index`);
    if (index.profileId !== "smoke" || index.counts.workflows !== 1 || index.counts.templates !== 3 || index.counts.templateConfigs !== 3) {
      throw new Error(`unexpected catalog index: ${JSON.stringify(index)}`);
    }
    const catalog = await waitForJSON(`${baseURL}/api/catalog`);
    const finder = catalog.presentation?.workflowFinder;
    if (finder?.targetStepCount !== 1 || finder?.targetInterfaceCount !== 1 || finder?.targetLabel !== "Configured workflow target") {
      throw new Error(`unexpected workflow finder config: ${JSON.stringify(catalog.presentation)}`);
    }

    const browser = await chromium.launch({ headless: true });
    try {
      const pages = [
        { path: "/index.html", root: "#react-sandbox-workbench-root", presentText: ["Configured workflow target", "MATCHING WORKFLOW", "Workflow Alpha", "安装到本地", "要求用例已通过", "要求工作流已通过", "验收并发布"], absentText: ["Agent Test Kit"], absentHrefs: ["agent-test.html"] },
        { path: "/dashboard.html", root: "#react-dashboard-root" },
        { path: "/workflows.html", root: "#react-workflows-root", presentText: ["Configured workflow target", "WORKFLOW MAP", "STEP", "INTERFACE", "CASE", "ACTIONS", "Runs", "ready"], presentHrefs: ["/api-cases.html?workflow=workflow.alpha&case=case.alpha"] },
        { path: "/workflow-detail.html?id=workflow.alpha", root: "#react-workflow-detail-root" },
        { path: "/workflow-blueprint-demo.html?workflow=workflow.alpha", root: "#react-workflow-blueprint-demo-root" },
        { path: "/workflow-blueprint-new.html", root: "#react-workflow-blueprint-demo-root" },
        { path: "/api-cases.html", root: "#react-api-cases-root", presentText: ["API Case 工作台", "Coverage matrix", "Case Management Search", "Readiness groups"] },
        { path: "/api-cases.html?workflow=workflow.alpha", root: "#react-api-cases-root", presentText: ["WORKFLOW CASE SET", "Workflow Alpha", "1 steps", "1 interfaces", "1 cases", "Workflow case sequence", "Case Alpha", "service.alpha", "needs-review · not-run", "Runs"], presentHrefs: ["/interface-nodes.html?serviceId=service.alpha&workflow=workflow.alpha&case=case.alpha", "/case-runs.html?case=case.alpha&workflow=workflow.alpha"] },
        { path: "/interface-nodes.html?serviceId=service.alpha&workflow=workflow.alpha&case=case.alpha", root: "#react-interface-nodes-root", presentText: ["Workflow case set", "Node Alpha", "service.alpha"], presentHrefs: ["/interface-node.html?id=node.alpha&workflow=workflow.alpha&case=case.alpha", "/api-cases.html?workflow=workflow.alpha&case=case.alpha"] },
        { path: "/interface-node.html?id=node.alpha&workflow=workflow.alpha&case=case.alpha", root: "#react-interface-node-root", presentText: ["Workflow case set"], presentHrefs: ["/api-cases.html?workflow=workflow.alpha&case=case.alpha"] },
        { path: "/case-runs.html", root: "#react-case-runs-root", presentText: ["Run Analysis Center", "Case run report workbench", "Failure triage", "Report Grid"] },
        { path: "/case-runs.html?case=case.alpha", root: "#react-case-runs-root", presentText: ["Run Analysis Center", "case: case.alpha", "CASE EXECUTION SUMMARY", "0 runs", "Report Grid"] },
        { path: "/case-runs.html?workflow=workflow.alpha&case=case.alpha", root: "#react-case-runs-root", presentText: ["Run Analysis Center", "WORKFLOW CONTEXT", "workflow.alpha", "Workflow case set", "case: case.alpha", "CASE EXECUTION SUMMARY"], presentHrefs: ["/api-cases.html?workflow=workflow.alpha&case=case.alpha"] },
        { path: "/interface-nodes.html", root: "#react-interface-nodes-root" },
      ];
      for (const page of pages) {
        await checkPage(browser, baseURL, page);
      }
      await checkEvidenceViewerTimeline(browser, baseURL);
      await checkWorkbenchVerify(browser, baseURL, profileDir);
      await checkWorkbenchInvalidInstalledProfile(browser, baseURL, profileHome);
      const removedPage = await fetch(`${baseURL}/agent-test.html`);
      if (removedPage.status !== 404) {
        throw new Error(`/agent-test.html returned ${removedPage.status}, want 404`);
      }
    } finally {
      await browser.close();
    }
    console.log(`control-plane smoke passed on ${baseURL}`);
  } finally {
    await stopServer(server);
    await rm(tempDir, { recursive: true, force: true });
    if (server.exitCode !== 0 && server.exitCode !== null && !output.includes("Server closed")) {
      process.stderr.write(output);
    }
  }
}

main().catch((error) => {
  console.error(error);
  process.exit(1);
});
