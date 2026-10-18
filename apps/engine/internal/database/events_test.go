package database

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"regrow/engine/internal/binance"
)

func syntheticOrderEvent() binance.UserEvent {
	return binance.UserEvent{
		Type:            binance.EventOrderTradeUpdate,
		EventTime:       1568879465651,
		TransactionTime: 1568879465650,
		OrderUpdate: &binance.OrderUpdate{
			Symbol: "BTCUSDT", ClientOrderID: "client-1", Side: "SELL", PositionSide: "SHORT",
			Type: "LIMIT", TimeInForce: "GTC", Quantity: "0.001", Price: "9910", StopPrice: "0",
			ActivatePrice: "0", WorkingType: "CONTRACT_PRICE", ExecutionType: "TRADE", OrderStatus: "PARTIALLY_FILLED",
			OrderID: "9007199254740993", LastFilledQty: "0.0005", CumulativeQty: "0.0005", LastFilledPrice: "9910.00",
			Commission: "0.0010", CommissionAsset: "USDT", TradeID: "12345678", ReduceOnly: true, ClosePosition: false, RealizedProfit: "-0.1",
		},
	}
}

func syntheticAccountEvent() binance.UserEvent {
	return binance.UserEvent{
		Type: binance.EventAccountUpdate, EventTime: 1564745798939, TransactionTime: 1564745798938,
		AccountUpdate: &binance.AccountUpdate{
			Reason:    "ORDER",
			Balances:  []binance.BalanceUpdate{{Asset: "USDT", WalletBalance: "122624.12345678", CrossWalletBalance: "100.12345678", BalanceChange: "50.12345678"}},
			Positions: []binance.PositionUpdate{{Symbol: "BTCUSDT", PositionAmount: "0", EntryPrice: "0.00000", AccumulatedRealized: "200", UnrealizedPnL: "0", MarginType: "isolated", IsolatedWallet: "0", PositionSide: "LONG"}},
		},
	}
}

func TestUserEventRejectsIncompleteBeforeDatabase(t *testing.T) {
	valid := syntheticOrderEvent()
	for name, mutate := range map[string]func(*binance.UserEvent){
		"missing order":   func(e *binance.UserEvent) { e.OrderUpdate = nil },
		"missing type":    func(e *binance.UserEvent) { e.Type = "" },
		"zero event time": func(e *binance.UserEvent) { e.EventTime = 0 },
		"bad symbol":      func(e *binance.UserEvent) { e.OrderUpdate.Symbol = "BTCUSDC" },
		"bad quantity":    func(e *binance.UserEvent) { e.OrderUpdate.Quantity = "NaN" },
		"account + order": func(e *binance.UserEvent) { e.AccountUpdate = syntheticAccountEvent().AccountUpdate },
	} {
		t.Run(name, func(t *testing.T) {
			event := valid
			mutate(&event)
			if _, err := (&Store{}).SaveUserEvents(context.Background(), "00000000-0000-4000-8000-000000000001", []binance.UserEvent{event}); !errors.Is(err, ErrEventInvalid) {
				t.Fatal("invalid event reached database")
			}
		})
	}
	if _, err := (&Store{}).SaveUserEvents(context.Background(), "forged", []binance.UserEvent{valid}); !errors.Is(err, ErrAccountMissing) {
		t.Fatal("forged account reached database")
	}
	if n, err := (&Store{}).SaveUserEvents(context.Background(), "00000000-0000-4000-8000-000000000001", nil); n != 0 || err != nil {
		t.Fatal("empty batch should be a no-op")
	}
}

// Runs only under TEST_DATABASE_URL (CI): dedup, ordering, RLS and no-delete.
func testUserEventLifecycle(t *testing.T, ctx context.Context, store *Store, hash, account string) {
	t.Helper()
	order := syntheticOrderEvent()
	accountUpdate := syntheticAccountEvent()
	order.EventTime = 1000
	accountUpdate.EventTime = 2000
	accountUpdate.TransactionTime = 2000
	saved, err := store.SaveUserEvents(ctx, account, []binance.UserEvent{order, accountUpdate})
	if err != nil || saved != 2 {
		t.Fatal("could not persist synthetic events", err, saved)
	}
	again, err := store.SaveUserEvents(ctx, account, []binance.UserEvent{order, accountUpdate})
	if err != nil || again != 0 {
		t.Fatal("dedup did not suppress duplicate events", err, again)
	}
	events, truncated, err := store.ListUserEvents(ctx, hash, account, 0, 200)
	if err != nil || truncated || len(events) != 2 || events[0].Event.EventTime != 1000 || events[1].Event.EventTime != 2000 ||
		events[0].Event.OrderUpdate.OrderID != "9007199254740993" || events[1].Event.AccountUpdate.Balances[0].WalletBalance != "122624.12345678" {
		t.Fatal("event journal not ordered or preserved decimals")
	}
	// A different owner cannot read the journal.
	if err := store.WithOwner(ctx, "00000000-0000-4000-8000-000000000002", func(tx *sql.Tx) error {
		var count int
		if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM binance_user_events`).Scan(&count); err != nil {
			return err
		}
		if count != 0 {
			return errors.New("cross-owner event read")
		}
		return nil
	}); err != nil {
		t.Fatal("event RLS failed")
	}
	// The runtime role must not delete journal rows.
	identity, err := store.ResolveSession(ctx, hash)
	if err != nil {
		t.Fatal("missing test identity")
	}
	if err := store.WithOwner(ctx, identity.OwnerID, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `DELETE FROM binance_user_events WHERE account_id=$1`, account)
		return err
	}); err == nil {
		t.Fatal("runtime can delete event journal")
	}
	if _, _, err := store.ListUserEvents(ctx, testDigest(), account, 0, 200); !errors.Is(err, ErrSessionInvalid) {
		t.Fatal("unregistered session read events")
	}
	if _, err := store.SaveUserEvents(ctx, "00000000-0000-4000-8000-000000000099", []binance.UserEvent{order}); !errors.Is(err, ErrAccountMissing) {
		t.Fatal("unknown account accepted events")
	}
}
