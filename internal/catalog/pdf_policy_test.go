package catalog

import "testing"

func TestRecommendedOCRMode(t *testing.T) {
	tests := map[string]string{
		"text":         "off",
		"comic":        "off",
		"scanned_book": "on_demand",
		"mixed":        "on_demand",
		"unknown":      "on_demand",
	}
	for kind, want := range tests {
		if got := recommendedOCRMode(kind); got != want {
			t.Fatalf("recommendedOCRMode(%q) = %q, want %q", kind, got, want)
		}
	}
}

func TestOCRStatusForMode(t *testing.T) {
	if got := ocrStatusForMode("off", "comic", 0, 220); got != "disabled" {
		t.Fatalf("comic off status = %q, want disabled", got)
	}
	if got := ocrStatusForMode("off", "text", 0, 100); got != "not_needed" {
		t.Fatalf("text off status = %q, want not_needed", got)
	}
	if got := ocrStatusForMode("on_demand", "scanned_book", 3, 100); got != "partial" {
		t.Fatalf("on-demand partial status = %q, want partial", got)
	}
	if got := ocrStatusForMode("full", "scanned_book", 100, 100); got != "complete" {
		t.Fatalf("full complete status = %q, want complete", got)
	}
}
