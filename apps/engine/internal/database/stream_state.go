package database

import (
	"context"
	"database/sql"
	"time"
)

type StreamState struct {
	Connected           bool      `json:"connected"`
	LastEventTime       int64     `json:"lastEventTime"`
	GappedSinceSnapshot bool      `json:"gappedSinceSnapshot"`
	UpdatedAt           time.Time `json:"updatedAt"`
}

// SetStreamConnected records a connection transition under the personal
// identity (no browser session). A disconnect marks a gap since the last
// snapshot; a connect clears the disconnected flag but never clears the gap.
func (s *Store) SetStreamConnected(ctx context.Context, account string, connected bool) error {
	if !ownerUUID.MatchString(account) {
		return ErrAccountMissing
	}
	return s.withPersonal(ctx, func(tx *sql.Tx, owner string) error {
		if !accountOwned(ctx, tx, account, owner) {
			return ErrAccountMissing
		}
		if connected {
			_, err := tx.ExecContext(ctx, `INSERT INTO binance_stream_state(account_id, owner_id, connected) VALUES($1,$2,true) ON CONFLICT(account_id) DO UPDATE SET connected=true, updated_at=clock_timestamp() WHERE binance_stream_state.owner_id=$2`, account, owner)
			return err
		}
		_, err := tx.ExecContext(ctx, `INSERT INTO binance_stream_state(account_id, owner_id, connected, gapped_since_snapshot) VALUES($1,$2,false,true) ON CONFLICT(account_id) DO UPDATE SET connected=false, gapped_since_snapshot=true, updated_at=clock_timestamp() WHERE binance_stream_state.owner_id=$2`, account, owner)
		return err
	})
}

// RecordStreamEvent advances the latest exchange event time for the personal
// owner's account; it never rolls the watermark backwards.
func (s *Store) RecordStreamEvent(ctx context.Context, account string, eventTime int64) error {
	if !ownerUUID.MatchString(account) || eventTime <= 0 || eventTime > 9007199254740991 {
		return ErrAccountMissing
	}
	return s.withPersonal(ctx, func(tx *sql.Tx, owner string) error {
		if !accountOwned(ctx, tx, account, owner) {
			return ErrAccountMissing
		}
		_, err := tx.ExecContext(ctx, `INSERT INTO binance_stream_state(account_id, owner_id, connected, last_event_time) VALUES($1,$2,true,$3) ON CONFLICT(account_id) DO UPDATE SET last_event_time=GREATEST(binance_stream_state.last_event_time, EXCLUDED.last_event_time), updated_at=clock_timestamp() WHERE binance_stream_state.owner_id=$2`, account, owner, eventTime)
		return err
	})
}

// LatestStreamState reads the continuity state for the authenticated owner.
// A missing row means never connected: disconnected and not gapped.
func (s *Store) LatestStreamState(ctx context.Context, hash, account string) (StreamState, error) {
	var result StreamState
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
		err := tx.QueryRowContext(ctx, `SELECT connected, last_event_time, gapped_since_snapshot, updated_at FROM binance_stream_state WHERE account_id=$1`, account).Scan(&result.Connected, &result.LastEventTime, &result.GappedSinceSnapshot, &result.UpdatedAt)
		if err != nil {
			if err == sql.ErrNoRows {
				result = StreamState{}
				return nil
			}
			return err
		}
		return nil
	})
	if err != nil {
		return StreamState{}, err
	}
	return result, nil
}

func accountOwned(ctx context.Context, tx *sql.Tx, account, owner string) bool {
	var exists bool
	if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM exchange_accounts WHERE id=$1 AND owner_id=$2 AND exchange='binance' AND environment='production')`, account, owner).Scan(&exists); err != nil {
		return false
	}
	return exists
}
