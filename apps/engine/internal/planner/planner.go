package planner

import (
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/shopspring/decimal"
)

var numeric = regexp.MustCompile(`^-?(?:0|[1-9][0-9]{0,15})(?:\.[0-9]{1,12})?$`)
var symbolPattern = regexp.MustCompile(`^[A-Z0-9]{2,20}USDT$`)
var hundred = decimal.NewFromInt(100)
var one = decimal.NewFromInt(1)

func amount(value, name string, positive bool) (decimal.Decimal, error) {
	if !numeric.MatchString(value) {
		return decimal.Zero, fmt.Errorf("%s: decimal inválido", name)
	}
	d, err := decimal.NewFromString(value)
	if err != nil || (positive && !d.IsPositive()) {
		return decimal.Zero, fmt.Errorf("%s: debe ser mayor que cero", name)
	}
	return d, nil
}

func nonnegative(value, name string) (decimal.Decimal, error) {
	d, err := amount(value, name, false)
	if err == nil && d.IsNegative() {
		err = fmt.Errorf("%s: no puede ser negativo", name)
	}
	return d, err
}

func ptr(d decimal.Decimal) *string { s := d.String(); return &s }

// Preview never sends orders. Prices are unquantized estimates, not executable instructions.
func Preview(r Request) (Plan, error) {
	if !symbolPattern.MatchString(r.Symbol) || strings.Contains(r.Symbol, " ") {
		return Plan{}, errors.New("símbolo USDT inválido")
	}
	if r.Side != "LONG" && r.Side != "SHORT" {
		return Plan{}, errors.New("lado inválido")
	}
	if r.Leverage < 1 || r.Leverage > 125 {
		return Plan{}, errors.New("apalancamiento fuera de rango")
	}
	if r.MaxLevels < 1 || r.MaxLevels > 11 {
		return Plan{}, errors.New("niveles: entre 1 y 11")
	}
	risk, err := amount(r.Risk, "riesgo", true)
	if err != nil {
		return Plan{}, err
	}
	if r.RiskUnit != "amount" && r.RiskUnit != "percent" {
		return Plan{}, errors.New("unidad de riesgo inválida")
	}
	if r.AccountBasis != "equity" && r.AccountBasis != "wallet" {
		return Plan{}, errors.New("base de cuenta inválida")
	}
	account, err := amount(r.AccountReference, "capital de referencia", true)
	if err != nil {
		return Plan{}, err
	}
	if r.RiskUnit == "percent" {
		if risk.GreaterThan(hundred) {
			return Plan{}, errors.New("riesgo porcentual mayor que 100 %")
		}
		risk = account.Mul(risk).Shift(-2)
	}
	reserve, err := nonnegative(r.CostReserve, "reserva de costes")
	if err != nil {
		return Plan{}, err
	}
	if !reserve.LessThan(risk) {
		return Plan{}, errors.New("la reserva debe ser menor que el riesgo")
	}
	entry, err := amount(r.Entry, "precio inicial", true)
	if err != nil {
		return Plan{}, err
	}
	p := Plan{Mode: r.Mode, Symbol: r.Symbol, Side: r.Side, Simulation: true, Risk: risk.String(), RiskInput: r.Risk, RiskUnit: r.RiskUnit, AccountReference: account.String(), AccountBasis: r.AccountBasis, CostReserve: reserve.String(), Levels: []Level{}, Warnings: []string{
		"Simulación: no coloca órdenes ni consulta el exchange.",
		"Precios sin ajustar a tickSize/stepSize. No se valida liquidación ni liquidez.",
		"La reserva de costes es una estimación manual; un SL no garantiza una pérdida exacta.",
	}}
	if r.Mode == "hedge" {
		return hedgePlan(r, p, entry, risk, reserve)
	}
	if r.Mode != "clean" && r.Mode != "dca" {
		return Plan{}, errors.New("modo inválido")
	}
	if r.Hedge != nil {
		return Plan{}, errors.New("datos de cobertura incompatibles con este modo")
	}
	qty, err := amount(r.Quantity, "cantidad", true)
	if err != nil {
		return Plan{}, err
	}
	ratio, err := amount(r.RewardRatio, "beneficio/riesgo", true)
	if err != nil {
		return Plan{}, err
	}
	if ratio.GreaterThan(decimal.NewFromInt(20)) {
		return Plan{}, errors.New("beneficio/riesgo máximo: 20")
	}
	maxMargin, err := amount(r.MaxMargin, "margen máximo", true)
	if err != nil {
		return Plan{}, err
	}
	spacing, err := amount(r.SpacingPercent, "separación", true)
	if err != nil {
		return Plan{}, err
	}
	if !spacing.LessThan(hundred) {
		return Plan{}, errors.New("separación: menor de 100 %")
	}
	p.SpacingPercent = spacing.String()
	factor := one.Add(spacing.Shift(-2))
	if r.Side == "LONG" {
		factor = one.Sub(spacing.Shift(-2))
	}
	limit := r.MaxLevels
	if r.Mode == "clean" {
		limit = 1
		p.SpacingPercent = "0"
	}
	total, value := decimal.Zero, decimal.Zero
	price, currentQty := entry, qty
	for i := 0; i < limit; i++ {
		if i > 1 {
			currentQty = currentQty.Mul(decimal.NewFromInt(2))
		}
		candidateTotal := total.Add(currentQty)
		candidateValue := value.Add(price.Mul(currentQty))
		avg := candidateValue.DivRound(candidateTotal, 32)
		margin := candidateValue.DivRound(decimal.NewFromInt(int64(r.Leverage)), 32)
		stop, tp := directionalPrices(r.Side, avg, candidateTotal, risk, ratio, reserve)
		if !stop.IsPositive() || !tp.IsPositive() {
			p.StopReason = "El siguiente nivel produce un SL o TP no válido."
			break
		}
		if margin.GreaterThan(maxMargin) {
			p.StopReason = "Límite de margen alcanzado."
			break
		}
		if r.Mode == "dca" && ((r.Side == "LONG" && !price.GreaterThan(stop)) || (r.Side == "SHORT" && !price.LessThan(stop))) {
			p.StopReason = "El siguiente nivel queda fuera del SL permitido por el riesgo."
			break
		}
		total, value = candidateTotal, candidateValue
		net := total
		if r.Side == "SHORT" {
			net = net.Neg()
		}
		p.Levels = append(p.Levels, Level{Index: i + 1, Price: price.String(), Quantity: currentQty.String(), CumulativeQuantity: total.String(), AveragePrice: avg.String(), Margin: margin.String(), StopPrice: ptr(stop), TakeProfit: ptr(tp), TakeProfitQuantity: total.String(), NetExposure: net.String()})
		price = price.Mul(factor)
	}
	if len(p.Levels) == 0 {
		return Plan{}, errors.New("no existe un plan válido con ese margen, cantidad y riesgo")
	}
	if p.StopReason == "" {
		p.StopReason = "Número máximo de niveles solicitado alcanzado."
	}
	if r.Mode == "dca" {
		p.Warnings = append(p.Warnings, "Los niveles son escenarios acumulados. La protección debe actualizarse también con ejecuciones parciales.")
	}
	return p, nil
}

func directionalPrices(side string, avg, qty, risk, ratio, reserve decimal.Decimal) (decimal.Decimal, decimal.Decimal) {
	lossDistance := risk.Sub(reserve).DivRound(qty, 32)
	profitDistance := risk.Mul(ratio).Add(reserve).DivRound(qty, 32)
	if side == "LONG" {
		return avg.Sub(lossDistance), avg.Add(profitDistance)
	}
	return avg.Add(lossDistance), avg.Sub(profitDistance)
}

type Exposure struct {
	LongEntry, ShortEntry, LongQuantity, ShortQuantity decimal.Decimal
	RealizedNet                                        decimal.Decimal
}

func (e Exposure) Pnl(price decimal.Decimal) decimal.Decimal {
	return e.RealizedNet.Add(price.Sub(e.LongEntry).Mul(e.LongQuantity)).Add(e.ShortEntry.Sub(price).Mul(e.ShortQuantity))
}

// Close supports partial fills and preserves the original exposure on validation failure.
func (e Exposure) Close(side string, qty, price, fee decimal.Decimal) (Exposure, error) {
	if !qty.IsPositive() || !price.IsPositive() || fee.IsNegative() {
		return e, errors.New("ejecución de cierre inválida")
	}
	switch side {
	case "LONG":
		if qty.GreaterThan(e.LongQuantity) {
			return e, errors.New("cierre mayor que la posición LONG")
		}
		e.LongQuantity = e.LongQuantity.Sub(qty)
		e.RealizedNet = e.RealizedNet.Add(price.Sub(e.LongEntry).Mul(qty)).Sub(fee)
	case "SHORT":
		if qty.GreaterThan(e.ShortQuantity) {
			return e, errors.New("cierre mayor que la posición SHORT")
		}
		e.ShortQuantity = e.ShortQuantity.Sub(qty)
		e.RealizedNet = e.RealizedNet.Add(e.ShortEntry.Sub(price).Mul(qty)).Sub(fee)
	default:
		return e, errors.New("lado de cierre inválido")
	}
	return e, nil
}

// StopPrice solves combined net PnL, including the manual future closing-cost reserve.
// A balanced exposure has no directional price stop; it must not divide by zero.
func (e Exposure) StopPrice(threshold, reserve decimal.Decimal) (*string, error) {
	net := e.LongQuantity.Sub(e.ShortQuantity)
	if net.IsZero() {
		return nil, nil
	}
	constant := e.RealizedNet.Sub(e.LongEntry.Mul(e.LongQuantity)).Add(e.ShortEntry.Mul(e.ShortQuantity)).Sub(reserve)
	price := threshold.Sub(constant).DivRound(net, 32)
	if !price.IsPositive() {
		return nil, errors.New("no existe un SL positivo para esta exposición y riesgo")
	}
	return ptr(price), nil
}

func hedgePlan(r Request, p Plan, entry, risk, reserve decimal.Decimal) (Plan, error) {
	if r.Hedge == nil {
		return Plan{}, errors.New("faltan datos de cobertura")
	}
	h := r.Hedge
	le, err := amount(h.LongEntry, "entrada LONG", true)
	if err != nil {
		return Plan{}, err
	}
	se, err := amount(h.ShortEntry, "entrada SHORT", true)
	if err != nil {
		return Plan{}, err
	}
	lq, err := amount(h.LongQuantity, "cantidad LONG", true)
	if err != nil {
		return Plan{}, err
	}
	sq, err := amount(h.ShortQuantity, "cantidad SHORT", true)
	if err != nil {
		return Plan{}, err
	}
	mark, err := amount(h.MarkPrice, "mark price", true)
	if err != nil {
		return Plan{}, err
	}
	realized, err := amount(h.RealizedPnl, "PnL realizado", false)
	if err != nil {
		return Plan{}, err
	}
	fees, err := nonnegative(h.FeesPaid, "comisiones")
	if err != nil {
		return Plan{}, err
	}
	funding, err := amount(h.FundingNet, "funding", false)
	if err != nil {
		return Plan{}, err
	}
	pct, err := amount(h.ClosePercent, "porcentaje de cierre", true)
	if err != nil {
		return Plan{}, err
	}
	if !pct.Equal(decimal.NewFromInt(5)) && !pct.Equal(decimal.NewFromInt(10)) {
		return Plan{}, errors.New("cierre inicial: 5 % o 10 %")
	}
	maxEntry := decimal.Max(le, se)
	distance := le.Sub(se).Abs().DivRound(maxEntry, 32).Mul(hundred)
	multiplier := decimal.RequireFromString("1.5")
	if pct.Equal(decimal.NewFromInt(10)) {
		multiplier = decimal.RequireFromString("1.2")
	}
	spacing := distance.Mul(multiplier)
	if !spacing.IsPositive() || !spacing.LessThan(hundred) {
		return Plan{}, errors.New("la distancia de cobertura debe producir una separación entre 0 y 100 %")
	}
	p.SpacingPercent = spacing.String()
	exposure := Exposure{LongEntry: le, ShortEntry: se, LongQuantity: lq, ShortQuantity: sq, RealizedNet: realized.Sub(fees).Add(funding)}
	baseline := exposure.Pnl(mark)
	threshold := baseline.Sub(risk)
	p.BaselinePnl, p.PnlThreshold = ptr(baseline), ptr(threshold)
	available := lq
	if r.Side == "SHORT" {
		available = sq
	}
	qty := available.Mul(pct).Shift(-2)
	total, value := decimal.Zero, decimal.Zero
	price := entry
	factor := one.Add(spacing.Shift(-2))
	if r.Side == "SHORT" {
		factor = one.Sub(spacing.Shift(-2))
	}
	for i := 0; i < r.MaxLevels; i++ {
		if i > 1 {
			qty = qty.Mul(decimal.NewFromInt(2))
		}
		if total.Add(qty).GreaterThan(available) {
			p.StopReason = "El siguiente nivel excede las monedas del lado seleccionado."
			break
		}
		exposure, err = exposure.Close(r.Side, qty, price, decimal.Zero)
		if err != nil {
			return Plan{}, err
		}
		total = total.Add(qty)
		value = value.Add(price.Mul(qty))
		avg := value.DivRound(total, 32)
		stop, err := exposure.StopPrice(threshold, reserve)
		if err != nil {
			if len(p.Levels) == 0 {
				return Plan{}, err
			}
			p.StopReason = "El siguiente escenario no permite un SL positivo."
			break
		}
		if exposure.Pnl(price).Sub(reserve).LessThanOrEqual(threshold) {
			if len(p.Levels) == 0 {
				return Plan{}, errors.New("el escenario de cierre ya alcanza el umbral de pérdida")
			}
			p.StopReason = "El siguiente escenario alcanza el umbral de pérdida autorizado."
			break
		}
		net := exposure.LongQuantity.Sub(exposure.ShortQuantity)
		if stop != nil {
			s, _ := decimal.NewFromString(*stop)
			if (net.IsPositive() && !s.LessThan(price)) || (net.IsNegative() && !s.GreaterThan(price)) {
				if len(p.Levels) == 0 {
					return Plan{}, errors.New("SL incompatible con la exposición del escenario")
				}
				p.StopReason = "El siguiente escenario tiene un SL incompatible con la exposición."
				break
			}
		}
		tp := avg.Mul(one.Sub(spacing.Shift(-2)))
		if r.Side == "SHORT" {
			tp = avg.Mul(one.Add(spacing.Shift(-2)))
		}
		opposite := exposure.ShortQuantity
		if r.Side == "SHORT" {
			opposite = exposure.LongQuantity
		}
		p.Levels = append(p.Levels, Level{Index: i + 1, Price: price.String(), Quantity: qty.String(), CumulativeQuantity: total.String(), AveragePrice: avg.String(), Margin: "0", StopPrice: stop, TakeProfit: ptr(tp), TakeProfitQuantity: decimal.Min(total, opposite).String(), NetExposure: net.String()})
		price = price.Mul(factor)
	}
	if p.StopReason == "" {
		p.StopReason = "Número máximo de niveles solicitado alcanzado."
	}
	p.Warnings = append(p.Warnings, "SL y TP son escenarios hipotéticos por nivel completo; no se garantiza protección de una cobertura real.", "El TP compensa monedas del lado contrario; completar el ciclo no implica cerrar todo el activo.")
	return p, nil
}
