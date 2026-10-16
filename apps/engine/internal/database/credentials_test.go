package database

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"regrow/engine/internal/vault"
	"strings"
	"testing"
)

func testCredentialLifecycle(t *testing.T, ctx context.Context, store *Store, admin *sql.DB, adminURI, runtimeURI string) {
	t.Helper()
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		t.Fatal("random data unavailable")
	}
	raw, _ := json.Marshal(map[string]string{"first": base64.StdEncoding.EncodeToString(key)})
	ring, err := vault.New("first", string(raw))
	if err != nil {
		t.Fatal(err)
	}
	credentials := vault.Credentials{APIKey: testDigest(), APISecret: testDigest()}
	id, err := ManageCredential(ctx, adminURI, "provision", "", "Synthetic read-only", ring, credentials)
	if err != nil {
		t.Fatal("administrative provisioning failed")
	}
	hash := testDigest()
	if _, err := store.CreateSession(ctx, hash); err != nil {
		t.Fatal("could not create resource test session")
	}
	accounts, err := store.ListReadAccounts(ctx, hash)
	if err != nil || len(accounts) != 1 || accounts[0].ID != id {
		t.Fatal("owned account not listed")
	}
	binding, envelope, err := store.ReadAccountCredential(ctx, hash, id)
	if err != nil {
		t.Fatal("owned credential unavailable")
	}
	opened, err := ring.Open(binding, envelope)
	if err != nil || opened != credentials {
		t.Fatal("persisted credential did not decrypt")
	}
	var stored string
	if err := admin.QueryRowContext(ctx, `SELECT envelope::text FROM exchange_credentials WHERE account_id=$1`, id).Scan(&stored); err != nil || strings.Contains(stored, credentials.APIKey) || strings.Contains(stored, credentials.APISecret) {
		t.Fatal("plaintext in database")
	}
	if _, err := ManageCredential(ctx, runtimeURI, "provision", "", "Forbidden", ring, credentials); err == nil {
		t.Fatal("runtime can provision credentials")
	}
	if _, err := ManageCredential(ctx, runtimeURI, "delete", id, "", nil, vault.Credentials{}); err == nil {
		t.Fatal("runtime can delete credentials")
	}
	if err := store.WithOwner(ctx, "00000000-0000-4000-8000-000000000001", func(tx *sql.Tx) error {
		var count int
		if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM exchange_credentials`).Scan(&count); err != nil {
			return err
		}
		if count != 0 {
			return errors.New("credential leaked across owner")
		}
		return nil
	}); err != nil {
		t.Fatal("credential RLS failed")
	}
	if _, _, err := store.ReadAccountCredential(ctx, testDigest(), id); !errors.Is(err, ErrSessionInvalid) {
		t.Fatal("unregistered session read credentials")
	}
	oldKey := base64.StdEncoding.EncodeToString(key)
	if _, err := rand.Read(key); err != nil {
		t.Fatal("random data unavailable")
	}
	newKey := base64.StdEncoding.EncodeToString(key)
	raw, _ = json.Marshal(map[string]string{"first": oldKey, "second": newKey})
	rotated, err := vault.New("second", string(raw))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ManageCredential(ctx, adminURI, "rotate", id, "", rotated, vault.Credentials{}); err != nil {
		t.Fatal("rotation failed")
	}
	binding, envelope, err = store.ReadAccountCredential(ctx, hash, id)
	if err != nil || envelope.KeyID != "second" {
		t.Fatal("rotation was not persisted")
	}
	opened, err = rotated.Open(binding, envelope)
	if err != nil || opened != credentials {
		t.Fatal("rotation changed credential")
	}
	if _, err := ring.Open(binding, envelope); err == nil {
		t.Fatal("old keyring decrypted new envelope")
	}
	testReadSnapshotLifecycle(t, ctx, store, admin, runtimeURI, hash, id)
	if _, err := ManageCredential(ctx, adminURI, "delete", id, "", nil, vault.Credentials{}); err != nil {
		t.Fatal("could not remove encrypted credential")
	}
	if _, _, err := store.ReadAccountCredential(ctx, hash, id); !errors.Is(err, ErrAccountMissing) {
		t.Fatal("deleted credential available")
	}
	if _, err := store.LatestReadSnapshot(ctx, hash, id); err != nil {
		t.Fatal("key deletion removed historical evidence")
	}
	if err := store.SaveReadSnapshot(ctx, hash, id, syntheticReadSnapshot()); !errors.Is(err, ErrAccountMissing) {
		t.Fatal("removed credential can still save readings")
	}
	if err := store.RevokeSession(ctx, hash, false); err != nil {
		t.Fatal("could not revoke resource test session")
	}
	if _, err := store.ListReadAccounts(ctx, hash); !errors.Is(err, ErrSessionInvalid) {
		t.Fatal("revoked session lists accounts")
	}
}
