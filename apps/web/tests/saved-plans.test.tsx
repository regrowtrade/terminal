import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import { PlanSave } from "@/components/plan-save";
import { SavedPlans } from "@/components/saved-plans";
import type { PlanRequest } from "@/lib/plans";

const account = {
  id: "00000000-0000-4000-8000-000000000001",
  name: "Synthetic",
  exchange: "binance",
  environment: "production",
};
const planId = "00000000-0000-4000-8000-000000000002";
const request: PlanRequest = {
  mode: "clean",
  symbol: "BTCUSDT",
  side: "LONG",
  entry: "100",
  quantity: "1",
  risk: "2",
  rewardRatio: "3",
  riskUnit: "amount",
  accountReference: "100",
  accountBasis: "equity",
  spacingPercent: "4",
  leverage: 10,
  maxLevels: 1,
  maxMargin: "100",
  costReserve: "0",
};
const plan = {
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
  warnings: [],
  stopReason: "Número máximo de niveles solicitado alcanzado.",
};

function accountsFetch() {
  return Response.json({ enabled: true, accounts: [account] });
}

afterEach(() => vi.unstubAllGlobals());

it("saves a computed plan and its derived intents without an execution control", async () => {
  const fetch = vi.fn(async (input: RequestInfo | URL) => {
    const url =
      typeof input === "string"
        ? input
        : input instanceof URL
          ? input.pathname
          : input.url;
    if (url.includes("connections/binance/list")) return accountsFetch();
    if (url.includes("/api/plans/save"))
      return Response.json({ campaignId: planId, planId, version: 1 });
    if (url.includes("/api/plans/intents/save"))
      return Response.json({ count: 3 });
    return Response.json({});
  });
  vi.stubGlobal("fetch", fetch);
  const user = userEvent.setup();
  render(<PlanSave request={request} />);
  expect(await screen.findByLabelText("Guardar en la cuenta")).toBeVisible();
  await user.click(screen.getByRole("button", { name: /Guardar plan/ }));
  expect(await screen.findByText(/Plan guardado/)).toBeVisible();
  expect(screen.getByText(/3 intenciones derivadas/)).toBeVisible();
  expect(
    screen.queryByRole("button", { name: /ejecutar|enviar orden/i }),
  ).not.toBeInTheDocument();
});

it("lists persisted plans and their never-sent intents for an account", async () => {
  const storedPlan = {
    campaignId: planId,
    planId,
    version: 1,
    mode: "clean",
    symbol: "BTCUSDT",
    riskAmount: "2",
    parameters: request,
    plan,
    createdAt: "2026-01-01T00:00:00Z",
  };
  const intent = {
    id: planId,
    planId,
    clientOrderId: "rt1234",
    exchangeOrderId: null,
    family: "regular",
    purpose: "ENTRY",
    positionSide: "LONG",
    status: "PENDING",
    quantity: "1",
    price: "100",
  };
  const fetch = vi.fn(async (input: RequestInfo | URL) => {
    const url =
      typeof input === "string"
        ? input
        : input instanceof URL
          ? input.pathname
          : input.url;
    if (url.includes("connections/binance/list")) return accountsFetch();
    if (url.includes("/api/plans/list")) return Response.json([storedPlan]);
    if (url.includes("/api/plans/intents/list")) return Response.json([intent]);
    return Response.json({});
  });
  vi.stubGlobal("fetch", fetch);
  render(<SavedPlans />);
  expect(await screen.findByText(/BTCUSDT · CLEAN/)).toBeVisible();
  expect(await screen.findByText(/NO EJECUTADO/)).toBeVisible();
  await waitFor(() => expect(screen.getByText(/ENTRY/)).toBeVisible());
  expect(screen.getByText(/rt1234/)).toBeVisible();
});
