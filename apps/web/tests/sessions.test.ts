import { randomBytes } from "node:crypto";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
vi.mock("server-only", () => ({}));
import { issueSession } from "@/lib/security";
import {
  sessionBackend,
  sessionDigest,
  sessionOperation,
} from "@/lib/sessions";

beforeEach(() => {
  vi.stubEnv("NODE_ENV", "production");
  vi.stubEnv("SESSION_BACKEND", "postgres");
  vi.stubEnv("SESSION_SECRET", randomBytes(48).toString("base64"));
  vi.stubEnv("TERMINAL_PASSWORD", randomBytes(48).toString("base64"));
  vi.stubEnv("ENGINE_API_TOKEN", randomBytes(48).toString("base64"));
  vi.stubEnv("ENGINE_URL", "http://engine.railway.internal:8080");
});
afterEach(() => {
  vi.unstubAllGlobals();
  vi.unstubAllEnvs();
});

function cookie() {
  return issueSession(process.env.SESSION_SECRET!);
}
function identity() {
  return {
    ownerId: "00000000-0000-4000-8000-000000000001",
    role: "owner",
    expiresAt: new Date(Date.now() + 60_000).toISOString(),
  };
}

it("sends only a digest to the private engine and disables caching and redirects", async () => {
  const fetch = vi.fn().mockResolvedValue(Response.json(identity()));
  vi.stubGlobal("fetch", fetch);
  const token = cookie();
  expect(await sessionOperation("create", token)).toBe(true);
  const [url, init] = fetch.mock.calls[0];
  expect(String(url)).toBe(
    "http://engine.railway.internal:8080/v1/auth/sessions/create",
  );
  expect(init).toMatchObject({
    cache: "no-store",
    redirect: "error",
    body: JSON.stringify({ tokenHash: sessionDigest(token) }),
  });
  expect(init.body).not.toContain(token);
  expect(init.body).not.toContain(process.env.TERMINAL_PASSWORD);
});

it("rejects forged and old stateless cookies without contacting the engine", async () => {
  const fetch = vi.fn();
  vi.stubGlobal("fetch", fetch);
  expect(await sessionOperation("resolve", "forged")).toBe(false);
  expect(fetch).not.toHaveBeenCalled();
  fetch.mockResolvedValue(new Response(null, { status: 401 }));
  expect(await sessionOperation("resolve", cookie())).toBe(false);
});

it.each([500, 503, 404, 302])(
  "fails closed without local fallback on engine status %s",
  async (status) => {
    const fetch = vi.fn().mockResolvedValue(new Response(null, { status }));
    vi.stubGlobal("fetch", fetch);
    await expect(sessionOperation("resolve", cookie())).rejects.toThrow(
      "sesión",
    );
    expect(fetch).toHaveBeenCalledTimes(1);
  },
);

it("does not allow a local session backend in production", async () => {
  vi.stubEnv("SESSION_BACKEND", "local");
  expect(sessionBackend()).toBeNull();
  await expect(sessionOperation("create", cookie())).rejects.toThrow();
});

it("credential rotation changes the lookup digest", () => {
  const token = cookie();
  const digest = sessionDigest(token);
  vi.stubEnv("TERMINAL_PASSWORD", randomBytes(48).toString("base64"));
  expect(sessionDigest(token)).not.toBe(digest);
});

it("rejects malformed or expired identity responses", async () => {
  for (const body of [
    { ...identity(), role: "admin" },
    { ...identity(), expiresAt: new Date(0).toISOString() },
    { ...identity(), token: "never-return-this" },
  ]) {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(Response.json(body)));
    try {
      expect(await sessionOperation("resolve", cookie())).toBe(false);
    } catch (error) {
      expect(error).toBeInstanceOf(Error);
    }
  }
});

it("development sessions are revocable, not stateless", async () => {
  vi.stubEnv("NODE_ENV", "development");
  vi.stubEnv("SESSION_BACKEND", "local");
  const first = cookie(),
    second = cookie();
  expect(await sessionOperation("resolve", first)).toBe(false);
  expect(await sessionOperation("create", first)).toBe(true);
  expect(await sessionOperation("create", second)).toBe(true);
  expect(await sessionOperation("revoke", first)).toBe(true);
  expect(await sessionOperation("resolve", first)).toBe(false);
  expect(await sessionOperation("resolve", second)).toBe(true);
  expect(await sessionOperation("revoke-all", first)).toBe(false);
  expect(await sessionOperation("revoke-all", second)).toBe(true);
  expect(await sessionOperation("resolve", second)).toBe(false);
});
