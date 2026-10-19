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
import { POST } from "@/app/api/connections/binance/order/route";
import { orderLookupSchema, orderQuerySchema } from "@/lib/order-lookup-schema";
import { sessionDigest } from "@/lib/sessions";
import { syntheticOrderLookup } from "./fixtures/order-lookup";

const accountId = "00000000-0000-4000-8000-000000000001";
beforeEach(() => {
  vi.resetModules();
  mocks.authenticated.mockResolvedValue(true);
  mocks.backend.mockReturnValue("postgres");
  vi.stubEnv("APP_ORIGIN", "https://terminal.example");
  vi.stubEnv("ENGINE_URL", "http://engine.railway.internal:8080");
  vi.stubEnv("ENGINE_API_TOKEN", randomBytes(48).toString("base64"));
});
afterEach(() => {
  vi.unstubAllEnvs();
  vi.unstubAllGlobals();
});
function request(body: unknown, origin = "https://terminal.example") {
  return new Request("https://terminal.example/api/connections/binance/order", {
    method: "POST",
    headers: { Origin: origin, "Content-Type": "application/json" },
    body: JSON.stringify(body),
  });
}
const valid = () => ({ accountId, ...syntheticOrderLookup().query });
// Reload the proxy for each test to isolate its deliberate per-process limiter.
async function post(body: unknown, origin?: string) {
  const { POST: isolated } =
    await import("@/app/api/connections/binance/order/route");
  return isolated(request(body, origin));
}
it.each(["observed", "unresolved"])(
  "returns only validated, query-bound %s observations",
  async (outcome) => {
    const body = syntheticOrderLookup();
    if (outcome === "unresolved") {
      body.outcome = "unresolved";
      body.reason = "exchange_not_found";
      body.order = null;
    }
    const fetch = vi.fn().mockResolvedValue(Response.json(body));
    vi.stubGlobal("fetch", fetch);
    const response = await post(valid());
    expect(response.status).toBe(200);
    expect(await response.json()).toEqual(body);
    expect(response.headers.get("cache-control")).toBe("no-store");
    expect(fetch).toHaveBeenCalledWith(
      new URL(
        "http://engine.railway.internal:8080/v1/connections/binance/order",
      ),
      expect.objectContaining({
        redirect: "error",
        cache: "no-store",
        body: JSON.stringify({
          ...valid(),
          tokenHash: sessionDigest("synthetic-cookie"),
        }),
      }),
    );
  },
);
it.each([
  { symbol: "BTCUSDT" },
  { symbol: "BTCUSDT", orderId: "1", clientOrderId: "both" },
  { symbol: "BTCUSDT", orderId: 9007199254740992 },
  { symbol: "BTCUSDT", orderId: "9223372036854775808" },
  { symbol: "BTCUSDT", orderId: "01" },
  { symbol: "BTCUSDC", orderId: "1" },
  { symbol: "BTCUSDT", clientOrderId: "a".repeat(37) },
  { symbol: "BTCUSDT", orderId: "1", ownerId: "attacker" },
])(
  "rejects invalid selectors and caller ownership without contacting the engine",
  async (query) => {
    const fetch = vi.fn();
    vi.stubGlobal("fetch", fetch);
    expect((await post({ accountId, ...query })).status).toBe(400);
    expect(fetch).not.toHaveBeenCalled();
  },
);
it("requires server authentication, same origin and PostgreSQL sessions", async () => {
  const fetch = vi.fn();
  vi.stubGlobal("fetch", fetch);
  mocks.authenticated.mockResolvedValue(false);
  expect((await post(valid())).status).toBe(401);
  mocks.authenticated.mockResolvedValue(true);
  expect((await post(valid(), "https://untrusted.example")).status).toBe(403);
  mocks.backend.mockReturnValue("local");
  expect((await post(valid())).status).toBe(503);
  expect(fetch).not.toHaveBeenCalled();
});
it.each([
  (b: ReturnType<typeof syntheticOrderLookup>) => ({
    ...b,
    executionReady: true,
  }),
  (b: ReturnType<typeof syntheticOrderLookup>) => ({
    ...b,
    query: { symbol: "BTCUSDT", orderId: "123" },
    order: { ...b.order, orderId: "123" },
  }),
  (b: ReturnType<typeof syntheticOrderLookup>) => ({
    ...b,
    order: { ...b.order, symbol: "ETHUSDT" },
  }),
  (b: ReturnType<typeof syntheticOrderLookup>) => ({
    ...b,
    secret: "private-response",
  }),
  (b: ReturnType<typeof syntheticOrderLookup>) => ({
    ...b,
    outcome: "unresolved",
    reason: "exchange_not_found",
  }),
  (b: ReturnType<typeof syntheticOrderLookup>) => ({
    ...b,
    checkedAt: "2026-01-01T00:00:30Z",
  }),
  (b: ReturnType<typeof syntheticOrderLookup>) => ({
    ...b,
    order: { ...b.order, executedQty: "3" },
  }),
  (b: ReturnType<typeof syntheticOrderLookup>) => ({
    ...b,
    order: { ...b.order, avgPrice: "NaN" },
  }),
])(
  "never exposes mismatched, unsafe or incomplete responses",
  async (mutate) => {
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue(Response.json(mutate(syntheticOrderLookup()))),
    );
    const response = await post(valid());
    expect(response.status).toBe(503);
    expect(await response.text()).not.toContain("private-response");
  },
);
it("scrubs exchange and driver errors", async () => {
  vi.stubGlobal(
    "fetch",
    vi
      .fn()
      .mockResolvedValue(
        Response.json({ error: "private-signed-url" }, { status: 503 }),
      ),
  );
  const response = await post(valid());
  expect(response.status).toBe(503);
  expect(await response.text()).not.toContain("private-signed-url");
});
it("accepts only one client identifier and preserves exact decimals", () => {
  expect(
    orderQuerySchema.safeParse({
      symbol: "BTCUSDT",
      clientOrderId: "a/b:c.d_-",
    }).success,
  ).toBe(true);
  const body = syntheticOrderLookup();
  body.query = { symbol: "BTCUSDT", clientOrderId: "synthetic-limit" };
  expect(orderLookupSchema.parse(body).order?.avgPrice).toBe("89.12345678");
  expect(typeof POST).toBe("function");
});

it("rejects zero quantity even when Decimal considers its sign positive", () => {
  const body = syntheticOrderLookup();
  expect(
    orderLookupSchema.safeParse({
      ...body,
      order: { ...body.order, origQty: "0", executedQty: "0", avgPrice: "0" },
    }).success,
  ).toBe(false);
});
