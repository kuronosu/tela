package standard_test

import (
	"slices"
	"testing"

	"github.com/go-text/typesetting/language"
	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/fonts/standard"
	"github.com/kuronosu/tela/internal/core"
	"github.com/kuronosu/tela/internal/stdfonts"
	"github.com/kuronosu/tela/internal/text"
)

func collection(t *testing.T) *text.Collection {
	t.Helper()
	fonts := make([]core.Font, len(standard.Fonts))
	for i, f := range standard.Fonts {
		fonts[i] = core.Font(f)
		if err := fonts[i].Verify(); err != nil {
			t.Fatal(err)
		}
	}
	c, err := core.NewFontCollection(fonts, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

// TestSameAsStdfonts: telatest draws with internal/stdfonts, apps import
// this package, and both must resolve every name to the same family.
func TestSameAsStdfonts(t *testing.T) {
	std := stdfonts.NewCollection()
	got := collection(t)
	if a, b := got.Families(), std.Families(); !slices.Equal(a, b) {
		t.Fatalf("families %v, stdfonts %v", a, b)
	}
	for _, g := range []string{"sans-serif", "system-ui", "ui-sans-serif", "serif", "ui-serif", "monospace", "ui-monospace", "emoji", "cursive", "Inter", "Source Serif 4"} {
		q := text.Query{Families: []string{g}}
		if a, b := got.Face(q, 16).Typeface().Family(), std.Face(q, 16).Typeface().Family(); a != b {
			t.Errorf("%s: %s, stdfonts %s", g, a, b)
		}
	}
	rs := []rune("Hi مرحبا 😀 ✓")
	runs := func(c *text.Collection) (out []string) {
		for _, r := range c.Itemize(rs, 0, len(rs), text.Query{Families: []string{"sans-serif"}}, 16, language.Latin, nil) {
			out = append(out, string(rs[r.Start:r.End])+"="+r.Face.Typeface().Family())
		}
		return out
	}
	if a, b := runs(got), runs(std); !slices.Equal(a, b) {
		t.Errorf("fallback runs %v, stdfonts %v", a, b)
	}
}

func TestAllUsable(t *testing.T) {
	if len(standard.Fonts) != 5 {
		t.Errorf("%d fonts, want 5", len(standard.Fonts))
	}
	var _ []tela.Font = standard.Fonts
}
