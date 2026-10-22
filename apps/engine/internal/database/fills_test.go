package database

import (
	"context"
	"database/sql"
	"errors"
	"reflect"
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

// Runs only under TEST_DATABASE_URL (CI): recovered REST fills dedup against the
// stream fills, require credentials/session and remain append-only under RLS.
func testRecoveredFillsLifecycle(t *testing.T, ctx context.Context, store *Store, admin *sql.DB, hash, account string) {
	t.Helper()
	beforeSnapshot, err := store.LatestReadSnapshot(ctx, hash, account)
	if err != nil {
		t.Fatal(err)
	}
	beforeStream, err := store.LatestStreamState(ctx, hash, account)
	if err != nil {
		t.Fatal(err)
	}
	empty, err := store.SaveRecoveredFills(ctx, hash, account, []binance.Fill{})
	if err != nil || empty != (RecoveredFillsResult{}) {
		t.Fatal("empty recovery must succeed after checking session and credentials", err)
	}
	fresh := binance.Fill{OrderID: "9007199254741111", TradeID: "22222222", Symbol: "BTCUSDT", Side: "BUY", PositionSide: "LONG", Price: "100.000", Quantity: "1.5", Commission: "-0.10", CommissionAsset: "USDT", EventTime: 5000, TransactionTime: 5000}
	result, err := store.SaveRecoveredFills(ctx, hash, account, []binance.Fill{fresh})
	if err != nil || result.Recovered != 1 || result.Duplicates != 0 {
		t.Fatal("recovered fill not inserted", err, result)
	}
	// Recovering the same fill is an explicit duplicate, never an overwrite.
	result, err = store.SaveRecoveredFills(ctx, hash, account, []binance.Fill{fresh})
	if err != nil || result.Recovered != 0 || result.Duplicates != 1 {
		t.Fatal("duplicate recovery not counted", err, result)
	}
	fills, total, truncated, err := store.ListFills(ctx, hash, account, 4000, 200)
	if err != nil || truncated || total != 1 || len(fills) != 1 || fills[0].TradeID != "22222222" || fills[0].EventTime != 5000 {
		t.Fatal("recovered fill did not layer on the exchange watermark", err, total)
	}
	if _, err := store.SaveRecoveredFills(ctx, hash, "bad", []binance.Fill{fresh}); !errors.Is(err, ErrAccountMissing) {
		t.Fatal("invalid account accepted recovery")
	}
	if _, err := store.SaveRecoveredFills(ctx, hash, account, nil); !errors.Is(err, ErrSnapshotInvalid) {
		t.Fatal("empty recovery accepted")
	}
	if _, err := store.SaveRecoveredFills(ctx, testDigest(), account, []binance.Fill{fresh}); !errors.Is(err, ErrSessionInvalid) {
		t.Fatal("unregistered session recovered fills")
	}
	if _, err := store.SaveRecoveredFills(ctx, testDigest(), account, []binance.Fill{}); !errors.Is(err, ErrSessionInvalid) {
		t.Fatal("empty recovery bypassed session revalidation", err)
	}
	if _, err := store.SaveRecoveredFills(ctx, hash, "00000000-0000-4000-8000-000000000099", []binance.Fill{fresh}); !errors.Is(err, ErrAccountMissing) {
		t.Fatal("unknown account accepted recovery")
	}
	var audits int
	if err := admin.QueryRowContext(ctx, `SELECT count(*) FROM audit_events WHERE action='account.fills_recovered' AND resource_id=$1 AND metadata IN ('{"recovered":0,"duplicates":0}'::jsonb,'{"recovered":1,"duplicates":0}'::jsonb,'{"recovered":0,"duplicates":1}'::jsonb)`, account).Scan(&audits); err != nil || audits != 3 {
		t.Fatal("missing or unsafe recovery audit", audits, err)
	}
	// An audit failure must roll back all new fills, never acknowledge a partial save.
	var role string
	if err := store.db.QueryRowContext(ctx, `SELECT current_user`).Scan(&role); err != nil {
		t.Fatal(err)
	}
	if _, err := admin.ExecContext(ctx, `REVOKE INSERT ON audit_events FROM `+role); err != nil {
		t.Fatal(err)
	}
	failed := fresh
	failed.TradeID = "22222223"
	result, saveErr := store.SaveRecoveredFills(ctx, hash, account, []binance.Fill{fresh, failed})
	if _, err := admin.ExecContext(ctx, `GRANT INSERT ON audit_events TO `+role); err != nil {
		t.Fatal(err)
	}
	if saveErr == nil || result != (RecoveredFillsResult{}) {
		t.Fatal("audit failure acknowledged partial recovery", saveErr, result)
	}
	_, total, _, err = store.ListFills(ctx, hash, account, 4000, 200)
	if err != nil || total != 1 {
		t.Fatal("audit failure did not roll back the recovered fill", err, total)
	}
	// Revocation is checked even for a successful external read with zero fills.
	revoked := testDigest()
	if _, err := store.CreateSession(ctx, revoked); err != nil {
		t.Fatal(err)
	}
	if err := store.RevokeSession(ctx, revoked, false); err != nil {
		t.Fatal(err)
	}
	if _, err := store.SaveRecoveredFills(ctx, revoked, account, []binance.Fill{}); !errors.Is(err, ErrSessionInvalid) {
		t.Fatal("revoked session saved empty recovery", err)
	}
	// Credential presence alone cannot authorize another owner or environment.
	identity, err := store.ResolveSession(ctx, hash)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ id, owner, environment string }{
		{"00000000-0000-4000-8000-000000000095", "00000000-0000-4000-8000-000000000001", "production"},
		{"00000000-0000-4000-8000-000000000096", identity.OwnerID, "testnet"},
	} {
		if _, err := admin.ExecContext(ctx, `INSERT INTO exchange_accounts(id,owner_id,exchange,environment,name) VALUES($1,$2,'binance',$3,'Synthetic recovery boundary')`, tc.id, tc.owner, tc.environment); err != nil {
			t.Fatal(err)
		}
		defer admin.ExecContext(ctx, `DELETE FROM exchange_accounts WHERE id=$1`, tc.id)
		if _, err := admin.ExecContext(ctx, `INSERT INTO exchange_credentials(account_id,owner_id,envelope) VALUES($1,$2,'{}')`, tc.id, tc.owner); err != nil {
			t.Fatal(err)
		}
		defer admin.ExecContext(ctx, `DELETE FROM exchange_credentials WHERE account_id=$1`, tc.id)
		if _, err := store.SaveRecoveredFills(ctx, hash, tc.id, []binance.Fill{}); !errors.Is(err, ErrAccountMissing) {
			t.Fatal("unsupported account accepted empty recovery", err)
		}
	}
	afterSnapshot, err := store.LatestReadSnapshot(ctx, hash, account)
	if err != nil {
		t.Fatal(err)
	}
	afterStream, err := store.LatestStreamState(ctx, hash, account)
	if err != nil || !reflect.DeepEqual(beforeSnapshot, afterSnapshot) || beforeStream != afterStream {
		t.Fatal("recovery changed the baseline or stream continuity", err)
	}
}

func TestRecoveredFillsRejectMalformedBatchesBeforeDatabase(t *testing.T) {
	store := &Store{}
	account := "00000000-0000-4000-8000-000000000001"
	fresh := binance.Fill{OrderID: "1", TradeID: "2", Symbol: "BTCUSDT", Side: "BUY", PositionSide: "LONG", Price: "100", Quantity: "1", Commission: "-0.1", CommissionAsset: "USDT", EventTime: 1000, TransactionTime: 1000}
	for _, mutate := range []func(*binance.Fill){
		func(f *binance.Fill) { f.Price = "NaN" },
		func(f *binance.Fill) { f.Quantity = "0" },
		func(f *binance.Fill) { f.TradeID = "9223372036854775808" },
		func(f *binance.Fill) { f.Commission = "1e-8" },
		func(f *binance.Fill) { f.EventTime++ },
	} {
		invalid := fresh
		mutate(&invalid)
		if _, err := store.SaveRecoveredFills(context.Background(), "synthetic", account, []binance.Fill{fresh, invalid}); !errors.Is(err, ErrSnapshotInvalid) {
			t.Fatal("malformed recovery reached database", err)
		}
	}
	if _, err := store.SaveRecoveredFills(context.Background(), "synthetic", account, make([]binance.Fill, maxRecoveredFills+1)); !errors.Is(err, ErrSnapshotInvalid) {
		t.Fatal("oversized recovery reached database", err)
	}
	if _, err := store.SaveRecoveredFills(context.Background(), "synthetic", account, []binance.Fill{}); !errors.Is(err, ErrSessionInvalid) {
		t.Fatal("empty recovery did not revalidate the session", err)
	}
}
