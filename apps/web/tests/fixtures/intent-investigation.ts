import { defaultRequest } from "@/lib/plans";
import type { StoredIntent, StoredPlan } from "@/lib/saved-plans-schema";
import type { IntentObservation } from "@/lib/intent-observation-schema";

// Synthetic persisted decisions and exchange evidence only, never real orders.
export const syntheticIntentAccount = {
  id: "00000000-0000-4000-8000-000000000001",
  name: "Synthetic intent account",
  exchange: "binance",
  environment: "production",
};
export const syntheticIntentId = "00000000-0000-4000-8000-000000000002";
export const syntheticPlanId = "00000000-0000-4000-8000-000000000003";
export const syntheticClientId = `rt${"a".repeat(30)}`;

export function syntheticSavedPlan(): StoredPlan {
  return {
    campaignId: syntheticPlanId,
    planId: syntheticPlanId,
    version: 1,
    mode: "clean",
    symbol: "BTCUSDT",
    riskAmount: "2",
    parameters: { ...defaultRequest, quantity: "2", maxLevels: 1 },
    createdAt: "2020-01-01T00:00:00Z",
    plan: {
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
          quantity: "2",
          cumulativeQuantity: "2",
          averagePrice: "100",
          margin: "20",
          stopPrice: "99",
          takeProfit: "103",
          takeProfitQuantity: "2",
          netExposure: "2",
        },
      ],
      warnings: [],
      stopReason: "Synthetic limit",
    },
  };
}
export function syntheticSavedIntent(): StoredIntent {
  return {
    id: syntheticIntentId,
    planId: syntheticPlanId,
    clientOrderId: syntheticClientId,
    exchangeOrderId: null,
    family: "regular",
    purpose: "ENTRY",
    positionSide: "LONG",
    status: "PENDING",
    quantity: "2",
    price: "100",
  };
}
export function syntheticIntentObservation(): IntentObservation {
  return {
    id: "00000000-0000-4000-8000-000000000004",
    storedAt: "2020-01-01T00:00:02Z",
    report: {
      expected: {
        intentId: syntheticIntentId,
        planId: syntheticPlanId,
        planFingerprint: "b".repeat(64),
        symbol: "BTCUSDT",
        clientOrderId: syntheticClientId,
        purpose: "ENTRY",
        positionSide: "LONG",
        quantity: "2",
        price: "100",
      },
      observation: {
        source: "binance-production",
        mode: "read_only",
        executionReady: false,
        startedAt: "2020-01-01T00:00:00Z",
        checkedAt: "2020-01-01T00:00:01Z",
        query: { symbol: "BTCUSDT", clientOrderId: syntheticClientId },
        outcome: "observed",
        reason: "",
        order: {
          orderId: "9007199254740993",
          clientOrderId: syntheticClientId,
          symbol: "BTCUSDT",
          side: "BUY",
          positionSide: "LONG",
          type: "LIMIT",
          status: "PARTIALLY_FILLED",
          origQty: "2.000",
          executedQty: "0.500000000000000001",
          price: "100.000",
          avgPrice: "99.123456789012345678",
          reduceOnly: false,
          closePosition: false,
          updateTime: 1577836801000,
        },
      },
      assessment: "attributes_match",
      mismatches: [],
      executionReady: false,
      ownershipConfirmed: false,
    },
  };
}
