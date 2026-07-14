package importer

import (
	"archive/tar"
	"bytes"
	"context"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

func TestInspectPreparedComicImportsCBT(t *testing.T) {
	root := t.TempDir()
	archivePath := filepath.Join(root, "volume.cbt")
	file, err := os.Create(archivePath)
	if err != nil {
		t.Fatal(err)
	}
	writer := tar.NewWriter(file)
	writeTarBytes(t, writer, "ComicInfo.xml", []byte(`<?xml version="1.0"?><ComicInfo><Title>Prepared Volume</Title><Series>TAR Series</Series><Manga>YesAndRightToLeft</Manga></ComicInfo>`))
	for _, name := range []string{"pages/10.png", "pages/2.png", "pages/1.png"} {
		var buffer bytes.Buffer
		if err := png.Encode(&buffer, image.NewRGBA(image.Rect(0, 0, 12, 18))); err != nil {
			t.Fatal(err)
		}
		writeTarBytes(t, writer, name, buffer.Bytes())
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}

	preparedRoot := filepath.Join(root, "prepared")
	book, err := inspectPreparedComic(context.Background(), "volume.cbt", archivePath, "cbt", "abc123", preparedRoot, testLimits(), externalArchiveTools{})
	if err != nil {
		t.Fatalf("inspectPreparedComic() error = %v", err)
	}
	if book.Format != "cbt" || book.Title != "Prepared Volume" || book.Series != "TAR Series" {
		t.Fatalf("unexpected book: %+v", book)
	}
	if book.ReadingDirection == nil || *book.ReadingDirection != "rtl" {
		t.Fatalf("reading direction = %+v", book.ReadingDirection)
	}
	if len(book.Pages) != 3 {
		t.Fatalf("pages = %d", len(book.Pages))
	}
	for index, page := range book.Pages {
		want := preparedPathPrefix + "pages/00000" + string(rune('0'+index)) + ".png"
		if page.ArchivePath != want {
			t.Fatalf("page %d path = %q, want %q", index, page.ArchivePath, want)
		}
		if page.Width != 12 || page.Height != 18 || page.MediaType != "image/png" {
			t.Fatalf("unexpected page metadata: %+v", page)
		}
		if _, err := os.Stat(filepath.Join(book.PreparedDir, "pages", filepath.Base(page.ArchivePath))); err != nil {
			t.Fatalf("prepared page missing: %v", err)
		}
	}
}

func TestExtractTarSecureRejectsTraversal(t *testing.T) {
	archivePath := filepath.Join(t.TempDir(), "bad.cbt")
	file, err := os.Create(archivePath)
	if err != nil {
		t.Fatal(err)
	}
	writer := tar.NewWriter(file)
	writeTarBytes(t, writer, "../page.png", []byte("not-an-image"))
	_ = writer.Close()
	_ = file.Close()
	if err := extractTarSecure(context.Background(), archivePath, t.TempDir(), testLimits()); err == nil {
		t.Fatal("extractTarSecure() accepted path traversal")
	}
}

func TestParseSevenZipListing(t *testing.T) {
	listing := "Path = pages/1.jpg\nSize = 120\nPacked Size = 80\nAttributes = A\nEncrypted = -\n\nPath = pages/2.jpg\nSize = 140\nPacked Size = 90\nAttributes = A\nEncrypted = -\n"
	entries, err := parseSevenZipListing(listing)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 || entries[0].Name != "pages/1.jpg" || entries[1].Size != 140 {
		t.Fatalf("unexpected entries: %+v", entries)
	}
	selected, err := validateListedEntries(entries, testLimits())
	if err != nil {
		t.Fatal(err)
	}
	if len(selected) != 2 {
		t.Fatalf("selected = %d", len(selected))
	}
}

func writeTarBytes(t *testing.T, writer *tar.Writer, name string, value []byte) {
	t.Helper()
	if err := writer.WriteHeader(&tar.Header{Name: name, Mode: 0o600, Size: int64(len(value)), Typeflag: tar.TypeReg}); err != nil {
		t.Fatal(err)
	}
	if _, err := writer.Write(value); err != nil {
		t.Fatal(err)
	}
}
