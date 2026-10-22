import { z } from "zod";
import Decimal from "decimal.js";

const symbol = z.string().regex(/^[A-Z0-9]{2,24}USDT$/);
const id = z
  .string()
  .regex(/^[1-9][0-9]{0,18}$/)
  .refine(
    (v) => /^[1-9][0-9]{0,18}$/.test(v) && BigInt(v) <= 9223372036854775807n,
  );
const clientId = z.string().regex(/^[.A-Z:/a-z0-9_-]{1,36}$/);
const nonnegative = z
  .string()
  .regex(/^(?:0|[1-9][0-9]{0,37})(?:\.[0-9]{1,18})?$/);
const time = z.number().int().nonnegative().max(Number.MAX_SAFE_INTEGER);

export const algoQuerySchema = z
  .strictObject({
    symbol,
    algoId: id.optional(),
    clientAlgoId: clientId.optional(),
  })
  .refine((q) => (q.algoId !== undefined) !== (q.clientAlgoId !== undefined));
export const algoLookupRequestSchema = z
  .strictObject({ accountId: z.uuid(), ...algoQuerySchema.shape })
  .refine(
    (q) =>
      algoQuerySchema.safeParse({
        symbol: q.symbol,
        algoId: q.algoId,
        clientAlgoId: q.clientAlgoId,
      }).success,
  );

const queriedAlgoSchema = z
  .strictObject({
    algoId: id,
    clientAlgoId: clientId,
    algoType: z.literal("CONDITIONAL"),
    symbol,
    side: z.enum(["BUY", "SELL"]),
    positionSide: z.enum(["BOTH", "LONG", "SHORT"]),
    orderType: z.enum([
      "STOP",
      "STOP_MARKET",
      "TAKE_PROFIT",
      "TAKE_PROFIT_MARKET",
      "TRAILING_STOP_MARKET",
    ]),
    algoStatus: z.enum([
      "NEW",
      "CANCELED",
      "TRIGGERED",
      "FINISHED",
      "REJECTED",
      "EXPIRED",
    ]),
    quantity: nonnegative,
    price: nonnegative,
    triggerPrice: nonnegative,
    reduceOnly: z.boolean(),
    closePosition: z.boolean(),
    workingType: z.enum(["MARK_PRICE", "CONTRACT_PRICE"]),
    activatePrice: nonnegative.optional(),
    callbackRate: nonnegative.optional(),
    updateTime: time.refine((v) => v > 0),
    actualOrderId: z.union([z.literal(""), id]),
    actualPrice: nonnegative,
    actualType: z.enum(["LIMIT", "MARKET"]).optional(),
    actualQty: nonnegative.optional(),
    triggerTime: time,
  })
  .refine((o) => {
    // Refinements can run despite another dirty field: never construct Decimal
    // until every relevant numeric string has passed its lexical bounds.
    const fields = [
      o.quantity,
      o.price,
      o.triggerPrice,
      o.actualPrice,
      o.actualQty ?? "0",
      o.activatePrice ?? "0",
      o.callbackRate ?? "0",
    ];
    if (!fields.every((v) => nonnegative.safeParse(v).success)) return false;
    const qty = new Decimal(o.quantity),
      actualQty = new Decimal(o.actualQty ?? "0"),
      actualPrice = new Decimal(o.actualPrice);
    if (!o.closePosition && !qty.gt(0)) return false;
    if (
      o.closePosition &&
      o.orderType !== "STOP_MARKET" &&
      o.orderType !== "TAKE_PROFIT_MARKET"
    )
      return false;
    if (
      (o.orderType === "STOP" || o.orderType === "TAKE_PROFIT") &&
      !new Decimal(o.price).gt(0)
    )
      return false;
    if (o.orderType === "TRAILING_STOP_MARKET") {
      const rate = new Decimal(o.callbackRate ?? "0");
      if (
        !new Decimal(o.activatePrice ?? "0").gt(0) ||
        rate.lt("0.1") ||
        rate.gt("10")
      )
        return false;
    } else if (!new Decimal(o.triggerPrice).gt(0)) return false;
    if (o.actualOrderId !== "" && o.triggerTime === 0) return false;
    if (
      o.actualOrderId === "" &&
      (actualPrice.gt(0) || actualQty.gt(0) || o.actualType !== undefined)
    )
      return false;
    if (
      o.algoStatus === "NEW" &&
      (o.actualOrderId !== "" || o.triggerTime !== 0)
    )
      return false;
    if (actualQty.gt(0) && !actualPrice.gt(0)) return false;
    return o.closePosition || !actualQty.gt(qty);
  });

export const algoLookupSchema = z
  .strictObject({
    source: z.literal("binance-production"),
    mode: z.literal("read_only"),
    family: z.literal("algo"),
    executionReady: z.literal(false),
    startedAt: z.iso.datetime({ offset: true }),
    checkedAt: z.iso.datetime({ offset: true }),
    query: algoQuerySchema,
    outcome: z.enum(["observed", "unresolved"]),
    reason: z.enum(["", "exchange_not_found"]),
    order: queriedAlgoSchema.nullable(),
  })
  .refine((r) => {
    const elapsed = Date.parse(r.checkedAt) - Date.parse(r.startedAt);
    if (elapsed < 0 || elapsed > 18_000) return false;
    if (r.outcome === "unresolved")
      return r.reason === "exchange_not_found" && r.order === null;
    return (
      r.reason === "" &&
      r.order !== null &&
      r.order.symbol === r.query.symbol &&
      (r.query.algoId === undefined || r.query.algoId === r.order.algoId) &&
      (r.query.clientAlgoId === undefined ||
        r.query.clientAlgoId === r.order.clientAlgoId)
    );
  });
export type AlgoLookup = z.infer<typeof algoLookupSchema>;
