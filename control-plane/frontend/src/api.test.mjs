import assert from "node:assert/strict";
import { afterEach, describe, it } from "node:test";

import {
  bootstrapEnvironment,
  fetchCurrentStore,
  inspectEnvironment,
  listEnvironments,
  publishVerifiedEnvironment,
  registerEnvironment,
  verifyEnvironment,
} from "./api.js";

const originalFetch = globalThis.fetch;

afterEach(() => {
  globalThis.fetch = originalFetch;
});

function mockFetch(handler) {
  const calls = [];
  globalThis.fetch = async (path, options = {}) => {
    calls.push({ path, options });
    const payload = handler(path, options);
    return {
      ok: payload.ok ?? true,
      statusText: payload.statusText || "OK",
      json: async () => payload.body ?? payload,
    };
  };
  return calls;
}

describe("environment catalog API", () => {
  it("lists verified discovery by default and all discovery on request", async () => {
    const calls = mockFetch((path) => ({ body: { ok: true, path } }));

    await listEnvironments();
    await listEnvironments({ all: true });

    assert.equal(calls[0].path, "/api/environments");
    assert.equal(calls[1].path, "/api/environments?all=true");
  });

  it("wraps register, inspect, bootstrap, verify, and publish verified endpoints", async () => {
    const calls = mockFetch(() => ({ body: { ok: true } }));

    await registerEnvironment({ id: "env.team.api" });
    await inspectEnvironment("env.team.api");
    await bootstrapEnvironment("env.team.api");
    await verifyEnvironment("env.team.api", { runId: "run-1", status: "passed" });
    await publishVerifiedEnvironment("env.team.api");

    assert.deepEqual(calls.map((call) => [call.path, call.options.method || "GET"]), [
      ["/api/environments", "POST"],
      ["/api/environments/env.team.api", "GET"],
      ["/api/environments/env.team.api/bootstrap", "GET"],
      ["/api/environments/env.team.api/verify", "POST"],
      ["/api/environments/env.team.api/publish-verified", "POST"],
    ]);
    assert.equal(calls[0].options.body, JSON.stringify({ id: "env.team.api" }));
    assert.equal(calls[3].options.body, JSON.stringify({ runId: "run-1", status: "passed" }));
  });
});

describe("store visibility API", () => {
  it("loads the current runtime Store metadata", async () => {
    const calls = mockFetch(() => ({ body: { ok: true, configured: true, backend: "postgres" } }));

    const payload = await fetchCurrentStore();

    assert.equal(calls[0].path, "/api/store/current");
    assert.equal(payload.backend, "postgres");
  });
});

describe("requestJSON", () => {
  it("attaches the response payload to thrown API errors", async () => {
    mockFetch(() => ({
      ok: false,
      statusText: "Bad Request",
      body: {
        ok: false,
        error: "profile verification failed",
        summary: { failedChecks: 1 },
      },
    }));

    await assert.rejects(
      fetchCurrentStore(),
      (error) => {
        assert.equal(error.message, "profile verification failed");
        assert.deepEqual(error.payload.summary, { failedChecks: 1 });
        return true;
      },
    );
  });
});
