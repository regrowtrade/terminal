-- Read-only comparison evidence. No intent transitions or order submission.
BEGIN;
ALTER TABLE order_intents ADD CONSTRAINT intent_plan_account_owner UNIQUE (id, plan_id, account_id, owner_id);
CREATE TABLE binance_intent_observations (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  account_id uuid NOT NULL,
  owner_id uuid NOT NULL,
  intent_id uuid NOT NULL,
  plan_id uuid NOT NULL,
  stored_at timestamptz NOT NULL DEFAULT clock_timestamp(),
  report jsonb NOT NULL CHECK (
    jsonb_typeof(report) = 'object'
    AND octet_length(report::text) <= 32768
    AND report @> '{"executionReady":false,"ownershipConfirmed":false}'::jsonb
    AND report ?& ARRAY['expected','observation','assessment','mismatches']
    AND jsonb_typeof(report->'expected') = 'object'
    AND (report->'expected') @> jsonb_build_object('intentId',intent_id::text,'planId',plan_id::text)
    AND jsonb_typeof(report->'observation') = 'object'
    AND (report->'observation') @> '{"source":"binance-production","mode":"read_only","executionReady":false}'::jsonb
    AND jsonb_typeof(report->'mismatches') = 'array'
    AND jsonb_typeof(report->'assessment') = 'string'
    AND report->>'assessment' IN ('attributes_match','mismatch','unresolved')
    AND (
      (report->>'assessment' = 'unresolved' AND report->'mismatches' = '[]'::jsonb AND (report->'observation') @> '{"outcome":"unresolved","reason":"exchange_not_found","order":null}'::jsonb)
      OR (report->>'assessment' = 'attributes_match' AND report->'mismatches' = '[]'::jsonb AND report->'observation'->>'outcome' = 'observed')
      OR (report->>'assessment' = 'mismatch' AND jsonb_array_length(report->'mismatches') > 0 AND report->'observation'->>'outcome' = 'observed')
    )
  ),
  FOREIGN KEY (intent_id, plan_id, account_id, owner_id) REFERENCES order_intents(id, plan_id, account_id, owner_id)
);
CREATE INDEX binance_intent_observations_intent_time ON binance_intent_observations(account_id, intent_id, stored_at DESC, id);
ALTER TABLE binance_intent_observations ENABLE ROW LEVEL SECURITY;
ALTER TABLE binance_intent_observations FORCE ROW LEVEL SECURITY;
CREATE POLICY owner_isolation ON binance_intent_observations
  USING (owner_id::text = current_setting('app.user_id', true))
  WITH CHECK (owner_id::text = current_setting('app.user_id', true));
COMMIT;
