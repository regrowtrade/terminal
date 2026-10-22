package database

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"regrow/engine/internal/binance"
	"testing"
	"time"
)

func syntheticAlgoLookup() binance.AlgoLookup {
	now := time.Now().UTC()
	return binance.AlgoLookup{Source: "binance-production", Mode: "read_only", Family: "algo", StartedAt: now, CheckedAt: now.Add(time.Second), Query: binance.AlgoQuery{Symbol: "BTCUSDT", AlgoID: "9007199254740993"}, Outcome: "observed", Order: &binance.QueriedAlgoOrder{
		AlgoOrder:     binance.AlgoOrder{ID: "9007199254740993", ClientID: "synthetic-algo", AlgoType: "CONDITIONAL", Symbol: "BTCUSDT", Side: "SELL", PositionSide: "LONG", Type: "STOP_MARKET", Status: "FINISHED", Quantity: "2.000", Price: "0", TriggerPrice: "95.123456789012345678", WorkingType: "MARK_PRICE", ReduceOnly: true, UpdateTime: 1750514941540},
		ActualOrderID: "9007199254740995", ActualPrice: "94.123456789012345678", ActualType: "MARKET", ActualQuantity: "0.500000000000000001", TriggerTime: 1750514941539,
	}}
}

func TestSaveAlgoObservationRejectsInvalidInputBeforeDatabase(t *testing.T) {
	s := &Store{}
	if err := s.SaveAlgoObservation(context.Background(), "synthetic", "invalid", syntheticAlgoLookup()); !errors.Is(err, ErrAccountMissing) {
		t.Fatal("invalid account accepted")
	}
	for _, mutate := range []func(*binance.AlgoLookup){func(r *binance.AlgoLookup) { r.Family = "regular" }, func(r *binance.AlgoLookup) { r.ExecutionReady = true }, func(r *binance.AlgoLookup) { r.Query.AlgoID = "1" }} {
		r := syntheticAlgoLookup()
		mutate(&r)
		if err := s.SaveAlgoObservation(context.Background(), "synthetic", "00000000-0000-4000-8000-000000000001", r); !errors.Is(err, ErrSnapshotInvalid) {
			t.Fatal("invalid evidence accepted")
		}
	}
}

func testAlgoObservationLifecycle(t *testing.T, ctx context.Context, s *Store, admin *sql.DB, hash, account string) {
	t.Helper()
	identity, err := s.ResolveSession(ctx, hash)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := s.LatestReadSnapshot(ctx, hash, account)
	if err != nil {
		t.Fatal(err)
	}
	stream, err := s.LatestStreamState(ctx, hash, account)
	if err != nil {
		t.Fatal(err)
	}
	var beforeFills, beforeCount int
	if err := admin.QueryRowContext(ctx, `SELECT count(*) FROM binance_fills WHERE account_id=$1`, account).Scan(&beforeFills); err != nil {
		t.Fatal(err)
	}
	if err := admin.QueryRowContext(ctx, `SELECT count(*) FROM binance_order_observations WHERE account_id=$1`, account).Scan(&beforeCount); err != nil {
		t.Fatal(err)
	}
	r := syntheticAlgoLookup()
	unresolved := r
	unresolved.Outcome = "unresolved"
	unresolved.Reason = "exchange_not_found"
	unresolved.Order = nil
	for _, item := range []binance.AlgoLookup{r, unresolved} {
		if err := s.SaveAlgoObservation(ctx, hash, account, item); err != nil {
			t.Fatal("save failed", err)
		}
	}
	checkCount := func() {
		t.Helper()
		var count int
		if err := admin.QueryRowContext(ctx, `SELECT count(*) FROM binance_order_observations WHERE account_id=$1`, account).Scan(&count); err != nil || count != beforeCount+2 {
			t.Fatal("partial or missing evidence", count, err)
		}
	}
	checkCount()
	var audits int
	if err := admin.QueryRowContext(ctx, `SELECT count(*) FROM audit_events WHERE action='account.algo_order_observed' AND resource_id=$1 AND metadata IN ('{"outcome":"observed"}'::jsonb,'{"outcome":"unresolved"}'::jsonb)`, account).Scan(&audits); err != nil || audits != 2 {
		t.Fatal("missing/unsafe audit", err)
	}
	var data []byte
	var started, checked time.Time
	if err := admin.QueryRowContext(ctx, `SELECT observation,started_at,checked_at FROM binance_order_observations WHERE account_id=$1 AND observation->>'family'='algo' AND observation->>'outcome'='observed'`, account).Scan(&data, &started, &checked); err != nil {
		t.Fatal(err)
	}
	var stored binance.AlgoLookup
	if json.Unmarshal(data, &stored) != nil || binance.ValidateAlgoLookup(stored) != nil || stored.Order == nil || *stored.Order != *r.Order || !stored.StartedAt.Equal(started) || !stored.CheckedAt.Equal(checked) {
		t.Fatal("lost exact evidence/time alignment")
	}
	for _, tc := range []struct {
		hash, account string
		want          error
	}{{testDigest(), account, ErrSessionInvalid}, {hash, "00000000-0000-4000-8000-000000000099", ErrAccountMissing}} {
		if err := s.SaveAlgoObservation(ctx, tc.hash, tc.account, r); !errors.Is(err, tc.want) {
			t.Fatal("boundary bypassed", err)
		}
	}
	for owner, want := range map[string]int{identity.OwnerID: 2, "00000000-0000-4000-8000-000000000001": 0} {
		if err := s.WithOwner(ctx, owner, func(tx *sql.Tx) error {
			var count int
			if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM binance_order_observations WHERE observation->>'family'='algo'`).Scan(&count); err != nil {
				return err
			}
			if count != want {
				return errors.New("RLS leaked algo evidence")
			}
			return nil
		}); err != nil {
			t.Fatal(err)
		}
	}
	var count int
	if err := s.db.QueryRowContext(ctx, `SELECT count(*) FROM binance_order_observations`).Scan(&count); err != nil || count != 0 {
		t.Fatal("pooled identity leaked")
	}
	var role string
	if err := s.db.QueryRowContext(ctx, `SELECT current_user`).Scan(&role); err != nil {
		t.Fatal(err)
	}
	if _, err := admin.ExecContext(ctx, `REVOKE INSERT ON audit_events FROM `+role); err != nil {
		t.Fatal(err)
	}
	err = s.SaveAlgoObservation(ctx, hash, account, r)
	if _, restore := admin.ExecContext(ctx, `GRANT INSERT ON audit_events TO `+role); restore != nil {
		t.Fatal(restore)
	}
	if err == nil {
		t.Fatal("audit failure not rolled back")
	}
	checkCount()
	for _, revoked := range []bool{true, false} {
		session := testDigest()
		if _, err := s.CreateSession(ctx, session); err != nil {
			t.Fatal(err)
		}
		if revoked {
			if err := s.RevokeSession(ctx, session, false); err != nil {
				t.Fatal(err)
			}
		} else {
			if _, err := admin.ExecContext(ctx, `UPDATE owner_sessions SET created_at=clock_timestamp()-interval '2 hours',expires_at=clock_timestamp()-interval '1 second' WHERE token_hash=$1`, session); err != nil {
				t.Fatal(err)
			}
		}
		if err := s.SaveAlgoObservation(ctx, session, account, r); !errors.Is(err, ErrSessionInvalid) {
			t.Fatal("inactive session saved algo", err)
		}
	}
	checkCount()
	after, err := s.LatestReadSnapshot(ctx, hash, account)
	if err != nil {
		t.Fatal(err)
	}
	a, _ := json.Marshal(snapshot)
	b, _ := json.Marshal(after)
	afterStream, err := s.LatestStreamState(ctx, hash, account)
	if err != nil {
		t.Fatal(err)
	}
	if string(a) != string(b) || stream != afterStream {
		t.Fatal("algo query changed baseline/continuity")
	}
	if err := admin.QueryRowContext(ctx, `SELECT count(*) FROM binance_fills WHERE account_id=$1`, account).Scan(&count); err != nil || count != beforeFills {
		t.Fatal("algo query fabricated fills")
	}
}
