import { ChatOpenAI } from "@langchain/openai";
import { CodexChatModel } from "./codex-model.mjs";

export function assistantModel(config) {
  if (config.provider === "codex") return new CodexChatModel(config);
  return new ChatOpenAI({
    model: config.model,
    apiKey: config.apiKey || "not-required",
    configuration: { baseURL: config.baseURL },
    temperature: 0,
    maxTokens: config.maxTokens || 1800,
    timeout: (config.timeoutSeconds || 45) * 1000,
    maxRetries: 1,
    useResponsesApi: false,
  });
}

export async function probeModel(config, signal) {
  const model = assistantModel(config);
  const response = await model.invoke(
    [
      {
        role: "user",
        content:
          "Reply with OK. This is a connection check; no project data is attached.",
      },
    ],
    { signal },
  );
  return {
    ok: true,
    model: config.model,
    text: String(response.content).slice(0, 200),
  };
}
