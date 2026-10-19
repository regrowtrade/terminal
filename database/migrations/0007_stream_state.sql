-- Durable per-account stream continuity state. It records the latest exchange
-- event time observed and whether a disconnection happened since the last
-- snapshot, so reconciliation can honestly report established vs gapped. It
-- never infers fills, continuity of fills or execution readiness.
BEGIN;
CREATE TABLE binance_stream_state (
  account_id uuid PRIMARY KEY,
  owner_id uuid NOT NULL,
  connected boolean NOT NULL DEFAULT false,
  last_event_time bigint NOT NULL DEFAULT 0 CHECK (last_event_time >= 0 AND last_event_time <= 9007199254740991),
  gapped_since_snapshot boolean NOT NULL DEFAULT false,
  updated_at timestamptz NOT NULL DEFAULT clock_timestamp(),
  FOREIGN KEY (account_id, owner_id) REFERENCES exchange_accounts(id, owner_id)
);
ALTER TABLE binance_stream_state ENABLE ROW LEVEL SECURITY;
ALTER TABLE binance_stream_state FORCE ROW LEVEL SECURITY;
CREATE POLICY owner_isolation ON binance_stream_state
  USING (owner_id::text = current_setting('app.user_id', true))
  WITH CHECK (owner_id::text = current_setting('app.user_id', true));
COMMIT;
