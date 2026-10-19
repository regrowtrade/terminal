package binance

import "testing"

func tradeEvent(executionType, tradeID string) UserEvent {
	return UserEvent{
		Type: EventOrderTradeUpdate, EventTime: 1000, TransactionTime: 999,
		OrderUpdate: &OrderUpdate{
			Symbol: "BTCUSDT", ClientOrderID: "client-1", Side: "SELL", PositionSide: "SHORT",
			Type: "LIMIT", TimeInForce: "GTC", Quantity: "1", Price: "9910", StopPrice: "0",
			ActivatePrice: "0", WorkingType: "CONTRACT_PRICE", ExecutionType: executionType, OrderStatus: "PARTIALLY_FILLED",
			OrderID: "9007199254740993", LastFilledQty: "0.5", CumulativeQty: "0.5", LastFilledPrice: "9910.00",
			Commission: "0.0010", CommissionAsset: "USDT", TradeID: tradeID, ReduceOnly: true, ClosePosition: false, RealizedProfit: "-0.1",
		},
	}
}

func TestFillExtractsTradeIdentityAndDecimals(t *testing.T) {
	f := tradeEvent("TRADE", "12345678").Fill()
	if f == nil || f.OrderID != "9007199254740993" || f.TradeID != "12345678" || f.Symbol != "BTCUSDT" ||
		f.Side != "SELL" || f.PositionSide != "SHORT" || f.Price != "9910.00" || f.Quantity != "0.5" ||
		f.Commission != "0.0010" || f.CommissionAsset != "USDT" || f.EventTime != 1000 || f.TransactionTime != 999 {
		t.Fatalf("fill lost identity or decimals: %+v", f)
	}
}

func TestFillSkipsNonTradeAndZeroTradeID(t *testing.T) {
	if tradeEvent("NEW", "12345678").Fill() != nil {
		t.Fatal("non-TRADE execution produced a fill")
	}
	if tradeEvent("TRADE", "0").Fill() != nil {
		t.Fatal("zero tradeId produced a fill")
	}
	account := UserEvent{Type: EventAccountUpdate, EventTime: 1, TransactionTime: 1, AccountUpdate: &AccountUpdate{Reason: "ORDER"}}
	if account.Fill() != nil {
		t.Fatal("account update produced a fill")
	}
}
