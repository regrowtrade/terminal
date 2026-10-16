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
import { POST as list } from "@/app/api/connections/binance/list/route";
import { POST as snapshot } from "@/app/api/connections/binance/snapshot/route";
import { POST as latest } from "@/app/api/connections/binance/latest/route";
import { sessionDigest } from "@/lib/sessions";
import { syntheticSnapshot } from "./fixtures/binance";

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
function request(body: unknown = {}, origin = "https://terminal.example") {
  return new Request("https://terminal.example/api/connections/binance/list", {
    method: "POST",
    headers: { Origin: origin, "Content-Type": "application/json" },
    body: JSON.stringify(body),
  });
}

it("forwards only a server-derived session digest and sanitized metadata", async () => {
  const fetch = vi
    .fn()
    .mockResolvedValue(Response.json({ enabled: false, accounts: [] }));
  vi.stubGlobal("fetch", fetch);
  const response = await list(request());
  expect(response.status).toBe(200);
  expect(response.headers.get("cache-control")).toBe("no-store");
  expect(fetch).toHaveBeenCalledWith(
    new URL("http://engine.railway.internal:8080/v1/connections/binance/list"),
    expect.objectContaining({
      cache: "no-store",
      redirect: "error",
      body: JSON.stringify({ tokenHash: sessionDigest("synthetic-cookie") }),
    }),
  );
  expect(await response.text()).not.toContain(process.env.ENGINE_API_TOKEN);
});
it("rejects anonymous or cross-origin requests before contacting the engine", async () => {
  const fetch = vi.fn();
  vi.stubGlobal("fetch", fetch);
  mocks.authenticated.mockResolvedValue(false);
  expect((await list(request())).status).toBe(401);
  mocks.authenticated.mockResolvedValue(true);
  expect((await list(request({}, "https://untrusted.example"))).status).toBe(
    403,
  );
  expect(fetch).not.toHaveBeenCalled();
});
it("does not enable live access from development sessions", async () => {
  mocks.backend.mockReturnValue("local");
  const fetch = vi.fn();
  vi.stubGlobal("fetch", fetch);
  expect((await list(request())).status).toBe(503);
  expect(fetch).not.toHaveBeenCalled();
});
it.each([
  { userId: "attacker" },
  { apiKey: "must-not-be-accepted" },
  { url: "https://untrusted.example" },
])("rejects caller credentials, owner or endpoint injection", async (body) => {
  const fetch = vi.fn();
  vi.stubGlobal("fetch", fetch);
  expect((await list(request(body))).status).toBe(400);
  expect(fetch).not.toHaveBeenCalled();
});
it("validates account IDs before a snapshot request", async () => {
  vi.stubGlobal("fetch", vi.fn());
  expect((await snapshot(request({ accountId: "not-a-uuid" }))).status).toBe(
    400,
  );
});
it("rejects an unexpected envelope or token in engine responses", async () => {
  vi.stubGlobal(
    "fetch",
    vi.fn().mockResolvedValue(
      Response.json({
        enabled: true,
        accounts: [],
        envelope: { secret: "private" },
      }),
    ),
  );
  const response = await list(request());
  expect(response.status).toBe(503);
  expect(await response.text()).not.toContain("private");
});
it("does not forward exchange/driver errors to the browser", async () => {
  vi.stubGlobal(
    "fetch",
    vi
      .fn()
      .mockResolvedValue(
        Response.json({ error: "signed-url-and-private-key" }, { status: 503 }),
      ),
  );
  const response = await list(request());
  expect(response.status).toBe(503);
  expect(await response.text()).not.toContain("signed-url-and-private-key");
});
it("proxies all read-only snapshot components without converting int64 IDs", async () => {
  const body = syntheticSnapshot();
  const fetch = vi.fn().mockResolvedValue(Response.json(body));
  vi.stubGlobal("fetch", fetch);
  const response = await snapshot(
    request({ accountId: "00000000-0000-4000-8000-000000000001" }),
  );
  expect(response.status).toBe(200);
  expect(await response.json()).toEqual(body);
});
it("does not return a partial snapshot if the engine omits algo orders", async () => {
  const { algoOrders: _omitted, ...body } = syntheticSnapshot();
  void _omitted;
  vi.stubGlobal("fetch", vi.fn().mockResolvedValue(Response.json(body)));
  const response = await snapshot(
    request({ accountId: "00000000-0000-4000-8000-000000000001" }),
  );
  expect(response.status).toBe(503);
  expect(await response.text()).not.toContain("100.00000001");
});
it("only recovers saved snapshots through the authenticated private latest endpoint", async () => {
  const body = {
    snapshot: syntheticSnapshot(),
    storedAt: new Date().toISOString(),
  };
  const fetch = vi.fn().mockResolvedValue(Response.json(body));
  vi.stubGlobal("fetch", fetch);
  const response = await latest(
    request({ accountId: "00000000-0000-4000-8000-000000000001" }),
  );
  expect(response.status).toBe(200);
  expect(await response.json()).toEqual(body);
  expect(fetch.mock.calls[0][0].pathname).toBe(
    "/v1/connections/binance/latest",
  );
});
it("does not recover saved data for anonymous users or allow owner injection", async () => {
  const fetch = vi.fn();
  vi.stubGlobal("fetch", fetch);
  mocks.authenticated.mockResolvedValue(false);
  expect((await latest(request())).status).toBe(401);
  mocks.authenticated.mockResolvedValue(true);
  expect(
    (
      await latest(
        request({
          accountId: "00000000-0000-4000-8000-000000000001",
          ownerId: "attacker",
        }),
      )
    ).status,
  ).toBe(400);
  expect(fetch).not.toHaveBeenCalled();
});
