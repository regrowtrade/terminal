package database

import (
	"context"
	"database/sql"
	"errors"
	"regexp"
	"time"
)

var ErrSessionInvalid = errors.New("sesión no válida")
var sessionHash = regexp.MustCompile(`^[a-f0-9]{64}$`)

type Identity struct {
	OwnerID   string    `json:"ownerId"`
	Role      string    `json:"role"`
	ExpiresAt time.Time `json:"expiresAt"`
}

// The personal binding is provisioned by an administrative migration. The
// runtime may read it but cannot insert/change identities or select an owner.
func (s *Store) withPersonal(ctx context.Context, fn func(*sql.Tx, string) error) error {
	var owner string
	if err := s.db.QueryRowContext(ctx, `SELECT owner_id::text FROM personal_identity WHERE singleton`).Scan(&owner); err != nil {
		return errors.New("identidad personal no disponible")
	}
	return s.WithOwner(ctx, owner, func(tx *sql.Tx) error {
		// Serialize create/check/revoke-all to define an unambiguous boundary.
		if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, owner); err != nil {
			return err
		}
		return fn(tx, owner)
	})
}

func sessionAudit(ctx context.Context, tx *sql.Tx, owner, action string) error {
	_, err := tx.ExecContext(ctx, `INSERT INTO audit_events(id, owner_id, action, correlation_id, metadata)
		VALUES (gen_random_uuid(), $1, $2, gen_random_uuid(), '{}')`, owner, action)
	return err
}

// Only the trusted web server calls this after password verification. No raw
// cookie, password or HMAC key is sent to PostgreSQL or the engine.
func (s *Store) CreateSession(ctx context.Context, hash string) (Identity, error) {
	var identity Identity
	if !sessionHash.MatchString(hash) {
		return identity, ErrSessionInvalid
	}
	err := s.withPersonal(ctx, func(tx *sql.Tx, owner string) error {
		if _, err := tx.ExecContext(ctx, `DELETE FROM owner_sessions WHERE expires_at <= now() OR revoked_at IS NOT NULL`); err != nil {
			return err
		}
		var count int
		if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM owner_sessions`).Scan(&count); err != nil {
			return err
		}
		if count >= 20 {
			return errors.New("límite de sesiones alcanzado")
		}
		identity.OwnerID, identity.Role = owner, "owner"
		if err := tx.QueryRowContext(ctx, `INSERT INTO owner_sessions(token_hash, owner_id) VALUES ($1, $2) RETURNING expires_at`, hash, owner).Scan(&identity.ExpiresAt); err != nil {
			return err
		}
		return sessionAudit(ctx, tx, owner, "session.created")
	})
	return identity, err
}

func (s *Store) ResolveSession(ctx context.Context, hash string) (Identity, error) {
	var identity Identity
	if !sessionHash.MatchString(hash) {
		return identity, ErrSessionInvalid
	}
	err := s.withPersonal(ctx, func(tx *sql.Tx, owner string) error {
		identity.OwnerID, identity.Role = owner, "owner"
		err := tx.QueryRowContext(ctx, `SELECT expires_at FROM owner_sessions WHERE token_hash = $1 AND revoked_at IS NULL AND expires_at > now()`, hash).Scan(&identity.ExpiresAt)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrSessionInvalid
		}
		return err
	})
	return identity, err
}

func (s *Store) RevokeSession(ctx context.Context, hash string, all bool) error {
	if !sessionHash.MatchString(hash) {
		return ErrSessionInvalid
	}
	return s.withPersonal(ctx, func(tx *sql.Tx, owner string) error {
		if all {
			var valid bool
			if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM owner_sessions WHERE token_hash = $1 AND revoked_at IS NULL AND expires_at > now())`, hash).Scan(&valid); err != nil {
				return err
			}
			if !valid {
				return ErrSessionInvalid
			}
		}
		result, err := tx.ExecContext(ctx, `UPDATE owner_sessions SET revoked_at = now() WHERE revoked_at IS NULL AND (token_hash = $1 OR $2)`, hash, all)
		if err != nil {
			return err
		}
		count, err := result.RowsAffected()
		if err != nil || count == 0 {
			return err
		}
		action := "session.revoked"
		if all {
			action = "session.revoked_all"
		}
		return sessionAudit(ctx, tx, owner, action)
	})
}
