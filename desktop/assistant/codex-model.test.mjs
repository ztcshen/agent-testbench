import test from "node:test";
import assert from "node:assert/strict";
import {
  SystemMessage,
  HumanMessage,
  AIMessage,
  ToolMessage,
} from "@langchain/core/messages";
import {
  CodexChatModel,
  codexAIMessage,
  codexContext,
} from "./codex-model.mjs";

const usage = {
  input: 10,
  output: 5,
  cacheRead: 2,
  cacheWrite: 0,
  totalTokens: 17,
  cost: { input: 0, output: 0, cacheRead: 0, cacheWrite: 0, total: 0 },
};
test("Codex bridge preserves tool identities and private reasoning continuity", async () => {
  const original = {
    role: "assistant",
    model: "gpt-5.6-sol",
    api: "openai-codex-responses",
    provider: "openai-codex",
    stopReason: "toolUse",
    usage,
    timestamp: 1,
    content: [
      { type: "thinking", thinking: "", thinkingSignature: "opaque-reasoning" },
      {
        type: "toolCall",
        id: "call_one|fc_one",
        name: "read_map",
        arguments: {},
      },
    ],
  };
  const ai = codexAIMessage(original);
  const context = codexContext(
    [
      new SystemMessage("Read-only"),
      new HumanMessage("Plan"),
      ai,
      new ToolMessage({
        content: '{"nodes":1}',
        tool_call_id: "call_one|fc_one",
      }),
    ],
    { model: "gpt-5.6-sol" },
  );
  assert.equal(ai.content, "");
  assert.equal(context.messages[1], original);
  assert.equal(context.messages[2].toolName, "read_map");
  assert.equal(context.messages[2].toolCallId, "call_one|fc_one");
  assert.throws(
    () => codexAIMessage({ ...original, stopReason: "length" }),
    /截断/,
  );
  let seen;
  const model = new CodexChatModel(
    { model: "gpt-5.6-sol", apiKey: "fixture-token", timeoutSeconds: 2 },
    (descriptor, context, options) => ({
      result: async () => {
        seen = { descriptor, context, options };
        return {
          ...original,
          stopReason: "stop",
          content: [{ type: "text", text: "OK" }],
        };
      },
    }),
  );
  const reply = await model
    .bindTools(
      [
        {
          type: "function",
          function: {
            name: "read_map",
            description: "Read map",
            parameters: { type: "object", properties: {} },
          },
        },
      ],
      { tool_choice: "any" },
    )
    .invoke([new HumanMessage("Read map")]);
  assert.equal(reply.content, "OK");
  assert.equal(seen.descriptor.id, "gpt-5.6-sol");
  assert.equal(seen.context.messages[0].toolsAdded[0].name, "read_map");
  assert.equal(seen.options.transport, "sse");
  assert.equal(seen.options.toolChoice, "required");
});
