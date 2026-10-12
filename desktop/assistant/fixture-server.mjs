import { createServer } from "node:http";

// A synthetic OpenAI-compatible service exercises the actual SDK/tool protocol.
export async function startAssistantFixture({
  invalidReference = false,
  delay = 0,
} = {}) {
  const requests = [];
  let responseDelay = delay;
  const server = createServer(async (request, response) => {
    let raw = "";
    for await (const chunk of request) raw += chunk;
    const input = JSON.parse(raw);
    requests.push(input);
    const tools = input.tools || [];
    const replies = input.messages.filter((message) => message.role === "tool");
    let message = { role: "assistant", content: "OK" };
    if (tools.length) {
      let name, args;
      if (
        !replies.some(
          (item) =>
            item.name === "read_map" || item.content.includes('"graph"'),
        )
      ) {
        name = "read_map";
        args = {};
      } else if (
        !replies.some(
          (item) =>
            item.name === "build_test_plan" ||
            item.content.includes('"physicalTasks"'),
        )
      ) {
        name = "build_test_plan";
        args = {};
      } else {
        const report = tools.find(
          (item) => item.function.parameters?.properties?.summary,
        );
        name = report.function.name;
        const context = JSON.parse(
          replies.find((item) => item.content.includes('"graph"')).content,
        );
        const id = context.graph.nodes[0].id;
        args = {
          summary:
            "已读取图谱，并通过现有 Planner 生成测试计划。此结果来自合成协议测试。",
          findings: [
            {
              kind: "observation",
              title: "依赖可追溯",
              detail:
                "当前节点已在 Store 图谱中登记，计划任务来自规则式 Planner。",
              nodeIds: [id],
              evidenceRefs: [
                invalidReference ? "node:invented" : `node:${id}`,
                "plan:preview",
              ],
            },
          ],
          nextActions: ["核对执行前置条件与已登记环境。"],
        };
      }
      message = {
        role: "assistant",
        content: null,
        tool_calls: [
          {
            id: `call-${requests.length}`,
            type: "function",
            function: { name, arguments: JSON.stringify(args) },
          },
        ],
      };
    }
    if (responseDelay)
      await new Promise((resolve) => setTimeout(resolve, responseDelay));
    response.setHeader("Content-Type", "application/json");
    response.end(
      JSON.stringify({
        id: `completion-${requests.length}`,
        object: "chat.completion",
        created: Math.floor(Date.now() / 1000),
        model: input.model,
        choices: [
          {
            index: 0,
            message,
            finish_reason: message.tool_calls ? "tool_calls" : "stop",
          },
        ],
        usage: { prompt_tokens: 20, completion_tokens: 30, total_tokens: 50 },
      }),
    );
  });
  await new Promise((resolve) => server.listen(0, "127.0.0.1", resolve));
  return {
    baseURL: `http://127.0.0.1:${server.address().port}/v1`,
    requests,
    setDelay: (value) => {
      responseDelay = value;
    },
    close: () =>
      new Promise((resolve) => {
        server.close(resolve);
        server.closeAllConnections();
      }),
  };
}
