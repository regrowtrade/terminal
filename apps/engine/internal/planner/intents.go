package planner

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strconv"

	"github.com/shopspring/decimal"
)

// Intent is a single planned (never sent) order decomposed from a plan. Prices
// are limit prices for regular orders and trigger prices for conditional ones.
type Intent struct {
	Purpose      string `json:"purpose"`
	Family       string `json:"family"`
	PositionSide string `json:"positionSide"`
	Quantity     string `json:"quantity"`
	Price        string `json:"price"`
}

var ErrIntentUnsupported = errors.New("la descomposición de intenciones no soporta este modo todavía")

func positiveDecimal(value string) bool {
	d, err := decimal.NewFromString(value)
	return err == nil && d.IsPositive()
}

// Intents decomposes a clean/dca plan into its deterministic order ladder:
// one ENTRY for the first level, one REBUY per additional level, and a single
// final TP (takeProfitQuantity) and SL (cumulative quantity) from the last
// level. Levels are cumulative scenarios, so protection is derived only from
// the final level rather than duplicated per step. Hedge decomposition is not
// implemented yet and fails closed. Nothing here sends or reserves an order.
func Intents(p Plan) ([]Intent, error) {
	if p.Mode != "clean" && p.Mode != "dca" {
		return nil, ErrIntentUnsupported
	}
	if p.Side != "LONG" && p.Side != "SHORT" {
		return nil, errors.New("lado de plan inválido")
	}
	if len(p.Levels) == 0 {
		return nil, errors.New("el plan no tiene niveles")
	}
	intents := []Intent{}
	for i, level := range p.Levels {
		purpose := "REBUY"
		if i == 0 {
			purpose = "ENTRY"
		}
		if !positiveDecimal(level.Quantity) || !positiveDecimal(level.Price) {
			return nil, errors.New("nivel con cantidad o precio no positivos")
		}
		intents = append(intents, Intent{Purpose: purpose, Family: "regular", PositionSide: p.Side, Quantity: level.Quantity, Price: level.Price})
	}
	last := p.Levels[len(p.Levels)-1]
	if last.TakeProfit != nil {
		if !positiveDecimal(*last.TakeProfit) || !positiveDecimal(last.TakeProfitQuantity) {
			return nil, errors.New("TP final no positivo")
		}
		intents = append(intents, Intent{Purpose: "TP", Family: "conditional", PositionSide: p.Side, Quantity: last.TakeProfitQuantity, Price: *last.TakeProfit})
	}
	if last.StopPrice != nil {
		if !positiveDecimal(*last.StopPrice) || !positiveDecimal(last.CumulativeQuantity) {
			return nil, errors.New("SL final no positivo")
		}
		intents = append(intents, Intent{Purpose: "SL", Family: "conditional", PositionSide: p.Side, Quantity: last.CumulativeQuantity, Price: *last.StopPrice})
	}
	return intents, nil
}

// IntentClientOrderID derives a stable, exchange-safe client order ID (<= 36
// characters, [a-z0-9]) from the plan and intent ordinal. Recomputing the same
// plan produces the same IDs, which keeps future retries idempotent.
func IntentClientOrderID(planID, purpose string, ordinal int) string {
	sum := sha256.Sum256([]byte(planID + "\x00" + purpose + "\x00" + strconv.Itoa(ordinal)))
	return "rt" + hex.EncodeToString(sum[:15])
}
