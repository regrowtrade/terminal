package database

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"time"

	"regrow/engine/internal/binance"
	"regrow/engine/internal/observations"
)

var ErrSnapshotMissing = errors.New("no hay una lectura guardada para esta cuenta")
var ErrSnapshotInvalid = errors.New("lectura incompleta o fuera de orden; no se guardó")

type StoredSnapshot struct {
	Snapshot binance.Snapshot `json:"snapshot"`
	StoredAt time.Time        `json:"storedAt"`
}

// The only writer is the internal read adapter, never a browser-provided JSON
// payload. Validate normalized financial data again at the durable boundary.
func completeSnapshot(s binance.Snapshot) bool {
	return binance.ValidateSnapshot(s) == nil
}

func (s *Store) SaveReadSnapshot(ctx context.Context, hash, account string, snapshot binance.Snapshot) error {
	if !ownerUUID.MatchString(account) {
		return ErrAccountMissing
	}
	if !completeSnapshot(snapshot) {
		return ErrSnapshotInvalid
	}
	// PostgreSQL timestamps have microsecond resolution; normalize JSON and
	// columns together rather than relying on differing round/truncate rules.
	snapshot.StartedAt = snapshot.StartedAt.UTC().Truncate(time.Microsecond)
	snapshot.CapturedAt = snapshot.CapturedAt.UTC().Truncate(time.Microsecond)
	data, err := json.Marshal(snapshot)
	if err != nil || len(data) > 8*1024*1024 {
		return ErrSnapshotInvalid
	}
	return s.withSession(ctx, hash, func(tx *sql.Tx, owner string) error {
		// Recheck the session and account after network access, without keeping
		// database locks open during exchange requests. A removed key prevents save.
		var exists bool
		if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM exchange_accounts a JOIN exchange_credentials c ON c.account_id=a.id AND c.owner_id=a.owner_id WHERE a.id=$1 AND a.exchange='binance' AND a.environment='production')`, account).Scan(&exists); err != nil {
			return err
		}
		if !exists {
			return ErrAccountMissing
		}
		// withSession serializes this owner's transactions. Read the previous
		// durable baseline here, not before network access, and replace both
		// observation and comparison together. No exchange calls under this lock.
		var prior []byte
		err := tx.QueryRowContext(ctx, `SELECT snapshot FROM exchange_read_snapshots WHERE account_id=$1 FOR UPDATE`, account).Scan(&prior)
		var baseline *binance.Snapshot
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		if err == nil {
			previous, err := decodeReadSnapshot(prior)
			if err != nil {
				return err
			}
			baseline = &previous
		}
		report, err := observations.Compare(baseline, snapshot)
		if err != nil {
			return ErrSnapshotInvalid
		}
		comparison, err := json.Marshal(report)
		if err != nil || len(comparison) > 2*1024*1024 {
			return ErrSnapshotInvalid
		}
		result, err := tx.ExecContext(ctx, `INSERT INTO exchange_read_snapshots(account_id,owner_id,started_at,captured_at,snapshot,comparison)
			VALUES($1,$2,$3,$4,$5,$6) ON CONFLICT(account_id) DO UPDATE
			SET started_at=EXCLUDED.started_at,captured_at=EXCLUDED.captured_at,snapshot=EXCLUDED.snapshot,comparison=EXCLUDED.comparison,stored_at=clock_timestamp()
			WHERE exchange_read_snapshots.owner_id=EXCLUDED.owner_id AND exchange_read_snapshots.captured_at<EXCLUDED.captured_at`, account, owner, snapshot.StartedAt, snapshot.CapturedAt, data, comparison)
		if err != nil {
			return err
		}
		count, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if count != 1 {
			return ErrSnapshotInvalid
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO audit_events(id,owner_id,action,resource_id,correlation_id,metadata) VALUES(gen_random_uuid(),$1,'account.read_saved',$2,gen_random_uuid(),'{}')`, owner, account)
		return err
	})
}

func (s *Store) LatestReadSnapshot(ctx context.Context, hash, account string) (StoredSnapshot, error) {
	var result StoredSnapshot
	if !ownerUUID.MatchString(account) {
		return result, ErrAccountMissing
	}
	err := s.withSession(ctx, hash, func(tx *sql.Tx, _ string) error {
		var exists bool
		if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM exchange_accounts WHERE id=$1 AND exchange='binance' AND environment='production')`, account).Scan(&exists); err != nil {
			return err
		}
		if !exists {
			return ErrAccountMissing
		}
		var data []byte
		if err := tx.QueryRowContext(ctx, `SELECT snapshot,stored_at FROM exchange_read_snapshots WHERE account_id=$1`, account).Scan(&data, &result.StoredAt); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return ErrSnapshotMissing
			}
			return err
		}
		var err error
		result.Snapshot, err = decodeReadSnapshot(data)
		return err
	})
	if err != nil {
		return StoredSnapshot{}, err
	}
	return result, nil
}

func decodeReadSnapshot(data []byte) (binance.Snapshot, error) {
	var snapshot binance.Snapshot
	if len(data) > 8*1024*1024 {
		return snapshot, ErrSnapshotInvalid
	}
	d := json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	if d.Decode(&snapshot) != nil || d.Decode(&struct{}{}) != io.EOF || !completeSnapshot(snapshot) {
		return binance.Snapshot{}, ErrSnapshotInvalid
	}
	return snapshot, nil
}
