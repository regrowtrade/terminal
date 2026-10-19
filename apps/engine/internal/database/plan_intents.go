package database

import (
	"context"
	"database/sql"
	"errors"

	"regrow/engine/internal/planner"
)

var ErrPlanMissing = errors.New("plan no disponible")

// StoredIntent is a planned (never sent) order with its deterministic client
// order ID and current status. exchangeOrderId remains nil until execution.
type StoredIntent struct {
	ID              string  `json:"id"`
	PlanID          string  `json:"planId"`
	ClientOrderID   string  `json:"clientOrderId"`
	ExchangeOrderID *string `json:"exchangeOrderId"`
	Family          string  `json:"family"`
	Purpose         string  `json:"purpose"`
	PositionSide    string  `json:"positionSide"`
	Status          string  `json:"status"`
	Quantity        string  `json:"quantity"`
	Price           *string `json:"price"`
}

// SavePlanIntents decomposes an owned plan into deterministic order intents and
// persists them. It re-reads the stored plan (single source of truth) and never
// trusts a client payload. Idempotent: recomputing the same plan yields the same
// client order IDs, so repeated saves insert nothing new.
func (s *Store) SavePlanIntents(ctx context.Context, hash, account, planID string) (int, error) {
	if !ownerUUID.MatchString(account) || !ownerUUID.MatchString(planID) {
		return 0, ErrPlanMissing
	}
	inserted := 0
	err := s.withSession(ctx, hash, func(tx *sql.Tx, owner string) error {
		var snapshot []byte
		err := tx.QueryRowContext(ctx, `SELECT snapshot FROM strategy_plans WHERE id=$1 AND account_id=$2`, planID, account).Scan(&snapshot)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrPlanMissing
		}
		if err != nil {
			return err
		}
		var plan planner.Plan
		if err := decodeStrict(snapshot, &plan); err != nil {
			return ErrPlanInvalid
		}
		intents, err := planner.Intents(plan)
		if err != nil {
			return ErrPlanInvalid
		}
		for i, intent := range intents {
			clientOrderID := planner.IntentClientOrderID(planID, intent.Purpose, i)
			res, err := tx.ExecContext(ctx, `INSERT INTO order_intents(id,owner_id,account_id,plan_id,client_order_id,order_family,purpose,position_side,status,quantity,price) VALUES(gen_random_uuid(),$1,$2,$3,$4,$5,$6,$7,'PENDING',$8::numeric,$9::numeric) ON CONFLICT (account_id, client_order_id) DO NOTHING`, owner, account, planID, clientOrderID, intent.Family, intent.Purpose, intent.PositionSide, intent.Quantity, intent.Price)
			if err != nil {
				return err
			}
			n, err := res.RowsAffected()
			if err != nil {
				return err
			}
			inserted += int(n)
		}
		if inserted > 0 {
			if _, err := tx.ExecContext(ctx, `INSERT INTO audit_events(id,owner_id,action,resource_id,correlation_id,metadata) VALUES(gen_random_uuid(),$1,'plan.intents_saved',$2,gen_random_uuid(),jsonb_build_object('count',$3::int))`, owner, planID, inserted); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	return inserted, nil
}

// ListPlanIntents returns the deterministic intents of an owned plan, bounded
// and ascending by creation. It does not contact an exchange or change status.
func (s *Store) ListPlanIntents(ctx context.Context, hash, account, planID string) ([]StoredIntent, error) {
	if !ownerUUID.MatchString(account) || !ownerUUID.MatchString(planID) {
		return nil, ErrPlanMissing
	}
	result := []StoredIntent{}
	err := s.withSession(ctx, hash, func(tx *sql.Tx, _ string) error {
		rows, err := tx.QueryContext(ctx, `SELECT id::text,plan_id::text,client_order_id,exchange_order_id,order_family,purpose,position_side,status,trim_scale(quantity)::text,trim_scale(price)::text FROM order_intents WHERE plan_id=$1 AND account_id=$2 ORDER BY created_at,id`, planID, account)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var i StoredIntent
			var price sql.NullString
			if err := rows.Scan(&i.ID, &i.PlanID, &i.ClientOrderID, &i.ExchangeOrderID, &i.Family, &i.Purpose, &i.PositionSide, &i.Status, &i.Quantity, &price); err != nil {
				return err
			}
			if price.Valid {
				i.Price = &price.String
			}
			result = append(result, i)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}
