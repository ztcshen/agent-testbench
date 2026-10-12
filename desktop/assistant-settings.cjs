const { readFileSync, writeFileSync } = require("node:fs");
const { join } = require("node:path");
const { randomUUID } = require("node:crypto");
const {
  codexLogin,
  codexAvailable,
  systemModelProxy,
} = require("./codex-auth.cjs");

function modelSettings(input) {
  const provider = input.provider === "codex" ? "codex" : "compatible";
  const url = new URL(
    provider === "codex"
      ? "https://chatgpt.com/backend-api"
      : String(input.baseURL || "http://127.0.0.1:11434/v1").trim(),
  );
  if (
    !["http:", "https:"].includes(url.protocol) ||
    url.username ||
    url.password ||
    url.search ||
    url.hash
  )
    throw new Error("模型 Base URL 应为 HTTP/HTTPS 地址，不包含账号或查询参数");
  const model = String(input.model || "").trim();
  if (model.length > 200) throw new Error("模型 ID 过长");
  return {
    provider,
    baseURL: url.href.replace(/\/+$/, ""),
    model,
    proxyURL: modelProxyURL(input.proxyURL),
    timeoutSeconds: Math.max(
      10,
      Math.min(120, Number(input.timeoutSeconds) || 45),
    ),
    maxTokens: Math.max(256, Math.min(8000, Number(input.maxTokens) || 1800)),
  };
}

function modelProxyURL(value) {
  if (!value) return "";
  const url = new URL(String(value));
  if (
    !["http:", "https:"].includes(url.protocol) ||
    url.username ||
    url.password ||
    url.search ||
    url.hash ||
    !["", "/"].includes(url.pathname)
  )
    throw new Error("代理地址应为不含账号和路径的 HTTP/HTTPS 地址");
  return url.origin;
}

class AssistantSettings {
  constructor({ dataDir, safeStorage, env = process.env }) {
    this.path = join(dataDir, "assistant-settings.json");
    this.safeStorage = safeStorage;
    this.env = env;
    this.temporaryKey = "";
    this.value = {
      ...modelSettings({}),
      ownerId: randomUUID(),
      encryptedKey: "",
    };
    try {
      this.value = {
        ...this.value,
        ...JSON.parse(readFileSync(this.path, "utf8")),
      };
    } catch {}
  }
  key() {
    if (this.temporaryKey) return this.temporaryKey;
    if (this.env.AGENT_TESTBENCH_MODEL_API_KEY)
      return this.env.AGENT_TESTBENCH_MODEL_API_KEY;
    if (!this.value.encryptedKey) return "";
    if (!this.safeStorage.isEncryptionAvailable())
      throw new Error("当前系统无法读取加密密钥，请在模型设置重新输入");
    return this.safeStorage.decryptString(
      Buffer.from(this.value.encryptedKey, "base64"),
    );
  }
  internal() {
    const value = modelSettings({
      ...this.value,
      provider: this.env.AGENT_TESTBENCH_MODEL_PROVIDER || this.value.provider,
      baseURL: this.env.AGENT_TESTBENCH_MODEL_BASE_URL || this.value.baseURL,
      model: this.env.AGENT_TESTBENCH_MODEL_ID || this.value.model,
    });
    if (value.provider === "codex")
      return {
        ...value,
        proxyURL: value.proxyURL || systemModelProxy(),
        apiKey: codexLogin(this.env).apiKey,
      };
    return { ...value, apiKey: this.key() };
  }
  public() {
    const value = modelSettings({
      ...this.value,
      provider: this.env.AGENT_TESTBENCH_MODEL_PROVIDER || this.value.provider,
      baseURL: this.env.AGENT_TESTBENCH_MODEL_BASE_URL || this.value.baseURL,
      model: this.env.AGENT_TESTBENCH_MODEL_ID || this.value.model,
    });
    return {
      ...value,
      codexAvailable: codexAvailable(this.env),
      hasKey: Boolean(
        this.temporaryKey ||
        this.env.AGENT_TESTBENCH_MODEL_API_KEY ||
        this.value.encryptedKey,
      ),
      configured: Boolean(value.model),
      keyStorage:
        value.provider === "codex"
          ? "codex-login"
          : this.temporaryKey
            ? "session"
            : this.env.AGENT_TESTBENCH_MODEL_API_KEY
              ? "environment"
              : this.value.encryptedKey
                ? "encrypted"
                : "none",
    };
  }
  save(input) {
    const value = modelSettings(input);
    if (
      new URL(value.baseURL).origin !== new URL(this.value.baseURL).origin &&
      !input.apiKey
    ) {
      this.value.encryptedKey = "";
      this.temporaryKey = "";
    }
    this.value = { ...this.value, ...value };
    if (input.clearKey) {
      this.value.encryptedKey = "";
      this.temporaryKey = "";
    }
    if (
      value.provider !== "codex" &&
      typeof input.apiKey === "string" &&
      input.apiKey
    ) {
      if (input.apiKey.length > 8192) throw new Error("API 密钥过长");
      if (this.safeStorage.isEncryptionAvailable()) {
        this.value.encryptedKey = this.safeStorage
          .encryptString(input.apiKey)
          .toString("base64");
        this.temporaryKey = "";
      } else {
        this.temporaryKey = input.apiKey;
        this.value.encryptedKey = "";
      }
    }
    writeFileSync(this.path, JSON.stringify(this.value, null, 2), {
      mode: 0o600,
    });
    return this.public();
  }
  persistOwner() {
    writeFileSync(this.path, JSON.stringify(this.value, null, 2), {
      mode: 0o600,
    });
  }
}

module.exports = { AssistantSettings, modelSettings };
