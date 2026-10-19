package binance

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"regexp"
	"time"

	"github.com/shopspring/decimal"
	"regrow/engine/internal/vault"
)

var ErrOrderQuery = errors.New("indica símbolo USDT y exactamente un orderId o clientOrderId válido")
var errOrderNotFound = errors.New("order query unresolved")
var normalClientID = regexp.MustCompile(`^[.A-Z:/a-z0-9_-]{1,36}$`)

type OrderQuery struct {
	Symbol        string `json:"symbol"`
	OrderID       string `json:"orderId,omitempty"`
	ClientOrderID string `json:"clientOrderId,omitempty"`
}

func (q OrderQuery) Valid() bool {
	return instrument.MatchString(q.Symbol) && ((q.OrderID != "" && q.ClientOrderID == "" && validID(q.OrderID)) || (q.ClientOrderID != "" && q.OrderID == "" && normalClientID.MatchString(q.ClientOrderID)))
}

// QueriedOrder describes a single normal LIMIT/MARKET order, not its individual
// fills, position closure or strategy ownership. Conditional/algo lookup is separate.
type QueriedOrder struct {
	OrderID       string `json:"orderId"`
	ClientOrderID string `json:"clientOrderId"`
	Symbol        string `json:"symbol"`
	Side          string `json:"side"`
	PositionSide  string `json:"positionSide"`
	Type          string `json:"type"`
	Status        string `json:"status"`
	Quantity      string `json:"origQty"`
	Executed      string `json:"executedQty"`
	Price         string `json:"price"`
	AveragePrice  string `json:"avgPrice"`
	ReduceOnly    bool   `json:"reduceOnly"`
	ClosePosition bool   `json:"closePosition"`
	UpdateTime    int64  `json:"updateTime"`
}

type OrderLookup struct {
	Source         string        `json:"source"`
	Mode           string        `json:"mode"`
	ExecutionReady bool          `json:"executionReady"`
	StartedAt      time.Time     `json:"startedAt"`
	CheckedAt      time.Time     `json:"checkedAt"`
	Query          OrderQuery    `json:"query"`
	Outcome        string        `json:"outcome"`
	Reason         string        `json:"reason"`
	Order          *QueriedOrder `json:"order"`
}

func validQueriedOrder(o QueriedOrder, q OrderQuery) bool {
	if !validID(o.OrderID) || !normalClientID.MatchString(o.ClientOrderID) || o.Symbol != q.Symbol ||
		(q.OrderID != "" && o.OrderID != q.OrderID) || (q.ClientOrderID != "" && o.ClientOrderID != q.ClientOrderID) ||
		!orderIdentity(o.Symbol, o.Side, o.PositionSide, o.UpdateTime) || (o.Type != "LIMIT" && o.Type != "MARKET") || o.ClosePosition ||
		!positive(o.Quantity) || !numeric(o.Executed, true) || !numeric(o.Price, true) || !numeric(o.AveragePrice, true) ||
		(o.Type == "LIMIT" && !positive(o.Price)) {
		return false
	}
	qty, _ := decimal.NewFromString(o.Quantity)
	executed, _ := decimal.NewFromString(o.Executed)
	if executed.GreaterThan(qty) || (executed.IsPositive() && !positive(o.AveragePrice)) || (executed.IsZero() && positive(o.AveragePrice)) {
		return false
	}
	switch o.Status {
	case "NEW", "REJECTED":
		return executed.IsZero()
	case "PARTIALLY_FILLED":
		return executed.IsPositive() && executed.LessThan(qty)
	case "FILLED":
		return executed.Equal(qty)
	case "CANCELED", "EXPIRED", "EXPIRED_IN_MATCH", "PENDING_CANCEL":
		return true
	default:
		return false
	}
}

func ValidateOrderLookup(r OrderLookup) error {
	if r.Source != "binance-production" || r.Mode != "read_only" || r.ExecutionReady || !r.Query.Valid() || r.StartedAt.IsZero() || r.CheckedAt.Before(r.StartedAt) || r.CheckedAt.Sub(r.StartedAt) > 18*time.Second {
		return ErrResponse
	}
	if r.Outcome == "unresolved" && r.Reason == "exchange_not_found" && r.Order == nil {
		return nil
	}
	if r.Outcome == "observed" && r.Reason == "" && r.Order != nil && validQueriedOrder(*r.Order, r.Query) {
		return nil
	}
	return ErrResponse
}

// LookupOrder never retries, submits or cancels. -2013 remains unresolved because
// exchange retention, the wrong identifier or a lost response can hide history.
func (c *Client) LookupOrder(ctx context.Context, credentials vault.Credentials, query OrderQuery) (OrderLookup, error) {
	if !query.Valid() {
		return OrderLookup{}, ErrOrderQuery
	}
	if err := c.begin(); err != nil {
		return OrderLookup{}, err
	}
	defer c.end()
	started := time.Now().UTC()
	ctx, cancel := context.WithTimeout(ctx, 18*time.Second)
	defer cancel()
	now, err := c.checked(ctx, credentials)
	if err != nil {
		return OrderLookup{}, err
	}
	params := url.Values{"symbol": {query.Symbol}}
	if query.OrderID != "" {
		params.Set("orderId", query.OrderID)
	} else {
		params.Set("origClientOrderId", query.ClientOrderID)
	}
	var raw struct {
		QueriedOrder
		ID     json.Number `json:"orderId"`
		Reduce *bool       `json:"reduceOnly"`
		Close  *bool       `json:"closePosition"`
	}
	err = c.getWithParams(ctx, "/fapi/v1/order", &credentials, now(), params, &raw)
	r := OrderLookup{Source: "binance-production", Mode: "read_only", StartedAt: started, CheckedAt: time.Now().UTC(), Query: query}
	if errors.Is(err, errOrderNotFound) {
		r.Outcome, r.Reason = "unresolved", "exchange_not_found"
	} else {
		if err != nil {
			return OrderLookup{}, err
		}
		if raw.Reduce == nil || raw.Close == nil {
			return OrderLookup{}, ErrResponse
		}
		o := raw.QueriedOrder
		o.OrderID, o.ReduceOnly, o.ClosePosition = raw.ID.String(), *raw.Reduce, *raw.Close
		r.Outcome, r.Order = "observed", &o
	}
	if err := ValidateOrderLookup(r); err != nil {
		return OrderLookup{}, err
	}
	return r, nil
}
