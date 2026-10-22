package binance

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"time"

	"github.com/shopspring/decimal"
	"regrow/engine/internal/vault"
)

var ErrAlgoQuery = errors.New("indica símbolo USDT y exactamente un algoId o clientAlgoId válido")

type AlgoQuery struct {
	Symbol       string `json:"symbol"`
	AlgoID       string `json:"algoId,omitempty"`
	ClientAlgoID string `json:"clientAlgoId,omitempty"`
}

func (q AlgoQuery) Valid() bool {
	return instrument.MatchString(q.Symbol) && validAlgoSelector(q.AlgoID, q.ClientAlgoID)
}

func validAlgoSelector(id, client string) bool {
	return (id != "" && client == "" && validID(id)) || (client != "" && id == "" && normalClientID.MatchString(client))
}

// The child order reference is exchange evidence only. It is never followed
// automatically, interpreted as a fill, or linked to a strategy intention.
type QueriedAlgoOrder struct {
	AlgoOrder
	ActualOrderID  string `json:"actualOrderId"`
	ActualPrice    string `json:"actualPrice"`
	ActualType     string `json:"actualType,omitempty"`
	ActualQuantity string `json:"actualQty,omitempty"`
	TriggerTime    int64  `json:"triggerTime"`
}

type AlgoLookup struct {
	Source         string            `json:"source"`
	Mode           string            `json:"mode"`
	Family         string            `json:"family"`
	ExecutionReady bool              `json:"executionReady"`
	StartedAt      time.Time         `json:"startedAt"`
	CheckedAt      time.Time         `json:"checkedAt"`
	Query          AlgoQuery         `json:"query"`
	Outcome        string            `json:"outcome"`
	Reason         string            `json:"reason"`
	Order          *QueriedAlgoOrder `json:"order"`
}

func validQueriedAlgo(o QueriedAlgoOrder, q AlgoQuery) bool {
	base := o.AlgoOrder
	// Open-algo validation contains the same conditional attribute invariants;
	// statuses in this point-in-time query are checked separately below.
	base.Status = "NEW"
	if !validAlgoOrder(base) || o.Symbol != q.Symbol ||
		(q.AlgoID != "" && o.ID != q.AlgoID) || (q.ClientAlgoID != "" && o.ClientID != q.ClientAlgoID) ||
		!normalClientID.MatchString(o.ClientID) || !numeric(o.ActualPrice, true) ||
		o.TriggerTime < 0 || o.TriggerTime > 9007199254740991 ||
		(o.ActualOrderID != "" && (!validID(o.ActualOrderID) || o.TriggerTime == 0)) ||
		(o.ActualType != "" && o.ActualType != "LIMIT" && o.ActualType != "MARKET") ||
		(o.ActualQuantity != "" && !numeric(o.ActualQuantity, true)) {
		return false
	}
	switch o.Status {
	case "NEW", "CANCELED", "TRIGGERED", "FINISHED", "REJECTED", "EXPIRED":
	default:
		return false
	}
	if o.ActualOrderID == "" && (positive(o.ActualPrice) || positive(o.ActualQuantity) || o.ActualType != "") {
		return false
	}
	if o.Status == "NEW" && (o.ActualOrderID != "" || o.TriggerTime != 0) {
		return false
	}
	if o.ActualQuantity != "" {
		qty, _ := decimal.NewFromString(o.ActualQuantity)
		limit, _ := decimal.NewFromString(o.Quantity)
		if (qty.IsPositive() && !positive(o.ActualPrice)) || (!o.ClosePosition && qty.GreaterThan(limit)) {
			return false
		}
	}
	return true
}

func ValidateAlgoLookup(r AlgoLookup) error {
	if r.Source != "binance-production" || r.Mode != "read_only" || r.Family != "algo" || r.ExecutionReady || !r.Query.Valid() || r.StartedAt.IsZero() || r.CheckedAt.Before(r.StartedAt) || r.CheckedAt.Sub(r.StartedAt) > 18*time.Second {
		return ErrResponse
	}
	if r.Outcome == "unresolved" && r.Reason == "exchange_not_found" && r.Order == nil {
		return nil
	}
	if r.Outcome == "observed" && r.Reason == "" && r.Order != nil && validQueriedAlgo(*r.Order, r.Query) {
		return nil
	}
	return ErrResponse
}

func (c *Client) LookupAlgoOrder(ctx context.Context, credentials vault.Credentials, query AlgoQuery) (AlgoLookup, error) {
	if !query.Valid() {
		return AlgoLookup{}, ErrAlgoQuery
	}
	if err := c.begin(); err != nil {
		return AlgoLookup{}, err
	}
	defer c.end()
	started := time.Now().UTC()
	ctx, cancel := context.WithTimeout(ctx, 18*time.Second)
	defer cancel()
	now, err := c.checked(ctx, credentials)
	if err != nil {
		return AlgoLookup{}, err
	}
	params := url.Values{}
	if query.AlgoID != "" {
		params.Set("algoId", query.AlgoID)
	} else {
		params.Set("clientAlgoId", query.ClientAlgoID)
	}
	// Unlike /order, /algoOrder accepts no symbol. The caller's expected USDT
	// symbol is checked against the response, never injected into that protocol.
	var raw struct {
		QueriedAlgoOrder
		ID       json.Number `json:"algoId"`
		Reduce   *bool       `json:"reduceOnly"`
		Close    *bool       `json:"closePosition"`
		ActualID *string     `json:"actualOrderId"`
		Trigger  *int64      `json:"triggerTime"`
	}
	err = c.getWithParams(ctx, "/fapi/v1/algoOrder", &credentials, now(), params, &raw)
	r := AlgoLookup{Source: "binance-production", Mode: "read_only", Family: "algo", StartedAt: started, CheckedAt: time.Now().UTC(), Query: query}
	if errors.Is(err, errOrderNotFound) {
		r.Outcome, r.Reason = "unresolved", "exchange_not_found"
	} else {
		if err != nil {
			return AlgoLookup{}, err
		}
		if raw.Reduce == nil || raw.Close == nil || raw.ActualID == nil || raw.Trigger == nil {
			return AlgoLookup{}, ErrResponse
		}
		o := raw.QueriedAlgoOrder
		o.ID, o.ReduceOnly, o.ClosePosition = raw.ID.String(), *raw.Reduce, *raw.Close
		o.ActualOrderID, o.TriggerTime = *raw.ActualID, *raw.Trigger
		r.Outcome, r.Order = "observed", &o
	}
	if err := ValidateAlgoLookup(r); err != nil {
		return AlgoLookup{}, err
	}
	return r, nil
}
