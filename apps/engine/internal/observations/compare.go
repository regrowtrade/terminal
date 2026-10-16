// Package observations compares bounded REST observations, not exchange events.
// Absence, equality and accumulated executedQty never establish a terminal order
// status, a fill ledger, stream continuity or permission to execute.
package observations

import (
	"errors"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/shopspring/decimal"
	"regrow/engine/internal/binance"
)

const DetailLimit = 200

var ErrInvalid = errors.New("no se pueden comparar lecturas incompletas o fuera de orden")

type FieldChange struct {
	Field  string  `json:"field"`
	Before *string `json:"before"`
	After  *string `json:"after"`
}
type Change struct {
	Entity string        `json:"entity"`
	Key    string        `json:"key"`
	Kind   string        `json:"kind"`
	Fields []FieldChange `json:"fields"`
}
type Report struct {
	SchemaVersion      int        `json:"schemaVersion"`
	Mode               string     `json:"mode"`
	Status             string     `json:"status"`
	PreviousCapturedAt *time.Time `json:"previousCapturedAt"`
	CurrentStartedAt   time.Time  `json:"currentStartedAt"`
	CurrentCapturedAt  time.Time  `json:"currentCapturedAt"`
	IntervalRelation   string     `json:"intervalRelation"`
	Continuity         string     `json:"continuity"`
	ExecutionReady     bool       `json:"executionReady"`
	TotalChanges       int        `json:"totalChanges"`
	DetailLimit        int        `json:"detailLimit"`
	Truncated          bool       `json:"truncated"`
	Changes            []Change   `json:"changes"`
}

type row map[string]string
type group map[string]row

var entities = []string{"asset", "position", "order", "algo", "instrument"}
var numericFields = map[string]bool{
	"walletBalance": true, "unrealizedProfit": true, "availableBalance": true,
	"positionAmt": true, "entryPrice": true, "markPrice": true, "unRealizedProfit": true, "liquidationPrice": true,
	"origQty": true, "executedQty": true, "price": true, "stopPrice": true, "activatePrice": true, "priceRate": true,
	"quantity": true, "triggerPrice": true, "callbackRate": true,
	"priceFilter.min": true, "priceFilter.max": true, "priceFilter.step": true,
	"lotSize.min": true, "lotSize.max": true, "lotSize.step": true,
	"marketLotSize.min": true, "marketLotSize.max": true, "marketLotSize.step": true,
	"minNotional": true, "multiplierUp": true, "multiplierDown": true,
}

func rows(s binance.Snapshot) map[string]group {
	result := map[string]group{}
	for _, entity := range entities {
		result[entity] = group{}
	}
	for _, a := range s.Assets {
		result["asset"][a.Asset] = row{"walletBalance": a.WalletBalance, "unrealizedProfit": a.UnrealizedProfit, "availableBalance": a.AvailableBalance}
	}
	for _, p := range s.Positions {
		result["position"][p.Symbol+":"+p.Side] = row{"positionAmt": p.Quantity, "entryPrice": p.EntryPrice, "markPrice": p.MarkPrice, "unRealizedProfit": p.UnrealizedProfit, "liquidationPrice": p.LiquidationPrice}
	}
	for _, o := range s.OpenOrders {
		r := row{"clientOrderId": o.ClientID, "side": o.Side, "positionSide": o.PositionSide, "type": o.Type, "status": o.Status,
			"origQty": o.Quantity, "executedQty": o.Executed, "price": o.Price, "stopPrice": o.StopPrice, "reduceOnly": strconv.FormatBool(o.ReduceOnly),
			"closePosition": strconv.FormatBool(o.ClosePosition), "workingType": o.WorkingType}
		if o.ActivatePrice != "" {
			r["activatePrice"] = o.ActivatePrice
		}
		if o.PriceRate != "" {
			r["priceRate"] = o.PriceRate
		}
		result["order"][o.Symbol+":"+o.ID] = r
	}
	for _, o := range s.AlgoOrders {
		r := row{"clientAlgoId": o.ClientID, "algoType": o.AlgoType, "side": o.Side, "positionSide": o.PositionSide, "orderType": o.Type, "algoStatus": o.Status,
			"quantity": o.Quantity, "price": o.Price, "triggerPrice": o.TriggerPrice, "reduceOnly": strconv.FormatBool(o.ReduceOnly),
			"closePosition": strconv.FormatBool(o.ClosePosition), "workingType": o.WorkingType}
		if o.ActivatePrice != "" {
			r["activatePrice"] = o.ActivatePrice
		}
		if o.CallbackRate != "" {
			r["callbackRate"] = o.CallbackRate
		}
		result["algo"][o.Symbol+":"+o.ID] = r
	}
	for _, i := range s.Instruments {
		unknown := append([]string{}, i.UnknownFilters...)
		sort.Strings(unknown)
		result["instrument"][i.Symbol] = row{"contractType": i.ContractType, "status": i.Status, "baseAsset": i.BaseAsset, "quoteAsset": i.QuoteAsset, "marginAsset": i.MarginAsset,
			"priceFilter.min": i.Price.Min, "priceFilter.max": i.Price.Max, "priceFilter.step": i.Price.Step,
			"lotSize.min": i.Lot.Min, "lotSize.max": i.Lot.Max, "lotSize.step": i.Lot.Step,
			"marketLotSize.min": i.MarketLot.Min, "marketLotSize.max": i.MarketLot.Max, "marketLotSize.step": i.MarketLot.Step,
			"minNotional": i.MinNotional, "multiplierUp": i.MultiplierUp, "multiplierDown": i.MultiplierDown,
			"maxNumOrders": strconv.FormatInt(i.MaxOrders, 10), "maxNumAlgoOrders": strconv.FormatInt(i.MaxAlgos, 10), "unrecognizedFilterTypes": strings.Join(unknown, ",")}
	}
	return result
}

func keys[T any](a, b map[string]T) []string {
	seen := map[string]bool{}
	for k := range a {
		seen[k] = true
	}
	for k := range b {
		seen[k] = true
	}
	result := make([]string, 0, len(seen))
	for k := range seen {
		result = append(result, k)
	}
	sort.Strings(result)
	return result
}
func text(value string) *string { return &value }
func equal(field, a, b string) bool {
	if a == b {
		return true
	}
	if !numericFields[field] {
		return false
	}
	left, e1 := decimal.NewFromString(a)
	right, e2 := decimal.NewFromString(b)
	return e1 == nil && e2 == nil && left.Equal(right)
}

// Compare requires observations of the same owned account; the database caller
// binds that identity. No network requests or writes occur in this pure function.
func Compare(previous *binance.Snapshot, current binance.Snapshot) (Report, error) {
	if binance.ValidateSnapshot(current) != nil {
		return Report{}, ErrInvalid
	}
	report := Report{SchemaVersion: 1, Mode: "rest_observation_comparison", Status: "initial_observation", CurrentStartedAt: current.StartedAt.UTC(), CurrentCapturedAt: current.CapturedAt.UTC(),
		IntervalRelation: "no_baseline", Continuity: "not_established", ExecutionReady: false, DetailLimit: DetailLimit, Changes: []Change{}}
	if previous == nil {
		return report, nil
	}
	if binance.ValidateSnapshot(*previous) != nil || !current.CapturedAt.After(previous.CapturedAt) {
		return Report{}, ErrInvalid
	}
	baseline := previous.CapturedAt.UTC()
	report.PreviousCapturedAt = &baseline
	report.Status = "compared"
	report.IntervalRelation = "disjoint"
	if current.StartedAt.Before(previous.CapturedAt) {
		report.IntervalRelation = "overlapping"
	}
	before, after := rows(*previous), rows(current)
	for _, entity := range entities {
		for _, key := range keys(before[entity], after[entity]) {
			left, right := before[entity][key], after[entity][key]
			change := Change{Entity: entity, Key: key, Kind: "changed", Fields: []FieldChange{}}
			retain := len(report.Changes) < DetailLimit
			changed := false
			if left == nil {
				change.Kind = "observed"
			} else if right == nil {
				change.Kind = "not_observed"
			}
			for _, field := range keys(left, right) {
				a, had := left[field]
				b, has := right[field]
				if had && has && equal(field, a, b) {
					continue
				}
				changed = true
				// Counts concern records, not fields. Once details are full, one
				// difference suffices; do not allocate discarded field payloads.
				if !retain {
					break
				}
				diff := FieldChange{Field: field}
				if had {
					diff.Before = text(a)
				}
				if has {
					diff.After = text(b)
				}
				change.Fields = append(change.Fields, diff)
			}
			if !changed {
				continue
			}
			report.TotalChanges++
			if retain {
				report.Changes = append(report.Changes, change)
			}
		}
	}
	report.Truncated = report.TotalChanges > len(report.Changes)
	return report, nil
}
