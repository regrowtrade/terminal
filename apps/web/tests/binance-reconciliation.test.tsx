import { act, fireEvent, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import { BinanceReconciliation } from "@/components/binance-reconciliation";
import { BinanceConnection } from "@/components/binance-connection";
import { ReconciliationReport } from "@/components/reconciliation-report";
import { syntheticReconciliation } from "./fixtures/reconciliation";

const accountId = "00000000-0000-4000-8000-000000000001";
const secondId = "00000000-0000-4000-8000-000000000002";
const accounts = [accountId, secondId].map((id, i) => ({
  id,
  name: `Synthetic ${i}`,
  exchange: "binance",
  environment: "production",
}));
const recovery = {
  source: "binance-production",
  mode: "read_only",
  executionReady: false,
  symbol: "BTCUSDT",
  startTime: Date.parse("2020-01-01T00:00:00Z"),
  endTime: Date.parse("2020-01-02T00:00:00Z"),
  recovered: 1,
  duplicates: 2,
  truncated: true,
};
const reportButton = () =>
  screen.getByRole("button", { name: "Consultar reporte de reconciliación" });
const recoverButton = () =>
  screen.getByRole("button", { name: "Recuperar y guardar fills" });
function fillWindow(end = "2020-01-02T00:00") {
  fireEvent.change(screen.getByLabelText("Símbolo de recuperación"), {
    target: { value: "BTCUSDT" },
  });
  fireEvent.change(screen.getByLabelText("Inicio de ventana (UTC)"), {
    target: { value: "2020-01-01T00:00" },
  });
  fireEvent.change(screen.getByLabelText("Fin de ventana (UTC)"), {
    target: { value: end },
  });
}
afterEach(() => vi.unstubAllGlobals());

it("only requests a report explicitly, even with the external reader disabled", async () => {
  const fetch = vi
    .fn()
    .mockResolvedValue(Response.json(syntheticReconciliation()));
  vi.stubGlobal("fetch", fetch);
  const user = userEvent.setup();
  render(<BinanceReconciliation accountId={accountId} enabled={false} />);
  expect(fetch).not.toHaveBeenCalled();
  expect(recoverButton()).toBeDisabled();
  await user.click(reportButton());
  expect(await screen.findByText(/REPORTE PUNTUAL · NO EN VIVO/)).toBeVisible();
  expect(screen.getByText(/SIN HUECOS REGISTRADOS/)).toBeVisible();
  expect(screen.getByText(/Solo PostgreSQL/)).toBeVisible();
  const table = screen.getByRole("table", {
    name: /Fills posteriores al corte/,
  });
  expect(
    within(table).getByRole("cell", { name: "9007199254740993 / 12345678" }),
  ).toBeVisible();
  expect(
    within(table).getByRole("cell", { name: "0.0010 / USDT" }),
  ).toBeVisible();
  expect(within(table).getByRole("cell", { name: "0.0005" })).toBeVisible();
  expect(fetch).toHaveBeenCalledTimes(1);
  expect(fetch.mock.calls[0][0]).toBe("/api/connections/binance/reconcile");
  expect(JSON.parse(fetch.mock.calls[0][1].body)).toEqual({ accountId });
  expect(fetch.mock.calls[0][1]).toMatchObject({
    method: "POST",
    cache: "no-store",
    redirect: "error",
  });
});

it("makes gaps and both truncation limits explicit, without inferring absent fills", () => {
  render(
    <ReconciliationReport
      report={{
        ...syntheticReconciliation(),
        continuity: "gapped",
        eventsTruncated: true,
        fillsTruncated: true,
        totalFills: 300,
        fills: [],
      }}
    />,
  );
  expect(screen.getByText(/CONTINUIDAD CON HUECOS/)).toBeVisible();
  expect(screen.getByText(/EVENTOS TRUNCADOS/)).toBeVisible();
  expect(screen.getByText(/FILLS TRUNCADOS/)).toBeVisible();
  expect(screen.getByText(/No hay fills en este reporte/)).toHaveTextContent(
    "No demuestra ausencia",
  );
});

it("renders a signed REST commission without rounding, absolute values or currency aggregation", () => {
  const report = syntheticReconciliation();
  report.fills[0].commission = "-0.001000000000000001";
  render(<ReconciliationReport report={report} />);
  expect(
    screen.getByRole("cell", { name: "-0.001000000000000001 / USDT" }),
  ).toBeVisible();
});

it("paginates returned fills locally while keeping the omitted total visible", async () => {
  const report = syntheticReconciliation();
  report.fills = Array.from({ length: 51 }, (_, i) => ({
    ...report.fills[0],
    tradeId: String(i + 1),
  }));
  report.totalFills = 300;
  report.fillsTruncated = true;
  const fetch = vi.fn();
  vi.stubGlobal("fetch", fetch);
  render(<ReconciliationReport report={report} />);
  const table = screen.getByRole("table", { name: /Fills posteriores/ });
  expect(within(table).getAllByRole("row")).toHaveLength(51);
  expect(screen.getByText("51 / 300")).toBeVisible();
  await userEvent
    .setup()
    .click(
      screen.getByRole("button", { name: "Siguientes fills del reporte" }),
    );
  expect(within(table).getAllByRole("row")).toHaveLength(2);
  expect(
    within(table).getByRole("cell", { name: "9007199254740993 / 51" }),
  ).toBeVisible();
  expect(fetch).not.toHaveBeenCalled();
});

it.each(["2020-01-09T00:00", "2019-12-31T00:00", "2020-01-01T00:00"])(
  "rejects an invalid recovery window (%s) without I/O",
  async (end) => {
    const fetch = vi.fn();
    vi.stubGlobal("fetch", fetch);
    render(<BinanceReconciliation accountId={accountId} enabled />);
    fillWindow(end);
    await userEvent.setup().click(recoverButton());
    expect(await screen.findByRole("alert")).toHaveTextContent(
      "ventana UTC válida",
    );
    expect(fetch).not.toHaveBeenCalled();
  },
);

it("sends exact UTC criteria, retires the old report and never refreshes it automatically", async () => {
  const fetch = vi
    .fn()
    .mockResolvedValueOnce(Response.json(syntheticReconciliation()))
    .mockResolvedValueOnce(Response.json(recovery));
  vi.stubGlobal("fetch", fetch);
  const user = userEvent.setup();
  render(<BinanceReconciliation accountId={accountId} enabled />);
  await user.click(reportButton());
  expect(await screen.findByText(/REPORTE PUNTUAL/)).toBeVisible();
  fillWindow();
  await user.click(recoverButton());
  expect(await screen.findByText(/RECUPERACIÓN GUARDADA/)).toBeVisible();
  expect(screen.getByText(/RECUPERACIÓN TRUNCADA/)).toBeVisible();
  expect(screen.getByText(/Fills nuevos: 1. Duplicados: 2/)).toBeVisible();
  expect(screen.queryByText(/REPORTE PUNTUAL/)).not.toBeInTheDocument();
  expect(fetch).toHaveBeenCalledTimes(2);
  expect(fetch.mock.calls[1][0]).toBe("/api/connections/binance/fills/recover");
  expect(JSON.parse(fetch.mock.calls[1][1].body)).toEqual({
    accountId,
    symbol: recovery.symbol,
    startTime: recovery.startTime,
    endTime: recovery.endTime,
  });
  fireEvent.change(screen.getByLabelText("Símbolo de recuperación"), {
    target: { value: "ETHUSDT" },
  });
  expect(screen.queryByText(/RECUPERACIÓN GUARDADA/)).not.toBeInTheDocument();
});

it("shows a saved empty recovery without claiming that no trades occurred", async () => {
  const fetch = vi.fn().mockResolvedValue(
    Response.json({
      ...recovery,
      recovered: 0,
      duplicates: 0,
      truncated: false,
    }),
  );
  vi.stubGlobal("fetch", fetch);
  render(<BinanceReconciliation accountId={accountId} enabled />);
  fillWindow();
  await userEvent.setup().click(recoverButton());
  expect(await screen.findByText(/RECUPERACIÓN GUARDADA/)).toBeVisible();
  expect(screen.getByText(/Fills nuevos: 0. Duplicados: 0/)).toBeVisible();
  expect(
    screen.getByText(/Cero fills nuevos no demuestra ausencia/),
  ).toBeVisible();
  expect(fetch).toHaveBeenCalledTimes(1);
});

it("retires the report on ambiguous recovery failures and never retries automatically", async () => {
  const fetch = vi
    .fn()
    .mockResolvedValueOnce(Response.json(syntheticReconciliation()))
    .mockRejectedValueOnce(new Error("synthetic network failure"));
  vi.stubGlobal("fetch", fetch);
  const user = userEvent.setup();
  render(<BinanceReconciliation accountId={accountId} enabled />);
  await user.click(reportButton());
  expect(await screen.findByText(/REPORTE PUNTUAL/)).toBeVisible();
  fillWindow();
  await user.click(recoverButton());
  expect(await screen.findByRole("alert")).toHaveTextContent(
    "Puede haberse guardado evidencia",
  );
  expect(
    screen.queryByText(/REPORTE PUNTUAL|RECUPERACIÓN GUARDADA/),
  ).not.toBeInTheDocument();
  expect(fetch).toHaveBeenCalledTimes(2);
});

it.each([
  { ...recovery, symbol: "ETHUSDT" },
  { ...recovery, startTime: recovery.startTime + 1 },
  { ...recovery, endTime: recovery.endTime + 1 },
  { ...recovery, executionReady: true },
])("discards malformed or mismatched recovery responses", async (body) => {
  const fetch = vi.fn().mockResolvedValue(Response.json(body));
  vi.stubGlobal("fetch", fetch);
  render(<BinanceReconciliation accountId={accountId} enabled />);
  fillWindow();
  await userEvent.setup().click(recoverButton());
  expect(await screen.findByRole("alert")).toHaveTextContent(
    "Puede haberse guardado evidencia",
  );
  expect(screen.queryByText(/RECUPERACIÓN GUARDADA/)).not.toBeInTheDocument();
  expect(fetch).toHaveBeenCalledTimes(1);
});

it.each([401, 429, 503])(
  "clears a previous report after a failed refresh (%s), without leaking upstream errors",
  async (status) => {
    const fetch = vi
      .fn()
      .mockResolvedValueOnce(Response.json(syntheticReconciliation()))
      .mockResolvedValueOnce(
        Response.json({ error: "private upstream detail" }, { status }),
      );
    vi.stubGlobal("fetch", fetch);
    const user = userEvent.setup();
    render(<BinanceReconciliation accountId={accountId} enabled />);
    await user.click(reportButton());
    expect(await screen.findByText(/REPORTE PUNTUAL/)).toBeVisible();
    await user.click(reportButton());
    expect(await screen.findByRole("alert")).toHaveTextContent(
      "No se pudo consultar el reporte",
    );
    expect(
      screen.queryByText(/REPORTE PUNTUAL|private upstream detail/),
    ).not.toBeInTheDocument();
    expect(fetch).toHaveBeenCalledTimes(2);
  },
);

it("explains a missing baseline and fails closed on an execution-enabled report", async () => {
  const fetch = vi
    .fn()
    .mockResolvedValueOnce(Response.json({}, { status: 404 }))
    .mockResolvedValueOnce(
      Response.json({ ...syntheticReconciliation(), executionReady: true }),
    );
  vi.stubGlobal("fetch", fetch);
  const user = userEvent.setup();
  render(<BinanceReconciliation accountId={accountId} enabled />);
  await user.click(reportButton());
  expect(await screen.findByRole("alert")).toHaveTextContent(
    "lectura base no disponible",
  );
  await user.click(reportButton());
  expect(await screen.findByRole("alert")).toHaveTextContent(
    "No se pudo consultar",
  );
  expect(screen.queryByText(/REPORTE PUNTUAL/)).not.toBeInTheDocument();
});

it("aborts a pending report on account changes and ignores late responses", async () => {
  let resolve!: (response: Response) => void;
  let signal!: AbortSignal;
  const fetch = vi
    .fn()
    .mockResolvedValueOnce(Response.json({ enabled: true, accounts }))
    .mockImplementationOnce((_url, options) => {
      signal = options.signal;
      return new Promise<Response>((r) => {
        resolve = r;
      });
    });
  vi.stubGlobal("fetch", fetch);
  const user = userEvent.setup();
  render(<BinanceConnection />);
  await user.click(screen.getByRole("button", { name: "Comprobar conexión" }));
  await user.click(reportButton());
  expect(reportButton()).toBeDisabled();
  expect(recoverButton()).toBeDisabled();
  await user.selectOptions(
    screen.getByLabelText("Cuenta de lectura"),
    secondId,
  );
  expect(signal.aborted).toBe(true);
  await act(async () => {
    resolve(Response.json(syntheticReconciliation()));
  });
  expect(screen.queryByText(/REPORTE PUNTUAL/)).not.toBeInTheDocument();
  expect(reportButton()).toBeEnabled();
  expect(fetch).toHaveBeenCalledTimes(2);
});

it("retires a report when another baseline operation starts, even when that operation fails", async () => {
  const fetch = vi
    .fn()
    .mockResolvedValueOnce(Response.json({ enabled: false, accounts }))
    .mockResolvedValueOnce(Response.json(syntheticReconciliation()))
    .mockResolvedValueOnce(Response.json({}, { status: 503 }));
  vi.stubGlobal("fetch", fetch);
  const user = userEvent.setup();
  render(<BinanceConnection />);
  await user.click(screen.getByRole("button", { name: "Comprobar conexión" }));
  await user.click(reportButton());
  expect(await screen.findByText(/REPORTE PUNTUAL/)).toBeVisible();
  await user.click(
    screen.getByRole("button", { name: "Mostrar última lectura guardada" }),
  );
  expect(await screen.findByRole("alert")).toBeVisible();
  expect(screen.queryByText(/REPORTE PUNTUAL/)).not.toBeInTheDocument();
  expect(fetch).toHaveBeenCalledTimes(3);
});

it("aborts recovery on unmount and suppresses a late success", async () => {
  let resolve!: (response: Response) => void;
  let signal!: AbortSignal;
  const fetch = vi.fn().mockImplementation((_url, options) => {
    signal = options.signal;
    return new Promise<Response>((r) => {
      resolve = r;
    });
  });
  vi.stubGlobal("fetch", fetch);
  const { unmount } = render(
    <BinanceReconciliation accountId={accountId} enabled />,
  );
  fillWindow();
  await userEvent.setup().click(recoverButton());
  expect(reportButton()).toBeDisabled();
  unmount();
  expect(signal.aborted).toBe(true);
  await act(async () => {
    resolve(Response.json(recovery));
  });
  expect(screen.queryByText(/RECUPERACIÓN GUARDADA/)).not.toBeInTheDocument();
});
