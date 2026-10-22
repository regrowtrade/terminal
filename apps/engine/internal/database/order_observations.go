package database

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"regrow/engine/internal/binance"
)

// SaveOrderObservation revalidates both session and credential ownership after
// the external read. No partial or unpersisted result may be returned by HTTP.
func (s *Store) SaveOrderObservation(ctx context.Context, hash, account string, observation binance.OrderLookup) error {
	if !ownerUUID.MatchString(account) {
		return ErrAccountMissing
	}
	if binance.ValidateOrderLookup(observation) != nil {
		return ErrSnapshotInvalid
	}
	observation.StartedAt = observation.StartedAt.UTC().Truncate(time.Microsecond)
	observation.CheckedAt = observation.CheckedAt.UTC().Truncate(time.Microsecond)
	return s.saveOrderEvidence(ctx, hash, account, observation.StartedAt, observation.CheckedAt, observation, observation.Outcome, "account.order_observed")
}

// Algo evidence reuses the append-only 0010 table, but has its own validated
// DTO, query identifiers, family discriminator and audit action.
func (s *Store) SaveAlgoObservation(ctx context.Context, hash, account string, observation binance.AlgoLookup) error {
	if !ownerUUID.MatchString(account) {
		return ErrAccountMissing
	}
	if binance.ValidateAlgoLookup(observation) != nil {
		return ErrSnapshotInvalid
	}
	observation.StartedAt = observation.StartedAt.UTC().Truncate(time.Microsecond)
	observation.CheckedAt = observation.CheckedAt.UTC().Truncate(time.Microsecond)
	return s.saveOrderEvidence(ctx, hash, account, observation.StartedAt, observation.CheckedAt, observation, observation.Outcome, "account.algo_order_observed")
}

func (s *Store) saveOrderEvidence(ctx context.Context, hash, account string, started, checked time.Time, observation any, outcome, action string) error {
	data, err := json.Marshal(observation)
	if err != nil || len(data) > 16384 {
		return ErrSnapshotInvalid
	}
	return s.withSession(ctx, hash, func(tx *sql.Tx, owner string) error {
		var exists bool
		if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM exchange_accounts a JOIN exchange_credentials c ON c.account_id=a.id AND c.owner_id=a.owner_id WHERE a.id=$1 AND a.owner_id=$2 AND a.exchange='binance' AND a.environment='production')`, account, owner).Scan(&exists); err != nil {
			return err
		}
		if !exists {
			return ErrAccountMissing
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO binance_order_observations(account_id,owner_id,started_at,checked_at,observation) VALUES($1,$2,$3,$4,$5)`, account, owner, started, checked, data); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx, `INSERT INTO audit_events(id,owner_id,action,resource_id,correlation_id,metadata) VALUES(gen_random_uuid(),$1,$4,$2,gen_random_uuid(),jsonb_build_object('outcome',$3::text))`, owner, account, outcome, action)
		return err
	})
}
