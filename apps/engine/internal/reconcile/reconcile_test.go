package reconcile

import (
	"testing"
	"time"

	"regrow/engine/internal/binance"
)

func tradeUpdate(orderID, tradeID string, executionType string, eventTime, transactionTime int64) binance.UserEvent {
	return binance.UserEvent{
		Type: binance.EventOrderTradeUpdate, EventTime: eventTime, TransactionTime: transactionTime,
		OrderUpdate: &binance.OrderUpdate{
			Symbol: "BTCUSDT", ClientOrderID: "client-1", Side: "SELL", PositionSide: "SHORT",
			Type: "LIMIT", TimeInForce: "GTC", Quantity: "1", Price: "9910", StopPrice: "0",
			ActivatePrice: "0", WorkingType: "CONTRACT_PRICE", ExecutionType: executionType, OrderStatus: "PARTIALLY_FILLED",
			OrderID: orderID, LastFilledQty: "0.5", CumulativeQty: "0.5", LastFilledPrice: "9910.00",
			Commission: "0.0010", CommissionAsset: "USDT", TradeID: tradeID, ReduceOnly: true, ClosePosition: false, RealizedProfit: "-0.1",
		},
	}
}

func accountUpdate(eventTime, transactionTime int64) binance.UserEvent {
	return binance.UserEvent{
		Type: binance.EventAccountUpdate, EventTime: eventTime, TransactionTime: transactionTime,
		AccountUpdate: &binance.AccountUpdate{
			Reason:    "ORDER",
			Balances:  []binance.BalanceUpdate{{Asset: "USDT", WalletBalance: "100", CrossWalletBalance: "50", BalanceChange: "10"}},
			Positions: []binance.PositionUpdate{{Symbol: "BTCUSDT", PositionAmount: "-1", EntryPrice: "9900", AccumulatedRealized: "0", UnrealizedPnL: "-1", MarginType: "isolated", IsolatedWallet: "0", PositionSide: "SHORT"}},
		},
	}
}

func fill(orderID, tradeID string, eventTime int64) binance.Fill {
	return binance.Fill{OrderID: orderID, TradeID: tradeID, Symbol: "BTCUSDT", Side: "SELL", PositionSide: "SHORT", Price: "9910.00", Quantity: "0.5", Commission: "0.0010", CommissionAsset: "USDT", EventTime: eventTime, TransactionTime: eventTime}
}

func snapshot() binance.Snapshot {
	return binance.Snapshot{Source: "binance-production", Mode: "read_only", Consistency: "rest_non_atomic", CapturedAt: time.Now().UTC(), CapturedAtMs: time.Now().UnixMilli()}
}

func TestReconcileCountsAndLayersDurableFills(t *testing.T) {
	events := []binance.UserEvent{
		tradeUpdate("9007199254740993", "1001", "TRADE", 1000, 999),
		tradeUpdate("9007199254740993", "1001", "TRADE", 1000, 999), // duplicate event
		tradeUpdate("9007199254740993", "1002", "TRADE", 1001, 1000),
		accountUpdate(1002, 1001),
	}
	fills := []binance.Fill{fill("9007199254740993", "1001", 1000), fill("9007199254740993", "1002", 1001)}
	report := Reconcile(snapshot(), events, fills, 2, false, true)
	if report.Continuity != ContinuityEstablished || report.ExecutionReady {
		t.Fatal("continuity or execution flag wrong")
	}
	if report.TotalEvents != 3 || report.OrderUpdates != 2 || report.AccountUpdates != 1 {
		t.Fatalf("event families miscounted: %+v", report)
	}
	if report.TotalFills != 2 || len(report.Fills) != 2 || report.FillsTruncated {
		t.Fatalf("durable fills not layered: %+v", report)
	}
	if report.Fills[0].TradeID != "1001" || report.Fills[1].TradeID != "1002" {
		t.Fatal("fills lost trade identity or order")
	}
	if report.Fills[0].Price != "9910.00" || report.Fills[0].Quantity != "0.5" || report.Fills[0].Commission != "0.0010" {
		t.Fatal("fill lost decimal fields")
	}
	if report.LastEventTime != 1002 {
		t.Fatal("last event time not tracked")
	}
}

func TestReconcileSortsOutOfOrderAndCountsAccountUpdates(t *testing.T) {
	events := []binance.UserEvent{
		accountUpdate(3000, 2999),
		accountUpdate(1000, 999),
		tradeUpdate("9007199254740993", "1001", "TRADE", 2000, 1999),
	}
	report := Reconcile(snapshot(), events, nil, 0, false, true)
	if report.TotalEvents != 3 || report.AccountUpdates != 2 || report.OrderUpdates != 1 {
		t.Fatal("event families miscounted")
	}
	if report.LastEventTime != 3000 {
		t.Fatal("out-of-order events not sorted for last event time")
	}
}

func TestReconcileGappedWhenDisconnected(t *testing.T) {
	report := Reconcile(snapshot(), []binance.UserEvent{accountUpdate(1000, 999)}, nil, 0, false, false)
	if report.Continuity != ContinuityGapped || report.ExecutionReady {
		t.Fatal("disconnection did not mark gapped")
	}
	if report.Fills == nil {
		t.Fatal("fills should be an empty slice, not nil")
	}
}

func TestReconcileTruncatesFills(t *testing.T) {
	fills := make([]binance.Fill, 0, FillLimit+5)
	for i := 0; i < FillLimit+5; i++ {
		fills = append(fills, fill("9007199254740993", id(i+1), int64(1000+i)))
	}
	report := Reconcile(snapshot(), nil, fills, FillLimit+5, true, true)
	if report.TotalFills != FillLimit+5 || len(report.Fills) != FillLimit || !report.FillsTruncated {
		t.Fatalf("fill truncation wrong: total=%d kept=%d truncated=%v", report.TotalFills, len(report.Fills), report.FillsTruncated)
	}
}

func TestReconcileSkipsInvalidEvents(t *testing.T) {
	invalid := tradeUpdate("9007199254740993", "1001", "TRADE", 1000, 999)
	invalid.OrderUpdate.Symbol = "BTCUSDC" // outside USDT scope
	report := Reconcile(snapshot(), []binance.UserEvent{invalid, accountUpdate(1000, 999)}, nil, 0, false, true)
	if report.TotalEvents != 1 || report.AccountUpdates != 1 || report.TotalFills != 0 {
		t.Fatalf("invalid event not skipped: %+v", report)
	}
}

func id(n int) string {
	const digits = "0123456789"
	out := make([]byte, 0, 8)
	x := n
	for {
		out = append([]byte{digits[x%10]}, out...)
		x /= 10
		if x == 0 {
			break
		}
	}
	return string(out)
}
