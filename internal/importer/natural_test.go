package importer

import (
	"reflect"
	"testing"
)

func TestNaturalSort(t *testing.T) {
	values := []string{"10.jpg", "2.jpg", "01.jpg", "1.jpg", "page20.png", "page3.png"}
	naturalSort(values)
	want := []string{"1.jpg", "01.jpg", "2.jpg", "10.jpg", "page3.png", "page20.png"}
	if !reflect.DeepEqual(values, want) {
		t.Fatalf("naturalSort() = %#v, want %#v", values, want)
	}
}

func TestNormalizeOriginalFilename(t *testing.T) {
	got := normalizeOriginalFilename(`C:\\Users\\Samuel\\volume-01.cbz`)
	if got != "volume-01.cbz" {
		t.Fatalf("normalizeOriginalFilename() = %q", got)
	}
}
