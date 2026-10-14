package database

import (
	"context"
	"database/sql"
	"errors"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
)

func privateHost(host string) bool {
	return host == "localhost" || host == "127.0.0.1" || host == "::1" || regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]*[a-z0-9])?\.railway\.internal$`).MatchString(host)
}

func connect(uri string) (*sql.DB, error) {
	u, err := url.Parse(uri)
	if err != nil || (u.Scheme != "postgres" && u.Scheme != "postgresql") || u.Hostname() == "" || u.User == nil || u.User.Username() == "" || u.Fragment != "" || strings.Contains(u.Host, ",") {
		return nil, errors.New("configuración PostgreSQL no válida")
	}
	// Prevent query parameters from replacing the validated destination/user.
	query := u.Query()
	for _, key := range []string{"host", "hostaddr", "port", "user", "password", "dbname", "service", "servicefile", "sslpassword"} {
		if query.Has(key) {
			return nil, errors.New("configuración PostgreSQL no válida")
		}
	}
	if !privateHost(u.Hostname()) && query.Get("sslmode") != "verify-full" {
		return nil, errors.New("PostgreSQL externo requiere TLS con verificación de identidad")
	}
	config, err := pgx.ParseConfig(uri)
	if err != nil {
		return nil, errors.New("configuración PostgreSQL no válida")
	}
	config.ConnectTimeout = 5 * time.Second
	config.RuntimeParams["search_path"] = "public"
	config.RuntimeParams["statement_timeout"] = "5000"
	db := stdlib.OpenDB(*config)
	db.SetMaxOpenConns(5)
	db.SetMaxIdleConns(2)
	db.SetConnMaxLifetime(30 * time.Minute)
	return db, nil
}

type Store struct {
	db         *sql.DB
	migrations []Migration
}

func Open(ctx context.Context, uri string, migrations []Migration) (*Store, error) {
	if len(migrations) == 0 {
		return nil, errors.New("no hay migraciones para verificar")
	}
	db, err := connect(uri)
	if err != nil {
		return nil, err
	}
	store := &Store{db: db, migrations: append([]Migration(nil), migrations...)}
	if err := store.Ready(ctx); err != nil {
		db.Close()
		return nil, err
	}
	return store, nil
}

func (s *Store) Close() error { return s.db.Close() }

func (s *Store) Ready(ctx context.Context) error {
	var elevated bool
	// An inherited owning role can also bypass owner boundaries. Refuse it,
	// even though our application tables additionally FORCE RLS.
	err := s.db.QueryRowContext(ctx, `SELECT r.rolsuper OR r.rolbypassrls OR r.rolcreaterole OR r.rolcreatedb
	OR has_schema_privilege(current_user, 'public', 'CREATE') OR EXISTS (
		SELECT 1 FROM pg_roles admin_role WHERE (admin_role.rolsuper OR admin_role.rolbypassrls OR admin_role.rolcreaterole OR admin_role.rolcreatedb)
		AND pg_has_role(current_user, admin_role.oid, 'MEMBER')
	) OR EXISTS (
		SELECT 1 FROM pg_class c JOIN pg_namespace n ON n.oid = c.relnamespace
		WHERE n.nspname = 'public' AND c.relkind IN ('r', 'p') AND pg_has_role(current_user, c.relowner, 'MEMBER')
	) FROM pg_roles r WHERE r.rolname = current_user`).Scan(&elevated)
	if err != nil {
		return errors.New("PostgreSQL no disponible")
	}
	if elevated {
		return errors.New("el motor requiere un rol PostgreSQL sin administración, propiedad de tablas ni BYPASSRLS")
	}
	rows, err := s.db.QueryContext(ctx, `SELECT name, checksum FROM public.schema_migrations ORDER BY name`)
	if err != nil {
		return errors.New("no se pudo verificar el esquema PostgreSQL")
	}
	_, err = verifyHistory(rows, s.migrations, true)
	return err
}

var ownerUUID = regexp.MustCompile(`^[a-fA-F0-9]{8}-[a-fA-F0-9]{4}-[a-fA-F0-9]{4}-[a-fA-F0-9]{4}-[a-fA-F0-9]{12}$`)

// WithOwner must only receive an identity verified on the server, never a
// client-supplied user ID. SET LOCAL cannot leak identity back into the pool.
func (s *Store) WithOwner(ctx context.Context, ownerID string, fn func(*sql.Tx) error) error {
	if !ownerUUID.MatchString(ownerID) || fn == nil {
		return errors.New("identidad de transacción no válida")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `SELECT set_config('app.user_id', $1, true)`, strings.ToLower(ownerID)); err != nil {
		return err
	}
	if err := fn(tx); err != nil {
		return err
	}
	return tx.Commit()
}
