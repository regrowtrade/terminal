package planner

import (
	"github.com/shopspring/decimal"
	"testing"
)

func d(s string) decimal.Decimal { return decimal.RequireFromString(s) }
func base() Request {
	return Request{Mode: "clean", Symbol: "BTCUSDT", Side: "LONG", Entry: "100", Quantity: "1", Risk: "2", RiskUnit: "amount", AccountReference: "100", AccountBasis: "equity", RewardRatio: "3", SpacingPercent: "4", Leverage: 10, MaxLevels: 4, MaxMargin: "10000", CostReserve: "0"}
}
func hedge() Request {
	r := base()
	r.Mode = "hedge"
	r.Side = "SHORT"
	r.Entry = "100"
	r.Risk = "20"
	r.Hedge = &HedgeInput{LongEntry: "100", ShortEntry: "98", LongQuantity: "10", ShortQuantity: "10", MarkPrice: "99", RealizedPnl: "0", FeesPaid: "0", FundingNet: "0", ClosePercent: "5"}
	return r
}
func equal(t *testing.T, got, want string) {
	t.Helper()
	if !d(got).Equal(d(want)) {
		t.Fatalf("got %s, want %s", got, want)
	}
}

func TestCleanRewardAndRisk(t *testing.T) {
	for _, side := range []string{"LONG", "SHORT"} {
		t.Run(side, func(t *testing.T) {
			r := base()
			r.Side = side
			p, err := Preview(r)
			if err != nil {
				t.Fatal(err)
			}
			if !p.Simulation || len(p.Levels) != 1 {
				t.Fatal("clean must be a single simulated entry")
			}
			stop, tp := "98", "106"
			if side == "SHORT" {
				stop, tp = "102", "94"
			}
			equal(t, *p.Levels[0].StopPrice, stop)
			equal(t, *p.Levels[0].TakeProfit, tp)
		})
	}
}
func TestCostReserve(t *testing.T) {
	r := base()
	r.CostReserve = "0.5"
	p, err := Preview(r)
	if err != nil {
		t.Fatal(err)
	}
	equal(t, *p.Levels[0].StopPrice, "98.5")
	equal(t, *p.Levels[0].TakeProfit, "106.5")
}
func TestLeverageDoesNotMultiplyPnl(t *testing.T) {
	r := base()
	a, _ := Preview(r)
	r.Leverage = 50
	b, _ := Preview(r)
	equal(t, *a.Levels[0].StopPrice, *b.Levels[0].StopPrice)
	equal(t, b.Levels[0].Margin, "2")
}
func TestDCALegacySequenceAndCompounding(t *testing.T) {
	r := base()
	r.Mode = "dca"
	r.Risk = "50"
	p, err := Preview(r)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Levels) != 4 {
		t.Fatal(p.StopReason)
	}
	for i, q := range []string{"1", "1", "2", "4"} {
		equal(t, p.Levels[i].Quantity, q)
	}
	equal(t, p.Levels[2].Price, "92.16")
	equal(t, p.Levels[3].Price, "88.4736")
}
func TestDCAStopsAtRisk(t *testing.T) {
	r := base()
	r.Mode = "dca"
	r.Risk = "10"
	p, err := Preview(r)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Levels) != 2 {
		t.Fatalf("expected 2 levels, got %d", len(p.Levels))
	}
}
func TestMarginCap(t *testing.T) {
	r := base()
	r.Mode = "dca"
	r.Risk = "50"
	r.MaxMargin = "20"
	p, err := Preview(r)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Levels) != 2 {
		t.Fatal("margin cap not applied")
	}
}
func TestHedgeSpacingAndCompensatingTP(t *testing.T) {
	for _, tc := range []struct{ pct, spacing string }{{"5", "3"}, {"10", "2.4"}} {
		t.Run(tc.pct, func(t *testing.T) {
			r := hedge()
			r.Hedge.ClosePercent = tc.pct
			p, err := Preview(r)
			if err != nil {
				t.Fatal(err)
			}
			equal(t, p.SpacingPercent, tc.spacing)
			equal(t, *p.Levels[0].TakeProfit, d("100").Mul(d("1").Add(d(tc.spacing).Div(d("100")))).String())
			equal(t, *p.BaselinePnl, "-20")
			equal(t, *p.PnlThreshold, "-40")
		})
	}
}
func TestHedgeNewActivationUsesRecoveredNet(t *testing.T) {
	r := hedge()
	r.MaxLevels = 1
	r.Hedge.RealizedPnl = "10"
	r.Risk = "10"
	p, err := Preview(r)
	if err != nil {
		t.Fatal(err)
	}
	equal(t, *p.BaselinePnl, "-10")
	equal(t, *p.PnlThreshold, "-20")
}
func TestHedgeAccountsForFundingAndFees(t *testing.T) {
	r := hedge()
	r.Hedge.FeesPaid = "2"
	r.Hedge.FundingNet = "1"
	p, err := Preview(r)
	if err != nil {
		t.Fatal(err)
	}
	equal(t, *p.BaselinePnl, "-21")
}
func TestHedgeNeverExceedsSelectedPosition(t *testing.T) {
	r := hedge()
	r.Risk = "40"
	r.MaxLevels = 11
	p, err := Preview(r)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Levels) != 5 {
		t.Fatalf("expected 5, got %d", len(p.Levels))
	}
	equal(t, p.Levels[4].CumulativeQuantity, "8")
}
func TestClosePartialAndRecalculateStop(t *testing.T) {
	e := Exposure{LongEntry: d("100"), ShortEntry: d("98"), LongQuantity: d("10"), ShortQuantity: d("10"), RealizedNet: d("0")}
	before := e.Pnl(d("100"))
	next, err := e.Close("SHORT", d("0.25"), d("100"), d("0.1"))
	if err != nil {
		t.Fatal(err)
	}
	equal(t, next.Pnl(d("100")).String(), before.Sub(d("0.1")).String())
	s, err := next.StopPrice(d("-40"), d("0"))
	if err != nil {
		t.Fatal(err)
	}
	equal(t, next.Pnl(d(*s)).String(), "-40")
	if e.ShortQuantity.String() != "10" {
		t.Fatal("original exposure mutated")
	}
}
func TestShortNetStop(t *testing.T) {
	e := Exposure{LongEntry: d("100"), ShortEntry: d("98"), LongQuantity: d("9"), ShortQuantity: d("10"), RealizedNet: d("0")}
	s, err := e.StopPrice(d("-40"), d("1"))
	if err != nil {
		t.Fatal(err)
	}
	equal(t, e.Pnl(d(*s)).Sub(d("1")).String(), "-40")
}
func TestBalancedExposureNoStop(t *testing.T) {
	e := Exposure{LongQuantity: d("1"), ShortQuantity: d("1")}
	s, err := e.StopPrice(d("-2"), d("0"))
	if err != nil || s != nil {
		t.Fatal("balanced exposure must not have directional stop")
	}
}
func TestInvalidPartialClose(t *testing.T) {
	e := Exposure{LongQuantity: d("1")}
	for _, q := range []string{"0", "-1", "2"} {
		if _, err := e.Close("LONG", d(q), d("100"), d("0")); err == nil {
			t.Fatal("invalid close accepted")
		}
	}
}
func TestValidation(t *testing.T) {
	tests := []struct {
		name   string
		change func(*Request)
	}{
		{"negative risk", func(r *Request) { r.Risk = "-1" }}, {"scientific notation", func(r *Request) { r.Risk = "1e2" }},
		{"infinity", func(r *Request) { r.Quantity = "Infinity" }}, {"oversized decimal", func(r *Request) { r.Entry = "99999999999999999" }},
		{"invalid side", func(r *Request) { r.Side = "BOTH" }}, {"invalid symbol", func(r *Request) { r.Symbol = "BTC/USDT" }},
		{"invalid leverage", func(r *Request) { r.Leverage = 0 }}, {"invalid mode", func(r *Request) { r.Mode = "live" }},
		{"too many levels", func(r *Request) { r.MaxLevels = 12 }}, {"reserve exceeds risk", func(r *Request) { r.CostReserve = "2" }},
		{"invalid spacing", func(r *Request) { r.SpacingPercent = "100" }}, {"missing hedge", func(r *Request) { r.Mode = "hedge" }},
		{"insufficient margin", func(r *Request) { r.MaxMargin = "1" }},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := base()
			tc.change(&r)
			if _, err := Preview(r); err == nil {
				t.Fatal("invalid request accepted")
			}
		})
	}
}
