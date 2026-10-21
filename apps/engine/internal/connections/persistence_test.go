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
	deniedStore
	binding       vault.Binding
	envelope      vault.Envelope
	err           error
	saves         int
	snapshot      binance.Snapshot
	listAfter     int64
	fillAfter     int64
	fillLimit     int
	fills         []binance.Fill
	fillTotal     int
	fillError     error
	fillTruncated bool
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
	return database.StoredSnapshot{Snapshot: s.snapshot}, s.err
}
func (s *snapshotStore) LatestReadComparison(context.Context, string, string) (database.StoredComparison, error) {
	return database.StoredComparison{}, s.err
}
func (s *snapshotStore) ListUserEvents(_ context.Context, _ string, _ string, after int64, _ int) ([]database.StoredEvent, bool, error) {
	s.listAfter = after
	return nil, false, s.err
}
func (s *snapshotStore) LatestStreamState(context.Context, string, string) (database.StreamState, error) {
	return database.StreamState{}, s.err
}
func (s *snapshotStore) ListFills(_ context.Context, _ string, _ string, after int64, limit int) ([]binance.Fill, int, bool, error) {
	s.fillAfter = after
	s.fillLimit = limit
	return s.fills, s.fillTotal, s.fillTruncated, s.fillError
}

type snapshotReader struct {
	err   error
	calls int
}

func (s *snapshotStore) SaveOrderObservation(context.Context, string, string, binance.OrderLookup) error {
	s.saves++
	return s.err
}
func (r *snapshotReader) LookupOrder(context.Context, vault.Credentials, binance.OrderQuery) (binance.OrderLookup, error) {
	r.calls++
	return binance.OrderLookup{}, r.err
}

func (r *snapshotReader) Snapshot(context.Context, vault.Credentials) (binance.Snapshot, error) {
	r.calls++
	return binance.Snapshot{Source: "binance-production"}, r.err
}

func TestReconcileUsesSnapshotExchangeWatermarkAsBoundary(t *testing.T) {
	store := &snapshotStore{snapshot: binance.Snapshot{CapturedAtMs: 1700000000000}}
	service := &Service{Store: store}
	if _, err := service.Reconcile(context.Background(), "synthetic", "00000000-0000-4000-8000-000000000001"); err != nil {
		t.Fatal(err)
	}
	if store.listAfter != 1700000000000 || store.fillAfter != 1700000000000 || store.fillLimit != 200 {
		t.Fatal("reconcile did not use the snapshot exchange watermark as the event boundary")
	}
}

func TestReconcileUsesDurableFillsWithoutJournalEvents(t *testing.T) {
	store := &snapshotStore{
		snapshot:  binance.Snapshot{CapturedAtMs: 1700000000000},
		fills:     []binance.Fill{{OrderID: "123", TradeID: "456", EventTime: 1700000000001}},
		fillTotal: 201, fillTruncated: true,
	}
	service := &Service{Store: store}
	report, err := service.Reconcile(context.Background(), "synthetic", "00000000-0000-4000-8000-000000000001")
	if err != nil || report.TotalEvents != 0 || report.TotalFills != 201 || !report.FillsTruncated || len(report.Fills) != 1 || report.Fills[0].TradeID != "456" || report.ExecutionReady {
		t.Fatal("reconciliation ignored the durable fill page", err)
	}
	store.fillError = database.ErrSessionInvalid
	report, err = service.Reconcile(context.Background(), "synthetic", "00000000-0000-4000-8000-000000000001")
	if !errors.Is(err, database.ErrSessionInvalid) || report.Fills != nil {
		t.Fatal("fill read failure exposed a partial report")
	}
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
