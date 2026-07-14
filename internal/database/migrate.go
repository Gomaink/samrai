package database

import (
	"context"
	"database/sql"
	"fmt"
	"io/fs"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	migrationfiles "samrai/internal/database/migrations"
)

func Migrate(ctx context.Context, db *sql.DB) error {
	if _, err := db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version INTEGER PRIMARY KEY,
			name TEXT NOT NULL,
			applied_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
		)
	`); err != nil {
		return fmt.Errorf("create schema_migrations: %w", err)
	}

	entries, err := fs.ReadDir(migrationfiles.FS, ".")
	if err != nil {
		return fmt.Errorf("read embedded migrations: %w", err)
	}

	var names []string
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".sql") {
			names = append(names, entry.Name())
		}
	}
	sort.Strings(names)

	for _, name := range names {
		version, err := migrationVersion(name)
		if err != nil {
			return err
		}

		applied, err := migrationApplied(ctx, db, version)
		if err != nil {
			return err
		}
		if applied {
			continue
		}

		contents, err := fs.ReadFile(migrationfiles.FS, name)
		if err != nil {
			return fmt.Errorf("read migration %q: %w", name, err)
		}

		if err := applyMigration(ctx, db, version, name, string(contents)); err != nil {
			return err
		}
	}

	return nil
}

func migrationVersion(name string) (int, error) {
	base := filepath.Base(name)
	separator := strings.IndexByte(base, '_')
	if separator < 1 {
		return 0, fmt.Errorf("invalid migration filename %q", name)
	}

	version, err := strconv.Atoi(base[:separator])
	if err != nil {
		return 0, fmt.Errorf("parse migration version from %q: %w", name, err)
	}
	return version, nil
}

func migrationApplied(ctx context.Context, db *sql.DB, version int) (bool, error) {
	var exists int
	err := db.QueryRowContext(
		ctx,
		"SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE version = ?)",
		version,
	).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("check migration %d: %w", version, err)
	}
	return exists == 1, nil
}

func applyMigration(ctx context.Context, db *sql.DB, version int, name, statement string) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin migration %q: %w", name, err)
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, statement); err != nil {
		return fmt.Errorf("execute migration %q: %w", name, err)
	}
	if _, err := tx.ExecContext(
		ctx,
		"INSERT INTO schema_migrations(version, name) VALUES (?, ?)",
		version,
		name,
	); err != nil {
		return fmt.Errorf("record migration %q: %w", name, err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit migration %q: %w", name, err)
	}
	return nil
}
