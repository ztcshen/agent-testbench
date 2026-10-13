import { execFileSync } from "node:child_process";
import {
  existsSync,
  mkdirSync,
  readFileSync,
  writeFileSync,
  renameSync,
  rmSync,
  chmodSync,
} from "node:fs";
import { join, dirname, resolve } from "node:path";
import { homedir } from "node:os";
import { createHash, randomUUID } from "node:crypto";
import { createRequire } from "node:module";
import { fileURLToPath } from "node:url";
const require = createRequire(import.meta.url);
const {
  applicationPath,
  bundleID,
  verifyBuild,
  artifactHashes,
} = require("./installation.cjs");
const lsregister =
  "/System/Library/Frameworks/CoreServices.framework/Frameworks/LaunchServices.framework/Support/lsregister";

function command(bin, args) {
  return execFileSync(bin, args, {
    encoding: "utf8",
    maxBuffer: 8 * 1024 * 1024,
  });
}
function inspectBundle(path) {
  try {
    return JSON.parse(
      command("plutil", [
        "-convert",
        "json",
        "-o",
        "-",
        join(path, "Contents", "Info.plist"),
      ]),
    );
  } catch {
    return null;
  }
}
function indexedCopies() {
  return command("mdfind", [`kMDItemCFBundleIdentifier == "${bundleID}"`])
    .trim()
    .split("\n")
    .filter(Boolean);
}
function processes() {
  return command("ps", ["-ww", "-axo", "pid=,ppid=,comm="])
    .split("\n")
    .flatMap((line) => {
      const match = line.match(/^\s*(\d+)\s+(\d+)\s+(.+)$/);
      return match
        ? [
            {
              pid: Number(match[1]),
              parent: Number(match[2]),
              executable: match[3],
            },
          ]
        : [];
    });
}
function assertNotRunning(path) {
  if (
    processes().some(
      (p) => p.executable === join(path, "Contents", "MacOS", "AgentTestBench"),
    )
  )
    throw new Error("AgentTestBench 正在运行，请保存编辑并退出后再安装");
}
function retireOrphans(path) {
  for (const item of processes())
    if (
      item.parent === 1 &&
      item.executable === join(path, "Contents", "Resources", "agent-testbench")
    )
      try {
        process.kill(item.pid, "SIGTERM");
      } catch (error) {
        if (error.code !== "ESRCH") throw error;
      }
}
function archiveBundle(path, directory) {
  mkdirSync(directory, { recursive: true, mode: 0o700 });
  let identity;
  try {
    identity = JSON.stringify(
      artifactHashes(join(path, "Contents", "Resources"), "darwin"),
    );
  } catch {
    identity = readFileSync(join(path, "Contents", "Resources", "app.asar"));
  }
  const digest = createHash("sha256").update(identity).digest("hex");
  const zip = join(
    directory,
    digest.slice(0, 16) + "." + randomUUID() + ".zip",
  );
  if (!existsSync(zip)) {
    const temporary = zip + ".partial";
    try {
      command("ditto", [
        "-c",
        "-k",
        "--sequesterRsrc",
        "--keepParent",
        path,
        temporary,
      ]);
      command("unzip", ["-tqq", temporary]);
      renameSync(temporary, zip);
      chmodSync(zip, 0o600);
    } finally {
      rmSync(temporary, { force: true });
    }
  } else command("unzip", ["-tqq", zip]);
  return zip;
}

export function installApplication(source, options = {}) {
  if (process.platform !== "darwin")
    throw new Error("desktop:install currently supports macOS");
  const destination = options.destination || applicationPath;
  const dataDir =
    options.dataDir ||
    join(homedir(), "Library", "Application Support", "AgentTestBench");
  const archives =
    options.archives || join(dataDir, "application-archives.noindex");
  const info = verifyBuild(join(source, "Contents", "Resources"));
  if (inspectBundle(source)?.CFBundleIdentifier !== bundleID)
    throw new Error("Unexpected application identity");
  const candidates = [
    ...new Set([destination, ...(options.copies || indexedCopies())]),
  ].filter(
    (path) =>
      existsSync(path) &&
      resolve(path) !== resolve(source) &&
      inspectBundle(path)?.CFBundleIdentifier === bundleID,
  );
  for (const path of candidates) assertNotRunning(path);
  const retired = [];
  for (const path of candidates) {
    retireOrphans(path);
    retired.push({ path, archive: archiveBundle(path, archives) });
  }
  const staging = join(
    dirname(destination),
    `.atb-install-${process.pid}.noindex`,
  );
  const previous = staging + ".previous";
  const receiptPath = join(dataDir, "desktop-installation.json");
  const receiptBefore = existsSync(receiptPath)
    ? readFileSync(receiptPath)
    : null;
  if (existsSync(staging) || existsSync(previous))
    throw new Error("Installation staging path is occupied");
  mkdirSync(dirname(destination), { recursive: true });
  try {
    command("ditto", [source, staging]);
    verifyBuild(join(staging, "Contents", "Resources"));
    if (existsSync(destination)) renameSync(destination, previous);
    renameSync(staging, destination);
    verifyBuild(join(destination, "Contents", "Resources"));
    mkdirSync(dataDir, { recursive: true, mode: 0o700 });
    writeFileSync(
      receiptPath,
      JSON.stringify(
        {
          applicationPath: destination,
          buildId: info.buildId,
          version: info.version,
          installedAt: new Date().toISOString(),
        },
        null,
        2,
      ),
      { mode: 0o600 },
    );
  } catch (error) {
    if (existsSync(previous)) {
      rmSync(destination, { recursive: true, force: true });
      renameSync(previous, destination);
    }
    if (receiptBefore)
      writeFileSync(receiptPath, receiptBefore, { mode: 0o600 });
    else if (existsSync(receiptPath)) rmSync(receiptPath);
    throw error;
  } finally {
    rmSync(staging, { recursive: true, force: true });
  }
  if (!options.skipRegistration) {
    for (const { path } of retired) command(lsregister, ["-u", path]);
    command(lsregister, ["-f", destination]);
  }
  rmSync(previous, { recursive: true, force: true });
  for (const { path } of retired)
    if (resolve(path) !== resolve(destination))
      rmSync(path, { recursive: true, force: true });
  return {
    applicationPath: destination,
    buildId: info.buildId,
    version: info.version,
    retired,
  };
}

if (
  process.argv[1] &&
  resolve(process.argv[1]) === fileURLToPath(import.meta.url)
) {
  const archive = process.argv
    .find((x) => x.startsWith("--archive="))
    ?.slice(10);
  if (!archive)
    throw new Error(
      "--archive=ZIP is required; build with npm run desktop:package",
    );
  const { mkdtempSync } = require("node:fs");
  const temporary = mkdtempSync(
    join(require("node:os").tmpdir(), "atb-install-"),
  );
  try {
    command("ditto", ["-x", "-k", archive, temporary]);
    console.log(
      JSON.stringify(
        installApplication(join(temporary, "AgentTestBench.app")),
        null,
        2,
      ),
    );
  } finally {
    rmSync(temporary, { recursive: true, force: true });
  }
}
