package binance

import (
	"context"
	"encoding/json"
	"regexp"
	"sort"

	"github.com/shopspring/decimal"
)

type RangeFilter struct {
	Min  string `json:"min"`
	Max  string `json:"max"`
	Step string `json:"step"`
}
type Instrument struct {
	Symbol         string      `json:"symbol"`
	ContractType   string      `json:"contractType"`
	Status         string      `json:"status"`
	BaseAsset      string      `json:"baseAsset"`
	QuoteAsset     string      `json:"quoteAsset"`
	MarginAsset    string      `json:"marginAsset"`
	Price          RangeFilter `json:"priceFilter"`
	Lot            RangeFilter `json:"lotSize"`
	MarketLot      RangeFilter `json:"marketLotSize"`
	MinNotional    string      `json:"minNotional"`
	MultiplierUp   string      `json:"multiplierUp"`
	MultiplierDown string      `json:"multiplierDown"`
	MaxOrders      int64       `json:"maxNumOrders"`
	MaxAlgos       int64       `json:"maxNumAlgoOrders"`
	UnknownFilters []string    `json:"unrecognizedFilterTypes"`
}
type instrumentResponse struct {
	Symbol       string            `json:"symbol"`
	ContractType string            `json:"contractType"`
	Status       string            `json:"status"`
	BaseAsset    string            `json:"baseAsset"`
	QuoteAsset   string            `json:"quoteAsset"`
	MarginAsset  string            `json:"marginAsset"`
	Filters      []json.RawMessage `json:"filters"`
}

var exchangeToken = regexp.MustCompile(`^[A-Z0-9_]{1,40}$`)

func validRange(r RangeFilter) bool {
	if !numeric(r.Min, true) || !numeric(r.Max, true) || !numeric(r.Step, true) {
		return false
	}
	min, _ := decimal.NewFromString(r.Min)
	max, _ := decimal.NewFromString(r.Max)
	// A zero filter component can mean disabled; do not substitute precision.
	return max.IsZero() || max.GreaterThanOrEqual(min)
}
func parseInstrument(raw instrumentResponse) (Instrument, error) {
	i := Instrument{Symbol: raw.Symbol, ContractType: raw.ContractType, Status: raw.Status, BaseAsset: raw.BaseAsset, QuoteAsset: raw.QuoteAsset, MarginAsset: raw.MarginAsset, UnknownFilters: []string{}}
	if !instrument.MatchString(i.Symbol) || i.ContractType != "PERPETUAL" || !exchangeToken.MatchString(i.Status) ||
		!exchangeToken.MatchString(i.BaseAsset) || i.QuoteAsset != "USDT" || i.MarginAsset != "USDT" || raw.Filters == nil || len(raw.Filters) > 64 {
		return Instrument{}, ErrResponse
	}
	seen := map[string]bool{}
	for _, data := range raw.Filters {
		var f struct {
			Type     string `json:"filterType"`
			MinPrice string `json:"minPrice"`
			MaxPrice string `json:"maxPrice"`
			Tick     string `json:"tickSize"`
			MinQty   string `json:"minQty"`
			MaxQty   string `json:"maxQty"`
			Step     string `json:"stepSize"`
			Notional string `json:"notional"`
			Up       string `json:"multiplierUp"`
			Down     string `json:"multiplierDown"`
			Limit    *int64 `json:"limit"`
		}
		if json.Unmarshal(data, &f) != nil || !exchangeToken.MatchString(f.Type) || seen[f.Type] {
			return Instrument{}, ErrResponse
		}
		seen[f.Type] = true
		switch f.Type {
		case "PRICE_FILTER":
			i.Price = RangeFilter{Min: f.MinPrice, Max: f.MaxPrice, Step: f.Tick}
		case "LOT_SIZE":
			i.Lot = RangeFilter{Min: f.MinQty, Max: f.MaxQty, Step: f.Step}
		case "MARKET_LOT_SIZE":
			i.MarketLot = RangeFilter{Min: f.MinQty, Max: f.MaxQty, Step: f.Step}
		case "MIN_NOTIONAL":
			i.MinNotional = f.Notional
		case "PERCENT_PRICE":
			i.MultiplierUp, i.MultiplierDown = f.Up, f.Down
		case "MAX_NUM_ORDERS", "MAX_NUM_ALGO_ORDERS":
			if f.Limit == nil || *f.Limit < 0 || *f.Limit > 9007199254740991 {
				return Instrument{}, ErrResponse
			}
			if f.Type == "MAX_NUM_ORDERS" {
				i.MaxOrders = *f.Limit
			} else {
				i.MaxAlgos = *f.Limit
			}
		default:
			// Report unknown filters explicitly; this is not execution validation.
			i.UnknownFilters = append(i.UnknownFilters, f.Type)
		}
	}
	for _, name := range []string{"PRICE_FILTER", "LOT_SIZE", "MARKET_LOT_SIZE", "MIN_NOTIONAL", "PERCENT_PRICE", "MAX_NUM_ORDERS", "MAX_NUM_ALGO_ORDERS"} {
		if !seen[name] {
			return Instrument{}, ErrResponse
		}
	}
	if !validRange(i.Price) || !validRange(i.Lot) || !validRange(i.MarketLot) || !numeric(i.MinNotional, true) || !positive(i.MultiplierUp) || !positive(i.MultiplierDown) {
		return Instrument{}, ErrResponse
	}
	up, _ := decimal.NewFromString(i.MultiplierUp)
	down, _ := decimal.NewFromString(i.MultiplierDown)
	if up.LessThan(down) {
		return Instrument{}, ErrResponse
	}
	sort.Strings(i.UnknownFilters)
	return i, nil
}
func (c *Client) instruments(ctx context.Context) ([]Instrument, error) {
	var response struct {
		Symbols []instrumentResponse `json:"symbols"`
	}
	if err := c.get(ctx, "/fapi/v1/exchangeInfo", nil, 0, &response); err != nil {
		return nil, err
	}
	if response.Symbols == nil || len(response.Symbols) > 10000 {
		return nil, ErrResponse
	}
	result := []Instrument{}
	seen := map[string]bool{}
	for _, raw := range response.Symbols {
		// The public catalogue includes other quotes and dated contracts. They
		// are outside this adapter's scope; private exposure is never filtered.
		if raw.QuoteAsset != "USDT" || raw.ContractType != "PERPETUAL" {
			continue
		}
		i, err := parseInstrument(raw)
		if err != nil || seen[i.Symbol] {
			return nil, ErrResponse
		}
		seen[i.Symbol] = true
		result = append(result, i)
	}
	if len(result) == 0 {
		return nil, ErrResponse
	}
	sort.Slice(result, func(a, b int) bool { return result[a].Symbol < result[b].Symbol })
	return result, nil
}
