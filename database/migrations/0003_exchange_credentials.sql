-- Envelope only: API keys, secrets, KEKs and raw snapshots must not be stored.
CREATE TABLE exchange_credentials (
  account_id uuid PRIMARY KEY,
  owner_id uuid NOT NULL,
  envelope jsonb NOT NULL CHECK (jsonb_typeof(envelope) = 'object' AND octet_length(envelope::text) <= 16384),
  updated_at timestamptz NOT NULL DEFAULT now(),
  FOREIGN KEY (account_id, owner_id) REFERENCES exchange_accounts(id, owner_id)
);
ALTER TABLE exchange_credentials ENABLE ROW LEVEL SECURITY;
ALTER TABLE exchange_credentials FORCE ROW LEVEL SECURITY;
CREATE POLICY owner_isolation ON exchange_credentials
  USING (owner_id::text = current_setting('app.user_id', true))
  WITH CHECK (owner_id::text = current_setting('app.user_id', true));
