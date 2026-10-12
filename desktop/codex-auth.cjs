const { readFileSync } = require("node:fs");
const { join } = require("node:path");
const { homedir } = require("node:os");
const { execFileSync } = require("node:child_process");

function codexLogin(env = process.env) {
  const path = join(env.CODEX_HOME || join(homedir(), ".codex"), "auth.json");
  let login;
  try {
    login = JSON.parse(readFileSync(path, "utf8"));
  } catch {
    throw new Error(
      "没有可用的本机 Codex 登录，请先完成 Codex 的 ChatGPT 登录",
    );
  }
  const key = login.tokens?.access_token;
  if (!key) throw new Error("本机 Codex 尚未使用 ChatGPT 登录");
  let expiresAt;
  try {
    expiresAt = JSON.parse(
      Buffer.from(key.split(".")[1], "base64url").toString(),
    ).exp;
  } catch {}
  if (expiresAt && expiresAt * 1000 <= Date.now())
    throw new Error("本机 Codex 登录已过期，请在 Codex 中刷新登录后重试");
  return { apiKey: key, expiresAt };
}

function codexAvailable(env = process.env) {
  try {
    return Boolean(codexLogin(env).apiKey);
  } catch {
    return false;
  }
}

function systemModelProxy() {
  if (process.platform !== "darwin") return "";
  try {
    const value = execFileSync("scutil", ["--proxy"], {
      encoding: "utf8",
      timeout: 1000,
    });
    const enabled = /HTTPSEnable\s*:\s*1/.test(value);
    const host = value.match(/HTTPSProxy\s*:\s*(\S+)/)?.[1];
    const port = value.match(/HTTPSPort\s*:\s*(\d+)/)?.[1];
    return enabled && host && port ? `http://${host}:${port}` : "";
  } catch {
    return "";
  }
}

module.exports = { codexLogin, codexAvailable, systemModelProxy };
