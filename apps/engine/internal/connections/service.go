package connections

import (
	"context"
	"errors"
	"regrow/engine/internal/binance"
	"regrow/engine/internal/database"
	"regrow/engine/internal/vault"
)

var ErrDisabled = errors.New("lectura Binance desactivada; requiere PostgreSQL, cifrado e IP de salida aprobada")

type State struct {
	Enabled  bool                   `json:"enabled"`
	Accounts []database.ReadAccount `json:"accounts"`
}
type Store interface {
	ListReadAccounts(context.Context, string) ([]database.ReadAccount, error)
	ReadAccountCredential(context.Context, string, string) (vault.Binding, vault.Envelope, error)
	SaveReadSnapshot(context.Context, string, string, binance.Snapshot) error
	LatestReadSnapshot(context.Context, string, string) (database.StoredSnapshot, error)
	LatestReadComparison(context.Context, string, string) (database.StoredComparison, error)
	ListUserEvents(context.Context, string, string, int64, int) ([]database.StoredEvent, bool, error)
}
type Reader interface {
	Snapshot(context.Context, vault.Credentials) (binance.Snapshot, error)
}
type Service struct {
	Store  Store
	Vault  *vault.Keyring
	Reader Reader
}

func (s *Service) State(ctx context.Context, hash string) (State, error) {
	if s == nil || s.Store == nil {
		return State{}, ErrDisabled
	}
	accounts, err := s.Store.ListReadAccounts(ctx, hash)
	return State{Enabled: s.Vault != nil && s.Reader != nil, Accounts: accounts}, err
}
func (s *Service) Snapshot(ctx context.Context, hash, account string) (binance.Snapshot, error) {
	if s == nil || s.Store == nil || s.Vault == nil || s.Reader == nil {
		return binance.Snapshot{}, ErrDisabled
	}
	binding, envelope, err := s.Store.ReadAccountCredential(ctx, hash, account)
	if err != nil {
		return binance.Snapshot{}, err
	}
	credentials, err := s.Vault.Open(binding, envelope)
	if err != nil {
		return binance.Snapshot{}, vault.ErrVault
	}
	snapshot, err := s.Reader.Snapshot(ctx, credentials)
	if err != nil {
		return binance.Snapshot{}, err
	}
	if err := s.Store.SaveReadSnapshot(ctx, hash, account, snapshot); err != nil {
		return binance.Snapshot{}, err
	}
	return snapshot, nil
}

// Historical recovery does not decrypt keys or make an external request, even
// when live reading is disabled. It still requires session/ownership checks.
func (s *Service) Latest(ctx context.Context, hash, account string) (database.StoredSnapshot, error) {
	if s == nil || s.Store == nil {
		return database.StoredSnapshot{}, ErrDisabled
	}
	return s.Store.LatestReadSnapshot(ctx, hash, account)
}

func (s *Service) Comparison(ctx context.Context, hash, account string) (database.StoredComparison, error) {
	if s == nil || s.Store == nil {
		return database.StoredComparison{}, ErrDisabled
	}
	return s.Store.LatestReadComparison(ctx, hash, account)
}

// Events returns the bounded, ascending journal of persisted private events for
// the authenticated owner. It is read-only and available with the live reader
// disabled. It never reconciles, infers fills or establishes continuity.
func (s *Service) Events(ctx context.Context, hash, account string, after int64) ([]database.StoredEvent, bool, error) {
	if s == nil || s.Store == nil {
		return nil, false, ErrDisabled
	}
	return s.Store.ListUserEvents(ctx, hash, account, after, 200)
}
