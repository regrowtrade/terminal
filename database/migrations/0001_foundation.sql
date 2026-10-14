-- PostgreSQL design baseline. Not applied by the application in v0.1.
-- Use a non-owner, non-BYPASSRLS application role and SET LOCAL app.user_id
-- from a verified identity inside each transaction. Never trust a client user ID.
BEGIN;

CREATE TABLE terminal_users (
  id uuid PRIMARY KEY,
  auth_subject text UNIQUE NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE exchange_accounts (
  id uuid PRIMARY KEY,
  owner_id uuid NOT NULL REFERENCES terminal_users(id),
  exchange text NOT NULL CHECK (exchange IN ('binance', 'bybit')),
  environment text NOT NULL CHECK (environment IN ('testnet', 'production')),
  name text NOT NULL CHECK (length(name) BETWEEN 1 AND 80),
  credential_reference text,
  created_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE (id, owner_id)
);

CREATE TABLE trade_campaigns (
  id uuid PRIMARY KEY,
  owner_id uuid NOT NULL REFERENCES terminal_users(id),
  account_id uuid NOT NULL,
  instrument text NOT NULL,
  status text NOT NULL CHECK (status IN ('DRAFT', 'ACTIVE', 'PAUSED', 'RECOVERING', 'CLOSING', 'CLOSED', 'ERROR')),
  version bigint NOT NULL DEFAULT 1 CHECK (version > 0),
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  FOREIGN KEY (account_id, owner_id) REFERENCES exchange_accounts(id, owner_id),
  UNIQUE (id, account_id, owner_id)
);

CREATE UNIQUE INDEX one_active_campaign_per_instrument
  ON trade_campaigns(account_id, instrument)
  WHERE status IN ('ACTIVE', 'PAUSED', 'RECOVERING', 'CLOSING', 'ERROR');

CREATE TABLE strategy_plans (
  id uuid PRIMARY KEY,
  owner_id uuid NOT NULL,
  account_id uuid NOT NULL,
  campaign_id uuid NOT NULL,
  mode text NOT NULL CHECK (mode IN ('clean', 'dca', 'hedge')),
  version integer NOT NULL CHECK (version > 0),
  parameters jsonb NOT NULL CHECK (jsonb_typeof(parameters) = 'object'),
  snapshot jsonb NOT NULL CHECK (jsonb_typeof(snapshot) = 'object'),
  risk_amount numeric(38,18) NOT NULL CHECK (risk_amount > 0),
  baseline_pnl numeric(38,18),
  pnl_threshold numeric(38,18),
  simulation boolean NOT NULL DEFAULT true,
  created_at timestamptz NOT NULL DEFAULT now(),
  FOREIGN KEY (campaign_id, account_id, owner_id) REFERENCES trade_campaigns(id, account_id, owner_id),
  UNIQUE (campaign_id, version),
  UNIQUE (id, account_id, owner_id)
);

CREATE TABLE order_intents (
  id uuid PRIMARY KEY,
  owner_id uuid NOT NULL,
  account_id uuid NOT NULL,
  plan_id uuid NOT NULL,
  client_order_id text NOT NULL CHECK (length(client_order_id) BETWEEN 1 AND 36),
  exchange_order_id text,
  order_family text NOT NULL CHECK (order_family IN ('regular', 'conditional')),
  purpose text NOT NULL CHECK (purpose IN ('ENTRY', 'REBUY', 'TP', 'SL', 'HEDGE_CLOSE', 'EMERGENCY_CLOSE')),
  position_side text NOT NULL CHECK (position_side IN ('LONG', 'SHORT')),
  status text NOT NULL CHECK (status IN ('PENDING', 'SENDING', 'UNKNOWN', 'ACKNOWLEDGED', 'PARTIAL', 'FILLED', 'CANCELLED', 'REJECTED', 'EXPIRED')),
  quantity numeric(38,18) NOT NULL CHECK (quantity > 0),
  price numeric(38,18) CHECK (price > 0),
  created_at timestamptz NOT NULL DEFAULT now(),
  FOREIGN KEY (plan_id, account_id, owner_id) REFERENCES strategy_plans(id, account_id, owner_id),
  UNIQUE (account_id, client_order_id),
  UNIQUE (id, account_id, owner_id)
);

CREATE TABLE executions (
  id uuid PRIMARY KEY,
  owner_id uuid NOT NULL,
  account_id uuid NOT NULL,
  order_id uuid NOT NULL,
  exchange_execution_id text NOT NULL,
  price numeric(38,18) NOT NULL CHECK (price > 0),
  quantity numeric(38,18) NOT NULL CHECK (quantity > 0),
  fee_amount numeric(38,18) NOT NULL CHECK (fee_amount >= 0),
  fee_asset text NOT NULL,
  realized_pnl numeric(38,18) NOT NULL,
  executed_at timestamptz NOT NULL,
  FOREIGN KEY (order_id, account_id, owner_id) REFERENCES order_intents(id, account_id, owner_id),
  UNIQUE (account_id, exchange_execution_id)
);

CREATE TABLE campaign_ledger (
  id uuid PRIMARY KEY,
  owner_id uuid NOT NULL,
  account_id uuid NOT NULL,
  campaign_id uuid NOT NULL,
  source_id text NOT NULL,
  kind text NOT NULL CHECK (kind IN ('FUNDING', 'ADJUSTMENT')),
  net_amount numeric(38,18) NOT NULL,
  asset text NOT NULL,
  occurred_at timestamptz NOT NULL,
  FOREIGN KEY (campaign_id, account_id, owner_id) REFERENCES trade_campaigns(id, account_id, owner_id),
  UNIQUE (account_id, kind, source_id)
);

CREATE TABLE journal_notes (
  id uuid PRIMARY KEY,
  owner_id uuid NOT NULL,
  account_id uuid NOT NULL,
  campaign_id uuid NOT NULL,
  body text NOT NULL CHECK (length(body) <= 20000),
  tags text[] NOT NULL DEFAULT '{}',
  created_at timestamptz NOT NULL DEFAULT now(),
  FOREIGN KEY (campaign_id, account_id, owner_id) REFERENCES trade_campaigns(id, account_id, owner_id)
);

CREATE TABLE audit_events (
  id uuid PRIMARY KEY,
  owner_id uuid NOT NULL REFERENCES terminal_users(id),
  action text NOT NULL,
  resource_id uuid,
  correlation_id uuid NOT NULL,
  metadata jsonb NOT NULL CHECK (jsonb_typeof(metadata) = 'object'),
  created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX campaigns_owner_created ON trade_campaigns(owner_id, created_at DESC);
CREATE INDEX orders_owner_status ON order_intents(owner_id, status);
CREATE INDEX executions_owner_time ON executions(owner_id, executed_at DESC);
CREATE INDEX audit_owner_time ON audit_events(owner_id, created_at DESC);

DO $$
DECLARE table_name text;
BEGIN
  FOREACH table_name IN ARRAY ARRAY['exchange_accounts', 'trade_campaigns', 'strategy_plans', 'order_intents', 'executions', 'campaign_ledger', 'journal_notes', 'audit_events'] LOOP
    EXECUTE format('ALTER TABLE %I ENABLE ROW LEVEL SECURITY', table_name);
    EXECUTE format('ALTER TABLE %I FORCE ROW LEVEL SECURITY', table_name);
    EXECUTE format('CREATE POLICY owner_isolation ON %I USING (owner_id::text = current_setting(''app.user_id'', true)) WITH CHECK (owner_id::text = current_setting(''app.user_id'', true))', table_name);
  END LOOP;
END $$;

ALTER TABLE terminal_users ENABLE ROW LEVEL SECURITY;
ALTER TABLE terminal_users FORCE ROW LEVEL SECURITY;
CREATE POLICY user_self ON terminal_users
  USING (id::text = current_setting('app.user_id', true))
  WITH CHECK (id::text = current_setting('app.user_id', true));

-- Before connecting: grant only SELECT/INSERT on audit_events to the application
-- role; provision identities through a separate trusted administrative role.
COMMIT;
