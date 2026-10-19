package database

import (
	"context"
	"database/sql"
	"errors"
	"strconv"
	"testing"

	"regrow/engine/internal/binance"
)

func TestListFillsRejectsInvalidInputBeforeDatabase(t *testing.T) {
	store := &Store{}
	if _, _, _, err := store.ListFills(context.Background(), "forged", "account", 0, 200); !errors.Is(err, ErrAccountMissing) {
		t.Fatal("invalid account reached database")
	}
	if _, _, _, err := store.ListFills(context.Background(), "synthetic", "00000000-0000-4000-8000-000000000001", 0, 200); !errors.Is(err, ErrSessionInvalid) {
		t.Fatal("unregistered session read fills")
	}
}

// Runs only under TEST_DATABASE_URL (CI): durable dedup, ordering, RLS and no-delete.
func testFillLifecycle(t *testing.T, ctx context.Context, store *Store, runtimeURI, hash, account string) {
	t.Helper()
	order := syntheticOrderEvent()
	order.EventTime = 1000
	order.TransactionTime = 1000
	if _, err := store.SaveUserEvents(ctx, account, []binance.UserEvent{order}); err != nil {
		t.Fatal("could not persist fill source", err)
	}
	fills, total, truncated, err := store.ListFills(ctx, hash, account, 0, 200)
	if err != nil || truncated || total != 1 || len(fills) != 1 ||
		fills[0].OrderID != "9007199254740993" || fills[0].TradeID != "12345678" ||
		fills[0].Price != "9910.00" || fills[0].Quantity != "0.0005" || fills[0].Commission != "0.0010" {
		t.Fatalf("fill not durable or decimals lost: %+v err=%v", fills, err)
	}
	// Redelivering the same event must not duplicate the fill.
	if _, err := store.SaveUserEvents(ctx, account, []binance.UserEvent{order}); err != nil {
		t.Fatal(err)
	}
	fills, total, _, err = store.ListFills(ctx, hash, account, 0, 200)
	if err != nil || total != 1 || len(fills) != 1 {
		t.Fatal("fill dedup by (order, trade) failed")
	}
	// A distinct journal event for the same trade must not duplicate or
	// overwrite its original execution data.
	redelivery := order
	changed := *order.OrderUpdate
	redelivery.OrderUpdate = &changed
	redelivery.EventTime = 1500
	redelivery.OrderUpdate.LastFilledPrice = "9920"
	if saved, err := store.SaveUserEvents(ctx, account, []binance.UserEvent{redelivery}); err != nil || saved != 1 {
		t.Fatal("distinct journal event did not persist", err)
	}
	fills, total, _, err = store.ListFills(ctx, hash, account, 0, 200)
	if err != nil || total != 1 || len(fills) != 1 || fills[0].EventTime != 1000 || fills[0].Price != "9910.00" {
		t.Fatal("same trade in another journal event overwrote the fill")
	}
	// A second tradeId on the same order is a distinct fill.
	second := order
	clone := *order.OrderUpdate
	second.OrderUpdate = &clone
	second.OrderUpdate.TradeID = "99999999"
	second.EventTime = 2000
	second.TransactionTime = 2000
	if _, err := store.SaveUserEvents(ctx, account, []binance.UserEvent{second}); err != nil {
		t.Fatal(err)
	}
	fills, total, truncated, err = store.ListFills(ctx, hash, account, 0, 200)
	if err != nil || truncated || total != 2 || len(fills) != 2 ||
		fills[0].TradeID != "12345678" || fills[1].TradeID != "99999999" || fills[0].EventTime != 1000 || fills[1].EventTime != 2000 {
		t.Fatalf("fills not deduplicated by tradeId or ordered: %+v", fills)
	}
	// The exchange watermark bounds the page strictly by event_time.
	fills, total, _, err = store.ListFills(ctx, hash, account, 1000, 200)
	if err != nil || total != 1 || len(fills) != 1 || fills[0].TradeID != "99999999" {
		t.Fatal("fill watermark did not bound the page")
	}
	// Complete totals must survive a bounded page and a connection-pool restart.
	batch := make([]binance.UserEvent, 0, 203)
	for i := range 203 {
		event := order
		update := *order.OrderUpdate
		event.OrderUpdate = &update
		event.OrderUpdate.TradeID = strconv.Itoa(100000000 + i)
		event.EventTime = int64(3000 + i)
		event.TransactionTime = event.EventTime
		batch = append(batch, event)
	}
	if _, err := store.SaveUserEvents(ctx, account, batch); err != nil {
		t.Fatal("could not persist bounded-page test fills", err)
	}
	restarted, err := Open(ctx, runtimeURI, store.migrations)
	if err != nil {
		t.Fatal("could not reopen fill pool", err)
	}
	defer restarted.Close()
	for _, limit := range []int{0, 200, 201} {
		fills, total, truncated, err = restarted.ListFills(ctx, hash, account, 0, limit)
		if err != nil || !truncated || total != 205 || len(fills) != 200 || fills[0].EventTime != 1000 || fills[199].EventTime != 3197 {
			t.Fatal("fill page lost complete count, ordering or durability", err)
		}
	}
	fills, total, truncated, err = store.ListFills(ctx, hash, account, 3201, 1)
	if err != nil || truncated || total != 1 || len(fills) != 1 || fills[0].EventTime != 3202 {
		t.Fatal("exact fill page incorrectly truncated")
	}
	fills, total, truncated, err = store.ListFills(ctx, hash, account, 3202, 200)
	if err != nil || truncated || total != 0 || fills == nil || len(fills) != 0 {
		t.Fatal("empty fill page was not explicit")
	}
	// A different owner cannot read the fills table.
	if err := store.WithOwner(ctx, "00000000-0000-4000-8000-000000000002", func(tx *sql.Tx) error {
		var count int
		if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM binance_fills`).Scan(&count); err != nil {
			return err
		}
		if count != 0 {
			return errors.New("cross-owner fill read")
		}
		return nil
	}); err != nil {
		t.Fatal("fill RLS failed")
	}
	// The runtime role must not delete fill rows.
	identity, err := store.ResolveSession(ctx, hash)
	if err != nil {
		t.Fatal("missing test identity")
	}
	if err := store.WithOwner(ctx, identity.OwnerID, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `DELETE FROM binance_fills WHERE account_id=$1`, account)
		return err
	}); err == nil {
		t.Fatal("runtime can delete fills")
	}
	if err := store.WithOwner(ctx, identity.OwnerID, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `UPDATE binance_fills SET price='1' WHERE account_id=$1`, account)
		return err
	}); err == nil {
		t.Fatal("runtime can overwrite fills")
	}
	if _, _, _, err := store.ListFills(ctx, testDigest(), account, 0, 200); !errors.Is(err, ErrSessionInvalid) {
		t.Fatal("unregistered session read fills")
	}
	if _, err := store.SaveUserEvents(ctx, "00000000-0000-4000-8000-000000000099", []binance.UserEvent{order}); !errors.Is(err, ErrAccountMissing) {
		t.Fatal("unknown account accepted fills")
	}
}
