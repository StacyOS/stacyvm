import { afterEach, describe, expect, test } from "bun:test";

import { Client } from "../src/client.js";

type RecordedRequest = {
  url: string;
  method: string;
  headers: Record<string, string>;
  body?: unknown;
};

const originalFetch = globalThis.fetch;
const requests: RecordedRequest[] = [];

function jsonResponse(body: unknown): Response {
  return new Response(JSON.stringify(body), {
    status: 200,
    headers: { "Content-Type": "application/json" },
  });
}

function installFetchStub(): void {
  requests.length = 0;
  globalThis.fetch = async (input: RequestInfo | URL, init?: RequestInit) => {
    const url = input.toString();
    const method = init?.method ?? "GET";
    const headers = new Headers(init?.headers);
    const body = typeof init?.body === "string" ? JSON.parse(init.body) : undefined;
    requests.push({
      url,
      method,
      headers: Object.fromEntries(headers.entries()),
      body,
    });

    if (url.endsWith("/api/v1/sandboxes") && method === "POST") {
      return jsonResponse({
        id: "sb-parity",
        state: "running",
        provider: "mock",
        image: body?.image ?? "alpine:latest",
        memory_mb: body?.memory_mb ?? 512,
        vcpus: body?.vcpus ?? 1,
        created_at: "2026-05-08T00:00:00Z",
        expires_at: "2026-05-08T00:30:00Z",
        metadata: body?.metadata ?? {},
        preview_domain: "localhost",
      });
    }

    if (url.endsWith("/api/v1/sandboxes/admission") && method === "POST") {
      return jsonResponse({
        allowed: true,
        queueable: false,
        active_sandboxes: 1,
        max_sandboxes: 100,
      });
    }

    if (url.endsWith("/api/v1/providers") && method === "GET") {
      return jsonResponse([{ name: "mock", healthy: true, default: true }]);
    }

    if (url.endsWith("/api/v1/quotas/summary") && method === "GET") {
      return jsonResponse({
        total: 1,
        with_max_sandboxes: 1,
        with_max_ttl: 0,
        with_max_exec_timeout: 0,
      });
    }

    return jsonResponse({ status: "ok", version: "test", uptime: "1s" });
  };
}

afterEach(() => {
  globalThis.fetch = originalFetch;
  requests.length = 0;
});

describe("Client public API parity", () => {
  test("spawn sends the same control-plane fields as the Python SDK", async () => {
    installFetchStub();
    const client = new Client({
      baseUrl: "http://stacyvm.test/",
      apiKey: "api-key",
      userId: "team-a",
    });

    const sandbox = await client.spawn({
      image: "python:3.12-slim",
      provider: "mock",
      memory_mb: 1024,
      vcpus: 2,
      ttl: "1h",
      owner_id: "team-a",
      template: "python-dev",
      metadata: { purpose: "parity" },
    });

    expect(sandbox.id).toBe("sb-parity");
    expect(requests[0]).toEqual({
      url: "http://stacyvm.test/api/v1/sandboxes",
      method: "POST",
      headers: {
        "content-type": "application/json",
        "x-api-key": "api-key",
        "x-user-id": "team-a",
      },
      body: {
        image: "python:3.12-slim",
        provider: "mock",
        memory_mb: 1024,
        vcpus: 2,
        ttl: "1h",
        owner_id: "team-a",
        template: "python-dev",
        metadata: { purpose: "parity" },
      },
    });
  });

  test("exposes admission, providers, quota summary, and health helpers", async () => {
    installFetchStub();
    const client = new Client("http://stacyvm.test");

    await expect(client.admission({ image: "alpine", owner_id: "team-a" })).resolves.toMatchObject({
      allowed: true,
    });
    await expect(client.providers()).resolves.toEqual([
      { name: "mock", healthy: true, default: true },
    ]);
    await expect(client.quotaSummary()).resolves.toMatchObject({ total: 1 });
    await expect(client.health()).resolves.toMatchObject({ status: "ok" });

    expect(requests.map((request) => request.url)).toEqual([
      "http://stacyvm.test/api/v1/sandboxes/admission",
      "http://stacyvm.test/api/v1/providers",
      "http://stacyvm.test/api/v1/quotas/summary",
      "http://stacyvm.test/api/v1/health",
    ]);
  });
});
