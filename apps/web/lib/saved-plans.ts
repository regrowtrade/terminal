import "server-only";
import { cookies } from "next/headers";
import { z } from "zod";
import { authenticated, cookieName } from "./auth";
import { engineURL } from "./engine";
import { planRequestSchema } from "./plans";
import { intentObservationSchema } from "./intent-observation-schema";
import { boundedText } from "./request";
import {
  savedPlanSchema,
  storedPlanSchema,
  storedIntentSchema,
} from "./saved-plans-schema";
import { sameOrigin, AttemptLimiter } from "./security";
import { sessionBackend, sessionDigest } from "./sessions";

const limiter = new AttemptLimiter(20, 60_000);

const intentsCountSchema = z.strictObject({
  count: z.number().int().nonnegative(),
});

type Operation =
  | "save"
  | "list"
  | "intents/save"
  | "intents/list"
  | "intents/observe"
  | "intents/observations";
const requestSchemas = {
  save: z.strictObject({ accountId: z.uuid(), plan: planRequestSchema }),
  list: z.strictObject({ accountId: z.uuid() }),
  "intents/save": z.strictObject({ accountId: z.uuid(), planId: z.uuid() }),
  "intents/list": z.strictObject({ accountId: z.uuid(), planId: z.uuid() }),
  "intents/observe": z.strictObject({
    accountId: z.uuid(),
    intentId: z.uuid(),
  }),
  "intents/observations": z.strictObject({
    accountId: z.uuid(),
    intentId: z.uuid(),
  }),
};
const responseSchemas = {
  save: savedPlanSchema,
  list: z.array(storedPlanSchema),
  "intents/save": intentsCountSchema,
  "intents/list": z.array(storedIntentSchema),
  "intents/observe": intentObservationSchema,
  "intents/observations": z.array(intentObservationSchema).max(20),
};

function json(body: unknown, status: number) {
  return Response.json(body, {
    status,
    headers: { "Cache-Control": "no-store" },
  });
}

export async function plansRequest(request: Request, operation: Operation) {
  if (!(await authenticated()))
    return json({ error: "Sesión requerida." }, 401);
  if (!sameOrigin(request.headers.get("origin"), process.env.APP_ORIGIN))
    return json({ error: "Origen no autorizado." }, 403);
  if (sessionBackend() !== "postgres")
    return json(
      {
        error:
          "Los planes requieren sesiones PostgreSQL. No se enviaron órdenes.",
      },
      503,
    );
  if (!limiter.allow())
    return json({ error: "Demasiadas consultas. Espera un minuto." }, 429);
  if (request.headers.get("content-type") !== "application/json")
    return json({ error: "Formato inválido." }, 415);
  let input: unknown;
  try {
    input = JSON.parse(await boundedText(request, 16384));
  } catch {
    return json({ error: "Solicitud inválida." }, 400);
  }
  const parsed = requestSchemas[operation].safeParse(input);
  if (!parsed.success) return json({ error: "Solicitud inválida." }, 400);
  const cookie = (await cookies()).get(cookieName())?.value;
  if (!cookie) return json({ error: "Sesión requerida." }, 401);
  const token = process.env.ENGINE_API_TOKEN ?? "";
  if (token.length < 32 || token.startsWith("replace-with-"))
    return json({ error: "Configura la conexión privada del motor." }, 503);
  try {
    const saveData = parsed.data as {
      accountId: string;
      plan?: Record<string, unknown>;
    };
    const body =
      operation === "save"
        ? { ...saveData.plan, accountId: saveData.accountId }
        : parsed.data;
    const response = await fetch(
      new URL(`/v1/plans/${operation}`, engineURL()),
      {
        method: "POST",
        redirect: "error",
        cache: "no-store",
        signal: AbortSignal.timeout(20_000),
        headers: {
          "Content-Type": "application/json",
          Authorization: `Bearer ${token}`,
        },
        body: JSON.stringify({ ...body, tokenHash: sessionDigest(cookie) }),
      },
    );
    if (!response.ok) {
      const errors: Record<number, string> = {
        401: "Sesión no válida. Vuelve a iniciar sesión.",
        404: "Cuenta, plan o intención no disponible.",
        409: "La intención o su plan cambió durante la consulta.",
        422: "Plan o intención no compatible, o permisos de lectura no válidos.",
        429: "Demasiadas consultas. Espera antes de reintentar.",
        503: "No se pudo guardar o leer el plan. No se enviaron órdenes.",
      };
      return json(
        { error: errors[response.status] ?? errors[503] },
        response.status in errors ? response.status : 503,
      );
    }
    const bodyText = await boundedText(response, 4 * 1024 * 1024);
    const data: unknown = JSON.parse(bodyText);
    const validated = responseSchemas[operation].parse(data);
    if (
      operation === "intents/observe" ||
      operation === "intents/observations"
    ) {
      const intentId = requestSchemas["intents/observe"]
        .parse(parsed.data)
        .intentId.toLowerCase();
      const observations = Array.isArray(validated) ? validated : [validated];
      if (
        observations.some(
          (item) =>
            !("report" in item) || item.report.expected.intentId !== intentId,
        )
      )
        throw new Error("Unexpected intent evidence");
    }
    return json(validated, 200);
  } catch {
    return json(
      { error: "No se pudo guardar o leer el plan. No se enviaron órdenes." },
      503,
    );
  }
}
