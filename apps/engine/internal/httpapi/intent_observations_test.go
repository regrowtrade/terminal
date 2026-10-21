package httpapi

import (
	"errors"
	"net/http/httptest"
	"strings"
	"testing"

	"regrow/engine/internal/binance"
	"regrow/engine/internal/database"
)

func TestIntentObservationBoundaries(t *testing.T) {
	valid := `{"tokenHash":"` + strings.Repeat("a", 64) + `","accountId":"00000000-0000-4000-8000-000000000001","intentId":"00000000-0000-4000-8000-000000000002"}`
	for _, op := range []string{"observe", "observations"} {
		for _, tc := range []struct {
			name, body, token, content string
			err                        error
			status, calls              int
		}{
			{"valid", valid, testToken, "application/json", nil, 200, 1},
			{"unauthorized", valid, "", "application/json", nil, 401, 0},
			{"content type", valid, testToken, "text/plain", nil, 415, 0},
			{"missing", `{}`, testToken, "application/json", nil, 400, 0},
			{"symbol injection", strings.TrimSuffix(valid, "}") + `,"symbol":"ETHUSDT"}`, testToken, "application/json", nil, 400, 0},
			{"order injection", strings.TrimSuffix(valid, "}") + `,"orderId":"123"}`, testToken, "application/json", nil, 400, 0},
			{"owner injection", strings.TrimSuffix(valid, "}") + `,"ownerId":"other"}`, testToken, "application/json", nil, 400, 0},
			{"concatenated", valid + `{}`, testToken, "application/json", nil, 400, 0},
			{"revoked", valid, testToken, "application/json", database.ErrSessionInvalid, 401, 1},
			{"foreign", valid, testToken, "application/json", database.ErrIntentMissing, 404, 1},
			{"changed", valid, testToken, "application/json", database.ErrIntentChanged, 409, 1},
			{"unsupported", valid, testToken, "application/json", database.ErrIntentUnsupported, 422, 1},
			{"rate limited", valid, testToken, "application/json", binance.ErrRateLimited, 429, 1},
			{"private error", valid, testToken, "application/json", errors.New("private-signed-url"), 503, 1},
		} {
			t.Run(op+"/"+tc.name, func(t *testing.T) {
				s := &fakeConnections{err: tc.err}
				r := httptest.NewRequest("POST", "/v1/plans/intents/"+op, strings.NewReader(tc.body))
				r.Header.Set("Authorization", "Bearer "+tc.token)
				r.Header.Set("Content-Type", tc.content)
				w := httptest.NewRecorder()
				HandlerWithConnections(testToken, nil, nil, s).ServeHTTP(w, r)
				if w.Code != tc.status || s.calls != tc.calls || strings.Contains(w.Body.String(), "private-signed-url") || w.Header().Get("Cache-Control") != "no-store" {
					t.Fatal("unsafe intent boundary", w.Code, s.calls)
				}
				if tc.status == 429 && w.Header().Get("Retry-After") != "60" {
					t.Fatal("missing cooldown")
				}
			})
		}
	}
}
