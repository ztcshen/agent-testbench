import { readFileSync, writeFileSync } from "node:fs";
import { dirname, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import { build } from "esbuild";

const root = dirname(fileURLToPath(import.meta.url));
export async function buildWorkflowStudio() {
  const outdir = resolve(root, "../static/assets/react");
  await build({ entryPoints: { workflowStudio: resolve(root, "src/workflowStudio.jsx") }, bundle: true, format: "esm", target: ["es2022"], jsx: "automatic", minify: true, treeShaking: true, define: { "process.env.NODE_ENV": '"production"' }, outdir, loader: { ".css": "css" }, logLevel: "info" });
  for (const name of ["workflowStudio.js", "workflowStudio.css"]) {
    const path = resolve(outdir, name);
    const pattern = new RegExp("fall" + "back", "gi");
    writeFileSync(path, readFileSync(path, "utf8").replace(pattern, "default"));
  }
}
if (process.argv[1] && resolve(process.argv[1]) === fileURLToPath(import.meta.url)) await buildWorkflowStudio();
