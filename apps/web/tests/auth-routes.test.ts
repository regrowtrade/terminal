import { randomBytes } from "node:crypto";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
vi.mock("server-only", () => ({}));
const mocks = vi.hoisted(() => ({
  operation: vi.fn(),
  cookie: { value: "synthetic-cookie" },
}));
vi.mock("next/headers", () => ({
  cookies: async () => ({ get: () => mocks.cookie }),
}));
vi.mock("@/lib/sessions", async (original) => ({
  ...(await original<typeof import("@/lib/sessions")>()),
  sessionOperation: mocks.operation,
}));

vi.mock("@/lib/auth", () => ({
  cookieName: () => "__Host-regrow-session",
  isConfigured: () => true,
}));
import { POST as login } from "@/app/api/auth/login/route";
import { POST as logout } from "@/app/api/auth/logout/route";
import { POST as logoutAll } from "@/app/api/auth/logout-all/route";
import { SessionUnavailable } from "@/lib/sessions";

const publicOrigin = "https://terminal.example";
beforeEach(() => {
  mocks.operation.mockReset().mockResolvedValue(true);
  vi.stubEnv("APP_ORIGIN", publicOrigin);
  vi.stubEnv("NODE_ENV", "production");
  vi.stubEnv("TERMINAL_PASSWORD", randomBytes(48).toString("base64"));
  vi.stubEnv("SESSION_SECRET", randomBytes(48).toString("base64"));
});
afterEach(() => vi.unstubAllEnvs());

function request(
  path: string,
  origin = publicOrigin,
  password = process.env.TERMINAL_PASSWORD,
) {
  return new Request(`http://0.0.0.0:3000${path}`, {
    method: "POST",
    headers: {
      Origin: origin,
      Host: "untrusted.example",
      "X-Forwarded-Host": "untrusted.example",
      "X-Forwarded-Proto": "http",
      "Content-Type": "application/x-www-form-urlencoded",
    },
    body: new URLSearchParams({ password: password ?? "" }),
  });
}

it("redirects login to the configured public origin, not the proxy or host header", async () => {
  const response = await login(request("/api/auth/login"));
  expect(response.status).toBe(303);
  expect(response.headers.get("Location")).toBe(`${publicOrigin}/`);
  expect(response.headers.get("Set-Cookie")).toContain("Secure");
  expect(response.headers.get("Set-Cookie")).toContain("HttpOnly");
  expect(response.headers.get("Set-Cookie")).toContain("SameSite=strict");
});

it("keeps failed login redirects on the configured origin", async () => {
  const response = await login(
    request("/api/auth/login", publicOrigin, "invalid"),
  );
  expect(response.headers.get("Location")).toBe(
    `${publicOrigin}/login?error=invalid`,
  );
  expect(response.headers.has("Set-Cookie")).toBe(false);
});

it("redirects logout to the configured public origin and expires its cookie", async () => {
  const response = await logout(request("/api/auth/logout"));
  expect(response.status).toBe(303);
  expect(response.headers.get("Location")).toBe(`${publicOrigin}/login`);
  expect(response.headers.get("Set-Cookie")).toContain("Max-Age=0");
  expect(mocks.operation).toHaveBeenCalledWith("revoke", mocks.cookie.value);
});

it("does not issue a cookie when persistence fails", async () => {
  mocks.operation.mockRejectedValue(new SessionUnavailable());
  const response = await login(request("/api/auth/login"));
  expect(response.headers.get("Location")).toBe(
    `${publicOrigin}/login?error=unavailable`,
  );
  expect(response.headers.has("Set-Cookie")).toBe(false);
});

it("does not pretend logout succeeded when revocation fails", async () => {
  mocks.operation.mockRejectedValue(new SessionUnavailable());
  const response = await logout(request("/api/auth/logout"));
  expect(response.status).toBe(503);
  expect(response.headers.has("Set-Cookie")).toBe(false);
});

it("revokes all sessions only with a valid current session", async () => {
  expect((await logoutAll(request("/api/auth/logout-all"))).status).toBe(303);
  expect(mocks.operation).toHaveBeenCalledWith(
    "revoke-all",
    mocks.cookie.value,
  );
  mocks.operation.mockResolvedValue(false);
  expect((await logoutAll(request("/api/auth/logout-all"))).status).toBe(401);
});

it("rejects an untrusted origin before login or logout", async () => {
  for (const handler of [login, logout]) {
    const response = await handler(
      request("/api/auth/login", "https://untrusted.example"),
    );
    expect(response.status).toBe(403);
    expect(response.headers.has("Location")).toBe(false);
  }
});

it.each([
  "",
  "https://terminal.example/path",
  "http://terminal.example",
  "https://user@terminal.example",
])("fails closed with an invalid application origin: %s", async (origin) => {
  vi.stubEnv("APP_ORIGIN", origin);
  for (const handler of [login, logout]) {
    const response = await handler(request("/api/auth/login"));
    expect(response.status).toBe(503);
    expect(response.headers.has("Location")).toBe(false);
  }
});
