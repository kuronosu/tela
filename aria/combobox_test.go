package aria_test

import (
	"strings"
	"testing"
	"time"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/aria"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/telatest"
)

// comboPage renders a combobox of fruits (a label, an input, a toggle
// button and a button after them) with the options o, and logs what its
// callbacks saw. The highlighted item has the class bg-sky-200.
func comboPage(o aria.ComboboxOptions[string], ev *events) func() tela.Node {
	return func() tela.Node {
		if o.Items == nil && o.Keys == nil {
			o.Items = fruits
		}
		if o.TextValue == nil {
			o.TextValue = capital
		}
		if ev != nil {
			if o.OnSelectionChange == nil {
				o.OnSelectionChange = func(s aria.Selection[string]) {
					k, ok := s.First()
					if !ok {
						k = "none"
					}
					ev.add("selected " + k)
				}
			}
			if o.OnInputChange == nil {
				o.OnInputChange = func(v string) { ev.add("input " + v) }
			}
		}
		return comboNode(o)
	}
}

// comboNode is the markup of the page, a custom hook's worth of code.
func comboNode(o aria.ComboboxOptions[string]) tela.Node {
	return tela.C(func() tela.Node { return useComboNode(o) })
}

func useComboNode(o aria.ComboboxOptions[string]) tela.Node {
	cb := aria.UseCombobox(o)
	return Div(Class("relative h-full w-full"),
		Div(cb.LabelProps, Class("h-6"), Text("Fruit")),
		Div(Class("flex h-8 w-56"),
			Input(cb.InputProps, Class("h-8 w-48 border")),
			Button(cb.ButtonProps, Class("h-8 w-8"), Text("v"))),
		Button(Class("h-8 w-24"), Text("Other")),
		Portal(If(cb.Open,
			Div(cb.PopoverProps, Class("w-48 bg-white shadow"),
				Div(cb.ListBox.Props, Class("max-h-48 overflow-y-auto"),
					Range(len(cb.Keys), func(i int) tela.Node {
						k := cb.Keys[i]
						return Div(cb.ListBox.ItemProps(k), Key(k), Class("h-8 px-2"),
							If(cb.ListBox.IsFocused(k), Class("bg-sky-200")), Text(o.TextValue(k)))
					}))))))
}

var comboInput = telatest.ByRole("combobox", "Fruit")
var highlighted = telatest.ByClass("bg-sky-200")

func (a *comboApp) closed() { a.t.Helper(); a.ExpectCount(telatest.ByRole("listbox", ""), 0) }

// comboApp is a telatest.App with a few helpers.
type comboApp struct {
	*telatest.App
	t *testing.T
}

func mountCombo(t *testing.T, o aria.ComboboxOptions[string], ev *events) *comboApp {
	t.Helper()
	return &comboApp{telatest.MountFunc(t, comboPage(o, ev), telatest.Size(400, 400)), t}
}

func (a *comboApp) options(names ...string) {
	a.t.Helper()
	var lines []string
	for _, n := range names {
		lines = append(lines, `  - option "`+n+`"`)
	}
	a.ExpectCount(telatest.ByRole("option", ""), len(names))
	snap := a.AriaSnapshot()
	i := strings.Index(snap, "- listbox")
	if i < 0 {
		a.t.Fatalf("no listbox in\n%s", snap)
	}
	got := strings.TrimSpace(snap[i:])
	want := `- listbox "Fruit":` + "\n" + strings.Join(lines, "\n")
	if strings.ReplaceAll(got, " [selected]", "") != want {
		a.t.Errorf("list\n%s\nwant\n%s", got, want)
	}
}

func TestComboboxTypingFiltersAndOpens(t *testing.T) {
	var ev events
	app := mountCombo(t, aria.ComboboxOptions[string]{}, &ev)
	app.Focus(comboInput)
	app.closed() // the default MenuTrigger is "input": focus does not open it
	app.Type("b")
	app.options("Banana", "Blueberry", "Elderberry")
	app.Expect(comboInput, telatest.Expanded(true), telatest.Focused(true))
	app.Type("l")
	app.options("Blueberry")
	// Contains, ignoring case.
	app.Press("Backspace")
	app.Press("Backspace")
	app.Type("RR")
	app.options("Blueberry", "Cherry", "Elderberry")
	app.Fill(comboInput, "zzz")
	app.closed() // nothing matches
	app.Fill(comboInput, "")
	app.options(fruits2()...) // an empty text matches everything, and opens the list
	if got := ev.take(); got != "input b input bl input b input  input R input RR input zzz input " {
		t.Errorf("events %q", got)
	}
}

func fruits2() []string {
	out := make([]string, len(fruits))
	for i, f := range fruits {
		out[i] = capital(f)
	}
	return out
}

func TestComboboxArrowsHighlightWhileTheInputKeepsTheFocus(t *testing.T) {
	var ev events
	app := mountCombo(t, aria.ComboboxOptions[string]{DisabledKeys: []string{"blueberry"}}, &ev)
	app.Focus(comboInput)
	app.Type("b")
	app.ExpectCount(highlighted, 0) // typing highlights nothing
	app.Press("ArrowDown")
	app.Expect(highlighted, telatest.HasName("Banana"))
	app.Press("ArrowDown") // blueberry is disabled
	app.Expect(highlighted, telatest.HasName("Elderberry"))
	app.Press("ArrowDown") // no wrap by default
	app.Expect(highlighted, telatest.HasName("Elderberry"))
	app.Press("ArrowUp")
	app.Expect(highlighted, telatest.HasName("Banana"))
	app.Expect(comboInput, telatest.Focused(true), telatest.Expanded(true))
	// Home and End stay with the text field: they do not move the highlight.
	app.Press("End")
	app.Expect(highlighted, telatest.HasName("Banana"))
	// The left and right arrows move the caret and drop the highlight.
	app.Press("ArrowLeft")
	app.ExpectCount(highlighted, 0)
	// Typing again drops it too.
	app.Press("ArrowDown")
	app.Expect(highlighted, telatest.HasName("Banana"))
	app.Type("a")
	app.ExpectCount(highlighted, 0)
	// Escape closes the list and reverts the text.
	app.Press("Escape")
	app.closed()
	app.ExpectValue(comboInput, "")
}

func TestComboboxEnterChoosesTheHighlightedItem(t *testing.T) {
	var ev events
	app := mountCombo(t, aria.ComboboxOptions[string]{}, &ev)
	app.Focus(comboInput)
	app.Type("blu")
	app.Press("ArrowDown")
	app.Press("Enter")
	app.closed()
	app.ExpectValue(comboInput, "Blueberry")
	app.Expect(comboInput, telatest.Focused(true), telatest.Expanded(false))
	if got := ev.take(); got != "input b input bl input blu selected blueberry input Blueberry" {
		t.Errorf("events %q", got)
	}
	// Opening it again with ArrowDown shows every item (the text is not a
	// filter then) and highlights the selected one.
	app.Press("ArrowDown")
	app.options(fruits2()...)
	app.Expect(highlighted, telatest.HasName("Blueberry"))
	app.Expect(telatest.ByRole("option", "Blueberry"), telatest.Selected(true))
	// Choosing the selected item again closes the list without a change.
	app.Press("Enter")
	app.closed()
	app.ExpectValue(comboInput, "Blueberry")
	if got := ev.take(); got != "" {
		t.Errorf("events %q", got)
	}
	// ArrowUp opens it on the last item... which the selection wins over.
	app.Press("Escape")
	app.Press("ArrowUp")
	app.Expect(highlighted, telatest.HasName("Blueberry"))
}

func TestComboboxArrowsOpenWithTheFirstOrLastItem(t *testing.T) {
	app := mountCombo(t, aria.ComboboxOptions[string]{}, nil)
	app.Focus(comboInput)
	app.Press("ArrowUp")
	app.Expect(highlighted, telatest.HasName("Fig"))
	app.Press("Escape")
	app.closed()
	app.Press("ArrowDown")
	app.Expect(highlighted, telatest.HasName("Apple"))
	app.Press("ArrowUp") // no wrap by default
	app.Expect(highlighted, telatest.HasName("Apple"))
	app.Press("End")
	app.Press("PageDown")
	app.Expect(highlighted, telatest.HasName("Fig"))
	app.Press("PageUp")
	app.Expect(highlighted, telatest.HasName("Apple"))
}

func TestComboboxClickAnOption(t *testing.T) {
	var ev events
	app := mountCombo(t, aria.ComboboxOptions[string]{}, &ev)
	app.Focus(comboInput)
	app.Type("e")
	app.Click(telatest.ByRole("option", "Date"))
	app.closed()
	app.ExpectValue(comboInput, "Date")
	app.Expect(comboInput, telatest.Focused(true)) // pressing an option keeps the focus
	if got := ev.take(); got != "input e selected date input Date" {
		t.Errorf("events %q", got)
	}
}

// The mouse highlights the item it moves over, and the input keeps the focus.
func TestComboboxHoverHighlights(t *testing.T) {
	app := mountCombo(t, aria.ComboboxOptions[string]{}, nil)
	app.Focus(comboInput)
	app.Type("e")
	app.Hover(telatest.ByRole("option", "Cherry"))
	app.Expect(highlighted, telatest.HasName("Cherry"))
	app.Expect(comboInput, telatest.Focused(true))
	app.Press("ArrowDown")
	app.Expect(highlighted, telatest.HasName("Date"))
}

func TestComboboxEscape(t *testing.T) {
	var ev events
	app := mountCombo(t, aria.ComboboxOptions[string]{}, &ev)
	app.Focus(comboInput)
	app.Type("ban")
	app.Press("Escape")
	app.closed()
	app.ExpectValue(comboInput, "") // no selection: the text is cleared
	// With a selection, Escape puts its text back.
	app.Type("ch")
	app.Press("ArrowDown")
	app.Press("Enter")
	app.ExpectValue(comboInput, "Cherry")
	app.Type("x")
	app.closed()
	app.ExpectValue(comboInput, "Cherryx")
	app.Press("Escape")
	app.ExpectValue(comboInput, "Cherry")
	// Escape on an open list closes it first and keeps the dialog
	// behind from seeing the key: checked in the dialog test.
	_ = ev.take()
}

func TestComboboxBlurCommits(t *testing.T) {
	var ev events
	app := mountCombo(t, aria.ComboboxOptions[string]{DefaultSelected: aria.SelectionOf("fig")}, &ev)
	app.ExpectValue(comboInput, "Fig") // the text of the selected item
	app.Focus(comboInput)
	app.Fill(comboInput, "xyz")
	app.Press("Tab")
	// Without custom values the text goes back to the selected item's.
	app.closed()
	app.ExpectValue(comboInput, "Fig")
	if got := ev.take(); got != "input xyz input Fig" {
		t.Errorf("events %q", got)
	}
	// A press elsewhere does the same.
	app.Focus(comboInput)
	app.Type("a")
	app.Click(telatest.ByRole("button", "Other"))
	app.closed()
	app.ExpectValue(comboInput, "Fig")
	// Tab chooses the highlighted item.
	app.Focus(comboInput)
	app.Fill(comboInput, "ch")
	app.Press("ArrowDown")
	app.Press("Tab")
	app.ExpectValue(comboInput, "Cherry")
	app.Expect(telatest.ByRole("button", "Other"), telatest.Focused(true))
	_ = ev.take()
}

func TestComboboxCustomValues(t *testing.T) {
	var ev events
	app := mountCombo(t, aria.ComboboxOptions[string]{AllowsCustomValue: true, DefaultSelected: aria.SelectionOf("fig")}, &ev)
	app.Focus(comboInput)
	app.Fill(comboInput, "kiwi")
	app.closed() // nothing matches
	app.Press("Tab")
	// The text stays and the selection goes.
	app.ExpectValue(comboInput, "kiwi")
	if got := ev.take(); got != "input kiwi selected none" {
		t.Errorf("events %q", got)
	}
	// Enter commits a custom value too.
	app.Focus(comboInput)
	app.Fill(comboInput, "ki")
	app.Press("Enter")
	app.ExpectValue(comboInput, "ki")
	// Text equal to an item's text keeps the selection.
	app.Fill(comboInput, "")
	app.Press("Escape")
	app.ExpectValue(comboInput, "") // custom with no selection: stays
	_ = ev.take()
}

func TestComboboxClearingTheTextClearsTheSelection(t *testing.T) {
	var ev events
	app := mountCombo(t, aria.ComboboxOptions[string]{DefaultSelected: aria.SelectionOf("fig")}, &ev)
	app.Focus(comboInput)
	app.Press("ControlOrMeta+a")
	app.Press("Backspace")
	if got := ev.take(); got != "input  selected none" {
		t.Errorf("events %q", got)
	}
}

func TestComboboxMenuTriggerModes(t *testing.T) {
	// "focus": focusing opens it with every item.
	app := mountCombo(t, aria.ComboboxOptions[string]{MenuTrigger: aria.MenuTriggerFocus, DefaultInputValue: "ban"}, nil)
	app.Focus(comboInput)
	app.options(fruits2()...)
	app.Press("Escape")
	app.closed()
	// Typing still opens it, filtered.
	app.Type("a")
	app.options("Apple", "Banana", "Date")

	// "manual": typing does not open it; the arrows and the button do.
	app = mountCombo(t, aria.ComboboxOptions[string]{MenuTrigger: aria.MenuTriggerManual}, nil)
	app.Focus(comboInput)
	app.Type("b")
	app.closed()
	app.Press("ArrowDown")
	app.options(fruits2()...) // all of them, whatever the text
	app.Expect(highlighted, telatest.HasName("Apple"))
	app.Type("l") // the text changed: the list filters again
	app.options("Blueberry")
	app.Press("Escape")
	app.Click(telatest.ByRole("button", "Show suggestions"))
	app.options(fruits2()...)
}

func TestComboboxButtonLabel(t *testing.T) {
	app := mountCombo(t, aria.ComboboxOptions[string]{ButtonLabel: "Mostrar sugerencias"}, nil)
	app.ExpectCount(telatest.ByRole("button", "Mostrar sugerencias"), 1)
	app.ExpectCount(telatest.ByRole("button", "Show suggestions"), 0)
}

func TestComboboxToggleButton(t *testing.T) {
	app := mountCombo(t, aria.ComboboxOptions[string]{DefaultInputValue: "app"}, nil)
	button := telatest.ByRole("button", "Show suggestions")
	app.Expect(button, telatest.Expanded(false))
	app.Focus(comboInput)
	app.Click(button)
	app.Expect(button, telatest.Expanded(true))
	app.options(fruits2()...)                      // every item, not just the matches
	app.Expect(comboInput, telatest.Focused(true)) // the button keeps the focus on the input
	app.Click(button)
	app.closed()
	app.Expect(comboInput, telatest.Focused(true))
	// The button opens it from outside the input too, and focuses the input.
	app.Press("Tab")
	app.Click(button)
	app.options(fruits2()...)
	app.Expect(comboInput, telatest.Focused(true))
	// It is not a tab stop.
	app.Press("Escape")
	app.Focus(comboInput)
	app.Press("Tab")
	app.Expect(telatest.ByRole("button", "Other"), telatest.Focused(true))
}

func TestComboboxFilters(t *testing.T) {
	// StartsWith.
	app := mountCombo(t, aria.ComboboxOptions[string]{Filter: aria.StartsWith}, nil)
	app.Focus(comboInput)
	app.Type("rr")
	app.closed()
	app.Fill(comboInput, "b")
	app.options("Banana", "Blueberry")
	// Case and diacritics are ignored.
	cities := []string{"Zürich", "Zagreb", "São Paulo", "Sao Tome", "Łódź"}
	app = mountCombo(t, aria.ComboboxOptions[string]{Items: cities, TextValue: func(s string) string { return s }}, nil)
	app.Focus(comboInput)
	app.Fill(comboInput, "zur")
	app.options("Zürich")
	app.Fill(comboInput, "SAO")
	app.options("São Paulo", "Sao Tome")
	app.Fill(comboInput, "ao p")
	app.options("São Paulo")
	app.Fill(comboInput, "ü") // u, in base sensitivity
	app.options("Zürich", "São Paulo")
	app.Fill(comboInput, "łó")
	app.options("Łódź")
}

// Keys: the caller filters.
func TestComboboxCallerFiltersKeys(t *testing.T) {
	var ev events
	page := func() tela.Node {
		text, setText := tela.UseState("")
		var keys []string
		for _, f := range fruits {
			if strings.HasPrefix(f, strings.ToLower(text)) {
				keys = append(keys, f)
			}
		}
		return comboNode(aria.ComboboxOptions[string]{
			Keys: keys, TextValue: capital, OnInputChange: func(v string) { ev.add("input " + v); setText.Set(v) },
		})
	}
	app := telatest.MountFunc(t, page, telatest.Size(400, 400))
	app.Focus(comboInput)
	app.Type("b")
	app.ExpectCount(telatest.ByRole("option", ""), 2)
	app.Type("l")
	app.ExpectCount(telatest.ByRole("option", ""), 1)
	app.Press("ArrowDown")
	app.Press("Enter")
	app.ExpectValue(comboInput, "Blueberry")
	if got := ev.take(); got != "input b input bl input Blueberry" {
		t.Errorf("events %q", got)
	}
}

func TestComboboxAllowsEmptyCollection(t *testing.T) {
	app := mountCombo(t, aria.ComboboxOptions[string]{AllowsEmptyCollection: true}, nil)
	app.Focus(comboInput)
	app.Type("zz")
	app.Expect(comboInput, telatest.Expanded(true))
	app.ExpectCount(telatest.ByRole("option", ""), 0)
}

func TestComboboxControlled(t *testing.T) {
	var ev events
	page := func() tela.Node {
		text, setText := tela.UseState("")
		sel, setSel := tela.UseState(aria.SelectionOf[string]())
		open, setOpen := tela.UseState(false)
		return comboNode(aria.ComboboxOptions[string]{
			Items: fruits, TextValue: capital,
			InputValue: &text, OnInputChange: func(v string) { ev.add("input " + v); setText.Set(v) },
			Selected: sel, OnSelectionChange: func(s aria.Selection[string]) {
				k, _ := s.First()
				ev.add("selected " + k)
				if k == "cherry" {
					return // refused
				}
				setSel.Set(s)
				if k != "" {
					setText.Set(capital(k))
				}
			},
			Open: &open, OnOpenChange: func(o bool) { ev.add(map[bool]string{true: "open", false: "close"}[o]); setOpen.Set(o) },
		})
	}
	app := &comboApp{telatest.MountFunc(t, page, telatest.Size(400, 400)), t}
	app.Focus(comboInput)
	app.Type("b")
	app.Press("ArrowDown")
	app.Press("Enter")
	app.ExpectValue(comboInput, "Banana")
	app.closed()
	app.Fill(comboInput, "che")
	app.Press("ArrowDown")
	app.Press("Enter") // the app refuses cherry
	app.Expect(telatest.ByRole("option", "Cherry"), telatest.Selected(false))
	if got := ev.take(); !strings.Contains(got, "selected banana") || !strings.Contains(got, "selected cherry") {
		t.Errorf("events %q", got)
	}
}

func TestComboboxAriaSnapshot(t *testing.T) {
	app := mountCombo(t, aria.ComboboxOptions[string]{DefaultSelected: aria.SelectionOf("fig"), DisabledKeys: []string{"banana"}}, nil)
	app.ExpectAria(`
		- text: Fruit
		- combobox "Fruit": Fig
		- button "Show suggestions": v
		- button "Other"
	`)
	app.Focus(comboInput)
	app.Fill(comboInput, "b")
	app.ExpectAria(`
		- text: Fruit
		- combobox "Fruit" [expanded]: b
		- button "Show suggestions" [expanded]: v
		- button "Other"
		- listbox "Fruit":
		  - option "Banana" [disabled]
		  - option "Blueberry"
		  - option "Elderberry"
	`)
}

func TestComboboxLabelAndAriaLabel(t *testing.T) {
	app := mountCombo(t, aria.ComboboxOptions[string]{}, nil)
	app.Click(telatest.ByText("Fruit"))
	app.Expect(comboInput, telatest.Focused(true))
	app2 := telatest.MountFunc(t, func() tela.Node {
		return tela.C(func() tela.Node {
			cb := aria.UseCombobox(aria.ComboboxOptions[string]{Items: fruits, AriaLabel: "Search"})
			return Input(cb.InputProps)
		})
	}, telatest.Size(300, 100))
	app2.ExpectAria(`- combobox "Search"`)
}

func TestComboboxDisabled(t *testing.T) {
	app := mountCombo(t, aria.ComboboxOptions[string]{Disabled: true}, nil)
	app.Expect(comboInput, telatest.Disabled(true))
	app.Expect(telatest.ByRole("button", "Show suggestions"), telatest.Disabled(true))
	// Read-only: the list does not open.
	app = mountCombo(t, aria.ComboboxOptions[string]{ReadOnly: true, DefaultInputValue: "x"}, nil)
	app.Focus(comboInput)
	app.Type("b")
	app.closed()
}

// The popover is as wide as the anchor.
func TestComboboxMatchWidth(t *testing.T) {
	app := mountCombo(t, aria.ComboboxOptions[string]{MatchWidth: true}, nil)
	app.Focus(comboInput)
	app.Type("b")
	in, list := app.Bounds(comboInput), app.Bounds(telatest.ByRole("listbox", "Fruit"))
	if in.Width != 192 || list.Width != 192 {
		t.Errorf("widths: input %v, list %v, want 192", in.Width, list.Width)
	}
	if list.Y < in.Bottom() {
		t.Errorf("list at %+v, input at %+v", list, in)
	}
}

func TestComboboxDevPanics(t *testing.T) {
	for _, c := range []struct {
		o    aria.ComboboxOptions[string]
		want string
	}{
		{aria.ComboboxOptions[string]{Keys: fruits, Items: fruits}, "give Keys (items filtered by you) or Items"},
		{aria.ComboboxOptions[string]{Keys: fruits, Filter: aria.StartsWith}, "Filter: it filters Items"},
		{aria.ComboboxOptions[string]{Items: fruits, MenuTrigger: "inpt"}, `did you mean "input"?`},
	} {
		msg := recoverMount(t, comboPage(c.o, nil))
		if !strings.Contains(msg, c.want) {
			t.Errorf("panic %q, want %q", msg, c.want)
		}
	}
}

// IME: while text is composed the keys do not navigate the list, and the
// composition is not disturbed by the filter.
func TestComboboxComposition(t *testing.T) {
	root := tela.C(comboPage(aria.ComboboxOptions[string]{Items: []string{"かんじ", "かな", "ことば", "apple"}, TextValue: func(s string) string { return s }}, nil))
	a := mountRaw(t, root, 400, 300)
	a.tap(platform.PointerMouse, 50, 40)
	ime := func(e platform.IMEEvent) {
		a.win.Inject(e)
		a.settle()
	}
	ime(platform.IMEEvent{Preedit: "か", CursorStart: 3, CursorEnd: 3})
	snap := a.tree.AriaSnapshot()
	if !strings.Contains(snap, `combobox "Fruit" [expanded]: か`) || !strings.Contains(snap, `option "かんじ"`) {
		t.Fatalf("while composing:\n%s", snap)
	}
	// Keys the IME did not consume still arrive here: a composing field
	// ignores them, and so does the list.
	pressKey(a, platform.KeyArrowDown, platform.CodeArrowDown)
	pressKey(a, platform.KeyEnter, platform.CodeEnter)
	pressKey(a, platform.KeyEscape, platform.CodeEscape)
	snap = a.tree.AriaSnapshot()
	if !strings.Contains(snap, `combobox "Fruit" [expanded]: か`) || !strings.Contains(snap, `option "かな"`) {
		t.Fatalf("after keys during the composition:\n%s", snap)
	}
	ime(platform.IMEEvent{Preedit: "かん", CursorStart: 6, CursorEnd: 6})
	snap = a.tree.AriaSnapshot()
	if !strings.Contains(snap, `: かん`) || strings.Contains(snap, `option "かな"`) || !strings.Contains(snap, `option "かんじ"`) {
		t.Fatalf("the list follows the preedit:\n%s", snap)
	}
	ime(platform.IMEEvent{Commit: "かんじ"})
	snap = a.tree.AriaSnapshot()
	if !strings.Contains(snap, `combobox "Fruit" [expanded]: かんじ`) {
		t.Fatalf("after the commit:\n%s", snap)
	}
	// Once committed, the arrows work again.
	pressKey(a, platform.KeyArrowDown, platform.CodeArrowDown)
	pressKey(a, platform.KeyEnter, platform.CodeEnter)
	snap = a.tree.AriaSnapshot()
	if !strings.Contains(snap, `combobox "Fruit": かんじ`) || strings.Contains(snap, "listbox") {
		t.Fatalf("after Enter:\n%s", snap)
	}
}

// aria-activedescendant follows the highlighted item, and the input keeps
// the focus.
func TestComboboxActiveDescendant(t *testing.T) {
	root := tela.C(comboPage(aria.ComboboxOptions[string]{}, nil))
	a := mountRaw(t, root, 400, 300)
	a.tap(platform.PointerMouse, 50, 40)
	pressKey(a, platform.KeyArrowDown, platform.CodeArrowDown)
	input := findNode(a.tree.Semantics(), "combobox", "Fruit")
	if input == nil || input.Controls == 0 || input.ActiveDescendant == 0 {
		t.Fatalf("combobox %+v", input)
	}
	opt := findNode(a.tree.Semantics(), "option", "Apple")
	if opt == nil || input.ActiveDescendant != opt.ID {
		t.Errorf("active descendant %v, want Apple %v", input.ActiveDescendant, opt)
	}
	list := findNode(a.tree.Semantics(), "listbox", "Fruit")
	if list == nil || input.Controls != list.ID {
		t.Errorf("controls %v, want the listbox %v", input.Controls, list)
	}
	pressKey(a, platform.KeyArrowDown, platform.CodeArrowDown)
	opt = findNode(a.tree.Semantics(), "option", "Banana")
	if input = findNode(a.tree.Semantics(), "combobox", "Fruit"); input.ActiveDescendant != opt.ID {
		t.Errorf("active descendant %v, want Banana %v", input.ActiveDescendant, opt.ID)
	}
	pressKey(a, platform.KeyEscape, platform.CodeEscape)
	if input = findNode(a.tree.Semantics(), "combobox", "Fruit"); input.ActiveDescendant != 0 || input.Controls != 0 {
		t.Errorf("closed: %+v", input)
	}
}

// Escape reaches an enclosing dialog only when there was nothing to
// revert, and a closed combobox's Escape closes the dialog.
func TestComboboxEscapeAndDialog(t *testing.T) {
	var closed int
	page := func() tela.Node {
		return tela.C(func() tela.Node {
			ov := aria.UseOverlay(aria.OverlayOptions{Open: true, OnClose: func() { closed++ }, Modal: true})
			return Div(ov.Props, Class("h-full"), comboNode(aria.ComboboxOptions[string]{Items: fruits, TextValue: capital}))
		})
	}
	app := telatest.MountFunc(t, page, telatest.Size(400, 300))
	app.Focus(comboInput)
	app.Press("Escape") // nothing typed: the dialog hears it
	if closed != 1 {
		t.Errorf("dialog closed %d times, want 1", closed)
	}
	app.Type("ban")
	app.Press("Escape") // closes the list and reverts the text
	if closed != 1 {
		t.Errorf("dialog closed %d times after an Escape that reverted, want 1", closed)
	}
	app.ExpectValue(comboInput, "")
	app.Press("Escape")
	if closed != 2 {
		t.Errorf("dialog closed %d times, want 2", closed)
	}
}

// An open combobox that nothing touches builds nothing (the caret blinks,
// which repaints only): its sync effect does not loop.
func TestComboboxIdleBuildsNothing(t *testing.T) {
	app := mountCombo(t, aria.ComboboxOptions[string]{}, nil)
	app.Focus(comboInput)
	app.Type("b")
	st := app.Measure(func() { app.Advance(2 * time.Second) })
	if st.Builds != 0 {
		t.Errorf("an idle open combobox built %d components in %d frames", st.Builds, st.Frames)
	}
}

// pressKey presses and releases a key.
func pressKey(a *rawApp, key platform.Key, code platform.KeyCode) {
	a.t.Helper()
	a.key(platform.KeyPress, key, code, false)
	a.key(platform.KeyRelease, key, code, false)
}
