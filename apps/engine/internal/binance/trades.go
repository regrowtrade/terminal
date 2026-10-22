package binance

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"regrow/engine/internal/vault"
)

const maxTradeWindowMs = int64(7 * 24 * 60 * 60 * 1000) // maximum query span, not retention
const maxTradePages = 20
const tradePageSize = 1000

var ErrTradeQuery = errors.New("indica símbolo USDT y una ventana de tiempo válida (máximo siete días)")

// TradeQuery bounds an explicit manual fill recovery window on the millisecond
// exchange clock. startTime is inclusive; endTime is exclusive at the client.
type TradeQuery struct {
	Symbol    string `json:"symbol"`
	StartTime int64  `json:"startTime"`
	EndTime   int64  `json:"endTime"`
}

func (q TradeQuery) Valid() bool {
	return instrument.MatchString(q.Symbol) && q.StartTime > 0 && q.EndTime > q.StartTime &&
		q.EndTime-q.StartTime <= maxTradeWindowMs && q.EndTime <= 9007199254740991
}

// FillRecovery reports a manual REST fill recovery against the durable dedup
// table. It is point-in-time evidence, never continuity or execution readiness.
type FillRecovery struct {
	Source         string `json:"source"`
	Mode           string `json:"mode"`
	ExecutionReady bool   `json:"executionReady"`
	Symbol         string `json:"symbol"`
	StartTime      int64  `json:"startTime"`
	EndTime        int64  `json:"endTime"`
	Recovered      int    `json:"recovered"`
	Duplicates     int    `json:"duplicates"`
	Truncated      bool   `json:"truncated"`
}

// tradeRow is a raw GET /fapi/v1/userTrades element. IDs remain json.Number so
// int64 trade/order IDs never pass through a float.
type tradeRow struct {
	ID              json.Number `json:"id"`
	OrderID         json.Number `json:"orderId"`
	Symbol          string      `json:"symbol"`
	Side            string      `json:"side"`
	PositionSide    string      `json:"positionSide"`
	Price           string      `json:"price"`
	Quantity        string      `json:"qty"`
	Commission      string      `json:"commission"`
	CommissionAsset string      `json:"commissionAsset"`
	Time            int64       `json:"time"`
}

func (t tradeRow) fill() (Fill, error) {
	orderID, tradeID := t.OrderID.String(), t.ID.String()
	if !validID(orderID) || !validID(tradeID) || !instrument.MatchString(t.Symbol) ||
		(t.Side != "BUY" && t.Side != "SELL") ||
		(t.PositionSide != "BOTH" && t.PositionSide != "LONG" && t.PositionSide != "SHORT") ||
		!positive(t.Price) || !positive(t.Quantity) || !numeric(t.Commission, false) ||
		t.CommissionAsset == "" || len(t.CommissionAsset) > 20 || strings.ContainsRune(t.CommissionAsset, '\x00') ||
		t.Time <= 0 || t.Time > 9007199254740991 {
		return Fill{}, ErrResponse
	}
	// REST trades carry a single trade timestamp; use it for both the event and
	// transaction clocks so recovered fills layer on the same snapshot cutoff.
	return Fill{OrderID: orderID, TradeID: tradeID, Symbol: t.Symbol, Side: t.Side,
		PositionSide: t.PositionSide, Price: t.Price, Quantity: t.Quantity,
		Commission: t.Commission, CommissionAsset: t.CommissionAsset,
		EventTime: t.Time, TransactionTime: t.Time}, nil
}

// ValidateRecoveredFill repeats REST normalization checks at service/storage
// boundaries. A REST trade has a single clock; it cannot invent an event time.
func ValidateRecoveredFill(f Fill) error {
	row := tradeRow{ID: json.Number(f.TradeID), OrderID: json.Number(f.OrderID),
		Symbol: f.Symbol, Side: f.Side, PositionSide: f.PositionSide, Price: f.Price,
		Quantity: f.Quantity, Commission: f.Commission, CommissionAsset: f.CommissionAsset,
		Time: f.TransactionTime}
	if _, err := row.fill(); err != nil || f.EventTime != f.TransactionTime {
		return ErrResponse
	}
	return nil
}

// RecoverTrades reads the bounded window from the userTrades endpoint and
// returns validated, key-deduplicated fills in ascending time order. It never
// submits, cancels or infers continuity; pagination is bounded and truncation
// is reported explicitly.
func (c *Client) RecoverTrades(ctx context.Context, credentials vault.Credentials, query TradeQuery) ([]Fill, bool, error) {
	if !query.Valid() {
		return nil, false, ErrTradeQuery
	}
	if err := c.begin(); err != nil {
		return nil, false, err
	}
	defer c.end()
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	now, err := c.checked(ctx, credentials)
	if err != nil {
		return nil, false, err
	}
	fills := []Fill{}
	seen := map[string]Fill{}
	truncated := false
	// Binance forbids fromId together with startTime/endTime. Split saturated
	// time windows instead, without assuming newest/oldest-first response order.
	// Subwindows are disjoint [start,end); endTime on the wire is inclusive.
	pending := []TradeQuery{query}
	for page := 0; page < maxTradePages && len(pending) > 0; page++ {
		window := pending[len(pending)-1]
		pending = pending[:len(pending)-1]
		params := url.Values{
			"symbol":    {query.Symbol},
			"startTime": {strconv.FormatInt(window.StartTime, 10)},
			"endTime":   {strconv.FormatInt(window.EndTime-1, 10)},
			"limit":     {strconv.Itoa(tradePageSize)},
		}
		var rows []tradeRow
		if err := c.getWithParams(ctx, "/fapi/v1/userTrades", &credentials, now(), params, &rows); err != nil {
			return nil, false, err
		}
		if rows == nil || len(rows) > tradePageSize {
			return nil, false, ErrResponse
		}
		for _, row := range rows {
			if row.Symbol != query.Symbol || row.Time < window.StartTime || row.Time >= window.EndTime {
				return nil, false, ErrResponse
			}
			fill, err := row.fill()
			if err != nil {
				return nil, false, err
			}
			key := fill.OrderID + ":" + fill.TradeID
			if previous, ok := seen[key]; ok {
				if previous != fill {
					return nil, false, ErrResponse
				}
				continue
			}
			seen[key] = fill
			fills = append(fills, fill)
		}
		if len(rows) == tradePageSize {
			if window.EndTime-window.StartTime == 1 {
				// A saturated millisecond cannot be split safely. Do not skip it
				// or present the page as a complete recovery.
				truncated = true
				continue
			}
			mid := window.StartTime + (window.EndTime-window.StartTime)/2
			pending = append(pending,
				TradeQuery{Symbol: query.Symbol, StartTime: mid, EndTime: window.EndTime},
				TradeQuery{Symbol: query.Symbol, StartTime: window.StartTime, EndTime: mid})
		}
	}
	truncated = truncated || len(pending) > 0
	sort.SliceStable(fills, func(i, j int) bool {
		if fills[i].EventTime != fills[j].EventTime {
			return fills[i].EventTime < fills[j].EventTime
		}
		if fills[i].OrderID != fills[j].OrderID {
			return len(fills[i].OrderID) < len(fills[j].OrderID) || (len(fills[i].OrderID) == len(fills[j].OrderID) && fills[i].OrderID < fills[j].OrderID)
		}
		return len(fills[i].TradeID) < len(fills[j].TradeID) || (len(fills[i].TradeID) == len(fills[j].TradeID) && fills[i].TradeID < fills[j].TradeID)
	})
	return fills, truncated, nil
}

// validTradeParams enforces the fixed allowlist for a signed userTrades request
// at the transport boundary, before any signature is computed.
func validTradeParams(params url.Values) bool {
	if len(params) != 4 {
		return false
	}
	for _, values := range params {
		if len(values) != 1 {
			return false
		}
	}
	for key := range params {
		switch key {
		case "symbol", "startTime", "endTime", "limit":
		default:
			return false
		}
	}
	start, err1 := strconv.ParseInt(params.Get("startTime"), 10, 64)
	end, err2 := strconv.ParseInt(params.Get("endTime"), 10, 64)
	limit, err3 := strconv.Atoi(params.Get("limit"))
	if err1 != nil || err2 != nil || err3 != nil || !instrument.MatchString(params.Get("symbol")) {
		return false
	}
	if start <= 0 || end < start || end-start >= maxTradeWindowMs || end > 9007199254740991 || limit < 1 || limit > tradePageSize {
		return false
	}
	return true
}
