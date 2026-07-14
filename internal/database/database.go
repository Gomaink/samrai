package database

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"path/filepath"

	_ "modernc.org/sqlite"
)

func Open(ctx context.Context, path string, maxConnections int) (*sql.DB, error) {
	absolutePath, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("resolve database path: %w", err)
	}

	dsn := sqliteDSN(absolutePath)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite database: %w", err)
	}

	db.SetMaxOpenConns(maxConnections)
	db.SetMaxIdleConns(maxConnections)

	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("ping sqlite database: %w", err)
	}

	var journalMode string
	if err := db.QueryRowContext(ctx, "PRAGMA journal_mode=WAL").Scan(&journalMode); err != nil {
		db.Close()
		return nil, fmt.Errorf("enable sqlite WAL: %w", err)
	}
	if journalMode != "wal" {
		db.Close()
		return nil, fmt.Errorf("enable sqlite WAL: got journal mode %q", journalMode)
	}

	return db, nil
}

// sqliteDSN converts an absolute filesystem path into a SQLite file URI.
// Windows drive paths need a leading slash (file:///C:/...), otherwise the
// drive letter is parsed as the URI authority and SQLite reports
// "invalid uri authority: C:".
func sqliteDSN(absolutePath string) string {
	slashPath := filepath.ToSlash(absolutePath)
	dsnURL := &url.URL{Scheme: "file"}

	if isWindowsDrivePath(slashPath) {
		dsnURL.Path = "/" + slashPath
	} else {
		dsnURL.Path = slashPath
	}

	query := dsnURL.Query()
	query.Add("_pragma", "foreign_keys(1)")
	query.Add("_pragma", "busy_timeout(5000)")
	query.Add("_pragma", "synchronous(NORMAL)")
	query.Add("_time_format", "sqlite")
	dsnURL.RawQuery = query.Encode()

	return dsnURL.String()
}

func isWindowsDrivePath(path string) bool {
	if len(path) < 3 || path[1] != ':' || path[2] != '/' {
		return false
	}

	letter := path[0]
	return (letter >= 'A' && letter <= 'Z') || (letter >= 'a' && letter <= 'z')
}
