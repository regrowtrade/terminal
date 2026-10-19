-- Explicit administrative retirement of pre-exchange-cutoff baselines. Creating
-- this table does not move any data. Runtime has no archive privileges.
BEGIN;
CREATE TABLE exchange_read_snapshot_archives (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  account_id uuid NOT NULL,
  owner_id uuid NOT NULL,
  started_at timestamptz NOT NULL,
  captured_at timestamptz NOT NULL CHECK (captured_at >= started_at),
  stored_at timestamptz NOT NULL,
  snapshot jsonb NOT NULL CHECK (
    jsonb_typeof(snapshot) = 'object'
    AND NOT (snapshot ? 'capturedAtMs')
    AND octet_length(snapshot::text) <= 8388608
  ),
  comparison jsonb,
  reason text NOT NULL CHECK (reason = 'missing_exchange_cutoff'),
  archived_at timestamptz NOT NULL DEFAULT clock_timestamp(),
  UNIQUE (account_id, captured_at),
  FOREIGN KEY (account_id, owner_id) REFERENCES exchange_accounts(id, owner_id)
);
ALTER TABLE exchange_read_snapshot_archives ENABLE ROW LEVEL SECURITY;
ALTER TABLE exchange_read_snapshot_archives FORCE ROW LEVEL SECURITY;
CREATE POLICY owner_isolation ON exchange_read_snapshot_archives
  USING (owner_id::text = current_setting('app.user_id', true))
  WITH CHECK (owner_id::text = current_setting('app.user_id', true));
COMMIT;
