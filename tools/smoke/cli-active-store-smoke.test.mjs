import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import test from "node:test";
import { fileURLToPath } from "node:url";

import { requiredSQLStoreDSN } from "./cli-active-store-smoke.mjs";

const smokeScript = readFileSync(fileURLToPath(new URL("./cli-active-store-smoke.mjs", import.meta.url)), "utf8");

test("active SQL Store CLI smoke accepts shared Store env", () => {
  const dsn = "MYSQL://user:secret@example.com:3306/agent_testbench_smoke?tls=false";

  assert.equal(requiredSQLStoreDSN({ AGENT_TESTBENCH_SMOKE_STORE: dsn }), dsn);
});

test("active SQL Store CLI smoke accepts SQLite Store env", () => {
  const dsn = "sqlite:///tmp/agent-testbench.sqlite";

  assert.equal(requiredSQLStoreDSN({ AGENT_TESTBENCH_SMOKE_STORE: dsn }), dsn);
});

test("active SQL Store CLI smoke documents every supported Store env", () => {
  assert.throws(
    () => requiredSQLStoreDSN({}),
    /AGENT_TESTBENCH_CLI_STORE_DSN, AGENT_TESTBENCH_SMOKE_STORE_DSN, or AGENT_TESTBENCH_SMOKE_STORE/,
  );
});

test("active SQL Store CLI smoke rejects unsupported shared Store env", () => {
  assert.throws(
    () => requiredSQLStoreDSN({ AGENT_TESTBENCH_SMOKE_STORE: "redis://127.0.0.1:6379/0" }),
    /requires a PostgreSQL, MySQL, or SQLite DSN/,
  );
});

test("active SQL Store CLI smoke rejects disabled SQLite Store env", () => {
  assert.throws(
    () => requiredSQLStoreDSN({ AGENT_TESTBENCH_SMOKE_STORE: "sqlite:///tmp/agent-testbench.sqlite", AGENT_TESTBENCH_DISABLE_SQLITE_STORE: "1" }),
    /cannot be combined/,
  );
});

test("active SQL Store CLI smoke refuses likely business MySQL databases", () => {
  assert.throws(
    () => requiredSQLStoreDSN({ AGENT_TESTBENCH_SMOKE_STORE: "mysql://user:secret@example.com:3306/business_prod?tls=false" }),
    /refuses database 'business_prod'/,
  );
});

test("active SQL Store CLI smoke uses current CLI surfaces", () => {
  assert.doesNotMatch(smokeScript, /\["config",\s*"publish"/);
  assert.doesNotMatch(smokeScript, /\["workflow",\s*"report"/);
  assert.doesNotMatch(smokeScript, /\["case",\s*"runs"/);
  assert.doesNotMatch(smokeScript, /\["case",\s*"evidence"/);
  assert.doesNotMatch(smokeScript, /\["evidence",\s*"tasks"/);
  assert.match(smokeScript, /\[\s*"map",\s*"import-workflows"/);
  assert.match(smokeScript, /\[\s*"map",\s*"run"/);
  assert.match(smokeScript, /\[\s*"case",\s*"inspect",\s*"--view",\s*"runs"/);
  assert.match(smokeScript, /\[\s*"case",\s*"inspect",\s*"--view",\s*"evidence"/);
  assert.match(smokeScript, /\[\s*"evidence",\s*"inspect",\s*"--view",\s*"tasks"/);
});
