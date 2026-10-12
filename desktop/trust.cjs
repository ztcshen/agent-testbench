const { pathToFileURL } = require("node:url");

function backendURL(value) {
  const parsed = new URL(String(value).trim());
  if (
    !["http:", "https:"].includes(parsed.protocol) ||
    parsed.username ||
    parsed.password ||
    parsed.search ||
    parsed.hash ||
    !["", "/"].includes(parsed.pathname)
  ) {
    throw new Error(
      "服务地址应为 HTTP 或 HTTPS 地址，不包含账号、路径或查询参数",
    );
  }
  return parsed.origin;
}

function apiRequest(input) {
  const path = String(input?.path || "");
  const decoded = decodeURIComponent(path.split("?")[0]);
  if (
    !path.startsWith("/api/") ||
    path.includes("\\") ||
    path.includes("#") ||
    decoded.includes("..") ||
    decoded.includes("//") ||
    /[\u0000-\u001f]/.test(path)
  ) {
    throw new Error("无效的 TestBench API 路径");
  }
  const method = String(input.method || "GET").toUpperCase();
  if (!["GET", "POST", "PUT", "PATCH", "DELETE"].includes(method))
    throw new Error("不支持该请求方法");
  const body =
    input.body === undefined ? undefined : JSON.stringify(input.body);
  if (body?.length > 2 * 1024 * 1024) throw new Error("请求配置超过 2 MiB");
  return { path, method, body, etag: String(input.etag || "") };
}

function trustedFrame(event, rendererPath) {
  return (
    event.senderFrame?.url === pathToFileURL(rendererPath).href &&
    event.senderFrame === event.sender.mainFrame
  );
}

module.exports = { backendURL, apiRequest, trustedFrame };
