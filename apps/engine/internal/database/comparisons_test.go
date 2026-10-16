package database

import (
	"context"
	"database/sql"
	"errors"
	"sync"
	"testing"
	"time"

	"regrow/engine/internal/binance"
)

func testReadComparisons(t *testing.T, ctx context.Context, store *Store, admin *sql.DB, hash, account string) {
	t.Helper()
	current, err := store.LatestReadSnapshot(ctx, hash, account)
	if err != nil {
		t.Fatal(err)
	}
	report, err := store.LatestReadComparison(ctx, hash, account)
	if err != nil || !report.StoredAt.Equal(current.StoredAt) || !report.Report.CurrentCapturedAt.Equal(current.Snapshot.CapturedAt) {
		t.Fatal("comparison detached from its exact observation")
	}
	if _, err := store.LatestReadComparison(ctx, testDigest(), account); !errors.Is(err, ErrSessionInvalid) {
		t.Fatal("forged session read comparison")
	}
	if _, err := store.LatestReadComparison(ctx, hash, "00000000-0000-4000-8000-000000000099"); !errors.Is(err, ErrAccountMissing) {
		t.Fatal("unknown account comparison exposed")
	}
	identity, err := store.ResolveSession(ctx, hash)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.WithOwner(ctx, identity.OwnerID, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `UPDATE exchange_read_snapshots SET comparison=jsonb_set(comparison,'{executionReady}','true'::jsonb) WHERE account_id=$1`, account)
		return err
	}); err == nil {
		t.Fatal("database allowed comparison to authorize execution")
	}
	// Simulate a pre-migration reading: a NULL comparison must not be treated
	// as no differences or fabricated from a missing previous snapshot.
	if _, err := admin.ExecContext(ctx, `UPDATE exchange_read_snapshots SET comparison=NULL WHERE account_id=$1`, account); err != nil {
		t.Fatal(err)
	}
	if _, err := store.LatestReadComparison(ctx, hash, account); !errors.Is(err, ErrComparisonMissing) {
		t.Fatal("legacy reading manufactured a report")
	}
	var original []byte
	if err := admin.QueryRowContext(ctx, `SELECT snapshot FROM exchange_read_snapshots WHERE account_id=$1`, account).Scan(&original); err != nil {
		t.Fatal(err)
	}
	// Corrupt a financial field while keeping metadata valid; neither compare
	// nor latest may accept it, and a failed save must not overwrite evidence.
	if _, err := admin.ExecContext(ctx, `UPDATE exchange_read_snapshots SET snapshot=jsonb_set(snapshot,'{assets,0,walletBalance}','"NaN"'::jsonb) WHERE account_id=$1`, account); err != nil {
		t.Fatal(err)
	}
	newer := current.Snapshot
	newer.CapturedAt = newer.CapturedAt.Add(time.Second)
	if err := store.SaveReadSnapshot(ctx, hash, account, newer); !errors.Is(err, ErrSnapshotInvalid) {
		t.Fatal("corrupt baseline silently replaced")
	}
	if _, err := store.LatestReadSnapshot(ctx, hash, account); !errors.Is(err, ErrSnapshotInvalid) {
		t.Fatal("corrupt snapshot returned")
	}
	if _, err := admin.ExecContext(ctx, `UPDATE exchange_read_snapshots SET snapshot=$1 WHERE account_id=$2`, original, account); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveReadSnapshot(ctx, hash, account, newer); err != nil {
		t.Fatal("could not compare complete legacy baseline", err)
	}
	recovered, err := store.LatestReadComparison(ctx, hash, account)
	if err != nil || recovered.Report.Status != "compared" {
		t.Fatal("baseline lost across migration/restart")
	}
	// The durable report is untrusted JSON at recovery too. This corruption
	// satisfies the database metadata constraint but must fail the DTO schema.
	var reportJSON []byte
	if err := admin.QueryRowContext(ctx, `SELECT comparison FROM exchange_read_snapshots WHERE account_id=$1`, account).Scan(&reportJSON); err != nil {
		t.Fatal(err)
	}
	if _, err := admin.ExecContext(ctx, `UPDATE exchange_read_snapshots SET comparison=jsonb_set(comparison,'{status}','"unknown"'::jsonb) WHERE account_id=$1`, account); err != nil {
		t.Fatal(err)
	}
	if _, err := store.LatestReadComparison(ctx, hash, account); !errors.Is(err, ErrSnapshotInvalid) {
		t.Fatal("corrupt report returned as trusted evidence")
	}
	if _, err := admin.ExecContext(ctx, `UPDATE exchange_read_snapshots SET comparison=$1 WHERE account_id=$2`, reportJSON, account); err != nil {
		t.Fatal(err)
	}
	// Independent in-flight collections may finish in either order. The newest
	// capture wins, and its comparison must be from the baseline read under lock.
	a, b := newer, newer
	a.Assets = append([]binance.Asset{}, newer.Assets...)
	b.Assets = append([]binance.Asset{}, newer.Assets...)
	a.Assets[0].WalletBalance = "102.00000001"
	b.Assets[0].WalletBalance = "103.00000001"
	a.CapturedAt = a.CapturedAt.Add(time.Second)
	b.CapturedAt = b.CapturedAt.Add(2 * time.Second)
	var workers sync.WaitGroup
	results := make(chan error, 2)
	for _, s := range []binance.Snapshot{a, b} {
		workers.Add(1)
		go func(s binance.Snapshot) {
			defer workers.Done()
			results <- store.SaveReadSnapshot(ctx, hash, account, s)
		}(s)
	}
	workers.Wait()
	close(results)
	for err := range results {
		if err != nil && !errors.Is(err, ErrSnapshotInvalid) {
			t.Fatal("concurrent save failed", err)
		}
	}
	last, err := store.LatestReadSnapshot(ctx, hash, account)
	if err != nil {
		t.Fatal(err)
	}
	lastReport, err := store.LatestReadComparison(ctx, hash, account)
	if err != nil || !last.Snapshot.CapturedAt.Equal(b.CapturedAt) || !lastReport.Report.CurrentCapturedAt.Equal(last.Snapshot.CapturedAt) || !last.StoredAt.Equal(lastReport.StoredAt) {
		t.Fatal("concurrent saves mixed report and observation")
	}
	if lastReport.Report.TotalChanges != 1 || len(lastReport.Report.Changes) != 1 || len(lastReport.Report.Changes[0].Fields) != 1 {
		t.Fatal("concurrent comparison lost changes")
	}
	field := lastReport.Report.Changes[0].Fields[0]
	prior := lastReport.Report.PreviousCapturedAt
	if prior == nil || field.Field != "walletBalance" || field.Before == nil || field.After == nil || *field.After != "103.00000001" {
		t.Fatal("concurrent comparison returned wrong financial field")
	}
	if prior.Equal(a.CapturedAt) {
		if *field.Before != "102.00000001" {
			t.Fatal("comparison used a stale baseline despite intermediate commit")
		}
	} else if !prior.Equal(newer.CapturedAt) || *field.Before != "101.00000001" {
		t.Fatal("comparison baseline not the previous committed observation")
	}
}
