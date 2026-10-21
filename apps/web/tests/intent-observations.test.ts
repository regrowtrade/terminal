import { randomBytes } from "node:crypto";
import { beforeEach, afterEach, expect, it, vi } from "vitest";
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
import { POST as observe } from "@/app/api/plans/intents/observe/route";
import { POST as history } from "@/app/api/plans/intents/observations/route";
import { intentObservationSchema } from "@/lib/intent-observation-schema";
import { sessionDigest } from "@/lib/sessions";

const accountId = "00000000-0000-4000-8000-000000000001";
const intentId = "00000000-0000-4000-8000-000000000002";
const planId = "00000000-0000-4000-8000-000000000003";
const clientOrderId = `rt${"a".repeat(30)}`;
const evidence = () => ({
  id: planId,
  storedAt: "2026-10-21T00:00:02Z",
  report: {
    expected: {
      intentId,
      planId,
      planFingerprint: "b".repeat(64),
      symbol: "BTCUSDT",
      clientOrderId,
      purpose: "ENTRY",
      positionSide: "LONG",
      quantity: "2",
      price: "100",
    },
    observation: {
      source: "binance-production",
      mode: "read_only",
      executionReady: false,
      startedAt: "2026-10-21T00:00:00Z",
      checkedAt: "2026-10-21T00:00:01Z",
      query: { symbol: "BTCUSDT", clientOrderId },
      outcome: "observed",
      reason: "",
      order: {
        orderId: "9007199254740993",
        clientOrderId,
        symbol: "BTCUSDT",
        side: "BUY",
        positionSide: "LONG",
        type: "LIMIT",
        status: "PARTIALLY_FILLED",
        origQty: "2.000",
        executedQty: "0.5",
        price: "100.000",
        avgPrice: "99",
        reduceOnly: false,
        closePosition: false,
        updateTime: 1750514941540,
      },
    },
    assessment: "attributes_match",
    mismatches: [] as string[],
    executionReady: false,
    ownershipConfirmed: false,
  },
});

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
function request(body: unknown, origin = "https://terminal.example") {
  return new Request("https://terminal.example/api/plans/intents/observe", {
    method: "POST",
    headers: { Origin: origin, "Content-Type": "application/json" },
    body: JSON.stringify(body),
  });
}

it("accepts decimal-equivalent attributes without confirming ownership or execution", () => {
  expect(intentObservationSchema.safeParse(evidence()).success).toBe(true);
  const r = evidence();
  r.report.observation.order.positionSide = "BOTH";
  expect(intentObservationSchema.safeParse(r).success).toBe(false);
  r.report.assessment = "mismatch";
  r.report.mismatches = ["positionSide"];
  expect(intentObservationSchema.safeParse(r).success).toBe(true);
  r.report.executionReady = true;
  expect(intentObservationSchema.safeParse(r).success).toBe(false);
  r.report.executionReady = false;
  r.report.ownershipConfirmed = true;
  expect(intentObservationSchema.safeParse(r).success).toBe(false);
  const corrupt = evidence();
  corrupt.report.expected.quantity = "not-a-decimal";
  expect(intentObservationSchema.safeParse(corrupt).success).toBe(false);
  const unresolved = evidence();
  const unresolvedReport = {
    ...unresolved,
    report: {
      ...unresolved.report,
      assessment: "unresolved",
      observation: {
        ...unresolved.report.observation,
        outcome: "unresolved",
        reason: "exchange_not_found",
        order: null,
      },
    },
  };
  expect(intentObservationSchema.safeParse(unresolvedReport).success).toBe(
    true,
  );
  unresolvedReport.report.assessment = "attributes_match";
  expect(intentObservationSchema.safeParse(unresolvedReport).success).toBe(
    false,
  );
});

it("forwards only intent/account IDs and a server-derived session digest", async () => {
  const fetch = vi.fn().mockResolvedValue(Response.json(evidence()));
  vi.stubGlobal("fetch", fetch);
  const response = await observe(request({ accountId, intentId }));
  expect(response.status).toBe(200);
  expect(response.headers.get("cache-control")).toBe("no-store");
  expect(fetch).toHaveBeenCalledWith(
    new URL("http://engine.railway.internal:8080/v1/plans/intents/observe"),
    expect.objectContaining({
      redirect: "error",
      cache: "no-store",
      body: JSON.stringify({
        accountId,
        intentId,
        tokenHash: sessionDigest("synthetic-cookie"),
      }),
    }),
  );
  expect(await response.text()).not.toContain(process.env.ENGINE_API_TOKEN);
});

it("rejects caller selectors, financial attributes and ownership", async () => {
  const fetch = vi.fn();
  vi.stubGlobal("fetch", fetch);
  for (const extra of [
    { symbol: "ETHUSDT" },
    { clientOrderId },
    { orderId: "123" },
    { ownerId: planId },
    { quantity: "5" },
    { tokenHash: "a".repeat(64) },
  ]) {
    expect(
      (await observe(request({ accountId, intentId, ...extra }))).status,
    ).toBe(400);
  }
  expect(fetch).not.toHaveBeenCalled();
});

it("validates bounded historical evidence and never substitutes a failed current read", async () => {
  vi.stubGlobal(
    "fetch",
    vi.fn().mockResolvedValue(Response.json([evidence()])),
  );
  const saved = await history(request({ accountId, intentId }));
  expect(saved.status).toBe(200);
  expect(await saved.json()).toEqual([evidence()]);
  vi.stubGlobal(
    "fetch",
    vi
      .fn()
      .mockResolvedValue(Response.json(Array.from({ length: 21 }, evidence))),
  );
  expect((await history(request({ accountId, intentId }))).status).toBe(503);
  vi.stubGlobal(
    "fetch",
    vi
      .fn()
      .mockResolvedValue(
        Response.json({ error: "private-signed-url" }, { status: 503 }),
      ),
  );
  const failed = await observe(request({ accountId, intentId }));
  expect(failed.status).toBe(503);
  expect(await failed.text()).not.toContain("private-signed-url");
});

it("rejects another intent, tampered assessments and unknown response fields", async () => {
  for (const mutate of [
    (r: ReturnType<typeof evidence>) => {
      r.report.expected.intentId = planId;
    },
    (r: ReturnType<typeof evidence>) => {
      r.report.assessment = "unresolved";
    },
    (r: ReturnType<typeof evidence>) => {
      r.report.observation.query.clientOrderId = "other";
    },
    (r: ReturnType<typeof evidence>) => {
      Object.assign(r, { secret: "x" });
    },
  ]) {
    const r = evidence();
    mutate(r);
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(Response.json(r)));
    expect((await observe(request({ accountId, intentId }))).status).toBe(503);
  }
});

it("requires authentication, same origin and persistent sessions", async () => {
  const fetch = vi.fn();
  vi.stubGlobal("fetch", fetch);
  mocks.authenticated.mockResolvedValue(false);
  expect((await observe(request({ accountId, intentId }))).status).toBe(401);
  mocks.authenticated.mockResolvedValue(true);
  expect(
    (await observe(request({ accountId, intentId }, "https://other.example")))
      .status,
  ).toBe(403);
  mocks.backend.mockReturnValue("local");
  expect((await history(request({ accountId, intentId }))).status).toBe(503);
  expect(fetch).not.toHaveBeenCalled();
});
