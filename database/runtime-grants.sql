-- Run explicitly as an administrator AFTER migrations, never on HTTP startup.
-- First create terminal_runtime LOGIN with a unique generated password through
-- a secure database console. Do not put passwords in this file or command logs.
-- Do not grant administrative/owning role membership to terminal_runtime.
BEGIN;
REVOKE CREATE ON SCHEMA public FROM PUBLIC;
GRANT USAGE ON SCHEMA public TO terminal_runtime;
GRANT SELECT ON public.schema_migrations, public.terminal_users TO terminal_runtime;
GRANT SELECT ON public.personal_identity TO terminal_runtime;
GRANT SELECT, INSERT, UPDATE, DELETE ON public.owner_sessions TO terminal_runtime;
-- Runtime may decrypt only after server-side session/ownership checks. Credential
-- provisioning/rotation/deletion belongs to the separate administrative CLI.
GRANT SELECT ON public.exchange_credentials TO terminal_runtime;
-- Replace only the latest observation. No runtime deletion of saved readings.
GRANT SELECT, INSERT, UPDATE ON public.exchange_read_snapshots TO terminal_runtime;
-- Append-only journal of read-only user data stream events; runtime never deletes it.
GRANT SELECT, INSERT ON public.binance_user_events TO terminal_runtime;
GRANT SELECT, INSERT, UPDATE, DELETE ON
  public.exchange_accounts, public.trade_campaigns, public.strategy_plans,
  public.order_intents, public.executions, public.campaign_ledger, public.journal_notes
  TO terminal_runtime;
GRANT SELECT, INSERT ON public.audit_events TO terminal_runtime;
COMMIT;
