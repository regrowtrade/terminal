import {
  createHmac,
  randomBytes,
  scryptSync,
  timingSafeEqual,
} from "node:crypto";

export const SESSION_SECONDS = 8 * 60 * 60;
const sessionPayload = /^[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+$/;
function validSecret(secret: string) {
  return secret.length >= 32 && !secret.startsWith("replace-with-");
}

export function configurationReady(password: string, secret: string) {
  return (
    password.length >= 16 &&
    !password.startsWith("replace-with-") &&
    validSecret(secret)
  );
}

export function verifyPassword(
  input: string,
  password: string,
  secret: string,
): boolean {
  if (!configurationReady(password, secret) || input.length > 256) return false;
  const expected = scryptSync(password, secret, 32);
  const actual = scryptSync(input, secret, 32);
  return timingSafeEqual(expected, actual);
}

export function issueSession(secret: string, now = Date.now()): string {
  if (!validSecret(secret))
    throw new Error("Session configuration unavailable");
  const issued = Math.floor(now / 1000);
  const body = Buffer.from(
    JSON.stringify({
      v: 1,
      role: "owner",
      iat: issued,
      exp: issued + SESSION_SECONDS,
      nonce: randomBytes(16).toString("hex"),
    }),
  ).toString("base64url");
  return `${body}.${createHmac("sha256", secret).update(body).digest("base64url")}`;
}

export function verifySession(
  token: string | undefined,
  secret: string,
  now = Date.now(),
): boolean {
  if (
    !token ||
    token.length > 1024 ||
    !validSecret(secret) ||
    !sessionPayload.test(token)
  )
    return false;
  try {
    const [body, signature] = token.split(".");
    const expected = createHmac("sha256", secret).update(body).digest();
    const supplied = Buffer.from(signature, "base64url");
    if (
      expected.length !== supplied.length ||
      !timingSafeEqual(expected, supplied)
    )
      return false;
    const data: unknown = JSON.parse(Buffer.from(body, "base64url").toString());
    if (typeof data !== "object" || data === null) return false;
    const p = data as Record<string, unknown>;
    const timestamp = Math.floor(now / 1000);
    return (
      p.v === 1 &&
      p.role === "owner" &&
      typeof p.nonce === "string" &&
      /^[a-f0-9]{32}$/.test(p.nonce) &&
      typeof p.iat === "number" &&
      Number.isInteger(p.iat) &&
      p.iat <= timestamp &&
      typeof p.exp === "number" &&
      Number.isInteger(p.exp) &&
      p.exp > timestamp &&
      p.exp - p.iat === SESSION_SECONDS
    );
  } catch {
    return false;
  }
}

// Single-instance throttling only. Replace with shared storage before horizontal scaling.
export class AttemptLimiter {
  private timestamps: number[] = [];
  constructor(
    private readonly maximum = 5,
    private readonly windowMs = 60_000,
  ) {}
  allow(now = Date.now()) {
    this.timestamps = this.timestamps.filter((t) => now - t < this.windowMs);
    if (this.timestamps.length >= this.maximum) return false;
    this.timestamps.push(now);
    return true;
  }
  reset() {
    this.timestamps = [];
  }
}

export function applicationOrigin(expected: string | undefined): URL | null {
  if (!expected) return null;
  try {
    const url = new URL(expected);
    if (expected !== url.origin) return null;
    if (
      url.protocol !== "https:" &&
      !(
        url.protocol === "http:" &&
        ["localhost", "127.0.0.1", "[::1]"].includes(url.hostname)
      )
    )
      return null;
    return url;
  } catch {
    return null;
  }
}

export function sameOrigin(
  origin: string | null,
  expected: string | undefined,
): boolean {
  const configured = applicationOrigin(expected);
  return configured !== null && origin === configured.origin;
}
