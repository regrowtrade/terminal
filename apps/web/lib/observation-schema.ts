import { z } from "zod";

const numericFields = [
  "walletBalance",
  "unrealizedProfit",
  "availableBalance",
  "positionAmt",
  "entryPrice",
  "markPrice",
  "unRealizedProfit",
  "liquidationPrice",
  "origQty",
  "executedQty",
  "price",
  "stopPrice",
  "activatePrice",
  "priceRate",
  "quantity",
  "triggerPrice",
  "callbackRate",
  "priceFilter.min",
  "priceFilter.max",
  "priceFilter.step",
  "lotSize.min",
  "lotSize.max",
  "lotSize.step",
  "marketLotSize.min",
  "marketLotSize.max",
  "marketLotSize.step",
  "minNotional",
  "multiplierUp",
  "multiplierDown",
] as const;
const fieldSchema = z.enum([
  ...numericFields,
  "clientOrderId",
  "clientAlgoId",
  "side",
  "positionSide",
  "type",
  "status",
  "reduceOnly",
  "closePosition",
  "workingType",
  "algoType",
  "orderType",
  "algoStatus",
  "contractType",
  "baseAsset",
  "quoteAsset",
  "marginAsset",
  "maxNumOrders",
  "maxNumAlgoOrders",
  "unrecognizedFilterTypes",
]);
const numericNames = new Set<string>(numericFields);
const decimal = /^-?(?:0|[1-9][0-9]{0,37})(?:\.[0-9]{1,18})?$/;
const fractionalSeconds = /\.(\d+)(?=Z|[+-]\d{2}:\d{2}$)/;
const timestamp = z.iso
  .datetime({ offset: true })
  .refine(
    (value) => (value.match(fractionalSeconds)?.[1].length ?? 0) <= 9,
    "Timestamp exceeds nanosecond precision",
  );
// Date.parse truncates sub-millisecond precision. Preserve PostgreSQL's
// microseconds (and Go's nanoseconds) when validating ordering and overlap.
function nanoseconds(value: string): bigint | null {
  const fraction = value.match(fractionalSeconds)?.[1] ?? "";
  const milliseconds = Date.parse(value.replace(fractionalSeconds, ""));
  if (!Number.isFinite(milliseconds)) return null;
  return (
    BigInt(milliseconds) * 1_000_000n +
    BigInt(fraction.slice(0, 9).padEnd(9, "0"))
  );
}
const fieldChangeSchema = z
  .object({
    field: fieldSchema,
    before: z.string().max(4096).nullable(),
    after: z.string().max(4096).nullable(),
  })
  .strict()
  .refine(
    (f) =>
      (f.before !== null || f.after !== null) &&
      (!numericNames.has(f.field) ||
        [f.before, f.after].every(
          (value) => value === null || decimal.test(value),
        )),
    "Invalid field comparison",
  );
const changeSchema = z
  .object({
    entity: z.enum(["asset", "position", "order", "algo", "instrument"]),
    key: z.string().min(1).max(80),
    kind: z.enum(["observed", "not_observed", "changed"]),
    fields: z.array(fieldChangeSchema).min(1).max(32),
  })
  .strict()
  .refine(
    (c) =>
      new Set(c.fields.map((f) => f.field)).size === c.fields.length &&
      c.fields.every((f) =>
        c.kind === "observed"
          ? f.before === null && f.after !== null
          : c.kind === "not_observed"
            ? f.before !== null && f.after === null
            : true,
      ),
    "Invalid observation change",
  );

export const observationReportSchema = z
  .object({
    schemaVersion: z.literal(1),
    mode: z.literal("rest_observation_comparison"),
    status: z.enum(["initial_observation", "compared"]),
    previousCapturedAt: timestamp.nullable(),
    currentStartedAt: timestamp,
    currentCapturedAt: timestamp,
    intervalRelation: z.enum(["no_baseline", "disjoint", "overlapping"]),
    continuity: z.literal("not_established"),
    executionReady: z.literal(false),
    totalChanges: z.number().int().min(0).max(82000),
    detailLimit: z.literal(200),
    truncated: z.boolean(),
    changes: z.array(changeSchema).max(200),
  })
  .strict()
  .refine((r) => {
    const start = nanoseconds(r.currentStartedAt);
    const captured = nanoseconds(r.currentCapturedAt);
    const previous =
      r.previousCapturedAt === null ? null : nanoseconds(r.previousCapturedAt);
    if (
      start === null ||
      captured === null ||
      start > captured ||
      captured - start > 30_000_000_000n
    )
      return false;
    if (
      r.status === "initial_observation" &&
      (r.previousCapturedAt !== null ||
        r.intervalRelation !== "no_baseline" ||
        r.totalChanges !== 0)
    )
      return false;
    if (
      r.status === "compared" &&
      (previous === null ||
        captured <= previous ||
        r.intervalRelation !== (start < previous ? "overlapping" : "disjoint"))
    )
      return false;
    return (
      r.totalChanges >= r.changes.length &&
      r.truncated === r.totalChanges > r.changes.length &&
      (!r.truncated || r.changes.length === 200) &&
      new Set(r.changes.map((c) => `${c.entity}:${c.key}`)).size ===
        r.changes.length
    );
  }, "Invalid observation report");
export const storedComparisonSchema = z
  .object({
    report: observationReportSchema,
    storedAt: timestamp,
  })
  .strict();
export type StoredComparison = z.infer<typeof storedComparisonSchema>;
