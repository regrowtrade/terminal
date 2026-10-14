import { describe, expect, it } from "vitest";
import { defaultRequest, planRequestSchema, planSchema } from "@/lib/plans";

describe("plan contracts", () => {
  it("accepts a valid request", () => {
    expect(planRequestSchema.safeParse(defaultRequest).success).toBe(true);
  });
  it("accepts computed precision beyond the input precision", () => {
    expect(
      planSchema.safeParse({
        mode: "dca",
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
        spacingPercent: "4",
        warnings: [],
        stopReason: "cap",
        levels: [
          {
            index: 1,
            price: "100",
            quantity: "1",
            cumulativeQuantity: "3",
            averagePrice: "98.66666666666666666666666666666667",
            margin: "29.6",
            stopPrice: "98",
            takeProfit: "106",
            takeProfitQuantity: "3",
            netExposure: "3",
          },
        ],
      }).success,
    ).toBe(true);
  });
  it.each([
    "0",
    "-1",
    "1e2",
    "NaN",
    "Infinity",
    "1,2",
    "99999999999999999",
    "0.1234567890123",
  ])("rejects invalid risk %s", (risk) => {
    expect(
      planRequestSchema.safeParse({ ...defaultRequest, risk }).success,
    ).toBe(false);
  });
  it("rejects unknown fields and invalid modes", () => {
    expect(
      planRequestSchema.safeParse({ ...defaultRequest, execute: true }).success,
    ).toBe(false);
    expect(
      planRequestSchema.safeParse({ ...defaultRequest, mode: "live" }).success,
    ).toBe(false);
  });
  it("requires hedge inputs only in hedge mode", () => {
    expect(
      planRequestSchema.safeParse({ ...defaultRequest, mode: "hedge" }).success,
    ).toBe(false);
    expect(
      planRequestSchema.safeParse({ ...defaultRequest, hedge: {} }).success,
    ).toBe(false);
  });
  it("rejects excessive leverage and levels", () => {
    expect(
      planRequestSchema.safeParse({ ...defaultRequest, leverage: 126 }).success,
    ).toBe(false);
    expect(
      planRequestSchema.safeParse({ ...defaultRequest, maxLevels: 12 }).success,
    ).toBe(false);
  });
  it("never accepts a live execution response", () => {
    expect(planSchema.safeParse({ simulation: false }).success).toBe(false);
  });
});
