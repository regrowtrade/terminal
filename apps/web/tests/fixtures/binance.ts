import type { AccountSnapshot } from "@/lib/connections-schema";

// Synthetic financial data only. Never load credentials or contact exchanges.
export function syntheticSnapshot(): AccountSnapshot {
  const capturedAt = new Date().toISOString();
  return {
    source: "binance-production",
    mode: "read_only",
    startedAt: capturedAt,
    capturedAt,
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
    openOrders: [
      {
        orderId: "9007199254740993",
        clientOrderId: "synthetic-limit",
        symbol: "BTCUSDT",
        side: "BUY",
        positionSide: "SHORT",
        type: "LIMIT",
        status: "PARTIALLY_FILLED",
        origQty: "2",
        executedQty: "0.5",
        price: "90.00000001",
        stopPrice: "0",
        reduceOnly: true,
        closePosition: false,
        workingType: "CONTRACT_PRICE",
        updateTime: 1750514941540,
      },
    ],
    algoOrders: [
      {
        algoId: "9007199254740995",
        clientAlgoId: "synthetic-stop",
        algoType: "CONDITIONAL",
        symbol: "BTCUSDT",
        side: "BUY",
        positionSide: "SHORT",
        orderType: "STOP_MARKET",
        algoStatus: "NEW",
        quantity: "0",
        price: "0",
        triggerPrice: "120",
        reduceOnly: false,
        closePosition: true,
        workingType: "MARK_PRICE",
        updateTime: 1750514941540,
      },
    ],
    instruments: [
      {
        symbol: "BTCUSDT",
        contractType: "PERPETUAL",
        status: "TRADING",
        baseAsset: "BTC",
        quoteAsset: "USDT",
        marginAsset: "USDT",
        priceFilter: { min: "0.10", max: "1000000", step: "0.10" },
        lotSize: { min: "0.001", max: "1000", step: "0.001" },
        marketLotSize: { min: "0.001", max: "100", step: "0.000" },
        minNotional: "5",
        multiplierUp: "1.05",
        multiplierDown: "0.95",
        maxNumOrders: 200,
        maxNumAlgoOrders: 10,
        unrecognizedFilterTypes: [],
      },
    ],
  };
}

export function syntheticAccountSnapshot(): AccountSnapshot {
  const result = syntheticSnapshot();
  result.positions = [
    {
      symbol: "BTCUSDT",
      positionSide: "LONG",
      positionAmt: "1.000000000000000001",
      entryPrice: "100.00000001",
      markPrice: "101",
      unRealizedProfit: "0.99999999",
      liquidationPrice: "50",
    },
    {
      symbol: "BTCUSDT",
      positionSide: "SHORT",
      positionAmt: "-2.000000000000000001",
      entryPrice: "110",
      markPrice: "101",
      unRealizedProfit: "18",
      liquidationPrice: "150",
    },
    {
      symbol: "ETHUSDT",
      positionSide: "BOTH",
      positionAmt: "0",
      entryPrice: "0",
      markPrice: "2",
      unRealizedProfit: "0",
      liquidationPrice: "0",
    },
  ];
  result.instruments.push({
    ...result.instruments[0],
    symbol: "ETHUSDT",
    baseAsset: "ETH",
  });
  result.openOrders.push({
    ...result.openOrders[0],
    symbol: "ETHUSDT",
    positionSide: "BOTH",
    orderId: "9007199254740997",
    clientOrderId: "synthetic-one-way",
    status: "NEW",
    executedQty: "0",
  });
  return result;
}
