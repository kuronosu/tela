package aqua_test

import (
	"strings"
	"testing"

	"github.com/kuronosu/tela"

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

// The search field is an input, so it follows the controls whatever the
// chrome is: Sequoia's is borderless gray with its magnifier close to the
// edge, Tahoe's is the glass capsule of the input recipe and adds nothing.
func TestSearchField(t *testing.T) {
	for key, l := range combinations() {
		controls := strings.Contains(key, "controls Sequoia")
		field, icon := l.Regions.SearchField, l.Regions.SearchIcon
		if controls {
			if !strings.Contains(field, "border-transparent") || !strings.Contains(icon, "left-2") {
				t.Errorf("%s: SearchField = %q, SearchIcon = %q, want Sequoia's borderless field", key, field, icon)
			}
		} else if field != "" || !strings.Contains(icon, "z-10") {
			t.Errorf("%s: SearchField = %q, SearchIcon = %q, want Tahoe's glass input", key, field, icon)
		}
	}
}

// The pop-up button of both designs draws the double chevron unless the app
// sets Icon (kit.SelectStyle.DefaultIcon).
func TestPopupButtonGlyph(t *testing.T) {
	for key, l := range combinations() {
		if l.Style.Select.DefaultIcon == (tela.Icon{}) {
			t.Errorf("%s: Select.DefaultIcon is empty, want the double chevron", key)
		}
	}
}
