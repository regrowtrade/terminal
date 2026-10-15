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
	return s.Reader.Snapshot(ctx, credentials)
}
