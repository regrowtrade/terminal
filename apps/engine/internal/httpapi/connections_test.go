package httpapi

import (
	"context"
	"errors"
	"net/http/httptest"
	"regrow/engine/internal/binance"
	"regrow/engine/internal/connections"
	"regrow/engine/internal/database"
	"regrow/engine/internal/reconcile"
	"strings"
	"testing"
)

type fakeConnections struct {
	err   error
	calls int
}

func (f *fakeConnections) State(context.Context, string) (connections.State, error) {
	f.calls++
	return connections.State{Accounts: []database.ReadAccount{}}, f.err
}
func (f *fakeConnections) Snapshot(context.Context, string, string) (binance.Snapshot, error) {
	f.calls++
	return binance.Snapshot{}, f.err
}
func (f *fakeConnections) Latest(context.Context, string, string) (database.StoredSnapshot, error) {
	f.calls++
	return database.StoredSnapshot{}, f.err
}
func (f *fakeConnections) Comparison(context.Context, string, string) (database.StoredComparison, error) {
	f.calls++
	return database.StoredComparison{}, f.err
}
func (f *fakeConnections) Events(context.Context, string, string, int64) ([]database.StoredEvent, bool, error) {
	f.calls++
	return nil, false, f.err
}
func (f *fakeConnections) Reconcile(context.Context, string, string) (reconcile.Report, error) {
	f.calls++
	return reconcile.Report{}, f.err
}
func TestConnectionBoundary(t *testing.T) {
	for _, tc := range []struct {
		name, body, token string
		err               error
		status            int
	}{
		{"unauthorized", `{}`, "", nil, 401},
		{"valid", `{"tokenHash":"` + strings.Repeat("a", 64) + `"}`, testToken, nil, 200},
		{"owner injection", `{"tokenHash":"` + strings.Repeat("a", 64) + `","userId":"attacker"}`, testToken, nil, 400},
		{"invalid digest", `{"tokenHash":"` + strings.Repeat("z", 64) + `"}`, testToken, nil, 400},
		{"revoked", `{"tokenHash":"` + strings.Repeat("a", 64) + `"}`, testToken, database.ErrSessionInvalid, 401},
		{"unknown account", `{"tokenHash":"` + strings.Repeat("a", 64) + `"}`, testToken, database.ErrAccountMissing, 404},
		{"permission", `{"tokenHash":"` + strings.Repeat("a", 64) + `"}`, testToken, binance.ErrPermissions, 422},
		{"cooldown", `{"tokenHash":"` + strings.Repeat("a", 64) + `"}`, testToken, binance.ErrRateLimited, 429},
		{"private error", `{"tokenHash":"` + strings.Repeat("a", 64) + `"}`, testToken, errors.New("driver signature-and-secret"), 503},
	} {
		t.Run(tc.name, func(t *testing.T) {
			service := &fakeConnections{err: tc.err}
			r := httptest.NewRequest("POST", "/v1/connections/binance/list", strings.NewReader(tc.body))
			r.Header.Set("Authorization", "Bearer "+tc.token)
			r.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			HandlerWithConnections(testToken, nil, nil, service).ServeHTTP(w, r)
			if w.Code != tc.status || strings.Contains(w.Body.String(), "signature-and-secret") || w.Header().Get("Cache-Control") != "no-store" {
				t.Fatal("unsafe connection response", w.Code)
			}
			if (tc.status == 400 || tc.token == "") && service.calls != 0 {
				t.Fatal("invalid request reached service")
			}
		})
	}
}
func TestStoredSnapshotBoundary(t *testing.T) {
	for _, tc := range []struct {
		err    error
		status int
	}{{nil, 200}, {database.ErrSnapshotMissing, 404}, {database.ErrSessionInvalid, 401}, {database.ErrAccountMissing, 404}, {errors.New("private-driver-details"), 503}} {
		service := &fakeConnections{err: tc.err}
		r := httptest.NewRequest("POST", "/v1/connections/binance/latest", strings.NewReader(`{"tokenHash":"`+strings.Repeat("a", 64)+`","accountId":"00000000-0000-4000-8000-000000000001"}`))
		r.Header.Set("Authorization", "Bearer "+testToken)
		r.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		HandlerWithConnections(testToken, nil, nil, service).ServeHTTP(w, r)
		if w.Code != tc.status || service.calls != 1 || strings.Contains(w.Body.String(), "private-driver-details") || w.Header().Get("Cache-Control") != "no-store" {
			t.Fatal("unsafe historical endpoint", w.Code)
		}
	}
}
func TestStoredComparisonBoundary(t *testing.T) {
	for _, tc := range []struct {
		err    error
		status int
	}{{nil, 200}, {database.ErrComparisonMissing, 404}, {database.ErrSessionInvalid, 401}, {database.ErrAccountMissing, 404}, {errors.New("private-driver-details"), 503}} {
		service := &fakeConnections{err: tc.err}
		r := httptest.NewRequest("POST", "/v1/connections/binance/comparison", strings.NewReader(`{"tokenHash":"`+strings.Repeat("a", 64)+`","accountId":"00000000-0000-4000-8000-000000000001"}`))
		r.Header.Set("Authorization", "Bearer "+testToken)
		r.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		HandlerWithConnections(testToken, nil, nil, service).ServeHTTP(w, r)
		if w.Code != tc.status || service.calls != 1 || strings.Contains(w.Body.String(), "private-driver-details") || w.Header().Get("Cache-Control") != "no-store" {
			t.Fatal("unsafe comparison response", w.Code)
		}
	}
}

func TestStoredEventsBoundary(t *testing.T) {
	valid := `{"tokenHash":"` + strings.Repeat("a", 64) + `","accountId":"00000000-0000-4000-8000-000000000001"}`
	for _, tc := range []struct {
		name, body string
		err        error
		status     int
		calls      int
	}{
		{"ok", valid, nil, 200, 1},
		{"revoked", valid, database.ErrSessionInvalid, 401, 1},
		{"unknown account", valid, database.ErrAccountMissing, 404, 1},
		{"invalid event", valid, database.ErrEventInvalid, 503, 1},
		{"negative after", strings.TrimSuffix(valid, "}") + `,"after":-1}`, nil, 400, 0},
		{"after on list", `{"tokenHash":"` + strings.Repeat("a", 64) + `","after":1}`, nil, 400, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			service := &fakeConnections{err: tc.err}
			op := "events"
			if tc.name == "after on list" {
				op = "list"
			}
			r := httptest.NewRequest("POST", "/v1/connections/binance/"+op, strings.NewReader(tc.body))
			r.Header.Set("Authorization", "Bearer "+testToken)
			r.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			HandlerWithConnections(testToken, nil, nil, service).ServeHTTP(w, r)
			if w.Code != tc.status || service.calls != tc.calls {
				t.Fatal("unsafe events response", w.Code, service.calls)
			}
		})
	}
}

func TestReconcileBoundary(t *testing.T) {
	valid := `{"tokenHash":"` + strings.Repeat("a", 64) + `","accountId":"00000000-0000-4000-8000-000000000001"}`
	for _, tc := range []struct {
		name, body string
		err        error
		status     int
		calls      int
	}{
		{"ok", valid, nil, 200, 1},
		{"revoked", valid, database.ErrSessionInvalid, 401, 1},
		{"unknown account", valid, database.ErrAccountMissing, 404, 1},
		{"missing snapshot", valid, database.ErrSnapshotMissing, 404, 1},
		{"after rejected", strings.TrimSuffix(valid, "}") + `,"after":1}`, nil, 400, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			service := &fakeConnections{err: tc.err}
			r := httptest.NewRequest("POST", "/v1/connections/binance/reconcile", strings.NewReader(tc.body))
			r.Header.Set("Authorization", "Bearer "+testToken)
			r.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			HandlerWithConnections(testToken, nil, nil, service).ServeHTTP(w, r)
			if w.Code != tc.status || service.calls != tc.calls {
				t.Fatal("unsafe reconcile response", w.Code, service.calls)
			}
		})
	}
}

func TestStoredComparisonRejectsUntrustedRequests(t *testing.T) {
	valid := `{"tokenHash":"` + strings.Repeat("a", 64) + `","accountId":"00000000-0000-4000-8000-000000000001"}`
	for _, tc := range []struct {
		name, body, token, contentType string
		status                         int
	}{
		{"anonymous", valid, "", "application/json", 401},
		{"wrong content type", valid, testToken, "text/plain", 415},
		{"owner injection", strings.TrimSuffix(valid, "}") + `,"ownerId":"attacker"}`, testToken, "application/json", 400},
		{"baseline injection", strings.TrimSuffix(valid, "}") + `,"previousSnapshot":{}}`, testToken, "application/json", 400},
		{"missing account", `{"tokenHash":"` + strings.Repeat("a", 64) + `"}`, testToken, "application/json", 400},
		{"invalid digest", strings.Replace(valid, strings.Repeat("a", 64), strings.Repeat("z", 64), 1), testToken, "application/json", 400},
		{"trailing input", valid + ` {}`, testToken, "application/json", 400},
	} {
		t.Run(tc.name, func(t *testing.T) {
			service := &fakeConnections{}
			r := httptest.NewRequest("POST", "/v1/connections/binance/comparison", strings.NewReader(tc.body))
			r.Header.Set("Authorization", "Bearer "+tc.token)
			r.Header.Set("Content-Type", tc.contentType)
			w := httptest.NewRecorder()
			HandlerWithConnections(testToken, nil, nil, service).ServeHTTP(w, r)
			if w.Code != tc.status || service.calls != 0 {
				t.Fatal("untrusted comparison request reached service", w.Code)
			}
		})
	}
}
