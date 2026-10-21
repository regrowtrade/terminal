import { z } from "zod";
import Decimal from "decimal.js";
import { orderLookupSchema } from "./order-lookup-schema";

const positive = z
  .string()
  .regex(/^(?:0|[1-9][0-9]{0,37})(?:\.[0-9]{1,18})?$/)
  .refine((v) => /[1-9]/.test(v));

const expectedSchema = z.strictObject({
  intentId: z.uuid(),
  planId: z.uuid(),
  planFingerprint: z.string().regex(/^[a-f0-9]{64}$/),
  symbol: z.string().regex(/^[A-Z0-9]{2,24}USDT$/),
  clientOrderId: z.string().regex(/^rt[a-f0-9]{30}$/),
  purpose: z.enum(["ENTRY", "REBUY"]),
  positionSide: z.enum(["LONG", "SHORT"]),
  quantity: positive,
  price: positive,
});

export const intentObservationSchema = z
  .strictObject({
    id: z.uuid(),
    storedAt: z.iso.datetime({ offset: true }),
    report: z.strictObject({
      expected: expectedSchema,
      observation: orderLookupSchema,
      assessment: z.enum(["attributes_match", "mismatch", "unresolved"]),
      mismatches: z
        .array(
          z.enum([
            "type",
            "side",
            "positionSide",
            "quantity",
            "price",
            "reduceOnly",
          ]),
        )
        .max(6),
      executionReady: z.literal(false),
      ownershipConfirmed: z.literal(false),
    }),
  })
  .refine(({ report: r }) => {
    const e = r.expected,
      observation = r.observation;
    if (
      observation.query.symbol !== e.symbol ||
      observation.query.clientOrderId !== e.clientOrderId ||
      observation.query.orderId !== undefined
    )
      return false;
    if (observation.outcome === "unresolved")
      return r.assessment === "unresolved" && r.mismatches.length === 0;
    const o = observation.order;
    if (!o) return false;
    if (
      ![e.quantity, e.price, o.origQty, o.price].every(
        (v) => positive.safeParse(v).success,
      )
    )
      return false;
    const checks: [string, boolean][] = [
      ["type", o.type === "LIMIT"],
      ["side", o.side === (e.positionSide === "LONG" ? "BUY" : "SELL")],
      ["positionSide", o.positionSide === e.positionSide],
      ["quantity", new Decimal(o.origQty).eq(e.quantity)],
      ["price", new Decimal(o.price).eq(e.price)],
      ["reduceOnly", !o.reduceOnly],
    ];
    const mismatches = checks
      .filter(([, match]) => !match)
      .map(([field]) => field);
    return (
      r.assessment === (mismatches.length ? "mismatch" : "attributes_match") &&
      JSON.stringify(mismatches) === JSON.stringify(r.mismatches)
    );
  });

export type IntentObservation = z.infer<typeof intentObservationSchema>;
