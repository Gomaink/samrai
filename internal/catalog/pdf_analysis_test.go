package catalog

import (
	"strings"
	"testing"
)

func TestNormalizePDFText(t *testing.T) {
	got := normalizePDFText("  First\x00 line\n\t second   line  ")
	if want := "First line second line"; got != want {
		t.Fatalf("normalizePDFText() = %q, want %q", got, want)
	}
}

func TestLimitPDFMetadata(t *testing.T) {
	got := limitPDFMetadata("  very long title  ", 6)
	if want := "title"; got != want {
		t.Fatalf("limitPDFMetadata() = %q, want %q", got, want)
	}
}

func TestCountFold(t *testing.T) {
	if got, want := countFold("Book book BOOK", "book"), 3; got != want {
		t.Fatalf("countFold() = %d, want %d", got, want)
	}
}

func TestPDFSearchExcerptUsesMatchedPosition(t *testing.T) {
	text := strings.Repeat("before ", 30) + "Montréal" + strings.Repeat(" after", 30)
	got := pdfSearchExcerpt(text, "montreal")
	if !strings.Contains(strings.ToLower(got), "montreal") {
		t.Fatalf("pdfSearchExcerpt() did not retain query: %q", got)
	}
	if !strings.HasPrefix(got, "…") || !strings.HasSuffix(got, "…") {
		t.Fatalf("pdfSearchExcerpt() should mark trimmed context: %q", got)
	}
}

func TestNormalizePDFSearchText(t *testing.T) {
	got := normalizePDFSearchText("  MONTRÉAL — Digital!  ")
	if want := "montreal digital"; got != want {
		t.Fatalf("normalizePDFSearchText() = %q, want %q", got, want)
	}
}

func TestPDFSearchMatchesWholeWordsAndPhrases(t *testing.T) {
	text := "True love is not merely lovely. It remained out of love."
	matches := findPDFSearchMatches(text, "love", "ocr")
	if got, want := len(matches), 2; got != want {
		t.Fatalf("findPDFSearchMatches() = %d, want %d", got, want)
	}
	phrase := findPDFSearchMatches("Although he has lived in Montreal for eight years, he hardly...", "eight years", "ocr")
	if got, want := len(phrase), 1; got != want {
		t.Fatalf("phrase matches = %d, want %d", got, want)
	}
}

func TestPDFSearchDoesNotCrossWordBoundaries(t *testing.T) {
	text := "the true poet died and a friend reflects"
	if got := findPDFSearchMatches(text, "love", "ocr"); len(got) != 0 {
		t.Fatalf("OCR search crossed word boundary: %#v", got)
	}
	if got := findPDFSearchMatches(text, "love", "pdf"); len(got) != 0 {
		t.Fatalf("PDF search crossed word boundary: %#v", got)
	}
}

func TestPDFSearchRepairsOnlyNativeSplitTokens(t *testing.T) {
	text := "Digi tal Font and Mont réal"
	matches := findPDFSearchMatches(text, "digital", "pdf")
	if got, want := len(matches), 1; got != want || matches[0].Kind != "repaired" {
		t.Fatalf("native repaired matches = %#v, want one repaired match", matches)
	}
	matches = findPDFSearchMatches(text, "Montreal", "pdf")
	if got, want := len(matches), 1; got != want || matches[0].Kind != "repaired" {
		t.Fatalf("native repaired suffix matches = %#v, want one repaired match", matches)
	}
	if got := findPDFSearchMatches(text, "digital", "ocr"); len(got) != 0 {
		t.Fatalf("OCR tokens must not be silently repaired: %#v", got)
	}
}

func TestPDFSearchAccentInsensitive(t *testing.T) {
	matches := findPDFSearchMatches("Montréal and café", "montreal", "ocr")
	if got, want := len(matches), 1; got != want {
		t.Fatalf("accent-insensitive phrase matches = %d, want %d", got, want)
	}
}
