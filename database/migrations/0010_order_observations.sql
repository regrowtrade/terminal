-- Append-only observations from explicit normal-order REST queries. A missing
-- order remains unresolved; these records never create fills or enable trading.
BEGIN;
CREATE TABLE binance_order_observations (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  account_id uuid NOT NULL,
  owner_id uuid NOT NULL,
  started_at timestamptz NOT NULL,
  checked_at timestamptz NOT NULL CHECK (checked_at >= started_at),
  stored_at timestamptz NOT NULL DEFAULT clock_timestamp(),
  observation jsonb NOT NULL CHECK (
    jsonb_typeof(observation) = 'object'
    AND octet_length(observation::text) <= 16384
    AND observation @> '{"source":"binance-production","mode":"read_only","executionReady":false}'::jsonb
    AND observation ?& ARRAY['query','outcome','reason','order','startedAt','checkedAt']
    AND jsonb_typeof(observation->'query') = 'object'
    AND jsonb_typeof(observation->'startedAt') = 'string'
    AND jsonb_typeof(observation->'checkedAt') = 'string'
    AND jsonb_typeof(observation->'outcome') = 'string'
    AND jsonb_typeof(observation->'reason') = 'string'
    AND (observation->>'startedAt')::timestamptz = started_at
    AND (observation->>'checkedAt')::timestamptz = checked_at
    AND (
      (observation->>'outcome' = 'observed' AND observation->>'reason' = '' AND jsonb_typeof(observation->'order') = 'object')
      OR (observation->>'outcome' = 'unresolved' AND observation->>'reason' = 'exchange_not_found' AND observation->'order' = 'null'::jsonb)
    )
  ),
  FOREIGN KEY (account_id, owner_id) REFERENCES exchange_accounts(id, owner_id)
);
CREATE INDEX binance_order_observations_account_time ON binance_order_observations(account_id, checked_at, id);
ALTER TABLE binance_order_observations ENABLE ROW LEVEL SECURITY;
ALTER TABLE binance_order_observations FORCE ROW LEVEL SECURITY;
CREATE POLICY owner_isolation ON binance_order_observations
  USING (owner_id::text = current_setting('app.user_id', true))
  WITH CHECK (owner_id::text = current_setting('app.user_id', true));
COMMIT;
