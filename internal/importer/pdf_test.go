package importer

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestInspectPDFAcceptsHeaderAndUsesFilename(t *testing.T) {
	path := filepath.Join(t.TempDir(), "book.pdf")
	if err := os.WriteFile(path, []byte("%PDF-1.7\n1 0 obj\n<<>>\nendobj\n%%EOF\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	book, err := inspectPDF(context.Background(), "The Stranger.pdf", path)
	if err != nil {
		t.Fatalf("inspectPDF() error = %v", err)
	}
	if book.Format != "pdf" || book.PageCount != 1 || book.Title != "The Stranger" {
		t.Fatalf("unexpected PDF metadata: %+v", book)
	}
}

func TestInspectPDFRejectsInvalidHeader(t *testing.T) {
	path := filepath.Join(t.TempDir(), "fake.pdf")
	if err := os.WriteFile(path, []byte("not a pdf"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := inspectPDF(context.Background(), "fake.pdf", path)
	if !errors.Is(err, ErrInvalidPDF) {
		t.Fatalf("inspectPDF() error = %v, want ErrInvalidPDF", err)
	}
}
