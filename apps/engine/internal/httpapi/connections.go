package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"regrow/engine/internal/binance"
	"regrow/engine/internal/connections"
	"regrow/engine/internal/database"
	"strings"
	"time"
)

type ConnectionService interface {
	State(context.Context, string) (connections.State, error)
	Snapshot(context.Context, string, string) (binance.Snapshot, error)
}

func registerConnections(mux *http.ServeMux, token string, service ConnectionService) {
	for _, operation := range []string{"list", "snapshot"} {
		mux.HandleFunc("POST /v1/connections/binance/"+operation, func(w http.ResponseWriter, r *http.Request) {
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
				AccountID string `json:"accountId,omitempty"`
			}
			d := json.NewDecoder(r.Body)
			d.DisallowUnknownFields()
			if d.Decode(&input) != nil || d.Decode(&struct{}{}) != io.EOF || len(input.TokenHash) != 64 || strings.Trim(input.TokenHash, "0123456789abcdef") != "" || (operation == "list" && input.AccountID != "") || (operation == "snapshot" && input.AccountID == "") {
				respond(w, 400, map[string]string{"error": "Solicitud inválida"})
				return
			}
			if service == nil {
				respond(w, 503, map[string]string{"error": connections.ErrDisabled.Error()})
				return
			}
			ctx, cancel := context.WithTimeout(r.Context(), 18*time.Second)
			defer cancel()
			var data any
			var err error
			if operation == "list" {
				data, err = service.State(ctx, input.TokenHash)
			} else {
				data, err = service.Snapshot(ctx, input.TokenHash, input.AccountID)
			}
			switch {
			case errors.Is(err, database.ErrSessionInvalid):
				respond(w, 401, map[string]string{"error": "Sesión no válida"})
			case errors.Is(err, database.ErrAccountMissing):
				respond(w, 404, map[string]string{"error": "Cuenta no disponible"})
			case errors.Is(err, binance.ErrPermissions):
				respond(w, 422, map[string]string{"error": binance.ErrPermissions.Error()})
			case errors.Is(err, binance.ErrRateLimited):
				w.Header().Set("Retry-After", "60")
				respond(w, 429, map[string]string{"error": binance.ErrRateLimited.Error()})
			case errors.Is(err, connections.ErrDisabled):
				respond(w, 503, map[string]string{"error": connections.ErrDisabled.Error()})
			case err != nil:
				respond(w, 503, map[string]string{"error": "No se pudo leer la cuenta de forma segura. No se enviaron órdenes."})
			default:
				respond(w, 200, data)
			}
		})
	}
}
