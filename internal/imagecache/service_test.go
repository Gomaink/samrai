package imagecache

import "testing"

func TestNormalizeWidthUsesBucketsAndLimits(t *testing.T) {
	service := &Service{options: Options{MaxWidth: 3840}}

	tests := []struct {
		name      string
		requested int
		original  int
		want      int
	}{
		{name: "minimum bucket", requested: 100, original: 2000, want: 480},
		{name: "round up", requested: 721, original: 2000, want: 960},
		{name: "never upscale", requested: 3000, original: 2200, want: 2200},
		{name: "maximum", requested: 9000, original: 8000, want: 3840},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := service.normalizeWidth(test.requested, test.original); got != test.want {
				t.Fatalf("normalizeWidth(%d, %d) = %d, want %d", test.requested, test.original, got, test.want)
			}
		})
	}
}

func TestParseWidth(t *testing.T) {
	if got := ParseWidth("1920", 480, 3840); got != 1920 {
		t.Fatalf("ParseWidth valid = %d, want 1920", got)
	}
	if got := ParseWidth("invalid", 480, 3840); got != 480 {
		t.Fatalf("ParseWidth fallback = %d, want 480", got)
	}
	if got := ParseWidth("9000", 480, 3840); got != 3840 {
		t.Fatalf("ParseWidth maximum = %d, want 3840", got)
	}
}
