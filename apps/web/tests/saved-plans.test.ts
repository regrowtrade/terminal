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
import { POST as save } from "@/app/api/plans/save/route";
import { POST as list } from "@/app/api/plans/list/route";
import { POST as intentsSave } from "@/app/api/plans/intents/save/route";
import { POST as intentsList } from "@/app/api/plans/intents/list/route";
import { sessionDigest } from "@/lib/sessions";

const accountId = "00000000-0000-4000-8000-000000000001";
const planId = "00000000-0000-4000-8000-000000000002";
const parameters = {
  mode: "clean",
  symbol: "BTCUSDT",
  side: "LONG",
  entry: "100",
  quantity: "1",
  risk: "2",
  rewardRatio: "3",
  riskUnit: "amount",
  accountReference: "100",
  accountBasis: "equity",
  spacingPercent: "4",
  leverage: 10,
  maxLevels: 1,
  maxMargin: "100",
  costReserve: "0",
};
const plan = {
  mode: "clean",
  symbol: "BTCUSDT",
  side: "LONG",
  simulation: true,
  risk: "2",
  riskInput: "2",
  riskUnit: "amount",
  accountReference: "100",
  accountBasis: "equity",
  costReserve: "0",
  baselinePnl: null,
  pnlThreshold: null,
  spacingPercent: "0",
  levels: [
    {
      index: 1,
      price: "100",
      quantity: "1",
      cumulativeQuantity: "1",
      averagePrice: "100",
      margin: "10",
      stopPrice: "98",
      takeProfit: "106",
      takeProfitQuantity: "1",
      netExposure: "1",
    },
  ],
  warnings: [],
  stopReason: "Número máximo de niveles solicitado alcanzado.",
};

beforeEach(() => {
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
function request(
  path: string,
  body: unknown,
  origin = "https://terminal.example",
) {
  return new Request(`https://terminal.example${path}`, {
    method: "POST",
    headers: { Origin: origin, "Content-Type": "application/json" },
    body: JSON.stringify(body),
  });
}

it("flattens a plan and forwards only the server-derived session digest", async () => {
  const fetch = vi.fn().mockResolvedValue(
    Response.json({
      campaignId: "00000000-0000-4000-8000-000000000009",
      planId,
      version: 1,
    }),
  );
  vi.stubGlobal("fetch", fetch);
  const response = await save(
    request("/api/plans/save", { accountId, plan: parameters }),
  );
  expect(response.status).toBe(200);
  expect(response.headers.get("cache-control")).toBe("no-store");
  expect(fetch).toHaveBeenCalledWith(
    new URL("http://engine.railway.internal:8080/v1/plans/save"),
    expect.objectContaining({
      redirect: "error",
      cache: "no-store",
      body: JSON.stringify({
        ...parameters,
        accountId,
        tokenHash: sessionDigest("synthetic-cookie"),
      }),
    }),
  );
  expect(await response.text()).not.toContain(process.env.ENGINE_API_TOKEN);
});

it("rejects invalid plans and caller ownership before contacting the engine", async () => {
  const fetch = vi.fn();
  vi.stubGlobal("fetch", fetch);
  expect(
    (
      await save(
        request("/api/plans/save", {
          accountId,
          plan: { ...parameters, entry: "-1" },
        }),
      )
    ).status,
  ).toBe(400);
  expect(
    (
      await save(
        request("/api/plans/save", {
          accountId,
          plan: parameters,
          ownerId: "attacker",
        }),
      )
    ).status,
  ).toBe(400);
  expect(
    (await list(request("/api/plans/list", { accountId, ownerId: "attacker" })))
      .status,
  ).toBe(400);
  expect(fetch).not.toHaveBeenCalled();
});

it("returns validated stored plans and intents, scrubbing engine errors", async () => {
  const storedPlan = {
    campaignId: planId,
    planId,
    version: 1,
    mode: "clean",
    symbol: "BTCUSDT",
    riskAmount: "2",
    parameters,
    plan,
    createdAt: "2026-01-01T00:00:00Z",
  };
  const fetch = vi.fn().mockResolvedValue(Response.json([storedPlan]));
  vi.stubGlobal("fetch", fetch);
  const response = await list(request("/api/plans/list", { accountId }));
  expect(response.status).toBe(200);
  expect(await response.json()).toEqual([storedPlan]);

  vi.stubGlobal(
    "fetch",
    vi
      .fn()
      .mockResolvedValue(
        Response.json({ error: "private-signed-url" }, { status: 503 }),
      ),
  );
  const failed = await list(request("/api/plans/list", { accountId }));
  expect(failed.status).toBe(503);
  expect(await failed.text()).not.toContain("private-signed-url");
});

it("requires planId for intents and validates their response shape", async () => {
  const intent = {
    id: planId,
    planId,
    clientOrderId: "rt1234",
    exchangeOrderId: null,
    family: "regular",
    purpose: "ENTRY",
    positionSide: "LONG",
    status: "PENDING",
    quantity: "1",
    price: "100",
  };
  const fetch = vi.fn().mockResolvedValue(Response.json([intent]));
  vi.stubGlobal("fetch", fetch);
  expect(
    (await intentsSave(request("/api/plans/intents/save", { accountId })))
      .status,
  ).toBe(400);
  expect(fetch).not.toHaveBeenCalled();
  const response = await intentsList(
    request("/api/plans/intents/list", { accountId, planId }),
  );
  expect(response.status).toBe(200);
  expect(await response.json()).toEqual([intent]);
  // An unexpected envelope is never exposed to the browser.
  vi.stubGlobal(
    "fetch",
    vi.fn().mockResolvedValue(Response.json([{ ...intent, secret: "x" }])),
  );
  expect(
    (
      await intentsList(
        request("/api/plans/intents/list", { accountId, planId }),
      )
    ).status,
  ).toBe(503);
});

it("requires sessions, same origin and PostgreSQL backend", async () => {
  const fetch = vi.fn();
  vi.stubGlobal("fetch", fetch);
  mocks.authenticated.mockResolvedValue(false);
  expect((await list(request("/api/plans/list", { accountId }))).status).toBe(
    401,
  );
  mocks.authenticated.mockResolvedValue(true);
  expect(
    (
      await list(
        request("/api/plans/list", { accountId }, "https://untrusted.example"),
      )
    ).status,
  ).toBe(403);
  mocks.backend.mockReturnValue("local");
  expect((await list(request("/api/plans/list", { accountId }))).status).toBe(
    503,
  );
  expect(fetch).not.toHaveBeenCalled();
});
