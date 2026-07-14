package ocr

import (
	"context"
	"errors"
	"testing"
)

func TestImageExtension(t *testing.T) {
	if got := imageExtension([]byte{0x89, 'P', 'N', 'G', 0x0d, 0x0a, 0x1a, 0x0a, 0, 0, 0, 0}); got != ".png" {
		t.Fatalf("png extension = %q", got)
	}
	if got := imageExtension([]byte{0xff, 0xd8, 0xff, 0, 0, 0, 0, 0, 0, 0, 0, 0}); got != ".jpg" {
		t.Fatalf("jpeg extension = %q", got)
	}
}

func TestUnavailable(t *testing.T) {
	service := &Service{workers: make(chan struct{}, 1)}
	if service.Status().Available {
		t.Fatal("expected OCR to be unavailable")
	}
	_, err := service.Recognize(context.Background(), make([]byte, 64), "por")
	if !errors.Is(err, ErrUnavailable) {
		t.Fatalf("Recognize() error = %v, want ErrUnavailable", err)
	}
}

func TestNormalizeText(t *testing.T) {
	got := normalizeText("  First   line\r\n\r\n  second\tline  \n")
	if want := "First line\nsecond line"; got != want {
		t.Fatalf("normalizeText() = %q, want %q", got, want)
	}
}
