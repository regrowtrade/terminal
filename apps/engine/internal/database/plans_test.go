package database

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"regrow/engine/internal/planner"
)

func syntheticPlanRequest() planner.Request {
	return planner.Request{
		Mode: "clean", Symbol: "BTCUSDT", Side: "LONG", Entry: "100",
		Quantity: "1", Risk: "2", RewardRatio: "3", RiskUnit: "amount",
		AccountReference: "100", AccountBasis: "equity", SpacingPercent: "4",
		Leverage: 10, MaxLevels: 1, MaxMargin: "100", CostReserve: "0",
	}
}

func TestSavePlanRejectsInvalidInputBeforeDatabase(t *testing.T) {
	store := &Store{}
	if _, err := store.SavePlan(context.Background(), "synthetic", "invalid", syntheticPlanRequest()); !errors.Is(err, ErrAccountMissing) {
		t.Fatal("invalid account accepted")
	}
	bad := syntheticPlanRequest()
	bad.Symbol = ""
	if _, err := store.SavePlan(context.Background(), "synthetic", "00000000-0000-4000-8000-000000000001", bad); !errors.Is(err, ErrPlanInvalid) {
		t.Fatal("invalid request persisted")
	}
	if _, err := store.ListPlans(context.Background(), "synthetic", "invalid", 20); !errors.Is(err, ErrAccountMissing) {
		t.Fatal("invalid account accepted for list")
	}
}

// Runs only under TEST_DATABASE_URL (CI): durable round-trip, RLS and audit.
func testPlanLifecycle(t *testing.T, ctx context.Context, store *Store, admin *sql.DB, hash, account string) {
	t.Helper()
	request := syntheticPlanRequest()
	saved, err := store.SavePlan(ctx, hash, account, request)
	if err != nil {
		t.Fatal("could not persist strategy plan", err)
	}
	if saved.CampaignID == "" || saved.PlanID == "" || saved.Version != 1 {
		t.Fatal("plan identifiers or version missing")
	}
	plans, err := store.ListPlans(ctx, hash, account, 20)
	if err != nil || len(plans) != 1 {
		t.Fatal("persisted plan not listed", err)
	}
	p := plans[0]
	if p.CampaignID != saved.CampaignID || p.PlanID != saved.PlanID || p.Mode != "clean" || p.Symbol != "BTCUSDT" || p.RiskAmount != "2" || p.Plan.Simulation != true || len(p.Plan.Levels) != 1 {
		t.Fatal("plan round-trip lost identity, mode, risk or levels")
	}
	if p.Parameters != request {
		t.Fatal("persisted request parameters diverged from the source")
	}
	var risk string
	if err := admin.QueryRowContext(ctx, `SELECT trim_scale(risk_amount)::text FROM strategy_plans WHERE id=$1`, saved.PlanID).Scan(&risk); err != nil || risk != "2" {
		t.Fatal("risk amount not persisted exactly", err)
	}
	var audits int
	if err := admin.QueryRowContext(ctx, `SELECT count(*) FROM audit_events WHERE action='plan.saved' AND resource_id=$1`, saved.PlanID).Scan(&audits); err != nil || audits != 1 {
		t.Fatal("plan audit missing or duplicated", err)
	}
	// Deterministic order intents decompose the plan once and stay idempotent.
	inserted, err := store.SavePlanIntents(ctx, hash, account, saved.PlanID)
	if err != nil || inserted != 3 {
		t.Fatal("clean plan did not produce entry+tp+sl intents", err, inserted)
	}
	intents, err := store.ListPlanIntents(ctx, hash, account, saved.PlanID)
	if err != nil || len(intents) != 3 {
		t.Fatal("persisted intents not listed", err)
	}
	byPurpose := map[string]StoredIntent{}
	for _, in := range intents {
		byPurpose[in.Purpose] = in
	}
	last := p.Plan.Levels[0]
	if e := byPurpose["ENTRY"]; e.Family != "regular" || e.PositionSide != "LONG" || e.Quantity != last.Quantity || e.Price == nil || *e.Price != last.Price {
		t.Fatal("entry intent lost family, side, quantity or price")
	}
	if tp := byPurpose["TP"]; tp.Family != "conditional" || tp.Price == nil || *tp.Price != *last.TakeProfit || tp.Quantity != last.TakeProfitQuantity {
		t.Fatal("tp intent lost trigger or quantity")
	}
	if sl := byPurpose["SL"]; sl.Family != "conditional" || sl.Price == nil || *sl.Price != *last.StopPrice || sl.Quantity != last.CumulativeQuantity {
		t.Fatal("sl intent lost trigger or quantity")
	}
	if byPurpose["ENTRY"].ClientOrderID != planner.IntentClientOrderID(saved.PlanID, "ENTRY", 0) ||
		byPurpose["TP"].ClientOrderID != planner.IntentClientOrderID(saved.PlanID, "TP", 1) ||
		byPurpose["SL"].ClientOrderID != planner.IntentClientOrderID(saved.PlanID, "SL", 2) {
		t.Fatal("client order ids not deterministic")
	}
	if again, err := store.SavePlanIntents(ctx, hash, account, saved.PlanID); err != nil || again != 0 {
		t.Fatal("intent save not idempotent", err, again)
	}
	// A second save is an independent DRAFT campaign, never an overwrite.
	again, err := store.SavePlan(ctx, hash, account, request)
	if err != nil || again.PlanID == saved.PlanID || again.CampaignID == saved.CampaignID {
		t.Fatal("repeat save reused the previous plan or campaign")
	}
	if plans, _ := store.ListPlans(ctx, hash, account, 20); len(plans) != 2 {
		t.Fatal("repeat save did not append a distinct plan")
	}
	// Ownership isolation: a different owner sees nothing under forced RLS.
	if err := store.WithOwner(ctx, "00000000-0000-4000-8000-000000000001", func(tx *sql.Tx) error {
		var count int
		if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM strategy_plans`).Scan(&count); err != nil {
			return err
		}
		if count != 0 {
			return errors.New("plan leaked across owner")
		}
		if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM order_intents`).Scan(&count); err != nil {
			return err
		}
		if count != 0 {
			return errors.New("intent leaked across owner")
		}
		return nil
	}); err != nil {
		t.Fatal("plan RLS failed", err)
	}
	// Unbound pooled connections must not reveal any plan or intent.
	if err := store.db.QueryRowContext(ctx, `SELECT count(*) FROM strategy_plans`).Scan(&audits); err != nil || audits != 0 {
		t.Fatal("unbound runtime can read plans")
	}
	if err := store.db.QueryRowContext(ctx, `SELECT count(*) FROM order_intents`).Scan(&audits); err != nil || audits != 0 {
		t.Fatal("unbound runtime can read intents")
	}
	// An unregistered session cannot persist or list.
	if _, err := store.SavePlan(ctx, testDigest(), account, request); !errors.Is(err, ErrSessionInvalid) {
		t.Fatal("unregistered session persisted a plan")
	}
	if _, err := store.ListPlans(ctx, testDigest(), account, 20); !errors.Is(err, ErrSessionInvalid) {
		t.Fatal("unregistered session listed plans")
	}
	if _, err := store.SavePlanIntents(ctx, testDigest(), account, saved.PlanID); !errors.Is(err, ErrSessionInvalid) {
		t.Fatal("unregistered session persisted intents")
	}
	if _, err := store.ListPlanIntents(ctx, testDigest(), account, saved.PlanID); !errors.Is(err, ErrSessionInvalid) {
		t.Fatal("unregistered session listed intents")
	}
	t.Run("intent_observations", func(t *testing.T) {
		testIntentObservationLifecycle(t, ctx, store, admin, hash, account, byPurpose["ENTRY"], byPurpose["TP"])
	})
}
