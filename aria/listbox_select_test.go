package aria_test

import (
	"strings"
	"testing"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/aria"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/internal/testutil/rawwin"
	"github.com/kuronosu/tela/telatest"
)

// selPage is a list of fruits that prints its selection in order.
func selPage(o aria.ListBoxOptions[string], ev *events) func() tela.Node {
	return func() tela.Node {
		opts := o
		opts.Keys, opts.TextValue, opts.AriaLabel = fruits, capital, "Fruit"
		if ev != nil {
			opts.OnSelectionChange = func(s aria.Selection[string]) {
				ev.add(s.String())
				if o.OnSelectionChange != nil {
					o.OnSelectionChange(s)
				}
			}
		}
		lb := aria.UseListBox(opts)
		return Div(
			Div(lb.Props, Class("w-48"),
				Range(len(fruits), func(i int) tela.Node {
					k := fruits[i]
					return Div(lb.ItemProps(k), Key(k), Class("h-8 px-2 aria-selected:bg-sky-200"), Text(capital(k)))
				})),
			Textf("selected: %s", strings.Join(lb.Selected.In(fruits), ",")))
	}
}

func TestListBoxMultipleToggle(t *testing.T) {
	var ev events
	o := aria.ListBoxOptions[string]{SelectionMode: "multiple"}
	app := telatest.MountFunc(t, selPage(o, &ev), telatest.Size(300, 400))
	// With the default "toggle" behavior plain clicks build a selection.
	app.Click(option("Apple"))
	app.Click(option("Cherry"))
	app.ExpectText("selected: apple,cherry")
	app.Click(option("Apple"))
	app.ExpectText("selected: cherry")
	app.Expect(option("Cherry"), telatest.Selected(true))
	// Arrow keys move the focus without selecting; Space toggles.
	app.Focus(option("Cherry"))
	app.Press("ArrowDown")
	app.ExpectFocused(option("Date"))
	app.ExpectText("selected: cherry")
	app.Press("Space")
	app.ExpectText("selected: cherry,date")
	// Shift+Arrow extends from the anchor (the last toggled item).
	app.Press("Shift+ArrowDown")
	app.Press("Shift+ArrowDown")
	app.ExpectText("selected: cherry,date,elderberry,fig")
	app.Press("Shift+ArrowUp")
	app.ExpectText("selected: cherry,date,elderberry")
	if got := ev.take(); !strings.HasPrefix(got, "{apple} {apple cherry} {cherry} {cherry date}") {
		t.Errorf("events %q", got)
	}
	// Escape clears the selection.
	app.Press("Escape")
	app.ExpectText("selected:")
	app.Expect(option("Date"), telatest.Selected(false))
}

func TestListBoxSelectAll(t *testing.T) {
	var ev events
	o := aria.ListBoxOptions[string]{SelectionMode: "multiple", DisabledKeys: []string{"fig"}}
	app := telatest.MountFunc(t, selPage(o, &ev), telatest.Size(300, 400))
	app.Focus(option("Banana"))
	app.Press("Control+a")
	// All selects every item that can be selected, not the disabled ones.
	app.Expect(option("Apple"), telatest.Selected(true))
	app.Expect(option("Fig"), telatest.Selected(false))
	if got := ev.take(); got != "all" {
		t.Errorf("events %q", got)
	}
	// Toggling one resolves All to its keys minus that one.
	app.Click(option("Cherry"))
	app.ExpectText("selected: apple,banana,blueberry,date,elderberry")
	app.Expect(option("Cherry"), telatest.Selected(false))
}

func TestListBoxDisallowSelectAll(t *testing.T) {
	o := aria.ListBoxOptions[string]{SelectionMode: "multiple", DisallowSelectAll: true}
	app := telatest.MountFunc(t, selPage(o, nil), telatest.Size(300, 400))
	app.Focus(option("Banana"))
	app.Press("Control+a")
	app.ExpectText("selected:")
	app.Expect(option("Apple"), telatest.Selected(false))
}

func TestListBoxReplaceBehavior(t *testing.T) {
	o := aria.ListBoxOptions[string]{SelectionMode: "multiple", SelectionBehavior: "replace"}
	app := telatest.MountFunc(t, selPage(o, nil), telatest.Size(300, 400))
	// A click replaces; the arrows move the selection with the focus.
	app.Click(option("Banana"))
	app.ExpectText("selected: banana")
	app.Click(option("Date"))
	app.ExpectText("selected: date")
	app.Press("ArrowDown")
	app.ExpectText("selected: elderberry")
	app.Press("Home")
	app.ExpectText("selected: apple")
	// Shift+Arrow extends, Ctrl+Arrow moves without selecting and
	// Ctrl+Space toggles.
	app.Press("Shift+ArrowDown")
	app.Press("Shift+ArrowDown")
	app.ExpectText("selected: apple,banana,blueberry")
	app.Press("Control+ArrowDown")
	app.Press("Control+ArrowDown")
	app.ExpectFocused(option("Date"))
	app.ExpectText("selected: apple,banana,blueberry")
	app.Press("Control+Space")
	app.ExpectText("selected: apple,banana,blueberry,date")
	// A plain Space replaces the selection with the focused item.
	app.Press("Space")
	app.ExpectText("selected: date")
}

func (a *rawApp) clickWith(x, y float32, mods platform.Modifiers) {
	a.t.Helper()
	for _, act := range []platform.PointerAction{platform.PointerMove, platform.PointerDown, platform.PointerUp} {
		e := rawwin.PointerEvent(platform.PointerMouse, act, x, y, platform.ButtonPrimary, false)
		e.Mods = mods
		a.win.Inject(e)
		a.settle()
	}
}

func TestListBoxShiftAndCtrlClick(t *testing.T) {
	o := aria.ListBoxOptions[string]{SelectionMode: "multiple", SelectionBehavior: "replace"}
	app := mountRaw(t, tela.C(selPage(o, nil)), 300, 400)
	click := func(i int, mods platform.Modifiers) {
		app.clickWith(20, float32(i)*32+16, mods)
	}
	want := func(what, sel string) {
		t.Helper()
		if s := app.tree.AriaSnapshot(); !strings.Contains(s, "selected: "+sel) {
			t.Errorf("%s:\n%s", what, s)
		}
	}
	click(1, 0)
	want("click", "banana")
	click(4, platform.ModShift)
	want("shift+click", "banana,blueberry,cherry,date")
	click(2, platform.ModCtrl)
	want("ctrl+click", "banana,cherry,date")
	click(0, 0)
	want("click", "apple")
}

func TestListBoxControlled(t *testing.T) {
	var ev events
	page := func() tela.Node {
		sel, setSel := tela.UseState(aria.SelectionOf("banana"))
		refuse, setRefuse := tela.UseState(false)
		lb := aria.UseListBox(aria.ListBoxOptions[string]{
			Keys: fruits, SelectionMode: "multiple", Selected: sel, AriaLabel: "Fruit",
			OnSelectionChange: func(s aria.Selection[string]) {
				ev.add(s.String())
				if !refuse {
					setSel.Set(s)
				}
			}})
		return Div(
			Div(lb.Props,
				Range(len(fruits), func(i int) tela.Node {
					return Div(lb.ItemProps(fruits[i]), Key(fruits[i]), Class("h-8"), Text(capital(fruits[i])))
				})),
			Button(OnClick(func() { setRefuse.Set(true) }), Text("Refuse")),
			Textf("selected: %s", strings.Join(sel.In(fruits), ",")))
	}
	app := telatest.MountFunc(t, page, telatest.Size(300, 400))
	app.ExpectText("selected: banana")
	app.Click(option("Date"))
	app.ExpectText("selected: banana,date")
	// A controlled list shows what the app passes: a change the app does
	// not store is refused.
	app.Click(telatest.ByRole("button", "Refuse"))
	app.Click(option("Fig"))
	app.ExpectText("selected: banana,date")
	app.Expect(option("Fig"), telatest.Selected(false))
	if got := ev.take(); got != "{banana date} {banana date fig}" {
		t.Errorf("events %q", got)
	}
}

func TestListBoxUncontrolledDefault(t *testing.T) {
	o := aria.ListBoxOptions[string]{SelectionMode: "multiple", DefaultSelected: aria.SelectionOf("fig", "apple")}
	app := telatest.MountFunc(t, selPage(o, nil), telatest.Size(300, 400))
	app.ExpectText("selected: apple,fig")
	app.Click(option("Apple"))
	app.ExpectText("selected: fig")
}

func TestListBoxActions(t *testing.T) {
	var ev events
	// No selection: a click and Enter run the action.
	o := aria.ListBoxOptions[string]{OnAction: func(k string) { ev.add("action:" + k) }}
	app := telatest.MountFunc(t, selPage(o, nil), telatest.Size(300, 400))
	app.Click(option("Cherry"))
	app.Press("ArrowDown")
	app.Press("Enter")
	if got := ev.take(); got != "action:cherry action:date" {
		t.Errorf("actions %q", got)
	}
	// With "replace" and a selection, a click selects and Enter runs the
	// action.
	o = aria.ListBoxOptions[string]{SelectionMode: "single", SelectionBehavior: "replace",
		OnAction: func(k string) { ev.add("action:" + k) }}
	app2 := telatest.MountFunc(t, selPage(o, nil), telatest.Size(300, 400))
	app2.Click(option("Cherry"))
	app2.ExpectText("selected: cherry")
	if got := ev.take(); got != "" {
		t.Errorf("a click acted: %q", got)
	}
	app2.Press("Enter")
	if got := ev.take(); got != "action:cherry" {
		t.Errorf("Enter acted: %q", got)
	}
	app2.Press("Space")
	if got := ev.take(); got != "" {
		t.Errorf("Space acted: %q", got)
	}
}

func TestListBoxHorizontal(t *testing.T) {
	page := func() tela.Node {
		lb := aria.UseListBox(aria.ListBoxOptions[string]{Keys: fruits[:3], Orientation: "horizontal", AriaLabel: "Row"})
		return Div(lb.Props, Class("flex"),
			Range(3, func(i int) tela.Node {
				return Div(lb.ItemProps(fruits[i]), Key(fruits[i]), Class("h-8 w-20"), Text(capital(fruits[i])))
			}))
	}
	app := telatest.MountFunc(t, page, telatest.Size(300, 100))
	app.Focus(option("Apple"))
	app.Press("ArrowDown") // does nothing in a horizontal list
	app.ExpectFocused(option("Apple"))
	app.Press("ArrowRight")
	app.ExpectFocused(option("Banana"))
	app.Press("ArrowLeft")
	app.ExpectFocused(option("Apple"))
}

func TestListBoxSections(t *testing.T) {
	page := func() tela.Node {
		lb := aria.UseListBox(aria.ListBoxOptions[string]{Keys: fruits[:4], AriaLabel: "Food"})
		sec := func(name string, title string, keys ...string) tela.Node {
			s := lb.Section(name)
			items := []tela.Child{s.Props, Div(s.HeadingProps, Text(title))}
			for _, k := range keys {
				items = append(items, Div(lb.ItemProps(k), Key(k), Class("h-8"), Text(capital(k))))
			}
			return Div(items...)
		}
		return Div(lb.Props, sec("a", "A fruits", "apple", "banana"), sec("b", "B fruits", "blueberry", "cherry"))
	}
	app := telatest.MountFunc(t, page, telatest.Size(300, 400))
	app.ExpectAria(`
		- listbox "Food":
		  - group "A fruits":
		    - text: A fruits
		    - option "Apple"
		    - option "Banana"
		  - group "B fruits":
		    - text: B fruits
		    - option "Blueberry"
		    - option "Cherry"
	`)
	// The keyboard crosses sections.
	app.Focus(option("Banana"))
	app.Press("ArrowDown")
	app.ExpectFocused(option("Blueberry"))
}
