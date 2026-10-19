package httpapi

import (
	"errors"
	"net/http/httptest"
	"strings"
	"testing"

	"regrow/engine/internal/binance"
	"regrow/engine/internal/database"
)

func TestOrderLookupBoundary(t *testing.T) {
	base := `"tokenHash":"` + strings.Repeat("a", 64) + `","accountId":"00000000-0000-4000-8000-000000000001","symbol":"BTCUSDT"`
	valid := `{` + base + `,"orderId":"9007199254740993"}`
	for _, tc := range []struct {
		name, body    string
		err           error
		status, calls int
	}{
		{"order ID", valid, nil, 200, 1},
		{"client ID", `{` + base + `,"clientOrderId":"synthetic"}`, nil, 200, 1},
		{"missing selector", `{` + base + `}`, nil, 400, 0},
		{"null selector", `{` + base + `,"orderId":"1","clientOrderId":null}`, nil, 400, 0},
		{"empty selector", `{` + base + `,"orderId":"1","clientOrderId":""}`, nil, 400, 0},
		{"both", `{` + base + `,"orderId":"1","clientOrderId":"synthetic"}`, nil, 400, 0},
		{"unsafe int64", `{` + base + `,"orderId":"9223372036854775808"}`, nil, 400, 0},
		{"numeric ID", `{` + base + `,"orderId":9007199254740993}`, nil, 400, 0},
		{"after", strings.TrimSuffix(valid, "}") + `,"after":1}`, nil, 400, 0},
		{"injection", strings.TrimSuffix(valid, "}") + `,"ownerId":"attacker"}`, nil, 400, 0},
		{"trailing", valid + ` {}`, nil, 400, 0},
		{"revoked", valid, database.ErrSessionInvalid, 401, 1},
		{"other owner", valid, database.ErrAccountMissing, 404, 1},
		{"permissions", valid, binance.ErrPermissions, 422, 1},
		{"cooldown", valid, binance.ErrRateLimited, 429, 1},
		{"invalid query", valid, binance.ErrOrderQuery, 400, 1},
		{"save failure", valid, errors.New("private-signed-url"), 503, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			service := &fakeConnections{err: tc.err}
			r := httptest.NewRequest("POST", "/v1/connections/binance/order", strings.NewReader(tc.body))
			r.Header.Set("Authorization", "Bearer "+testToken)
			r.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			HandlerWithConnections(testToken, nil, nil, service).ServeHTTP(w, r)
			if w.Code != tc.status || service.calls != tc.calls || strings.Contains(w.Body.String(), "private-signed-url") || w.Header().Get("Cache-Control") != "no-store" {
				t.Fatal("unsafe order lookup boundary", w.Code, service.calls)
			}
		})
	}
	for _, op := range []string{"list", "latest", "snapshot", "comparison", "events", "reconcile"} {
		service := &fakeConnections{}
		r := httptest.NewRequest("POST", "/v1/connections/binance/"+op, strings.NewReader(valid))
		r.Header.Set("Authorization", "Bearer "+testToken)
		r.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		HandlerWithConnections(testToken, nil, nil, service).ServeHTTP(w, r)
		if w.Code != 400 || service.calls != 0 {
			t.Fatal("order selectors accepted on unrelated endpoint", op)
		}
	}
}
