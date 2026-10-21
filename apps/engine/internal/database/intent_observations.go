package database

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/shopspring/decimal"
	"regrow/engine/internal/binance"
	"regrow/engine/internal/intentcheck"
	"regrow/engine/internal/planner"
)

var ErrIntentMissing = errors.New("intención no disponible")
var ErrIntentUnsupported = errors.New("solo se investigan entradas y recompras normales")
var ErrIntentChanged = errors.New("la intención o su plan cambió durante la consulta")

type StoredIntentObservation struct {
	ID       string             `json:"id"`
	StoredAt time.Time          `json:"storedAt"`
	Report   intentcheck.Report `json:"report"`
}

// readIntentExpected checks the stored decomposition against the plan, not just
// a caller-supplied symbol/ID. It rejects unsupported families before any read.
func readIntentExpected(ctx context.Context, tx *sql.Tx, account, intent string) (intentcheck.Expected, error) {
	var e intentcheck.Expected
	var family, instrument, mode, environment string
	var price sql.NullString
	var snapshot []byte
	var simulation bool
	err := tx.QueryRowContext(ctx, `SELECT i.id::text,i.plan_id::text,i.client_order_id,i.purpose,i.position_side,trim_scale(i.quantity)::text,trim_scale(i.price)::text,i.order_family,p.snapshot,c.instrument,p.mode,p.simulation,a.environment
		FROM order_intents i JOIN strategy_plans p ON (p.id,p.account_id,p.owner_id)=(i.plan_id,i.account_id,i.owner_id)
		JOIN trade_campaigns c ON (c.id,c.account_id,c.owner_id)=(p.campaign_id,p.account_id,p.owner_id)
		JOIN exchange_accounts a ON (a.id,a.owner_id)=(i.account_id,i.owner_id)
		WHERE i.id=$1 AND i.account_id=$2 AND a.exchange='binance'`, intent, account).Scan(&e.IntentID, &e.PlanID, &e.ClientOrderID, &e.Purpose, &e.PositionSide, &e.Quantity, &price, &family, &snapshot, &instrument, &mode, &simulation, &environment)
	if errors.Is(err, sql.ErrNoRows) {
		return e, ErrIntentMissing
	}
	if err != nil {
		return e, err
	}
	if family != "regular" || (e.Purpose != "ENTRY" && e.Purpose != "REBUY") || environment != "production" {
		return e, ErrIntentUnsupported
	}
	var plan planner.Plan
	if len(snapshot) > maxPlanJSON || decodeStrict(snapshot, &plan) != nil || !simulation || !plan.Simulation || plan.Mode != mode || plan.Symbol != instrument || plan.Side != e.PositionSide || !price.Valid {
		return e, ErrPlanInvalid
	}
	e.Symbol, e.Price = plan.Symbol, price.String
	sum := sha256.Sum256(snapshot)
	e.PlanFingerprint = hex.EncodeToString(sum[:])
	if !e.Valid() {
		return e, ErrPlanInvalid
	}
	intents, err := planner.Intents(plan)
	if err != nil {
		return e, ErrPlanInvalid
	}
	for index, candidate := range intents {
		if planner.IntentClientOrderID(e.PlanID, candidate.Purpose, index) != e.ClientOrderID {
			continue
		}
		qty, qerr := decimal.NewFromString(candidate.Quantity)
		p, perr := decimal.NewFromString(candidate.Price)
		if qerr == nil && perr == nil && candidate.Family == family && candidate.Purpose == e.Purpose && candidate.PositionSide == e.PositionSide && qty.Equal(decimal.RequireFromString(e.Quantity)) && p.Equal(decimal.RequireFromString(e.Price)) {
			return e, nil
		}
	}
	return e, ErrPlanInvalid
}

func (s *Store) ReadIntentExpected(ctx context.Context, hash, account, intent string) (intentcheck.Expected, error) {
	if !ownerUUID.MatchString(account) || !ownerUUID.MatchString(intent) {
		return intentcheck.Expected{}, ErrIntentMissing
	}
	var result intentcheck.Expected
	err := s.withSession(ctx, hash, func(tx *sql.Tx, _ string) error {
		var err error
		result, err = readIntentExpected(ctx, tx, account, intent)
		return err
	})
	if err != nil {
		return intentcheck.Expected{}, err
	}
	return result, nil
}

// SaveIntentObservation rechecks session, account credentials and the persisted
// decision after external I/O. Report + audit commit together; nothing updates
// order_intents, snapshots, continuity, executions or fills.
func (s *Store) SaveIntentObservation(ctx context.Context, hash, account string, expected intentcheck.Expected, observation binance.OrderLookup) (StoredIntentObservation, error) {
	if !ownerUUID.MatchString(account) {
		return StoredIntentObservation{}, ErrAccountMissing
	}
	if _, err := intentcheck.Assess(expected, observation); err != nil {
		return StoredIntentObservation{}, ErrSnapshotInvalid
	}
	observation.StartedAt = observation.StartedAt.UTC().Truncate(time.Microsecond)
	observation.CheckedAt = observation.CheckedAt.UTC().Truncate(time.Microsecond)
	if _, err := intentcheck.Assess(expected, observation); err != nil {
		return StoredIntentObservation{}, ErrSnapshotInvalid
	}
	var result StoredIntentObservation
	err := s.withSession(ctx, hash, func(tx *sql.Tx, owner string) error {
		current, err := readIntentExpected(ctx, tx, account, expected.IntentID)
		if err != nil {
			return err
		}
		if current != expected {
			return ErrIntentChanged
		}
		var exists bool
		if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM exchange_credentials WHERE account_id=$1 AND owner_id=$2)`, account, owner).Scan(&exists); err != nil {
			return err
		}
		if !exists {
			return ErrAccountMissing
		}
		result.Report, err = intentcheck.Assess(current, observation)
		if err != nil {
			return ErrSnapshotInvalid
		}
		data, err := json.Marshal(result.Report)
		if err != nil || len(data) > 32768 {
			return ErrSnapshotInvalid
		}
		if err := tx.QueryRowContext(ctx, `INSERT INTO binance_intent_observations(account_id,owner_id,intent_id,plan_id,report) VALUES($1,$2,$3,$4,$5) RETURNING id::text,stored_at`, account, owner, current.IntentID, current.PlanID, data).Scan(&result.ID, &result.StoredAt); err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO audit_events(id,owner_id,action,resource_id,correlation_id,metadata) VALUES(gen_random_uuid(),$1,'intent.observed',$2,gen_random_uuid(),jsonb_build_object('assessment',$3::text))`, owner, current.IntentID, result.Report.Assessment)
		return err
	})
	if err != nil {
		return StoredIntentObservation{}, err
	}
	return result, nil
}

// History requires ownership but no keys or current connection. Latest 20 only;
// it is historical evidence, never an implicit current result or full ledger.
func (s *Store) ListIntentObservations(ctx context.Context, hash, account, intent string) ([]StoredIntentObservation, error) {
	if !ownerUUID.MatchString(account) || !ownerUUID.MatchString(intent) {
		return nil, ErrIntentMissing
	}
	result := []StoredIntentObservation{}
	err := s.withSession(ctx, hash, func(tx *sql.Tx, _ string) error {
		var exists bool
		if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM order_intents WHERE id=$1 AND account_id=$2)`, intent, account).Scan(&exists); err != nil {
			return err
		}
		if !exists {
			return ErrIntentMissing
		}
		rows, err := tx.QueryContext(ctx, `SELECT id::text,stored_at,report FROM binance_intent_observations WHERE account_id=$1 AND intent_id=$2 ORDER BY stored_at DESC,id LIMIT 20`, account, intent)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var r StoredIntentObservation
			var data []byte
			if err := rows.Scan(&r.ID, &r.StoredAt, &data); err != nil {
				return err
			}
			if decodeStrict(data, &r.Report) != nil || intentcheck.Validate(r.Report) != nil || r.Report.Expected.IntentID != strings.ToLower(intent) {
				return ErrSnapshotInvalid
			}
			result = append(result, r)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}
