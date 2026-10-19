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

// gateReader emits a synthetic event into the gate while the read is in flight,
// simulating a concurrent stream event that must be buffered, not dropped.
type gateReader struct {
	t       *testing.T
	gate    *SnapshotGate
	account string
}

func (r gateReader) Snapshot(context.Context, vault.Credentials) (binance.Snapshot, error) {
	if !r.gate.ingest(r.account, accountUpdate(9)) {
		r.t.Fatal("event arriving during the read was not buffered")
	}
	return binance.Snapshot{Source: "binance-production"}, nil
}

func TestSnapshotBuffersConcurrentEventsThroughGate(t *testing.T) {
	ring, _, binding, envelope := sealedAccount(t)
	store := &snapshotStore{binding: binding, envelope: envelope}
	gate := NewSnapshotGate()
	spy := &journalSpy{}
	gate.setJournal(binding.AccountID, spy.record)
	service := &Service{Store: store, Vault: ring, Reader: gateReader{t: t, gate: gate, account: binding.AccountID}, Gate: gate}
	if _, err := service.Snapshot(context.Background(), "synthetic", binding.AccountID); err != nil {
		t.Fatal(err)
	}
	if store.saves != 1 {
		t.Fatal("snapshot was not persisted")
	}
	if spy.count() != 1 || spy.times()[0] != 9 {
		t.Fatal("event arriving during the read was not flushed on commit")
	}
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
