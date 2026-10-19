package database

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"regrow/engine/internal/binance"
	"regrow/engine/internal/vault"
)

var ErrEventInvalid = errors.New("evento privado inválido o incompleto; no se guardó")

// StreamAccount carries a personal owner's provisioned account with its
// encrypted credential envelope for the background monitor. It is never
// exposed over HTTP and never accepts a caller-selected owner.
type StreamAccount struct {
	ID       string
	Binding  vault.Binding
	Envelope vault.Envelope
}

type StoredEvent struct {
	Event      binance.UserEvent `json:"event"`
	ReceivedAt time.Time         `json:"receivedAt"`
}

// ListStreamAccounts enumerates the personal owner's production Binance
// accounts under the personal identity only (no browser session).
func (s *Store) ListStreamAccounts(ctx context.Context) ([]StreamAccount, error) {
	result := []StreamAccount{}
	err := s.withPersonal(ctx, func(tx *sql.Tx, owner string) error {
		rows, err := tx.QueryContext(ctx, `SELECT a.id::text, c.envelope FROM exchange_accounts a JOIN exchange_credentials c ON c.account_id=a.id AND c.owner_id=a.owner_id WHERE a.exchange='binance' AND a.environment='production' ORDER BY a.created_at, a.id`)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var account StreamAccount
			var data []byte
			if err := rows.Scan(&account.ID, &data); err != nil {
				return err
			}
			if json.Unmarshal(data, &account.Envelope) != nil {
				return vault.ErrVault
			}
			account.Binding = vault.Binding{OwnerID: owner, AccountID: strings.ToLower(account.ID), Exchange: "binance", Environment: "production"}
			result = append(result, account)
		}
		return rows.Err()
	})
	return result, err
}

// SaveUserEvents persists validated private events idempotently under the
// personal identity, rechecking account ownership. Duplicate dedup keys are
// ignored; journal rows are never overwritten or deleted.
func (s *Store) SaveUserEvents(ctx context.Context, account string, events []binance.UserEvent) (int, error) {
	if !ownerUUID.MatchString(account) {
		return 0, ErrAccountMissing
	}
	if len(events) == 0 {
		return 0, nil
	}
	if len(events) > 1000 {
		return 0, ErrEventInvalid
	}
	type prepared struct {
		event binance.UserEvent
		key   string
		data  []byte
	}
	batch := make([]prepared, 0, len(events))
	for _, event := range events {
		if !event.Valid() {
			return 0, ErrEventInvalid
		}
		key := event.DedupKey()
		if len(key) != 64 {
			return 0, ErrEventInvalid
		}
		data, err := json.Marshal(event)
		if err != nil || len(data) > 65536 {
			return 0, ErrEventInvalid
		}
		batch = append(batch, prepared{event: event, key: key, data: data})
	}
	saved := 0
	err := s.withPersonal(ctx, func(tx *sql.Tx, owner string) error {
		var exists bool
		if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM exchange_accounts WHERE id=$1 AND owner_id=$2 AND exchange='binance' AND environment='production')`, account, owner).Scan(&exists); err != nil {
			return err
		}
		if !exists {
			return ErrAccountMissing
		}
		for _, p := range batch {
			result, err := tx.ExecContext(ctx, `INSERT INTO binance_user_events(account_id,owner_id,dedup_key,event_type,event_time,transaction_time,payload) VALUES($1,$2,$3,$4,$5,$6,$7) ON CONFLICT (account_id, dedup_key) DO NOTHING`, account, owner, p.key, p.event.Type, p.event.EventTime, p.event.TransactionTime, p.data)
			if err != nil {
				return err
			}
			n, err := result.RowsAffected()
			if err != nil {
				return err
			}
			saved += int(n)
		}
		// Extract TRADE executions into the durable fills table in the same
		// transaction; the (order, trade) key deduplicates at-least-once delivery.
		if err := saveFills(ctx, tx, account, owner, events); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	return saved, nil
}

// ListUserEvents returns the bounded, ascending journal for the authenticated
// owner. It requires a valid session and never decrypts credentials. truncated
// reports when more events remain beyond the requested page.
func (s *Store) ListUserEvents(ctx context.Context, hash, account string, after int64, limit int) ([]StoredEvent, bool, error) {
	if !ownerUUID.MatchString(account) {
		return nil, false, ErrAccountMissing
	}
	if limit <= 0 || limit > 200 {
		limit = 200
	}
	result := []StoredEvent{}
	truncated := false
	err := s.withSession(ctx, hash, func(tx *sql.Tx, _ string) error {
		var exists bool
		if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM exchange_accounts WHERE id=$1 AND exchange='binance' AND environment='production')`, account).Scan(&exists); err != nil {
			return err
		}
		if !exists {
			return ErrAccountMissing
		}
		rows, err := tx.QueryContext(ctx, `SELECT payload, received_at FROM binance_user_events WHERE account_id=$1 AND event_time>$2 ORDER BY event_time, transaction_time, received_at LIMIT $3`, account, after, limit+1)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var data []byte
			var stored StoredEvent
			if err := rows.Scan(&data, &stored.ReceivedAt); err != nil {
				return err
			}
			var event binance.UserEvent
			if json.Unmarshal(data, &event) != nil || !event.Valid() {
				return ErrEventInvalid
			}
			stored.Event = event
			result = append(result, stored)
		}
		if err := rows.Err(); err != nil {
			return err
		}
		if len(result) > limit {
			truncated = true
			result = result[:limit]
		}
		return nil
	})
	if err != nil {
		return nil, false, err
	}
	return result, truncated, nil
}
