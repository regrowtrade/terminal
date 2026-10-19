import "server-only";
import { cookies } from "next/headers";
import { z } from "zod";
import { authenticated, cookieName } from "./auth";
import { engineURL } from "./engine";
import { planRequestSchema, planSchema } from "./plans";
import { boundedText } from "./request";
import { sameOrigin, AttemptLimiter } from "./security";
import { sessionBackend, sessionDigest } from "./sessions";

const limiter = new AttemptLimiter(20, 60_000);

export const savedPlanSchema = z
  .strictObject({
    campaignId: z.uuid(),
    planId: z.uuid(),
    version: z.number().int().positive(),
  })
  .strict();

const riskAmount = z.string().regex(/^-?[0-9]{1,80}(?:\.[0-9]{1,200})?$/);
export const storedPlanSchema = z
  .strictObject({
    campaignId: z.uuid(),
    planId: z.uuid(),
    version: z.number().int().positive(),
    mode: z.enum(["clean", "dca", "hedge"]),
    symbol: z.string(),
    riskAmount,
    parameters: planRequestSchema,
    plan: planSchema,
    createdAt: z.iso.datetime({ offset: true }),
  })
  .strict();

export const storedIntentSchema = z
  .strictObject({
    id: z.uuid(),
    planId: z.uuid(),
    clientOrderId: z.string().min(1),
    exchangeOrderId: z.string().nullable(),
    family: z.enum(["regular", "conditional"]),
    purpose: z.enum([
      "ENTRY",
      "REBUY",
      "TP",
      "SL",
      "HEDGE_CLOSE",
      "EMERGENCY_CLOSE",
    ]),
    positionSide: z.enum(["LONG", "SHORT"]),
    status: z.string().min(1),
    quantity: z.string(),
    price: z.string().nullable(),
  })
  .strict();

const intentsCountSchema = z.strictObject({
  count: z.number().int().nonnegative(),
});

type Operation = "save" | "list" | "intents/save" | "intents/list";
const requestSchemas = {
  save: z.strictObject({ accountId: z.uuid(), plan: planRequestSchema }),
  list: z.strictObject({ accountId: z.uuid() }),
  "intents/save": z.strictObject({ accountId: z.uuid(), planId: z.uuid() }),
  "intents/list": z.strictObject({ accountId: z.uuid(), planId: z.uuid() }),
};
const responseSchemas = {
  save: savedPlanSchema,
  list: z.array(storedPlanSchema),
  "intents/save": intentsCountSchema,
  "intents/list": z.array(storedIntentSchema),
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
        404: "Cuenta o plan no disponible.",
        422: "El plan no es válido; no se guardó.",
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
    return json(responseSchemas[operation].parse(data), 200);
  } catch {
    return json(
      { error: "No se pudo guardar o leer el plan. No se enviaron órdenes." },
      503,
    );
  }
}
