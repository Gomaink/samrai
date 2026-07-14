package catalog

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"samrai/internal/database"
)

func TestReconcileManagedStorageRebasesCopiedDataDirectory(t *testing.T) {
	ctx := context.Background()
	oldRoot := filepath.Join(t.TempDir(), "old-release")
	newRoot := filepath.Join(t.TempDir(), "new-release")
	oldLibrary := filepath.Join(oldRoot, "data", "library")
	newData := filepath.Join(newRoot, "data")
	newLibrary := filepath.Join(newData, "library")
	if err := os.MkdirAll(oldLibrary, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(newLibrary, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(newData, "config"), 0o750); err != nil {
		t.Fatal(err)
	}

	db, err := database.Open(ctx, filepath.Join(newData, "config", "samrai.db"), 1)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := database.Migrate(ctx, db); err != nil {
		t.Fatal(err)
	}

	const hash = "f7662abd8fcc64eb4fc63709bf15d878aaad99cc3571a33587eeb8f5c6aa7b5b"
	oldBookPath := filepath.Join(oldLibrary, hash+".epub")
	newBookPath := filepath.Join(newLibrary, hash+".epub")
	if err := os.WriteFile(newBookPath, []byte("epub"), 0o640); err != nil {
		t.Fatal(err)
	}
	result, err := db.ExecContext(ctx, `INSERT INTO libraries(name, path, management_mode) VALUES ('Main', ?, 'managed')`, oldLibrary)
	if err != nil {
		t.Fatal(err)
	}
	libraryID, _ := result.LastInsertId()
	if _, err := db.ExecContext(ctx, `
		INSERT INTO books(library_id, title, file_path, original_filename, file_size, file_hash, status, format)
		VALUES (?, 'Book', ?, 'book.epub', 4, ?, 'ready', 'epub')
	`, libraryID, oldBookPath, hash); err != nil {
		t.Fatal(err)
	}

	report, err := ReconcileManagedStorage(ctx, db, newData)
	if err != nil {
		t.Fatal(err)
	}
	if report.RebasedBooks != 1 || report.RebasedLibraries != 1 || report.MissingBooks != 0 {
		t.Fatalf("unexpected report: %+v", report)
	}
	var storedBookPath, storedLibraryPath string
	if err := db.QueryRowContext(ctx, `SELECT file_path FROM books LIMIT 1`).Scan(&storedBookPath); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, `SELECT path FROM libraries LIMIT 1`).Scan(&storedLibraryPath); err != nil {
		t.Fatal(err)
	}
	if !sameFilesystemPath(storedBookPath, newBookPath) {
		t.Fatalf("book path = %q, want %q", storedBookPath, newBookPath)
	}
	if !sameFilesystemPath(storedLibraryPath, newLibrary) {
		t.Fatalf("library path = %q, want %q", storedLibraryPath, newLibrary)
	}
}

func TestReconcileManagedStorageReportsMissingOriginal(t *testing.T) {
	ctx := context.Background()
	dataDir := filepath.Join(t.TempDir(), "data")
	if err := os.MkdirAll(filepath.Join(dataDir, "config"), 0o750); err != nil {
		t.Fatal(err)
	}
	db, err := database.Open(ctx, filepath.Join(dataDir, "config", "samrai.db"), 1)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := database.Migrate(ctx, db); err != nil {
		t.Fatal(err)
	}
	result, err := db.ExecContext(ctx, `INSERT INTO libraries(name, path, management_mode) VALUES ('Main', ?, 'managed')`, filepath.Join("C:", "old", "library"))
	if err != nil {
		t.Fatal(err)
	}
	libraryID, _ := result.LastInsertId()
	if _, err := db.ExecContext(ctx, `
		INSERT INTO books(library_id, title, file_path, original_filename, file_size, file_hash, status, format)
		VALUES (?, 'Book', ?, 'book.epub', 4, 'missinghash', 'ready', 'epub')
	`, libraryID, filepath.Join("C:", "old", "library", "missinghash.epub")); err != nil {
		t.Fatal(err)
	}
	report, err := ReconcileManagedStorage(ctx, db, dataDir)
	if err != nil {
		t.Fatal(err)
	}
	if report.MissingBooks != 1 {
		t.Fatalf("missing books = %d, want 1", report.MissingBooks)
	}
}
