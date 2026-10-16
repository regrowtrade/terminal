package binance

import (
	"context"
	"encoding/json"
	"regexp"
	"strconv"

	"github.com/shopspring/decimal"
	"regrow/engine/internal/vault"
)

// IDs are strings at our boundary: exchange int64 IDs exceed JavaScript's
// exact integer range. Quantities and prices also remain unmodified strings.
type Order struct {
	ID            string `json:"orderId"`
	ClientID      string `json:"clientOrderId"`
	Symbol        string `json:"symbol"`
	Side          string `json:"side"`
	PositionSide  string `json:"positionSide"`
	Type          string `json:"type"`
	Status        string `json:"status"`
	Quantity      string `json:"origQty"`
	Executed      string `json:"executedQty"`
	Price         string `json:"price"`
	StopPrice     string `json:"stopPrice"`
	ReduceOnly    bool   `json:"reduceOnly"`
	ClosePosition bool   `json:"closePosition"`
	WorkingType   string `json:"workingType"`
	ActivatePrice string `json:"activatePrice,omitempty"`
	PriceRate     string `json:"priceRate,omitempty"`
	UpdateTime    int64  `json:"updateTime"`
}

type AlgoOrder struct {
	ID            string `json:"algoId"`
	ClientID      string `json:"clientAlgoId"`
	AlgoType      string `json:"algoType"`
	Symbol        string `json:"symbol"`
	Side          string `json:"side"`
	PositionSide  string `json:"positionSide"`
	Type          string `json:"orderType"`
	Status        string `json:"algoStatus"`
	Quantity      string `json:"quantity"`
	Price         string `json:"price"`
	TriggerPrice  string `json:"triggerPrice"`
	ReduceOnly    bool   `json:"reduceOnly"`
	ClosePosition bool   `json:"closePosition"`
	WorkingType   string `json:"workingType"`
	ActivatePrice string `json:"activatePrice,omitempty"`
	CallbackRate  string `json:"callbackRate,omitempty"`
	UpdateTime    int64  `json:"updateTime"`
}

type orderResponse struct {
	Order
	ID            json.Number `json:"orderId"`
	ReduceOnly    *bool       `json:"reduceOnly"`
	ClosePosition *bool       `json:"closePosition"`
}
type algoResponse struct {
	AlgoOrder
	ID            json.Number `json:"algoId"`
	ReduceOnly    *bool       `json:"reduceOnly"`
	ClosePosition *bool       `json:"closePosition"`
}

var clientID = regexp.MustCompile(`^[.A-Z:/a-z0-9_-]{1,64}$`)
var positiveID = regexp.MustCompile(`^[1-9][0-9]{0,18}$`)

func validID(id string) bool {
	_, err := strconv.ParseInt(id, 10, 64)
	return positiveID.MatchString(id) && err == nil
}
func orderIdentity(symbol, side, positionSide string, updated int64) bool {
	return instrument.MatchString(symbol) && (side == "BUY" || side == "SELL") &&
		(positionSide == "BOTH" || positionSide == "LONG" || positionSide == "SHORT") && updated > 0 && updated <= 9007199254740991
}
func conditional(kind string) bool {
	return kind == "STOP" || kind == "STOP_MARKET" || kind == "TAKE_PROFIT" || kind == "TAKE_PROFIT_MARKET" || kind == "TRAILING_STOP_MARKET"
}
func positive(value string) bool {
	if !numeric(value, true) {
		return false
	}
	d, _ := decimal.NewFromString(value)
	return d.IsPositive()
}
func conditionalFields(kind, trigger, activate, rate, working string) bool {
	if working != "MARK_PRICE" && working != "CONTRACT_PRICE" {
		return false
	}
	if kind == "TRAILING_STOP_MARKET" {
		if !positive(activate) || !positive(rate) {
			return false
		}
		d, _ := decimal.NewFromString(rate)
		return d.GreaterThanOrEqual(decimal.RequireFromString("0.1")) && d.LessThanOrEqual(decimal.NewFromInt(10))
	}
	return positive(trigger) && (activate == "" || numeric(activate, true)) && (rate == "" || numeric(rate, true))
}
func (c *Client) orders(ctx context.Context, credentials vault.Credentials, now func() int64) ([]Order, []AlgoOrder, error) {
	var raw []orderResponse
	if err := c.get(ctx, "/fapi/v1/openOrders", &credentials, now(), &raw); err != nil {
		return nil, nil, err
	}
	if raw == nil || len(raw) > 10000 {
		return nil, nil, ErrResponse
	}
	orders := make([]Order, 0, len(raw))
	seen := map[string]bool{}
	for _, r := range raw {
		o := r.Order
		o.ID = r.ID.String()
		key := o.Symbol + ":" + o.ID
		if !validID(o.ID) || !clientID.MatchString(o.ClientID) || seen[key] || r.ReduceOnly == nil || r.ClosePosition == nil ||
			!orderIdentity(o.Symbol, o.Side, o.PositionSide, o.UpdateTime) ||
			(o.Status != "NEW" && o.Status != "PARTIALLY_FILLED") ||
			(o.Type != "LIMIT" && o.Type != "MARKET" && !conditional(o.Type)) ||
			(o.WorkingType != "" && o.WorkingType != "MARK_PRICE" && o.WorkingType != "CONTRACT_PRICE") ||
			(o.ActivatePrice != "" && !numeric(o.ActivatePrice, true)) || (o.PriceRate != "" && !numeric(o.PriceRate, true)) ||
			!numeric(o.Quantity, true) || !numeric(o.Executed, true) || !numeric(o.Price, true) || !numeric(o.StopPrice, true) {
			return nil, nil, ErrResponse
		}
		o.ReduceOnly, o.ClosePosition = *r.ReduceOnly, *r.ClosePosition
		qty, _ := decimal.NewFromString(o.Quantity)
		executed, _ := decimal.NewFromString(o.Executed)
		if executed.GreaterThan(qty) || (o.Status == "NEW" && !executed.IsZero()) ||
			(o.Status == "PARTIALLY_FILLED" && (!executed.IsPositive() || !executed.LessThan(qty))) ||
			(!o.ClosePosition && !qty.IsPositive()) ||
			(o.Type == "LIMIT" && !positive(o.Price)) ||
			((o.Type == "STOP" || o.Type == "TAKE_PROFIT") && !positive(o.Price)) ||
			(conditional(o.Type) && !conditionalFields(o.Type, o.StopPrice, o.ActivatePrice, o.PriceRate, o.WorkingType)) ||
			(o.ClosePosition && o.Type != "STOP_MARKET" && o.Type != "TAKE_PROFIT_MARKET") {
			return nil, nil, ErrResponse
		}
		seen[key] = true
		orders = append(orders, o)
	}
	var rawAlgos []algoResponse
	if err := c.get(ctx, "/fapi/v1/openAlgoOrders", &credentials, now(), &rawAlgos); err != nil {
		return nil, nil, err
	}
	if rawAlgos == nil || len(rawAlgos) > 10000 {
		return nil, nil, ErrResponse
	}
	algos := make([]AlgoOrder, 0, len(rawAlgos))
	seen = map[string]bool{}
	for _, r := range rawAlgos {
		o := r.AlgoOrder
		o.ID = r.ID.String()
		key := o.Symbol + ":" + o.ID
		if !validID(o.ID) || !clientID.MatchString(o.ClientID) || seen[key] || r.ReduceOnly == nil || r.ClosePosition == nil ||
			!orderIdentity(o.Symbol, o.Side, o.PositionSide, o.UpdateTime) || o.AlgoType != "CONDITIONAL" || o.Status != "NEW" ||
			!conditional(o.Type) || !numeric(o.Quantity, true) || !numeric(o.Price, true) || !numeric(o.TriggerPrice, true) ||
			!conditionalFields(o.Type, o.TriggerPrice, o.ActivatePrice, o.CallbackRate, o.WorkingType) {
			return nil, nil, ErrResponse
		}
		o.ReduceOnly, o.ClosePosition = *r.ReduceOnly, *r.ClosePosition
		if (!o.ClosePosition && !positive(o.Quantity)) ||
			((o.Type == "STOP" || o.Type == "TAKE_PROFIT") && !positive(o.Price)) ||
			(o.ClosePosition && o.Type != "STOP_MARKET" && o.Type != "TAKE_PROFIT_MARKET") {
			return nil, nil, ErrResponse
		}
		seen[key] = true
		algos = append(algos, o)
	}
	return orders, algos, nil
}
