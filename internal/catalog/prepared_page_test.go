package catalog

import (
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestOpenPreparedPage(t *testing.T) {
	dataDir := filepath.Join(t.TempDir(), "data")
	hash := "abc123"
	bookPath := filepath.Join(dataDir, "library", hash+".cbt")
	pagePath := filepath.Join(dataDir, "prepared", hash, "pages", "000000.jpg")
	if err := os.MkdirAll(filepath.Dir(bookPath), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(pagePath), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(bookPath, []byte("tar"), 0o640); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(pagePath, []byte("page"), 0o640); err != nil {
		t.Fatal(err)
	}
	asset, err := openPreparedPage(PageDescriptor{
		BookID: 1, PageNumber: 0, FilePath: bookPath, FileHash: hash,
		ArchivePath: "prepared:pages/000000.jpg", MediaType: "image/jpeg", Size: 4,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer asset.Stream.Close()
	content, err := io.ReadAll(asset.Stream)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "page" {
		t.Fatalf("content = %q", content)
	}
}

func TestOpenPreparedPageRejectsTraversal(t *testing.T) {
	_, err := openPreparedPage(PageDescriptor{FilePath: filepath.Join(t.TempDir(), "data", "library", "book.cbt"), FileHash: "hash", ArchivePath: "prepared:../secret"})
	if err == nil {
		t.Fatal("openPreparedPage accepted traversal")
	}
}
