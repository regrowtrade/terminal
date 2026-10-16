import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import { BinanceConnection } from "@/components/binance-connection";
import { syntheticSnapshot } from "./fixtures/binance";

afterEach(() => vi.unstubAllGlobals());
it("does not ask for keys or contact any endpoint automatically", async () => {
  const fetch = vi
    .fn()
    .mockResolvedValue(Response.json({ enabled: false, accounts: [] }));
  vi.stubGlobal("fetch", fetch);
  const user = userEvent.setup();
  render(<BinanceConnection />);
  expect(screen.queryByRole("textbox")).not.toBeInTheDocument();
  expect(fetch).not.toHaveBeenCalled();
  await user.click(screen.getByRole("button", { name: "Comprobar conexión" }));
  expect(
    await screen.findByText(/El motor mantiene la lectura real desactivada/),
  ).toBeVisible();
  expect(fetch).toHaveBeenCalledTimes(1);
});
it("shows a non-atomic decimal snapshot and clears it after failed refresh", async () => {
  const fetch = vi
    .fn()
    .mockResolvedValueOnce(
      Response.json({
        enabled: true,
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
    .mockResolvedValueOnce(Response.json(syntheticSnapshot()))
    .mockResolvedValueOnce(
      Response.json({ error: "Lectura no disponible." }, { status: 503 }),
    );
  vi.stubGlobal("fetch", fetch);
  const user = userEvent.setup();
  render(<BinanceConnection />);
  await user.click(screen.getByRole("button", { name: "Comprobar conexión" }));
  await user.click(
    await screen.findByRole("button", {
      name: "Consultar cuenta y órdenes",
    }),
  );
  expect(await screen.findByText("100.00000001")).toBeVisible();
  expect(screen.getByText(/REST no atómicas/)).toBeVisible();
  expect(screen.getByRole("cell", { name: /9007199254740993/ })).toBeVisible();
  expect(screen.getByRole("cell", { name: /9007199254740995/ })).toBeVisible();
  expect(screen.getByText(/LIMIT \/ PARTIALLY_FILLED/)).toBeVisible();
  expect(screen.getByText(/STOP_MARKET \/ NEW/)).toBeVisible();
  await user.click(screen.getByText(/Instrumentos y filtros USDT/));
  expect(
    screen.getByRole("table", { name: /Filtros publicados/ }),
  ).toBeVisible();
  expect(
    screen.queryByRole("button", { name: /ejecutar|enviar orden/i }),
  ).not.toBeInTheDocument();
  await user.click(
    screen.getByRole("button", { name: "Consultar cuenta y órdenes" }),
  );
  expect(await screen.findByRole("alert")).toHaveTextContent(
    "Lectura no disponible.",
  );
  expect(screen.queryByText("100.00000001")).not.toBeInTheDocument();
  expect(
    screen.queryByRole("cell", { name: /9007199254740993/ }),
  ).not.toBeInTheDocument();
});
it("recovers saved readings explicitly without presenting reception time as live data", async () => {
  const old = {
    ...syntheticSnapshot(),
    startedAt: "2020-01-01T00:00:00Z",
    capturedAt: "2020-01-01T00:00:01Z",
  };
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
    .mockResolvedValueOnce(
      Response.json({ snapshot: old, storedAt: "2020-01-01T00:00:02Z" }),
    );
  vi.stubGlobal("fetch", fetch);
  const user = userEvent.setup();
  render(<BinanceConnection />);
  await user.click(screen.getByRole("button", { name: "Comprobar conexión" }));
  expect(
    await screen.findByRole("button", { name: "Consultar cuenta y órdenes" }),
  ).toBeDisabled();
  expect(fetch).toHaveBeenCalledTimes(1);
  await user.click(
    screen.getByRole("button", { name: "Mostrar última lectura guardada" }),
  );
  expect(await screen.findByText(/LECTURA GUARDADA · NO ACTUAL/)).toBeVisible();
  expect(screen.getByText(/Recuperada de PostgreSQL/)).toBeVisible();
  expect(screen.queryByText(/LECTURA PUNTUAL/)).not.toBeInTheDocument();
  expect(fetch).toHaveBeenCalledTimes(2);
  expect(fetch.mock.calls[1][0]).toBe("/api/connections/binance/latest");
});
