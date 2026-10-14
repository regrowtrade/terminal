import { authenticated } from "@/lib/auth";
import { previewPlan } from "@/lib/engine";
import { planRequestSchema } from "@/lib/plans";
import { boundedText } from "@/lib/request";
import { AttemptLimiter, sameOrigin } from "@/lib/security";

const limiter = new AttemptLimiter(60, 60_000);
function json(body: unknown, status: number) {
  return Response.json(body, {
    status,
    headers: { "Cache-Control": "no-store" },
  });
}
export async function POST(request: Request) {
  if (!(await authenticated()))
    return json({ error: "Sesión requerida." }, 401);
  if (!sameOrigin(request.headers.get("origin"), process.env.APP_ORIGIN))
    return json({ error: "Origen no autorizado." }, 403);
  if (!limiter.allow())
    return json({ error: "Demasiados cálculos. Espera un minuto." }, 429);
  if (request.headers.get("content-type") !== "application/json")
    return json({ error: "Formato inválido." }, 415);
  let input: unknown;
  try {
    input = JSON.parse(await boundedText(request));
  } catch {
    return json({ error: "Solicitud inválida o demasiado grande." }, 400);
  }
  const validated = planRequestSchema.safeParse(input);
  if (!validated.success)
    return json(
      {
        error: "Revisa los campos del plan.",
        issues: validated.error.issues.map((i) => ({
          field: i.path.join("."),
          message: i.message,
        })),
      },
      422,
    );
  const result = await previewPlan(validated.data);
  return result.ok
    ? json(result.data, 200)
    : json({ error: result.error }, result.status);
}
