package backup

import (
	"archive/zip"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"samrai/internal/database"
)

func TestCreateAndRestore(t *testing.T) {
	ctx := context.Background()
	dataDir := filepath.Join(t.TempDir(), "source")
	if err := os.MkdirAll(filepath.Join(dataDir, "library", "aa"), 0o750); err != nil {
		t.Fatal(err)
	}
	bookBytes := []byte("fake-cbz-for-backup-test")
	if err := os.WriteFile(filepath.Join(dataDir, "library", "aa", "book.cbz"), bookBytes, 0o600); err != nil {
		t.Fatal(err)
	}

	if err := os.MkdirAll(filepath.Join(dataDir, "config"), 0o750); err != nil {
		t.Fatal(err)
	}
	db, err := database.Open(ctx, filepath.Join(dataDir, "config", "samrai.db"), 1)
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer db.Close()
	if err := database.Migrate(ctx, db); err != nil {
		t.Fatalf("migrate database: %v", err)
	}

	archivePath, archiveName, err := NewService(db, dataDir, "0.1.0-test").Create(ctx)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if archiveName == "" {
		t.Fatal("Create() returned empty archive name")
	}

	zr, err := zip.OpenReader(archivePath)
	if err != nil {
		t.Fatalf("open created archive: %v", err)
	}
	var manifest Manifest
	foundDatabase, foundBook := false, false
	for _, f := range zr.File {
		switch f.Name {
		case "manifest.json":
			r, err := f.Open()
			if err != nil {
				t.Fatal(err)
			}
			if err := json.NewDecoder(r).Decode(&manifest); err != nil {
				r.Close()
				t.Fatal(err)
			}
			r.Close()
		case "config/samrai.db":
			foundDatabase = true
		case "library/aa/book.cbz":
			foundBook = true
		}
	}
	zr.Close()
	if manifest.FormatVersion != FormatVersion || manifest.AppVersion != "0.1.0-test" || !foundDatabase || !foundBook {
		t.Fatalf("unexpected backup contents: manifest=%+v database=%t book=%t", manifest, foundDatabase, foundBook)
	}

	restoredDir := filepath.Join(t.TempDir(), "restored")
	if err := Restore(archivePath, restoredDir, false); err != nil {
		t.Fatalf("Restore() error = %v", err)
	}
	got, err := os.ReadFile(filepath.Join(restoredDir, "library", "aa", "book.cbz"))
	if err != nil {
		t.Fatalf("read restored book: %v", err)
	}
	if string(got) != string(bookBytes) {
		t.Fatalf("restored book = %q, want %q", got, bookBytes)
	}
	if _, err := os.Stat(filepath.Join(restoredDir, "config", "samrai.db")); err != nil {
		t.Fatalf("restored database missing: %v", err)
	}
}

func TestSafePathRejectsTraversal(t *testing.T) {
	for _, value := range []string{"../config/samrai.db", "/etc/passwd", "C:/Windows/file", "library/../../escape", "..\\escape"} {
		if _, err := safePath(value); err == nil {
			t.Fatalf("safePath(%q) did not reject unsafe path", value)
		}
	}
}

func TestRestoreAcceptsLegacyDatabaseName(t *testing.T) {
	archivePath := filepath.Join(t.TempDir(), "legacy-backup.zip")
	file, err := os.Create(archivePath)
	if err != nil {
		t.Fatal(err)
	}
	writer := zip.NewWriter(file)
	manifestEntry, err := writer.Create("manifest.json")
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := json.Marshal(Manifest{FormatVersion: FormatVersion, AppVersion: "0.2.0-rc.1", IncludesBooks: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manifestEntry.Write(manifest); err != nil {
		t.Fatal(err)
	}
	databaseEntry, err := writer.Create("config/pageturner.db")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := databaseEntry.Write([]byte("legacy-database")); err != nil {
		t.Fatal(err)
	}
	bookEntry, err := writer.Create("library/book.cbz")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := bookEntry.Write([]byte("book")); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}

	target := filepath.Join(t.TempDir(), "restored")
	if err := Restore(archivePath, target, false); err != nil {
		t.Fatalf("Restore() error = %v", err)
	}
	contents, err := os.ReadFile(filepath.Join(target, "config", "samrai.db"))
	if err != nil {
		t.Fatalf("read migrated database: %v", err)
	}
	if string(contents) != "legacy-database" {
		t.Fatalf("migrated database = %q", contents)
	}
	if _, err := os.Stat(filepath.Join(target, "config", "pageturner.db")); !os.IsNotExist(err) {
		t.Fatalf("legacy database name should not remain, stat error = %v", err)
	}
}
