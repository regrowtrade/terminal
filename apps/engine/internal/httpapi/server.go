package httpapi

import (
	"crypto/subtle"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"regrow/engine/internal/planner"
)

func Handler(token string) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		respond(w, 200, map[string]string{"status": "ok", "mode": "simulation", "version": "0.1.0"})
	})
	mux.HandleFunc("POST /v1/plans/preview", func(w http.ResponseWriter, r *http.Request) {
		if len(token) < 32 || strings.HasPrefix(token, "replace-with-") || subtle.ConstantTimeCompare([]byte(r.Header.Get("Authorization")), []byte("Bearer "+token)) != 1 {
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
		var request planner.Request
		if err := decoder.Decode(&request); err != nil {
			respond(w, 400, map[string]string{"error": "Solicitud inválida"})
			return
		}
		if err := decoder.Decode(&struct{}{}); err != io.EOF {
			respond(w, 400, map[string]string{"error": "Se permite un único objeto JSON"})
			return
		}
		plan, err := planner.Preview(request)
		if err != nil {
			respond(w, 422, map[string]string{"error": err.Error()})
			return
		}
		respond(w, 200, plan)
	})
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'")
		mux.ServeHTTP(w, r)
	})
}

func respond(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}

func NewServer(address, token string) *http.Server {
	return &http.Server{Addr: address, Handler: Handler(token), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 10 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 8192}
}
