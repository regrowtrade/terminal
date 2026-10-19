package connections

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"regrow/engine/internal/binance"
	"regrow/engine/internal/database"
	"regrow/engine/internal/vault"
)

type lookupReader struct {
	*snapshotReader
	result binance.OrderLookup
	query  binance.OrderQuery
}

func (r *lookupReader) LookupOrder(_ context.Context, _ vault.Credentials, q binance.OrderQuery) (binance.OrderLookup, error) {
	r.calls++
	r.query = q
	return r.result, r.err
}

func TestLookupRequiresAuthorizationValidationAndDurableSave(t *testing.T) {
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(map[string]string{"test": base64.StdEncoding.EncodeToString(key)})
	ring, err := vault.New("test", string(raw))
	if err != nil {
		t.Fatal(err)
	}
	binding := vault.Binding{OwnerID: "00000000-0000-4000-8000-000000000001", AccountID: "00000000-0000-4000-8000-000000000002", Exchange: "binance", Environment: "production"}
	// Ephemeral synthetic values, never live exchange keys.
	envelope, err := ring.Seal(binding, vault.Credentials{APIKey: hex.EncodeToString(key), APISecret: hex.EncodeToString(key)})
	if err != nil {
		t.Fatal(err)
	}
	query := binance.OrderQuery{Symbol: "BTCUSDT", OrderID: "9007199254740993"}
	valid := binance.OrderLookup{Source: "binance-production", Mode: "read_only", StartedAt: time.Now().UTC(), CheckedAt: time.Now().UTC(), Query: query, Outcome: "unresolved", Reason: "exchange_not_found"}
	for _, tc := range []struct {
		name             string
		saveErr, readErr error
		mutate           func(*Service, *lookupReader)
		want             error
		saves, calls     int
	}{
		{name: "persist unresolved", saves: 1, calls: 1},
		{name: "save failure", saveErr: database.ErrSessionInvalid, want: database.ErrSessionInvalid, saves: 1, calls: 1},
		{name: "reader failure", readErr: binance.ErrUnavailable, want: binance.ErrUnavailable, calls: 1},
		{name: "mismatch", mutate: func(_ *Service, r *lookupReader) { r.result.Query.OrderID = "123" }, want: binance.ErrResponse, calls: 1},
		{name: "invalid result", mutate: func(_ *Service, r *lookupReader) { r.result.ExecutionReady = true }, want: binance.ErrResponse, calls: 1},
		{name: "disabled", mutate: func(s *Service, _ *lookupReader) { s.Vault = nil }, want: ErrDisabled},
		{name: "corrupt envelope", mutate: func(s *Service, _ *lookupReader) { s.Store.(*snapshotStore).envelope = vault.Envelope{} }, want: vault.ErrVault},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store := &snapshotStore{binding: binding, envelope: envelope, err: tc.saveErr}
			reader := &lookupReader{snapshotReader: &snapshotReader{err: tc.readErr}, result: valid}
			service := &Service{Store: store, Vault: ring, Reader: reader}
			if tc.mutate != nil {
				tc.mutate(service, reader)
			}
			result, err := service.LookupOrder(context.Background(), "synthetic", binding.AccountID, query)
			if !errors.Is(err, tc.want) || store.saves != tc.saves || reader.calls != tc.calls {
				t.Fatal("lookup did not enforce boundary", err, store.saves, reader.calls)
			}
			if err != nil && result.Source != "" {
				t.Fatal("failed lookup returned partial observation")
			}
			if tc.calls > 0 && reader.query != query {
				t.Fatal("reader used different selector")
			}
		})
	}
	for _, denial := range []error{database.ErrSessionInvalid, database.ErrAccountMissing} {
		service := &Service{Store: deniedStore{err: denial}, Vault: ring, Reader: neverReader{t}}
		if _, err := service.LookupOrder(context.Background(), "synthetic", binding.AccountID, query); !errors.Is(err, denial) {
			t.Fatal("credential ownership bypassed", err)
		}
	}
	service := &Service{Store: deniedStore{}, Vault: ring, Reader: neverReader{t}}
	if _, err := service.LookupOrder(context.Background(), "synthetic", binding.AccountID, binance.OrderQuery{}); !errors.Is(err, binance.ErrOrderQuery) {
		t.Fatal("invalid query accepted")
	}
}
