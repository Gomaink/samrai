package catalog

import "testing"

func TestValidateEPUBAnnotationInput(t *testing.T) {
	valid := CreateEPUBAnnotationInput{
		SpineIndex:   0,
		ResourcePath: "OEBPS/chapter.xhtml",
		Kind:         "highlight",
		Color:        "yellow",
		SelectedText: "Once upon a time",
		Anchor: EPUBAnnotationAnchor{
			StartPath: "0/1/0", StartOffset: 0,
			EndPath: "0/1/0", EndOffset: 12,
		},
	}
	if err := validateEPUBAnnotationInput(valid); err != nil {
		t.Fatalf("valid annotation rejected: %v", err)
	}

	invalid := valid
	invalid.SelectedText = ""
	if err := validateEPUBAnnotationInput(invalid); err == nil {
		t.Fatal("empty highlighted text accepted")
	}

	invalid = valid
	invalid.Anchor.StartPath = ""
	if err := validateEPUBAnnotationInput(invalid); err == nil {
		t.Fatal("empty start path accepted")
	}
}
