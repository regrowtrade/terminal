package connections

import (
	"context"
	"regrow/engine/internal/binance"
	"regrow/engine/internal/vault"
)

// A separate evidence family: never falls back to /order, follows a child,
// resets continuity, changes a snapshot, or associates a strategy intention.
func (s *Service) LookupAlgoOrder(ctx context.Context, hash, account string, query binance.AlgoQuery) (binance.AlgoLookup, error) {
	if !query.Valid() {
		return binance.AlgoLookup{}, binance.ErrAlgoQuery
	}
	if s == nil || s.Store == nil || s.Vault == nil || s.Reader == nil {
		return binance.AlgoLookup{}, ErrDisabled
	}
	binding, envelope, err := s.Store.ReadAccountCredential(ctx, hash, account)
	if err != nil {
		return binance.AlgoLookup{}, err
	}
	if binding.AccountID != account || binding.Exchange != "binance" || binding.Environment != "production" {
		return binance.AlgoLookup{}, vault.ErrVault
	}
	credentials, err := s.Vault.Open(binding, envelope)
	if err != nil {
		return binance.AlgoLookup{}, vault.ErrVault
	}
	result, err := s.Reader.LookupAlgoOrder(ctx, credentials, query)
	if err != nil {
		return binance.AlgoLookup{}, err
	}
	if result.Query != query || binance.ValidateAlgoLookup(result) != nil {
		return binance.AlgoLookup{}, binance.ErrResponse
	}
	if err := s.Store.SaveAlgoObservation(ctx, hash, account, result); err != nil {
		return binance.AlgoLookup{}, err
	}
	return result, nil
}
