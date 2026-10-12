const test = require("node:test");
const assert = require("node:assert/strict");
const { backendURL, apiRequest, trustedFrame } = require("./trust.cjs");
const { pathToFileURL } = require("node:url");

test("desktop API bridge confines requests to the configured API origin", () => {
  assert.equal(backendURL("http://127.0.0.1:8080/"), "http://127.0.0.1:8080");
  for (const url of [
    "file:///tmp/x",
    "https://user:secret@example.com",
    "https://example.com/api",
    "https://example.com?token=x",
  ])
    assert.throws(() => backendURL(url));
  for (const path of [
    "https://example.com/api/x",
    "//example.com/api/x",
    "/api/../x",
    "/api/%2e%2e/x",
    "/api/\\x",
    "/api/x#fragment",
    "/api/%2f%2fexample.com",
  ])
    assert.throws(() => apiRequest({ path }));
  assert.deepEqual(
    apiRequest({ path: "/api/case/evidence?runId=run.1", method: "get" }),
    {
      path: "/api/case/evidence?runId=run.1",
      method: "GET",
      body: undefined,
      etag: "",
    },
  );
  assert.throws(() => apiRequest({ path: "/api/catalog", method: "CONNECT" }));
});

test("only the packaged main frame may invoke desktop IPC", () => {
  const path = "/local/renderer/index.html";
  const mainFrame = { url: pathToFileURL(path).href };
  const sender = { mainFrame };
  assert.equal(trustedFrame({ sender, senderFrame: mainFrame }, path), true);
  assert.equal(
    trustedFrame({ sender, senderFrame: { url: mainFrame.url } }, path),
    false,
  );
  assert.equal(
    trustedFrame({ sender, senderFrame: { url: "https://example.com" } }, path),
    false,
  );
});
