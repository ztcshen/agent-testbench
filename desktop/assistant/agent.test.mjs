import test from "node:test";
import assert from "node:assert/strict";
import { runAssistant, validateAnalysis, boundToolData } from "./agent.mjs";
import { startAssistantFixture } from "./fixture-server.mjs";

process.env.LANGSMITH_TRACING = "false";
process.env.LANGCHAIN_TRACING_V2 = "false";

function invocation(baseURL, signal = new AbortController().signal) {
  return {
    config: {
      baseURL,
      model: "fixture-model",
      apiKey: "fixture-key",
      maxTokens: 800,
      timeoutSeconds: 3,
    },
    focus: { nodeId: "node.create" },
    messages: [{ role: "user", content: "Generate a test plan" }],
    signal,
    invokeTool: async (action) => ({
      ok: true,
      data:
        action === "map.read"
          ? {
              graph: { nodes: [{ id: "node.create" }], paths: [] },
              context: { cases: [], interfaces: [] },
            }
          : {
              physicalTasks: [
                { id: "task.create", kind: "run_case", nodeId: "node.create" },
              ],
            },
    }),
    emit() {},
  };
}

test("the actual LangChain agent calls scoped tools and returns validated references", async () => {
  const service = await startAssistantFixture();
  const events = [];
  try {
    const result = await runAssistant({
      ...invocation(service.baseURL),
      emit: (event) => events.push(event),
    });
    assert.deepEqual(
      result.receipts.map((item) => item.action),
      ["map.read", "plan.explain"],
    );
    assert.deepEqual(result.result.findings[0].nodeIds, ["node.create"]);
    assert.equal(
      events.filter((item) => item.type === "tool_completed").length,
      2,
    );
    assert(service.requests.length >= 3);
  } finally {
    await service.close();
  }
});

test("model output cannot invent a node or an Evidence reference", async () => {
  const service = await startAssistantFixture({ invalidReference: true });
  try {
    await assert.rejects(
      runAssistant(invocation(service.baseURL)),
      /未读取的证据/,
    );
  } finally {
    await service.close();
  }
  assert.throws(
    () =>
      validateAnalysis(
        { summary: "Passed", findings: [], nextActions: [] },
        [],
      ),
    /没有读取/,
  );
  assert.equal(boundToolData(undefined), undefined);
  const large = boundToolData({
    physicalTasks: Array.from({ length: 60 }, (_, i) => ({
      id: `task.${i}`,
      description: "x".repeat(5000),
      required: Object.fromEntries(
        Array.from({ length: 40 }, (_, j) => [`field${j}`, "x".repeat(2000)]),
      ),
    })),
  });
  assert.equal(large.truncated, true);
  assert(JSON.stringify(large).length <= 30000);
});

test("cancellation aborts model requests without creating an analysis", async () => {
  const service = await startAssistantFixture({ delay: 300 });
  const controller = new AbortController();
  const timer = setTimeout(() => controller.abort(), 60);
  try {
    await assert.rejects(
      runAssistant(invocation(service.baseURL, controller.signal)),
    );
  } finally {
    clearTimeout(timer);
    await service.close();
  }
});
