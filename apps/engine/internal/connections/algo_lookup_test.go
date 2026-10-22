package connections

import (
	"context"
	"errors"
	"regrow/engine/internal/binance"
	"regrow/engine/internal/database"
	"regrow/engine/internal/vault"
	"testing"
	"time"
)

func (s deniedStore) SaveAlgoObservation(context.Context, string, string, binance.AlgoLookup) error {
	return s.err
}
func (s *snapshotStore) SaveAlgoObservation(context.Context, string, string, binance.AlgoLookup) error {
	s.saves++
	return s.err
}
func (r *snapshotReader) LookupAlgoOrder(context.Context, vault.Credentials, binance.AlgoQuery) (binance.AlgoLookup, error) {
	r.calls++
	return binance.AlgoLookup{}, r.err
}
func (r *fillReader) LookupAlgoOrder(context.Context, vault.Credentials, binance.AlgoQuery) (binance.AlgoLookup, error) {
	return binance.AlgoLookup{}, r.err
}
func (r neverReader) LookupAlgoOrder(context.Context, vault.Credentials, binance.AlgoQuery) (binance.AlgoLookup, error) {
	r.t.Fatal("unauthorized algo lookup")
	return binance.AlgoLookup{}, nil
}
func (r gateReader) LookupAlgoOrder(context.Context, vault.Credentials, binance.AlgoQuery) (binance.AlgoLookup, error) {
	r.t.Fatal("unexpected algo lookup")
	return binance.AlgoLookup{}, nil
}

type algoReader struct {
	*snapshotReader
	result binance.AlgoLookup
	query  binance.AlgoQuery
}

func (r *algoReader) LookupAlgoOrder(_ context.Context, _ vault.Credentials, q binance.AlgoQuery) (binance.AlgoLookup, error) {
	r.calls++
	r.query = q
	return r.result, r.err
}

func TestAlgoLookupRequiresBoundCredentialValidatedResultAndDurableSave(t *testing.T) {
	ring, _, binding, envelope := sealedAccount(t)
	query := binance.AlgoQuery{Symbol: "BTCUSDT", AlgoID: "9007199254740993"}
	now := time.Now().UTC()
	valid := binance.AlgoLookup{Source: "binance-production", Mode: "read_only", Family: "algo", StartedAt: now, CheckedAt: now, Query: query, Outcome: "unresolved", Reason: "exchange_not_found"}
	for _, tc := range []struct {
		name         string
		mutate       func(*Service, *snapshotStore, *algoReader)
		want         error
		calls, saves int
	}{
		{name: "persist unresolved", calls: 1, saves: 1},
		{name: "save revoked", mutate: func(_ *Service, s *snapshotStore, _ *algoReader) { s.err = database.ErrSessionInvalid }, want: database.ErrSessionInvalid, calls: 1, saves: 1},
		{name: "save failure", mutate: func(_ *Service, s *snapshotStore, _ *algoReader) { s.err = errors.New("audit failed") }, calls: 1, saves: 1},
		{name: "reader failure", mutate: func(_ *Service, _ *snapshotStore, r *algoReader) { r.err = binance.ErrUnavailable }, want: binance.ErrUnavailable, calls: 1},
		{name: "other selector", mutate: func(_ *Service, _ *snapshotStore, r *algoReader) { r.result.Query.AlgoID = "123" }, want: binance.ErrResponse, calls: 1},
		{name: "other symbol", mutate: func(_ *Service, _ *snapshotStore, r *algoReader) { r.result.Query.Symbol = "ETHUSDT" }, want: binance.ErrResponse, calls: 1},
		{name: "invalid evidence", mutate: func(_ *Service, _ *snapshotStore, r *algoReader) { r.result.ExecutionReady = true }, want: binance.ErrResponse, calls: 1},
		{name: "disabled", mutate: func(s *Service, _ *snapshotStore, _ *algoReader) { s.Reader = nil }, want: ErrDisabled},
		{name: "corrupt envelope", mutate: func(_ *Service, s *snapshotStore, _ *algoReader) { s.envelope = vault.Envelope{} }, want: vault.ErrVault},
		{name: "wrong account binding", mutate: func(_ *Service, s *snapshotStore, _ *algoReader) { s.binding.AccountID = "other" }, want: vault.ErrVault},
		{name: "wrong environment", mutate: func(_ *Service, s *snapshotStore, _ *algoReader) { s.binding.Environment = "testnet" }, want: vault.ErrVault},
		{name: "wrong exchange", mutate: func(_ *Service, s *snapshotStore, _ *algoReader) { s.binding.Exchange = "bybit" }, want: vault.ErrVault},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store := &snapshotStore{binding: binding, envelope: envelope}
			reader := &algoReader{snapshotReader: &snapshotReader{}, result: valid}
			s := &Service{Store: store, Vault: ring, Reader: reader}
			if tc.mutate != nil {
				tc.mutate(s, store, reader)
			}
			result, err := s.LookupAlgoOrder(context.Background(), "synthetic", binding.AccountID, query)
			if tc.name == "save failure" {
				if err == nil {
					t.Fatal("audit failure ignored")
				}
			} else if !errors.Is(err, tc.want) {
				t.Fatal("wrong error", err)
			}
			if reader.calls != tc.calls || store.saves != tc.saves || (err != nil && result.Source != "") {
				t.Fatal("unsafe read/save/partial", reader.calls, store.saves, result)
			}
			if reader.calls > 0 && reader.query != query {
				t.Fatal("selector changed")
			}
		})
	}
	for _, denial := range []error{database.ErrSessionInvalid, database.ErrAccountMissing} {
		s := &Service{Store: deniedStore{err: denial}, Vault: ring, Reader: neverReader{t}}
		if _, err := s.LookupAlgoOrder(context.Background(), "synthetic", binding.AccountID, query); !errors.Is(err, denial) {
			t.Fatal("ownership bypassed", err)
		}
	}
	if _, err := (&Service{}).LookupAlgoOrder(context.Background(), "synthetic", binding.AccountID, binance.AlgoQuery{}); !errors.Is(err, binance.ErrAlgoQuery) {
		t.Fatal("invalid query accepted", err)
	}
}
