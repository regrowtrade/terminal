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
import { POST } from "@/app/api/connections/binance/comparison/route";
import { sessionDigest } from "@/lib/sessions";
import { syntheticComparison } from "./fixtures/comparison";

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
    "https://terminal.example/api/connections/binance/comparison",
    {
      method: "POST",
      headers: { Origin: origin, "Content-Type": "application/json" },
      body: JSON.stringify(body),
    },
  );
}
it("uses only server-derived identity and the fixed historical comparison endpoint", async () => {
  const fetch = vi.fn().mockResolvedValue(Response.json(syntheticComparison()));
  vi.stubGlobal("fetch", fetch);
  const response = await POST(request());
  expect(response.status).toBe(200);
  expect(response.headers.get("cache-control")).toBe("no-store");
  expect(await response.json()).toEqual(syntheticComparison());
  expect(fetch).toHaveBeenCalledWith(
    new URL(
      "http://engine.railway.internal:8080/v1/connections/binance/comparison",
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
it("rejects anonymous, cross-origin and local-session access before the engine", async () => {
  const fetch = vi.fn();
  vi.stubGlobal("fetch", fetch);
  mocks.authenticated.mockResolvedValue(false);
  expect((await POST(request())).status).toBe(401);
  mocks.authenticated.mockResolvedValue(true);
  expect(
    (await POST(request({ accountId }, "https://untrusted.example"))).status,
  ).toBe(403);
  mocks.backend.mockReturnValue("local");
  expect((await POST(request())).status).toBe(503);
  expect(fetch).not.toHaveBeenCalled();
});
it.each([
  { ownerId: "attacker" },
  { previousSnapshot: {} },
  { apiKey: "forbidden" },
  { url: "https://untrusted.example" },
])(
  "rejects caller-supplied baselines, credentials and owner injection",
  async (extra) => {
    const fetch = vi.fn();
    vi.stubGlobal("fetch", fetch);
    expect((await POST(request({ accountId, ...extra }))).status).toBe(400);
    expect(fetch).not.toHaveBeenCalled();
  },
);
it("keeps missing legacy comparisons explicit and does not fetch another operation", async () => {
  const fetch = vi
    .fn()
    .mockResolvedValue(
      Response.json({ error: "private-driver-details" }, { status: 404 }),
    );
  vi.stubGlobal("fetch", fetch);
  const response = await POST(request());
  expect(response.status).toBe(404);
  expect(await response.text()).not.toContain("private-driver-details");
  expect(fetch).toHaveBeenCalledTimes(1);
});
it("rejects private or overclaiming fields instead of forwarding the engine payload", async () => {
  const body = syntheticComparison();
  const fetch = vi.fn().mockResolvedValue(
    Response.json({
      ...body,
      report: {
        ...body.report,
        changes: [
          {
            ...body.report.changes[0],
            fields: [{ field: "apiKey", before: null, after: "must-not-leak" }],
          },
        ],
      },
    }),
  );
  vi.stubGlobal("fetch", fetch);
  const response = await POST(request());
  expect(response.status).toBe(503);
  expect(await response.text()).not.toContain("must-not-leak");
});
