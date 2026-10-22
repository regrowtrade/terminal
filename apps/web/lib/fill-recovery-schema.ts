import { z } from "zod";

const symbol = z.string().regex(/^[A-Z0-9]{2,24}USDT$/);
const windowMs = 7 * 24 * 60 * 60 * 1000;

export const fillRecoveryRequestSchema = z
  .object({
    accountId: z.uuid(),
    symbol,
    startTime: z.number().int().positive().max(Number.MAX_SAFE_INTEGER),
    endTime: z.number().int().positive().max(Number.MAX_SAFE_INTEGER),
  })
  .strict()
  .refine(
    (q) => q.endTime > q.startTime && q.endTime - q.startTime <= windowMs,
  );

export const fillRecoverySchema = z
  .object({
    source: z.literal("binance-production"),
    mode: z.literal("read_only"),
    executionReady: z.literal(false),
    symbol,
    startTime: z.number().int().positive().max(Number.MAX_SAFE_INTEGER),
    endTime: z.number().int().positive().max(Number.MAX_SAFE_INTEGER),
    recovered: z.number().int().nonnegative().max(20000),
    duplicates: z.number().int().nonnegative().max(20000),
    truncated: z.boolean(),
  })
  .strict()
  .refine(
    (r) =>
      r.endTime > r.startTime &&
      r.endTime - r.startTime <= windowMs &&
      r.recovered + r.duplicates <= 20000,
  );
