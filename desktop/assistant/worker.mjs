import { runAssistant } from "./agent.mjs";
import { probeModel } from "./model.mjs";

const pending = new Map();
let active;
let requestCounter = 0;
const send = (value) => process.parentPort.postMessage(value);
process.parentPort.on("message", async ({ data }) => {
  if (data.type === "tool_result") {
    const promise = pending.get(data.requestId);
    if (promise) {
      pending.delete(data.requestId);
      promise.resolve(data.response);
    }
    return;
  }
  if (data.type === "cancel") {
    active?.abort();
    for (const promise of pending.values())
      promise.reject(new Error("分析已取消"));
    pending.clear();
    return;
  }
  if (data.type !== "start" && data.type !== "probe") return;
  active = new AbortController();
  try {
    const value =
      data.type === "probe"
        ? await probeModel(data.config, active.signal)
        : await runAssistant({
            ...data,
            signal: active.signal,
            emit: (event) => send({ type: "event", event }),
            invokeTool: (action, args) =>
              new Promise((resolve, reject) => {
                const requestId = ++requestCounter;
                pending.set(requestId, { resolve, reject });
                send({ type: "tool", requestId, action, args });
              }),
          });
    send({ type: "done", value });
  } catch (error) {
    send({
      type: "error",
      cancelled: active.signal.aborted,
      error: String(error.message || error),
    });
  }
});
