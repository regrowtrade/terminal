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

func syntheticReadSnapshot() binance.Snapshot {
	now := time.Now().UTC().Truncate(time.Microsecond).Add(-2 * time.Second)
	return binance.Snapshot{Source: "binance-production", Mode: "read_only", Consistency: "rest_non_atomic", StartedAt: now.Add(-time.Second), CapturedAt: now,
		Assets: []binance.Asset{{Asset: "USDT", WalletBalance: "100.00000001", UnrealizedProfit: "-2", AvailableBalance: "80"}}, Positions: []binance.Position{}, OpenOrders: []binance.Order{}, AlgoOrders: []binance.AlgoOrder{},
		Instruments: []binance.Instrument{{Symbol: "BTCUSDT", ContractType: "PERPETUAL", Status: "TRADING", BaseAsset: "BTC", QuoteAsset: "USDT", MarginAsset: "USDT", Price: binance.RangeFilter{Min: "0.1", Max: "1000000", Step: "0.1"}, Lot: binance.RangeFilter{Min: "0.001", Max: "1000", Step: "0.001"}, MarketLot: binance.RangeFilter{Min: "0.001", Max: "100", Step: "0.001"}, MinNotional: "5", MultiplierUp: "1.05", MultiplierDown: "0.95", MaxOrders: 200, MaxAlgos: 10, UnknownFilters: []string{}}}}
}

func TestSnapshotMetadataRejectsIncompleteBeforeDatabase(t *testing.T) {
	for _, mutate := range []func(*binance.Snapshot){
		func(s *binance.Snapshot) { s.OpenOrders = nil }, func(s *binance.Snapshot) { s.AlgoOrders = nil }, func(s *binance.Snapshot) { s.Instruments = nil },
		func(s *binance.Snapshot) { s.ExecutionReady = true }, func(s *binance.Snapshot) { s.Mode = "live" }, func(s *binance.Snapshot) { s.CapturedAt = s.StartedAt.Add(-time.Second) },
		func(s *binance.Snapshot) { s.CapturedAt = s.StartedAt.Add(time.Minute) },
	} {
		snapshot := syntheticReadSnapshot()
		mutate(&snapshot)
		if err := (&Store{}).SaveReadSnapshot(context.Background(), testDigest(), "00000000-0000-4000-8000-000000000001", snapshot); !errors.Is(err, ErrSnapshotInvalid) {
			t.Fatal("incomplete snapshot reached database")
		}
	}
	if err := (&Store{}).SaveReadSnapshot(context.Background(), "forged", "00000000-0000-4000-8000-000000000001", syntheticReadSnapshot()); !errors.Is(err, ErrSessionInvalid) {
		t.Fatal("forged session reached database")
	}
}

func testReadSnapshotLifecycle(t *testing.T, ctx context.Context, store *Store, admin *sql.DB, runtimeURI, hash, account string) {
	t.Helper()
	if _, err := store.LatestReadSnapshot(ctx, hash, account); !errors.Is(err, ErrSnapshotMissing) {
		t.Fatal("missing saved reading was not explicit")
	}
	snapshot := syntheticReadSnapshot()
	if err := store.SaveReadSnapshot(ctx, hash, account, snapshot); err != nil {
		t.Fatal("could not persist synthetic reading", err)
	}
	initial, err := store.LatestReadComparison(ctx, hash, account)
	if err != nil || initial.Report.Status != "initial_observation" || initial.Report.PreviousCapturedAt != nil || initial.Report.TotalChanges != 0 {
		t.Fatal("initial observation invented a baseline")
	}
	restarted, err := Open(ctx, runtimeURI, store.migrations)
	if err != nil {
		t.Fatal("could not reopen snapshot pool")
	}
	defer restarted.Close()
	saved, err := restarted.LatestReadSnapshot(ctx, hash, account)
	if err != nil || saved.Snapshot.Assets[0].WalletBalance != "100.00000001" || saved.StoredAt.IsZero() || !saved.Snapshot.CapturedAt.Equal(snapshot.CapturedAt) {
		t.Fatal("snapshot did not survive pool restart")
	}
	newer := snapshot
	newer.Assets = append([]binance.Asset{}, snapshot.Assets...)
	newer.Assets[0].WalletBalance = "101.00000001"
	newer.CapturedAt = newer.CapturedAt.Add(time.Second)
	if err := store.SaveReadSnapshot(ctx, hash, account, newer); err != nil {
		t.Fatal("newer snapshot rejected")
	}
	if err := store.SaveReadSnapshot(ctx, hash, account, snapshot); !errors.Is(err, ErrSnapshotInvalid) {
		t.Fatal("older observation overwrote latest")
	}
	comparison, err := restarted.LatestReadComparison(ctx, hash, account)
	if err != nil || comparison.Report.Status != "compared" || comparison.Report.TotalChanges != 1 ||
		comparison.Report.PreviousCapturedAt == nil || !comparison.Report.PreviousCapturedAt.Equal(snapshot.CapturedAt) ||
		!comparison.Report.CurrentCapturedAt.Equal(newer.CapturedAt) || *comparison.Report.Changes[0].Fields[0].Before != "100.00000001" {
		t.Fatal("comparison not durable or baseline/result mismatched")
	}
	var count int
	if err := admin.QueryRowContext(ctx, `SELECT count(*) FROM exchange_read_snapshots WHERE account_id=$1`, account).Scan(&count); err != nil || count != 1 {
		t.Fatal("more than latest snapshot retained")
	}
	if err := store.WithOwner(ctx, "00000000-0000-4000-8000-000000000001", func(tx *sql.Tx) error {
		if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM exchange_read_snapshots`).Scan(&count); err != nil {
			return err
		}
		if count != 0 {
			return errors.New("cross-owner snapshot read")
		}
		return nil
	}); err != nil {
		t.Fatal("snapshot RLS failed")
	}
	if _, err := store.LatestReadSnapshot(ctx, testDigest(), account); !errors.Is(err, ErrSessionInvalid) {
		t.Fatal("unregistered session read snapshot")
	}
	identity, err := store.ResolveSession(ctx, hash)
	if err != nil {
		t.Fatal("missing test identity")
	}
	if err := store.WithOwner(ctx, identity.OwnerID, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `DELETE FROM exchange_read_snapshots WHERE account_id=$1`, account)
		return err
	}); err == nil {
		t.Fatal("runtime can delete stored snapshots")
	}
	for _, expression := range []string{`snapshot - 'algoOrders'`, `jsonb_set(snapshot,'{startedAt}','null'::jsonb)`} {
		if err := store.WithOwner(ctx, identity.OwnerID, func(tx *sql.Tx) error {
			_, err := tx.ExecContext(ctx, `UPDATE exchange_read_snapshots SET snapshot=`+expression+` WHERE account_id=$1`, account)
			return err
		}); err == nil {
			t.Fatal("database accepted incomplete snapshot")
		}
	}
	payload, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.WithOwner(ctx, "00000000-0000-4000-8000-000000000001", func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `INSERT INTO exchange_read_snapshots(account_id,owner_id,started_at,captured_at,snapshot) VALUES($1,'00000000-0000-4000-8000-000000000001',$2,$3,$4)`, account, snapshot.StartedAt, snapshot.CapturedAt, payload)
		return err
	}); err == nil {
		t.Fatal("cross-owner snapshot write accepted")
	}
	if _, err := store.LatestReadSnapshot(ctx, hash, "00000000-0000-4000-8000-000000000099"); !errors.Is(err, ErrAccountMissing) {
		t.Fatal("unknown account not rejected")
	}
	// A session revoked while the exchange request was in progress must not
	// be able to persist or recover data afterwards.
	transient := testDigest()
	if _, err := store.CreateSession(ctx, transient); err != nil {
		t.Fatal(err)
	}
	if err := store.RevokeSession(ctx, transient, false); err != nil {
		t.Fatal(err)
	}
	newer.CapturedAt = newer.CapturedAt.Add(time.Second)
	if err := store.SaveReadSnapshot(ctx, transient, account, newer); !errors.Is(err, ErrSessionInvalid) {
		t.Fatal("revoked session saved observation")
	}
	if _, err := store.LatestReadSnapshot(ctx, transient, account); !errors.Is(err, ErrSessionInvalid) {
		t.Fatal("revoked session recovered observation")
	}
	if _, err := store.LatestReadComparison(ctx, transient, account); !errors.Is(err, ErrSessionInvalid) {
		t.Fatal("revoked session recovered comparison")
	}
	testReadComparisons(t, ctx, store, admin, hash, account)
}
