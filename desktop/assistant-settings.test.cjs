const test = require("node:test");
const assert = require("node:assert/strict");
const { mkdtempSync, readFileSync, rmSync } = require("node:fs");
const { tmpdir } = require("node:os");
const { join } = require("node:path");
const {
  AssistantSettings,
  modelSettings,
} = require("./assistant-settings.cjs");

test("model credentials remain outside renderer payloads and plaintext files", () => {
  const dataDir = mkdtempSync(join(tmpdir(), "assistant-vault-"));
  const safeStorage = {
    isEncryptionAvailable: () => true,
    encryptString: (value) => Buffer.from(value.split("").reverse().join("")),
    decryptString: (value) => value.toString().split("").reverse().join(""),
  };
  try {
    const settings = new AssistantSettings({ dataDir, safeStorage, env: {} });
    const visible = settings.save({
      baseURL: "https://model.example/v1",
      model: "model-example",
      apiKey: "private-test-key",
    });
    assert.equal(visible.hasKey, true);
    assert(!JSON.stringify(visible).includes("private-test-key"));
    assert(!readFileSync(settings.path, "utf8").includes("private-test-key"));
    assert.equal(
      new AssistantSettings({ dataDir, safeStorage, env: {} }).internal()
        .apiKey,
      "private-test-key",
    );
    const temporary = new AssistantSettings({
      dataDir,
      safeStorage: { isEncryptionAvailable: () => false },
      env: {},
    });
    temporary.save({
      baseURL: "http://127.0.0.1/v1",
      model: "local",
      apiKey: "session-key",
    });
    assert(!readFileSync(temporary.path, "utf8").includes("session-key"));
    assert.equal(temporary.public().keyStorage, "session");
  } finally {
    rmSync(dataDir, { recursive: true, force: true });
  }
  assert.throws(() =>
    modelSettings({ baseURL: "https://model.example/v1?api_key=secret" }),
  );
  assert.throws(() => modelSettings({ baseURL: "file:///tmp/model" }));
});
