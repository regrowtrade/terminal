package binance

import (
	"github.com/shopspring/decimal"
	"regexp"
	"strings"
)

var decimalText = regexp.MustCompile(`^-?(?:0|[1-9][0-9]{0,37})(?:\.[0-9]{1,18})?$`)
var instrument = regexp.MustCompile(`^[A-Z0-9]{2,24}USDT$`)

func numeric(value string, nonnegative bool) bool {
	if !decimalText.MatchString(value) {
		return false
	}
	d, err := decimal.NewFromString(value)
	return err == nil && (!nonnegative || !d.IsNegative())
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
