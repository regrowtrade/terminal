import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import { ObservationComparison } from "@/components/observation-comparison";
import { BinanceConnection } from "@/components/binance-connection";
import { storedComparisonSchema } from "@/lib/observation-schema";
import { syntheticComparison } from "./fixtures/comparison";

afterEach(() => vi.unstubAllGlobals());
it("validates interval relations and strict ordering without losing timestamp precision", () => {
  const data = syntheticComparison();
  data.report.previousCapturedAt = "2020-01-01T00:01:00.000001Z";
  data.report.currentStartedAt = "2020-01-01T01:01:00.000000+01:00";
  data.report.currentCapturedAt = "2020-01-01T00:01:00.000002Z";
  data.report.intervalRelation = "overlapping";
  expect(storedComparisonSchema.safeParse(data).success).toBe(true);
  data.report.intervalRelation = "disjoint";
  expect(storedComparisonSchema.safeParse(data).success).toBe(false);
  data.report.currentStartedAt = data.report.previousCapturedAt;
  expect(storedComparisonSchema.safeParse(data).success).toBe(true);
  data.report.currentCapturedAt = data.report.previousCapturedAt;
  expect(storedComparisonSchema.safeParse(data).success).toBe(false);
  data.report.currentCapturedAt = "2020-01-01T00:01:00.000000Z";
  expect(storedComparisonSchema.safeParse(data).success).toBe(false);
  data.report.currentCapturedAt = "not-a-date";
  expect(storedComparisonSchema.safeParse(data).success).toBe(false);
});
it("renders historical decimal differences without turning absence into a terminal state", () => {
  render(<ObservationComparison comparison={syntheticComparison()} />);
  expect(screen.getByRole("status")).toHaveTextContent(
    "COMPARACIÓN GUARDADA · NO ESTADO ACTUAL",
  );
  expect(
    screen.getByRole("cell", { name: "1.000000000000000001" }),
  ).toBeVisible();
  expect(screen.getByRole("cell", { name: /9007199254740993/ })).toBeVisible();
  expect(screen.getByText(/estado final desconocido/)).toBeVisible();
  expect(screen.queryByText(/CANCELED|FILLED|CLOSED/)).not.toBeInTheDocument();
});
it("labels initial observations instead of reporting invented new records", () => {
  const data = syntheticComparison();
  data.report = {
    ...data.report,
    status: "initial_observation",
    previousCapturedAt: null,
    intervalRelation: "no_baseline",
    totalChanges: 0,
    changes: [],
  };
  expect(storedComparisonSchema.safeParse(data).success).toBe(true);
  render(<ObservationComparison comparison={data} />);
  expect(screen.getByText(/Primera observación completa/)).toBeVisible();
  expect(screen.queryByRole("table")).not.toBeInTheDocument();
});
it("keeps a no-change comparison explicitly unreconciled", () => {
  const data = syntheticComparison();
  data.report = {
    ...data.report,
    totalChanges: 0,
    changes: [],
    intervalRelation: "overlapping",
    currentStartedAt: "2020-01-01T00:00:00Z",
    currentCapturedAt: "2020-01-01T00:00:02Z",
  };
  render(<ObservationComparison comparison={data} />);
  expect(
    screen.getByText(/Los intervalos de lectura se solapan/),
  ).toBeVisible();
  expect(
    screen.getByText(
      /No se detectaron diferencias.*no demuestra sincronización/,
    ),
  ).toBeVisible();
});
it("discloses a detail limit while retaining the complete record count", () => {
  const data = syntheticComparison();
  data.report = {
    ...data.report,
    totalChanges: 201,
    truncated: true,
    changes: Array.from({ length: 200 }, (_, i) => ({
      entity: "position",
      key: `S${i}USDT:LONG`,
      kind: "changed",
      fields: [{ field: "positionAmt", before: "1", after: "2" }],
    })),
  };
  expect(storedComparisonSchema.safeParse(data).success).toBe(true);
  render(<ObservationComparison comparison={data} />);
  expect(
    screen.getByText(/201 diferencias.*200.*DETALLE LIMITADO/),
  ).toBeVisible();
});
it("fetches comparisons only on explicit action, including when live reads are disabled, and clears on failure", async () => {
  const fetch = vi
    .fn()
    .mockResolvedValueOnce(
      Response.json({
        enabled: false,
        accounts: [
          {
            id: "00000000-0000-4000-8000-000000000001",
            name: "Synthetic",
            exchange: "binance",
            environment: "production",
          },
        ],
      }),
    )
    .mockResolvedValueOnce(Response.json(syntheticComparison()))
    .mockResolvedValueOnce(
      Response.json({ error: "Comparación no disponible." }, { status: 503 }),
    );
  vi.stubGlobal("fetch", fetch);
  const user = userEvent.setup();
  render(<BinanceConnection />);
  expect(fetch).not.toHaveBeenCalled();
  await user.click(screen.getByRole("button", { name: "Comprobar conexión" }));
  const button = await screen.findByRole("button", {
    name: "Ver cambios de la última lectura",
  });
  expect(fetch).toHaveBeenCalledTimes(1);
  await user.click(button);
  expect(
    await screen.findByRole("region", {
      name: "Comparación guardada de Binance",
    }),
  ).toBeVisible();
  expect(fetch.mock.calls[1][0]).toBe("/api/connections/binance/comparison");
  await user.click(button);
  expect(await screen.findByRole("alert")).toHaveTextContent(
    "Comparación no disponible.",
  );
  expect(
    screen.queryByRole("region", { name: "Comparación guardada de Binance" }),
  ).not.toBeInTheDocument();
});
it.each([
  (data: ReturnType<typeof syntheticComparison>) => ({
    ...data,
    report: { ...data.report, executionReady: true },
  }),
  (data: ReturnType<typeof syntheticComparison>) => ({
    ...data,
    report: { ...data.report, totalChanges: 3 },
  }),
  (data: ReturnType<typeof syntheticComparison>) => ({
    ...data,
    report: { ...data.report, continuity: "continuous" },
  }),
  (data: ReturnType<typeof syntheticComparison>) => ({
    ...data,
    report: {
      ...data.report,
      changes: [{ ...data.report.changes[0], kind: "FILLED" }],
    },
  }),
  (data: ReturnType<typeof syntheticComparison>) => ({ ...data, envelope: {} }),
])("rejects corrupt or overclaiming comparison responses", (mutate) => {
  expect(
    storedComparisonSchema.safeParse(mutate(syntheticComparison())).success,
  ).toBe(false);
});
