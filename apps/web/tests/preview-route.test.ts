import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { defaultRequest } from "@/lib/plans";

vi.mock("@/lib/auth", () => ({ authenticated: vi.fn() }));
vi.mock("@/lib/engine", () => ({ previewPlan: vi.fn() }));
import { authenticated } from "@/lib/auth";
import { previewPlan } from "@/lib/engine";
import { POST } from "@/app/api/plans/preview/route";

beforeEach(() => {
  vi.stubEnv("APP_ORIGIN", "http://localhost:3000");
  vi.mocked(authenticated).mockResolvedValue(true);
  vi.mocked(previewPlan).mockClear();
});
afterEach(() => vi.unstubAllEnvs());
const request = (
  body: unknown = defaultRequest,
  origin = "http://localhost:3000",
) =>
  new Request("http://localhost:3000/api/plans/preview", {
    method: "POST",
    headers: { origin, "Content-Type": "application/json" },
    body: JSON.stringify(body),
  });
it("rejects anonymous requests before contacting the engine", async () => {
  vi.mocked(authenticated).mockResolvedValue(false);
  expect((await POST(request())).status).toBe(401);
  expect(previewPlan).not.toHaveBeenCalled();
});
it("rejects cross-origin requests", async () => {
  expect(
    (await POST(request(defaultRequest, "https://attacker.example"))).status,
  ).toBe(403);
  expect(previewPlan).not.toHaveBeenCalled();
});
it("rejects unknown execution flags", async () => {
  expect(
    (await POST(request({ ...defaultRequest, execute: true }))).status,
  ).toBe(422);
  expect(previewPlan).not.toHaveBeenCalled();
});
it("propagates safe engine errors without exposing credentials", async () => {
  vi.mocked(previewPlan).mockResolvedValue({
    ok: false,
    status: 503,
    error: "Motor no disponible.",
  });
  const result = await POST(request());
  expect(result.status).toBe(503);
  expect(result.headers.get("Cache-Control")).toBe("no-store");
  expect(await result.json()).toEqual({ error: "Motor no disponible." });
});
