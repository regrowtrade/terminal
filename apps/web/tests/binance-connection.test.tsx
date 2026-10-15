import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import { BinanceConnection } from "@/components/binance-connection";

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
    .mockResolvedValueOnce(
      Response.json({
        source: "binance-production",
        mode: "read_only",
        capturedAt: new Date().toISOString(),
        consistency: "rest_non_atomic",
        executionReady: false,
        assets: [
          {
            asset: "USDT",
            walletBalance: "100.00000001",
            availableBalance: "80",
            unrealizedProfit: "-2",
          },
        ],
        positions: [],
      }),
    )
    .mockResolvedValueOnce(
      Response.json({ error: "Lectura no disponible." }, { status: 503 }),
    );
  vi.stubGlobal("fetch", fetch);
  const user = userEvent.setup();
  render(<BinanceConnection />);
  await user.click(screen.getByRole("button", { name: "Comprobar conexión" }));
  await user.click(
    await screen.findByRole("button", {
      name: "Consultar saldos y posiciones",
    }),
  );
  expect(await screen.findByText("100.00000001")).toBeVisible();
  expect(screen.getByText(/REST no atómicas/)).toBeVisible();
  expect(
    screen.queryByRole("button", { name: /ejecutar|enviar orden/i }),
  ).not.toBeInTheDocument();
  await user.click(
    screen.getByRole("button", { name: "Consultar saldos y posiciones" }),
  );
  expect(await screen.findByRole("alert")).toHaveTextContent(
    "Lectura no disponible.",
  );
  expect(screen.queryByText("100.00000001")).not.toBeInTheDocument();
});
