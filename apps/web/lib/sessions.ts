import "server-only";
import { createHmac } from "node:crypto";
import { z } from "zod";
import { engineURL } from "./engine";
import { SESSION_SECONDS, verifySession } from "./security";

export function sessionBackend(): "postgres" | "local" | null {
  const configured =
    process.env.SESSION_BACKEND ??
    (process.env.NODE_ENV === "production" ? "postgres" : "local");
  if (configured === "postgres") return "postgres";
  if (configured === "local" && process.env.NODE_ENV !== "production")
    return "local";
  return null;
}

const identitySchema = z
  .object({
    ownerId: z.uuid(),
    role: z.literal("owner"),
    expiresAt: z.iso.datetime({ offset: true }),
  })
  .strict();
// Only a credential-bound digest leaves the web server. Rotation of either the
// password or secret makes previous cookies unusable, including after restart.
export function sessionDigest(cookie: string) {
  return createHmac("sha256", process.env.SESSION_SECRET ?? "")
    .update(JSON.stringify([process.env.TERMINAL_PASSWORD ?? "", cookie]))
    .digest("hex");
}

// Development route bundles/HMR can evaluate this module more than once.
// Share only digest state inside this process, never between installations.
const developmentState = globalThis as typeof globalThis & {
  regrowLocalSessions?: Map<string, number>;
};
const local = (developmentState.regrowLocalSessions ??= new Map<
  string,
  number
>());
export class SessionUnavailable extends Error {
  constructor() {
    super("No se pudo verificar o actualizar la sesión. Inténtalo de nuevo.");
  }
}

export async function sessionOperation(
  operation: "create" | "resolve" | "revoke" | "revoke-all",
  cookie: string,
): Promise<boolean> {
  if (!verifySession(cookie, process.env.SESSION_SECRET ?? "")) return false;
  const backend = sessionBackend();
  if (!backend) throw new SessionUnavailable();
  const hash = sessionDigest(cookie);
  if (backend === "local") {
    for (const [key, expiry] of local)
      if (expiry <= Date.now()) local.delete(key);
    if (operation === "create") {
      if (local.size >= 20) throw new SessionUnavailable();
      local.set(hash, Date.now() + SESSION_SECONDS * 1000);
      return true;
    }
    if (operation === "revoke") {
      local.delete(hash);
      return true;
    }
    if (!local.has(hash)) return false;
    if (operation === "revoke-all") local.clear();
    return true;
  }
  const token = process.env.ENGINE_API_TOKEN ?? "";
  if (token.length < 32 || token.startsWith("replace-with-"))
    throw new SessionUnavailable();
  try {
    const response = await fetch(
      new URL(`/v1/auth/sessions/${operation}`, engineURL()),
      {
        method: "POST",
        redirect: "error",
        cache: "no-store",
        signal: AbortSignal.timeout(5000),
        headers: {
          "Content-Type": "application/json",
          Authorization: `Bearer ${token}`,
        },
        body: JSON.stringify({ tokenHash: hash }),
      },
    );
    if (response.status === 401 && operation !== "create") return false;
    if (!response.ok) throw new SessionUnavailable();
    const body: unknown = await response.json();
    if (operation === "create" || operation === "resolve") {
      const identity = identitySchema.parse(body);
      if (Date.parse(identity.expiresAt) <= Date.now()) return false;
    } else {
      z.object({ ok: z.literal(true) })
        .strict()
        .parse(body);
    }
    return true;
  } catch {
    throw new SessionUnavailable();
  }
}
