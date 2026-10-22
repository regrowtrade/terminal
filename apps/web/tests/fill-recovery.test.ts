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
import { fillRecoverySchema } from "@/lib/fill-recovery-schema";
import { sessionDigest } from "@/lib/sessions";

const accountId = "00000000-0000-4000-8000-000000000001";
const valid = () => ({
  accountId,
  symbol: "BTCUSDT",
  startTime: 1700000000000,
  endTime: 1700000086400,
});
const summary = () => ({
  source: "binance-production",
  mode: "read_only",
  executionReady: false,
  symbol: "BTCUSDT",
  startTime: 1700000000000,
  endTime: 1700000086400,
  recovered: 3,
  duplicates: 1,
  truncated: false,
});

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
  return new Request(
    "https://terminal.example/api/connections/binance/fills/recover",
    {
      method: "POST",
      headers: { Origin: origin, "Content-Type": "application/json" },
      body: JSON.stringify(body),
    },
  );
}
async function post(body: unknown, origin?: string) {
  const { POST: isolated } =
    await import("@/app/api/connections/binance/fills/recover/route");
  return isolated(request(body, origin));
}

it("forwards only the bounded window and a server-derived digest", async () => {
  const fetch = vi.fn().mockResolvedValue(Response.json(summary()));
  vi.stubGlobal("fetch", fetch);
  const response = await post(valid());
  expect(response.status).toBe(200);
  expect(response.headers.get("cache-control")).toBe("no-store");
  expect(await response.json()).toEqual(summary());
  expect(fetch).toHaveBeenCalledWith(
    new URL(
      "http://engine.railway.internal:8080/v1/connections/binance/fills/recover",
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
});

it("rejects invalid selectors and caller ownership without contacting the engine", async () => {
  const fetch = vi.fn();
  vi.stubGlobal("fetch", fetch);
  for (const query of [
    { ...valid(), symbol: "BTCUSDC" },
    { ...valid(), startTime: 0 },
    { ...valid(), startTime: 1700000086400, endTime: 1700000000000 },
    { ...valid(), startTime: 1, endTime: 1 + 7 * 24 * 60 * 60 * 1000 + 1 },
    { ...valid(), endTime: 9999999999999999 },
    { ...valid(), ownerId: "attacker" },
    { ...valid(), tokenHash: "a".repeat(64) },
  ]) {
    expect((await post(query)).status).toBe(400);
  }
  expect(fetch).not.toHaveBeenCalled();
});

it("never exposes mismatched or unsafe responses", async () => {
  for (const mutate of [
    (r: ReturnType<typeof summary>) => {
      r.executionReady = true;
    },
    (r: ReturnType<typeof summary>) => {
      r.recovered = -1;
    },
    (r: ReturnType<typeof summary>) => {
      r.symbol = "BTCUSDC";
    },
    (r: ReturnType<typeof summary>) => {
      r.symbol = "ETHUSDT";
    },
    (r: ReturnType<typeof summary>) => {
      r.startTime++;
    },
    (r: ReturnType<typeof summary>) => {
      r.endTime++;
    },
    (r: ReturnType<typeof summary>) => {
      r.recovered = 20001;
    },
    (r: ReturnType<typeof summary>) => {
      r.duplicates = 20000;
    },
    (r: ReturnType<typeof summary>) => {
      Object.assign(r, { secret: "x" });
    },
  ]) {
    const body = summary();
    mutate(body);
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(Response.json(body)));
    expect((await post(valid())).status).toBe(503);
  }
  vi.stubGlobal(
    "fetch",
    vi
      .fn()
      .mockResolvedValue(
        Response.json({ error: "private-signed-url" }, { status: 503 }),
      ),
  );
  const failed = await post(valid());
  expect(failed.status).toBe(503);
  expect(await failed.text()).not.toContain("private-signed-url");
});

it("requires authentication, same origin and persistent sessions", async () => {
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

it("validates the recovery summary shape", () => {
  expect(fillRecoverySchema.safeParse(summary()).success).toBe(true);
  expect(
    fillRecoverySchema.safeParse({ ...summary(), executionReady: true })
      .success,
  ).toBe(false);
  expect(
    fillRecoverySchema.safeParse({ ...summary(), truncated: "yes" }).success,
  ).toBe(false);
  expect(
    fillRecoverySchema.safeParse({ ...summary(), endTime: 1 }).success,
  ).toBe(false);
  expect(
    fillRecoverySchema.safeParse({
      ...summary(),
      endTime: summary().startTime + 7 * 24 * 60 * 60 * 1000 + 1,
    }).success,
  ).toBe(false);
});

it("returns an authenticated zero-result recovery without inferring absence", async () => {
  const body = { ...summary(), recovered: 0, duplicates: 0 };
  vi.stubGlobal("fetch", vi.fn().mockResolvedValue(Response.json(body)));
  const response = await post(valid());
  expect(response.status).toBe(200);
  expect(await response.json()).toEqual(body);
});
