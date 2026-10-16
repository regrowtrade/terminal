package connections

import (
	"context"
	"log/slog"
	"sync"

	"regrow/engine/internal/binance"
	"regrow/engine/internal/database"
	"regrow/engine/internal/vault"
)

// Streamer runs the read-only user data stream for one credential set, invoking
// a sink for each validated event until the context is cancelled.
type Streamer interface {
	Run(context.Context, vault.Credentials, func(binance.UserEvent)) error
}

// MonitorStore is the subset of the store the background monitor needs. It runs
// under the personal identity without a browser session.
type MonitorStore interface {
	ListStreamAccounts(context.Context) ([]database.StreamAccount, error)
	SaveUserEvents(context.Context, string, []binance.UserEvent) (int, error)
}

// Monitor streams private events for every provisioned account and journals them.
// It is read-only: it never places, cancels or amends orders. Events lost during
// a disconnection or database outage are recovered later from REST snapshots.
type Monitor struct {
	Store  MonitorStore
	Vault  *vault.Keyring
	Stream Streamer
	Log    *slog.Logger
}

func (m *Monitor) Run(ctx context.Context) error {
	if m == nil || m.Store == nil || m.Vault == nil || m.Stream == nil {
		return ErrDisabled
	}
	logger := m.Log
	if logger == nil {
		logger = slog.Default()
	}
	accounts, err := m.Store.ListStreamAccounts(ctx)
	if err != nil {
		return err
	}
	var wg sync.WaitGroup
	for _, account := range accounts {
		credentials, err := m.Vault.Open(account.Binding, account.Envelope)
		if err != nil {
			logger.Error("monitor: no se pudo abrir credencial de cuenta", "account", account.ID)
			continue
		}
		wg.Add(1)
		go func(accountID string, credentials vault.Credentials) {
			defer wg.Done()
			sink := func(event binance.UserEvent) {
				if _, err := m.Store.SaveUserEvents(ctx, accountID, []binance.UserEvent{event}); err != nil {
					logger.Error("monitor: no se pudo guardar evento privado", "account", accountID, "type", event.Type, "error", err)
				}
			}
			if err := m.Stream.Run(ctx, credentials, sink); err != nil && ctx.Err() == nil {
				logger.Error("monitor: flujo de eventos detenido", "account", accountID, "error", err)
			}
		}(account.ID, credentials)
	}
	wg.Wait()
	return ctx.Err()
}
