package importer

import (
	"archive/zip"
	"context"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

func TestInspectArchiveOrdersPagesNaturally(t *testing.T) {
	path := filepath.Join(t.TempDir(), "book.cbz")
	writeArchive(t, path, []string{"pages/10.png", "pages/2.png", "pages/1.png"})

	book, err := inspectArchive(context.Background(), "My Series.cbz", path, testLimits())
	if err != nil {
		t.Fatalf("inspectArchive() error = %v", err)
	}
	if book.Title != "My Series" {
		t.Fatalf("title = %q", book.Title)
	}
	if len(book.Pages) != 3 {
		t.Fatalf("page count = %d", len(book.Pages))
	}
	want := []string{"pages/1.png", "pages/2.png", "pages/10.png"}
	for index, page := range book.Pages {
		if page.ArchivePath != want[index] {
			t.Fatalf("page %d path = %q, want %q", index, page.ArchivePath, want[index])
		}
		if page.Width != 12 || page.Height != 18 || page.MediaType != "image/png" {
			t.Fatalf("unexpected page metadata: %+v", page)
		}
	}
}

func TestInspectArchiveRejectsTraversal(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bad.cbz")
	writeArchive(t, path, []string{"../cover.png"})
	if _, err := inspectArchive(context.Background(), "bad.cbz", path, testLimits()); err == nil {
		t.Fatal("inspectArchive() accepted path traversal")
	}
}

func TestValidateArchivePathRejectsWindowsDrive(t *testing.T) {
	if err := validateArchivePath("C:/Windows/page.png"); err == nil {
		t.Fatal("validateArchivePath() accepted drive path")
	}
}

func writeArchive(t *testing.T, destination string, names []string) {
	t.Helper()
	file, err := os.Create(destination)
	if err != nil {
		t.Fatalf("create archive: %v", err)
	}
	writer := zip.NewWriter(file)
	for _, name := range names {
		entry, err := writer.Create(name)
		if err != nil {
			t.Fatalf("create entry: %v", err)
		}
		canvas := image.NewRGBA(image.Rect(0, 0, 12, 18))
		if err := png.Encode(entry, canvas); err != nil {
			t.Fatalf("encode png: %v", err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close zip: %v", err)
	}
	if err := file.Close(); err != nil {
		t.Fatalf("close archive: %v", err)
	}
}

func testLimits() Limits {
	return Limits{
		MaxUploadBytes:    10 << 20,
		MaxArchiveBytes:   50 << 20,
		MaxPageBytes:      5 << 20,
		MaxArchiveEntries: 100,
		MaxPages:          50,
	}
}

func TestInspectArchiveReadsComicInfo(t *testing.T) {
	archivePath := filepath.Join(t.TempDir(), "metadata.cbz")
	file, err := os.Create(archivePath)
	if err != nil {
		t.Fatal(err)
	}
	writer := zip.NewWriter(file)
	metadata, err := writer.Create("ComicInfo.xml")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = metadata.Write([]byte(`<?xml version="1.0"?><ComicInfo><Title>Special Edition</Title><Series>Absolute Batman</Series><Summary>A synopsis.</Summary><Writer>Scott Snyder</Writer><Publisher>DC</Publisher><Year>2026</Year><Volume>1</Volume><Number>22</Number><LanguageISO>en</LanguageISO><Manga>YesAndRightToLeft</Manga></ComicInfo>`))
	page, err := writer.Create("001.png")
	if err != nil {
		t.Fatal(err)
	}
	if err := png.Encode(page, image.NewRGBA(image.Rect(0, 0, 12, 18))); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}

	book, err := inspectArchive(context.Background(), "fallback.cbz", archivePath, testLimits())
	if err != nil {
		t.Fatalf("inspectArchive() error = %v", err)
	}
	if book.Title != "Special Edition" || book.Series != "Absolute Batman" || book.Writer != "Scott Snyder" {
		t.Fatalf("unexpected metadata: %+v", book)
	}
	if book.PublicationYear == nil || *book.PublicationYear != 2026 {
		t.Fatalf("unexpected year: %+v", book.PublicationYear)
	}
	if book.ReadingDirection == nil || *book.ReadingDirection != "rtl" {
		t.Fatalf("unexpected reading direction: %+v", book.ReadingDirection)
	}
}
