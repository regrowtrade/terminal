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
	"regrow/engine/internal/reconcile"
	"strings"
	"time"
)

type ConnectionService interface {
	State(context.Context, string) (connections.State, error)
	Snapshot(context.Context, string, string) (binance.Snapshot, error)
	Latest(context.Context, string, string) (database.StoredSnapshot, error)
	Comparison(context.Context, string, string) (database.StoredComparison, error)
	Events(context.Context, string, string, int64) ([]database.StoredEvent, bool, error)
	Reconcile(context.Context, string, string) (reconcile.Report, error)
	LookupOrder(context.Context, string, string, binance.OrderQuery) (binance.OrderLookup, error)
	LookupAlgoOrder(context.Context, string, string, binance.AlgoQuery) (binance.AlgoLookup, error)
	ObserveIntent(context.Context, string, string, string) (database.StoredIntentObservation, error)
	IntentObservations(context.Context, string, string, string) ([]database.StoredIntentObservation, error)
	RecoverFills(context.Context, string, string, binance.TradeQuery) (binance.FillRecovery, error)
}

func registerConnections(mux *http.ServeMux, token string, service ConnectionService) {
	for _, operation := range []string{"list", "snapshot", "latest", "comparison", "events", "reconcile", "order"} {
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
				TokenHash     string          `json:"tokenHash"`
				AccountID     string          `json:"accountId,omitempty"`
				After         int64           `json:"after,omitempty"`
				Symbol        json.RawMessage `json:"symbol,omitempty"`
				OrderID       json.RawMessage `json:"orderId,omitempty"`
				ClientOrderID json.RawMessage `json:"clientOrderId,omitempty"`
			}
			d := json.NewDecoder(r.Body)
			d.DisallowUnknownFields()
			decodeErr := d.Decode(&input)
			var query binance.OrderQuery
			validOrderFields := true
			for _, field := range []struct {
				raw   json.RawMessage
				value *string
			}{
				{input.Symbol, &query.Symbol}, {input.OrderID, &query.OrderID}, {input.ClientOrderID, &query.ClientOrderID},
			} {
				if len(field.raw) > 0 && (operation != "order" || json.Unmarshal(field.raw, field.value) != nil || *field.value == "") {
					validOrderFields = false
				}
			}
			if decodeErr != nil || d.Decode(&struct{}{}) != io.EOF ||
				len(input.TokenHash) != 64 || strings.Trim(input.TokenHash, "0123456789abcdef") != "" ||
				(operation == "list" && (input.AccountID != "" || input.After != 0)) ||
				(operation != "list" && input.AccountID == "") ||
				(operation != "events" && input.After != 0) || (operation == "events" && input.After < 0) ||
				!validOrderFields || (operation == "order" && !query.Valid()) {
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
			} else if operation == "latest" {
				data, err = service.Latest(ctx, input.TokenHash, input.AccountID)
			} else if operation == "comparison" {
				data, err = service.Comparison(ctx, input.TokenHash, input.AccountID)
			} else if operation == "events" {
				var events []database.StoredEvent
				var truncated bool
				events, truncated, err = service.Events(ctx, input.TokenHash, input.AccountID, input.After)
				if err == nil {
					data = map[string]any{"events": events, "truncated": truncated}
				}
			} else if operation == "reconcile" {
				data, err = service.Reconcile(ctx, input.TokenHash, input.AccountID)
			} else if operation == "order" {
				data, err = service.LookupOrder(ctx, input.TokenHash, input.AccountID, query)
			} else {
				data, err = service.Snapshot(ctx, input.TokenHash, input.AccountID)
			}
			switch {
			case errors.Is(err, binance.ErrOrderQuery):
				respond(w, 400, map[string]string{"error": binance.ErrOrderQuery.Error()})
			case errors.Is(err, database.ErrSessionInvalid):
				respond(w, 401, map[string]string{"error": "Sesión no válida"})
			case errors.Is(err, database.ErrAccountMissing):
				respond(w, 404, map[string]string{"error": "Cuenta no disponible"})
			case errors.Is(err, database.ErrSnapshotMissing):
				respond(w, 404, map[string]string{"error": database.ErrSnapshotMissing.Error()})
			case errors.Is(err, database.ErrComparisonMissing):
				respond(w, 404, map[string]string{"error": database.ErrComparisonMissing.Error()})
			case errors.Is(err, database.ErrEventInvalid):
				respond(w, 503, map[string]string{"error": "evento guardado no válido; no se devuelve un parcial"})
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
