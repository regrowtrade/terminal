package httpapi

import (
	"context"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"regrow/engine/internal/database"
)

type fakeSessions struct {
	calls int
	err   error
	all   bool
}

func (s *fakeSessions) CreateSession(context.Context, string) (database.Identity, error) {
	s.calls++
	return database.Identity{OwnerID: "00000000-0000-4000-8000-000000000001", Role: "owner", ExpiresAt: time.Now().Add(time.Hour)}, s.err
}
func (s *fakeSessions) ResolveSession(ctx context.Context, hash string) (database.Identity, error) {
	return s.CreateSession(ctx, hash)
}
func (s *fakeSessions) RevokeSession(_ context.Context, _ string, all bool) error {
	s.calls++
	s.all = all
	return s.err
}

func TestPrivateSessionContract(t *testing.T) {
	hash := strings.Repeat("a", 64)
	for _, operation := range []string{"create", "resolve", "revoke", "revoke-all"} {
		t.Run(operation, func(t *testing.T) {
			for _, tc := range []struct {
				name, auth, body, content string
				storeErr                  error
				status                    int
			}{
				{"unauthorized", "", `{"tokenHash":"` + hash + `"}`, "application/json", nil, 401},
				{"valid", "Bearer " + testToken, `{"tokenHash":"` + hash + `"}`, "application/json", nil, 200},
				{"owner injection", "Bearer " + testToken, `{"tokenHash":"` + hash + `","ownerId":"attacker"}`, "application/json", nil, 400},
				{"concatenated", "Bearer " + testToken, `{"tokenHash":"` + hash + `"}{}`, "application/json", nil, 400},
				{"malformed", "Bearer " + testToken, `{"tokenHash":"cookie"}`, "application/json", nil, 400},
				{"invalid session", "Bearer " + testToken, `{"tokenHash":"` + hash + `"}`, "application/json", database.ErrSessionInvalid, 401},
				{"unavailable", "Bearer " + testToken, `{"tokenHash":"` + hash + `"}`, "application/json", errors.New("driver secret internal"), 503},
				{"wrong content", "Bearer " + testToken, `{}`, "text/plain", nil, 415},
			} {
				t.Run(tc.name, func(t *testing.T) {
					store := &fakeSessions{err: tc.storeErr}
					r := httptest.NewRequest("POST", "/v1/auth/sessions/"+operation, strings.NewReader(tc.body))
					r.Header.Set("Authorization", tc.auth)
					r.Header.Set("Content-Type", tc.content)
					w := httptest.NewRecorder()
					HandlerWithSessions(testToken, nil, store).ServeHTTP(w, r)
					if w.Code != tc.status || strings.Contains(w.Body.String(), hash) || strings.Contains(w.Body.String(), "driver secret") || w.Header().Get("Cache-Control") != "no-store" {
						t.Fatal("unsafe session contract", w.Code)
					}
					if (tc.status == 400 || tc.status == 401 && tc.storeErr == nil || tc.status == 415) && store.calls != 0 {
						t.Fatal("invalid request reached store")
					}
					if operation == "revoke-all" && tc.status == 200 && !store.all {
						t.Fatal("wrong revoke scope")
					}
				})
			}
		})
	}
}

func TestSessionPersistenceRequired(t *testing.T) {
	r := httptest.NewRequest("POST", "/v1/auth/sessions/create", strings.NewReader(`{"tokenHash":"`+strings.Repeat("a", 64)+`"}`))
	r.Header.Set("Authorization", "Bearer "+testToken)
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	Handler(testToken).ServeHTTP(w, r)
	if w.Code != 503 {
		t.Fatal("session created without persistence")
	}
}
