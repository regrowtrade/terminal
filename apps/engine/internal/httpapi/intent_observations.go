package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"

	"regrow/engine/internal/binance"
	"regrow/engine/internal/connections"
	"regrow/engine/internal/database"
)

func registerIntentObservations(mux *http.ServeMux, token string, service ConnectionService) {
	for _, operation := range []string{"observe", "observations"} {
		mux.HandleFunc("POST /v1/plans/intents/"+operation, func(w http.ResponseWriter, r *http.Request) {
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
				AccountID string `json:"accountId"`
				IntentID  string `json:"intentId"`
			}
			d := json.NewDecoder(r.Body)
			d.DisallowUnknownFields()
			if d.Decode(&input) != nil || d.Decode(&struct{}{}) != io.EOF || !validPlanHead(input.TokenHash, input.AccountID) || !planIDPattern.MatchString(input.IntentID) {
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
			if operation == "observe" {
				data, err = service.ObserveIntent(ctx, input.TokenHash, input.AccountID, input.IntentID)
			} else {
				data, err = service.IntentObservations(ctx, input.TokenHash, input.AccountID, input.IntentID)
			}
			switch {
			case errors.Is(err, database.ErrSessionInvalid):
				respond(w, 401, map[string]string{"error": "Sesión no válida"})
			case errors.Is(err, database.ErrIntentMissing), errors.Is(err, database.ErrAccountMissing):
				respond(w, 404, map[string]string{"error": "Intención no disponible"})
			case errors.Is(err, database.ErrIntentChanged):
				respond(w, 409, map[string]string{"error": database.ErrIntentChanged.Error()})
			case errors.Is(err, database.ErrIntentUnsupported), errors.Is(err, database.ErrPlanInvalid):
				respond(w, 422, map[string]string{"error": "Intención no compatible o plan no válido. No se enviaron órdenes."})
			case errors.Is(err, binance.ErrPermissions):
				respond(w, 422, map[string]string{"error": binance.ErrPermissions.Error()})
			case errors.Is(err, binance.ErrRateLimited):
				w.Header().Set("Retry-After", "60")
				respond(w, 429, map[string]string{"error": binance.ErrRateLimited.Error()})
			case err != nil:
				respond(w, 503, map[string]string{"error": "No se pudo investigar la intención. No se enviaron órdenes."})
			default:
				respond(w, 200, data)
			}
		})
	}
}
