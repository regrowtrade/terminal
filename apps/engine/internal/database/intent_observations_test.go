package database

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"reflect"
	"testing"
	"time"

	"regrow/engine/internal/binance"
	"regrow/engine/internal/intentcheck"
)

func TestIntentObservationRejectsInvalidIdentifiers(t *testing.T) {
	s := &Store{}
	if _, err := s.ReadIntentExpected(context.Background(), "synthetic", "bad", "bad"); !errors.Is(err, ErrIntentMissing) {
		t.Fatal("invalid IDs accepted")
	}
	if _, err := s.ListIntentObservations(context.Background(), "synthetic", "bad", "bad"); !errors.Is(err, ErrIntentMissing) {
		t.Fatal("invalid history IDs accepted")
	}
	if _, err := s.SaveIntentObservation(context.Background(), "synthetic", "bad", intentcheck.Expected{}, binance.OrderLookup{}); !errors.Is(err, ErrAccountMissing) {
		t.Fatal("invalid save accepted")
	}
}

func testIntentObservationLifecycle(t *testing.T, ctx context.Context, store *Store, admin *sql.DB, hash, account string, entry, conditional StoredIntent) {
	t.Helper()
	e, err := store.ReadIntentExpected(ctx, hash, account, entry.ID)
	if err != nil || !e.Valid() || e.PlanID != entry.PlanID || e.ClientOrderID != entry.ClientOrderID {
		t.Fatal("could not derive stored criteria", err)
	}
	if _, err := store.ReadIntentExpected(ctx, hash, account, conditional.ID); !errors.Is(err, ErrIntentUnsupported) {
		t.Fatal("conditional investigation allowed", err)
	}
	now := time.Now().UTC()
	lookup := binance.OrderLookup{Source: "binance-production", Mode: "read_only", StartedAt: now, CheckedAt: now.Add(time.Second), Query: e.Query(), Outcome: "observed", Order: &binance.QueriedOrder{OrderID: "9007199254740993", ClientOrderID: e.ClientOrderID, Symbol: e.Symbol, Side: "BUY", PositionSide: "LONG", Type: "LIMIT", Status: "NEW", Quantity: e.Quantity, Price: e.Price, Executed: "0", AveragePrice: "0", UpdateTime: 1750514941540}}
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
	matched, err := store.SaveIntentObservation(ctx, hash, account, e, lookup)
	if err != nil || matched.ID == "" || matched.Report.Assessment != "attributes_match" || matched.Report.OwnershipConfirmed || matched.Report.ExecutionReady {
		t.Fatal("comparison not persisted", err)
	}
	mismatch := lookup
	mismatchOrder := *lookup.Order
	mismatchOrder.PositionSide = "BOTH"
	mismatch.Order = &mismatchOrder
	if r, err := store.SaveIntentObservation(ctx, hash, account, e, mismatch); err != nil || r.Report.Assessment != "mismatch" {
		t.Fatal("mismatch not preserved", err)
	}
	unresolved := lookup
	unresolved.Outcome, unresolved.Reason, unresolved.Order = "unresolved", "exchange_not_found", nil
	if r, err := store.SaveIntentObservation(ctx, hash, account, e, unresolved); err != nil || r.Report.Assessment != "unresolved" {
		t.Fatal("absence reinterpreted", err)
	}
	assertCount := func(want int) {
		t.Helper()
		var n int
		if err := admin.QueryRowContext(ctx, `SELECT count(*) FROM binance_intent_observations WHERE intent_id=$1`, entry.ID).Scan(&n); err != nil || n != want {
			t.Fatal("unexpected evidence count", n, err)
		}
	}
	assertCount(3)
	history, err := store.ListIntentObservations(ctx, hash, account, entry.ID)
	if err != nil || len(history) != 3 {
		t.Fatal("history unavailable", err)
	}
	found := false
	for _, r := range history {
		if r.ID == matched.ID {
			found = reflect.DeepEqual(r, matched)
		}
	}
	if !found {
		t.Fatal("durable evidence lost precision or timestamps")
	}
	// Corrupt historical assessments are never returned, even as a partial page.
	var original []byte
	if err := admin.QueryRowContext(ctx, `SELECT report FROM binance_intent_observations WHERE id=$1`, matched.ID).Scan(&original); err != nil {
		t.Fatal(err)
	}
	if _, err := admin.ExecContext(ctx, `UPDATE binance_intent_observations SET report=jsonb_set(jsonb_set(report,'{assessment}','"mismatch"'::jsonb),'{mismatches}','["side"]'::jsonb) WHERE id=$1`, matched.ID); err != nil {
		t.Fatal(err)
	}
	partial, corruptErr := store.ListIntentObservations(ctx, hash, account, entry.ID)
	if _, err := admin.ExecContext(ctx, `UPDATE binance_intent_observations SET report=$2 WHERE id=$1`, matched.ID, original); err != nil {
		t.Fatal(err)
	}
	if !errors.Is(corruptErr, ErrSnapshotInvalid) || partial != nil {
		t.Fatal("corrupt history returned partial evidence", corruptErr)
	}
	identity, err := store.ResolveSession(ctx, hash)
	if err != nil {
		t.Fatal(err)
	}
	for owner, want := range map[string]int{identity.OwnerID: 3, "00000000-0000-4000-8000-000000000001": 0} {
		if err := store.WithOwner(ctx, owner, func(tx *sql.Tx) error {
			var n int
			if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM binance_intent_observations`).Scan(&n); err != nil {
				return err
			}
			if n != want {
				return errors.New("evidence leaked across owner")
			}
			return nil
		}); err != nil {
			t.Fatal(err)
		}
	}
	var n int
	if err := store.db.QueryRowContext(ctx, `SELECT count(*) FROM binance_intent_observations`).Scan(&n); err != nil || n != 0 {
		t.Fatal("unbound runtime can read intent evidence")
	}
	for _, statement := range []string{`UPDATE binance_intent_observations SET report=report`, `DELETE FROM binance_intent_observations`} {
		if err := store.WithOwner(ctx, identity.OwnerID, func(tx *sql.Tx) error { _, err := tx.ExecContext(ctx, statement); return err }); err == nil {
			t.Fatal("runtime can rewrite evidence")
		}
	}
	// Persisted decision changes during the network request are not accepted.
	var snapshot []byte
	if err := admin.QueryRowContext(ctx, `SELECT snapshot FROM strategy_plans WHERE id=$1`, e.PlanID).Scan(&snapshot); err != nil {
		t.Fatal(err)
	}
	if _, err := admin.ExecContext(ctx, `UPDATE strategy_plans SET snapshot=jsonb_set(snapshot,'{warnings}','["synthetic changed decision"]'::jsonb) WHERE id=$1`, e.PlanID); err != nil {
		t.Fatal(err)
	}
	_, changedErr := store.SaveIntentObservation(ctx, hash, account, e, lookup)
	if _, err := admin.ExecContext(ctx, `UPDATE strategy_plans SET snapshot=$2 WHERE id=$1`, e.PlanID, snapshot); err != nil {
		t.Fatal(err)
	}
	if !errors.Is(changedErr, ErrIntentChanged) {
		t.Fatal("changed plan accepted", changedErr)
	}
	if _, err := store.SaveIntentObservation(ctx, testDigest(), account, e, lookup); !errors.Is(err, ErrSessionInvalid) {
		t.Fatal("unregistered session saved evidence")
	}
	if _, err := store.ReadIntentExpected(ctx, hash, "00000000-0000-4000-8000-000000000098", entry.ID); !errors.Is(err, ErrIntentMissing) {
		t.Fatal("wrong account accepted")
	}
	if _, err := store.ListIntentObservations(ctx, testDigest(), account, entry.ID); !errors.Is(err, ErrSessionInvalid) {
		t.Fatal("unregistered session read history")
	}
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
		if r, err := store.SaveIntentObservation(ctx, session, account, e, lookup); !errors.Is(err, ErrSessionInvalid) || r.ID != "" {
			t.Fatal("inactive session saved evidence", err)
		}
	}
	// A failed audit rolls back the evidence already inserted in this transaction.
	var role string
	if err := store.db.QueryRowContext(ctx, `SELECT current_user`).Scan(&role); err != nil {
		t.Fatal(err)
	}
	if _, err := admin.ExecContext(ctx, `REVOKE INSERT ON audit_events FROM `+role); err != nil {
		t.Fatal(err)
	}
	r, saveErr := store.SaveIntentObservation(ctx, hash, account, e, lookup)
	if _, err := admin.ExecContext(ctx, `GRANT INSERT ON audit_events TO `+role); err != nil {
		t.Fatal(err)
	}
	if saveErr == nil || r.ID != "" {
		t.Fatal("audit failure returned evidence")
	}
	assertCount(3)
	// Database constraints forbid execution authority and cross-plan linkage.
	for _, field := range []string{"executionReady", "ownershipConfirmed"} {
		if err := store.WithOwner(ctx, identity.OwnerID, func(tx *sql.Tx) error {
			_, err := tx.ExecContext(ctx, `INSERT INTO binance_intent_observations(account_id,owner_id,intent_id,plan_id,report) SELECT account_id,owner_id,intent_id,plan_id,jsonb_set(report,ARRAY[$2::text],'true'::jsonb) FROM binance_intent_observations WHERE id=$1`, matched.ID, field)
			return err
		}); err == nil {
			t.Fatal("database accepted execution authority")
		}
	}
	var otherPlan string
	if err := admin.QueryRowContext(ctx, `SELECT id::text FROM strategy_plans WHERE account_id=$1 AND id<>$2 LIMIT 1`, account, e.PlanID).Scan(&otherPlan); err != nil {
		t.Fatal(err)
	}
	if err := store.WithOwner(ctx, identity.OwnerID, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `INSERT INTO binance_intent_observations(account_id,owner_id,intent_id,plan_id,report) SELECT account_id,owner_id,intent_id,$2,jsonb_set(report,'{expected,planId}',to_jsonb($2::text)) FROM binance_intent_observations WHERE id=$1`, matched.ID, otherPlan)
		return err
	}); err == nil {
		t.Fatal("evidence linked intent to a different plan")
	}
	if err := store.WithOwner(ctx, "00000000-0000-4000-8000-000000000001", func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `INSERT INTO binance_intent_observations(account_id,owner_id,intent_id,plan_id,report) VALUES($1,$2,$3,$4,$5)`, account, identity.OwnerID, entry.ID, e.PlanID, original)
		return err
	}); err == nil {
		t.Fatal("cross-owner evidence insert accepted")
	}
	// Repeated explicit checks append observations; history is bounded to 20.
	for range 20 {
		if _, err := store.SaveIntentObservation(ctx, hash, account, e, unresolved); err != nil {
			t.Fatal(err)
		}
	}
	history, err = store.ListIntentObservations(ctx, hash, account, entry.ID)
	if err != nil || len(history) != 20 {
		t.Fatal("history is not bounded", err)
	}
	for i := 1; i < len(history); i++ {
		if history[i].StoredAt.After(history[i-1].StoredAt) {
			t.Fatal("history not newest first")
		}
	}
	var current StoredIntent
	intents, err := store.ListPlanIntents(ctx, hash, account, e.PlanID)
	if err != nil {
		t.Fatal(err)
	}
	for _, in := range intents {
		if in.ID == entry.ID {
			current = in
		}
	}
	if !reflect.DeepEqual(current, entry) {
		t.Fatal("investigation changed intent state or exchange ID")
	}
	afterSnapshot, err := store.LatestReadSnapshot(ctx, hash, account)
	if err != nil {
		t.Fatal(err)
	}
	a, _ := json.Marshal(beforeSnapshot)
	b, _ := json.Marshal(afterSnapshot)
	afterStream, err := store.LatestStreamState(ctx, hash, account)
	if err != nil || string(a) != string(b) || beforeStream != afterStream {
		t.Fatal("investigation changed baseline or continuity", err)
	}
	if err := admin.QueryRowContext(ctx, `SELECT count(*) FROM binance_fills WHERE account_id=$1`, account).Scan(&n); err != nil || n != beforeFills {
		t.Fatal("investigation fabricated fills")
	}
}
