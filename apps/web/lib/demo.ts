// Synthetic fixtures only. Never mix these values with exchange/account data.
export const examplePositions = [
  {
    id: "example-btc-long",
    symbol: "BTCUSDT",
    asset: "BTC",
    side: "LONG" as const,
    entry: 100,
    mark: 103,
    quantity: 1,
    pnl: 3,
    strategy: "Entrada limpia",
    color: "orange",
  },
  {
    id: "example-eth-long",
    symbol: "ETHUSDT",
    asset: "ETH",
    side: "LONG" as const,
    entry: 100,
    mark: 99,
    quantity: 1,
    pnl: -1,
    strategy: "Cobertura",
    color: "purple",
  },
  {
    id: "example-eth-short",
    symbol: "ETHUSDT",
    asset: "ETH",
    side: "SHORT" as const,
    entry: 98,
    mark: 99,
    quantity: 1,
    pnl: -1,
    strategy: "Cobertura",
    color: "purple",
  },
];
export const money = (n: number, digits = 2) =>
  new Intl.NumberFormat("es", {
    minimumFractionDigits: digits,
    maximumFractionDigits: digits,
  }).format(n);
