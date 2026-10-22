package aria_test

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/aria"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/telatest"
)

var fruits = []string{"apple", "banana", "blueberry", "cherry", "date", "elderberry", "fig"}

func capital(s string) string { return strings.ToUpper(s[:1]) + s[1:] }

// listPage renders a list box of fruits with the options o (Keys and
// TextValue are filled in when empty) and logs what the callbacks saw.
func listPage(o aria.ListBoxOptions[string], ev *events) func() tela.Node {
	return func() tela.Node {
		if o.Keys == nil {
			o.Keys = fruits
		}
		if o.TextValue == nil {
			o.TextValue = capital
		}
		if o.AriaLabel == "" {
			o.AriaLabel = "Fruit"
		}
		lb := aria.UseListBox(o)
		return Div(
			Button(Class("h-8"), Text("Before")),
			Div(lb.Props, Class("w-48 overflow-y-auto"),
				Range(len(o.Keys), func(i int) tela.Node {
					k := o.Keys[i]
					return Div(lb.ItemProps(k), Key(k), Class("h-8 px-2 aria-selected:bg-sky-200 focus-visible:ring-2"),
						Text(capital(k)))
				})),
			Button(Class("h-8"), Text("After")))
	}
}

func option(name string) telatest.Locator { return telatest.ByRole("option", name) }

func TestListBoxKeyboardNavigation(t *testing.T) {
	app := telatest.MountFunc(t, listPage(aria.ListBoxOptions[string]{}, &events{}), telatest.Size(300, 400))
	// Tab enters the list: the first item takes the focus.
	app.Press("Tab")
	app.Press("Tab")
	app.ExpectFocused(option("Apple"))
	app.Press("ArrowDown")
	app.ExpectFocused(option("Banana"))
	app.Press("ArrowDown")
	app.Press("ArrowDown")
	app.ExpectFocused(option("Cherry"))
	app.Press("ArrowUp")
	app.ExpectFocused(option("Blueberry"))
	app.Press("End")
	app.ExpectFocused(option("Fig"))
	// No wrap by default.
	app.Press("ArrowDown")
	app.ExpectFocused(option("Fig"))
	app.Press("Home")
	app.ExpectFocused(option("Apple"))
	app.Press("ArrowUp")
	app.ExpectFocused(option("Apple"))
	// Only the focused item is a tab stop: Tab leaves the list.
	app.Press("Tab")
	app.ExpectFocused(telatest.ByRole("button", "After"))
	app.Press("Shift+Tab")
	app.ExpectFocused(option("Apple")) // the list remembers its focused item
	app.Press("Shift+Tab")
	app.ExpectFocused(telatest.ByRole("button", "Before"))
}

func TestListBoxWrapAndDisabled(t *testing.T) {
	o := aria.ListBoxOptions[string]{ShouldFocusWrap: true, DisabledKeys: []string{"apple", "cherry", "fig"}}
	app := telatest.MountFunc(t, listPage(o, &events{}), telatest.Size(300, 400))
	app.Press("Tab")
	app.Press("Tab")
	app.ExpectFocused(option("Banana")) // apple is disabled
	app.Expect(option("Apple"), telatest.Disabled(true))
	app.Press("ArrowDown")
	app.ExpectFocused(option("Blueberry"))
	app.Press("ArrowDown") // cherry is skipped
	app.ExpectFocused(option("Date"))
	app.Press("End")
	app.ExpectFocused(option("Elderberry")) // fig is disabled
	app.Press("ArrowDown")                  // wraps past the disabled end and start
	app.ExpectFocused(option("Banana"))
	app.Press("ArrowUp")
	app.ExpectFocused(option("Elderberry"))
	// A disabled item ignores the mouse.
	r := app.Bounds(option("Apple"))
	app.ClickAt(r.X+10, r.Y+10)
	app.ExpectFocused(option("Elderberry"))
}

func TestListBoxTypeahead(t *testing.T) {
	app := telatest.MountFunc(t, listPage(aria.ListBoxOptions[string]{}, &events{}), telatest.Size(300, 400), telatest.NoSettle())
	app.Focus(option("Apple"))
	app.Press("b")
	app.ExpectFocused(option("Banana"))
	// A longer search narrows the match; it starts at the focused item.
	app.Press("l")
	app.ExpectFocused(option("Blueberry"))
	app.Press("Space") // a space inside a search is a character
	app.ExpectFocused(option("Blueberry"))
	// The search expires after a second without keys.
	app.Advance(1100 * time.Millisecond)
	app.Press("e")
	app.ExpectFocused(option("Elderberry"))
	app.Advance(500 * time.Millisecond)
	app.Press("l") // "el" still matches
	app.ExpectFocused(option("Elderberry"))
	app.Advance(1100 * time.Millisecond)
	app.Press("d")
	app.ExpectFocused(option("Date"))
	// Case is ignored, and a search that matches nothing keeps the focus.
	app.Advance(1100 * time.Millisecond)
	app.Press("Shift+C")
	app.ExpectFocused(option("Cherry"))
	app.Press("z")
	app.ExpectFocused(option("Cherry"))
}

func TestListBoxPages(t *testing.T) {
	// Eight 32 px items fit the 256 px viewport; the list has 40.
	keys := make([]string, 40)
	for i := range keys {
		keys[i] = fmt.Sprintf("item%02d", i)
	}
	page := func() tela.Node {
		lb := aria.UseListBox(aria.ListBoxOptions[string]{Keys: keys, AriaLabel: "Items"})
		return Div(lb.Props, Class("h-64 w-48 overflow-y-auto"),
			Range(len(keys), func(i int) tela.Node {
				return Div(lb.ItemProps(keys[i]), Key(keys[i]), Class("h-8 shrink-0"), Text(keys[i]))
			}))
	}
	app := telatest.MountFunc(t, page, telatest.Size(300, 400))
	app.Focus(option("item00"))
	app.Press("PageDown")
	app.ExpectFocused(option("item08"))
	app.Press("PageDown")
	app.ExpectFocused(option("item16"))
	app.Press("PageUp")
	app.ExpectFocused(option("item08"))
	app.Press("PageUp")
	app.Press("PageUp")
	app.ExpectFocused(option("item00"))
	app.Press("End")
	app.ExpectFocused(option("item39"))
	app.Press("PageDown")
	app.ExpectFocused(option("item39"))
	// The focused item is scrolled into view.
	r, b := app.Bounds(option("item39")), app.Bounds(telatest.ByRole("listbox", "Items"))
	if r.Y < b.Y || r.Y+r.Height > b.Y+b.Height+0.5 {
		t.Errorf("item39 at %+v is outside the list at %+v", r, b)
	}
}

func TestListBoxSingleSelection(t *testing.T) {
	var ev events
	o := aria.ListBoxOptions[string]{SelectionMode: "single",
		OnSelectionChange: func(s aria.Selection[string]) { ev.add(s.String()) }}
	app := telatest.MountFunc(t, listPage(o, &ev), telatest.Size(300, 400))
	app.Click(option("Banana"))
	app.Expect(option("Banana"), telatest.Selected(true))
	app.Press("Space")
	app.Expect(option("Banana"), telatest.Selected(false))
	app.Press("ArrowDown")
	app.Press("Enter")
	app.Expect(option("Blueberry"), telatest.Selected(true))
	app.Click(option("Date"))
	app.Expect(option("Blueberry"), telatest.Selected(false))
	app.Expect(option("Date"), telatest.Selected(true))
	if got := ev.take(); got != "{banana} {} {blueberry} {date}" {
		t.Errorf("selection events %q", got)
	}
	app.ExpectAria(`
		- button "Before"
		- listbox "Fruit":
		  - option "Apple"
		  - option "Banana"
		  - option "Blueberry"
		  - option "Cherry"
		  - option "Date" [selected]
		  - option "Elderberry"
		  - option "Fig"
		- button "After"
	`)
}

func TestListBoxDisallowEmptySelection(t *testing.T) {
	o := aria.ListBoxOptions[string]{SelectionMode: "single", DisallowEmptySelection: true, DefaultSelected: aria.SelectionOf("cherry")}
	app := telatest.MountFunc(t, listPage(o, &events{}), telatest.Size(300, 400))
	app.Expect(option("Cherry"), telatest.Selected(true))
	app.Click(option("Cherry"))
	app.Expect(option("Cherry"), telatest.Selected(true))
}

func TestListBoxEntersOnTheSelectedItem(t *testing.T) {
	o := aria.ListBoxOptions[string]{SelectionMode: "single", DefaultSelected: aria.SelectionOf("date")}
	app := telatest.MountFunc(t, listPage(o, &events{}), telatest.Size(300, 400))
	app.Press("Tab")
	app.Press("Tab")
	app.ExpectFocused(option("Date"))
}
