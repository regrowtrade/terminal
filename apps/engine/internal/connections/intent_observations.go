package connections

import (
	"context"

	"regrow/engine/internal/database"
	"regrow/engine/internal/intentcheck"
	"regrow/engine/internal/vault"
)

// ObserveIntent is an explicit normal-order GET using only persisted criteria.
// It does not call LookupOrder on the service: the evidence and assessment must
// be committed together, with the intent rechecked after external I/O.
func (s *Service) ObserveIntent(ctx context.Context, hash, account, intent string) (database.StoredIntentObservation, error) {
	if s == nil || s.Store == nil || s.Vault == nil || s.Reader == nil {
		return database.StoredIntentObservation{}, ErrDisabled
	}
	expected, err := s.Store.ReadIntentExpected(ctx, hash, account, intent)
	if err != nil {
		return database.StoredIntentObservation{}, err
	}
	if !expected.Valid() || expected.IntentID != intent {
		return database.StoredIntentObservation{}, database.ErrPlanInvalid
	}
	binding, envelope, err := s.Store.ReadAccountCredential(ctx, hash, account)
	if err != nil {
		return database.StoredIntentObservation{}, err
	}
	if binding.AccountID != account || binding.Exchange != "binance" || binding.Environment != "production" {
		return database.StoredIntentObservation{}, vault.ErrVault
	}
	credentials, err := s.Vault.Open(binding, envelope)
	if err != nil {
		return database.StoredIntentObservation{}, err
	}
	observation, err := s.Reader.LookupOrder(ctx, credentials, expected.Query())
	if err != nil {
		return database.StoredIntentObservation{}, err
	}
	if _, err := intentcheck.Assess(expected, observation); err != nil {
		return database.StoredIntentObservation{}, err
	}
	return s.Store.SaveIntentObservation(ctx, hash, account, expected, observation)
}

func (s *Service) IntentObservations(ctx context.Context, hash, account, intent string) ([]database.StoredIntentObservation, error) {
	if s == nil || s.Store == nil {
		return nil, ErrDisabled
	}
	return s.Store.ListIntentObservations(ctx, hash, account, intent)
}
