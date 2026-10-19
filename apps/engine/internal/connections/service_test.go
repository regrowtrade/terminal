package connections

import (
	"context"
	"errors"
	"regrow/engine/internal/binance"
	"regrow/engine/internal/database"
	"regrow/engine/internal/vault"
	"testing"
)

type deniedStore struct{ err error }

func (s deniedStore) ListReadAccounts(context.Context, string) ([]database.ReadAccount, error) {
	return nil, s.err
}
func (s deniedStore) ReadAccountCredential(context.Context, string, string) (vault.Binding, vault.Envelope, error) {
	return vault.Binding{}, vault.Envelope{}, s.err
}
func (s deniedStore) SaveReadSnapshot(context.Context, string, string, binance.Snapshot) error {
	return s.err
}
func (s deniedStore) LatestReadSnapshot(context.Context, string, string) (database.StoredSnapshot, error) {
	return database.StoredSnapshot{}, s.err
}
func (s deniedStore) LatestReadComparison(context.Context, string, string) (database.StoredComparison, error) {
	return database.StoredComparison{}, s.err
}
func (s deniedStore) ListUserEvents(context.Context, string, string, int64, int) ([]database.StoredEvent, bool, error) {
	return nil, false, s.err
}
func (s deniedStore) LatestStreamState(context.Context, string, string) (database.StreamState, error) {
	return database.StreamState{}, s.err
}

type neverReader struct{ t *testing.T }

func (r neverReader) Snapshot(context.Context, vault.Credentials) (binance.Snapshot, error) {
	r.t.Fatal("unauthorized request reached exchange reader")
	return binance.Snapshot{}, nil
}
func TestDisabledOrUnauthorizedCannotDecryptOrRead(t *testing.T) {
	s := &Service{Store: deniedStore{err: database.ErrSessionInvalid}, Reader: neverReader{t}}
	if _, err := s.State(context.Background(), "forged"); !errors.Is(err, database.ErrSessionInvalid) {
		t.Fatal("state ignored session")
	}
	if _, err := s.Snapshot(context.Background(), "forged", "account"); !errors.Is(err, ErrDisabled) {
		t.Fatal("disabled vault accepted")
	}
	s.Vault = &vault.Keyring{}
	if _, err := s.Snapshot(context.Background(), "forged", "account"); !errors.Is(err, database.ErrSessionInvalid) {
		t.Fatal("snapshot ignored session")
	}
	s.Store = deniedStore{}
	if _, err := s.Snapshot(context.Background(), "forged", "account"); !errors.Is(err, vault.ErrVault) {
		t.Fatal("malformed envelope was accepted")
	}
}
func TestHistoricalReadsStillRequireAuthorizationWithoutVault(t *testing.T) {
	s := &Service{Store: deniedStore{err: database.ErrSessionInvalid}, Reader: neverReader{t}}
	if _, err := s.Latest(context.Background(), "forged", "account"); !errors.Is(err, database.ErrSessionInvalid) {
		t.Fatal("history bypassed session")
	}
	if _, err := (*Service)(nil).Latest(context.Background(), "forged", "account"); !errors.Is(err, ErrDisabled) {
		t.Fatal("missing store accepted")
	}
	if _, err := s.Comparison(context.Background(), "forged", "account"); !errors.Is(err, database.ErrSessionInvalid) {
		t.Fatal("comparison bypassed authorization")
	}
	if _, err := (*Service)(nil).Comparison(context.Background(), "forged", "account"); !errors.Is(err, ErrDisabled) {
		t.Fatal("comparison accepted missing store")
	}
	s.Store = deniedStore{}
	if _, err := s.Comparison(context.Background(), "synthetic", "account"); err != nil {
		t.Fatal("disabled live reader prevented local comparison")
	}
}
