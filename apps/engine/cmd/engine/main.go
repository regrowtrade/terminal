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

	"regrow/engine/internal/binance"
	"regrow/engine/internal/connections"
	"regrow/engine/internal/database"
	"regrow/engine/internal/httpapi"
	"regrow/engine/internal/vault"
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
	if required := os.Getenv("DATABASE_REQUIRED"); required != "" && required != "true" && required != "false" {
		slog.Error("DATABASE_REQUIRED debe ser true o false")
		os.Exit(1)
	}
	var store *database.Store
	var monitor *connections.Monitor
	enabled := os.Getenv("BINANCE_READ_ENABLED")
	if enabled != "" && enabled != "false" && enabled != "true" {
		slog.Error("BINANCE_READ_ENABLED debe ser true o false")
		os.Exit(1)
	}
	uri := os.Getenv("DATABASE_URL")
	if uri == "" && os.Getenv("DATABASE_REQUIRED") == "true" {
		slog.Error("PostgreSQL es obligatorio en este despliegue; configura DATABASE_URL con un rol limitado")
		os.Exit(1)
	}
	if uri != "" {
		directory := os.Getenv("MIGRATIONS_DIR")
		if directory == "" {
			directory = "../../database/migrations"
		}
		migrations, err := database.Load(directory)
		if err != nil {
			slog.Error(err.Error())
			os.Exit(1)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		store, err = database.Open(ctx, uri, migrations)
		cancel()
		if err != nil {
			slog.Error(err.Error())
			os.Exit(1)
		}
		defer store.Close()
	}
	server := httpapi.NewServer(address, token)
	if store != nil {
		service := &connections.Service{Store: store}
		if enabled == "true" {
			if os.Getenv("BINANCE_EGRESS_CONFIRMED") != "true" {
				slog.Error("Confirma región permitida e IP de salida fija restringida antes de habilitar lectura")
				os.Exit(1)
			}
			keys, err := vault.New(os.Getenv("CREDENTIAL_ACTIVE_KEY_ID"), os.Getenv("CREDENTIAL_KEYS_JSON"))
			if err != nil {
				slog.Error("Configura el keyring de credenciales solo en el motor")
				os.Exit(1)
			}
			client := binance.New()
			service.Vault, service.Reader = keys, client
			// The read-only user data stream journals private events for later
			// reconciliation. It never submits, cancels or amends orders.
			monitor = &connections.Monitor{Store: store, Vault: keys, Stream: binance.NewStream(client), Log: slog.Default()}
		}
		server.Handler = httpapi.HandlerWithConnections(token, store.Ready, store, service)
	} else if enabled == "true" {
		slog.Error("La lectura Binance requiere PostgreSQL")
		os.Exit(1)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if monitor != nil {
		go func() {
			if err := monitor.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
				slog.Error("Monitor de eventos privados detenido", "error", err)
			}
		}()
	}
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
