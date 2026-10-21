package httpapi

import (
	"errors"
	"net/http/httptest"
	"strings"
	"testing"

	"regrow/engine/internal/binance"
	"regrow/engine/internal/database"
)

func TestFillRecoveryBoundaries(t *testing.T) {
	valid := `{"tokenHash":"` + strings.Repeat("a", 64) + `","accountId":"00000000-0000-4000-8000-000000000001","symbol":"BTCUSDT","startTime":1700000000000,"endTime":1700000086400}`
	for _, tc := range []struct {
		name, body, token, content string
		err                        error
		status, calls              int
	}{
		{"valid", valid, testToken, "application/json", nil, 200, 1},
		{"unauthorized", valid, "", "application/json", nil, 401, 0},
		{"content type", valid, testToken, "text/plain", nil, 415, 0},
		{"missing", `{}`, testToken, "application/json", nil, 400, 0},
		{"bad symbol", strings.Replace(valid, "BTCUSDT", "BTCUSDC", 1), testToken, "application/json", nil, 400, 0},
		{"inverted window", strings.Replace(valid, `"startTime":1700000000000,"endTime":1700000086400`, `"startTime":1700000086400,"endTime":1700000000000`, 1), testToken, "application/json", nil, 400, 0},
		{"huge window", strings.Replace(valid, `"endTime":1700000086400`, `"endTime":9999999999999`, 1), testToken, "application/json", nil, 400, 0},
		{"owner injection", strings.TrimSuffix(valid, "}") + `,"ownerId":"other"}`, testToken, "application/json", nil, 400, 0},
		{"concatenated", valid + `{}`, testToken, "application/json", nil, 400, 0},
		{"revoked", valid, testToken, "application/json", database.ErrSessionInvalid, 401, 1},
		{"foreign", valid, testToken, "application/json", database.ErrAccountMissing, 404, 1},
		{"permissions", valid, testToken, "application/json", binance.ErrPermissions, 422, 1},
		{"rate limited", valid, testToken, "application/json", binance.ErrRateLimited, 429, 1},
		{"private error", valid, testToken, "application/json", errors.New("private-signed-url"), 503, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			service := &fakeConnections{err: tc.err}
			r := httptest.NewRequest("POST", "/v1/connections/binance/fills/recover", strings.NewReader(tc.body))
			r.Header.Set("Authorization", "Bearer "+tc.token)
			r.Header.Set("Content-Type", tc.content)
			w := httptest.NewRecorder()
			HandlerWithConnections(testToken, nil, nil, service).ServeHTTP(w, r)
			if w.Code != tc.status || service.calls != tc.calls || strings.Contains(w.Body.String(), "private-signed-url") || w.Header().Get("Cache-Control") != "no-store" {
				t.Fatal("unsafe fill recovery boundary", w.Code, service.calls, w.Body.String())
			}
			if tc.status == 429 && w.Header().Get("Retry-After") != "60" {
				t.Fatal("missing cooldown")
			}
		})
	}
}
