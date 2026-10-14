import { randomBytes } from "node:crypto";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { defaultRequest } from "@/lib/plans";

vi.mock("server-only", () => ({}));
import { engineStatus, previewPlan } from "@/lib/engine";

beforeEach(() => {
  vi.stubEnv("ENGINE_API_TOKEN", randomBytes(48).toString("base64"));
  vi.stubEnv("ENGINE_URL", "http://127.0.0.1:8082");
});
afterEach(() => {
  vi.unstubAllEnvs();
  vi.unstubAllGlobals();
});

it.each([404, 405])(
  "identifies a wrong service or endpoint (%s)",
  async (status) => {
    vi.stubGlobal(
      "fetch",
      vi
        .fn()
        .mockResolvedValue(new Response("<html>Apache</html>", { status })),
    );
    const result = await previewPlan(defaultRequest);
    expect(result).toMatchObject({ ok: false, status: 503 });
    expect(result).toHaveProperty(
      "error",
      expect.stringContaining("ENGINE_URL"),
    );
    expect(result).toHaveProperty(
      "error",
      expect.stringContaining("otro servicio"),
    );
    expect(JSON.stringify(result)).not.toContain(process.env.ENGINE_API_TOKEN);
    expect(JSON.stringify(result)).not.toContain("<html>");
  },
);

it.each([401, 403])(
  "identifies private authentication failures (%s)",
  async (status) => {
    vi.stubGlobal(
      "fetch",
      vi
        .fn()
        .mockResolvedValue(
          Response.json({ error: "No autorizado" }, { status }),
        ),
    );
    const result = await previewPlan(defaultRequest);
    expect(result).toMatchObject({ ok: false, status: 503 });
    expect(result).toHaveProperty(
      "error",
      expect.stringContaining("mismo ENGINE_API_TOKEN"),
    );
    expect(JSON.stringify(result)).not.toContain(process.env.ENGINE_API_TOKEN);
  },
);

it("does not treat an Apache health response as an available engine", async () => {
  vi.stubGlobal(
    "fetch",
    vi
      .fn()
      .mockResolvedValue(
        new Response("<html>Not Found</html>", { status: 404 }),
      ),
  );
  expect(await engineStatus()).toBe("unavailable");
});

it("keeps strategy validation errors from the engine", async () => {
  vi.stubGlobal(
    "fetch",
    vi
      .fn()
      .mockResolvedValue(
        Response.json({ error: "Riesgo insuficiente" }, { status: 422 }),
      ),
  );
  expect(await previewPlan(defaultRequest)).toEqual({
    ok: false,
    status: 422,
    error: "Riesgo insuficiente",
  });
});

it("does not contact the engine without a configured private token", async () => {
  vi.stubEnv("ENGINE_API_TOKEN", "");
  const fetch = vi.fn();
  vi.stubGlobal("fetch", fetch);
  expect(await previewPlan(defaultRequest)).toMatchObject({
    ok: false,
    status: 503,
  });
  expect(fetch).not.toHaveBeenCalled();
});

it("contacts an exact Railway private service without exposing its token in results", async () => {
  vi.stubEnv("ENGINE_URL", "http://engine.railway.internal:8080");
  const fetch = vi
    .fn()
    .mockResolvedValue(
      Response.json({ error: "Synthetic risk error" }, { status: 422 }),
    );
  vi.stubGlobal("fetch", fetch);
  expect(await previewPlan(defaultRequest)).toMatchObject({
    ok: false,
    status: 422,
  });
  expect(fetch).toHaveBeenCalledWith(
    new URL("http://engine.railway.internal:8080/v1/plans/preview"),
    expect.objectContaining({ redirect: "error" }),
  );
});

it.each([
  "http://engine.railway.internal.attacker.example:8080",
  "http://railway.internal:8080",
  "http://engine.railway.internal:8080/untrusted",
  "http://engine.railway.internal:8080?untrusted=true",
  "http://example.com:8080",
])("rejects an unsafe engine origin: %s", async (url) => {
  vi.stubEnv("ENGINE_URL", url);
  const fetch = vi.fn();
  vi.stubGlobal("fetch", fetch);
  expect(await previewPlan(defaultRequest)).toMatchObject({
    ok: false,
    status: 503,
  });
  expect(fetch).not.toHaveBeenCalled();
});
