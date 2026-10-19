package httpapi

import (
	"context"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"

	"regrow/engine/internal/database"
	"regrow/engine/internal/planner"
)

type fakePlans struct {
	err     error
	saved   database.SavedPlan
	plans   []database.StoredPlan
	intents []database.StoredIntent
	count   int
	calls   int
}

func (f *fakePlans) SavePlan(context.Context, string, string, planner.Request) (database.SavedPlan, error) {
	f.calls++
	return f.saved, f.err
}
func (f *fakePlans) ListPlans(context.Context, string, string, int) ([]database.StoredPlan, error) {
	f.calls++
	return f.plans, f.err
}
func (f *fakePlans) SavePlanIntents(context.Context, string, string, string) (int, error) {
	f.calls++
	return f.count, f.err
}
func (f *fakePlans) ListPlanIntents(context.Context, string, string, string) ([]database.StoredIntent, error) {
	f.calls++
	return f.intents, f.err
}

const planHead = `"tokenHash":"` + "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa" + `","accountId":"00000000-0000-4000-8000-000000000001"`

func TestPlanSaveBoundary(t *testing.T) {
	valid := `{` + planHead + `,"mode":"clean","symbol":"BTCUSDT","side":"LONG","entry":"100","quantity":"1","risk":"2","rewardRatio":"3","riskUnit":"amount","accountReference":"100","accountBasis":"equity","spacingPercent":"4","leverage":10,"maxLevels":1,"maxMargin":"100","costReserve":"0"}`
	for _, tc := range []struct {
		name, body string
		err        error
		status     int
		calls      int
	}{
		{"ok", valid, nil, 200, 1},
		{"revoked", valid, database.ErrSessionInvalid, 401, 1},
		{"account missing", valid, database.ErrAccountMissing, 404, 1},
		{"plan invalid", valid, database.ErrPlanInvalid, 422, 1},
		{"driver", valid, errors.New("private-signed-url"), 503, 1},
		{"owner injection", strings.TrimSuffix(valid, "}") + `,"ownerId":"attacker"}`, nil, 400, 0},
		{"bad digest", strings.Replace(valid, strings.Repeat("a", 64), strings.Repeat("z", 64), 1), nil, 400, 0},
		{"missing account", `{` + `"tokenHash":"` + strings.Repeat("a", 64) + `","mode":"clean","symbol":"BTCUSDT"}`, nil, 400, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			service := &fakePlans{err: tc.err, saved: database.SavedPlan{CampaignID: "00000000-0000-4000-8000-000000000002", PlanID: "00000000-0000-4000-8000-000000000003", Version: 1}}
			r := httptest.NewRequest("POST", "/v1/plans/save", strings.NewReader(tc.body))
			r.Header.Set("Authorization", "Bearer "+testToken)
			r.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			HandlerWithPlans(testToken, nil, nil, nil, service).ServeHTTP(w, r)
			if w.Code != tc.status || service.calls != tc.calls || strings.Contains(w.Body.String(), "private-signed-url") || w.Header().Get("Cache-Control") != "no-store" {
				t.Fatal("unsafe plan save boundary", w.Code, service.calls)
			}
		})
	}
}

func TestPlanReadBoundaries(t *testing.T) {
	plans := []database.StoredPlan{{CampaignID: "00000000-0000-4000-8000-000000000002", PlanID: "00000000-0000-4000-8000-000000000003", Version: 1}}
	intents := []database.StoredIntent{{ID: "00000000-0000-4000-8000-000000000004", PlanID: "00000000-0000-4000-8000-000000000003", Purpose: "ENTRY"}}
	for _, tc := range []struct {
		name, operation, body string
		err                   error
		status, calls         int
	}{
		{"list ok", "list", `{` + planHead + `}`, nil, 200, 1},
		{"intents save ok", "intents/save", `{` + planHead + `,"planId":"00000000-0000-4000-8000-000000000003"}`, nil, 200, 1},
		{"intents list ok", "intents/list", `{` + planHead + `,"planId":"00000000-0000-4000-8000-000000000003"}`, nil, 200, 1},
		{"intents save missing plan", "intents/save", `{` + planHead + `}`, nil, 400, 0},
		{"unknown field", "list", `{` + planHead + `,"ownerId":"attacker"}`, nil, 400, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			service := &fakePlans{err: tc.err, plans: plans, intents: intents, count: 3}
			r := httptest.NewRequest("POST", "/v1/plans/"+tc.operation, strings.NewReader(tc.body))
			r.Header.Set("Authorization", "Bearer "+testToken)
			r.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			HandlerWithPlans(testToken, nil, nil, nil, service).ServeHTTP(w, r)
			if w.Code != tc.status || service.calls != tc.calls {
				t.Fatal("unsafe plan read boundary", tc.operation, w.Code, service.calls)
			}
			if tc.operation == "intents/save" && w.Code == 200 && !strings.Contains(w.Body.String(), `"count":3`) {
				t.Fatal("intents save did not return count")
			}
		})
	}
}

func TestPlansRequireStoreAndAuthentication(t *testing.T) {
	r := httptest.NewRequest("POST", "/v1/plans/list", strings.NewReader(`{`+planHead+`}`))
	r.Header.Set("Authorization", "Bearer "+testToken)
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	HandlerWithPlans(testToken, nil, nil, nil, nil).ServeHTTP(w, r)
	if w.Code != 503 {
		t.Fatal("missing store did not fail closed", w.Code)
	}
	r = httptest.NewRequest("POST", "/v1/plans/list", strings.NewReader(`{`+planHead+`}`))
	r.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	HandlerWithPlans(testToken, nil, nil, nil, &fakePlans{}).ServeHTTP(w, r)
	if w.Code != 401 {
		t.Fatal("anonymous plan read allowed", w.Code)
	}
}
