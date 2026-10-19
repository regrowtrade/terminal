import type { OrderLookup } from "@/lib/order-lookup-schema";

export function syntheticOrderLookup(): OrderLookup {
  return {
    source: "binance-production",
    mode: "read_only",
    executionReady: false,
    startedAt: "2026-01-01T00:00:00Z",
    checkedAt: "2026-01-01T00:00:01Z",
    query: { symbol: "BTCUSDT", orderId: "9007199254740993" },
    outcome: "observed",
    reason: "",
    order: {
      orderId: "9007199254740993",
      clientOrderId: "synthetic-limit",
      symbol: "BTCUSDT",
      side: "BUY",
      positionSide: "SHORT",
      type: "LIMIT",
      status: "FILLED",
      origQty: "2.000",
      executedQty: "2.000",
      price: "90.00000001",
      avgPrice: "89.12345678",
      reduceOnly: true,
      closePosition: false,
      updateTime: 1750514941540,
    },
  };
}
