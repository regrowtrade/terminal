package database

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"testing"
)

func testDigest() string {
	raw := make([]byte, 48)
	if _, err := rand.Read(raw); err != nil {
		panic("cannot generate synthetic digest")
	}
	digest := sha256.Sum256(raw)
	return hex.EncodeToString(digest[:])
}

func TestSessionsRejectMalformedDigests(t *testing.T) {
	s := &Store{}
	for _, hash := range []string{"", "cookie", "not-a-digest", "' OR true --"} {
		if _, err := s.CreateSession(context.Background(), hash); !errors.Is(err, ErrSessionInvalid) {
			t.Fatal("malformed create")
		}
		if _, err := s.ResolveSession(context.Background(), hash); !errors.Is(err, ErrSessionInvalid) {
			t.Fatal("malformed resolve")
		}
		if err := s.RevokeSession(context.Background(), hash, true); !errors.Is(err, ErrSessionInvalid) {
			t.Fatal("malformed revoke")
		}
	}
}

func testSessionLifecycle(t *testing.T, ctx context.Context, store *Store, admin *sql.DB, uri string) {
	t.Helper()
	first, second, expired := testDigest(), testDigest(), testDigest()
	identity, err := store.CreateSession(ctx, first)
	if err != nil || identity.Role != "owner" || identity.OwnerID == "" {
		t.Fatal("personal identity creation failed")
	}
	if _, err := store.CreateSession(ctx, first); err == nil {
		t.Fatal("duplicate token accepted")
	}
	other, err := store.CreateSession(ctx, second)
	if err != nil || other.OwnerID != identity.OwnerID {
		t.Fatal("personal identity is not stable")
	}
	if _, err := store.ResolveSession(ctx, first); err != nil {
		t.Fatal("registered session missing")
	}
	// A fresh connection pool must resolve the same persisted session.
	restarted, err := Open(ctx, uri, store.migrations)
	if err != nil {
		t.Fatal("could not open new session pool")
	}
	defer restarted.Close()
	if _, err := restarted.ResolveSession(ctx, second); err != nil {
		t.Fatal("session not durable")
	}
	if err := store.RevokeSession(ctx, first, false); err != nil {
		t.Fatal("could not revoke")
	}
	if _, err := store.ResolveSession(ctx, first); !errors.Is(err, ErrSessionInvalid) {
		t.Fatal("copied cookie remains valid")
	}
	if _, err := restarted.ResolveSession(ctx, first); !errors.Is(err, ErrSessionInvalid) {
		t.Fatal("revocation did not reach the second instance")
	}
	if _, err := store.ResolveSession(ctx, second); err != nil {
		t.Fatal("individual logout revoked another session")
	}
	if err := store.RevokeSession(ctx, testDigest(), true); !errors.Is(err, ErrSessionInvalid) {
		t.Fatal("anonymous revoke-all accepted")
	}
	if err := store.RevokeSession(ctx, second, true); err != nil {
		t.Fatal("could not revoke all")
	}
	if _, err := store.ResolveSession(ctx, second); !errors.Is(err, ErrSessionInvalid) {
		t.Fatal("revoke-all left a valid cookie")
	}
	if _, err := store.CreateSession(ctx, expired); err != nil {
		t.Fatal("could not create expiration test")
	}
	if _, err := admin.ExecContext(ctx, `UPDATE owner_sessions SET created_at = now() - interval '9 hours', expires_at = now() - interval '1 hour' WHERE token_hash = $1`, expired); err != nil {
		t.Fatal("could not simulate expiration")
	}
	if _, err := store.ResolveSession(ctx, expired); !errors.Is(err, ErrSessionInvalid) {
		t.Fatal("expired session accepted")
	}
	var visible int
	if err := store.db.QueryRowContext(ctx, `SELECT count(*) FROM owner_sessions`).Scan(&visible); err != nil || visible != 0 {
		t.Fatal("session RLS missing")
	}
	if _, err := store.db.ExecContext(ctx, `UPDATE personal_identity SET owner_id = gen_random_uuid()`); err == nil {
		t.Fatal("runtime can reassign personal identity")
	}
	if err := store.WithOwner(ctx, "00000000-0000-4000-8000-000000000001", func(tx *sql.Tx) error {
		return tx.QueryRowContext(ctx, `SELECT count(*) FROM owner_sessions`).Scan(&visible)
	}); err != nil || visible != 0 {
		t.Fatal("another owner can view personal sessions")
	}
	var audit int
	if err := admin.QueryRowContext(ctx, `SELECT count(*) FROM audit_events WHERE action LIKE 'session.%' AND metadata = '{}'::jsonb`).Scan(&audit); err != nil || audit < 5 {
		t.Fatal("session audit missing or contains session data")
	}
	var current string
	for range 20 {
		current = testDigest()
		if _, err := store.CreateSession(ctx, current); err != nil {
			t.Fatal("could not fill session limit")
		}
	}
	if _, err := store.CreateSession(ctx, testDigest()); err == nil {
		t.Fatal("active session limit not enforced")
	}
	if err := store.RevokeSession(ctx, current, true); err != nil {
		t.Fatal("session limit prevented logout")
	}
	if _, err := store.CreateSession(ctx, testDigest()); err != nil {
		t.Fatal("revoked sessions prevent a new login")
	}
}
