import { z } from "zod";

const decimal = z
  .string()
  .regex(
    /^-?(?:0|[1-9][0-9]{0,15})(?:\.[0-9]{1,12})?$/,
    "Usa un decimal válido, sin separadores de miles.",
  );
const positive = decimal.refine(
  (v) => !v.startsWith("-") && /[1-9]/.test(v),
  "Debe ser mayor que cero.",
);
const nonnegative = decimal.refine(
  (v) => !v.startsWith("-"),
  "No puede ser negativo.",
);
export const planRequestSchema = z
  .strictObject({
    mode: z.enum(["clean", "dca", "hedge"]),
    symbol: z.string().regex(/^[A-Z0-9]{2,20}USDT$/),
    side: z.enum(["LONG", "SHORT"]),
    entry: positive,
    quantity: positive,
    risk: positive,
    rewardRatio: positive,
    riskUnit: z.enum(["amount", "percent"]),
    accountReference: positive,
    accountBasis: z.enum(["equity", "wallet"]),
    spacingPercent: positive,
    leverage: z.number().int().min(1).max(125),
    maxLevels: z.number().int().min(1).max(11),
    maxMargin: positive,
    costReserve: nonnegative,
    hedge: z
      .strictObject({
        longEntry: positive,
        shortEntry: positive,
        longQuantity: positive,
        shortQuantity: positive,
        markPrice: positive,
        realizedPnl: decimal,
        feesPaid: nonnegative,
        fundingNet: decimal,
        closePercent: z.enum(["5", "10"]),
      })
      .optional(),
  })
  .superRefine((data, ctx) => {
    if (data.mode === "hedge" && !data.hedge)
      ctx.addIssue({
        code: "custom",
        message: "Faltan datos de cobertura.",
        path: ["hedge"],
      });
    if (data.mode !== "hedge" && data.hedge)
      ctx.addIssue({
        code: "custom",
        message: "Datos de cobertura incompatibles.",
        path: ["hedge"],
      });
  });
export type PlanRequest = z.infer<typeof planRequestSchema>;

const calculatedDecimal = z
  .string()
  .regex(/^-?[0-9]{1,80}(?:\.[0-9]{1,200})?$/);
const level = z.strictObject({
  index: z.number().int().positive(),
  price: calculatedDecimal,
  quantity: calculatedDecimal,
  cumulativeQuantity: calculatedDecimal,
  averagePrice: calculatedDecimal,
  margin: calculatedDecimal,
  stopPrice: calculatedDecimal.nullable(),
  takeProfit: calculatedDecimal.nullable(),
  takeProfitQuantity: calculatedDecimal,
  netExposure: calculatedDecimal,
});
export const planSchema = z.strictObject({
  mode: z.enum(["clean", "dca", "hedge"]),
  symbol: z.string(),
  side: z.enum(["LONG", "SHORT"]),
  simulation: z.literal(true),
  risk: calculatedDecimal,
  costReserve: calculatedDecimal,
  riskInput: decimal,
  riskUnit: z.enum(["amount", "percent"]),
  accountReference: decimal,
  accountBasis: z.enum(["equity", "wallet"]),
  baselinePnl: calculatedDecimal.nullable(),
  pnlThreshold: calculatedDecimal.nullable(),
  spacingPercent: calculatedDecimal,
  levels: z.array(level).min(1).max(11),
  warnings: z.array(z.string()),
  stopReason: z.string(),
});
export type Plan = z.infer<typeof planSchema>;
export const defaultRequest: PlanRequest = {
  mode: "clean",
  symbol: "BTCUSDT",
  side: "LONG",
  entry: "100",
  quantity: "1",
  risk: "2",
  rewardRatio: "3",
  spacingPercent: "4",
  leverage: 10,
  maxLevels: 4,
  maxMargin: "100",
  costReserve: "0",
  riskUnit: "amount",
  accountReference: "100",
  accountBasis: "equity",
};
