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
    type: z.enum(["LIMIT", "MARKET"]),
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
    closePosition: z.literal(false),
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
      average = new Decimal(o.avgPrice);
    if (
      !qty.gt(0) ||
      executed.gt(qty) ||
      executed.gt(0) !== average.gt(0) ||
      (o.type === "LIMIT" && !new Decimal(o.price).gt(0))
    )
      return false;
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
