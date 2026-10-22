import type { AlgoLookup } from "@/lib/algo-lookup-schema";

export function syntheticAlgoLookup(): AlgoLookup {
  return {
    source: "binance-production",
    mode: "read_only",
    family: "algo",
    executionReady: false,
    startedAt: "2026-01-01T00:00:00Z",
    checkedAt: "2026-01-01T00:00:01Z",
    query: { symbol: "BTCUSDT", algoId: "9007199254740993" },
    outcome: "observed",
    reason: "",
    order: {
      algoId: "9007199254740993",
      clientAlgoId: "synthetic-algo",
      algoType: "CONDITIONAL",
      symbol: "BTCUSDT",
      side: "SELL",
      positionSide: "LONG",
      orderType: "STOP_MARKET",
      algoStatus: "FINISHED",
      quantity: "2.000",
      price: "0",
      triggerPrice: "95.123456789012345678",
      workingType: "MARK_PRICE",
      reduceOnly: true,
      closePosition: false,
      updateTime: 1750514941540,
      actualOrderId: "9007199254740995",
      actualPrice: "94.123456789012345678",
      actualType: "MARKET",
      actualQty: "0.500000000000000001",
      triggerTime: 1750514941539,
    },
  };
}
