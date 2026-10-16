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
import { sessionDigest } from "@/lib/sessions";

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
