package catalog

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// StorageRepairReport summarizes path repairs applied after a self-hosted
// installation is moved to another directory or restored from backup.
type StorageRepairReport struct {
	RebasedBooks     int
	MissingBooks     int
	RebasedLibraries int
}

// ReconcileManagedStorage makes managed-library paths portable across project
// folders. Older samrai builds stored absolute paths in SQLite, so copying
// the data directory to a new release could leave the database pointing at the
// previous installation directory even though the files were copied correctly.
func ReconcileManagedStorage(ctx context.Context, db *sql.DB, dataDir string) (StorageRepairReport, error) {
	if db == nil {
		return StorageRepairReport{}, errors.New("database is required")
	}
	libraryDir, err := filepath.Abs(filepath.Join(dataDir, "library"))
	if err != nil {
		return StorageRepairReport{}, fmt.Errorf("resolve managed library directory: %w", err)
	}
	if err := os.MkdirAll(libraryDir, 0o750); err != nil {
		return StorageRepairReport{}, fmt.Errorf("create managed library directory: %w", err)
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return StorageRepairReport{}, fmt.Errorf("begin storage reconciliation: %w", err)
	}
	defer tx.Rollback()

	report := StorageRepairReport{}

	// A normal samrai instance has one managed library. Rebase it before
	// the importer starts so future books are attached to the same library.
	var managedLibraryID int64
	err = tx.QueryRowContext(ctx, `
		SELECT id FROM libraries
		WHERE management_mode = 'managed'
		ORDER BY id
		LIMIT 1
	`).Scan(&managedLibraryID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return StorageRepairReport{}, fmt.Errorf("find managed library: %w", err)
	}
	if err == nil {
		var currentPath string
		if scanErr := tx.QueryRowContext(ctx, `SELECT path FROM libraries WHERE id = ?`, managedLibraryID).Scan(&currentPath); scanErr != nil {
			return StorageRepairReport{}, fmt.Errorf("read managed library path: %w", scanErr)
		}
		if !sameFilesystemPath(currentPath, libraryDir) {
			var conflictingID int64
			conflictErr := tx.QueryRowContext(ctx, `SELECT id FROM libraries WHERE path = ?`, libraryDir).Scan(&conflictingID)
			switch {
			case errors.Is(conflictErr, sql.ErrNoRows):
				if _, updateErr := tx.ExecContext(ctx, `
					UPDATE libraries
					SET path = ?, updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
					WHERE id = ?
				`, libraryDir, managedLibraryID); updateErr != nil {
					return StorageRepairReport{}, fmt.Errorf("rebase managed library: %w", updateErr)
				}
				report.RebasedLibraries++
			case conflictErr != nil:
				return StorageRepairReport{}, fmt.Errorf("check managed library conflict: %w", conflictErr)
			}
		}
	}

	rows, err := tx.QueryContext(ctx, `
		SELECT b.id, b.file_path, b.file_hash, b.format
		FROM books b
		JOIN libraries l ON l.id = b.library_id
		WHERE l.management_mode = 'managed'
	`)
	if err != nil {
		return StorageRepairReport{}, fmt.Errorf("list managed books: %w", err)
	}
	type managedBook struct {
		id                         int64
		filePath, fileHash, format string
	}
	books := make([]managedBook, 0)
	for rows.Next() {
		var id int64
		var filePath, fileHash, format string
		if err := rows.Scan(&id, &filePath, &fileHash, &format); err != nil {
			rows.Close()
			return StorageRepairReport{}, fmt.Errorf("scan managed book: %w", err)
		}
		books = append(books, managedBook{id: id, filePath: filePath, fileHash: fileHash, format: format})
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return StorageRepairReport{}, fmt.Errorf("iterate managed books: %w", err)
	}
	rows.Close()

	for _, book := range books {
		candidate := filepath.Join(libraryDir, book.fileHash+managedBookExtension(book.format, book.filePath))
		candidateExists := regularFileExists(candidate)
		storedExists := regularFileExists(book.filePath)

		if candidateExists {
			if !sameFilesystemPath(book.filePath, candidate) {
				if _, err := tx.ExecContext(ctx, `
					UPDATE books
					SET file_path = ?, updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
					WHERE id = ?
				`, candidate, book.id); err != nil {
					return StorageRepairReport{}, fmt.Errorf("rebase book %d: %w", book.id, err)
				}
				report.RebasedBooks++
			}
			continue
		}
		if !storedExists {
			report.MissingBooks++
		}
	}

	if err := tx.Commit(); err != nil {
		return StorageRepairReport{}, fmt.Errorf("commit storage reconciliation: %w", err)
	}
	return report, nil
}

func managedBookExtension(format, storedPath string) string {
	switch strings.ToLower(strings.TrimSpace(format)) {
	case "pdf":
		return ".pdf"
	case "epub":
		return ".epub"
	case "cbr":
		return ".cbr"
	case "cb7":
		return ".cb7"
	case "cbt":
		return ".cbt"
	case "cbz", "zip", "image_pages":
		return ".cbz"
	}
	if extension := strings.ToLower(filepath.Ext(storedPath)); extension != "" {
		return extension
	}
	return ".cbz"
}

func regularFileExists(path string) bool {
	if strings.TrimSpace(path) == "" {
		return false
	}
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular()
}

func sameFilesystemPath(left, right string) bool {
	leftClean := filepath.Clean(left)
	rightClean := filepath.Clean(right)
	if filepath.Separator == '\\' {
		return strings.EqualFold(leftClean, rightClean)
	}
	return leftClean == rightClean
}
