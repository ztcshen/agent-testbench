import assert from "node:assert/strict";
import { spawnSync } from "node:child_process";
import { mkdtemp, readFile, rm } from "node:fs/promises";
import os from "node:os";
import path from "node:path";
import { test } from "node:test";
import { fileURLToPath } from "node:url";

const rootDir = path.resolve(fileURLToPath(new URL("../..", import.meta.url)));

function runGenerator(args = []) {
  return spawnSync("node", ["tools/docs/generate-cli-reference.mjs", ...args], {
    cwd: rootDir,
    encoding: "utf8",
    stdio: "pipe",
  });
}

test("CLI reference generator renders the current command catalog", async () => {
  const tempDir = await mkdtemp(path.join(os.tmpdir(), "agent-testbench-cli-reference-"));
  try {
    const output = path.join(tempDir, "cli-reference.md");
    const result = runGenerator(["--output", output]);

    assert.equal(result.status, 0, result.stderr || result.stdout);
    const markdown = await readFile(output, "utf8");

    assert.match(markdown, /# AgentTestBench CLI Reference/);
    assert.match(markdown, /Generated from `agent-testbench commands --all --json`/);
    assert.match(markdown, /Total commands: `124`/);
    assert.match(markdown, /## Default Commands/);
    assert.match(markdown, /## Command Areas/);
    assert.match(markdown, /### `map`/);
    assert.match(markdown, /\| `map inspect` \| yes \|/);
    assert.match(markdown, /\| `map run` \| yes \|/);
    assert.match(markdown, /\| `case inspect` \| yes \|/);
    assert.match(markdown, /\| `evidence inspect` \| yes \|/);
    assert.doesNotMatch(markdown, /Daily Agent Surface/);
    assert.doesNotMatch(markdown, /\bTier\b/);
    assert.doesNotMatch(markdown, /\bAudience\b/);
    assert.doesNotMatch(markdown, /\bStability\b/);
    assert.doesNotMatch(markdown, /\bdaily\b/);
    assert.doesNotMatch(markdown, /\badvanced\b/);
    assert.doesNotMatch(markdown, /config publish/);
    assert.doesNotMatch(markdown, /template-package catalog-index/);
    assert.doesNotMatch(markdown, /workflow report/);
    assert.doesNotMatch(markdown, /workflow latest-step/);
  } finally {
    await rm(tempDir, { recursive: true, force: true });
  }
});

test("committed CLI reference is in sync with the command catalog", () => {
  const result = runGenerator(["--check"]);

  assert.equal(result.status, 0, result.stderr || result.stdout);
  assert.match(result.stdout, /docs\/cli-reference\.md is up to date/);
});
