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
import { algoLookupSchema, algoQuerySchema } from "@/lib/algo-lookup-schema";
import { orderLookupSchema } from "@/lib/order-lookup-schema";
import { sessionDigest } from "@/lib/sessions";
import { syntheticAlgoLookup } from "./fixtures/algo-lookup";

const accountId = "00000000-0000-4000-8000-000000000001";
type MutableEvidence = {
  query: Record<string, unknown>;
  order: Record<string, unknown>;
  [key: string]: unknown;
};
beforeEach(() => {
  vi.resetModules();
  mocks.authenticated.mockResolvedValue(true);
  mocks.backend.mockReturnValue("postgres");
  vi.stubEnv("APP_ORIGIN", "https://terminal.example");
  vi.stubEnv("ENGINE_URL", "http://engine.railway.internal:8080");
  vi.stubEnv("ENGINE_API_TOKEN", randomBytes(48).toString("base64"));
});
afterEach(() => {
  vi.unstubAllGlobals();
  vi.unstubAllEnvs();
});
const valid = () => ({ accountId, ...syntheticAlgoLookup().query });
async function post(
  body: unknown,
  origin = "https://terminal.example",
  contentType = "application/json",
) {
  const { POST } = await import("@/app/api/connections/binance/algo/route");
  return POST(
    new Request("https://terminal.example/api/connections/binance/algo", {
      method: "POST",
      headers: { Origin: origin, "Content-Type": contentType },
      body: JSON.stringify(body),
    }),
  );
}

it.each(["observed", "unresolved"])(
  "proxies validated %s algo evidence and derives the session on the server",
  async (outcome) => {
    const body = syntheticAlgoLookup();
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
    expect(fetch).toHaveBeenCalledTimes(1);
    expect(fetch).toHaveBeenCalledWith(
      new URL(
        "http://engine.railway.internal:8080/v1/connections/binance/algo",
      ),
      expect.objectContaining({
        method: "POST",
        redirect: "error",
        cache: "no-store",
        body: JSON.stringify({
          ...valid(),
          tokenHash: sessionDigest("synthetic-cookie"),
        }),
      }),
    );
    expect(orderLookupSchema.safeParse(body).success).toBe(false);
  },
);

it("accepts a single client algo identifier without coercing exact decimals or IDs", async () => {
  const body = syntheticAlgoLookup();
  body.query = { symbol: "BTCUSDT", clientAlgoId: "synthetic-algo" };
  vi.stubGlobal("fetch", vi.fn().mockResolvedValue(Response.json(body)));
  const response = await post({ accountId, ...body.query });
  expect(response.status).toBe(200);
  const result = await response.json();
  expect(result.order.actualOrderId).toBe("9007199254740995");
  expect(result.order.actualQty).toBe("0.500000000000000001");
  expect(result.order.triggerPrice).toBe("95.123456789012345678");
});

it.each([
  { symbol: "BTCUSDT" },
  { symbol: "BTCUSDT", algoId: "1", clientAlgoId: "both" },
  { symbol: "BTCUSDT", algoId: "1", clientAlgoId: null },
  { symbol: "BTCUSDT", algoId: "1", clientAlgoId: "" },
  { symbol: "BTCUSDT", algoId: 9007199254740992 },
  { symbol: "BTCUSDT", algoId: "9223372036854775808" },
  { symbol: "BTCUSDT", algoId: "01" },
  { symbol: "BTCUSDC", algoId: "1" },
  { symbol: "BTCUSDT", clientAlgoId: "x".repeat(37) },
  { symbol: "BTCUSDT", algoId: "1", orderId: "1" },
  { symbol: "BTCUSDT", algoId: "1", tokenHash: "a".repeat(64) },
  { symbol: "BTCUSDT", algoId: "1", ownerId: "attacker" },
])(
  "rejects invalid selectors and authority injection without engine access",
  async (query) => {
    const fetch = vi.fn();
    vi.stubGlobal("fetch", fetch);
    expect((await post({ accountId, ...query })).status).toBe(400);
    expect(fetch).not.toHaveBeenCalled();
  },
);

it("requires authentication, same origin, PostgreSQL sessions and exact JSON content type", async () => {
  const fetch = vi.fn();
  vi.stubGlobal("fetch", fetch);
  mocks.authenticated.mockResolvedValue(false);
  expect((await post(valid())).status).toBe(401);
  mocks.authenticated.mockResolvedValue(true);
  expect((await post(valid(), "https://other.example")).status).toBe(403);
  mocks.backend.mockReturnValue("local");
  expect((await post(valid())).status).toBe(503);
  mocks.backend.mockReturnValue("postgres");
  expect((await post(valid(), undefined, "text/plain")).status).toBe(415);
  expect(fetch).not.toHaveBeenCalled();
});

it.each([
  (b: MutableEvidence) => {
    b.family = "regular";
  },
  (b: MutableEvidence) => {
    b.executionReady = true;
  },
  (b: MutableEvidence) => {
    b.query.algoId = "123";
    b.order.algoId = "123";
  },
  (b: MutableEvidence) => {
    b.query.symbol = "ETHUSDT";
    b.order.symbol = "ETHUSDT";
  },
  (b: MutableEvidence) => {
    b.query = { symbol: "BTCUSDT", clientAlgoId: "synthetic-algo" };
  },
  (b: MutableEvidence) => {
    b.order.actualQty = "NaN";
  },
  (b: MutableEvidence) => {
    b.order.quantity = "0";
  },
  (b: MutableEvidence) => {
    b.order.actualQty = "2.000000000000000001";
  },
  (b: MutableEvidence) => {
    b.order.algoStatus = "FILLED";
  },
  (b: MutableEvidence) => {
    b.order.triggerTime = 0;
  },
  (b: MutableEvidence) => {
    b.order.actualOrderId = "9223372036854775808";
  },
  (b: MutableEvidence) => {
    b.order.actualPrice = "0";
  },
  (b: MutableEvidence) => {
    b.order.algoStatus = "NEW";
  },
  (b: MutableEvidence) => {
    b.order.orderType = "STOP";
    b.order.price = "0";
  },
  (b: MutableEvidence) => {
    b.order.triggerPrice = "0";
  },
  (b: MutableEvidence) => {
    b.order.workingType = "LAST_PRICE";
  },
  (b: MutableEvidence) => {
    b.order.actualOrderId = "";
  },
  (b: MutableEvidence) => {
    delete b.order.closePosition;
  },
  (b: MutableEvidence) => {
    b.outcome = "unresolved";
    b.reason = "exchange_not_found";
  },
  (b: MutableEvidence) => {
    b.private = "private-response";
  },
  (b: MutableEvidence) => {
    b.checkedAt = "2026-01-01T00:00:30Z";
  },
])(
  "rejects mismatched, malformed or unsafe evidence as a whole",
  async (mutate) => {
    const body = syntheticAlgoLookup();
    mutate(body as unknown as MutableEvidence);
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(Response.json(body)));
    const response = await post(valid());
    expect(response.status).toBe(503);
    expect(await response.text()).not.toContain("private-response");
  },
);

it.each([401, 404, 422, 429, 503, 500])(
  "scrubs upstream error details (%s) and does not retry",
  async (status) => {
    const fetch = vi
      .fn()
      .mockResolvedValue(
        Response.json({ error: "private-signed-url" }, { status }),
      );
    vi.stubGlobal("fetch", fetch);
    const response = await post(valid());
    expect(response.status).toBe(status === 500 ? 503 : status);
    expect(await response.text()).not.toContain("private-signed-url");
    expect(fetch).toHaveBeenCalledTimes(1);
  },
);

it("supports close-all zero quantity and validated trailing without inferring fills", () => {
  const body = syntheticAlgoLookup();
  body.order!.closePosition = true;
  body.order!.quantity = "0";
  expect(algoLookupSchema.safeParse(body).success).toBe(true);
  body.order!.closePosition = false;
  body.order!.quantity = "2";
  body.order!.orderType = "TRAILING_STOP_MARKET";
  body.order!.activatePrice = "100";
  body.order!.callbackRate = "0.1";
  body.order!.triggerPrice = "0";
  expect(algoLookupSchema.safeParse(body).success).toBe(true);
  body.order!.callbackRate = "0.099999999999999999";
  expect(algoLookupSchema.safeParse(body).success).toBe(false);
  expect(
    algoQuerySchema.safeParse({ symbol: "BTCUSDT", clientAlgoId: "a/b:c.d_-" })
      .success,
  ).toBe(true);
});
