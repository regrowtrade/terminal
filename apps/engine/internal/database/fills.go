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
