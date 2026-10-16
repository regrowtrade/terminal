package binance

import (
	"github.com/shopspring/decimal"
	"regexp"
	"strings"
	"time"
)

var decimalText = regexp.MustCompile(`^-?(?:0|[1-9][0-9]{0,37})(?:\.[0-9]{1,18})?$`)
var instrument = regexp.MustCompile(`^[A-Z0-9]{2,24}USDT$`)

func numeric(value string, nonnegative bool) bool {
	if !decimalText.MatchString(value) {
		return false
	}
	d, err := decimal.NewFromString(value)
	return err == nil && (!nonnegative || (!d.IsNegative() && !strings.HasPrefix(value, "-")))
}

// ValidateSnapshot applies the same normalized-data rules to fresh and durable
// observations before comparison. It does not validate execution or protection.
func ValidateSnapshot(s Snapshot) error {
	if s.Source != "binance-production" || s.Mode != "read_only" || s.Consistency != "rest_non_atomic" || s.ExecutionReady ||
		s.StartedAt.IsZero() || s.CapturedAt.Before(s.StartedAt) || s.CapturedAt.Sub(s.StartedAt) > 30*time.Second ||
		s.OpenOrders == nil || len(s.OpenOrders) > 10000 || s.AlgoOrders == nil || len(s.AlgoOrders) > 10000 ||
		len(s.Instruments) == 0 || len(s.Instruments) > 10000 {
		return ErrResponse
	}
	if err := validate(s.Assets, s.Positions); err != nil {
		return err
	}
	known := map[string]bool{}
	for _, i := range s.Instruments {
		if !validInstrument(i) || known[i.Symbol] {
			return ErrResponse
		}
		known[i.Symbol] = true
	}
	for _, p := range s.Positions {
		if !known[p.Symbol] {
			return ErrResponse
		}
	}
	seen := map[string]bool{}
	for _, o := range s.OpenOrders {
		key := o.Symbol + ":" + o.ID
		if !validOrder(o) || !known[o.Symbol] || seen[key] {
			return ErrResponse
		}
		seen[key] = true
	}
	seen = map[string]bool{}
	for _, o := range s.AlgoOrders {
		key := o.Symbol + ":" + o.ID
		if !validAlgoOrder(o) || !known[o.Symbol] || seen[key] {
			return ErrResponse
		}
		seen[key] = true
	}
	return nil
}
func validate(assets []Asset, positions []Position) error {
	if assets == nil || positions == nil || len(assets) > 1000 || len(positions) > 10000 {
		return ErrResponse
	}
	seen := map[string]bool{}
	for _, a := range assets {
		if a.Asset == "" || len(a.Asset) > 20 || seen[a.Asset] || !numeric(a.WalletBalance, false) || !numeric(a.AvailableBalance, false) || !numeric(a.UnrealizedProfit, false) {
			return ErrResponse
		}
		seen[a.Asset] = true
	}
	seen = map[string]bool{}
	for _, p := range positions {
		key := p.Symbol + ":" + p.Side
		// This first adapter supports USDT contracts only; fail rather than
		// silently omit an unsupported exposure from a supposedly complete view.
		if !instrument.MatchString(p.Symbol) || !strings.HasSuffix(p.Symbol, "USDT") || seen[key] || (p.Side != "LONG" && p.Side != "SHORT" && p.Side != "BOTH") {
			return ErrResponse
		}
		seen[key] = true
		if !numeric(p.Quantity, false) || !numeric(p.EntryPrice, true) || !numeric(p.MarkPrice, true) || !numeric(p.LiquidationPrice, true) || !numeric(p.UnrealizedProfit, false) {
			return ErrResponse
		}
		quantity, _ := decimal.NewFromString(p.Quantity)
		if (p.Side == "LONG" && quantity.IsNegative()) || (p.Side == "SHORT" && quantity.IsPositive()) {
			return ErrResponse
		}
	}
	return nil
}
