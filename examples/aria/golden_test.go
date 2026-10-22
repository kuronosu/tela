package main

import (
	"fmt"
	"strings"
	"testing"

	"github.com/kuronosu/tela/telatest"
)

// schemes are the two looks of the screen: every golden exists in both.
var schemes = []struct {
	name string
	opts []telatest.Option
}{
	{"light", nil},
	{"dark", []telatest.Option{telatest.Dark()}},
}

// slug turns a preview name such as "2 Actions menu" into "actions_menu".
func slug(name string) string {
	_, rest, _ := strings.Cut(name, " ")
	return strings.ReplaceAll(strings.ToLower(rest), " ", "_")
}

// TestGolden renders each preview, the main screen and every overlay open, in
// both schemes at scales 1 and 2. The comparison is bit by bit:
// `TELA_UPDATE=1 CGO_ENABLED=0 go test -run TestGolden ./examples/aria`
// writes the pictures, which must be looked at before they are committed.
func TestGolden(t *testing.T) {
	for _, name := range Previews.Names() {
		for _, s := range schemes {
			t.Run(fmt.Sprintf("%s/%s", name, s.name), func(t *testing.T) {
				a := mount(t, Previews[name], s.opts...)
				a.GoldenScales(fmt.Sprintf("aria_%s_%s", slug(name), s.name), 1, 2)
			})
		}
	}
}

// TestGoldenKeyboardStates are the states only the keyboard makes: the ring of
// focus-visible: on a button reached with Tab (a click shows none), and the
// highlighted option of the combobox while the focus stays on the input.
func TestGoldenKeyboardStates(t *testing.T) {
	for _, s := range schemes {
		t.Run("focus ring/"+s.name, func(t *testing.T) {
			a := mount(t, Page{}, s.opts...)
			tab(a, 3)
			a.ExpectFocused(viewButton)
			a.GoldenScales("aria_focus_ring_"+s.name, 1, 2)
		})
		t.Run("highlight/"+s.name, func(t *testing.T) {
			a := mount(t, Page{}, s.opts...)
			a.Press("Tab")
			a.Press("Control+a")
			a.Type("se")
			a.Press("ArrowDown")
			a.Press("ArrowDown")
			a.Expect(highlighted, telatest.HasName("analytics-warehouse Database"))
			a.GoldenScales("aria_combobox_highlight_"+s.name, 1, 2)
		})
	}
}

// TestAriaGoldens pin the whole aria snapshot of the screen, closed and with
// the combobox's list open.
func TestAriaGoldens(t *testing.T) {
	mount(t, Page{}).AriaGolden("aria_screen.aria.yml")
	a := mount(t, Page{})
	a.Press("Tab")
	a.Press("Control+a")
	a.Type("se")
	a.Press("ArrowDown")
	a.AriaGolden("aria_combobox_open.aria.yml")
}
