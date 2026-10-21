import { z } from "zod";
import Decimal from "decimal.js";

const symbol = z.string().regex(/^[A-Z0-9]{2,24}USDT$/);
const orderId = z
  .string()
  .regex(/^[1-9][0-9]{0,18}$/)
  .refine(
    (id) => /^[1-9][0-9]{0,18}$/.test(id) && BigInt(id) <= 9223372036854775807n,
  );
const clientOrderId = z.string().regex(/^[.A-Z:/a-z0-9_-]{1,36}$/);
const nonnegative = z
  .string()
  .regex(/^(?:0|[1-9][0-9]{0,37})(?:\.[0-9]{1,18})?$/);
export const orderQuerySchema = z
  .object({
    symbol,
    orderId: orderId.optional(),
    clientOrderId: clientOrderId.optional(),
  })
  .strict()
  .refine(
    (q) => (q.orderId !== undefined) !== (q.clientOrderId !== undefined),
    "Choose exactly one identifier",
  );
export const orderLookupRequestSchema = z
  .object({
    accountId: z.uuid(),
    ...orderQuerySchema.shape,
  })
  .strict()
  .refine(
    (q) =>
      orderQuerySchema.safeParse({
        symbol: q.symbol,
        orderId: q.orderId,
        clientOrderId: q.clientOrderId,
      }).success,
  );

const queriedOrderSchema = z
  .object({
    orderId,
    clientOrderId,
    symbol,
    side: z.enum(["BUY", "SELL"]),
    positionSide: z.enum(["BOTH", "LONG", "SHORT"]),
    type: z.enum([
      "LIMIT",
      "MARKET",
      "STOP",
      "STOP_MARKET",
      "TAKE_PROFIT",
      "TAKE_PROFIT_MARKET",
      "TRAILING_STOP_MARKET",
    ]),
    status: z.enum([
      "NEW",
      "PARTIALLY_FILLED",
      "FILLED",
      "CANCELED",
      "EXPIRED",
      "EXPIRED_IN_MATCH",
      "REJECTED",
      "PENDING_CANCEL",
    ]),
    origQty: nonnegative,
    executedQty: nonnegative,
    price: nonnegative,
    avgPrice: nonnegative,
    reduceOnly: z.boolean(),
    closePosition: z.boolean(),
    stopPrice: nonnegative.optional(),
    workingType: z.enum(["MARK_PRICE", "CONTRACT_PRICE"]).optional(),
    activatePrice: nonnegative.optional(),
    priceRate: nonnegative.optional(),
    updateTime: z.number().int().positive().max(Number.MAX_SAFE_INTEGER),
  })
  .strict()
  .refine((o) => {
    if (
      ![o.origQty, o.executedQty, o.price, o.avgPrice].every(
        (v) => nonnegative.safeParse(v).success,
      )
    )
      return false;
    const qty = new Decimal(o.origQty),
      executed = new Decimal(o.executedQty),
      price = new Decimal(o.price),
      average = new Decimal(o.avgPrice);
    if (!qty.gt(0) || executed.gt(qty) || executed.gt(0) !== average.gt(0))
      return false;
    const stop = o.stopPrice ?? "";
    const working = o.workingType ?? "";
    const activate = o.activatePrice ?? "";
    const rate = o.priceRate ?? "";
    if (o.type === "LIMIT" || o.type === "MARKET") {
      if (o.closePosition) return false;
      if (o.type === "LIMIT" && !price.gt(0)) return false;
      if (
        o.stopPrice !== undefined ||
        o.workingType !== undefined ||
        o.activatePrice !== undefined ||
        o.priceRate !== undefined
      )
        return false;
    } else {
      if (working !== "MARK_PRICE" && working !== "CONTRACT_PRICE")
        return false;
      if (!nonnegative.safeParse(stop).success) return false;
      if (o.type === "TRAILING_STOP_MARKET") {
        if (!new Decimal(activate).gt(0) || !new Decimal(rate).gt(0))
          return false;
        const r = new Decimal(rate);
        if (r.lt("0.1") || r.gt("10")) return false;
      } else {
        if (!new Decimal(stop).gt(0)) return false;
        if (activate !== "" && !nonnegative.safeParse(activate).success)
          return false;
        if (rate !== "" && !nonnegative.safeParse(rate).success) return false;
      }
      if ((o.type === "STOP" || o.type === "TAKE_PROFIT") && !price.gt(0))
        return false;
      if (
        o.closePosition &&
        o.type !== "STOP_MARKET" &&
        o.type !== "TAKE_PROFIT_MARKET"
      )
        return false;
    }
    if (o.status === "NEW" || o.status === "REJECTED") return executed.isZero();
    if (o.status === "PARTIALLY_FILLED")
      return executed.gt(0) && executed.lt(qty);
    if (o.status === "FILLED") return executed.eq(qty);
    return true;
  });

export const orderLookupSchema = z
  .object({
    source: z.literal("binance-production"),
    mode: z.literal("read_only"),
    executionReady: z.literal(false),
    startedAt: z.iso.datetime({ offset: true }),
    checkedAt: z.iso.datetime({ offset: true }),
    query: orderQuerySchema,
    outcome: z.enum(["observed", "unresolved"]),
    reason: z.enum(["", "exchange_not_found"]),
    order: queriedOrderSchema.nullable(),
  })
  .strict()
  .refine((r) => {
    const elapsed = Date.parse(r.checkedAt) - Date.parse(r.startedAt);
    if (elapsed < 0 || elapsed > 18_000) return false;
    if (r.outcome === "unresolved")
      return r.reason === "exchange_not_found" && r.order === null;
    return (
      r.reason === "" &&
      r.order !== null &&
      r.order.symbol === r.query.symbol &&
      (!r.query.orderId || r.order.orderId === r.query.orderId) &&
      (!r.query.clientOrderId ||
        r.order.clientOrderId === r.query.clientOrderId)
    );
  });
export type OrderLookup = z.infer<typeof orderLookupSchema>;
