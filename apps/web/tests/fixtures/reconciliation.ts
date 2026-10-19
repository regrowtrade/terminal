import type { z } from "zod";
import { reconciliationReportSchema } from "@/lib/reconciliation-schema";

// Synthetic financial data only. Never loads credentials or contacts exchanges.
export function syntheticReconciliation(): z.infer<
  typeof reconciliationReportSchema
> {
  return {
    continuity: "established",
    executionReady: false,
    snapshotCapturedAt: "2020-01-01T00:00:00Z",
    eventsTruncated: false,
    totalEvents: 2,
    orderUpdates: 1,
    accountUpdates: 1,
    lastEventTime: 1568879465651,
    totalFills: 1,
    fillLimit: 200,
    fillsTruncated: false,
    fills: [
      {
        orderId: "9007199254740993",
        tradeId: "12345678",
        symbol: "BTCUSDT",
        side: "SELL",
        positionSide: "SHORT",
        price: "9910.00",
        quantity: "0.0005",
        commission: "0.0010",
        commissionAsset: "USDT",
        eventTime: 1568879465651,
        transactionTime: 1568879465650,
      },
    ],
  };
}
