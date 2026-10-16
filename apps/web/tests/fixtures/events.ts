import type { z } from "zod";
import { eventsResponseSchema } from "@/lib/events-schema";

// Synthetic financial data only. Never loads credentials or contacts exchanges.
export function syntheticEventsResponse(): z.infer<
  typeof eventsResponseSchema
> {
  return {
    events: [
      {
        event: {
          type: "ORDER_TRADE_UPDATE",
          eventTime: 1568879465651,
          transactionTime: 1568879465650,
          orderUpdate: {
            symbol: "BTCUSDT",
            clientOrderId: "client-1",
            side: "SELL",
            positionSide: "SHORT",
            type: "LIMIT",
            timeInForce: "GTC",
            quantity: "0.001",
            price: "9910",
            stopPrice: "0",
            activatePrice: "0",
            workingType: "CONTRACT_PRICE",
            executionType: "TRADE",
            orderStatus: "PARTIALLY_FILLED",
            orderId: "9007199254740993",
            lastFilledQty: "0.0005",
            cumulativeQty: "0.0005",
            lastFilledPrice: "9910.00",
            commission: "0.0010",
            commissionAsset: "USDT",
            tradeId: "12345678",
            reduceOnly: true,
            closePosition: false,
            realizedProfit: "-0.1",
          },
        },
        receivedAt: "2020-01-01T00:00:02Z",
      },
      {
        event: {
          type: "ACCOUNT_UPDATE",
          eventTime: 1564745798939,
          transactionTime: 1564745798938,
          accountUpdate: {
            reason: "ORDER",
            balances: [
              {
                asset: "USDT",
                walletBalance: "122624.12345678",
                crossWalletBalance: "100.12345678",
                balanceChange: "50.12345678",
              },
            ],
            positions: [
              {
                symbol: "BTCUSDT",
                positionAmount: "0",
                entryPrice: "0.00000",
                accumulatedRealized: "200",
                unrealizedPnL: "0",
                marginType: "isolated",
                isolatedWallet: "0",
                positionSide: "LONG",
              },
            ],
          },
        },
        receivedAt: "2020-01-01T00:00:03Z",
      },
    ],
    truncated: false,
  };
}
