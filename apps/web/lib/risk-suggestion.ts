import type { PlanRequest } from "./plans";

const scale = 10n ** 12n;
function units(value: string): bigint {
  if (!/^-?(?:0|[1-9][0-9]{0,15})(?:\.[0-9]{1,12})?$/.test(value))
    throw new Error("Invalid decimal");
  const negative = value.startsWith("-");
  const [integer, fraction = ""] = value.replace("-", "").split(".");
  const result = BigInt(integer) * scale + BigInt(fraction.padEnd(12, "0"));
  return negative ? -result : result;
}
function formatted(value: bigint, places: number) {
  const negative = value < 0n;
  const digits = (negative ? -value : value)
    .toString()
    .padStart(places + 1, "0");
  const fraction = digits.slice(-places).replace(/0+$/, "");
  return `${negative ? "-" : ""}${digits.slice(0, -places)}${fraction ? `.${fraction}` : ""}`;
}

// Informational UI suggestion only. The Go engine independently calculates the confirmed snapshot.
export function riskSuggestion(
  hedge: NonNullable<PlanRequest["hedge"]>,
  unit: PlanRequest["riskUnit"],
  account: string,
) {
  const net =
    (units(hedge.realizedPnl) -
      units(hedge.feesPaid) +
      units(hedge.fundingNet)) *
      scale +
    (units(hedge.markPrice) - units(hedge.longEntry)) *
      units(hedge.longQuantity) +
    (units(hedge.shortEntry) - units(hedge.markPrice)) *
      units(hedge.shortQuantity);
  const reference = units(account);
  if (reference <= 0n) throw new Error("Invalid reference");
  const loss = net < 0n ? -net : 0n;
  // Round the editable suggestion up at 12 places, rather than silently losing tiny amounts.
  const suggested =
    unit === "amount"
      ? (loss + scale - 1n) / scale
      : (loss * 100n + reference - 1n) / reference;
  return { net: formatted(net, 24), suggested: formatted(suggested, 12) };
}
