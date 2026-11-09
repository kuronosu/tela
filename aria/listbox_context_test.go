package aria_test

import (
	"strings"
	"testing"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/aria"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/internal/core"
	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/internal/testutil/rawwin"
)

// ctxListPage is a multiple-selection list inside an area with a context
// menu; menuSaw is the selection of the build that shows the menu.
func ctxListPage(menuSaw *string) func() tela.Node {
	return func() tela.Node {
		lb := aria.UseListBox(aria.ListBoxOptions[string]{
			Keys: fruits, TextValue: capital, AriaLabel: "Fruit",
			SelectionMode: "multiple", SelectionBehavior: "replace",
		})
		sel := strings.Join(lb.Selected.In(fruits), ",")
		cm := aria.UseContextMenuTrigger(aria.ContextMenuTriggerOptions{
			OnOpenChange: func(open bool) {
				if open {
					*menuSaw = "opening"
				}
			},
		})
		if cm.Open && *menuSaw == "opening" {
			*menuSaw = sel
		}
		var ev events
		return Div(
			Div(cm.TriggerProps, Class("w-48"),
				Div(lb.Props,
					Range(len(fruits), func(i int) tela.Node {
						k := fruits[i]
						return Div(lb.ItemProps(k), Key(k), Class("h-8 px-2"), Text(capital(k)))
					}))),
			Textf("selected: %s", sel),
			Portal(If(cm.Open, ctxMenu{cm, &ev})))
	}
}

// A context click on an item that is not selected selects it (and only
// it) before the context menu opens, as Finder does; one on an item of the
// selection keeps the selection. On a Mac a Control+click is a context
// click; elsewhere it toggles (the primary modifier).
func TestListBoxContextClickSelects(t *testing.T) {
	for _, fl := range []platform.Flavor{platform.FlavorMac, platform.FlavorLinux, platform.FlavorWindows} {
		t.Run(fl.String(), func(t *testing.T) {
			var saw string
			b, host, win := rawwin.Mount(t, tela.C(ctxListPage(&saw)), 300, 400, 0, core.Config{Flavor: fl})
			app := &rawApp{t: t, b: b, host: host, win: win, tree: host.Tree()}
			y := func(i int) float32 { return float32(i)*32 + 16 }
			want := func(what, sel string) {
				t.Helper()
				if s := app.tree.AriaSnapshot(); !strings.Contains(s, `"selected: `+sel+`"`) {
					t.Errorf("%s: want %q:\n%s", what, sel, s)
				}
			}
			escape := func() {
				app.key(platform.KeyPress, platform.KeyEscape, platform.CodeEscape, false)
				app.key(platform.KeyRelease, platform.KeyEscape, platform.CodeEscape, false)
			}
			primary := platform.ModCtrl
			if fl == platform.FlavorMac {
				primary = platform.ModSuper
			}
			app.clickWith(20, y(0), 0)
			app.clickWith(20, y(2), primary)
			want("two selected", "apple,blueberry")
			// A right click on a selected item keeps the selection.
			app.rightClick(20, y(2))
			want("right click on the selection", "apple,blueberry")
			if !hasMenu(app) || saw != "apple,blueberry" {
				t.Errorf("menu %v, it saw %q", hasMenu(app), saw)
			}
			escape()
			// One on an item outside the selection selects it, before the
			// menu opens.
			app.rightClick(20, y(3))
			want("right click outside the selection", "cherry")
			if !hasMenu(app) || saw != "cherry" {
				t.Errorf("menu %v, it saw %q", hasMenu(app), saw)
			}
			escape()
			// Control+click on Date: a context click on a Mac, a toggle
			// elsewhere.
			app.clickWith(20, y(4), platform.ModCtrl)
			if fl == platform.FlavorMac {
				want("Control+click on a Mac", "date")
				if !hasMenu(app) || saw != "date" {
					t.Errorf("menu %v, it saw %q", hasMenu(app), saw)
				}
			} else {
				want("Control+click", "cherry,date")
				if hasMenu(app) {
					t.Errorf("Control+click opened the menu")
				}
			}
		})
	}
}
