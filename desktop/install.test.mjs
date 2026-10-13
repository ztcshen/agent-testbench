import test from "node:test";
import assert from "node:assert/strict";
import {
  mkdtempSync,
  mkdirSync,
  writeFileSync,
  readFileSync,
  existsSync,
  rmSync,
} from "node:fs";
import { join } from "node:path";
import { tmpdir } from "node:os";
import { createRequire } from "node:module";
import { execFileSync } from "node:child_process";
import { installApplication } from "./install.mjs";
const { createBuildInfo, bundleID } = createRequire(import.meta.url)(
  "./installation.cjs",
);

function bundle(directory, label, id = bundleID) {
  const path = join(directory, label + ".app");
  const resources = join(path, "Contents", "Resources");
  mkdirSync(resources, { recursive: true });
  writeFileSync(
    join(path, "Contents", "Info.plist"),
    JSON.stringify({
      CFBundleIdentifier: id,
      CFBundleShortVersionString: "1.0.0",
    }),
  );
  for (const name of ["app.asar", "agent-testbench", "assistant-worker.mjs"])
    writeFileSync(join(resources, name), label + name);
  writeFileSync(
    join(resources, "build-info.json"),
    JSON.stringify(
      createBuildInfo(resources, { platform: "darwin", version: "1.0.0" }),
    ),
  );
  return path;
}

test(
  "installation retires launchable copies with verified recoverable archives and preserves settings",
  { skip: process.platform !== "darwin" },
  (t) => {
    const root = mkdtempSync(join(tmpdir(), "atb-installer-"));
    t.after(() => rmSync(root, { recursive: true, force: true }));
    const source = bundle(root, "source"),
      destination = bundle(root, "installed"),
      duplicate = bundle(root, "previous"),
      other = bundle(root, "unrelated", "example.other");
    const dataDir = join(root, "data");
    mkdirSync(dataDir);
    writeFileSync(
      join(dataDir, "desktop-settings.json"),
      "settings remain byte-identical",
    );
    const before = readFileSync(
      join(destination, "Contents", "Resources", "agent-testbench"),
    );
    const result = installApplication(source, {
      destination,
      dataDir,
      archives: join(root, "archives.noindex"),
      copies: [duplicate, other],
      skipRegistration: true,
    });
    assert(!existsSync(duplicate));
    assert(existsSync(other));
    assert.equal(result.retired.length, 2);
    assert.equal(
      readFileSync(join(dataDir, "desktop-settings.json"), "utf8"),
      "settings remain byte-identical",
    );
    assert.equal(
      JSON.parse(readFileSync(join(dataDir, "desktop-installation.json")))
        .buildId,
      result.buildId,
    );
    const restore = join(root, "restore");
    mkdirSync(restore);
    execFileSync("ditto", [
      "-x",
      "-k",
      result.retired.find((x) => x.path === destination).archive,
      restore,
    ]);
    assert.deepEqual(
      readFileSync(
        join(
          restore,
          "installed.app",
          "Contents",
          "Resources",
          "agent-testbench",
        ),
      ),
      before,
    );
  },
);

test(
  "a failed installation restores the preceding application",
  { skip: process.platform !== "darwin" },
  (t) => {
    const root = mkdtempSync(join(tmpdir(), "atb-installer-failure-"));
    t.after(() => rmSync(root, { recursive: true, force: true }));
    const source = bundle(root, "source"),
      destination = bundle(root, "installed");
    const dataDir = join(root, "not-a-directory");
    writeFileSync(dataDir, "preserve");
    const before = readFileSync(
      join(destination, "Contents", "Resources", "agent-testbench"),
    );
    assert.throws(() =>
      installApplication(source, {
        destination,
        dataDir,
        archives: join(root, "archives.noindex"),
        copies: [],
        skipRegistration: true,
      }),
    );
    assert.deepEqual(
      readFileSync(
        join(destination, "Contents", "Resources", "agent-testbench"),
      ),
      before,
    );
    assert.equal(readFileSync(dataDir, "utf8"), "preserve");
  },
);
