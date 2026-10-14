package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"regrow/engine/internal/httpapi"
)

func main() {
	token := os.Getenv("ENGINE_API_TOKEN")
	if len(token) < 32 || strings.HasPrefix(token, "replace-with-") {
		slog.Error("ENGINE_API_TOKEN debe contener al menos 32 caracteres")
		os.Exit(1)
	}
	address := os.Getenv("ENGINE_ADDRESS")
	if address == "" {
		address = "127.0.0.1:8080"
	}
	server := httpapi.NewServer(address, token)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	failure := make(chan error, 1)
	go func() {
		slog.Info("Motor iniciado", "address", address, "mode", "simulation")
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			failure <- err
		}
	}()
	select {
	case <-ctx.Done():
	case <-failure:
		slog.Error("No se pudo iniciar el motor; comprueba dirección y puerto")
		os.Exit(1)
	}
	shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := server.Shutdown(shutdown); err != nil {
		slog.Error("No se pudo cerrar el servidor limpiamente")
	}
}
