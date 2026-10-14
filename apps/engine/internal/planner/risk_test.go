package planner

import "testing"

func TestPercentageRiskUsesCapturedReference(t *testing.T) {
	r := base()
	r.RiskUnit = "percent"
	r.AccountReference = "250"
	r.Risk = "2"
	p, err := Preview(r)
	if err != nil {
		t.Fatal(err)
	}
	equal(t, p.Risk, "5")
	equal(t, *p.Levels[0].StopPrice, "95")
	equal(t, p.AccountReference, "250")
}
func TestTinyRiskDoesNotRoundToZero(t *testing.T) {
	r := base()
	r.RiskUnit = "percent"
	r.AccountReference = "0.000000000001"
	r.Risk = "0.000000000001"
	r.Quantity = "0.000000000001"
	r.Entry = "1"
	p, err := Preview(r)
	if err != nil {
		t.Fatal(err)
	}
	equal(t, p.Risk, "0.00000000000000000000000001")
	if *p.Levels[0].StopPrice == r.Entry {
		t.Fatal("risk precision lost")
	}
}
func TestInvalidRiskBasis(t *testing.T) {
	for _, change := range []func(*Request){func(r *Request) { r.RiskUnit = "auto" }, func(r *Request) { r.AccountBasis = "available" }, func(r *Request) { r.AccountReference = "0" }, func(r *Request) { r.RiskUnit = "percent"; r.Risk = "101" }} {
		r := base()
		change(&r)
		if _, err := Preview(r); err == nil {
			t.Fatal("invalid risk configuration accepted")
		}
	}
}
func TestHedgeTruncatesUnsafeFutureScenarios(t *testing.T) {
	r := hedge()
	r.Risk = "10"
	p, err := Preview(r)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Levels) >= 4 || len(p.Levels) == 0 {
		t.Fatal("unsafe scenarios not truncated")
	}
}
func TestOppositeTPQuantityIsCapped(t *testing.T) {
	r := hedge()
	r.Side = "LONG"
	r.Hedge.LongQuantity = "10"
	r.Hedge.ShortQuantity = "0.1"
	r.MaxLevels = 1
	p, err := Preview(r)
	if err != nil {
		t.Fatal(err)
	}
	equal(t, p.Levels[0].TakeProfitQuantity, "0.1")
}
func TestInvalidHedgeAndUnprotectableFirstScenario(t *testing.T) {
	for _, change := range []func(*Request){
		func(r *Request) { r.Hedge.ClosePercent = "7" }, func(r *Request) { r.Hedge.LongEntry = r.Hedge.ShortEntry },
		func(r *Request) { r.Hedge.FeesPaid = "-1" }, func(r *Request) { r.Hedge.LongQuantity = "0" },
		func(r *Request) { r.Risk = "1000" },
	} {
		r := hedge()
		change(&r)
		if _, err := Preview(r); err == nil {
			t.Fatal("invalid hedge accepted")
		}
	}
}
