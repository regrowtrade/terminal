package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"regrow/engine/internal/database"
	"regrow/engine/internal/planner"
)

var planIDPattern = regexp.MustCompile(`^[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$`)

type PlanStore interface {
	SavePlan(context.Context, string, string, planner.Request) (database.SavedPlan, error)
	ListPlans(context.Context, string, string, int) ([]database.StoredPlan, error)
	SavePlanIntents(context.Context, string, string, string) (int, error)
	ListPlanIntents(context.Context, string, string, string) ([]database.StoredIntent, error)
}

func plansError(err error) (int, string) {
	switch {
	case errors.Is(err, database.ErrSessionInvalid):
		return 401, "Sesión no válida"
	case errors.Is(err, database.ErrAccountMissing):
		return 404, "Cuenta no disponible"
	case errors.Is(err, database.ErrPlanMissing):
		return 404, "Plan no disponible"
	case errors.Is(err, database.ErrPlanInvalid):
		return 422, "Plan no válido"
	default:
		return 503, "No se pudo guardar o leer el plan. No se enviaron órdenes."
	}
}

func registerPlans(mux *http.ServeMux, token string, store PlanStore) {
	mux.HandleFunc("POST /v1/plans/save", func(w http.ResponseWriter, r *http.Request) {
		var input struct {
			TokenHash string `json:"tokenHash"`
			AccountID string `json:"accountId"`
			planner.Request
		}
		if !authorized(r, token) {
			respond(w, 401, map[string]string{"error": "No autorizado"})
			return
		}
		if r.Header.Get("Content-Type") != "application/json" {
			respond(w, 415, map[string]string{"error": "Se requiere application/json"})
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, 16384)
		defer r.Body.Close()
		decoder := json.NewDecoder(r.Body)
		decoder.DisallowUnknownFields()
		if decoder.Decode(&input) != nil || decoder.Decode(&struct{}{}) != io.EOF || !validPlanHead(input.TokenHash, input.AccountID) {
			respond(w, 400, map[string]string{"error": "Solicitud inválida"})
			return
		}
		if store == nil {
			respond(w, 503, map[string]string{"error": "PostgreSQL es obligatorio para planes"})
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 18*time.Second)
		defer cancel()
		saved, err := store.SavePlan(ctx, input.TokenHash, input.AccountID, input.Request)
		if err != nil {
			status, message := plansError(err)
			respond(w, status, map[string]string{"error": message})
			return
		}
		respond(w, 200, saved)
	})
	mux.HandleFunc("POST /v1/plans/list", func(w http.ResponseWriter, r *http.Request) {
		respondPlanRead(w, r, token, store, false, func(ctx context.Context, hash, account, _ string) (any, error) {
			return store.ListPlans(ctx, hash, account, 20)
		})
	})
	mux.HandleFunc("POST /v1/plans/intents/save", func(w http.ResponseWriter, r *http.Request) {
		respondPlanRead(w, r, token, store, true, func(ctx context.Context, hash, account, planID string) (any, error) {
			count, err := store.SavePlanIntents(ctx, hash, account, planID)
			if err != nil {
				return nil, err
			}
			return map[string]int{"count": count}, nil
		})
	})
	mux.HandleFunc("POST /v1/plans/intents/list", func(w http.ResponseWriter, r *http.Request) {
		respondPlanRead(w, r, token, store, true, func(ctx context.Context, hash, account, planID string) (any, error) {
			return store.ListPlanIntents(ctx, hash, account, planID)
		})
	})
}

func validPlanHead(hash, account string) bool {
	return len(hash) == 64 && strings.Trim(hash, "0123456789abcdef") == "" && planIDPattern.MatchString(account)
}

func respondPlanRead(w http.ResponseWriter, r *http.Request, token string, store PlanStore, needsPlan bool, fn func(context.Context, string, string, string) (any, error)) {
	if !authorized(r, token) {
		respond(w, 401, map[string]string{"error": "No autorizado"})
		return
	}
	if r.Header.Get("Content-Type") != "application/json" {
		respond(w, 415, map[string]string{"error": "Se requiere application/json"})
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 16384)
	defer r.Body.Close()
	var head struct {
		TokenHash string `json:"tokenHash"`
		AccountID string `json:"accountId"`
		PlanID    string `json:"planId,omitempty"`
	}
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if decoder.Decode(&head) != nil || decoder.Decode(&struct{}{}) != io.EOF || !validPlanHead(head.TokenHash, head.AccountID) || (needsPlan && !planIDPattern.MatchString(head.PlanID)) {
		respond(w, 400, map[string]string{"error": "Solicitud inválida"})
		return
	}
	if store == nil {
		respond(w, 503, map[string]string{"error": "PostgreSQL es obligatorio para planes"})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 18*time.Second)
	defer cancel()
	data, err := fn(ctx, head.TokenHash, head.AccountID, head.PlanID)
	if err != nil {
		status, message := plansError(err)
		respond(w, status, map[string]string{"error": message})
		return
	}
	respond(w, 200, data)
}
