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
