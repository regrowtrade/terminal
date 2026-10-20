import { z } from "zod";
import { planRequestSchema, planSchema } from "./plans";

export const savedPlanSchema = z
  .strictObject({
    campaignId: z.uuid(),
    planId: z.uuid(),
    version: z.number().int().positive(),
  })
  .strict();

const riskAmount = z.string().regex(/^-?[0-9]{1,80}(?:\.[0-9]{1,200})?$/);
export const storedPlanSchema = z
  .strictObject({
    campaignId: z.uuid(),
    planId: z.uuid(),
    version: z.number().int().positive(),
    mode: z.enum(["clean", "dca", "hedge"]),
    symbol: z.string(),
    riskAmount,
    parameters: planRequestSchema,
    plan: planSchema,
    createdAt: z.iso.datetime({ offset: true }),
  })
  .strict();

export const storedIntentSchema = z
  .strictObject({
    id: z.uuid(),
    planId: z.uuid(),
    clientOrderId: z.string().min(1),
    exchangeOrderId: z.string().nullable(),
    family: z.enum(["regular", "conditional"]),
    purpose: z.enum([
      "ENTRY",
      "REBUY",
      "TP",
      "SL",
      "HEDGE_CLOSE",
      "EMERGENCY_CLOSE",
    ]),
    positionSide: z.enum(["LONG", "SHORT"]),
    status: z.string().min(1),
    quantity: z.string(),
    price: z.string().nullable(),
  })
  .strict();

export type SavedPlan = z.infer<typeof savedPlanSchema>;
export type StoredPlan = z.infer<typeof storedPlanSchema>;
export type StoredIntent = z.infer<typeof storedIntentSchema>;
