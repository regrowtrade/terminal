import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { Planner } from "@/components/planner";
import type { Plan } from "@/lib/plans";

const response: Plan = {
  mode: "clean",
  symbol: "BTCUSDT",
  side: "LONG",
  simulation: true,
  risk: "2",
  riskInput: "2",
  riskUnit: "amount",
  accountReference: "100",
  accountBasis: "equity",
  costReserve: "0",
  baselinePnl: null,
  pnlThreshold: null,
  spacingPercent: "0",
  warnings: ["Simulación únicamente"],
  stopReason: "Una entrada",
  levels: [
    {
      index: 1,
      price: "100",
      quantity: "1",
      cumulativeQuantity: "1",
      averagePrice: "100",
      margin: "10",
      stopPrice: "98",
      takeProfit: "106",
      takeProfitQuantity: "1",
      netExposure: "1",
    },
  ],
};
afterEach(() => vi.unstubAllGlobals());
describe("planner interaction", () => {
  it("accepts a Spanish decimal comma in editable monetary fields", async () => {
    const user = userEvent.setup();
    render(<Planner />);
    const input = screen.getByLabelText("Riesgo total autorizado");
    await user.clear(input);
    await user.type(input, "1,5");
    expect(input).toHaveValue("1.5");
  });
  it("clearly labels simulation and has no execution control", () => {
    render(<Planner />);
    expect(screen.getByText("Vista previa, no ejecución.")).toBeVisible();
    expect(
      screen.queryByRole("button", { name: /ejecutar|enviar orden/i }),
    ).not.toBeInTheDocument();
  });
  it("switches methodology and offers an editable hedge suggestion", async () => {
    const user = userEvent.setup();
    render(<Planner />);
    await user.click(screen.getByRole("button", { name: /Cobertura/ }));
    expect(screen.getByLabelText("Riesgo adicional autorizado")).toHaveValue(
      "2",
    );
    await user.clear(
      screen.getByLabelText("PnL realizado de toda la operación"),
    );
    await user.type(
      screen.getByLabelText("PnL realizado de toda la operación"),
      "1",
    );
    await user.click(
      screen.getByRole("button", { name: /Usar adicional sugerido/ }),
    );
    expect(screen.getByLabelText("Riesgo adicional autorizado")).toHaveValue(
      "1",
    );
  });
  it("validates inputs before contacting the backend", async () => {
    const fetch = vi.fn();
    vi.stubGlobal("fetch", fetch);
    const user = userEvent.setup();
    render(<Planner />);
    await user.clear(screen.getByLabelText("Riesgo total autorizado"));
    await user.type(screen.getByLabelText("Riesgo total autorizado"), "-2");
    await user.click(
      screen.getByRole("button", { name: "Calcular vista previa" }),
    );
    expect(screen.getByRole("alert")).toHaveTextContent("Revisa los campos");
    expect(fetch).not.toHaveBeenCalled();
  });
  it("shows actual backend preview results without calling an exchange", async () => {
    const fetch = vi.fn().mockResolvedValue(Response.json(response));
    vi.stubGlobal("fetch", fetch);
    const user = userEvent.setup();
    render(<Planner />);
    await user.click(
      screen.getByRole("button", { name: "Calcular vista previa" }),
    );
    expect(await screen.findByText("NO EJECUTADO")).toBeVisible();
    expect(
      screen.getByRole("heading", { name: "BTCUSDT · LONG" }),
    ).toBeVisible();
    expect(fetch).toHaveBeenCalledWith(
      "/api/plans/preview",
      expect.objectContaining({ method: "POST" }),
    );
    await user.type(screen.getByLabelText("Precio de entrada"), "1");
    expect(screen.queryByText("NO EJECUTADO")).not.toBeInTheDocument();
  });
  it("reports unavailable engine and permits retry", async () => {
    vi.stubGlobal(
      "fetch",
      vi
        .fn()
        .mockResolvedValue(
          Response.json({ error: "Motor no disponible." }, { status: 503 }),
        ),
    );
    const user = userEvent.setup();
    render(<Planner />);
    await user.click(
      screen.getByRole("button", { name: "Calcular vista previa" }),
    );
    expect(await screen.findByRole("alert")).toHaveTextContent(
      "Motor no disponible.",
    );
    expect(
      screen.getByRole("button", { name: "Calcular vista previa" }),
    ).toBeEnabled();
  });
});
