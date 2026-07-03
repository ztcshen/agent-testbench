import assert from "node:assert/strict";
import { spawn, spawnSync } from "node:child_process";
import { mkdir, mkdtemp, rm } from "node:fs/promises";
import net from "node:net";
import os from "node:os";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { test } from "node:test";

const rootDir = path.resolve(fileURLToPath(new URL("../..", import.meta.url)));

function run(command, args, options = {}) {
  const result = spawnSync(command, args, {
    cwd: rootDir,
    encoding: "utf8",
    stdio: "pipe",
    ...options,
  });
  if (result.status !== 0) {
    throw new Error(`${command} ${args.join(" ")} failed\n${result.stdout}\n${result.stderr}`);
  }
  return result.stdout.trim();
}

function goHostTarget() {
  const raw = run("go", ["env", "GOOS", "GOARCH"]);
  const [goos, goarch] = raw.split(/\s+/).filter(Boolean);
  if (!goos || !goarch) {
    throw new Error(`could not resolve host Go target from: ${raw}`);
  }
  return { goos, goarch };
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

async function waitForText(url, timeoutMs = 30000) {
  const deadline = Date.now() + timeoutMs;
  let lastError;
  while (Date.now() < deadline) {
    try {
      const response = await fetch(url);
      const body = await response.text();
      if (response.ok) return body;
      lastError = new Error(`${url} returned ${response.status}: ${body.slice(0, 200)}`);
    } catch (error) {
      lastError = new Error(`fetch ${url} failed: ${error.message}`);
    }
    await new Promise((resolve) => setTimeout(resolve, 250));
  }
  throw lastError || new Error(`timed out waiting for ${url}`);
}

async function waitForJSON(url, timeoutMs = 30000) {
  const body = await waitForText(url, timeoutMs);
  return JSON.parse(body);
}

async function stopProcess(child) {
  if (child.exitCode !== null || child.signalCode !== null) return;
  child.kill("SIGTERM");
  await Promise.race([
    new Promise((resolve) => child.once("exit", resolve)),
    new Promise((resolve) => setTimeout(resolve, 3000)),
  ]);
  if (child.exitCode === null && child.signalCode === null) {
    child.kill("SIGKILL");
  }
}

test("release archive can serve the workbench outside a source checkout", async () => {
  const tempDir = await mkdtemp(path.join(os.tmpdir(), "agent-testbench-release-serve-"));
  const { goos, goarch } = goHostTarget();
  const version = "0.4.0-serve-smoke";
  const outputDir = path.join(tempDir, "dist");
  const extractDir = path.join(tempDir, "extract");
  const archive = path.join(outputDir, `agent-testbench_${version}_${goos}_${goarch}.tar.gz`);
  const releaseRoot = path.join(extractDir, `agent-testbench_${version}_${goos}_${goarch}`);
  const binary = path.join(releaseRoot, goos === "windows" ? "agent-testbench.exe" : "agent-testbench");
  const storePath = path.join(tempDir, "serve.sqlite");
  const port = await freePort();
  const serverURL = `http://127.0.0.1:${port}`;
  let child;

  try {
    run("bash", [
      "scripts/build-release.sh",
      "--version", version,
      "--revision", "release-archive-serve-smoke",
      "--output-dir", outputDir,
      "--target", `${goos}/${goarch}`,
    ]);
    await mkdir(extractDir, { recursive: true });
    run("tar", ["-xzf", archive, "-C", extractDir]);

    child = spawn(binary, [
      "serve",
      "--store", `sqlite://${storePath}`,
      "--host", "127.0.0.1",
      "--port", String(port),
    ], {
      cwd: releaseRoot,
      env: {
        ...process.env,
        AGENT_TESTBENCH_CONFIG_HOME: path.join(tempDir, "config"),
      },
      stdio: ["ignore", "pipe", "pipe"],
    });

    let output = "";
    child.stdout.on("data", (chunk) => { output += chunk; });
    child.stderr.on("data", (chunk) => { output += chunk; });
    child.once("exit", (code, signal) => {
      if (code !== null && code !== 0) output += `\nserve exited with code ${code}`;
      if (signal) output += `\nserve exited with signal ${signal}`;
    });

    const html = await waitForText(`${serverURL}/`);
    assert.match(html, /react-sandbox-workbench-root/);
    assert.match(await waitForText(`${serverURL}/assets/react/controlPlane.js`), /createRoot/);

    const currentStore = await waitForJSON(`${serverURL}/api/store/current`);
    assert.equal(currentStore.ok, true, output);
    assert.equal(currentStore.backend, "sqlite", output);
    assert.equal(currentStore.configured, true, output);
  } finally {
    if (child) await stopProcess(child);
    await rm(tempDir, { recursive: true, force: true });
  }
});
