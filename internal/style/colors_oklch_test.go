package style

import (
	"regexp"
	"strings"
	"testing"

	"github.com/kuronosu/tela/internal/gfx"
)

// TestParseColorOKLCHIsStrict: oklch() and oklab() follow CSS Color 4 like
// rgb() and hsl() (ADR-193): they only have the space form.
func TestParseColorOKLCHIsStrict(t *testing.T) {
	for _, tt := range []struct{ in, want string }{
		{"oklch(0.5,0.1/200)", "not with commas"},
		{"oklch(0.5, 0.1, 200)", "not with commas"},
		{"oklch(0.5 0.1 200, 0.5)", "not with commas"},
		{"oklab(0.5, 0.1, 0.1)", "oklab() separates its arguments with spaces"},
		{"oklch(0.5 0.1 200 0.5)", "oklch() takes three components"},
		{"oklch(0.5 0.1)", "oklch() takes three components"},
		{"oklab(0.5 0.1)", "oklab() takes three components"},
		{"oklch(0.5 0.1 200 /)", "not with commas"},
		{"oklch(x 0.1 200)", `invalid color component "x"`},
		{"oklch(0.5 0.1 red)", `invalid hue "red"`},
		{"oklch(0.5 0.1 200 / z)", `invalid color component "z"`},
		{"oklab(0.5 0.1 90deg)", `invalid color component "90deg"`},
	} {
		_, err := parseColor(tt.in)
		if err == nil || !strings.Contains(err.Error(), tt.want) {
			t.Errorf("parseColor(%q) error = %v, want one with %q", tt.in, err, tt.want)
		}
	}
	for _, tt := range []struct {
		in   string
		want gfx.Color
	}{
		{"oklch(0.5 0.1 200)", gfx.OKLCH(0.5, 0.1, 200, 1)},
		{"oklch(0.5 0.1 200 / 0.5)", gfx.OKLCH(0.5, 0.1, 200, 0.5)},
		{"oklch(0.5 0.1 200/50%)", gfx.OKLCH(0.5, 0.1, 200, 0.5)},
		{"oklch(0.5 0.1 0.5turn)", gfx.OKLCH(0.5, 0.1, 180, 1)},
		{"oklch(1 0 none)", gfx.OKLCH(1, 0, 0, 1)},
		{"oklch(0.5 0.1 200 / none)", gfx.OKLCH(0.5, 0.1, 200, 0)},
		{"oklab(none 0.1 -0.1)", gfx.Oklab(0, 0.1, -0.1, 1)},
	} {
		got, err := parseColor(tt.in)
		if err != nil {
			t.Errorf("parseColor(%q): %v", tt.in, err)
			continue
		}
		if !colorNear(got.c, tt.want, 1e-4) {
			t.Errorf("parseColor(%q) = %v, want %v", tt.in, got.c, tt.want)
		}
	}
	// The underscore form of a class.
	if err := Compile("bg-[oklch(0.5_0.1_200)]").Err; err != nil {
		t.Errorf("bg-[oklch(0.5_0.1_200)]: %v", err)
	}
	if Compile("bg-[oklch(0.5,0.1,200)]").Err == nil {
		t.Error("bg-[oklch(0.5,0.1,200)] compiled")
	}
}

// TestColorFuncErrorExamples: the color functions a message quotes as an
// example are valid colors.
func TestColorFuncErrorExamples(t *testing.T) {
	call := regexp.MustCompile(`(?:rgb|hsl|oklch|oklab)\(([^)]+)\)`)
	var inputs []string
	for _, fn := range []string{"rgb", "hsl", "oklch", "oklab"} {
		inputs = append(inputs, fn+"(1)", fn+"(1,2 3)", fn+"(1 2 3, 4)", fn+"(1,2,3 / 4)", fn+"(1,2%,3)", fn+"(120,100,25)", fn+"(x 1 1)", fn+"(1 1 1 / y)")
	}
	seen := 0
	for _, in := range inputs {
		_, err := parseColor(in)
		if err == nil {
			continue
		}
		for _, m := range call.FindAllString(err.Error(), -1) {
			seen++
			if _, perr := parseColor(m); perr != nil {
				t.Errorf("parseColor(%q): the example %q in %q is not valid: %v", in, m, err, perr)
			}
		}
	}
	if seen < 8 {
		t.Errorf("found only %d examples in the messages", seen)
	}
}
