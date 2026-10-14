import { describe, expect, it } from "vitest";
import {
  AttemptLimiter,
  configurationReady,
  issueSession,
  sameOrigin,
  SESSION_SECONDS,
  verifyPassword,
  verifySession,
} from "@/lib/security";

const secret = "unit-test-session-secret-only-never-deploy-123";
const password = "unit-test-password-only-123";
describe("private authentication", () => {
  it("fails closed without secrets and with example configuration", () => {
    expect(configurationReady("", "")).toBe(false);
    expect(configurationReady("replace-with-a-password", secret)).toBe(false);
    expect(verifySession("", secret)).toBe(false);
    expect(() => issueSession("short")).toThrow();
  });
  it("verifies only the configured password", () => {
    expect(verifyPassword(password, password, secret)).toBe(true);
    expect(verifyPassword("wrong", password, secret)).toBe(false);
    expect(verifyPassword("x".repeat(257), password, secret)).toBe(false);
  });
  it("validates a signed session", () => {
    expect(verifySession(issueSession(secret, 100_000), secret, 100_001)).toBe(
      true,
    );
  });
  it("rejects modified, malformed and oversized sessions", () => {
    const token = issueSession(secret, 100_000);
    expect(verifySession(`x${token}`, secret, 100_001)).toBe(false);
    expect(verifySession("not.a.token", secret)).toBe(false);
    expect(verifySession("x".repeat(1025), secret)).toBe(false);
  });
  it("rejects expired and future-issued sessions", () => {
    const token = issueSession(secret, 100_000);
    expect(verifySession(token, secret, 100_000 + SESSION_SECONDS * 1000)).toBe(
      false,
    );
    expect(verifySession(token, secret, 99_000)).toBe(false);
  });
  it("invalidates sessions on secret rotation", () => {
    expect(verifySession(issueSession(secret), `${secret}-rotated`)).toBe(
      false,
    );
  });
});
describe("request protection", () => {
  it("requires an exact configured origin", () => {
    expect(
      sameOrigin("https://terminal.example", "https://terminal.example"),
    ).toBe(true);
    for (const origin of [
      null,
      "null",
      "https://attacker.example",
      "https://terminal.example.attacker.test",
      "https://terminal.example/path",
      "http://terminal.example",
    ])
      expect(sameOrigin(origin, "https://terminal.example")).toBe(false);
    expect(sameOrigin("https://terminal.example", undefined)).toBe(false);
    expect(
      sameOrigin("https://terminal.example", "https://terminal.example/path"),
    ).toBe(false);
    expect(
      sameOrigin("http://terminal.example", "http://terminal.example"),
    ).toBe(false);
  });
  it("caps attempts and releases the window", () => {
    const limiter = new AttemptLimiter(2, 1000);
    expect(limiter.allow(0)).toBe(true);
    expect(limiter.allow(10)).toBe(true);
    expect(limiter.allow(20)).toBe(false);
    expect(limiter.allow(1001)).toBe(true);
    limiter.reset();
    expect(limiter.allow(1002)).toBe(true);
  });
});
