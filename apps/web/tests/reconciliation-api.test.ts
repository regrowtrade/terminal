import { randomBytes } from "node:crypto";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
vi.mock("server-only", () => ({}));
const mocks = vi.hoisted(() => ({ authenticated: vi.fn(), backend: vi.fn() }));
vi.mock("@/lib/auth", () => ({
  authenticated: mocks.authenticated,
  cookieName: () => "test-session",
}));
vi.mock("next/headers", () => ({
  cookies: async () => ({ get: () => ({ value: "synthetic-cookie" }) }),
}));
vi.mock("@/lib/sessions", async (original) => ({
  ...(await original<typeof import("@/lib/sessions")>()),
  sessionBackend: mocks.backend,
}));
import { POST } from "@/app/api/connections/binance/reconcile/route";
import { sessionDigest } from "@/lib/sessions";
import { syntheticReconciliation } from "./fixtures/reconciliation";

beforeEach(() => {
  mocks.authenticated.mockResolvedValue(true);
  mocks.backend.mockReturnValue("postgres");
  vi.stubEnv("APP_ORIGIN", "https://terminal.example");
  vi.stubEnv("ENGINE_URL", "http://engine.railway.internal:8080");
  for (const name of [
    "ENGINE_API_TOKEN",
    "SESSION_SECRET",
    "TERMINAL_PASSWORD",
  ])
    vi.stubEnv(name, randomBytes(48).toString("base64"));
});
afterEach(() => {
  vi.unstubAllEnvs();
  vi.unstubAllGlobals();
});
const accountId = "00000000-0000-4000-8000-000000000001";
function request(
  body: unknown = { accountId },
  origin = "https://terminal.example",
) {
  return new Request(
    "https://terminal.example/api/connections/binance/reconcile",
    {
      method: "POST",
      headers: { Origin: origin, "Content-Type": "application/json" },
      body: JSON.stringify(body),
    },
  );
}
it("forwards only server-derived identity and parses the report", async () => {
  const payload = syntheticReconciliation();
  const fetch = vi.fn().mockResolvedValue(Response.json(payload));
  vi.stubGlobal("fetch", fetch);
  const response = await POST(request());
  expect(response.status).toBe(200);
  expect(response.headers.get("cache-control")).toBe("no-store");
  expect(await response.json()).toEqual(payload);
  expect(fetch).toHaveBeenCalledWith(
    new URL(
      "http://engine.railway.internal:8080/v1/connections/binance/reconcile",
    ),
    expect.objectContaining({
      method: "POST",
      cache: "no-store",
      redirect: "error",
      body: JSON.stringify({
        accountId,
        tokenHash: sessionDigest("synthetic-cookie"),
      }),
    }),
  );
});
it("rejects anonymous, cross-origin, local-session and cursor access", async () => {
  const fetch = vi.fn();
  vi.stubGlobal("fetch", fetch);
  mocks.authenticated.mockResolvedValue(false);
  expect((await POST(request())).status).toBe(401);
  mocks.authenticated.mockResolvedValue(true);
  expect(
    (await POST(request({ accountId }, "https://untrusted.example"))).status,
  ).toBe(403);
  expect((await POST(request({ accountId, after: 1 }))).status).toBe(400);
  mocks.backend.mockReturnValue("local");
  expect((await POST(request())).status).toBe(503);
  expect(fetch).not.toHaveBeenCalled();
});
it.each([{ ownerId: "attacker" }, { apiKey: "forbidden" }])(
  "rejects caller-supplied owner and credentials",
  async (extra) => {
    const fetch = vi.fn();
    vi.stubGlobal("fetch", fetch);
    expect((await POST(request({ accountId, ...extra }))).status).toBe(400);
    expect(fetch).not.toHaveBeenCalled();
  },
);
it("rejects overclaiming engine payloads without leaking details", async () => {
  const body = syntheticReconciliation();
  const fetch = vi
    .fn()
    .mockResolvedValue(Response.json({ ...body, executionReady: true }));
  vi.stubGlobal("fetch", fetch);
  const response = await POST(request());
  expect(response.status).toBe(503);
  expect(await response.text()).not.toContain("must-not-leak");
});

it("preserves signed REST commissions as original decimals", async () => {
  const body = syntheticReconciliation();
  body.fills[0].commission = "-0.001000000000000001";
  vi.stubGlobal("fetch", vi.fn().mockResolvedValue(Response.json(body)));
  const response = await POST(request());
  expect(response.status).toBe(200);
  expect((await response.json()).fills[0].commission).toBe(
    "-0.001000000000000001",
  );
});
