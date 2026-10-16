-- Durable append-only journal of the read-only user data stream. Events are
-- deduplicated by (account, key); nothing here infers fills, continuity or
-- execution. This is raw material for a later reconciliation step.
BEGIN;
CREATE TABLE binance_user_events (
  account_id uuid NOT NULL,
  owner_id uuid NOT NULL,
  dedup_key text NOT NULL CHECK (length(dedup_key) = 64),
  event_type text NOT NULL CHECK (event_type IN ('ORDER_TRADE_UPDATE', 'ACCOUNT_UPDATE')),
  event_time bigint NOT NULL CHECK (event_time > 0 AND event_time <= 9007199254740991),
  transaction_time bigint NOT NULL CHECK (transaction_time > 0 AND transaction_time <= 9007199254740991),
  payload jsonb NOT NULL CHECK (
    jsonb_typeof(payload) = 'object'
    AND octet_length(payload::text) <= 65536
    AND payload ?& ARRAY['type', 'eventTime', 'transactionTime']
    AND (payload->>'type') IN ('ORDER_TRADE_UPDATE', 'ACCOUNT_UPDATE')
  ),
  received_at timestamptz NOT NULL DEFAULT clock_timestamp(),
  PRIMARY KEY (account_id, dedup_key),
  FOREIGN KEY (account_id, owner_id) REFERENCES exchange_accounts(id, owner_id)
);
CREATE INDEX binance_user_events_account_time
  ON binance_user_events (account_id, event_time, transaction_time);
ALTER TABLE binance_user_events ENABLE ROW LEVEL SECURITY;
ALTER TABLE binance_user_events FORCE ROW LEVEL SECURITY;
CREATE POLICY owner_isolation ON binance_user_events
  USING (owner_id::text = current_setting('app.user_id', true))
  WITH CHECK (owner_id::text = current_setting('app.user_id', true));
COMMIT;
