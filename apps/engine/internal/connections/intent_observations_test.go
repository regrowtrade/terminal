package connections

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"regrow/engine/internal/binance"
	"regrow/engine/internal/database"
	"regrow/engine/internal/intentcheck"
	"regrow/engine/internal/vault"
)

type intentStore struct {
	*snapshotStore
	expected      intentcheck.Expected
	readErr       error
	credentialErr error
	saveErr       error
	saved         int
}

func (s *intentStore) ReadAccountCredential(ctx context.Context, hash, account string) (vault.Binding, vault.Envelope, error) {
	if s.credentialErr != nil {
		return vault.Binding{}, vault.Envelope{}, s.credentialErr
	}
	return s.snapshotStore.ReadAccountCredential(ctx, hash, account)
}

func (s *intentStore) ReadIntentExpected(context.Context, string, string, string) (intentcheck.Expected, error) {
	return s.expected, s.readErr
}
func (s *intentStore) SaveIntentObservation(_ context.Context, _, _ string, e intentcheck.Expected, o binance.OrderLookup) (database.StoredIntentObservation, error) {
	s.saved++
	if s.saveErr != nil {
		return database.StoredIntentObservation{}, s.saveErr
	}
	r, err := intentcheck.Assess(e, o)
	return database.StoredIntentObservation{ID: e.IntentID, StoredAt: time.Now().UTC(), Report: r}, err
}

func TestIntentInvestigationUsesStoredCriteriaAndRequiresCommit(t *testing.T) {
	ring, _, binding, envelope := sealedAccount(t)
	e := intentcheck.Expected{IntentID: "00000000-0000-4000-8000-000000000003", PlanID: "00000000-0000-4000-8000-000000000004", PlanFingerprint: strings.Repeat("b", 64), Symbol: "BTCUSDT", ClientOrderID: "rt" + strings.Repeat("a", 30), Purpose: "ENTRY", PositionSide: "LONG", Quantity: "1", Price: "100"}
	now := time.Now().UTC()
	valid := binance.OrderLookup{Source: "binance-production", Mode: "read_only", StartedAt: now, CheckedAt: now, Query: e.Query(), Outcome: "unresolved", Reason: "exchange_not_found"}
	for _, tc := range []struct {
		name         string
		mutate       func(*Service, *intentStore, *lookupReader)
		want         error
		calls, saves int
	}{
		{name: "unresolved persisted", calls: 1, saves: 1},
		{name: "revoked before read", mutate: func(_ *Service, s *intentStore, _ *lookupReader) { s.readErr = database.ErrSessionInvalid }, want: database.ErrSessionInvalid},
		{name: "foreign intent", mutate: func(_ *Service, s *intentStore, _ *lookupReader) { s.readErr = database.ErrIntentMissing }, want: database.ErrIntentMissing},
		{name: "conditional rejected", mutate: func(_ *Service, s *intentStore, _ *lookupReader) { s.readErr = database.ErrIntentUnsupported }, want: database.ErrIntentUnsupported},
		{name: "invalid source", mutate: func(_ *Service, s *intentStore, _ *lookupReader) { s.expected.Price = "0" }, want: database.ErrPlanInvalid},
		{name: "credential removed", mutate: func(_ *Service, s *intentStore, _ *lookupReader) { s.credentialErr = database.ErrAccountMissing }, want: database.ErrAccountMissing},
		{name: "wrong credential account", mutate: func(_ *Service, s *intentStore, _ *lookupReader) { s.binding.AccountID = e.PlanID }, want: vault.ErrVault},
		{name: "corrupt credential", mutate: func(_ *Service, s *intentStore, _ *lookupReader) { s.envelope = vault.Envelope{} }, want: vault.ErrVault},
		{name: "reader error", mutate: func(_ *Service, _ *intentStore, r *lookupReader) { r.err = binance.ErrUnavailable }, want: binance.ErrUnavailable, calls: 1},
		{name: "wrong selector", mutate: func(_ *Service, _ *intentStore, r *lookupReader) { r.result.Query.OrderID = "1" }, want: intentcheck.ErrEvidence, calls: 1},
		{name: "revoked while reading", mutate: func(_ *Service, s *intentStore, _ *lookupReader) { s.saveErr = database.ErrSessionInvalid }, want: database.ErrSessionInvalid, calls: 1, saves: 1},
		{name: "plan changed", mutate: func(_ *Service, s *intentStore, _ *lookupReader) { s.saveErr = database.ErrIntentChanged }, want: database.ErrIntentChanged, calls: 1, saves: 1},
		{name: "audit unavailable", mutate: func(_ *Service, s *intentStore, _ *lookupReader) {
			s.saveErr = errors.New("synthetic database failure")
		}, calls: 1, saves: 1},
		{name: "disabled", mutate: func(s *Service, _ *intentStore, _ *lookupReader) { s.Reader = nil }, want: ErrDisabled},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store := &intentStore{snapshotStore: &snapshotStore{binding: binding, envelope: envelope}, expected: e}
			reader := &lookupReader{snapshotReader: &snapshotReader{}, result: valid}
			service := &Service{Store: store, Vault: ring, Reader: reader}
			if tc.mutate != nil {
				tc.mutate(service, store, reader)
			}
			want := tc.want
			if tc.name == "audit unavailable" {
				want = store.saveErr
			}
			result, err := service.ObserveIntent(context.Background(), "synthetic", binding.AccountID, e.IntentID)
			if !errors.Is(err, want) || reader.calls != tc.calls || store.saved != tc.saves {
				t.Fatal("investigation crossed boundary", err, reader.calls, store.saved)
			}
			if err != nil && result.ID != "" {
				t.Fatal("failed save returned partial evidence")
			}
			if reader.calls > 0 && reader.query != e.Query() {
				t.Fatal("query was not derived from stored intent")
			}
			if store.snapshotStore.saves != 0 {
				t.Fatal("investigation saved unrelated snapshot/order evidence")
			}
		})
	}
}

func TestIntentHistoryNeedsAuthorizationButNotReader(t *testing.T) {
	service := &Service{Store: deniedStore{err: database.ErrSessionInvalid}}
	if _, err := service.IntentObservations(context.Background(), "synthetic", "account", "intent"); !errors.Is(err, database.ErrSessionInvalid) {
		t.Fatal("history bypassed authorization")
	}
	service.Store = deniedStore{}
	if _, err := service.IntentObservations(context.Background(), "synthetic", "account", "intent"); err != nil {
		t.Fatal("history required exchange credentials")
	}
	if _, err := (*Service)(nil).ObserveIntent(context.Background(), "synthetic", "account", "intent"); !errors.Is(err, ErrDisabled) {
		t.Fatal("nil service accepted")
	}
}
