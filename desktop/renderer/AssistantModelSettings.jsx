import { useState } from "react";
import { X, PlugZap, Save, LoaderCircle } from "lucide-react";
import { IconButton } from "./components.jsx";

export default function AssistantModelSettings({ value, saved, close }) {
  const [draft, setDraft] = useState({ ...value, apiKey: "" });
  const [busy, setBusy] = useState(false);
  const [message, setMessage] = useState("");
  const [error, setError] = useState("");
  const patch = (key, value) =>
    setDraft((current) => ({ ...current, [key]: value }));
  async function submit(probe = false) {
    setBusy(true);
    setError("");
    setMessage("");
    try {
      const config = await window.testbench.assistantSaveSettings(draft);
      saved(config);
      setDraft((current) => ({ ...current, apiKey: "" }));
      if (probe) {
        await window.testbench.assistantProbe();
        setMessage("模型连接成功。图谱分析还要求模型支持工具调用。");
      } else
        setMessage(
          config.keyStorage === "session"
            ? "配置已保存；系统加密不可用，密钥仅在本次应用会话中有效。"
            : "模型配置已保存。",
        );
    } catch (error) {
      setError(error.message);
    } finally {
      setBusy(false);
    }
  }
  return (
    <div className="assistant-model-backdrop">
      <section
        className="assistant-model-dialog"
        role="dialog"
        aria-modal="true"
        aria-label="OpenAI 兼容模型设置"
      >
        <header>
          <div>
            <strong>OpenAI 兼容模型</strong>
            <span>连接现有服务或本机模型</span>
          </div>
          <IconButton icon={X} title="关闭模型设置" onClick={close} />
        </header>
        <div className="assistant-model-fields">
          <label>
            调用方式
            <select
              aria-label="模型调用方式"
              value={draft.provider || "compatible"}
              onChange={(event) =>
                setDraft((current) => ({
                  ...current,
                  provider: event.target.value,
                  model:
                    event.target.value === "codex"
                      ? "gpt-5.6-sol"
                      : current.model,
                  baseURL:
                    event.target.value === "codex"
                      ? "https://chatgpt.com/backend-api"
                      : "http://127.0.0.1:11434/v1",
                  apiKey: "",
                }))
              }
            >
              <option value="compatible">OpenAI 兼容服务</option>
              <option value="codex">Codex · 本机 ChatGPT 登录</option>
            </select>
          </label>
          {draft.provider === "codex" && (
            <p>
              {value.codexAvailable
                ? "检测到本机 Codex 登录，可直接使用订阅模型。"
                : "请先在 Codex 中完成 ChatGPT 登录。"}
              应用只在调用时读取登录态，不复制或修改凭证。
            </p>
          )}
          <label>
            Base URL
            <input
              aria-label="模型 Base URL"
              value={draft.baseURL}
              disabled={draft.provider === "codex"}
              placeholder="https://your-provider.example/v1"
              onChange={(event) => patch("baseURL", event.target.value)}
            />
          </label>
          <label>
            模型 ID
            <input
              aria-label="模型 ID"
              value={draft.model}
              placeholder="服务商提供的模型 ID"
              onChange={(event) => patch("model", event.target.value)}
            />
          </label>
          {draft.provider !== "codex" && (
            <label>
              API 密钥
              <input
                aria-label="模型 API 密钥"
                type="password"
                autoComplete="off"
                value={draft.apiKey}
                placeholder={
                  value.hasKey
                    ? "已设置；留空保留现有密钥"
                    : "无认证的本机服务可留空"
                }
                onChange={(event) => patch("apiKey", event.target.value)}
              />
            </label>
          )}
          {draft.provider === "codex" && (
            <label>
              代理地址（可选）
              <input
                aria-label="模型代理地址"
                value={draft.proxyURL || ""}
                placeholder="留空使用系统 HTTPS 代理"
                onChange={(event) => patch("proxyURL", event.target.value)}
              />
            </label>
          )}
          <div className="assistant-model-limits">
            <label>
              请求超时（秒）
              <input
                aria-label="模型请求超时"
                type="number"
                min="10"
                max="120"
                value={draft.timeoutSeconds}
                onChange={(event) =>
                  patch("timeoutSeconds", Number(event.target.value))
                }
              />
            </label>
            <label>
              最大输出 Token
              <input
                aria-label="最大输出 Token"
                type="number"
                min="256"
                max="8000"
                value={draft.maxTokens}
                onChange={(event) =>
                  patch("maxTokens", Number(event.target.value))
                }
              />
            </label>
          </div>
          <p>
            模型会读取所选图谱及脱敏技术证据。密钥由桌面进程保管，系统支持时加密保存；助手记录保存在当前
            Store。
          </p>
          {error && <div className="notice error">{error}</div>}
          {message && <div className="notice">{message}</div>}
        </div>
        <footer>
          <button
            className="button"
            disabled={busy}
            onClick={() => submit(true)}
          >
            {busy ? (
              <LoaderCircle size={14} className="spin" />
            ) : (
              <PlugZap size={14} />
            )}
            测试连接
          </button>
          <button
            className="button primary"
            disabled={busy}
            onClick={() => submit()}
          >
            <Save size={14} />
            保存配置
          </button>
        </footer>
      </section>
    </div>
  );
}
