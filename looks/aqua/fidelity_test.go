package aqua_test

import (
	"strings"
	"testing"

	"github.com/kuronosu/tela/looks"
	"github.com/kuronosu/tela/looks/aqua"
)

// The glyphs of a sidebar entry are tinted with the accent, selected or not,
// in both designs and in any mix that has the design's chrome; the toolbar's
// stay gray.
func TestSidebarGlyphsAreTinted(t *testing.T) {
	for _, d := range designs {
		l := aqua.Look(d)
		if v := l.Style.Toggle.IconVariants[looks.Sidebar]; !strings.Contains(v, "text-primary") {
			t.Errorf("%v: sidebar glyph classes = %q, want the accent", d, v)
		}
		if v := l.Style.Toggle.IconVariants[looks.Toolbar]; v != "" {
			t.Errorf("%v: toolbar glyph classes = %q, want none", d, v)
		}
	}
	mix := aqua.Look(aqua.Sequoia, aqua.Chrome(aqua.Tahoe))
	if v := mix.Style.Toggle.IconVariants[looks.Sidebar]; !strings.Contains(v, "text-primary") {
		t.Errorf("Tahoe chrome: sidebar glyph classes = %q, want the accent", v)
	}
}

// Sequoia's search field is borderless gray; Tahoe's is glass and adds
// nothing.
func TestSearchField(t *testing.T) {
	if c := aqua.Look(aqua.Sequoia).Regions.SearchField; !strings.Contains(c, "border-transparent") {
		t.Errorf("Sequoia SearchField = %q, want a borderless field", c)
	}
	if c := aqua.Look(aqua.Tahoe).Regions.SearchField; c != "" {
		t.Errorf("Tahoe SearchField = %q, want none", c)
	}
}
