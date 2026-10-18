import "server-only";
import { cookies } from "next/headers";
import { z } from "zod";
import { authenticated, cookieName } from "./auth";
import { engineURL } from "./engine";
import {
  connectionStateSchema,
  snapshotSchema,
  storedSnapshotSchema,
} from "./connections-schema";
import { eventsResponseSchema } from "./events-schema";
import { boundedText } from "./request";
import { storedComparisonSchema } from "./observation-schema";
import { sameOrigin, AttemptLimiter } from "./security";
import { sessionBackend, sessionDigest } from "./sessions";

const limiter = new AttemptLimiter(12, 60_000);
const responseSchemas = {
  list: connectionStateSchema,
  snapshot: snapshotSchema,
  latest: storedSnapshotSchema,
  comparison: storedComparisonSchema,
  events: eventsResponseSchema,
};
function json(body: unknown, status: number) {
  return Response.json(body, {
    status,
    headers: { "Cache-Control": "no-store" },
  });
}
export async function connectionRequest(
  request: Request,
  operation: "list" | "snapshot" | "latest" | "comparison" | "events",
) {
  if (!(await authenticated()))
    return json({ error: "Sesión requerida." }, 401);
  if (!sameOrigin(request.headers.get("origin"), process.env.APP_ORIGIN))
    return json({ error: "Origen no autorizado." }, 403);
  if (sessionBackend() !== "postgres")
    return json(
      {
        error:
          "Las conexiones reales requieren sesiones PostgreSQL. No hay claves ni conexión en el modo local.",
      },
      503,
    );
  if (!limiter.allow())
    return json({ error: "Demasiadas consultas. Espera un minuto." }, 429);
  if (request.headers.get("content-type") !== "application/json")
    return json({ error: "Formato inválido." }, 415);
  let input: unknown;
  try {
    input = JSON.parse(await boundedText(request, 1024));
  } catch {
    return json({ error: "Solicitud inválida." }, 400);
  }
  const parsed = (
    operation === "list"
      ? z.object({}).strict()
      : operation === "events"
        ? z
            .object({
              accountId: z.uuid(),
              after: z
                .number()
                .int()
                .nonnegative()
                .max(Number.MAX_SAFE_INTEGER)
                .optional(),
            })
            .strict()
        : z.object({ accountId: z.uuid() }).strict()
  ).safeParse(input);
  if (!parsed.success) return json({ error: "Solicitud inválida." }, 400);
  const cookie = (await cookies()).get(cookieName())?.value;
  if (!cookie) return json({ error: "Sesión requerida." }, 401);
  const token = process.env.ENGINE_API_TOKEN ?? "";
  if (token.length < 32 || token.startsWith("replace-with-"))
    return json({ error: "Configura la conexión privada del motor." }, 503);
  try {
    const response = await fetch(
      new URL(`/v1/connections/binance/${operation}`, engineURL()),
      {
        method: "POST",
        redirect: "error",
        cache: "no-store",
        signal: AbortSignal.timeout(20_000),
        headers: {
          "Content-Type": "application/json",
          Authorization: `Bearer ${token}`,
        },
        body: JSON.stringify({
          ...parsed.data,
          tokenHash: sessionDigest(cookie),
        }),
      },
    );
    if (!response.ok) {
      const errors: Record<number, string> = {
        401: "Sesión no válida. Vuelve a iniciar sesión.",
        404:
          operation === "latest"
            ? "Cuenta o lectura guardada no disponible."
            : operation === "comparison"
              ? "Cuenta o comparación guardada no disponible."
              : "Cuenta no disponible.",
        422: "La clave no cumple los permisos de lectura e IP requeridos. Revisa los permisos en Binance; no actives trading.",
        429: "Binance limitó temporalmente las consultas. Espera antes de actualizar.",
        503: "Lectura no disponible. Revisa PostgreSQL, cifrado y habilitación del motor; no se enviaron órdenes.",
      };
      const status = response.status in errors ? response.status : 503;
      return json({ error: errors[status] }, status);
    }
    const body: unknown = JSON.parse(
      await boundedText(response, 8 * 1024 * 1024),
    );
    return json(responseSchemas[operation].parse(body), 200);
  } catch {
    return json(
      {
        error:
          "No se pudo leer la cuenta de forma segura. No se enviaron órdenes.",
      },
      503,
    );
  }
}
