package database

import (
	"context"
	"database/sql"
	"errors"
	"testing"
)

func TestStreamStateRejectsInvalidInputBeforeDatabase(t *testing.T) {
	store := &Store{}
	if err := store.SetStreamConnected(context.Background(), "forged", true); !errors.Is(err, ErrAccountMissing) {
		t.Fatal("invalid account reached database")
	}
	if err := store.RecordStreamEvent(context.Background(), "00000000-0000-4000-8000-000000000001", 0); !errors.Is(err, ErrAccountMissing) {
		t.Fatal("invalid event time reached database")
	}
	if err := store.RecordStreamEvent(context.Background(), "00000000-0000-4000-8000-000000000001", 9007199254740992); !errors.Is(err, ErrAccountMissing) {
		t.Fatal("oversized event time reached database")
	}
	if _, err := store.LatestStreamState(context.Background(), "forged", "account"); !errors.Is(err, ErrAccountMissing) {
		t.Fatal("invalid account read accepted")
	}
}

// Runs only under TEST_DATABASE_URL (CI).
func testStreamStateLifecycle(t *testing.T, ctx context.Context, store *Store, hash, account string) {
	t.Helper()
	state, err := store.LatestStreamState(ctx, hash, account)
	if err != nil || state.Connected || state.GappedSinceSnapshot || state.LastEventTime != 0 {
		t.Fatal("missing state should default to disconnected and not gapped")
	}
	if err := store.SetStreamConnected(ctx, account, true); err != nil {
		t.Fatal(err)
	}
	state, err = store.LatestStreamState(ctx, hash, account)
	if err != nil || !state.Connected || state.GappedSinceSnapshot {
		t.Fatal("connect transition not durable")
	}
	if err := store.RecordStreamEvent(ctx, account, 100); err != nil {
		t.Fatal(err)
	}
	if err := store.RecordStreamEvent(ctx, account, 50); err != nil {
		t.Fatal(err)
	}
	state, err = store.LatestStreamState(ctx, hash, account)
	if err != nil || state.LastEventTime != 100 {
		t.Fatal("event watermark rolled backwards")
	}
	if err := store.SetStreamConnected(ctx, account, false); err != nil {
		t.Fatal(err)
	}
	state, err = store.LatestStreamState(ctx, hash, account)
	if err != nil || state.Connected || !state.GappedSinceSnapshot {
		t.Fatal("disconnect did not mark a gap")
	}
	// A fresh snapshot resets the gap but not the watermark.
	if err := store.SaveReadSnapshot(ctx, hash, account, syntheticReadSnapshot()); err != nil {
		t.Fatal(err)
	}
	state, err = store.LatestStreamState(ctx, hash, account)
	if err != nil || state.GappedSinceSnapshot || state.LastEventTime != 100 {
		t.Fatal("snapshot did not reset the gap")
	}
	// A different owner cannot read stream state.
	if err := store.WithOwner(ctx, "00000000-0000-4000-8000-000000000002", func(tx *sql.Tx) error {
		var count int
		if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM binance_stream_state`).Scan(&count); err != nil {
			return err
		}
		if count != 0 {
			return errors.New("cross-owner stream state read")
		}
		return nil
	}); err != nil {
		t.Fatal("stream state RLS failed")
	}
	if _, err := store.LatestStreamState(ctx, testDigest(), account); !errors.Is(err, ErrSessionInvalid) {
		t.Fatal("unregistered session read stream state")
	}
}
