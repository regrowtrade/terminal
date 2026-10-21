package connections

import (
	"context"
	"errors"
	"testing"

	"regrow/engine/internal/binance"
	"regrow/engine/internal/database"
	"regrow/engine/internal/vault"
)

type fillStore struct {
	*snapshotStore
	credentialErr error
	saveErr       error
	savedCount    int
	saveResult    database.RecoveredFillsResult
}

func (s *fillStore) ReadAccountCredential(ctx context.Context, hash, account string) (vault.Binding, vault.Envelope, error) {
	if s.credentialErr != nil {
		return vault.Binding{}, vault.Envelope{}, s.credentialErr
	}
	return s.snapshotStore.ReadAccountCredential(ctx, hash, account)
}

func (s *fillStore) SaveRecoveredFills(_ context.Context, _, _ string, fills []binance.Fill) (database.RecoveredFillsResult, error) {
	s.savedCount = len(fills)
	if s.saveErr != nil {
		return database.RecoveredFillsResult{}, s.saveErr
	}
	return s.saveResult, nil
}

type fillReader struct {
	fills     []binance.Fill
	truncated bool
	err       error
	calls     int
	query     binance.TradeQuery
}

func (r *fillReader) Snapshot(context.Context, vault.Credentials) (binance.Snapshot, error) {
	return binance.Snapshot{}, r.err
}
func (r *fillReader) LookupOrder(context.Context, vault.Credentials, binance.OrderQuery) (binance.OrderLookup, error) {
	return binance.OrderLookup{}, r.err
}
func (r *fillReader) RecoverTrades(_ context.Context, _ vault.Credentials, q binance.TradeQuery) ([]binance.Fill, bool, error) {
	r.calls++
	r.query = q
	return r.fills, r.truncated, r.err
}

func TestRecoverFillsDecryptsReadsAndPersists(t *testing.T) {
	ring, _, binding, envelope := sealedAccount(t)
	query := binance.TradeQuery{Symbol: "BTCUSDT", StartTime: 1700000000000, EndTime: 1700000086400}
	one := binance.Fill{OrderID: "9007199254741111", TradeID: "22222222", Symbol: "BTCUSDT", Side: "BUY", PositionSide: "LONG", Price: "100", Quantity: "1", Commission: "-0.1", CommissionAsset: "USDT", EventTime: 1700000086000, TransactionTime: 1700000086000}
	// An invalid query never reaches the store, vault or exchange.
	empty := &Service{Store: &fillStore{snapshotStore: &snapshotStore{binding: binding, envelope: envelope}}, Vault: ring, Reader: &fillReader{fills: []binance.Fill{one}}}
	if _, err := empty.RecoverFills(context.Background(), "synthetic", binding.AccountID, binance.TradeQuery{}); !errors.Is(err, binance.ErrTradeQuery) {
		t.Fatal("invalid trade query accepted")
	}
	for _, tc := range []struct {
		name          string
		mutate        func(*Service, *fillStore, *fillReader)
		want          error
		wantCalls     int
		wantRecovered int
	}{
		{name: "recovered and truncated", wantCalls: 1, wantRecovered: 1},
		{name: "reader error", mutate: func(_ *Service, _ *fillStore, r *fillReader) { r.err = binance.ErrUnavailable }, want: binance.ErrUnavailable, wantCalls: 1},
		{name: "credential removed", mutate: func(_ *Service, s *fillStore, _ *fillReader) { s.credentialErr = database.ErrAccountMissing }, want: database.ErrAccountMissing},
		{name: "wrong account", mutate: func(_ *Service, s *fillStore, _ *fillReader) { s.binding.AccountID = query.Symbol }, want: vault.ErrVault},
		{name: "corrupt credential", mutate: func(_ *Service, s *fillStore, _ *fillReader) { s.envelope = vault.Envelope{} }, want: vault.ErrVault},
		{name: "revoked while saving", mutate: func(_ *Service, s *fillStore, _ *fillReader) { s.saveErr = database.ErrSessionInvalid }, want: database.ErrSessionInvalid, wantCalls: 1},
		{name: "disabled", mutate: func(s *Service, _ *fillStore, _ *fillReader) { s.Reader = nil }, want: ErrDisabled},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store := &fillStore{snapshotStore: &snapshotStore{binding: binding, envelope: envelope}, saveResult: database.RecoveredFillsResult{Recovered: 1}}
			reader := &fillReader{fills: []binance.Fill{one}, truncated: true}
			service := &Service{Store: store, Vault: ring, Reader: reader}
			if tc.mutate != nil {
				tc.mutate(service, store, reader)
			}
			result, err := service.RecoverFills(context.Background(), "synthetic", binding.AccountID, query)
			if !errors.Is(err, tc.want) || reader.calls != tc.wantCalls {
				t.Fatal("recovery crossed boundary", err, reader.calls)
			}
			if err == nil {
				if result.Recovered != tc.wantRecovered || !result.Truncated || result.ExecutionReady || result.Source != "binance-production" || result.Mode != "read_only" {
					t.Fatal("summary lost semantics", result)
				}
				if store.savedCount != 1 || reader.query != query {
					t.Fatal("fill recovery did not forward stored query or fills")
				}
			}
		})
	}
}
