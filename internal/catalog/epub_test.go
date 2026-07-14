package catalog

import (
	"strings"
	"testing"
)

func TestEPUBSearchUsesWholeWordsAndIgnoresAccents(t *testing.T) {
	words := epubWordTokens("Love lives in Montréal, but not inside lovelessness.")
	if got := findEPUBTokenSequence(words, tokenizeEPUBSearch("love")); len(got) != 1 {
		t.Fatalf("love matches = %v", got)
	}
	if got := findEPUBTokenSequence(words, tokenizeEPUBSearch("montreal")); len(got) != 1 {
		t.Fatalf("montreal matches = %v", got)
	}
}

func TestEPUBLocationAtEnd(t *testing.T) {
	cases := []struct {
		name string
		json string
		want bool
	}{
		{"fixed", `{"column_index":0,"column_count":1}`, true},
		{"middle", `{"column_index":1,"column_count":3,"chapter_progress":0.5}`, false},
		{"last column", `{"column_index":2,"column_count":3}`, true},
		{"progress", `{"column_index":1,"column_count":3,"chapter_progress":1}`, true},
		{"invalid", `{`, false},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			if got := epubLocationAtEnd(test.json); got != test.want {
				t.Fatalf("epubLocationAtEnd() = %v, want %v", got, test.want)
			}
		})
	}
}

func TestEPUBContentURLUsesCurrentReaderRevision(t *testing.T) {
	got := epubContentURL(7, "OEBPS/chapter.xhtml")
	if !strings.Contains(got, "reader=3") {
		t.Fatalf("epubContentURL = %q, want current reader revision", got)
	}
}
