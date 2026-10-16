-- Only the latest complete manual observation is retained per account. This is
-- not an execution ledger, reconciliation checkpoint or claim of live data.
BEGIN;
CREATE TABLE exchange_read_snapshots (
  account_id uuid PRIMARY KEY,
  owner_id uuid NOT NULL,
  started_at timestamptz NOT NULL,
  captured_at timestamptz NOT NULL CHECK (captured_at >= started_at),
  stored_at timestamptz NOT NULL DEFAULT clock_timestamp(),
  snapshot jsonb NOT NULL CHECK (
    jsonb_typeof(snapshot) = 'object'
    AND octet_length(snapshot::text) <= 8388608
    AND snapshot @> '{"source":"binance-production","mode":"read_only","consistency":"rest_non_atomic","executionReady":false}'::jsonb
    AND snapshot ?& ARRAY['startedAt','capturedAt','assets','positions','openOrders','algoOrders','instruments']
    AND jsonb_typeof(snapshot->'startedAt') = 'string'
    AND jsonb_typeof(snapshot->'capturedAt') = 'string'
    AND jsonb_typeof(snapshot->'assets') = 'array'
    AND jsonb_typeof(snapshot->'positions') = 'array'
    AND jsonb_typeof(snapshot->'openOrders') = 'array'
    AND jsonb_typeof(snapshot->'algoOrders') = 'array'
    AND jsonb_typeof(snapshot->'instruments') = 'array'
    AND (snapshot->>'startedAt')::timestamptz = started_at
    AND (snapshot->>'capturedAt')::timestamptz = captured_at
  ),
  FOREIGN KEY (account_id, owner_id) REFERENCES exchange_accounts(id, owner_id)
);
ALTER TABLE exchange_read_snapshots ENABLE ROW LEVEL SECURITY;
ALTER TABLE exchange_read_snapshots FORCE ROW LEVEL SECURITY;
CREATE POLICY owner_isolation ON exchange_read_snapshots
  USING (owner_id::text = current_setting('app.user_id', true))
  WITH CHECK (owner_id::text = current_setting('app.user_id', true));
COMMIT;
