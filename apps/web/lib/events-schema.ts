import { z } from "zod";

// Mirrors the engine's normalized private user data stream events. Amounts stay
// as decimal strings and int64 IDs as strings; nothing here is a reconciled fill.
const decimal = z
  .string()
  .regex(/^-?(?:0|[1-9][0-9]{0,37})(?:\.[0-9]{1,18})?$/);
const nonnegative = z
  .string()
  .regex(/^(?:0|[1-9][0-9]{0,37})(?:\.[0-9]{1,18})?$/);
const symbol = z.string().regex(/^[A-Z0-9]{2,24}USDT$/);
const side = z.enum(["BOTH", "LONG", "SHORT"]);
const orderId = z
  .string()
  .regex(/^[1-9][0-9]{0,18}$/)
  .refine(
    (id) => /^[1-9][0-9]{0,18}$/.test(id) && BigInt(id) <= 9223372036854775807n,
  );
const conditionalType = z.enum([
  "STOP",
  "STOP_MARKET",
  "TAKE_PROFIT",
  "TAKE_PROFIT_MARKET",
  "TRAILING_STOP_MARKET",
]);
const token = z.string().regex(/^[A-Z][A-Z0-9_]{0,31}$/);

export const orderUpdateSchema = z
  .object({
    symbol,
    clientOrderId: z.string().max(64),
    side: z.enum(["BUY", "SELL"]),
    positionSide: side,
    type: z.union([z.enum(["LIMIT", "MARKET"]), conditionalType]),
    timeInForce: z.string().max(16),
    quantity: nonnegative,
    price: nonnegative,
    stopPrice: nonnegative,
    activatePrice: nonnegative.optional(),
    workingType: z.enum(["", "MARK_PRICE", "CONTRACT_PRICE"]),
    executionType: token,
    orderStatus: token,
    orderId,
    lastFilledQty: nonnegative,
    cumulativeQty: nonnegative,
    lastFilledPrice: nonnegative,
    commission: nonnegative,
    commissionAsset: z.string().max(20),
    tradeId: z
      .string()
      .regex(/^(?:0|[1-9][0-9]{0,18})$/)
      .max(20),
    reduceOnly: z.boolean(),
    closePosition: z.boolean(),
    realizedProfit: decimal,
  })
  .strict();

export const balanceUpdateSchema = z
  .object({
    asset: z.string().min(1).max(20),
    walletBalance: decimal,
    crossWalletBalance: decimal,
    balanceChange: decimal,
  })
  .strict();

export const positionUpdateSchema = z
  .object({
    symbol,
    positionAmount: decimal,
    entryPrice: decimal,
    accumulatedRealized: decimal,
    unrealizedPnL: decimal,
    marginType: z.enum(["isolated", "cross"]),
    isolatedWallet: decimal,
    positionSide: side,
  })
  .strict();

export const accountUpdateSchema = z
  .object({
    reason: z.enum(["ORDER", "FUND"]),
    balances: z.array(balanceUpdateSchema).max(1000),
    positions: z.array(positionUpdateSchema).max(10000),
  })
  .strict();

export const userEventSchema = z
  .object({
    type: z.enum(["ORDER_TRADE_UPDATE", "ACCOUNT_UPDATE"]),
    eventTime: z.number().int().positive().max(Number.MAX_SAFE_INTEGER),
    transactionTime: z.number().int().positive().max(Number.MAX_SAFE_INTEGER),
    orderUpdate: orderUpdateSchema.optional(),
    accountUpdate: accountUpdateSchema.optional(),
  })
  .strict();

export const storedEventSchema = z
  .object({
    event: userEventSchema,
    receivedAt: z.iso.datetime({ offset: true }),
  })
  .strict();

export const eventsResponseSchema = z
  .object({
    events: z.array(storedEventSchema).max(200),
    truncated: z.boolean(),
  })
  .strict();

export type StoredEvent = z.infer<typeof storedEventSchema>;
export type UserEvent = z.infer<typeof userEventSchema>;
