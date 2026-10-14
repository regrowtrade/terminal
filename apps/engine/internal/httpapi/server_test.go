package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const testToken = "test-only-token-not-a-production-secret-123"
const valid = `{"mode":"clean","symbol":"BTCUSDT","side":"LONG","entry":"100","quantity":"1","risk":"2","riskUnit":"amount","accountReference":"100","accountBasis":"equity","rewardRatio":"3","spacingPercent":"4","leverage":10,"maxLevels":4,"maxMargin":"1000","costReserve":"0"}`

func TestSecurityAndValidation(t *testing.T) {
	for _, tc := range []struct {
		name, token, body, content string
		status                     int
	}{
		{"valid", testToken, valid, "application/json", 200},
		{"missing auth", "", valid, "application/json", 401},
		{"wrong auth", "bad", valid, "application/json", 401},
		{"wrong content", testToken, valid, "text/plain", 415},
		{"unknown field", testToken, strings.Replace(valid, `"mode":`, `"execute":true,"mode":`, 1), "application/json", 400},
		{"multiple objects", testToken, valid + valid, "application/json", 400},
		{"oversized", testToken, strings.Repeat(" ", 17000) + valid, "application/json", 400},
		{"bad risk", testToken, strings.Replace(valid, `"risk":"2"`, `"risk":"0"`, 1), "application/json", 422},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodPost, "/v1/plans/preview", strings.NewReader(tc.body))
			r.Header.Set("Authorization", "Bearer "+tc.token)
			r.Header.Set("Content-Type", tc.content)
			w := httptest.NewRecorder()
			Handler(testToken).ServeHTTP(w, r)
			if w.Code != tc.status {
				t.Fatalf("got %d, want %d: %s", w.Code, tc.status, w.Body.String())
			}
			if w.Header().Get("Cache-Control") != "no-store" {
				t.Fatal("missing cache protection")
			}
		})
	}
}
func TestNoExecutionEndpoint(t *testing.T) {
	r := httptest.NewRequest("POST", "/v1/orders", nil)
	w := httptest.NewRecorder()
	Handler(testToken).ServeHTTP(w, r)
	if w.Code != 404 {
		t.Fatal("execution must not be available")
	}
}
func TestUnconfiguredServerFailsClosed(t *testing.T) {
	r := httptest.NewRequest("POST", "/v1/plans/preview", strings.NewReader(valid))
	w := httptest.NewRecorder()
	Handler("").ServeHTTP(w, r)
	if w.Code != 401 {
		t.Fatal("unconfigured auth accepted")
	}
}
