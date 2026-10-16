// Private user data stream events. This is read-only: the listenKey stream
// receives account/order updates but never places, cancels or amends orders.
// Events are normalized and validated for durable journaling; they do not
// reconcile fills, establish continuity or enable execution.
package binance

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
)

const (
	EventOrderTradeUpdate = "ORDER_TRADE_UPDATE"
	EventAccountUpdate    = "ACCOUNT_UPDATE"
	EventListenKeyExpired = "listenKeyExpired"
)

// UserEvent is the normalized subset of a private stream message we retain.
// Only ORDER_TRADE_UPDATE and ACCOUNT_UPDATE are modeled; everything else is
// ignored by the stream without being treated as a trade.
type UserEvent struct {
	Type            string         `json:"type"`
	EventTime       int64          `json:"eventTime"`
	TransactionTime int64          `json:"transactionTime"`
	OrderUpdate     *OrderUpdate   `json:"orderUpdate,omitempty"`
	AccountUpdate   *AccountUpdate `json:"accountUpdate,omitempty"`
}

// OrderUpdate carries a single order lifecycle transition. Decimal amounts and
// int64 IDs stay as original strings; they are never float or JS numbers.
type OrderUpdate struct {
	Symbol          string `json:"symbol"`
	ClientOrderID   string `json:"clientOrderId"`
	Side            string `json:"side"`
	PositionSide    string `json:"positionSide"`
	Type            string `json:"type"`
	TimeInForce     string `json:"timeInForce"`
	Quantity        string `json:"quantity"`
	Price           string `json:"price"`
	StopPrice       string `json:"stopPrice"`
	ActivatePrice   string `json:"activatePrice,omitempty"`
	WorkingType     string `json:"workingType"`
	ExecutionType   string `json:"executionType"`
	OrderStatus     string `json:"orderStatus"`
	OrderID         string `json:"orderId"`
	LastFilledQty   string `json:"lastFilledQty"`
	CumulativeQty   string `json:"cumulativeQty"`
	LastFilledPrice string `json:"lastFilledPrice"`
	Commission      string `json:"commission"`
	CommissionAsset string `json:"commissionAsset"`
	TradeID         string `json:"tradeId"`
	ReduceOnly      bool   `json:"reduceOnly"`
	ClosePosition   bool   `json:"closePosition"`
	RealizedProfit  string `json:"realizedProfit"`
}

type AccountUpdate struct {
	Reason    string           `json:"reason"`
	Balances  []BalanceUpdate  `json:"balances"`
	Positions []PositionUpdate `json:"positions"`
}
type BalanceUpdate struct {
	Asset              string `json:"asset"`
	WalletBalance      string `json:"walletBalance"`
	CrossWalletBalance string `json:"crossWalletBalance"`
	BalanceChange      string `json:"balanceChange"`
}
type PositionUpdate struct {
	Symbol              string `json:"symbol"`
	PositionAmount      string `json:"positionAmount"`
	EntryPrice          string `json:"entryPrice"`
	AccumulatedRealized string `json:"accumulatedRealized"`
	UnrealizedPnL       string `json:"unrealizedPnL"`
	MarginType          string `json:"marginType"`
	IsolatedWallet      string `json:"isolatedWallet"`
	PositionSide        string `json:"positionSide"`
}

type streamEnvelope struct {
	EventType       string          `json:"e"`
	EventTime       int64           `json:"E"`
	TransactionTime int64           `json:"T"`
	Order           json.RawMessage `json:"o"`
	Account         json.RawMessage `json:"a"`
}

// StreamEventType reports the exchange event discriminator without decoding the
// financial payload, so unknown events can be ignored while listenKeyExpired
// still triggers a reconnect.
func StreamEventType(raw []byte) string {
	var e struct {
		EventType       string `json:"e"`
		EventTime       int64  `json:"E"`
		TransactionTime int64  `json:"T"`
	}
	if json.Unmarshal(raw, &e) != nil {
		return ""
	}
	return e.EventType
}

// ParseUserEvent decodes and validates ORDER_TRADE_UPDATE or ACCOUNT_UPDATE.
// A recognized-but-malformed message is an error (fail closed); an unknown
// type is reported separately by StreamEventType and never reaches this path.
func ParseUserEvent(raw []byte) (UserEvent, error) {
	var envelope streamEnvelope
	if json.Unmarshal(raw, &envelope) != nil {
		return UserEvent{}, ErrResponse
	}
	switch envelope.EventType {
	case EventOrderTradeUpdate:
		return parseOrderUpdate(envelope)
	case EventAccountUpdate:
		return parseAccountUpdate(envelope)
	default:
		return UserEvent{}, ErrResponse
	}
}

func parseOrderUpdate(envelope streamEnvelope) (UserEvent, error) {
	event := UserEvent{Type: EventOrderTradeUpdate, EventTime: envelope.EventTime, TransactionTime: envelope.TransactionTime}
	if len(envelope.Order) == 0 {
		return UserEvent{}, ErrResponse
	}
	var o struct {
		Symbol          string      `json:"s"`
		ClientOrderID   string      `json:"c"`
		Side            string      `json:"S"`
		PositionSide    string      `json:"ps"`
		Type            string      `json:"o"`
		TimeInForce     string      `json:"f"`
		Quantity        string      `json:"q"`
		Price           string      `json:"p"`
		StopPrice       string      `json:"sp"`
		ActivatePrice   string      `json:"ap"`
		WorkingType     string      `json:"wt"`
		ExecutionType   string      `json:"x"`
		OrderStatus     string      `json:"X"`
		OrderID         json.Number `json:"i"`
		LastFilledQty   string      `json:"l"`
		CumulativeQty   string      `json:"z"`
		LastFilledPrice string      `json:"L"`
		Commission      string      `json:"n"`
		CommissionAsset string      `json:"N"`
		TradeID         json.Number `json:"t"`
		ReduceOnly      bool        `json:"R"`
		ClosePosition   bool        `json:"cp"`
		RealizedProfit  string      `json:"rp"`
	}
	if json.Unmarshal(envelope.Order, &o) != nil {
		return UserEvent{}, ErrResponse
	}
	event.OrderUpdate = &OrderUpdate{
		Symbol: o.Symbol, ClientOrderID: o.ClientOrderID, Side: o.Side, PositionSide: o.PositionSide,
		Type: o.Type, TimeInForce: o.TimeInForce, Quantity: o.Quantity, Price: o.Price,
		StopPrice: o.StopPrice, ActivatePrice: o.ActivatePrice, WorkingType: o.WorkingType,
		ExecutionType: o.ExecutionType, OrderStatus: o.OrderStatus, OrderID: o.OrderID.String(),
		LastFilledQty: o.LastFilledQty, CumulativeQty: o.CumulativeQty, LastFilledPrice: o.LastFilledPrice,
		Commission: o.Commission, CommissionAsset: o.CommissionAsset, TradeID: o.TradeID.String(),
		ReduceOnly: o.ReduceOnly, ClosePosition: o.ClosePosition, RealizedProfit: o.RealizedProfit,
	}
	if !validOrderUpdate(event.OrderUpdate) {
		return UserEvent{}, ErrResponse
	}
	return event, nil
}

func parseAccountUpdate(envelope streamEnvelope) (UserEvent, error) {
	event := UserEvent{Type: EventAccountUpdate, EventTime: envelope.EventTime, TransactionTime: envelope.TransactionTime}
	if len(envelope.Account) == 0 {
		return UserEvent{}, ErrResponse
	}
	var a struct {
		Reason   string `json:"m"`
		Balances []struct {
			Asset              string `json:"a"`
			WalletBalance      string `json:"wb"`
			CrossWalletBalance string `json:"cw"`
			BalanceChange      string `json:"bc"`
		} `json:"B"`
		Positions []struct {
			Symbol              string `json:"s"`
			PositionAmount      string `json:"pa"`
			EntryPrice          string `json:"ep"`
			AccumulatedRealized string `json:"cr"`
			UnrealizedPnL       string `json:"up"`
			MarginType          string `json:"mt"`
			IsolatedWallet      string `json:"iw"`
			PositionSide        string `json:"ps"`
		} `json:"P"`
	}
	if json.Unmarshal(envelope.Account, &a) != nil {
		return UserEvent{}, ErrResponse
	}
	update := AccountUpdate{Reason: a.Reason}
	for _, b := range a.Balances {
		update.Balances = append(update.Balances, BalanceUpdate{Asset: b.Asset, WalletBalance: b.WalletBalance, CrossWalletBalance: b.CrossWalletBalance, BalanceChange: b.BalanceChange})
	}
	for _, p := range a.Positions {
		update.Positions = append(update.Positions, PositionUpdate{Symbol: p.Symbol, PositionAmount: p.PositionAmount, EntryPrice: p.EntryPrice, AccumulatedRealized: p.AccumulatedRealized, UnrealizedPnL: p.UnrealizedPnL, MarginType: p.MarginType, IsolatedWallet: p.IsolatedWallet, PositionSide: p.PositionSide})
	}
	event.AccountUpdate = &update
	if !validAccountUpdate(event) {
		return UserEvent{}, ErrResponse
	}
	return event, nil
}

var orderStatusSet = map[string]bool{
	"NEW": true, "PARTIALLY_FILLED": true, "FILLED": true, "CANCELED": true,
	"EXPIRED": true, "REJECTED": true, "PENDING_CANCEL": true,
}
var executionTypeSet = map[string]bool{
	"NEW": true, "CANCELED": true, "EXPIRED": true, "TRADE": true, "AMENDMENT": true,
}

func validOrderUpdate(o *OrderUpdate) bool {
	if o == nil || !instrument.MatchString(o.Symbol) || !validID(o.OrderID) ||
		(o.ClientOrderID != "" && !clientID.MatchString(o.ClientOrderID)) ||
		(o.Side != "BUY" && o.Side != "SELL") ||
		(o.PositionSide != "BOTH" && o.PositionSide != "LONG" && o.PositionSide != "SHORT") ||
		(o.Type != "LIMIT" && o.Type != "MARKET" && !conditional(o.Type)) ||
		(o.WorkingType != "" && o.WorkingType != "MARK_PRICE" && o.WorkingType != "CONTRACT_PRICE") ||
		!orderStatusSet[o.OrderStatus] || !executionTypeSet[o.ExecutionType] ||
		!numeric(o.Quantity, true) || !numeric(o.Price, true) || !numeric(o.StopPrice, true) ||
		!numeric(o.LastFilledQty, true) || !numeric(o.CumulativeQty, true) || !numeric(o.LastFilledPrice, true) ||
		!numeric(o.Commission, true) || !numeric(o.RealizedProfit, false) ||
		(o.ActivatePrice != "" && !numeric(o.ActivatePrice, true)) ||
		(o.TradeID != "0" && !validID(o.TradeID)) {
		return false
	}
	if o.CommissionAsset != "" && (len(o.CommissionAsset) > 20 || strings.ContainsAny(o.CommissionAsset, "\x00")) {
		return false
	}
	return true
}

func validEventTimes(eventTime, transactionTime int64) bool {
	return eventTime > 0 && transactionTime > 0 && eventTime <= 9007199254740991 && transactionTime <= 9007199254740991
}

func validAccountUpdate(event UserEvent) bool {
	a := event.AccountUpdate
	if a == nil || !validEventTimes(event.EventTime, event.TransactionTime) {
		return false
	}
	if a.Reason != "ORDER" && a.Reason != "FUND" {
		return false
	}
	if a.Balances == nil || a.Positions == nil || len(a.Balances) > 1000 || len(a.Positions) > 10000 {
		return false
	}
	seen := map[string]bool{}
	for _, b := range a.Balances {
		if b.Asset == "" || len(b.Asset) > 20 || strings.ContainsAny(b.Asset, "\x00") || seen[b.Asset] ||
			!numeric(b.WalletBalance, false) || !numeric(b.CrossWalletBalance, false) || !numeric(b.BalanceChange, false) {
			return false
		}
		seen[b.Asset] = true
	}
	seen = map[string]bool{}
	for _, p := range a.Positions {
		key := p.Symbol + ":" + p.PositionSide
		if !instrument.MatchString(p.Symbol) || seen[key] ||
			(p.PositionSide != "BOTH" && p.PositionSide != "LONG" && p.PositionSide != "SHORT") ||
			!numeric(p.PositionAmount, false) || !numeric(p.EntryPrice, false) ||
			!numeric(p.AccumulatedRealized, false) || !numeric(p.UnrealizedPnL, false) || !numeric(p.IsolatedWallet, false) ||
			(p.MarginType != "isolated" && p.MarginType != "cross") {
			return false
		}
		seen[key] = true
	}
	return true
}

// Valid re-checks event invariants at the durable boundary (defence in depth).
func (e UserEvent) Valid() bool {
	if e.Type == EventOrderTradeUpdate {
		return e.AccountUpdate == nil && validEventTimes(e.EventTime, e.TransactionTime) && validOrderUpdate(e.OrderUpdate)
	}
	if e.Type == EventAccountUpdate {
		return e.OrderUpdate == nil && validAccountUpdate(e)
	}
	return false
}

// DedupKey is a stable bounded identity so at-least-once redelivery of the same
// exchange event never double-applies. It is not a total order and does not
// reconcile out-of-order or missed events.
func (e UserEvent) DedupKey() string {
	var canonical any
	if e.OrderUpdate != nil {
		canonical = struct {
			Type, Symbol, OrderID, ClientOrderID, ExecutionType, TradeID string
			EventTime, TransactionTime                                   int64
		}{e.Type, e.OrderUpdate.Symbol, e.OrderUpdate.OrderID, e.OrderUpdate.ClientOrderID, e.OrderUpdate.ExecutionType, e.OrderUpdate.TradeID, e.EventTime, e.TransactionTime}
	} else if e.AccountUpdate != nil {
		canonical = struct {
			Type, Reason               string
			EventTime, TransactionTime int64
		}{e.Type, e.AccountUpdate.Reason, e.EventTime, e.TransactionTime}
	} else {
		return ""
	}
	data, err := json.Marshal(canonical)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
