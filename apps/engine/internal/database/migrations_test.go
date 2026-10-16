package database

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeMigration(t *testing.T, directory, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(directory, name), []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
}

func TestLoadLegacyFoundation(t *testing.T) {
	migrations, err := Load("../../../../database/migrations")
	if err != nil {
		t.Fatal(err)
	}
	if len(migrations) != 5 || migrations[0].Name != "0001_foundation.sql" || len(migrations[0].Checksum) != 64 {
		t.Fatal("unexpected migration manifest")
	}
	if strings.HasPrefix(migrations[0].SQL, "BEGIN;") || strings.HasSuffix(migrations[0].SQL, "COMMIT;") {
		t.Fatal("runner must own the transaction")
	}
	if !strings.Contains(migrations[0].SQL, "FORCE ROW LEVEL SECURITY") {
		t.Fatal("missing RLS")
	}
}

func TestLoadCanonicalChecksum(t *testing.T) {
	directory := t.TempDir()
	writeMigration(t, directory, "0001_test.sql", "-- example\nBEGIN;\nCREATE TABLE example(id int);\nCOMMIT;\n")
	first, err := Load(directory)
	if err != nil {
		t.Fatal(err)
	}
	writeMigration(t, directory, "0001_test.sql", "-- example\r\nBEGIN;\r\nCREATE TABLE example(id int);\r\nCOMMIT;\r\n")
	second, err := Load(directory)
	if err != nil || first[0].Checksum != second[0].Checksum || first[0].SQL != second[0].SQL {
		t.Fatal("Windows line endings changed the schema manifest")
	}
	writeMigration(t, directory, "0001_test.sql", "CREATE TABLE changed(id int);\n")
	third, err := Load(directory)
	if err != nil || third[0].Checksum == first[0].Checksum {
		t.Fatal("schema changes must change the checksum")
	}
}

func TestLoadRejectsInvalidManifests(t *testing.T) {
	for _, tc := range []struct{ name, file, body string }{
		{"bad name", "migration.sql", "SELECT 1;"},
		{"gap", "0002_test.sql", "SELECT 1;"},
		{"empty", "0001_test.sql", " "},
		{"incomplete wrapper", "0001_test.sql", "BEGIN; SELECT 1;"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			directory := t.TempDir()
			writeMigration(t, directory, tc.file, tc.body)
			if _, err := Load(directory); err == nil {
				t.Fatal("expected rejection")
			}
		})
	}
	directory := t.TempDir()
	if _, err := Load(directory); err == nil {
		t.Fatal("empty manifest accepted")
	}
	writeMigration(t, directory, "0001_first.sql", "SELECT 1;")
	writeMigration(t, directory, "0001_duplicate.sql", "SELECT 2;")
	if _, err := Load(directory); err == nil {
		t.Fatal("duplicate version accepted")
	}
}

func TestConnectionConfiguration(t *testing.T) {
	for _, uri := range []string{
		"postgresql://runtime@127.0.0.1/test?sslmode=disable",
		"postgresql://runtime@postgres.railway.internal/test?sslmode=disable",
		"postgresql://runtime@database.example/test?sslmode=verify-full",
	} {
		db, err := connect(uri)
		if err != nil {
			t.Fatal(err)
		}
		db.Close() // No connection is made by this unit test.
	}
	for _, uri := range []string{
		"", "file:///tmp/database", "postgresql:///test", "postgresql://127.0.0.1/test",
		"postgresql://runtime@database.example/test?sslmode=disable",
		"postgresql://runtime@postgres.railway.internal.attacker.example/test?sslmode=disable",
		"postgresql://runtime@127.0.0.1/test?host=attacker.example&sslmode=disable",
		"postgresql://runtime@127.0.0.1/test?user=postgres",
	} {
		if db, err := connect(uri); err == nil {
			db.Close()
			t.Fatal("unsafe configuration accepted")
		}
	}
}

func TestOwnerIdentityRejectedBeforeTransaction(t *testing.T) {
	store := &Store{}
	for _, owner := range []string{"", "user", "00000000-0000-0000-0000-000000000000'; RESET ROLE; --"} {
		if err := store.WithOwner(context.Background(), owner, nil); err == nil {
			t.Fatal("unverified identity shape accepted")
		}
	}
}
