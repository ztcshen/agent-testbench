import { spawn } from "node:child_process";
import { mkdtemp, rm } from "node:fs/promises";
import http from "node:http";
import os from "node:os";
import path from "node:path";
import { fileURLToPath } from "node:url";

const rootDir = path.resolve(fileURLToPath(new URL("../..", import.meta.url)));

function freeServer() {
  return new Promise((resolve, reject) => {
    const server = http.createServer(async (request, response) => {
      const body = await readBody(request);

      if (request.method === "POST" && request.url === "/v1/items") {
        response.writeHead(201, { "content-type": "application/json" });
        response.end(JSON.stringify({ status: "created", received: body ? JSON.parse(body) : null }));
        return;
      }

      response.writeHead(404, { "content-type": "application/json" });
      response.end(JSON.stringify({ error: "not found" }));
    });

    server.once("error", reject);
    server.listen(0, "127.0.0.1", () => {
      const address = server.address();
      resolve({ server, baseURL: `http://127.0.0.1:${address.port}` });
    });
  });
}

function readBody(request) {
  return new Promise((resolve, reject) => {
    let body = "";
    request.setEncoding("utf8");
    request.on("data", (chunk) => {
      body += chunk;
    });
    request.on("end", () => resolve(body));
    request.on("error", reject);
  });
}

async function closeServer(server) {
  await new Promise((resolve) => server.close(resolve));
}

function run(args) {
  return new Promise((resolve, reject) => {
    const child = spawn("./bin/otsandbox.sh", args, {
      cwd: rootDir,
      stdio: ["ignore", "pipe", "pipe"],
    });
    let stdout = "";
    let stderr = "";
    child.stdout.setEncoding("utf8");
    child.stderr.setEncoding("utf8");
    child.stdout.on("data", (chunk) => {
      stdout += chunk;
    });
    child.stderr.on("data", (chunk) => {
      stderr += chunk;
    });
    child.on("error", reject);
    child.on("close", (code) => {
      if (code !== 0) {
        reject(new Error(`${args.join(" ")} failed\n${stdout}\n${stderr}`));
        return;
      }
      resolve(stdout.trim());
    });
  });
}

async function main() {
  const tempDir = await mkdtemp(path.join(os.tmpdir(), "otsandbox-api-case-demo-"));
  const { server, baseURL } = await freeServer();

  try {
    const evidenceDir = path.join(tempDir, "evidence");
    const output = await run([
      "case",
      "run",
      "--case",
      "examples/api-cases/create-item.json",
      "--base-url",
      baseURL,
      "--run-id",
      "demo-create-item",
      "--evidence-dir",
      evidenceDir,
    ]);

    console.log(output);
    console.log(`Demo endpoint: ${baseURL}`);
    console.log(`Evidence bundle: ${path.join(evidenceDir, "demo-create-item")}`);
  } finally {
    await closeServer(server);
    if (process.env.OTSANDBOX_CLEAN_DEMO_OUTPUT === "1") {
      await rm(tempDir, { recursive: true, force: true });
    } else {
      console.log(`Demo output root: ${tempDir}`);
      console.log("Set OTSANDBOX_CLEAN_DEMO_OUTPUT=1 to remove demo output automatically.");
    }
  }
}

main().catch((error) => {
  console.error(error);
  process.exit(1);
});
