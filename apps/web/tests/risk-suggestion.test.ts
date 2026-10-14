import { describe, expect, it } from "vitest";
import { riskSuggestion } from "@/lib/risk-suggestion";

const hedge = {
  longEntry: "100",
  shortEntry: "98",
  longQuantity: "1",
  shortQuantity: "1",
  markPrice: "99",
  realizedPnl: "0",
  feesPaid: "0",
  fundingNet: "0",
  closePercent: "5" as const,
};
describe("editable risk suggestion", () => {
  it("suggests the current combined loss", () => {
    expect(riskSuggestion(hedge, "amount", "100")).toEqual({
      net: "-2",
      suggested: "2",
    });
  });
  it("uses recovered realized PnL at a new activation", () => {
    expect(
      riskSuggestion({ ...hedge, realizedPnl: "1" }, "amount", "100"),
    ).toEqual({ net: "-1", suggested: "1" });
  });
  it("includes fees and signed funding", () => {
    expect(
      riskSuggestion(
        { ...hedge, feesPaid: "0.2", fundingNet: "-0.1" },
        "amount",
        "100",
      ),
    ).toEqual({ net: "-2.3", suggested: "2.3" });
  });
  it("converts to a percentage of the captured reference", () => {
    expect(riskSuggestion(hedge, "percent", "200").suggested).toBe("1");
  });
  it("does not recommend a negative budget when profitable", () => {
    expect(
      riskSuggestion({ ...hedge, realizedPnl: "3" }, "amount", "100"),
    ).toEqual({ net: "1", suggested: "0" });
  });
  it("does not use binary float for fractional coins", () => {
    expect(
      riskSuggestion(
        { ...hedge, longQuantity: "0.1", shortQuantity: "0.1" },
        "amount",
        "100",
      ),
    ).toEqual({ net: "-0.2", suggested: "0.2" });
  });
  it("rejects an invalid or zero reference", () => {
    expect(() => riskSuggestion(hedge, "percent", "0")).toThrow();
  });
});
