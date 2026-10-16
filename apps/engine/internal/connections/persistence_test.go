package connections

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"testing"

	"regrow/engine/internal/binance"
	"regrow/engine/internal/database"
	"regrow/engine/internal/vault"
)

type snapshotStore struct {
	binding  vault.Binding
	envelope vault.Envelope
	err      error
	saves    int
}

func (s *snapshotStore) ListReadAccounts(context.Context, string) ([]database.ReadAccount, error) {
	return []database.ReadAccount{}, nil
}
func (s *snapshotStore) ReadAccountCredential(context.Context, string, string) (vault.Binding, vault.Envelope, error) {
	return s.binding, s.envelope, nil
}
func (s *snapshotStore) SaveReadSnapshot(context.Context, string, string, binance.Snapshot) error {
	s.saves++
	return s.err
}
func (s *snapshotStore) LatestReadSnapshot(context.Context, string, string) (database.StoredSnapshot, error) {
	return database.StoredSnapshot{}, s.err
}

type snapshotReader struct {
	err   error
	calls int
}

func (r *snapshotReader) Snapshot(context.Context, vault.Credentials) (binance.Snapshot, error) {
	r.calls++
	return binance.Snapshot{Source: "binance-production"}, r.err
}

func TestSnapshotRequiresSuccessfulPersistenceAndNeverFallsBack(t *testing.T) {
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		t.Fatal("random data unavailable")
	}
	raw, _ := json.Marshal(map[string]string{"test": base64.StdEncoding.EncodeToString(key)})
	ring, err := vault.New("test", string(raw))
	if err != nil {
		t.Fatal(err)
	}
	binding := vault.Binding{OwnerID: "00000000-0000-4000-8000-000000000001", AccountID: "00000000-0000-4000-8000-000000000002", Exchange: "binance", Environment: "production"}
	if _, err := rand.Read(key); err != nil {
		t.Fatal("random data unavailable")
	}
	api := hex.EncodeToString(key)
	if _, err := rand.Read(key); err != nil {
		t.Fatal("random data unavailable")
	}
	envelope, err := ring.Seal(binding, vault.Credentials{APIKey: api, APISecret: hex.EncodeToString(key)})
	if err != nil {
		t.Fatal(err)
	}
	store := &snapshotStore{binding: binding, envelope: envelope}
	reader := &snapshotReader{}
	service := &Service{Store: store, Vault: ring, Reader: reader}
	if _, err := service.Snapshot(context.Background(), "synthetic", binding.AccountID); err != nil || store.saves != 1 {
		t.Fatal("complete read not persisted")
	}
	store.err = database.ErrSessionInvalid
	result, err := service.Snapshot(context.Background(), "synthetic", binding.AccountID)
	if !errors.Is(err, database.ErrSessionInvalid) || result.Source != "" {
		t.Fatal("save failure exposed current/old result")
	}
	before := store.saves
	reader.err = binance.ErrUnavailable
	if _, err := service.Snapshot(context.Background(), "synthetic", binding.AccountID); !errors.Is(err, binance.ErrUnavailable) || store.saves != before {
		t.Fatal("failed read saved or fell back to history")
	}
	readerCalls := reader.calls
	service.Vault = nil
	if _, err := service.Latest(context.Background(), "synthetic", binding.AccountID); !errors.Is(err, database.ErrSessionInvalid) || reader.calls != readerCalls {
		t.Fatal("history contacted exchange or ignored authorization")
	}
}
