-- Durable deduplicated executions observed in the read-only stream. Each row is
-- one TRADE fill keyed by (account, order, trade) so at-least-once redelivery
-- never duplicates it. Amounts remain original decimal strings; nothing here is
-- a reconciled ledger or execution readiness.
BEGIN;
CREATE TABLE binance_fills (
  account_id uuid NOT NULL,
  owner_id uuid NOT NULL,
  order_id text NOT NULL CHECK (order_id ~ '^[1-9][0-9]{0,18}$'),
  trade_id text NOT NULL CHECK (trade_id ~ '^[1-9][0-9]{0,18}$'),
  symbol text NOT NULL CHECK (symbol ~ '^[A-Z0-9]{2,24}USDT$'),
  side text NOT NULL CHECK (side IN ('BUY', 'SELL')),
  position_side text NOT NULL CHECK (position_side IN ('BOTH', 'LONG', 'SHORT')),
  price text NOT NULL,
  quantity text NOT NULL,
  commission text NOT NULL,
  commission_asset text NOT NULL CHECK (length(commission_asset) <= 20),
  event_time bigint NOT NULL CHECK (event_time > 0 AND event_time <= 9007199254740991),
  transaction_time bigint NOT NULL CHECK (transaction_time > 0 AND transaction_time <= 9007199254740991),
  received_at timestamptz NOT NULL DEFAULT clock_timestamp(),
  PRIMARY KEY (account_id, order_id, trade_id),
  FOREIGN KEY (account_id, owner_id) REFERENCES exchange_accounts(id, owner_id)
);
CREATE INDEX binance_fills_account_time
  ON binance_fills (account_id, event_time, transaction_time);
ALTER TABLE binance_fills ENABLE ROW LEVEL SECURITY;
ALTER TABLE binance_fills FORCE ROW LEVEL SECURITY;
CREATE POLICY owner_isolation ON binance_fills
  USING (owner_id::text = current_setting('app.user_id', true))
  WITH CHECK (owner_id::text = current_setting('app.user_id', true));
COMMIT;
