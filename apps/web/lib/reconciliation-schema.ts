import { z } from "zod";

// Mirrors the engine's reconciliation report. Amounts stay as decimal strings
// and int64 IDs as strings; nothing here is a reconciled ledger or execution.
const nonnegative = z
  .string()
  .regex(/^(?:0|[1-9][0-9]{0,37})(?:\.[0-9]{1,18})?$/);
const symbol = z.string().regex(/^[A-Z0-9]{2,24}USDT$/);
const side = z.enum(["BOTH", "LONG", "SHORT"]);
const int64Id = z
  .string()
  .regex(/^[1-9][0-9]{0,18}$/)
  .refine(
    (id) => /^[1-9][0-9]{0,18}$/.test(id) && BigInt(id) <= 9223372036854775807n,
  );

export const fillSchema = z
  .object({
    orderId: int64Id,
    tradeId: int64Id,
    symbol,
    side: z.enum(["BUY", "SELL"]),
    positionSide: side,
    price: nonnegative,
    quantity: nonnegative,
    commission: nonnegative,
    commissionAsset: z.string().max(20),
    eventTime: z.number().int().positive().max(Number.MAX_SAFE_INTEGER),
    transactionTime: z.number().int().positive().max(Number.MAX_SAFE_INTEGER),
  })
  .strict();

export const reconciliationReportSchema = z
  .object({
    continuity: z.enum(["established", "gapped"]),
    executionReady: z.literal(false),
    snapshotCapturedAt: z.iso.datetime({ offset: true }),
    eventsTruncated: z.boolean(),
    totalEvents: z.number().int().nonnegative(),
    orderUpdates: z.number().int().nonnegative(),
    accountUpdates: z.number().int().nonnegative(),
    lastEventTime: z.number().int().nonnegative().max(Number.MAX_SAFE_INTEGER),
    totalFills: z.number().int().nonnegative(),
    fillLimit: z.number().int().positive(),
    fillsTruncated: z.boolean(),
    fills: z.array(fillSchema).max(200),
  })
  .strict();

export type ReconciliationReport = z.infer<typeof reconciliationReportSchema>;
