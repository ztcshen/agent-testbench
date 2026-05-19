import { spawnSync } from "node:child_process";
import { describe, it } from "node:test";
import assert from "node:assert/strict";
import path from "node:path";
import { fileURLToPath } from "node:url";

const rootDir = path.resolve(fileURLToPath(new URL("../..", import.meta.url)));

describe("control-plane smoke Store selection", () => {
  it("prepares a named PostgreSQL Store when a smoke DSN is provided", () => {
    const result = spawnSync(process.execPath, [
      "--input-type=module",
      "-e",
      [
        "import { prepareSmokeStoreReference } from './tools/smoke/control-plane-smoke.mjs';",
        "const calls = [];",
        "const ref = await prepareSmokeStoreReference('/tmp/ots-smoke', { OTSANDBOX_SMOKE_STORE_DSN: 'postgres://user:secret@example.com:5432/ots?sslmode=disable' }, (command, args, options) => calls.push({ command, args, env: options.env }));",
        "if (ref.storeRef !== 'smoke-postgres') throw new Error(JSON.stringify(ref));",
        "if (!ref.serverEnv.OTSANDBOX_CONFIG_HOME?.endsWith('/store-config')) throw new Error(JSON.stringify(ref));",
        "if (calls.length !== 3) throw new Error(JSON.stringify(calls));",
        "if (calls[0].args.join(' ') !== 'run ./cmd/otsandbox store config set smoke-postgres --url postgres://user:secret@example.com:5432/ots?sslmode=disable') throw new Error(JSON.stringify(calls));",
        "if (calls[1].args.join(' ') !== 'run ./cmd/otsandbox store use smoke-postgres') throw new Error(JSON.stringify(calls));",
        "if (calls[2].args.join(' ') !== 'run ./cmd/otsandbox store upgrade --store smoke-postgres') throw new Error(JSON.stringify(calls));",
      ].join("\n"),
    ], {
      cwd: rootDir,
      encoding: "utf8",
      env: { ...process.env, OTSANDBOX_SMOKE_IMPORT_ONLY: "1" },
    });
    assert.equal(result.status, 0, result.stderr || result.stdout);
  });

  it("requires a PostgreSQL DSN when SQLite Store is disabled", () => {
    const result = spawnSync(process.execPath, [
      "--input-type=module",
      "-e",
      [
        "import { prepareSmokeStoreReference } from './tools/smoke/control-plane-smoke.mjs';",
        "await prepareSmokeStoreReference('/tmp/ots-smoke', { OTSANDBOX_DISABLE_SQLITE_STORE: '1' }, () => {});",
      ].join("\n"),
    ], {
      cwd: rootDir,
      encoding: "utf8",
    });
    assert.notEqual(result.status, 0);
    assert.match(result.stderr, /OTSANDBOX_SMOKE_STORE_DSN/);
  });
});

describe("control-plane smoke Evidence assertions", () => {
  it("requires Store-backed request, response, assertion, and topology evidence for the workflow run case", () => {
    const result = spawnSync(process.execPath, [
      "--input-type=module",
      "-e",
      [
        "import { assertWorkflowCaseEvidence } from './tools/smoke/control-plane-smoke.mjs';",
        "assertWorkflowCaseEvidence({ ok: true, evidence: { summary: { case_id: 'case.alpha', case_run_id: 'run.case', run_id: 'run.workflow', step_id: 'step.alpha', status: 'passed' }, request: { method: 'GET', path: '/v1/items', evidence_uri: '/e/request.json' }, response: { http_code: 200, evidence_uri: '/e/response.json' }, assertions: { status: 'passed', passed: true }, topology: { provider: 'skywalking', status: 'complete', traceId: 'trace.smoke.1', confirmedEdges: [{}] } } }, { runID: 'run.workflow', caseID: 'case.alpha', stepID: 'step.alpha' });",
      ].join("\n"),
    ], {
      cwd: rootDir,
      encoding: "utf8",
    });
    assert.equal(result.status, 0, result.stderr || result.stdout);
  });
});
