package database

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"time"

	"regrow/engine/internal/planner"
)

var ErrPlanInvalid = errors.New("plan no válido; no se guardó")

// SavedPlan identifies a newly persisted campaign and its first strategy plan.
type SavedPlan struct {
	CampaignID string `json:"campaignId"`
	PlanID     string `json:"planId"`
	Version    int    `json:"version"`
}

// StoredPlan is the durable decision: the request that produced it, the full
// computed plan and its bounded metadata. It is not an execution instruction.
type StoredPlan struct {
	CampaignID string          `json:"campaignId"`
	PlanID     string          `json:"planId"`
	Version    int             `json:"version"`
	Mode       string          `json:"mode"`
	Symbol     string          `json:"symbol"`
	RiskAmount string          `json:"riskAmount"`
	Parameters planner.Request `json:"parameters"`
	Plan       planner.Plan    `json:"plan"`
	CreatedAt  time.Time       `json:"createdAt"`
}

const maxPlanJSON = 64 * 1024

// SavePlan recomputes the plan from the request (never trusting a client
// result), then persists a DRAFT campaign and its first strategy plan tied to
// an owned Binance account. Order intents and execution are intentionally out
// of scope here: a plan is a decision, not an instruction.
func (s *Store) SavePlan(ctx context.Context, hash, account string, request planner.Request) (SavedPlan, error) {
	if !ownerUUID.MatchString(account) {
		return SavedPlan{}, ErrAccountMissing
	}
	plan, err := planner.Preview(request)
	if err != nil {
		return SavedPlan{}, ErrPlanInvalid
	}
	parameters, err := json.Marshal(request)
	if err != nil || len(parameters) > maxPlanJSON {
		return SavedPlan{}, ErrPlanInvalid
	}
	snapshot, err := json.Marshal(plan)
	if err != nil || len(snapshot) > maxPlanJSON {
		return SavedPlan{}, ErrPlanInvalid
	}
	var baseline, threshold any
	if plan.BaselinePnl != nil {
		baseline = *plan.BaselinePnl
	}
	if plan.PnlThreshold != nil {
		threshold = *plan.PnlThreshold
	}
	var result SavedPlan
	err = s.withSession(ctx, hash, func(tx *sql.Tx, owner string) error {
		var exists bool
		if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM exchange_accounts WHERE id=$1 AND owner_id=$2 AND exchange='binance' AND environment IN ('testnet','production'))`, account, owner).Scan(&exists); err != nil {
			return err
		}
		if !exists {
			return ErrAccountMissing
		}
		if err := tx.QueryRowContext(ctx, `INSERT INTO trade_campaigns(id,owner_id,account_id,instrument,status) VALUES(gen_random_uuid(),$1,$2,$3,'DRAFT') RETURNING id::text`, owner, account, plan.Symbol).Scan(&result.CampaignID); err != nil {
			return err
		}
		if err := tx.QueryRowContext(ctx, `INSERT INTO strategy_plans(id,owner_id,account_id,campaign_id,mode,version,parameters,snapshot,risk_amount,baseline_pnl,pnl_threshold,simulation) VALUES(gen_random_uuid(),$1,$2,$3,$4,1,$5,$6,$7::numeric,$8::numeric,$9::numeric,true) RETURNING id::text,version`, owner, account, result.CampaignID, plan.Mode, parameters, snapshot, plan.Risk, baseline, threshold).Scan(&result.PlanID, &result.Version); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx, `INSERT INTO audit_events(id,owner_id,action,resource_id,correlation_id,metadata) VALUES(gen_random_uuid(),$1,'plan.saved',$2,gen_random_uuid(),jsonb_build_object('campaignId',$3::text,'mode',$4::text,'symbol',$5::text))`, owner, result.PlanID, result.CampaignID, plan.Mode, plan.Symbol)
		return err
	})
	if err != nil {
		return SavedPlan{}, err
	}
	return result, nil
}

func decodeStrict(data []byte, target any) error {
	d := json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	if d.Decode(target) != nil || d.Decode(&struct{}{}) != io.EOF {
		return ErrPlanInvalid
	}
	return nil
}

// ListPlans returns the most recent persisted plans for an owned account,
// bounded and ascending by creation. It does not contact an exchange and does
// not interpret plans as executions.
func (s *Store) ListPlans(ctx context.Context, hash, account string, limit int) ([]StoredPlan, error) {
	if !ownerUUID.MatchString(account) {
		return nil, ErrAccountMissing
	}
	if limit <= 0 || limit > 50 {
		limit = 20
	}
	result := []StoredPlan{}
	err := s.withSession(ctx, hash, func(tx *sql.Tx, owner string) error {
		var exists bool
		if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM exchange_accounts WHERE id=$1 AND owner_id=$2 AND exchange='binance')`, account, owner).Scan(&exists); err != nil {
			return err
		}
		if !exists {
			return ErrAccountMissing
		}
		rows, err := tx.QueryContext(ctx, `SELECT p.campaign_id::text,p.id::text,p.version,p.mode,c.instrument,trim_scale(p.risk_amount)::text,p.parameters,p.snapshot,p.created_at FROM strategy_plans p JOIN trade_campaigns c ON c.id=p.campaign_id AND c.owner_id=p.owner_id WHERE p.account_id=$1 ORDER BY p.created_at DESC,p.id LIMIT $2`, account, limit)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var p StoredPlan
			var parameters, snapshot []byte
			if err := rows.Scan(&p.CampaignID, &p.PlanID, &p.Version, &p.Mode, &p.Symbol, &p.RiskAmount, &parameters, &snapshot, &p.CreatedAt); err != nil {
				return err
			}
			if err := decodeStrict(parameters, &p.Parameters); err != nil {
				return err
			}
			if err := decodeStrict(snapshot, &p.Plan); err != nil {
				return err
			}
			result = append(result, p)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}
