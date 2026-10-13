const test = require("node:test");
const assert = require("node:assert/strict");
const { mkdtempSync, writeFileSync, rmSync } = require("node:fs");
const { join } = require("node:path");
const { tmpdir } = require("node:os");
const {
  createBuildInfo,
  verifyBuild,
  verifyReceipt,
  launchDecision,
  applicationPath,
} = require("./installation.cjs");

test("build identity detects a mixed engine or changed renderer bundle", (t) => {
  const dir = mkdtempSync(join(tmpdir(), "atb-build-"));
  t.after(() => rmSync(dir, { recursive: true, force: true }));
  for (const name of ["app.asar", "agent-testbench", "assistant-worker.mjs"])
    writeFileSync(join(dir, name), name);
  const info = createBuildInfo(dir, { platform: "darwin", version: "1.0.0" });
  writeFileSync(join(dir, "build-info.json"), JSON.stringify(info));
  assert.equal(verifyBuild(dir).buildId, info.buildId);
  for (const name of ["agent-testbench", "app.asar"]) {
    writeFileSync(join(dir, name), "older component");
    assert.throws(() => verifyBuild(dir), /版本不一致/);
    writeFileSync(join(dir, name), name);
  }
  writeFileSync(
    join(dir, "desktop-installation.json"),
    JSON.stringify({ applicationPath, buildId: info.buildId }),
  );
  verifyReceipt(dir, info);
  assert.throws(
    () => verifyReceipt(dir, { ...info, buildId: "0".repeat(64) }),
    /最新构建/,
  );
});

test("ordinary copies redirect to the canonical app while isolated tests remain usable", () => {
  assert.equal(
    launchDecision({
      platform: "darwin",
      executable: applicationPath + "/Contents/MacOS/AgentTestBench",
    }),
    "launch",
  );
  assert.equal(
    launchDecision({
      platform: "darwin",
      executable: "/temporary/older.app/Contents/MacOS/AgentTestBench",
    }),
    "redirect",
  );
  assert.equal(
    launchDecision({
      platform: "darwin",
      executable: "/source/Electron",
      isolatedTest: true,
    }),
    "launch",
  );
  assert.equal(
    launchDecision({
      platform: "darwin",
      executable: "/source/Electron",
      development: true,
    }),
    "launch",
  );
});
