import "server-only";
import { planSchema, type PlanRequest } from "./plans";

function engineURL() {
  const url = new URL(process.env.ENGINE_URL ?? "http://127.0.0.1:8080");
  if (
    url.username ||
    url.password ||
    url.search ||
    url.hash ||
    url.pathname !== "/"
  )
    throw new Error("Invalid engine configuration");
  if (
    url.protocol !== "https:" &&
    !(
      url.protocol === "http:" &&
      (["127.0.0.1", "localhost", "engine"].includes(url.hostname) ||
        /^[a-z0-9](?:[a-z0-9-]*[a-z0-9])?\.railway\.internal$/.test(
          url.hostname,
        ))
    )
  )
    throw new Error("Engine requires a private network or HTTPS");
  return url;
}
export async function engineStatus(): Promise<"available" | "unavailable"> {
  try {
    const response = await fetch(new URL("/healthz", engineURL()), {
      cache: "no-store",
      redirect: "error",
      signal: AbortSignal.timeout(1500),
    });
    if (!response.ok) return "unavailable";
    const body: unknown = await response.json();
    return typeof body === "object" &&
      body !== null &&
      "mode" in body &&
      body.mode === "simulation"
      ? "available"
      : "unavailable";
  } catch {
    return "unavailable";
  }
}
export async function previewPlan(request: PlanRequest) {
  const token = process.env.ENGINE_API_TOKEN ?? "";
  if (token.length < 32 || token.startsWith("replace-with-"))
    return {
      ok: false as const,
      status: 503,
      error: "Configura el token privado del motor para calcular planes.",
    };
  try {
    const response = await fetch(new URL("/v1/plans/preview", engineURL()), {
      method: "POST",
      redirect: "error",
      headers: {
        "Content-Type": "application/json",
        Authorization: `Bearer ${token}`,
      },
      body: JSON.stringify(request),
      cache: "no-store",
      signal: AbortSignal.timeout(5000),
    });
    if (response.status === 401 || response.status === 403)
      return {
        ok: false as const,
        status: 503,
        error:
          "El motor rechazó la conexión privada. Configura el mismo ENGINE_API_TOKEN en la web y el motor y reinicia ambos. No se envió ninguna orden.",
      };
    if (response.status === 404 || response.status === 405)
      return {
        ok: false as const,
        status: 503,
        error:
          "La dirección configurada no ofrece la vista previa del motor. Comprueba ENGINE_URL y que el puerto no esté ocupado por otro servicio. No se envió ninguna orden.",
      };
    if (response.status === 422) {
      const body: unknown = await response.json();
      const error =
        typeof body === "object" &&
        body !== null &&
        "error" in body &&
        typeof body.error === "string"
          ? body.error
          : "El plan no cumple los límites de riesgo.";
      return { ok: false as const, status: 422, error };
    }
    if (!response.ok)
      return {
        ok: false as const,
        status: 502,
        error: "No se pudo validar el plan con el motor.",
      };
    const data = planSchema.parse(await response.json());
    return { ok: true as const, data };
  } catch {
    return {
      ok: false as const,
      status: 503,
      error:
        "Motor no disponible. Inícialo y vuelve a intentar. No se envió ninguna orden.",
    };
  }
}
