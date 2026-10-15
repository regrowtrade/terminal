package main

import (
	"context"
	"log/slog"
	"os"
	"time"

	"regrow/engine/internal/database"
)

func main() {
	uri := os.Getenv("MIGRATION_DATABASE_URL")
	if uri == "" {
		slog.Error("Configura MIGRATION_DATABASE_URL en el proceso administrativo, no en el motor")
		os.Exit(1)
	}
	directory := os.Getenv("MIGRATIONS_DIR")
	if directory == "" {
		directory = "../../database/migrations"
	}
	migrations, err := database.Load(directory)
	if err != nil {
		slog.Error(err.Error())
		os.Exit(1)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	if err := database.Migrate(ctx, uri, migrations); err != nil {
		slog.Error(err.Error())
		os.Exit(1)
	}
	slog.Info("Migraciones verificadas y aplicadas", "count", len(migrations))
}
