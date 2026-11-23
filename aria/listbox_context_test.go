package aria_test

import (
	"strings"
	"testing"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/aria"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/telatest"
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
	for _, goos := range []string{"darwin", "linux", "windows"} {
		t.Run(goos, func(t *testing.T) {
			var saw string
			app := telatest.MountFunc(t, ctxListPage(&saw), telatest.Size(300, 400), telatest.PlatformKeys(goos))
			y := func(i int) float32 { return float32(i)*32 + 16 }
			want := func(what, sel string) {
				t.Helper()
				app.ExpectText("selected: " + sel)
			}
			app.ClickAt(20, y(0))
			app.ClickAt(20, y(2), telatest.Modifiers("ControlOrMeta"))
			want("two selected", "apple,blueberry")
			// A right click on a selected item keeps the selection.
			app.ClickAt(20, y(2), telatest.Right())
			want("right click on the selection", "apple,blueberry")
			if app.Count(ctxMenuLoc) != 1 || saw != "apple,blueberry" {
				t.Errorf("menu %v, it saw %q", app.Count(ctxMenuLoc), saw)
			}
			app.Press("Escape")
			// One on an item outside the selection selects it, before the
			// menu opens.
			app.ClickAt(20, y(3), telatest.Right())
			want("right click outside the selection", "cherry")
			if app.Count(ctxMenuLoc) != 1 || saw != "cherry" {
				t.Errorf("menu %v, it saw %q", app.Count(ctxMenuLoc), saw)
			}
			app.Press("Escape")
			// Control+click on Date: a context click on a Mac, a toggle
			// elsewhere.
			app.ClickAt(20, y(4), telatest.Modifiers("Control"))
			if goos == "darwin" {
				want("Control+click on a Mac", "date")
				if app.Count(ctxMenuLoc) != 1 || saw != "date" {
					t.Errorf("menu %v, it saw %q", app.Count(ctxMenuLoc), saw)
				}
			} else {
				want("Control+click", "cherry,date")
				if app.Count(ctxMenuLoc) != 0 {
					t.Errorf("Control+click opened the menu")
				}
			}
		})
	}
}
