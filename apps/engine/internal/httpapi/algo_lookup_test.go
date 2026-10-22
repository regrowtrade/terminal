package httpapi

import (
	"context"
	"errors"
	"net/http/httptest"
	"regrow/engine/internal/binance"
	"regrow/engine/internal/database"
	"strings"
	"testing"
)

func (f *fakeConnections) LookupAlgoOrder(context.Context, string, string, binance.AlgoQuery) (binance.AlgoLookup, error) {
	f.calls++
	return binance.AlgoLookup{}, f.err
}

func TestAlgoLookupHTTPBoundary(t *testing.T) {
	base := `"tokenHash":"` + strings.Repeat("a", 64) + `","accountId":"00000000-0000-4000-8000-000000000001","symbol":"BTCUSDT"`
	valid := `{` + base + `,"algoId":"9007199254740993"}`
	for _, tc := range []struct {
		name, body    string
		err           error
		status, calls int
	}{
		{"algo ID", valid, nil, 200, 1}, {"client ID", `{` + base + `,"clientAlgoId":"synthetic"}`, nil, 200, 1},
		{"missing selector", `{` + base + `}`, nil, 400, 0}, {"null", `{` + base + `,"algoId":"1","clientAlgoId":null}`, nil, 400, 0}, {"empty", `{` + base + `,"algoId":"1","clientAlgoId":""}`, nil, 400, 0}, {"both", `{` + base + `,"algoId":"1","clientAlgoId":"both"}`, nil, 400, 0}, {"numeric", `{` + base + `,"algoId":9007199254740993}`, nil, 400, 0}, {"overflow", `{` + base + `,"algoId":"9223372036854775808"}`, nil, 400, 0},
		{"normal selector", `{` + base + `,"orderId":"1"}`, nil, 400, 0}, {"owner injection", strings.TrimSuffix(valid, "}") + `,"ownerId":"attacker"}`, nil, 400, 0}, {"trailing", valid + ` {}`, nil, 400, 0}, {"size", valid + strings.Repeat(" ", 1024), nil, 400, 0},
		{"revoked", valid, database.ErrSessionInvalid, 401, 1}, {"foreign", valid, database.ErrAccountMissing, 404, 1}, {"permissions", valid, binance.ErrPermissions, 422, 1}, {"cooldown", valid, binance.ErrRateLimited, 429, 1}, {"query", valid, binance.ErrAlgoQuery, 400, 1}, {"save", valid, errors.New("private-signed-url"), 503, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			service := &fakeConnections{err: tc.err}
			r := httptest.NewRequest("POST", "/v1/connections/binance/algo", strings.NewReader(tc.body))
			r.Header.Set("Authorization", "Bearer "+testToken)
			r.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			HandlerWithConnections(testToken, nil, nil, service).ServeHTTP(w, r)
			if w.Code != tc.status || service.calls != tc.calls || strings.Contains(w.Body.String(), "private-signed-url") || w.Header().Get("Cache-Control") != "no-store" {
				t.Fatal("unsafe algo boundary", w.Code, service.calls)
			}
			if tc.status == 429 && w.Header().Get("Retry-After") == "" {
				t.Fatal("missing cooldown")
			}
		})
	}
	for _, tc := range []struct {
		auth, content string
		status        int
	}{{"", "application/json", 401}, {"Bearer " + testToken, "text/plain", 415}} {
		s := &fakeConnections{}
		r := httptest.NewRequest("POST", "/v1/connections/binance/algo", strings.NewReader(valid))
		r.Header.Set("Authorization", tc.auth)
		r.Header.Set("Content-Type", tc.content)
		w := httptest.NewRecorder()
		HandlerWithConnections(testToken, nil, nil, s).ServeHTTP(w, r)
		if w.Code != tc.status || s.calls != 0 {
			t.Fatal("authentication/format bypassed", w.Code)
		}
	}
	for _, op := range []string{"order", "snapshot", "latest", "list", "events", "comparison", "reconcile"} {
		s := &fakeConnections{}
		r := httptest.NewRequest("POST", "/v1/connections/binance/"+op, strings.NewReader(valid))
		r.Header.Set("Authorization", "Bearer "+testToken)
		r.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		HandlerWithConnections(testToken, nil, nil, s).ServeHTTP(w, r)
		if w.Code != 400 || s.calls != 0 {
			t.Fatal("algo selectors leaked into unrelated endpoint", op, w.Code)
		}
	}
}
