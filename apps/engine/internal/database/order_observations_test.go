package database

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"regrow/engine/internal/binance"
)

func syntheticOrderLookup() binance.OrderLookup {
	now := time.Now().UTC()
	return binance.OrderLookup{Source: "binance-production", Mode: "read_only", StartedAt: now, CheckedAt: now.Add(time.Second), Query: binance.OrderQuery{Symbol: "BTCUSDT", OrderID: "9007199254740993"}, Outcome: "observed", Order: &binance.QueriedOrder{
		OrderID: "9007199254740993", ClientOrderID: "synthetic-order", Symbol: "BTCUSDT", Side: "BUY", PositionSide: "BOTH", Type: "LIMIT", Status: "FILLED", Quantity: "2.000", Executed: "2.000", Price: "90.00000001", AveragePrice: "89.12345678", ReduceOnly: true, UpdateTime: 1750514941540,
	}}
}

func TestSaveOrderObservationRejectsInvalidInputBeforeDatabase(t *testing.T) {
	store := &Store{}
	if err := store.SaveOrderObservation(context.Background(), "synthetic", "invalid", syntheticOrderLookup()); !errors.Is(err, ErrAccountMissing) {
		t.Fatal("invalid account accepted")
	}
	invalid := syntheticOrderLookup()
	invalid.ExecutionReady = true
	if err := store.SaveOrderObservation(context.Background(), "synthetic", "00000000-0000-4000-8000-000000000001", invalid); !errors.Is(err, ErrSnapshotInvalid) {
		t.Fatal("invalid evidence accepted")
	}
}

func testOrderObservationLifecycle(t *testing.T, ctx context.Context, store *Store, admin *sql.DB, hash, account string) {
	t.Helper()
	identity, err := store.ResolveSession(ctx, hash)
	if err != nil {
		t.Fatal(err)
	}
	beforeSnapshot, err := store.LatestReadSnapshot(ctx, hash, account)
	if err != nil {
		t.Fatal(err)
	}
	beforeStream, err := store.LatestStreamState(ctx, hash, account)
	if err != nil {
		t.Fatal(err)
	}
	var beforeFills int
	if err := admin.QueryRowContext(ctx, `SELECT count(*) FROM binance_fills WHERE account_id=$1`, account).Scan(&beforeFills); err != nil {
		t.Fatal(err)
	}
	observed := syntheticOrderLookup()
	unresolved := observed
	unresolved.Outcome = "unresolved"
	unresolved.Reason = "exchange_not_found"
	unresolved.Order = nil
	for _, r := range []binance.OrderLookup{observed, unresolved} {
		if err := store.SaveOrderObservation(ctx, hash, account, r); err != nil {
			t.Fatal("could not save order evidence", err)
		}
	}
	var count, audits int
	checkCount := func(want int) {
		t.Helper()
		if err := admin.QueryRowContext(ctx, `SELECT count(*) FROM binance_order_observations WHERE account_id=$1`, account).Scan(&count); err != nil || count != want {
			t.Fatal("unexpected order evidence count", count, err)
		}
	}
	checkCount(2)
	if err := admin.QueryRowContext(ctx, `SELECT count(*) FROM audit_events WHERE action='account.order_observed' AND resource_id=$1 AND metadata IN ('{"outcome":"observed"}'::jsonb,'{"outcome":"unresolved"}'::jsonb)`, account).Scan(&audits); err != nil || audits != 2 {
		t.Fatal("missing or unsafe audit", err)
	}
	var raw []byte
	var started, checked time.Time
	if err := admin.QueryRowContext(ctx, `SELECT observation,started_at,checked_at FROM binance_order_observations WHERE account_id=$1 AND observation->>'outcome'='observed'`, account).Scan(&raw, &started, &checked); err != nil {
		t.Fatal(err)
	}
	var saved binance.OrderLookup
	if json.Unmarshal(raw, &saved) != nil || binance.ValidateOrderLookup(saved) != nil || saved.Order == nil || *saved.Order != *observed.Order || !saved.StartedAt.Equal(started) || !saved.CheckedAt.Equal(checked) {
		t.Fatal("order evidence lost precision, identifiers or time alignment")
	}
	afterSnapshot, err := store.LatestReadSnapshot(ctx, hash, account)
	if err != nil {
		t.Fatal(err)
	}
	a, _ := json.Marshal(beforeSnapshot)
	b, _ := json.Marshal(afterSnapshot)
	afterStream, err := store.LatestStreamState(ctx, hash, account)
	if err != nil {
		t.Fatal(err)
	}
	if string(a) != string(b) || beforeStream != afterStream {
		t.Fatal("order lookup changed baseline or continuity")
	}
	if err := admin.QueryRowContext(ctx, `SELECT count(*) FROM binance_fills WHERE account_id=$1`, account).Scan(&count); err != nil || count != beforeFills {
		t.Fatal("order query fabricated fills")
	}
	// Every failed boundary must leave the append-only evidence untouched.
	if err := store.SaveOrderObservation(ctx, testDigest(), account, observed); !errors.Is(err, ErrSessionInvalid) {
		t.Fatal("unregistered session saved order evidence")
	}
	if err := store.SaveOrderObservation(ctx, hash, "00000000-0000-4000-8000-000000000099", observed); !errors.Is(err, ErrAccountMissing) {
		t.Fatal("missing account accepted")
	}
	foreign := "00000000-0000-4000-8000-000000000098"
	if _, err := admin.ExecContext(ctx, `INSERT INTO exchange_accounts(id,owner_id,exchange,environment,name) VALUES($1,'00000000-0000-4000-8000-000000000001','binance','production','Foreign synthetic')`, foreign); err != nil {
		t.Fatal(err)
	}
	defer admin.ExecContext(ctx, `DELETE FROM exchange_accounts WHERE id=$1`, foreign)
	if err := store.SaveOrderObservation(ctx, hash, foreign, observed); !errors.Is(err, ErrAccountMissing) {
		t.Fatal("foreign account accepted", err)
	}
	for owner, want := range map[string]int{identity.OwnerID: 2, "00000000-0000-4000-8000-000000000001": 0} {
		if err := store.WithOwner(ctx, owner, func(tx *sql.Tx) error {
			var got int
			if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM binance_order_observations`).Scan(&got); err != nil {
				return err
			}
			if got != want {
				return errors.New("order evidence leaked across owner")
			}
			return nil
		}); err != nil {
			t.Fatal("RLS failed", err)
		}
	}
	// No transaction identity remains on pooled connections.
	if err := store.db.QueryRowContext(ctx, `SELECT count(*) FROM binance_order_observations`).Scan(&count); err != nil || count != 0 {
		t.Fatal("unbound runtime can read evidence")
	}
	for _, statement := range []string{`UPDATE binance_order_observations SET observation=observation`, `DELETE FROM binance_order_observations`} {
		if err := store.WithOwner(ctx, identity.OwnerID, func(tx *sql.Tx) error { _, err := tx.ExecContext(ctx, statement); return err }); err == nil {
			t.Fatal("runtime can rewrite order evidence")
		}
	}
	if err := store.WithOwner(ctx, "00000000-0000-4000-8000-000000000001", func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `INSERT INTO binance_order_observations(account_id,owner_id,started_at,checked_at,observation) VALUES($1,$2,$3,$4,$5)`, account, identity.OwnerID, started, checked, raw)
		return err
	}); err == nil {
		t.Fatal("cross-owner insert accepted")
	}
	// A failed audit must roll back the already inserted observation.
	var role string
	if err := store.db.QueryRowContext(ctx, `SELECT current_user`).Scan(&role); err != nil {
		t.Fatal(err)
	}
	if _, err := admin.ExecContext(ctx, `REVOKE INSERT ON audit_events FROM `+role); err != nil {
		t.Fatal(err)
	}
	err = store.SaveOrderObservation(ctx, hash, account, observed)
	if _, restore := admin.ExecContext(ctx, `GRANT INSERT ON audit_events TO `+role); restore != nil {
		t.Fatal(restore)
	}
	if err == nil {
		t.Fatal("save ignored audit failure")
	}
	checkCount(2)
	// Revoked and expired sessions are rechecked at the persistence boundary.
	for _, revoke := range []bool{true, false} {
		session := testDigest()
		if _, err := store.CreateSession(ctx, session); err != nil {
			t.Fatal(err)
		}
		if revoke {
			if err := store.RevokeSession(ctx, session, false); err != nil {
				t.Fatal(err)
			}
		} else {
			if _, err := admin.ExecContext(ctx, `UPDATE owner_sessions SET created_at=clock_timestamp()-interval '2 hours',expires_at=clock_timestamp()-interval '1 second' WHERE token_hash=$1`, session); err != nil {
				t.Fatal(err)
			}
		}
		if err := store.SaveOrderObservation(ctx, session, account, observed); !errors.Is(err, ErrSessionInvalid) {
			t.Fatal("inactive session saved evidence", err)
		}
	}
	checkCount(2)
}
