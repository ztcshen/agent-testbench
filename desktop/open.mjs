import { execFileSync } from "node:child_process";
import { createRequire } from "node:module";
import { existsSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
const { applicationPath } = createRequire(import.meta.url)(
  "./installation.cjs",
);
if (process.platform === "darwin") {
  if (!existsSync(applicationPath))
    throw new Error("请先运行 npm run desktop:install");
  execFileSync("open", [applicationPath]);
} else {
  const desktop = dirname(fileURLToPath(import.meta.url));
  execFileSync(process.execPath, [join(desktop, "build.mjs"), "--engine"], {
    stdio: "inherit",
  });
  execFileSync(
    createRequire(import.meta.url)("electron"),
    [join(desktop, "main.cjs")],
    { stdio: "inherit" },
  );
}
