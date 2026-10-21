package connections

import (
	"context"
	"errors"
	"regrow/engine/internal/binance"
	"regrow/engine/internal/database"
	"regrow/engine/internal/intentcheck"
	"regrow/engine/internal/reconcile"
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
	LatestStreamState(context.Context, string, string) (database.StreamState, error)
	ListFills(context.Context, string, string, int64, int) ([]binance.Fill, int, bool, error)
	SaveOrderObservation(context.Context, string, string, binance.OrderLookup) error
	ReadIntentExpected(context.Context, string, string, string) (intentcheck.Expected, error)
	SaveIntentObservation(context.Context, string, string, intentcheck.Expected, binance.OrderLookup) (database.StoredIntentObservation, error)
	ListIntentObservations(context.Context, string, string, string) ([]database.StoredIntentObservation, error)
	SaveRecoveredFills(context.Context, string, string, []binance.Fill) (database.RecoveredFillsResult, error)
}
type Reader interface {
	Snapshot(context.Context, vault.Credentials) (binance.Snapshot, error)
	LookupOrder(context.Context, vault.Credentials, binance.OrderQuery) (binance.OrderLookup, error)
	RecoverTrades(context.Context, vault.Credentials, binance.TradeQuery) ([]binance.Fill, bool, error)
}
type Service struct {
	Store  Store
	Vault  *vault.Keyring
	Reader Reader
	// Gate buffers events that arrive while a snapshot read is in flight so
	// they are not lost across the capturedAt boundary. Optional.
	Gate *SnapshotGate
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
	// Buffer events during the read so the monitor's journal never races the
	// snapshot save and no event straddling the boundary is dropped.
	commit := func() {}
	if s.Gate != nil {
		commit = s.Gate.begin(account)
	}
	defer commit()
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

// LookupOrder is an explicit read of a normal order. It never mutates a saved
// snapshot, resets stream gaps, synthesizes fills or enables execution.
func (s *Service) LookupOrder(ctx context.Context, hash, account string, query binance.OrderQuery) (binance.OrderLookup, error) {
	if !query.Valid() {
		return binance.OrderLookup{}, binance.ErrOrderQuery
	}
	if s == nil || s.Store == nil || s.Vault == nil || s.Reader == nil {
		return binance.OrderLookup{}, ErrDisabled
	}
	binding, envelope, err := s.Store.ReadAccountCredential(ctx, hash, account)
	if err != nil {
		return binance.OrderLookup{}, err
	}
	credentials, err := s.Vault.Open(binding, envelope)
	if err != nil {
		return binance.OrderLookup{}, vault.ErrVault
	}
	result, err := s.Reader.LookupOrder(ctx, credentials, query)
	if err != nil {
		return binance.OrderLookup{}, err
	}
	if result.Query != query || binance.ValidateOrderLookup(result) != nil {
		return binance.OrderLookup{}, binance.ErrResponse
	}
	if err := s.Store.SaveOrderObservation(ctx, hash, account, result); err != nil {
		return binance.OrderLookup{}, err
	}
	return result, nil
}

// RecoverFills reads a bounded REST trade window and upserts the validated
// fills into the durable deduplicated table. Recovered fills only add evidence;
// they never infer continuity, enable execution or overwrite existing rows.
func (s *Service) RecoverFills(ctx context.Context, hash, account string, query binance.TradeQuery) (binance.FillRecovery, error) {
	if !query.Valid() {
		return binance.FillRecovery{}, binance.ErrTradeQuery
	}
	if s == nil || s.Store == nil || s.Vault == nil || s.Reader == nil {
		return binance.FillRecovery{}, ErrDisabled
	}
	binding, envelope, err := s.Store.ReadAccountCredential(ctx, hash, account)
	if err != nil {
		return binance.FillRecovery{}, err
	}
	if binding.AccountID != account || binding.Exchange != "binance" || binding.Environment != "production" {
		return binance.FillRecovery{}, vault.ErrVault
	}
	credentials, err := s.Vault.Open(binding, envelope)
	if err != nil {
		return binance.FillRecovery{}, err
	}
	fills, truncated, err := s.Reader.RecoverTrades(ctx, credentials, query)
	if err != nil {
		return binance.FillRecovery{}, err
	}
	result, err := s.Store.SaveRecoveredFills(ctx, hash, account, fills)
	if err != nil {
		return binance.FillRecovery{}, err
	}
	return binance.FillRecovery{Source: "binance-production", Mode: "read_only", ExecutionReady: false, Symbol: query.Symbol, StartTime: query.StartTime, EndTime: query.EndTime, Recovered: result.Recovered, Duplicates: result.Duplicates, Truncated: truncated}, nil
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

// Reconcile layers the deduplicated, normalized event stream over the latest
// saved snapshot and reports stream continuity. It is read-only and available
// with the live reader disabled; it never infers fills from absence or enables
// execution. When the stream has disconnected since the snapshot, continuity is
// gapped and the event-derived view is explicitly incomplete.
func (s *Service) Reconcile(ctx context.Context, hash, account string) (reconcile.Report, error) {
	if s == nil || s.Store == nil {
		return reconcile.Report{}, ErrDisabled
	}
	snapshot, err := s.Store.LatestReadSnapshot(ctx, hash, account)
	if err != nil {
		return reconcile.Report{}, err
	}
	state, err := s.Store.LatestStreamState(ctx, hash, account)
	if err != nil {
		return reconcile.Report{}, err
	}
	// The snapshot's exchange-clock watermark is the boundary: only events with
	// event_time strictly after it happened after the snapshot and may be
	// layered on top. Older events are already reflected in the snapshot.
	stored, truncated, err := s.Store.ListUserEvents(ctx, hash, account, snapshot.Snapshot.CapturedAtMs, 200)
	if err != nil {
		return reconcile.Report{}, err
	}
	events := make([]binance.UserEvent, 0, len(stored))
	for _, e := range stored {
		events = append(events, e.Event)
	}
	// Fills come from the durable deduplicated table keyed by (order, trade);
	// the bounded page carries a complete total and a truncation flag.
	fills, fillTotal, fillsTruncated, err := s.Store.ListFills(ctx, hash, account, snapshot.Snapshot.CapturedAtMs, reconcile.FillLimit)
	if err != nil {
		return reconcile.Report{}, err
	}
	report := reconcile.Reconcile(snapshot.Snapshot, events, fills, fillTotal, fillsTruncated, state.Connected && !state.GappedSinceSnapshot)
	report.EventsTruncated = truncated
	return report, nil
}
