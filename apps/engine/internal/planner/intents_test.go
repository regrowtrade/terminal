package planner

import (
	"strings"
	"testing"
)

func planWithLevels(mode string, levels []Level) Plan {
	return Plan{Mode: mode, Symbol: "BTCUSDT", Side: "LONG", Simulation: true, Levels: levels}
}

func TestIntentsCleanAndDcaLadder(t *testing.T) {
	stop, tp := "90", "110"
	levels := []Level{
		{Index: 1, Price: "100", Quantity: "1", CumulativeQuantity: "1", TakeProfitQuantity: "1", StopPrice: &stop, TakeProfit: &tp},
		{Index: 2, Price: "96", Quantity: "2", CumulativeQuantity: "3", TakeProfitQuantity: "3", StopPrice: &stop, TakeProfit: &tp},
	}
	got, err := Intents(planWithLevels("dca", levels))
	if err != nil || len(got) != 4 {
		t.Fatal("unexpected dca intent ladder", err, len(got))
	}
	want := []Intent{
		{Purpose: "ENTRY", Family: "regular", PositionSide: "LONG", Quantity: "1", Price: "100"},
		{Purpose: "REBUY", Family: "regular", PositionSide: "LONG", Quantity: "2", Price: "96"},
		{Purpose: "TP", Family: "conditional", PositionSide: "LONG", Quantity: "3", Price: "110"},
		{Purpose: "SL", Family: "conditional", PositionSide: "LONG", Quantity: "3", Price: "90"},
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatal("intent mismatch at", i, got[i], want[i])
		}
	}
	clean, err := Intents(planWithLevels("clean", levels[:1]))
	if err != nil || len(clean) != 3 {
		t.Fatal("clean mode should produce entry+tp+sl", err, len(clean))
	}
	if clean[0].Purpose != "ENTRY" || clean[1].Purpose != "TP" || clean[2].Purpose != "SL" {
		t.Fatal("clean ladder purposes wrong", clean)
	}
}

func TestIntentsRejectsUnsupportedAndMalformedPlans(t *testing.T) {
	if _, err := Intents(planWithLevels("hedge", []Level{{Price: "1", Quantity: "1"}})); err != ErrIntentUnsupported {
		t.Fatal("hedge intents must fail closed", err)
	}
	if _, err := Intents(planWithLevels("clean", nil)); err == nil {
		t.Fatal("empty levels accepted")
	}
	bad := planWithLevels("clean", []Level{{Price: "0", Quantity: "1", TakeProfit: nil, StopPrice: nil}})
	if _, err := Intents(bad); err == nil {
		t.Fatal("non-positive price accepted")
	}
	badSide := planWithLevels("clean", []Level{{Price: "1", Quantity: "1"}})
	badSide.Side = "BOTH"
	if _, err := Intents(badSide); err == nil {
		t.Fatal("invalid side accepted")
	}
}

func TestIntentClientOrderIDIsDeterministicAndSafe(t *testing.T) {
	a := IntentClientOrderID("00000000-0000-4000-8000-000000000001", "ENTRY", 0)
	b := IntentClientOrderID("00000000-0000-4000-8000-000000000001", "ENTRY", 0)
	if a != b {
		t.Fatal("client order id not deterministic")
	}
	if len(a) > 36 || !strings.HasPrefix(a, "rt") || strings.ToLower(a) != a {
		t.Fatal("client order id not exchange-safe", a)
	}
	if IntentClientOrderID("00000000-0000-4000-8000-000000000001", "ENTRY", 0) == IntentClientOrderID("00000000-0000-4000-8000-000000000001", "ENTRY", 1) {
		t.Fatal("distinct ordinals collided")
	}
}
