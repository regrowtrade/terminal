package database

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

// Run only against an explicitly selected disposable test server. A unique
// database/role is created and removed; this never touches Railway or exchanges.
func TestPostgresIsolationAndMigrations(t *testing.T) {
	uri := os.Getenv("TEST_DATABASE_URL")
	if uri == "" {
		t.Skip("TEST_DATABASE_URL not set; PostgreSQL integration runs in CI")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	admin, err := connect(uri)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { admin.Close() })
	suffix := make([]byte, 12)
	if _, err := rand.Read(suffix); err != nil {
		t.Fatal("could not generate test namespace")
	}
	dbName := "terminal_test_" + hex.EncodeToString(suffix)
	role := "terminal_role_" + hex.EncodeToString(suffix)
	password := make([]byte, 48)
	if _, err := rand.Read(password); err != nil {
		t.Fatal("could not generate test credential")
	}
	if _, err := admin.ExecContext(ctx, `CREATE DATABASE `+dbName); err != nil {
		t.Fatal("could not create isolated test database")
	}
	t.Cleanup(func() {
		if _, err := admin.ExecContext(context.Background(), `DROP DATABASE `+dbName+` WITH (FORCE)`); err != nil {
			t.Error("could not clean isolated test database")
		}
		if _, err := admin.ExecContext(context.Background(), `DROP ROLE IF EXISTS `+role); err != nil {
			t.Error("could not clean isolated test role")
		}
	})
	u, err := url.Parse(uri)
	if err != nil {
		t.Fatal("invalid test URL")
	}
	u.Path = "/" + dbName
	testURI := u.String()
	migrations, err := Load("../../../../database/migrations")
	if err != nil {
		t.Fatal(err)
	}
	var workers sync.WaitGroup
	migrationErrors := make(chan error, 2)
	for range 2 {
		workers.Add(1)
		go func() {
			defer workers.Done()
			migrationErrors <- Migrate(ctx, testURI, migrations)
		}()
	}
	workers.Wait()
	close(migrationErrors)
	for err := range migrationErrors {
		if err != nil {
			t.Fatal(err)
		}
	}
	setup, err := connect(testURI)
	if err != nil {
		t.Fatal(err)
	}
	defer setup.Close()
	if _, err := Open(ctx, testURI, migrations); err == nil {
		t.Fatal("administrative runtime role accepted")
	}
	if _, err := setup.ExecContext(ctx, `CREATE ROLE `+role+` LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOINHERIT NOBYPASSRLS PASSWORD '`+hex.EncodeToString(password)+`'`); err != nil {
		t.Fatal("could not create restricted test role")
	}
	grants, err := os.ReadFile("../../../../database/runtime-grants.sql")
	if err != nil {
		t.Fatal("could not load reviewed runtime grants")
	}
	if _, err := setup.ExecContext(ctx, strings.ReplaceAll(string(grants), "terminal_runtime", role)); err != nil {
		t.Fatal("could not provision restricted test grants")
	}
	const ownerA = "00000000-0000-4000-8000-000000000001"
	const ownerB = "00000000-0000-4000-8000-000000000002"
	if _, err := setup.ExecContext(ctx, `INSERT INTO terminal_users(id, auth_subject) VALUES ($1, 'test-owner-a'), ($2, 'test-owner-b')`, ownerA, ownerB); err != nil {
		t.Fatal("could not provision synthetic test identities")
	}
	u.User = url.UserPassword(role, hex.EncodeToString(password))
	store, err := Open(ctx, u.String(), migrations)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	testSessionLifecycle(t, ctx, store, setup, u.String())
	if err := store.WithOwner(ctx, ownerA, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `INSERT INTO exchange_accounts(id, owner_id, exchange, environment, name) VALUES ('00000000-0000-4000-8000-000000000003', $1, 'binance', 'testnet', 'Synthetic')`, ownerA)
		return err
	}); err != nil {
		t.Fatal("could not insert owned account")
	}
	for owner, expected := range map[string]int{ownerA: 1, ownerB: 0} {
		if err := store.WithOwner(ctx, owner, func(tx *sql.Tx) error {
			var count int
			if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM exchange_accounts`).Scan(&count); err != nil {
				return err
			}
			if count != expected {
				return errors.New("RLS disclosed another owner's account")
			}
			return nil
		}); err != nil {
			t.Fatal("ownership isolation failed")
		}
	}
	if err := store.WithOwner(ctx, ownerB, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `INSERT INTO exchange_accounts(id, owner_id, exchange, environment, name) VALUES ('00000000-0000-4000-8000-000000000004', $1, 'binance', 'testnet', 'Forbidden')`, ownerA)
		return err
	}); err == nil {
		t.Fatal("cross-owner write accepted")
	}
	var count int
	if err := store.db.QueryRowContext(ctx, `SELECT count(*) FROM exchange_accounts`).Scan(&count); err != nil || count != 0 {
		t.Fatal("pooled transaction leaked its identity")
	}
	if err := store.WithOwner(ctx, ownerA, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `UPDATE audit_events SET action = 'tampered'`)
		return err
	}); err == nil {
		t.Fatal("runtime can alter audit history")
	}
	if err := store.WithOwner(ctx, ownerA, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `INSERT INTO exchange_accounts(id, owner_id, exchange, environment, name) VALUES ('00000000-0000-4000-8000-000000000005', $1, 'binance', 'testnet', 'Rollback')`, ownerA)
		if err != nil {
			return err
		}
		return errors.New("synthetic callback failure")
	}); err == nil {
		t.Fatal("callback failure was ignored")
	}
	if err := store.WithOwner(ctx, ownerA, func(tx *sql.Tx) error {
		return tx.QueryRowContext(ctx, `SELECT count(*) FROM exchange_accounts`).Scan(&count)
	}); err != nil || count != 1 {
		t.Fatal("failed callback was not rolled back")
	}
	broken := append(append([]Migration(nil), migrations...), Migration{
		Name: "0003_broken.sql", Checksum: "synthetic-invalid-migration",
		SQL: "CREATE TABLE atomic_check(id int); SELECT 1 / 0;",
	})
	if err := Migrate(ctx, testURI, broken); err == nil {
		t.Fatal("invalid migration succeeded")
	}
	var table sql.NullString
	if err := setup.QueryRowContext(ctx, `SELECT to_regclass('public.atomic_check')::text`).Scan(&table); err != nil || table.Valid {
		t.Fatal("failed migration was not atomic")
	}
	if err := store.Ready(ctx); err != nil {
		t.Fatal("failed migration changed history")
	}
	if _, err := setup.ExecContext(ctx, `UPDATE schema_migrations SET checksum = 'changed'`); err != nil {
		t.Fatal("could not simulate schema drift")
	}
	if err := store.Ready(ctx); err == nil {
		t.Fatal("runtime accepted a modified migration")
	}
	if err := Migrate(ctx, testURI, migrations); err == nil {
		t.Fatal("migrator accepted a modified migration")
	}
}
