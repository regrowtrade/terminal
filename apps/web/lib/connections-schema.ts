import { z } from "zod";

const decimal = z
  .string()
  .regex(/^-?(?:0|[1-9][0-9]{0,37})(?:\.[0-9]{1,18})?$/);
const nonnegative = z
  .string()
  .regex(/^(?:0|[1-9][0-9]{0,37})(?:\.[0-9]{1,18})?$/);
const symbol = z.string().regex(/^[A-Z0-9]{2,24}USDT$/);
const side = z.enum(["BOTH", "LONG", "SHORT"]);
const exchangeToken = z.string().regex(/^[A-Z0-9_]{1,40}$/);
const orderId = z
  .string()
  .regex(/^[1-9][0-9]{0,18}$/)
  .refine(
    (id) => /^[1-9][0-9]{0,18}$/.test(id) && BigInt(id) <= 9223372036854775807n,
  );
const clientId = z.string().regex(/^[.A-Z:/a-z0-9_-]{1,64}$/);
const conditionalType = z.enum([
  "STOP",
  "STOP_MARKET",
  "TAKE_PROFIT",
  "TAKE_PROFIT_MARKET",
  "TRAILING_STOP_MARKET",
]);
const rangeFilter = z
  .object({ min: nonnegative, max: nonnegative, step: nonnegative })
  .strict();
const orderFields = {
  symbol,
  side: z.enum(["BUY", "SELL"]),
  positionSide: side,
  price: nonnegative,
  reduceOnly: z.boolean(),
  closePosition: z.boolean(),
  workingType: z.enum(["", "MARK_PRICE", "CONTRACT_PRICE"]),
  activatePrice: nonnegative.optional(),
  updateTime: z.number().int().positive().max(Number.MAX_SAFE_INTEGER),
};
export const openOrderSchema = z
  .object({
    ...orderFields,
    orderId,
    clientOrderId: clientId,
    type: z.union([z.enum(["LIMIT", "MARKET"]), conditionalType]),
    status: z.enum(["NEW", "PARTIALLY_FILLED"]),
    origQty: nonnegative,
    executedQty: nonnegative,
    stopPrice: nonnegative,
    priceRate: nonnegative.optional(),
  })
  .strict();
export const algoOrderSchema = z
  .object({
    ...orderFields,
    algoId: orderId,
    clientAlgoId: clientId,
    algoType: z.literal("CONDITIONAL"),
    orderType: conditionalType,
    algoStatus: z.literal("NEW"),
    quantity: nonnegative,
    triggerPrice: nonnegative,
    workingType: z.enum(["MARK_PRICE", "CONTRACT_PRICE"]),
    callbackRate: nonnegative.optional(),
  })
  .strict();
export const instrumentSchema = z
  .object({
    symbol,
    contractType: z.literal("PERPETUAL"),
    status: exchangeToken,
    baseAsset: exchangeToken,
    quoteAsset: z.literal("USDT"),
    marginAsset: z.literal("USDT"),
    priceFilter: rangeFilter,
    lotSize: rangeFilter,
    marketLotSize: rangeFilter,
    minNotional: nonnegative,
    multiplierUp: nonnegative,
    multiplierDown: nonnegative,
    maxNumOrders: z.number().int().nonnegative().max(Number.MAX_SAFE_INTEGER),
    maxNumAlgoOrders: z
      .number()
      .int()
      .nonnegative()
      .max(Number.MAX_SAFE_INTEGER),
    unrecognizedFilterTypes: z.array(exchangeToken).max(64),
  })
  .strict();
export const connectionStateSchema = z
  .object({
    enabled: z.boolean(),
    accounts: z.array(
      z
        .object({
          id: z.uuid(),
          name: z.string().min(1).max(80),
          exchange: z.literal("binance"),
          environment: z.literal("production"),
        })
        .strict(),
    ),
  })
  .strict();
export const snapshotSchema = z
  .object({
    source: z.literal("binance-production"),
    mode: z.literal("read_only"),
    capturedAt: z.iso.datetime({ offset: true }),
    capturedAtMs: z.number().int().positive().max(Number.MAX_SAFE_INTEGER),
    startedAt: z.iso.datetime({ offset: true }),
    consistency: z.literal("rest_non_atomic"),
    executionReady: z.literal(false),
    assets: z
      .array(
        z
          .object({
            asset: z.string().min(1).max(20),
            walletBalance: decimal,
            unrealizedProfit: decimal,
            availableBalance: decimal,
          })
          .strict(),
      )
      .max(1000),
    positions: z
      .array(
        z
          .object({
            symbol,
            positionSide: side,
            positionAmt: decimal,
            entryPrice: decimal,
            markPrice: decimal,
            unRealizedProfit: decimal,
            liquidationPrice: decimal,
          })
          .strict(),
      )
      .max(10000),
    openOrders: z.array(openOrderSchema).max(10000),
    algoOrders: z.array(algoOrderSchema).max(10000),
    instruments: z.array(instrumentSchema).min(1).max(10000),
  })
  .strict()
  .refine(
    (s) => Date.parse(s.startedAt) <= Date.parse(s.capturedAt),
    "Invalid observation interval",
  );
export type ConnectionState = z.infer<typeof connectionStateSchema>;
export type AccountSnapshot = z.infer<typeof snapshotSchema>;
export const storedSnapshotSchema = z
  .object({
    snapshot: snapshotSchema,
    storedAt: z.iso.datetime({ offset: true }),
  })
  .strict();
