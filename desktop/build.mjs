import { build } from "esbuild";
import { mkdir, writeFile, copyFile, chmod, access } from "node:fs/promises";
import { spawn } from "node:child_process";
import { dirname, resolve, join } from "node:path";
import { fileURLToPath } from "node:url";

const desktop = dirname(fileURLToPath(import.meta.url));
const repo = resolve(desktop, "..");
const out = join(repo, "dist", "desktop");
await mkdir(join(out, "renderer"), { recursive: true });
await build({
  entryPoints: [join(desktop, "renderer", "entry.jsx")],
  outfile: join(out, "renderer", "app.js"),
  bundle: true,
  format: "iife",
  target: "chrome140",
  jsx: "automatic",
  minify: true,
  define: { "process.env.NODE_ENV": '"production"' },
  logLevel: "info",
});
await writeFile(
  join(out, "renderer", "index.html"),
  '<!doctype html><html lang="zh-CN"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><meta http-equiv="Content-Security-Policy" content="default-src \'self\'; script-src \'self\'; style-src \'self\' \'unsafe-inline\'; img-src \'self\' data:; connect-src \'none\'; font-src \'self\'; base-uri \'none\'; form-action \'none\'"><title>AgentTestBench</title><link rel="stylesheet" href="app.css"></head><body><div id="root"></div><script src="app.js"></script></body></html>',
);
if (process.argv.includes("--engine")) {
  await mkdir(join(out, "bin"), { recursive: true });
  const executable = join(
    out,
    "bin",
    process.platform === "win32" ? "agent-testbench.exe" : "agent-testbench",
  );
  const buildEnv = { ...process.env };
  if (process.platform === "darwin" && !buildEnv.DEVELOPER_DIR) {
    try {
      await access("/Library/Developer/CommandLineTools/usr/bin/clang");
      buildEnv.DEVELOPER_DIR = "/Library/Developer/CommandLineTools";
    } catch {}
  }
  await new Promise((yes, no) => {
    const child = spawn(
      "go",
      ["build", "-o", executable, "./cmd/agent-testbench"],
      { cwd: repo, stdio: "inherit", env: buildEnv },
    );
    child.once("error", no);
    child.once("exit", (code) =>
      code === 0 ? yes() : no(new Error(`Go build exited ${code}`)),
    );
  });
  if (process.platform !== "win32") await chmod(executable, 0o755);
}
if (process.argv.includes("--package")) {
  const { packager } = await import("@electron/packager");
  const staging = join(out, "app");
  await mkdir(staging, { recursive: true });
  await mkdir(join(staging, "renderer"), { recursive: true });
  for (const name of ["main.cjs", "preload.cjs", "backend.cjs", "trust.cjs"])
    await copyFile(join(desktop, name), join(staging, name));
  for (const name of ["index.html", "app.js", "app.css"])
    await copyFile(
      join(out, "renderer", name),
      join(staging, "renderer", name),
    );
  const pkg = JSON.parse(
    await import("node:fs").then((fs) =>
      fs.readFileSync(join(repo, "package.json"), "utf8"),
    ),
  );
  const checksums = JSON.parse(
    await import("node:fs").then((fs) =>
      fs.readFileSync(
        join(repo, "node_modules", "electron", "checksums.json"),
        "utf8",
      ),
    ),
  );
  await writeFile(
    join(staging, "package.json"),
    JSON.stringify({
      name: "agent-testbench-desktop",
      productName: "AgentTestBench",
      version: pkg.version,
      main: "main.cjs",
      description: pkg.description,
      author: "AgentTestBench contributors",
      license: pkg.license,
    }),
  );
  const icon =
    process.platform === "darwin"
      ? join(desktop, "assets", "icon.icns")
      : process.platform === "linux"
        ? join(desktop, "assets", "icon.png")
        : undefined;
  const packaged = await packager({
    dir: staging,
    out: join(repo, "dist", "desktop-release"),
    name: "AgentTestBench",
    platform: process.platform,
    arch: process.arch,
    electronVersion: pkg.devDependencies.electron,
    download: { checksums },
    appBundleId: "org.agenttestbench.desktop",
    appCategoryType: "public.app-category.developer-tools",
    icon,
    asar: true,
    overwrite: true,
    extraResource: [
      join(
        out,
        "bin",
        process.platform === "win32"
          ? "agent-testbench.exe"
          : "agent-testbench",
      ),
    ],
  });
  process.stdout.write(`Desktop application: ${packaged.join(", ")}\n`);
}
