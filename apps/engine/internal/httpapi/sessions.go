package httpapi

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"regrow/engine/internal/database"
)

type SessionStore interface {
	CreateSession(context.Context, string) (database.Identity, error)
	ResolveSession(context.Context, string) (database.Identity, error)
	RevokeSession(context.Context, string, bool) error
}

func authorized(r *http.Request, token string) bool {
	return len(token) >= 32 && !strings.HasPrefix(token, "replace-with-") && subtle.ConstantTimeCompare([]byte(r.Header.Get("Authorization")), []byte("Bearer "+token)) == 1
}

func registerSessions(mux *http.ServeMux, token string, store SessionStore) {
	for _, operation := range []string{"create", "resolve", "revoke", "revoke-all"} {
		mux.HandleFunc("POST /v1/auth/sessions/"+operation, func(w http.ResponseWriter, r *http.Request) {
			if !authorized(r, token) {
				respond(w, 401, map[string]string{"error": "No autorizado"})
				return
			}
			if r.Header.Get("Content-Type") != "application/json" {
				respond(w, 415, map[string]string{"error": "Se requiere application/json"})
				return
			}
			r.Body = http.MaxBytesReader(w, r.Body, 1024)
			defer r.Body.Close()
			var input struct {
				TokenHash string `json:"tokenHash"`
			}
			decoder := json.NewDecoder(r.Body)
			decoder.DisallowUnknownFields()
			if err := decoder.Decode(&input); err != nil {
				respond(w, 400, map[string]string{"error": "Solicitud inválida"})
				return
			}
			if decoder.Decode(&struct{}{}) != io.EOF || len(input.TokenHash) != 64 || strings.Trim(input.TokenHash, "0123456789abcdef") != "" {
				respond(w, 400, map[string]string{"error": "Solicitud inválida"})
				return
			}
			if store == nil {
				respond(w, 503, map[string]string{"error": "Sesiones persistidas no disponibles"})
				return
			}
			ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
			defer cancel()
			var identity database.Identity
			var err error
			switch operation {
			case "create":
				identity, err = store.CreateSession(ctx, input.TokenHash)
			case "resolve":
				identity, err = store.ResolveSession(ctx, input.TokenHash)
			default:
				err = store.RevokeSession(ctx, input.TokenHash, operation == "revoke-all")
			}
			if errors.Is(err, database.ErrSessionInvalid) {
				respond(w, 401, map[string]string{"error": "Sesión no válida"})
				return
			}
			if err != nil {
				respond(w, 503, map[string]string{"error": "Sesiones persistidas no disponibles"})
				return
			}
			if operation == "create" || operation == "resolve" {
				respond(w, 200, identity)
				return
			}
			respond(w, 200, map[string]bool{"ok": true})
		})
	}
}
