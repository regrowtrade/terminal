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
// a sink for each validated event and a status callback on connection changes.
type Streamer interface {
	Run(context.Context, vault.Credentials, func(binance.UserEvent)) error
	SetStatus(func(connected bool))
}

// MonitorStore is the subset of the store the background monitor needs. It runs
// under the personal identity without a browser session.
type MonitorStore interface {
	ListStreamAccounts(context.Context) ([]database.StreamAccount, error)
	SaveUserEvents(context.Context, string, []binance.UserEvent) (int, error)
	SetStreamConnected(context.Context, string, bool) error
	RecordStreamEvent(context.Context, string, int64) error
}

// Monitor streams private events for every provisioned account and journals them
// together with durable continuity state. It is read-only: it never places,
// cancels or amends orders. Events lost during a disconnection or database
// outage are recovered later from REST snapshots.
type Monitor struct {
	Store  MonitorStore
	Vault  *vault.Keyring
	Stream Streamer
	Log    *slog.Logger
	// Gate buffers events while a snapshot read is in flight. Optional: a nil
	// gate journals every event immediately.
	Gate *SnapshotGate
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
			stream := m.Stream
			stream.SetStatus(func(connected bool) {
				if err := m.Store.SetStreamConnected(ctx, accountID, connected); err != nil {
					logger.Error("monitor: no se pudo registrar estado de conexión", "account", accountID, "error", err)
				}
			})
			journal := func(events []binance.UserEvent) {
				if _, err := m.Store.SaveUserEvents(ctx, accountID, events); err != nil {
					logger.Error("monitor: no se pudo guardar evento privado", "account", accountID, "events", len(events), "error", err)
					return
				}
				for _, event := range events {
					if err := m.Store.RecordStreamEvent(ctx, accountID, event.EventTime); err != nil {
						logger.Error("monitor: no se pudo registrar tiempo de evento", "account", accountID, "error", err)
					}
				}
			}
			if m.Gate != nil {
				m.Gate.setJournal(accountID, journal)
			}
			sink := func(event binance.UserEvent) {
				if m.Gate != nil && m.Gate.ingest(accountID, event) {
					return
				}
				journal([]binance.UserEvent{event})
			}
			if err := stream.Run(ctx, credentials, sink); err != nil && ctx.Err() == nil {
				logger.Error("monitor: flujo de eventos detenido", "account", accountID, "error", err)
			}
		}(account.ID, credentials)
	}
	wg.Wait()
	return ctx.Err()
}
