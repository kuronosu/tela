package aria_test

import (
	"testing"
	"time"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/aria"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/telatest"
)

// shrinkPage is a list box of the keys a..f whose Delete key replaces the
// keys and the disabled keys, in one build, with the ones the test gives.
func shrinkPage(next, nextDisabled []string) func() tela.Node {
	type state struct{ keys, disabled []string }
	return func() tela.Node {
		st, setSt := tela.UseState(state{keys: []string{"a", "b", "c", "d", "e", "f"}})
		lb := aria.UseListBox(aria.ListBoxOptions[string]{Keys: st.keys, DisabledKeys: st.disabled, TextValue: capital, AriaLabel: "Letters"})
		return Div(Div(lb.Props, OnKeyDown(func(e tela.KeyboardEvent) {
			if e.Key == tela.KeyDelete {
				setSt.Set(state{keys: next, disabled: nextDisabled})
			}
		}), Range(len(st.keys), func(i int) tela.Node {
			return Div(lb.ItemProps(st.keys[i]), Key(st.keys[i]), Class("h-8"), Text(capital(st.keys[i])))
		})))
	}
}

// The focus is repaired with the disabled keys of the build that removed the
// focused item, not those of the build before.
func TestListBoxRepairUsesThisBuildsDisabledKeys(t *testing.T) {
	// b is focused and goes while c is disabled in the same build: the
	// focus must not land on c.
	app := telatest.MountFunc(t, shrinkPage([]string{"a", "c", "d", "e", "f"}, []string{"c"}), telatest.Size(300, 400))
	app.Focus(option("B"))
	app.Press("Delete")
	app.ExpectFocused(option("D"))
	// With every item that follows disabled too, the focus goes back.
	app = telatest.MountFunc(t, shrinkPage([]string{"a", "c", "d", "e", "f"}, []string{"c", "d", "e", "f"}), telatest.Size(300, 400))
	app.Focus(option("B"))
	app.Press("Delete")
	app.ExpectFocused(option("A"))
}

// React Aria's useListState: when several items go, the focus moves back by
// the number that went and takes the item that is there, not the one at the
// old index.
func TestListBoxRepairAfterSeveralItemsWent(t *testing.T) {
	// a..f, d focused; b, c and d go: [a e f]. Old index 3, 3 gone: index 1.
	app := telatest.MountFunc(t, shrinkPage([]string{"a", "e", "f"}, nil), telatest.Size(300, 400))
	app.Focus(option("D"))
	app.Press("Delete")
	app.ExpectFocused(option("E"))
	// The last items go: the focus goes to the one before them.
	app = telatest.MountFunc(t, shrinkPage([]string{"a", "b", "c"}, nil), telatest.Size(300, 400))
	app.Focus(option("E"))
	app.Press("Delete")
	app.ExpectFocused(option("C"))
}

// Only a press closes the tooltip of a focused trigger: Shift, arrows and
// shortcuts leave it, Enter and Space close it (usePress' onPressStart).
func TestTooltipOnlyAPressOfAKeyClosesIt(t *testing.T) {
	app := telatest.MountFunc(t, tooltipPage(aria.TooltipTriggerOptions{}), telatest.Size(400, 300), telatest.NoSettle())
	app.Press("Tab")
	if !tipOpen(app, "tip-save") {
		t.Fatal("the keyboard focus did not open the tooltip")
	}
	for _, key := range []string{"Shift", "ArrowDown", "ArrowLeft", "Control+s", "a"} {
		app.Press(key)
		if !tipOpen(app, "tip-save") {
			t.Fatalf("%s closed the tooltip", key)
		}
	}
	app.Press("Enter")
	if tipOpen(app, "tip-save") {
		t.Error("Enter left the tooltip open")
	}
	app.Press("Tab")
	app.Press("Shift+Tab")
	if !tipOpen(app, "tip-save") {
		t.Fatal("the focus back did not open the tooltip")
	}
	app.Press("Space")
	if tipOpen(app, "tip-save") {
		t.Error("Space left the tooltip open")
	}
}

// Without TextValue, list boxes and menus search the text of a key as
// fmt.Sprint prints it, as UseSelect does (tab lists have no typeahead).
func TestCollectionsDefaultTextValue(t *testing.T) {
	lb := telatest.MountFunc(t, func() tela.Node {
		l := aria.UseListBox(aria.ListBoxOptions[string]{Keys: fruits, AriaLabel: "Fruit"})
		return Div(l.Props, Range(len(fruits), func(i int) tela.Node {
			return Div(l.ItemProps(fruits[i]), Key(fruits[i]), Class("h-8"), Text(capital(fruits[i])))
		}))
	}, telatest.Size(300, 400), telatest.NoSettle())
	lb.Focus(option("Apple"))
	lb.Press("c")
	lb.ExpectFocused(option("Cherry"))

	menu := telatest.MountFunc(t, func() tela.Node {
		m := aria.UseMenu(aria.MenuOptions[string]{Keys: actions, AriaLabel: "Actions"})
		return Div(m.Props, Range(len(actions), func(i int) tela.Node {
			return Div(m.ItemProps(actions[i]), Key(actions[i]), Class("h-8"), Text(capital(actions[i])))
		}))
	}, telatest.Size(300, 400), telatest.NoSettle())
	menu.Focus(menuitem("Edit"))
	menu.Press("a")
	menu.ExpectFocused(menuitem("Archive"))
}

// A search lives a second; a Space typed inside it is a character of the
// search and selects nothing (React Aria's useTypeSelect).
func TestTypeaheadSwallowsSpaceWithinItsWindow(t *testing.T) {
	var ev events
	app := telatest.MountFunc(t, listPage(aria.ListBoxOptions[string]{SelectionMode: "single"}, &ev), telatest.Size(300, 400), telatest.NoSettle())
	app.Focus(option("Apple"))
	app.Press("b")
	app.ExpectFocused(option("Banana"))
	app.Press("Space")
	app.Expect(option("Banana"), telatest.Selected(false))
	app.Advance(1100 * time.Millisecond)
	app.Press("Space")
	app.Expect(option("Banana"), telatest.Selected(true))
}
