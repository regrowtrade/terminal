package observations

import (
	"regexp"
	"time"
)

var decimalText = regexp.MustCompile(`^-?(?:0|[1-9][0-9]{0,37})(?:\.[0-9]{1,18})?$`)
var fields = map[string]bool{
	"clientOrderId": true, "clientAlgoId": true, "side": true, "positionSide": true, "type": true, "status": true,
	"reduceOnly": true, "closePosition": true, "workingType": true, "algoType": true, "orderType": true, "algoStatus": true,
	"contractType": true, "baseAsset": true, "quoteAsset": true, "marginAsset": true, "maxNumOrders": true, "maxNumAlgoOrders": true, "unrecognizedFilterTypes": true,
}

func ValidateReport(r Report) error {
	if r.SchemaVersion != 1 || r.Mode != "rest_observation_comparison" || r.Continuity != "not_established" || r.ExecutionReady ||
		r.CurrentStartedAt.IsZero() || r.CurrentCapturedAt.Before(r.CurrentStartedAt) || r.CurrentCapturedAt.Sub(r.CurrentStartedAt) > 30*time.Second ||
		r.DetailLimit != DetailLimit || r.TotalChanges < 0 || r.TotalChanges > 82000 || r.Changes == nil || len(r.Changes) > DetailLimit ||
		r.TotalChanges < len(r.Changes) || r.Truncated != (r.TotalChanges > len(r.Changes)) ||
		(!r.Truncated && r.TotalChanges != len(r.Changes)) || (r.Truncated && len(r.Changes) != DetailLimit) {
		return ErrInvalid
	}
	if r.Status == "initial_observation" {
		if r.PreviousCapturedAt != nil || r.IntervalRelation != "no_baseline" || r.TotalChanges != 0 {
			return ErrInvalid
		}
	} else if r.Status == "compared" {
		if r.PreviousCapturedAt == nil || r.PreviousCapturedAt.IsZero() || !r.CurrentCapturedAt.After(*r.PreviousCapturedAt) {
			return ErrInvalid
		}
		relation := "disjoint"
		if r.CurrentStartedAt.Before(*r.PreviousCapturedAt) {
			relation = "overlapping"
		}
		if r.IntervalRelation != relation {
			return ErrInvalid
		}
	} else {
		return ErrInvalid
	}
	seen := map[string]bool{}
	for _, change := range r.Changes {
		known := false
		for _, entity := range entities {
			known = known || change.Entity == entity
		}
		key := change.Entity + ":" + change.Key
		if !known || change.Key == "" || len(change.Key) > 80 || seen[key] || len(change.Fields) == 0 || len(change.Fields) > 32 ||
			(change.Kind != "observed" && change.Kind != "not_observed" && change.Kind != "changed") {
			return ErrInvalid
		}
		seen[key] = true
		seenFields := map[string]bool{}
		for _, field := range change.Fields {
			if (!numericFields[field.Field] && !fields[field.Field]) || seenFields[field.Field] || (field.Before == nil && field.After == nil) ||
				(change.Kind == "observed" && (field.Before != nil || field.After == nil)) || (change.Kind == "not_observed" && (field.Before == nil || field.After != nil)) {
				return ErrInvalid
			}
			seenFields[field.Field] = true
			for _, value := range []*string{field.Before, field.After} {
				if value != nil && (len(*value) > 4096 || (numericFields[field.Field] && !decimalText.MatchString(*value))) {
					return ErrInvalid
				}
			}
		}
	}
	return nil
}
