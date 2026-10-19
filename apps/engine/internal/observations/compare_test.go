package observations

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"regrow/engine/internal/binance"
)

func sample() binance.Snapshot {
	now := time.Date(2026, 1, 1, 0, 0, 1, 0, time.UTC)
	return binance.Snapshot{Source: "binance-production", Mode: "read_only", Consistency: "rest_non_atomic", StartedAt: now.Add(-time.Second), CapturedAt: now, CapturedAtMs: now.UnixMilli(),
		Assets:      []binance.Asset{{Asset: "USDT", WalletBalance: "100.00000001", UnrealizedProfit: "2", AvailableBalance: "80"}},
		Positions:   []binance.Position{{Symbol: "BTCUSDT", Side: "LONG", Quantity: "1.000", EntryPrice: "100", MarkPrice: "102", UnrealizedProfit: "2", LiquidationPrice: "50"}},
		OpenOrders:  []binance.Order{{ID: "9007199254740993", ClientID: "synthetic-limit", Symbol: "BTCUSDT", Side: "BUY", PositionSide: "LONG", Type: "LIMIT", Status: "NEW", Quantity: "2", Executed: "0", Price: "90", StopPrice: "0", WorkingType: "CONTRACT_PRICE", UpdateTime: 1750514941540}},
		AlgoOrders:  []binance.AlgoOrder{{ID: "9007199254740993", ClientID: "synthetic-stop", AlgoType: "CONDITIONAL", Symbol: "BTCUSDT", Side: "SELL", PositionSide: "LONG", Type: "STOP_MARKET", Status: "NEW", Quantity: "0", Price: "0", TriggerPrice: "80", ClosePosition: true, WorkingType: "MARK_PRICE", UpdateTime: 1750514941540}},
		Instruments: []binance.Instrument{{Symbol: "BTCUSDT", ContractType: "PERPETUAL", Status: "TRADING", BaseAsset: "BTC", QuoteAsset: "USDT", MarginAsset: "USDT", Price: binance.RangeFilter{Min: "0.1", Max: "1000000", Step: "0.1"}, Lot: binance.RangeFilter{Min: "0.001", Max: "1000", Step: "0.001"}, MarketLot: binance.RangeFilter{Min: "0.001", Max: "100", Step: "0.001"}, MinNotional: "5", MultiplierUp: "1.05", MultiplierDown: "0.95", MaxOrders: 200, MaxAlgos: 10, UnknownFilters: []string{}}}}
}
func next(t *testing.T, previous binance.Snapshot) binance.Snapshot {
	t.Helper()
	data, err := json.Marshal(previous)
	if err != nil {
		t.Fatal(err)
	}
	var result binance.Snapshot
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatal(err)
	}
	result.StartedAt = previous.CapturedAt.Add(time.Second)
	result.CapturedAt = result.StartedAt.Add(time.Second)
	return result
}
func compare(t *testing.T, before *binance.Snapshot, after binance.Snapshot) Report {
	t.Helper()
	report, err := Compare(before, after)
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateReport(report); err != nil {
		t.Fatal("generated invalid report")
	}
	if report.ExecutionReady || report.Continuity != "not_established" {
		t.Fatal("comparison claimed execution/stream readiness")
	}
	return report
}
func TestInitialObservationIsNotAComparedBaseline(t *testing.T) {
	r := compare(t, nil, sample())
	if r.Status != "initial_observation" || r.PreviousCapturedAt != nil || r.TotalChanges != 0 || r.Changes == nil {
		t.Fatal("invented initial differences")
	}
}
func TestDecimalFormattingAndMetadataDoNotInventActivity(t *testing.T) {
	previous := sample()
	previous.Instruments[0].UnknownFilters = []string{"ZZ_FILTER", "AA_FILTER"}
	current := next(t, previous)
	current.Positions[0].Quantity = "1"
	current.OpenOrders[0].Price = "90.000000000000000000"
	current.OpenOrders[0].UpdateTime++
	current.AlgoOrders[0].UpdateTime++
	current.Instruments[0].Price.Step = "0.100000000000000000"
	current.Instruments[0].UnknownFilters = []string{"AA_FILTER", "ZZ_FILTER"}
	r := compare(t, &previous, current)
	if r.Status != "compared" || r.TotalChanges != 0 || r.Truncated {
		t.Fatal("format-only changes treated as activity")
	}
	if previous.Instruments[0].UnknownFilters[0] != "ZZ_FILTER" {
		t.Fatal("comparison mutated its baseline")
	}
}
func TestDecimalDifferenceIsExactAndPreservesOriginalText(t *testing.T) {
	previous := sample()
	current := next(t, previous)
	current.Positions[0].Quantity = "1.000000000000000001"
	r := compare(t, &previous, current)
	if r.TotalChanges != 1 || r.Changes[0].Entity != "position" || r.Changes[0].Kind != "changed" || *r.Changes[0].Fields[0].Before != "1.000" || *r.Changes[0].Fields[0].After != "1.000000000000000001" {
		t.Fatal("decimal difference lost")
	}
}
func TestAbsenceNeverBecomesFillCancelOrZeroQuantity(t *testing.T) {
	previous := sample()
	current := next(t, previous)
	current.Positions = []binance.Position{}
	current.OpenOrders = []binance.Order{}
	current.AlgoOrders = []binance.AlgoOrder{}
	r := compare(t, &previous, current)
	if r.TotalChanges != 3 {
		t.Fatal("families or position side merged")
	}
	for _, c := range r.Changes {
		if c.Kind != "not_observed" {
			t.Fatal("absence classified as terminal state")
		}
		for _, f := range c.Fields {
			if f.Before == nil || f.After != nil {
				t.Fatal("absence replaced by a fabricated value")
			}
		}
	}
	encoded, _ := json.Marshal(r)
	for _, inferred := range []string{"CANCELED", "FILLED", "FINISHED", "CLOSED"} {
		if strings.Contains(string(encoded), inferred) {
			t.Fatal("terminal state inferred")
		}
	}
	if r.Changes[1].Key != "BTCUSDT:9007199254740993" || r.Changes[2].Key != r.Changes[1].Key {
		t.Fatal("int64 identity or family separation lost")
	}
}
func TestObservedZeroPositionIsDistinctFromMissingPosition(t *testing.T) {
	previous := sample()
	current := next(t, previous)
	current.Positions[0].Quantity = "0"
	r := compare(t, &previous, current)
	if r.Changes[0].Kind != "changed" || *r.Changes[0].Fields[0].After != "0" {
		t.Fatal("explicit zero confused with absence")
	}
}
func TestFieldsIncludePartialAccumulationAndInstrumentFilterChanges(t *testing.T) {
	previous := sample()
	current := next(t, previous)
	current.OpenOrders[0].Status = "PARTIALLY_FILLED"
	current.OpenOrders[0].Executed = "0.5"
	current.Instruments[0].Price.Step = "0.5"
	r := compare(t, &previous, current)
	if r.TotalChanges != 2 || r.Changes[0].Entity != "order" || len(r.Changes[0].Fields) != 2 || r.Changes[1].Fields[0].Field != "priceFilter.step" {
		t.Fatal("observed fields omitted")
	}
}
func TestIntervalsRemainExplicitWithoutClaimingContinuity(t *testing.T) {
	previous := sample()
	current := next(t, previous)
	if compare(t, &previous, current).IntervalRelation != "disjoint" {
		t.Fatal("disjoint interval mislabeled")
	}
	current.StartedAt = previous.StartedAt
	if compare(t, &previous, current).IntervalRelation != "overlapping" {
		t.Fatal("overlap hidden")
	}
	current.CapturedAt = previous.CapturedAt
	if _, err := Compare(&previous, current); !errors.Is(err, ErrInvalid) {
		t.Fatal("duplicate/out-of-order comparison accepted")
	}
}
func TestDetailsAreBoundedButCountIsCompleteAndDeterministic(t *testing.T) {
	previous := sample()
	previous.Assets = []binance.Asset{}
	current := next(t, previous)
	for i := DetailLimit + 5; i > 0; i-- {
		current.Assets = append(current.Assets, binance.Asset{Asset: fmt.Sprintf("A%03d", i), WalletBalance: "1", UnrealizedProfit: "0", AvailableBalance: "1"})
	}
	r := compare(t, &previous, current)
	if !r.Truncated || r.TotalChanges != 205 || len(r.Changes) != DetailLimit || r.Changes[0].Key != "A001" || r.Changes[199].Key != "A200" {
		t.Fatal("truncation hidden or nondeterministic")
	}
	for _, c := range r.Changes {
		if c.Kind != "observed" {
			t.Fatal("new observation misclassified")
		}
	}
	if current.Assets[0].Asset != "A205" {
		t.Fatal("input sorted in place")
	}
}
func TestMalformedFinancialDataAndDuplicatesRejectWholeComparison(t *testing.T) {
	for _, mutate := range []func(*binance.Snapshot){
		func(s *binance.Snapshot) { s.Positions[0].Quantity = "NaN" },
		func(s *binance.Snapshot) { s.OpenOrders = append(s.OpenOrders, s.OpenOrders[0]) },
		func(s *binance.Snapshot) { s.Instruments[0].Price.Step = "-0" },
		func(s *binance.Snapshot) { s.Instruments[0].Symbol = "ETHUSDT" },
		func(s *binance.Snapshot) { s.OpenOrders[0].Executed = "3" },
		func(s *binance.Snapshot) { s.AlgoOrders = nil },
	} {
		previous := sample()
		current := next(t, previous)
		mutate(&current)
		if r, err := Compare(&previous, current); !errors.Is(err, ErrInvalid) || r.Changes != nil {
			t.Fatal("partial comparison of invalid current data")
		}
		if _, err := Compare(&current, previous); !errors.Is(err, ErrInvalid) {
			t.Fatal("invalid durable baseline accepted")
		}
	}
}
func TestReportValidationFailsClosedOnCorruption(t *testing.T) {
	previous := sample()
	current := next(t, previous)
	current.Positions[0].Quantity = "2"
	for _, mutate := range []func(*Report){
		func(r *Report) { r.ExecutionReady = true }, func(r *Report) { r.Continuity = "continuous" }, func(r *Report) { r.TotalChanges = 2 },
		func(r *Report) { r.Changes[0].Fields[0].Field = "apiKey" }, func(r *Report) { r.Changes[0].Fields[0].After = text("NaN") },
		func(r *Report) { r.Changes[0].Kind = "FILLED" }, func(r *Report) { r.PreviousCapturedAt = nil }, func(r *Report) { r.IntervalRelation = "overlapping" },
	} {
		r := compare(t, &previous, current)
		mutate(&r)
		if ValidateReport(r) == nil {
			t.Fatal("corrupt report accepted")
		}
	}
}
func TestAllNormalizedEntityFieldsCanBeReported(t *testing.T) {
	previous := sample()
	previous.Assets = []binance.Asset{}
	previous.Positions = []binance.Position{}
	previous.OpenOrders = []binance.Order{}
	previous.AlgoOrders = []binance.AlgoOrder{}
	current := next(t, sample())
	extra := current.Instruments[0]
	extra.Symbol, extra.BaseAsset = "ETHUSDT", "ETH"
	current.Instruments = append(current.Instruments, extra)
	r := compare(t, &previous, current)
	if r.TotalChanges != 5 {
		t.Fatal("an observed normalized entity was omitted")
	}
	for _, c := range r.Changes {
		if c.Kind != "observed" {
			t.Fatal("new observed record misclassified")
		}
	}
}
