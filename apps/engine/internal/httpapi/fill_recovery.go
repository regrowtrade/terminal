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

// registerFillRecovery exposes an explicit manual REST fill recovery: it reads a
// bounded userTrades window and upserts validated fills into the durable table.
// It never submits, cancels, infers continuity or enables execution.
func registerFillRecovery(mux *http.ServeMux, token string, service ConnectionService) {
	mux.HandleFunc("POST /v1/connections/binance/fills/recover", func(w http.ResponseWriter, r *http.Request) {
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
			Symbol    string `json:"symbol"`
			StartTime int64  `json:"startTime"`
			EndTime   int64  `json:"endTime"`
		}
		d := json.NewDecoder(r.Body)
		d.DisallowUnknownFields()
		if d.Decode(&input) != nil || d.Decode(&struct{}{}) != io.EOF || !validPlanHead(input.TokenHash, input.AccountID) {
			respond(w, 400, map[string]string{"error": "Solicitud inválida"})
			return
		}
		query := binance.TradeQuery{Symbol: input.Symbol, StartTime: input.StartTime, EndTime: input.EndTime}
		if !query.Valid() {
			respond(w, 400, map[string]string{"error": binance.ErrTradeQuery.Error()})
			return
		}
		if service == nil {
			respond(w, 503, map[string]string{"error": connections.ErrDisabled.Error()})
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
		defer cancel()
		data, err := service.RecoverFills(ctx, input.TokenHash, input.AccountID, query)
		switch {
		case errors.Is(err, binance.ErrTradeQuery):
			respond(w, 400, map[string]string{"error": binance.ErrTradeQuery.Error()})
		case errors.Is(err, database.ErrSessionInvalid):
			respond(w, 401, map[string]string{"error": "Sesión no válida"})
		case errors.Is(err, database.ErrAccountMissing):
			respond(w, 404, map[string]string{"error": "Cuenta no disponible"})
		case errors.Is(err, binance.ErrPermissions):
			respond(w, 422, map[string]string{"error": binance.ErrPermissions.Error()})
		case errors.Is(err, binance.ErrRateLimited):
			w.Header().Set("Retry-After", "60")
			respond(w, 429, map[string]string{"error": binance.ErrRateLimited.Error()})
		case err != nil:
			respond(w, 503, map[string]string{"error": "No se pudo recuperar fills de forma segura. No se enviaron órdenes."})
		default:
			respond(w, 200, data)
		}
	})
}
