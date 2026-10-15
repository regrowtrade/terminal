-- One personal identity per installation; no browser-selected owner.
CREATE TABLE personal_identity (
  singleton boolean PRIMARY KEY DEFAULT true CHECK (singleton),
  owner_id uuid UNIQUE NOT NULL REFERENCES terminal_users(id)
);

DO $$
DECLARE personal_id uuid := gen_random_uuid();
BEGIN
  PERFORM set_config('app.user_id', personal_id::text, true);
  INSERT INTO terminal_users(id, auth_subject) VALUES (personal_id, 'personal-owner');
  INSERT INTO personal_identity(singleton, owner_id) VALUES (true, personal_id);
END $$;

CREATE TABLE owner_sessions (
  token_hash text PRIMARY KEY CHECK (token_hash ~ '^[a-f0-9]{64}$'),
  owner_id uuid NOT NULL REFERENCES terminal_users(id),
  created_at timestamptz NOT NULL DEFAULT now(),
  expires_at timestamptz NOT NULL DEFAULT (now() + interval '8 hours'),
  revoked_at timestamptz,
  CHECK (expires_at > created_at AND expires_at <= created_at + interval '8 hours')
);
CREATE INDEX owner_sessions_expiration ON owner_sessions(owner_id, expires_at);
ALTER TABLE owner_sessions ENABLE ROW LEVEL SECURITY;
ALTER TABLE owner_sessions FORCE ROW LEVEL SECURITY;
CREATE POLICY owner_isolation ON owner_sessions
  USING (owner_id::text = current_setting('app.user_id', true))
  WITH CHECK (owner_id::text = current_setting('app.user_id', true));
