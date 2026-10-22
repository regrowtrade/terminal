package binance

// Fill is a single TRADE execution from the stream or REST, deduplicated by
// (orderId, tradeId). Amounts remain original decimal strings; it is never a
// reconciled ledger entry and does not infer continuity or execution readiness.
type Fill struct {
	OrderID         string `json:"orderId"`
	TradeID         string `json:"tradeId"`
	Symbol          string `json:"symbol"`
	Side            string `json:"side"`
	PositionSide    string `json:"positionSide"`
	Price           string `json:"price"`
	Quantity        string `json:"quantity"`
	Commission      string `json:"commission"`
	CommissionAsset string `json:"commissionAsset"`
	EventTime       int64  `json:"eventTime"`
	TransactionTime int64  `json:"transactionTime"`
}

// Fill returns the execution carried by a TRADE order update, or nil when the
// event is not a TRADE or carries no real tradeId. It is pure: callers validate
// the event before persisting it.
func (e UserEvent) Fill() *Fill {
	if e.Type != EventOrderTradeUpdate || e.OrderUpdate == nil || e.OrderUpdate.ExecutionType != "TRADE" || e.OrderUpdate.TradeID == "0" {
		return nil
	}
	o := e.OrderUpdate
	return &Fill{
		OrderID: o.OrderID, TradeID: o.TradeID, Symbol: o.Symbol, Side: o.Side,
		PositionSide: o.PositionSide, Price: o.LastFilledPrice, Quantity: o.LastFilledQty,
		Commission: o.Commission, CommissionAsset: o.CommissionAsset,
		EventTime: e.EventTime, TransactionTime: e.TransactionTime,
	}
}
