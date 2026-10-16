export type AccountFilter = {
  query: string;
  positionSide: "all" | "BOTH" | "LONG" | "SHORT";
};
export const allAccountRecords: AccountFilter = {
  query: "",
  positionSide: "all",
};

// Display-only filtering; never changes the complete snapshot or establishes
// strategy ownership. BOTH is Binance's one-way position side, not a hedge.
export function matchesAccountFilter(
  record: { symbol: string; positionSide: string },
  filter: AccountFilter,
) {
  return (
    record.symbol.includes(filter.query.trim().toUpperCase()) &&
    (filter.positionSide === "all" ||
      record.positionSide === filter.positionSide)
  );
}
