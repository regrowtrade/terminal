import { z } from "zod";

const decimal = z
  .string()
  .regex(/^-?(?:0|[1-9][0-9]{0,37})(?:\.[0-9]{1,18})?$/);
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
            symbol: z.string().regex(/^[A-Z0-9]{2,24}USDT$/),
            positionSide: z.enum(["BOTH", "LONG", "SHORT"]),
            positionAmt: decimal,
            entryPrice: decimal,
            markPrice: decimal,
            unRealizedProfit: decimal,
            liquidationPrice: decimal,
          })
          .strict(),
      )
      .max(10000),
  })
  .strict();
export type ConnectionState = z.infer<typeof connectionStateSchema>;
export type AccountSnapshot = z.infer<typeof snapshotSchema>;
