package database

import (
	"context"
	"database/sql"

	"regrow/engine/internal/binance"
)

// saveFills upserts TRADE executions extracted from already-validated events
// into the durable deduplicated fills table. The (account, order, trade)
// primary key makes at-least-once redelivery idempotent; fills are never
// overwritten or deleted by the runtime.
func saveFills(ctx context.Context, tx *sql.Tx, account, owner string, events []binance.UserEvent) error {
	for _, event := range events {
		fill := event.Fill()
		if fill == nil {
			continue
		}
		_, err := tx.ExecContext(ctx, `INSERT INTO binance_fills(account_id,owner_id,order_id,trade_id,symbol,side,position_side,price,quantity,commission,commission_asset,event_time,transaction_time) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13) ON CONFLICT (account_id, order_id, trade_id) DO NOTHING`, account, owner, fill.OrderID, fill.TradeID, fill.Symbol, fill.Side, fill.PositionSide, fill.Price, fill.Quantity, fill.Commission, fill.CommissionAsset, fill.EventTime, fill.TransactionTime)
		if err != nil {
			return err
		}
	}
	return nil
}

// maxRecoveredFills bounds a manual REST fill recovery against abuse; it matches
// the client-side page bound (maxTradePages x tradePageSize).
const maxRecoveredFills = 20000

// RecoveredFillsResult reports how many recovered fills were newly inserted
// versus already-known duplicates. It never implies continuity or execution.
type RecoveredFillsResult struct {
	Recovered  int `json:"recovered"`
	Duplicates int `json:"duplicates"`
}

// SaveRecoveredFills revalidates session and credential ownership, then upserts
// already-validated fills into the durable deduplicated table in one
// transaction. Nothing is updated, overwritten or deleted; only insertion with
// ON CONFLICT DO NOTHING is allowed, so an ambiguous insert is never a fill.
func (s *Store) SaveRecoveredFills(ctx context.Context, hash, account string, fills []binance.Fill) (RecoveredFillsResult, error) {
	if !ownerUUID.MatchString(account) {
		return RecoveredFillsResult{}, ErrAccountMissing
	}
	if fills == nil || len(fills) > maxRecoveredFills {
		return RecoveredFillsResult{}, ErrSnapshotInvalid
	}
	for _, fill := range fills {
		if binance.ValidateRecoveredFill(fill) != nil {
			return RecoveredFillsResult{}, ErrSnapshotInvalid
		}
	}
	var result RecoveredFillsResult
	err := s.withSession(ctx, hash, func(tx *sql.Tx, owner string) error {
		var exists bool
		if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM exchange_accounts a JOIN exchange_credentials c ON c.account_id=a.id AND c.owner_id=a.owner_id WHERE a.id=$1 AND a.owner_id=$2 AND a.exchange='binance' AND a.environment='production')`, account, owner).Scan(&exists); err != nil {
			return err
		}
		if !exists {
			return ErrAccountMissing
		}
		for _, fill := range fills {
			res, err := tx.ExecContext(ctx, `INSERT INTO binance_fills(account_id,owner_id,order_id,trade_id,symbol,side,position_side,price,quantity,commission,commission_asset,event_time,transaction_time) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13) ON CONFLICT (account_id, order_id, trade_id) DO NOTHING`, account, owner, fill.OrderID, fill.TradeID, fill.Symbol, fill.Side, fill.PositionSide, fill.Price, fill.Quantity, fill.Commission, fill.CommissionAsset, fill.EventTime, fill.TransactionTime)
			if err != nil {
				return err
			}
			n, err := res.RowsAffected()
			if err != nil {
				return err
			}
			if n == 0 {
				result.Duplicates++
			} else {
				result.Recovered++
			}
		}
		_, err := tx.ExecContext(ctx, `INSERT INTO audit_events(id,owner_id,action,resource_id,correlation_id,metadata) VALUES(gen_random_uuid(),$1,'account.fills_recovered',$2,gen_random_uuid(),jsonb_build_object('recovered',$3::int,'duplicates',$4::int))`, owner, account, result.Recovered, result.Duplicates)
		return err
	})
	if err != nil {
		return RecoveredFillsResult{}, err
	}
	return result, nil
}

// ListFills returns the bounded, ascending deduplicated fills with event_time
// strictly after the given exchange watermark, the complete total count and
// whether the page was truncated. It requires a valid session and ownership.
func (s *Store) ListFills(ctx context.Context, hash, account string, after int64, limit int) ([]binance.Fill, int, bool, error) {
	if !ownerUUID.MatchString(account) {
		return nil, 0, false, ErrAccountMissing
	}
	if limit <= 0 || limit > 200 {
		limit = 200
	}
	result := []binance.Fill{}
	total := 0
	truncated := false
	err := s.withSession(ctx, hash, func(tx *sql.Tx, _ string) error {
		var exists bool
		if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM exchange_accounts WHERE id=$1 AND exchange='binance' AND environment='production')`, account).Scan(&exists); err != nil {
			return err
		}
		if !exists {
			return ErrAccountMissing
		}
		if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM binance_fills WHERE account_id=$1 AND event_time>$2`, account, after).Scan(&total); err != nil {
			return err
		}
		rows, err := tx.QueryContext(ctx, `SELECT order_id,trade_id,symbol,side,position_side,price,quantity,commission,commission_asset,event_time,transaction_time FROM binance_fills WHERE account_id=$1 AND event_time>$2 ORDER BY event_time, transaction_time, received_at LIMIT $3`, account, after, limit+1)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var f binance.Fill
			if err := rows.Scan(&f.OrderID, &f.TradeID, &f.Symbol, &f.Side, &f.PositionSide, &f.Price, &f.Quantity, &f.Commission, &f.CommissionAsset, &f.EventTime, &f.TransactionTime); err != nil {
				return err
			}
			result = append(result, f)
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
		return nil, 0, false, err
	}
	return result, total, truncated, nil
}
