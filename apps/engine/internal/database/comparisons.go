package database

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"time"

	"regrow/engine/internal/observations"
)

var ErrComparisonMissing = errors.New("no hay una comparación guardada para esta cuenta")

type StoredComparison struct {
	Report   observations.Report `json:"report"`
	StoredAt time.Time           `json:"storedAt"`
}

// Comparison retrieval is local and explicitly historical. It neither opens
// credentials nor repairs/replaces a baseline or fetches a new observation.
func (s *Store) LatestReadComparison(ctx context.Context, hash, account string) (StoredComparison, error) {
	var result StoredComparison
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
		var started, captured time.Time
		if err := tx.QueryRowContext(ctx, `SELECT comparison,stored_at,started_at,captured_at FROM exchange_read_snapshots WHERE account_id=$1`, account).Scan(&data, &result.StoredAt, &started, &captured); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return ErrComparisonMissing
			}
			return err
		}
		if data == nil {
			return ErrComparisonMissing
		}
		if len(data) > 2*1024*1024 {
			return ErrSnapshotInvalid
		}
		d := json.NewDecoder(bytes.NewReader(data))
		d.DisallowUnknownFields()
		if d.Decode(&result.Report) != nil || d.Decode(&struct{}{}) != io.EOF || observations.ValidateReport(result.Report) != nil ||
			!result.Report.CurrentStartedAt.Equal(started) || !result.Report.CurrentCapturedAt.Equal(captured) {
			return ErrSnapshotInvalid
		}
		return nil
	})
	if err != nil {
		return StoredComparison{}, err
	}
	return result, nil
}
