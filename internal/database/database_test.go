package database

import (
	"context"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"

	migrationfiles "samrai/internal/database/migrations"
)

func TestOpenAndMigrate(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "samrai.db")

	db, err := Open(ctx, path, 1)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer db.Close()

	if err := Migrate(ctx, db); err != nil {
		t.Fatalf("Migrate() error = %v", err)
	}
	if err := Migrate(ctx, db); err != nil {
		t.Fatalf("second Migrate() error = %v", err)
	}

	var count int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM schema_migrations").Scan(&count); err != nil {
		t.Fatalf("count schema migrations: %v", err)
	}
	expected, err := embeddedMigrationCount()
	if err != nil {
		t.Fatalf("count embedded migrations: %v", err)
	}
	if count != expected {
		t.Fatalf("migration count = %d, want %d", count, expected)
	}

	var tableName string
	if err := db.QueryRowContext(
		ctx,
		"SELECT name FROM sqlite_master WHERE type = 'table' AND name = 'books'",
	).Scan(&tableName); err != nil {
		t.Fatalf("find books table: %v", err)
	}
}

func TestSQLiteDSNWindowsDrivePath(t *testing.T) {
	dsn := sqliteDSN("C:/Users/Samuel/samrai/data/config/samrai.db")

	if want := "file:///C:/Users/Samuel/samrai/data/config/samrai.db"; len(dsn) < len(want) || dsn[:len(want)] != want {
		t.Fatalf("sqliteDSN() = %q, want prefix %q", dsn, want)
	}
}

func TestSQLiteDSNUnixPath(t *testing.T) {
	dsn := sqliteDSN("/var/lib/samrai/samrai.db")

	if want := "file:///var/lib/samrai/samrai.db"; len(dsn) < len(want) || dsn[:len(want)] != want {
		t.Fatalf("sqliteDSN() = %q, want prefix %q", dsn, want)
	}
}

func TestEnglishDefaultsMigrationOnlyChangesUntouchedLibraryName(t *testing.T) {
	ctx := context.Background()
	db, err := Open(ctx, filepath.Join(t.TempDir(), "english-defaults.db"), 1)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer db.Close()
	if err := Migrate(ctx, db); err != nil {
		t.Fatalf("Migrate() error = %v", err)
	}

	if _, err := db.ExecContext(ctx, `INSERT INTO libraries(name,path,management_mode) VALUES
		('Biblioteca principal','managed','managed'),
		('Biblioteca principal','external','external'),
		('My Library','custom','managed')`); err != nil {
		t.Fatalf("insert libraries: %v", err)
	}
	statement, err := fs.ReadFile(migrationfiles.FS, "015_english_defaults.sql")
	if err != nil {
		t.Fatalf("read migration: %v", err)
	}
	if _, err := db.ExecContext(ctx, string(statement)); err != nil {
		t.Fatalf("execute migration: %v", err)
	}

	wants := map[string]string{
		"managed":  "Main library",
		"external": "Biblioteca principal",
		"custom":   "My Library",
	}
	for path, want := range wants {
		var got string
		if err := db.QueryRowContext(ctx, `SELECT name FROM libraries WHERE path = ?`, path).Scan(&got); err != nil {
			t.Fatalf("read %s library: %v", path, err)
		}
		if got != want {
			t.Fatalf("library %s name = %q, want %q", path, got, want)
		}
	}
}

func embeddedMigrationCount() (int, error) {
	entries, err := fs.ReadDir(migrationfiles.FS, ".")
	if err != nil {
		return 0, err
	}

	count := 0
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".sql") {
			count++
		}
	}
	return count, nil
}
