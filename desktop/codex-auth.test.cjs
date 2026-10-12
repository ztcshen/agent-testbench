const test = require("node:test");
const assert = require("node:assert/strict");
const { mkdtempSync, writeFileSync, readFileSync, rmSync } = require("node:fs");
const { join } = require("node:path");
const { tmpdir } = require("node:os");
const { codexLogin, codexAvailable } = require("./codex-auth.cjs");
const { AssistantSettings } = require("./assistant-settings.cjs");
test("Codex login is read only, target-bound and absent from preferences/renderer", () => {
  const dir = mkdtempSync(join(tmpdir(), "testbench-codex-"));
  const path = join(dir, "auth.json");
  try {
    const token =
      "header." +
      Buffer.from(
        JSON.stringify({ exp: Math.floor(Date.now() / 1000) + 3600 }),
      ).toString("base64url") +
      ".signature";
    const raw = JSON.stringify({ tokens: { access_token: token } });
    writeFileSync(path, raw);
    const env = { CODEX_HOME: dir };
    assert.equal(codexLogin(env).apiKey, token);
    assert(codexAvailable(env));
    const settings = new AssistantSettings({
      dataDir: dir,
      safeStorage: { isEncryptionAvailable: () => false },
      env,
    });
    const visible = settings.save({
      provider: "codex",
      model: "gpt-5.6-sol",
      baseURL: "https://untrusted.example",
      proxyURL: "",
    });
    assert.equal(visible.baseURL, "https://chatgpt.com/backend-api");
    assert.equal(visible.keyStorage, "codex-login");
    assert(!JSON.stringify(visible).includes(token));
    assert(!readFileSync(settings.path, "utf8").includes(token));
    assert.equal(readFileSync(path, "utf8"), raw);
    writeFileSync(
      path,
      JSON.stringify({
        tokens: {
          access_token:
            "header." +
            Buffer.from(JSON.stringify({ exp: 1 })).toString("base64url") +
            ".signature",
        },
      }),
    );
    assert.throws(() => codexLogin(env), /过期/);
  } finally {
    rmSync(dir, { recursive: true, force: true });
  }
});
