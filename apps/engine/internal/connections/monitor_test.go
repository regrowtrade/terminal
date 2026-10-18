package connections

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"io"
	"log/slog"
	"strings"
	"testing"

	"regrow/engine/internal/binance"
	"regrow/engine/internal/database"
	"regrow/engine/internal/vault"
)

type fakeMonitorStore struct {
	accounts []database.StreamAccount
	saved    []binance.UserEvent
	listErr  error
	saveErr  error
}

func (f *fakeMonitorStore) ListStreamAccounts(context.Context) ([]database.StreamAccount, error) {
	return f.accounts, f.listErr
}
func (f *fakeMonitorStore) SaveUserEvents(_ context.Context, _ string, events []binance.UserEvent) (int, error) {
	if f.saveErr != nil {
		return 0, f.saveErr
	}
	f.saved = append(f.saved, events...)
	return len(events), nil
}

type fakeStreamer struct {
	event binance.UserEvent
	calls int
}

func (f *fakeStreamer) Run(_ context.Context, _ vault.Credentials, sink func(binance.UserEvent)) error {
	f.calls++
	sink(f.event)
	return nil
}

func sealedAccount(t *testing.T) (*vault.Keyring, vault.Credentials, vault.Binding, vault.Envelope) {
	t.Helper()
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(map[string]string{"k": base64.StdEncoding.EncodeToString(key)})
	ring, err := vault.New("k", string(raw))
	if err != nil {
		t.Fatal(err)
	}
	credentials := vault.Credentials{APIKey: strings.Repeat("a", 64), APISecret: strings.Repeat("b", 64)}
	binding := vault.Binding{OwnerID: "00000000-0000-4000-8000-000000000001", AccountID: "00000000-0000-4000-8000-00000000000a", Exchange: "binance", Environment: "production"}
	envelope, err := ring.Seal(binding, credentials)
	if err != nil {
		t.Fatal(err)
	}
	return ring, credentials, binding, envelope
}

func discardLogger() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

func TestMonitorRequiresComponents(t *testing.T) {
	stream := &fakeStreamer{}
	for name, m := range map[string]*Monitor{
		"nil":       nil,
		"no vault":  {Store: &fakeMonitorStore{}, Stream: stream},
		"no store":  {Vault: &vault.Keyring{}, Stream: stream},
		"no stream": {Store: &fakeMonitorStore{}, Vault: &vault.Keyring{}},
	} {
		t.Run(name, func(t *testing.T) {
			if err := m.Run(context.Background()); err == nil {
				t.Fatal("incomplete monitor accepted")
			}
		})
	}
}

func TestMonitorStreamsAndJournalsEvents(t *testing.T) {
	ring, credentials, binding, envelope := sealedAccount(t)
	store := &fakeMonitorStore{accounts: []database.StreamAccount{{ID: binding.AccountID, Binding: binding, Envelope: envelope}}}
	stream := &fakeStreamer{event: binance.UserEvent{Type: binance.EventAccountUpdate, EventTime: 1, TransactionTime: 1, AccountUpdate: &binance.AccountUpdate{Reason: "ORDER"}}}
	m := &Monitor{Store: store, Vault: ring, Stream: stream, Log: discardLogger()}
	if err := m.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if stream.calls != 1 || len(store.saved) != 1 || store.saved[0].Type != binance.EventAccountUpdate {
		t.Fatal("monitor did not journal the streamed event")
	}
	opened, err := ring.Open(binding, envelope)
	if err != nil || opened != credentials {
		t.Fatal("monitor did not open the provisioned credential")
	}
}

func TestMonitorSkipsUnopenableCredential(t *testing.T) {
	ring, _, binding, envelope := sealedAccount(t)
	corrupt := envelope
	corrupt.Ciphertext = append([]byte(nil), corrupt.Ciphertext...)
	if len(corrupt.Ciphertext) > 0 {
		corrupt.Ciphertext[0] ^= 0xff
	}
	store := &fakeMonitorStore{accounts: []database.StreamAccount{{ID: binding.AccountID, Binding: binding, Envelope: corrupt}}}
	stream := &fakeStreamer{}
	m := &Monitor{Store: store, Vault: ring, Stream: stream, Log: discardLogger()}
	if err := m.Run(context.Background()); err != nil {
		t.Fatal("unopenable credential was not skipped gracefully", err)
	}
	if stream.calls != 0 || len(store.saved) != 0 {
		t.Fatal("unopenable credential still streamed or journaled")
	}
}
