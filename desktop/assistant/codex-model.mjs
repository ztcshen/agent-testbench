import { BaseChatModel } from "@langchain/core/language_models/chat_models";
import { AIMessage } from "@langchain/core/messages";
import { convertToOpenAITool } from "@langchain/core/utils/function_calling";
import { stream as streamCodex } from "@earendil-works/pi-ai/api/openai-codex-responses";
import { normalizeContext } from "@earendil-works/pi-ai/utils/transcript";
import { fetch as modelFetch, ProxyAgent } from "undici";

export function codexContext(messages, config) {
  const result = [];
  const names = new Map();
  const system = [];
  const plain = (content) =>
    typeof content === "string"
      ? content
      : (content || [])
          .filter((item) => item.type === "text")
          .map((item) => item.text)
          .join("\n");
  for (const message of messages) {
    const type = message.getType();
    if (type === "system") {
      system.push(plain(message.content));
      continue;
    }
    if (type === "human") {
      result.push({
        role: "user",
        content: plain(message.content),
        timestamp: Date.now(),
      });
      continue;
    }
    if (type === "ai") {
      const original = message.additional_kwargs?.codexMessage;
      const content = original?.content || [
        ...(plain(message.content)
          ? [{ type: "text", text: plain(message.content) }]
          : []),
        ...(message.tool_calls || []).map((item) => ({
          type: "toolCall",
          id: item.id,
          name: item.name,
          arguments: item.args,
        })),
      ];
      for (const item of content)
        if (item.type === "toolCall") names.set(item.id, item.name);
      result.push(
        original || {
          role: "assistant",
          content,
          api: "openai-codex-responses",
          provider: "openai-codex",
          model: config.model,
          stopReason: "stop",
          timestamp: Date.now(),
          usage: {
            input: 0,
            output: 0,
            cacheRead: 0,
            cacheWrite: 0,
            totalTokens: 0,
            cost: {
              input: 0,
              output: 0,
              cacheRead: 0,
              cacheWrite: 0,
              total: 0,
            },
          },
        },
      );
    }
    if (type === "tool")
      result.push({
        role: "toolResult",
        toolCallId: message.tool_call_id,
        toolName: message.name || names.get(message.tool_call_id) || "tool",
        content: [{ type: "text", text: plain(message.content) }],
        isError: message.status === "error",
        timestamp: Date.now(),
      });
  }
  return { systemPrompt: system.join("\n\n"), messages: result };
}

export function codexModelDescriptor(config) {
  return {
    id: config.model,
    name: config.model,
    api: "openai-codex-responses",
    provider: "openai-codex",
    baseUrl: "https://chatgpt.com/backend-api",
    reasoning: true,
    input: ["text"],
    contextWindow: 128000,
    maxTokens: config.maxTokens || 1800,
    cost: { input: 0, output: 0, cacheRead: 0, cacheWrite: 0 },
  };
}

export function codexAIMessage(output) {
  if (["error", "aborted"].includes(output.stopReason))
    throw new Error(output.errorMessage || "Codex 模型请求失败");
  if (output.stopReason === "length")
    throw new Error("Codex 输出被截断，未执行其工具调用");
  return new AIMessage({
    content: output.content
      .filter((item) => item.type === "text")
      .map((item) => item.text)
      .join("\n"),
    tool_calls: output.content
      .filter((item) => item.type === "toolCall")
      .map((item) => ({
        type: "tool_call",
        id: item.id,
        name: item.name,
        args: item.arguments,
      })),
    additional_kwargs: { codexMessage: output },
    response_metadata: {
      model: output.responseModel || output.model,
      stopReason: output.stopReason,
      responseId: output.responseId,
    },
    usage_metadata: {
      input_tokens: output.usage.input + output.usage.cacheRead,
      output_tokens: output.usage.output,
      total_tokens: output.usage.totalTokens,
    },
  });
}

export class CodexChatModel extends BaseChatModel {
  constructor(config, stream = streamCodex) {
    super({});
    this.config = config;
    this.streamCodex = stream;
    this.proxy = config.proxyURL ? new ProxyAgent(config.proxyURL) : null;
  }
  _llmType() {
    return "openai-codex-responses";
  }
  bindTools(tools, options = {}) {
    return this.withConfig({
      ...options,
      tools: tools.map((tool) => convertToOpenAITool(tool)),
    });
  }
  async _generate(messages, options) {
    const descriptor = codexModelDescriptor(this.config);
    const context = codexContext(messages, this.config);
    context.tools = (options.tools || []).map((item) => ({
      name: item.function.name,
      description: item.function.description || "",
      parameters: item.function.parameters,
    }));
    const timeout = AbortSignal.timeout(
      (this.config.timeoutSeconds || 60) * 1000,
    );
    const signal = options.signal
      ? AbortSignal.any([options.signal, timeout])
      : timeout;
    const output = await this.streamCodex(
      descriptor,
      normalizeContext(context),
      {
        apiKey: this.config.apiKey,
        transport: "sse",
        signal,
        reasoningEffort: "low",
        reasoningSummary: "concise",
        textVerbosity: "low",
        maxRetries: 1,
        toolChoice: ["any", "required"].includes(options.tool_choice)
          ? "required"
          : options.tool_choice === "none"
            ? "none"
            : "auto",
        fetch: (url, init) =>
          modelFetch(url, {
            ...init,
            ...(this.proxy ? { dispatcher: this.proxy } : {}),
          }),
      },
    ).result();
    const message = codexAIMessage(output);
    return {
      generations: [
        {
          text: typeof message.content === "string" ? message.content : "",
          message,
        },
      ],
      llmOutput: {
        tokenUsage: {
          promptTokens: message.usage_metadata.input_tokens,
          completionTokens: message.usage_metadata.output_tokens,
          totalTokens: message.usage_metadata.total_tokens,
        },
      },
    };
  }
}
