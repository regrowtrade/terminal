package database

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"

	"regrow/engine/internal/vault"
)

var ErrAccountMissing = errors.New("cuenta no disponible")

type ReadAccount struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Exchange    string `json:"exchange"`
	Environment string `json:"environment"`
}

// Validate session and ownership in the same transaction as the resource read.
func (s *Store) withSession(ctx context.Context, hash string, fn func(*sql.Tx, string) error) error {
	if !sessionHash.MatchString(hash) {
		return ErrSessionInvalid
	}
	return s.withPersonal(ctx, func(tx *sql.Tx, owner string) error {
		var valid bool
		if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM owner_sessions WHERE token_hash=$1 AND revoked_at IS NULL AND expires_at>now())`, hash).Scan(&valid); err != nil {
			return err
		}
		if !valid {
			return ErrSessionInvalid
		}
		return fn(tx, owner)
	})
}
func (s *Store) ListReadAccounts(ctx context.Context, hash string) ([]ReadAccount, error) {
	result := []ReadAccount{}
	err := s.withSession(ctx, hash, func(tx *sql.Tx, _ string) error {
		rows, err := tx.QueryContext(ctx, `SELECT a.id::text,a.name,a.exchange,a.environment FROM exchange_accounts a JOIN exchange_credentials c ON c.account_id=a.id AND c.owner_id=a.owner_id WHERE a.exchange='binance' AND a.environment='production' ORDER BY a.created_at,a.id`)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var a ReadAccount
			if err := rows.Scan(&a.ID, &a.Name, &a.Exchange, &a.Environment); err != nil {
				return err
			}
			result = append(result, a)
		}
		return rows.Err()
	})
	return result, err
}
func (s *Store) ReadAccountCredential(ctx context.Context, hash, accountID string) (vault.Binding, vault.Envelope, error) {
	var binding vault.Binding
	var envelope vault.Envelope
	if !ownerUUID.MatchString(accountID) {
		return binding, envelope, ErrAccountMissing
	}
	err := s.withSession(ctx, hash, func(tx *sql.Tx, owner string) error {
		var data []byte
		err := tx.QueryRowContext(ctx, `SELECT c.envelope FROM exchange_credentials c JOIN exchange_accounts a ON a.id=c.account_id AND a.owner_id=c.owner_id WHERE a.id=$1 AND a.exchange='binance' AND a.environment='production'`, accountID).Scan(&data)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrAccountMissing
		}
		if err != nil {
			return err
		}
		if json.Unmarshal(data, &envelope) != nil {
			return vault.ErrVault
		}
		binding = vault.Binding{OwnerID: owner, AccountID: strings.ToLower(accountID), Exchange: "binance", Environment: "production"}
		_, err = tx.ExecContext(ctx, `INSERT INTO audit_events(id,owner_id,action,resource_id,correlation_id,metadata) VALUES(gen_random_uuid(),$1,'account.read_requested',$2,gen_random_uuid(),'{}')`, owner, accountID)
		return err
	})
	return binding, envelope, err
}

// ManageCredential is administrative only; the HTTP runtime has SELECT but no
// INSERT/UPDATE/DELETE on exchange_credentials. No caller-selected owner exists.
func ManageCredential(ctx context.Context, uri, action, accountID, name string, k *vault.Keyring, credentials vault.Credentials) (string, error) {
	if (action != "provision" && action != "rotate" && action != "delete") || (action != "provision" && !ownerUUID.MatchString(accountID)) {
		return "", ErrAccountMissing
	}
	db, err := connect(uri)
	if err != nil {
		return "", err
	}
	defer db.Close()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return "", errors.New("no se pudo conectar al proceso administrativo")
	}
	defer tx.Rollback()
	var owner string
	if err := tx.QueryRowContext(ctx, `SELECT owner_id::text FROM personal_identity WHERE singleton`).Scan(&owner); err != nil {
		return "", errors.New("identidad personal no disponible")
	}
	if _, err := tx.ExecContext(ctx, `SELECT set_config('app.user_id',$1,true)`, owner); err != nil {
		return "", ErrAccountMissing
	}
	if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, owner); err != nil {
		return "", ErrAccountMissing
	}
	if action == "provision" {
		if strings.TrimSpace(name) == "" || len(name) > 80 || strings.ContainsAny(name, "\r\n\x00") || !credentials.Valid() {
			return "", vault.ErrVault
		}
		if err := tx.QueryRowContext(ctx, `INSERT INTO exchange_accounts(id,owner_id,exchange,environment,name) VALUES(gen_random_uuid(),$1,'binance','production',$2) RETURNING id::text`, owner, name).Scan(&accountID); err != nil {
			return "", ErrAccountMissing
		}
	} else {
		var data []byte
		if err := tx.QueryRowContext(ctx, `SELECT c.envelope FROM exchange_credentials c JOIN exchange_accounts a ON a.id=c.account_id AND a.owner_id=c.owner_id WHERE a.id=$1 AND a.exchange='binance' AND a.environment='production' FOR UPDATE OF c`, accountID).Scan(&data); err != nil {
			return "", ErrAccountMissing
		}
		if action == "rotate" {
			var envelope vault.Envelope
			if json.Unmarshal(data, &envelope) != nil {
				return "", vault.ErrVault
			}
			credentials, err = k.Open(vault.Binding{OwnerID: owner, AccountID: strings.ToLower(accountID), Exchange: "binance", Environment: "production"}, envelope)
			if err != nil {
				return "", err
			}
		}
	}
	if action == "delete" {
		if _, err := tx.ExecContext(ctx, `DELETE FROM exchange_credentials WHERE account_id=$1`, accountID); err != nil {
			return "", ErrAccountMissing
		}
	} else {
		envelope, err := k.Seal(vault.Binding{OwnerID: owner, AccountID: strings.ToLower(accountID), Exchange: "binance", Environment: "production"}, credentials)
		if err != nil {
			return "", err
		}
		data, err := json.Marshal(envelope)
		if err != nil {
			return "", vault.ErrVault
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO exchange_credentials(account_id,owner_id,envelope) VALUES($1,$2,$3) ON CONFLICT(account_id) DO UPDATE SET envelope=EXCLUDED.envelope,updated_at=now()`, accountID, owner, data); err != nil {
			return "", vault.ErrVault
		}
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO audit_events(id,owner_id,action,resource_id,correlation_id,metadata) VALUES(gen_random_uuid(),$1,$2,$3,gen_random_uuid(),'{}')`, owner, "credential."+action, accountID); err != nil {
		return "", ErrAccountMissing
	}
	if err := tx.Commit(); err != nil {
		return "", errors.New("no se pudo confirmar el cambio de credenciales; verificar antes de reintentar")
	}
	return accountID, nil
}
