package intentcheck

import (
	"strings"
	"testing"
	"time"

	"regrow/engine/internal/binance"
)

func synthetic() (Expected, binance.OrderLookup) {
	e := Expected{IntentID: "00000000-0000-4000-8000-000000000001", PlanID: "00000000-0000-4000-8000-000000000002", PlanFingerprint: strings.Repeat("b", 64), Symbol: "BTCUSDT", ClientOrderID: "rt" + strings.Repeat("a", 30), Purpose: "ENTRY", PositionSide: "LONG", Quantity: "2", Price: "100"}
	now := time.Now().UTC()
	o := binance.OrderLookup{Source: "binance-production", Mode: "read_only", StartedAt: now, CheckedAt: now, Query: e.Query(), Outcome: "observed", Order: &binance.QueriedOrder{OrderID: "9007199254740993", ClientOrderID: e.ClientOrderID, Symbol: e.Symbol, Side: "BUY", PositionSide: "LONG", Type: "LIMIT", Status: "PARTIALLY_FILLED", Quantity: "2.000", Executed: "0.5", Price: "100.000", AveragePrice: "99", UpdateTime: 1750514941540}}
	return e, o
}

func TestAttributeAssessmentDoesNotConfirmOwnershipOrExecution(t *testing.T) {
	for _, side := range []string{"LONG", "SHORT"} {
		e, o := synthetic()
		e.PositionSide, o.Order.PositionSide = side, side
		if side == "SHORT" {
			o.Order.Side = "SELL"
			e.Purpose = "REBUY"
		}
		r, err := Assess(e, o)
		if err != nil || r.Assessment != "attributes_match" || len(r.Mismatches) != 0 || r.ExecutionReady || r.OwnershipConfirmed || Validate(r) != nil {
			t.Fatal("unsafe assessment", err, r)
		}
		// Even FILLED is only the exchange's order status, not a position close.
		o.Order.Status, o.Order.Executed = "FILLED", "2"
		r, err = Assess(e, o)
		if err != nil || r.Assessment != "attributes_match" || r.ExecutionReady || r.OwnershipConfirmed {
			t.Fatal("fill enabled execution")
		}
	}
}

func TestMismatchesUseDecimalEqualityAndExplicitPositionMode(t *testing.T) {
	for _, tc := range []struct {
		field  string
		mutate func(*binance.QueriedOrder)
	}{
		{"type", func(o *binance.QueriedOrder) { o.Type = "MARKET" }},
		{"side", func(o *binance.QueriedOrder) { o.Side = "SELL" }},
		{"positionSide", func(o *binance.QueriedOrder) { o.PositionSide = "BOTH" }},
		{"quantity", func(o *binance.QueriedOrder) { o.Quantity = "2.000000000000000001" }},
		{"price", func(o *binance.QueriedOrder) { o.Price = "100.000000000000000001" }},
		{"reduceOnly", func(o *binance.QueriedOrder) { o.ReduceOnly = true }},
	} {
		t.Run(tc.field, func(t *testing.T) {
			e, o := synthetic()
			tc.mutate(o.Order)
			r, err := Assess(e, o)
			if err != nil || r.Assessment != "mismatch" || len(r.Mismatches) != 1 || r.Mismatches[0] != tc.field || Validate(r) != nil {
				t.Fatal("missed mismatch", err, r)
			}
		})
	}
}

func TestAbsenceAndInvalidEvidenceFailClosed(t *testing.T) {
	e, o := synthetic()
	o.Outcome, o.Reason, o.Order = "unresolved", "exchange_not_found", nil
	r, err := Assess(e, o)
	if err != nil || r.Assessment != "unresolved" || len(r.Mismatches) != 0 || Validate(r) != nil {
		t.Fatal("absence reinterpreted")
	}
	for _, mutate := range []func(*Report){
		func(r *Report) { r.ExecutionReady = true },
		func(r *Report) { r.OwnershipConfirmed = true },
		func(r *Report) { r.Assessment = "attributes_match" },
		func(r *Report) { r.Mismatches = []string{"price"} },
		func(r *Report) { r.Expected.Price = "0" },
		func(r *Report) { r.Observation.Query.ClientOrderID = "other" },
		func(r *Report) { r.Observation.ExecutionReady = true },
	} {
		bad := r
		mutate(&bad)
		if Validate(bad) == nil {
			t.Fatal("corrupt evidence accepted")
		}
	}
}
