// Package intentcheck compares normal-order evidence with a stored decision.
// Attribute agreement is not proof that this system submitted or owns an order.
package intentcheck

import (
	"errors"
	"reflect"
	"regexp"

	"github.com/shopspring/decimal"
	"regrow/engine/internal/binance"
)

var ErrEvidence = errors.New("evidencia de intención no válida")
var uuid = regexp.MustCompile(`^[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$`)
var clientID = regexp.MustCompile(`^rt[a-f0-9]{30}$`)
var fingerprint = regexp.MustCompile(`^[a-f0-9]{64}$`)
var number = regexp.MustCompile(`^(?:0|[1-9][0-9]{0,37})(?:\.[0-9]{1,18})?$`)

// Expected is derived from an owned persisted plan and its normal entry/rebuy.
// This release requires exact LONG/SHORT positionSide agreement; BOTH is not
// silently treated as compatible with a plan that has no one-way mode policy.
type Expected struct {
	IntentID        string `json:"intentId"`
	PlanID          string `json:"planId"`
	PlanFingerprint string `json:"planFingerprint"`
	Symbol          string `json:"symbol"`
	ClientOrderID   string `json:"clientOrderId"`
	Purpose         string `json:"purpose"`
	PositionSide    string `json:"positionSide"`
	Quantity        string `json:"quantity"`
	Price           string `json:"price"`
}

func (e Expected) Query() binance.OrderQuery {
	return binance.OrderQuery{Symbol: e.Symbol, ClientOrderID: e.ClientOrderID}
}

func positive(s string) bool {
	d, err := decimal.NewFromString(s)
	return number.MatchString(s) && err == nil && d.IsPositive()
}

func (e Expected) Valid() bool {
	return uuid.MatchString(e.IntentID) && uuid.MatchString(e.PlanID) && fingerprint.MatchString(e.PlanFingerprint) && clientID.MatchString(e.ClientOrderID) &&
		e.Query().Valid() && (e.Purpose == "ENTRY" || e.Purpose == "REBUY") &&
		(e.PositionSide == "LONG" || e.PositionSide == "SHORT") && positive(e.Quantity) && positive(e.Price)
}

type Report struct {
	Expected           Expected            `json:"expected"`
	Observation        binance.OrderLookup `json:"observation"`
	Assessment         string              `json:"assessment"`
	Mismatches         []string            `json:"mismatches"`
	ExecutionReady     bool                `json:"executionReady"`
	OwnershipConfirmed bool                `json:"ownershipConfirmed"`
}

// Assess never changes intent state, synthesizes fills, or permits a retry.
// -2013 remains unresolved even though the client ID is deterministic.
func Assess(e Expected, observation binance.OrderLookup) (Report, error) {
	if !e.Valid() || observation.Query != e.Query() || binance.ValidateOrderLookup(observation) != nil {
		return Report{}, ErrEvidence
	}
	r := Report{Expected: e, Observation: observation, Assessment: "unresolved", Mismatches: []string{}}
	if observation.Outcome == "unresolved" {
		return r, nil
	}
	o := observation.Order
	side := "BUY"
	if e.PositionSide == "SHORT" {
		side = "SELL"
	}
	for _, check := range []struct {
		field string
		match bool
	}{
		{"type", o.Type == "LIMIT"},
		{"side", o.Side == side},
		{"positionSide", o.PositionSide == e.PositionSide},
		{"quantity", decimal.RequireFromString(o.Quantity).Equal(decimal.RequireFromString(e.Quantity))},
		{"price", decimal.RequireFromString(o.Price).Equal(decimal.RequireFromString(e.Price))},
		{"reduceOnly", !o.ReduceOnly},
	} {
		if !check.match {
			r.Mismatches = append(r.Mismatches, check.field)
		}
	}
	r.Assessment = "attributes_match"
	if len(r.Mismatches) > 0 {
		r.Assessment = "mismatch"
	}
	return r, nil
}

// Validate rederives assessments when recovering append-only historical data.
func Validate(r Report) error {
	want, err := Assess(r.Expected, r.Observation)
	if err != nil || !reflect.DeepEqual(want, r) {
		return ErrEvidence
	}
	return nil
}
