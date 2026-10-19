// Explicit administrative retirement of legacy read-only baselines. This is
// neither a migration-on-startup nor an exchange connection.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"time"

	"regrow/engine/internal/database"
)

func run(args []string, output io.Writer) error {
	flags := flag.NewFlagSet("snapshot-transition", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	account := flags.String("account", "", "UUID de la cuenta personal Binance production")
	apply := flags.Bool("apply", false, "archivar y retirar el baseline antiguo; por defecto solo inspecciona")
	expected := flags.String("expected-sha256", "", "huella obtenida en la inspección; obligatoria con --apply")
	if err := flags.Parse(args); err != nil || flags.NArg() != 0 {
		return errors.New("uso: snapshot-transition --account UUID [--apply --expected-sha256 HUELLA]")
	}
	if *account == "" || (*apply && *expected == "") || (!*apply && *expected != "") {
		return errors.New("indica la cuenta; --apply requiere la huella de una inspección previa")
	}
	uri := os.Getenv("MIGRATION_DATABASE_URL")
	if uri == "" {
		return errors.New("configura MIGRATION_DATABASE_URL solo en la consola administrativa")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	result, err := database.TransitionLegacySnapshot(ctx, uri, *account, *expected, *apply)
	if err != nil {
		return err
	}
	return json.NewEncoder(output).Encode(result)
}

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
