package importer

import "testing"

func TestApplyUploadMetadata(t *testing.T) {
	book := inspectedBook{Title: "Internal Title", Series: "Internal Series", Volume: "9"}
	applyUploadMetadata(&book, UploadMetadata{Title: "Berserk 01", Series: "Berserk", Volume: "1"})
	if book.Title != "Berserk 01" || book.Series != "Berserk" || book.Volume != "1" {
		t.Fatalf("metadata not applied: %+v", book)
	}
}

func TestApplyUploadMetadataKeepsDiscoveredValuesWhenEmpty(t *testing.T) {
	book := inspectedBook{Title: "Internal Title", Series: "Internal Series", Volume: "9"}
	applyUploadMetadata(&book, UploadMetadata{})
	if book.Title != "Internal Title" || book.Series != "Internal Series" || book.Volume != "9" {
		t.Fatalf("empty metadata changed discovered values: %+v", book)
	}
}

func TestNormalizeUploadMetadataLimitsAndRemovesNulls(t *testing.T) {
	metadata := normalizeUploadMetadata(UploadMetadata{
		Title:  "  Berserk\x00 01  ",
		Series: "  Berserk  ",
		Volume: "  1  ",
	})
	if metadata.Title != "Berserk 01" || metadata.Series != "Berserk" || metadata.Volume != "1" {
		t.Fatalf("unexpected normalized metadata: %+v", metadata)
	}
}
