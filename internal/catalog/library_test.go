package catalog

import "testing"

func TestNormalizeBookListOptions(t *testing.T) {
	got := normalizeBookListOptions(BookListOptions{Status: "unknown", Sort: "unknown", Limit: 9999, Offset: -5})
	if got.Status != "all" || got.Sort != "recent" || got.Limit != 100 || got.Offset != 0 {
		t.Fatalf("normalizeBookListOptions() = %#v", got)
	}
}

func TestEscapeLike(t *testing.T) {
	if got, want := escapeLike(`100%_\\`), `100\%\_\\\\`; got != want {
		t.Fatalf("escapeLike() = %q, want %q", got, want)
	}
}

func TestBookOrderIsWhitelisted(t *testing.T) {
	if got := bookOrder("anything injected"); got != "b.created_at DESC, b.id DESC" {
		t.Fatalf("bookOrder fallback = %q", got)
	}
}
