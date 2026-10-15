package database

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

type Migration struct {
	Name, Checksum, SQL string
}

var migrationName = regexp.MustCompile(`^[0-9]{4}_[a-z0-9_]+\.sql$`)
var legacyBegin = regexp.MustCompile(`(?s)^(?:\s|--[^\n]*\n)*BEGIN;`)

// Load treats migration files as immutable, including comments, with canonical LF.
func Load(directory string) ([]Migration, error) {
	entries, err := os.ReadDir(directory)
	if err != nil {
		return nil, errors.New("no se pudo leer el directorio de migraciones")
	}
	var names []string
	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}
		if !migrationName.MatchString(entry.Name()) || entry.Type()&os.ModeSymlink != 0 || entry.IsDir() {
			return nil, errors.New("archivo de migración no válido")
		}
		names = append(names, entry.Name())
	}
	sort.Strings(names)
	if len(names) == 0 {
		return nil, errors.New("no hay migraciones")
	}
	result := make([]Migration, 0, len(names))
	for index, name := range names {
		if name[:4] != fmt.Sprintf("%04d", index+1) {
			return nil, errors.New("las migraciones deben ser consecutivas desde 0001")
		}
		content, err := os.ReadFile(filepath.Join(directory, name))
		if err != nil {
			return nil, errors.New("no se pudo leer una migración")
		}
		canonical := strings.ReplaceAll(string(content), "\r\n", "\n")
		body := strings.TrimSpace(canonical)
		// The original 0001 script predates the runner and has an outer transaction.
		// Remove only that wrapper: the runner owns atomicity and migration history.
		if legacyBegin.MatchString(body) {
			if !strings.HasSuffix(body, "COMMIT;") {
				return nil, errors.New("transacción de migración incompleta")
			}
			body = strings.TrimSpace(strings.TrimSuffix(legacyBegin.ReplaceAllString(body, ""), "COMMIT;"))
		}
		if body == "" {
			return nil, errors.New("migración vacía")
		}
		result = append(result, Migration{Name: name, Checksum: fmt.Sprintf("%x", sha256.Sum256([]byte(canonical))), SQL: body})
	}
	return result, nil
}

func verifyHistory(rows *sql.Rows, migrations []Migration, requireAll bool) (int, error) {
	defer rows.Close()
	index := 0
	for rows.Next() {
		var name, checksum string
		if err := rows.Scan(&name, &checksum); err != nil {
			return 0, errors.New("historial de migraciones no válido")
		}
		if index >= len(migrations) || name != migrations[index].Name || checksum != migrations[index].Checksum {
			return 0, errors.New("el esquema no coincide con las migraciones; no se aplicaron cambios")
		}
		index++
	}
	if rows.Err() != nil || (requireAll && index != len(migrations)) {
		return 0, errors.New("el esquema requiere migraciones")
	}
	return index, nil
}

// Migrate is administrative and explicit. The HTTP engine never invokes it.
// The transaction-scoped lock serializes migrators; failures roll back history
// and all pending DDL together. There is deliberately no automatic down path.
func Migrate(ctx context.Context, uri string, migrations []Migration) error {
	if len(migrations) == 0 {
		return errors.New("no hay migraciones")
	}
	db, err := connect(uri)
	if err != nil {
		return err
	}
	defer db.Close()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return errors.New("no se pudo conectar para migrar")
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(734620918); SET LOCAL statement_timeout = '60s'`); err != nil {
		return errors.New("no se pudo bloquear la migración")
	}
	if _, err := tx.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS public.schema_migrations (
		name text PRIMARY KEY, checksum text NOT NULL, applied_at timestamptz NOT NULL DEFAULT now()
	)`); err != nil {
		return errors.New("no se pudo preparar el historial de migraciones")
	}
	rows, err := tx.QueryContext(ctx, `SELECT name, checksum FROM public.schema_migrations ORDER BY name`)
	if err != nil {
		return errors.New("no se pudo leer el historial de migraciones")
	}
	applied, err := verifyHistory(rows, migrations, false)
	if err != nil {
		return err
	}
	for _, migration := range migrations[applied:] {
		if _, err := tx.ExecContext(ctx, migration.SQL); err != nil {
			return fmt.Errorf("falló la migración %s; transacción revertida", migration.Name)
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO public.schema_migrations(name, checksum) VALUES ($1, $2)`, migration.Name, migration.Checksum); err != nil {
			return errors.New("no se pudo registrar la migración")
		}
	}
	if err := tx.Commit(); err != nil {
		return errors.New("no se pudo confirmar la migración; verifica el historial antes de reintentar")
	}
	return nil
}
