const { readFileSync, existsSync } = require("node:fs");
const { createHash } = require("node:crypto");
const { join, resolve } = require("node:path");

const bundleID = "org.agenttestbench.desktop";
const applicationPath = "/Applications/AgentTestBench.app";
const manifestName = "build-info.json";

function artifactNames(platform) {
  return [
    "app.asar",
    platform === "win32" ? "agent-testbench.exe" : "agent-testbench",
    "assistant-worker.mjs",
  ];
}

function artifactHashes(resources, platform) {
  return Object.fromEntries(
    artifactNames(platform).map((name) => [
      name,
      createHash("sha256")
        .update(readFileSync(join(resources, name)))
        .digest("hex"),
    ]),
  );
}

function buildID(artifacts) {
  return createHash("sha256").update(JSON.stringify(artifacts)).digest("hex");
}

function createBuildInfo(resources, metadata) {
  const artifacts = artifactHashes(resources, metadata.platform);
  return {
    schemaVersion: 1,
    ...metadata,
    buildId: buildID(artifacts),
    artifacts,
  };
}

function verifyBuild(resources) {
  const info = JSON.parse(readFileSync(join(resources, manifestName), "utf8"));
  if (
    info.schemaVersion !== 1 ||
    !["darwin", "linux", "win32"].includes(info.platform) ||
    !info.version ||
    !/^[a-f0-9]{64}$/.test(info.buildId)
  )
    throw new Error("应用构建信息无效，请重新安装最新构建");
  const observed = artifactHashes(resources, info.platform);
  for (const name of artifactNames(info.platform))
    if (observed[name] !== info.artifacts?.[name])
      throw new Error(`应用组件版本不一致：${name}，请重新安装最新构建`);
  if (buildID(observed) !== info.buildId)
    throw new Error("应用构建标识不一致，请重新安装最新构建");
  return info;
}

function launchDecision({ platform, executable, isolatedTest, development }) {
  if (isolatedTest || development || platform !== "darwin") return "launch";
  return resolve(executable) ===
    join(applicationPath, "Contents", "MacOS", "AgentTestBench")
    ? "launch"
    : "redirect";
}

function verifyReceipt(dataDir, info) {
  const path = join(dataDir, "desktop-installation.json");
  if (!existsSync(path))
    throw new Error("缺少安装记录，请运行 npm run desktop:install");
  const receipt = JSON.parse(readFileSync(path, "utf8"));
  if (
    receipt.applicationPath !== applicationPath ||
    receipt.buildId !== info.buildId
  )
    throw new Error("当前应用不是已登记的最新构建，请重新安装");
}

module.exports = {
  bundleID,
  applicationPath,
  manifestName,
  artifactHashes,
  createBuildInfo,
  verifyBuild,
  verifyReceipt,
  launchDecision,
};
