package h

import (
	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/internal/core"
)

// The semantics props: React's role and aria-* attributes, and Img's alt
// text. They describe what an element is to assistive technologies, to
// telatest's ByRole queries and to agents (`tela inspect --aria` prints
// the tree as a Playwright aria snapshot); nothing changes on screen.
// Elements have implicit roles: Button is a button, H1 to H6 headings, P a
// paragraph, Img an image, A with Href a link; Div and Span have none.
//
// The props that hold a state also drive the class variants of the same
// name: AriaExpanded(true) matches aria-expanded:, AriaChecked(true)
// matches aria-checked:, and so on for AriaBusy, AriaDisabled, AriaHidden,
// AriaInvalid, AriaPressed, AriaReadOnly, AriaRequired and AriaSelected
// (with the group-aria-* and peer-aria-* forms), as Tailwind's
// [aria-x="true"]; AriaSort("ascending") and AriaSort("descending") match
// aria-[sort=ascending]: and aria-[sort=descending]:, and AriaCurrent("page")
// (or step, location, date, time, true) matches aria-[current=page]: and
// its siblings.

// Role sets the ARIA role of an element (WAI-ARIA 1.2), for components
// built from divs: Role("switch"), Role("dialog"), Role("tab"). An unknown
// role panics in dev mode (on in development builds, see tela.App.Dev)
// with the closest valid one.
//
//	Button(Role("switch"), AriaChecked(on), AriaLabel("Wi-Fi"),
//	    OnClick(func() { setOn.Set(!on) }), knob)
func Role(role string) tela.Prop { return core.Role(role) }

// AriaLabel sets the accessible name of an element, for elements whose
// content does not name them (an icon button).
//
//	Button(AriaLabel("Close"), OnClick(close), closeIcon)
func AriaLabel(label string) tela.Prop { return core.AriaLabel(label) }

// AriaLabelledBy names an element with the text of other elements, attached
// to refs with Ref. With several references the name is the text of each one
// that is mounted, in the order given, joined by a space
// (aria-labelledby="a b"), as a select is named by its value and its label.
//
//	title := tela.UseElementRef()
//	Div(Role("dialog"), AriaLabelledBy(title),
//	    H2(Ref(title), Text("Delete file?")), ...)
//
//	Button(AriaLabelledBy(value, label), ...)
//
// Calling it with no reference, or with the zero ElementRef, panics.
func AriaLabelledBy(refs ...tela.ElementRef) tela.Prop {
	if len(refs) == 0 {
		panic("tela: AriaLabelledBy() with no references: pass one or more ElementRefs from tela.UseElementRef, attached with Ref to the labelling elements")
	}
	return core.AriaLabelledBy(refs[0], refs[1:]...)
}

// AriaDescription sets the accessible description of an element: a
// longer hint read after its name.
//
//	Button(AriaDescription("Removes the file for everyone"), Text("Delete"))
func AriaDescription(description string) tela.Prop { return core.AriaDescription(description) }

// AriaDescribedBy describes an element with the text of another one,
// attached to a ref with Ref (HTML's aria-describedby): a tooltip
// describes its trigger. AriaDescription wins over it.
//
//	Div(Role("tooltip"), Ref(tip), Text("Saves the file")),
//	Button(AriaDescribedBy(tip), Text("Save"))
func AriaDescribedBy(ref tela.ElementRef) tela.Prop { return core.AriaDescribedBy(ref) }

// AriaHidden hides an element and its subtree from the semantics tree
// when true (decorations, duplicated content), without hiding it on
// screen.
//
//	Span(AriaHidden(true), Text("★"))
func AriaHidden(hidden bool) tela.Prop { return core.AriaHidden(hidden) }

// AriaChecked sets the checked state of a checkbox, radio or switch.
//
//	Div(Role("checkbox"), AriaChecked(done), TabIndex(0), Text("Done"))
func AriaChecked(checked bool) tela.Prop { return core.AriaChecked(checked) }

// AriaPressed makes a button a toggle button and sets whether it is
// pressed.
//
//	Button(AriaPressed(bold), OnClick(toggleBold), Text("B"))
func AriaPressed(pressed bool) tela.Prop { return core.AriaPressed(pressed) }

// AriaSelected sets whether a tab, option, row or cell is selected.
//
//	Button(Role("tab"), AriaSelected(active == "general"), Text("General"))
func AriaSelected(selected bool) tela.Prop { return core.AriaSelected(selected) }

// AriaExpanded sets whether what a button or link controls (a menu, a
// disclosure) is expanded.
//
//	Button(AriaExpanded(open), OnClick(toggle), Text("Options"))
func AriaExpanded(expanded bool) tela.Prop { return core.AriaExpanded(expanded) }

// AriaDisabled marks an element and its descendants as disabled for
// assistive technologies without making them inert (Disabled does both).
//
//	Button(AriaDisabled(true), OnClick(explainWhy), Text("Publish"))
func AriaDisabled(disabled bool) tela.Prop { return core.AriaDisabled(disabled) }

// AriaLevel sets the level of an element with Role("heading") (H1 to H6
// have theirs), or of a list item, row or tree item.
//
//	Div(Role("heading"), AriaLevel(2), Text("Billing"))
func AriaLevel(level int) tela.Prop { return core.AriaLevel(level) }

// AriaRowCount sets the total number of rows of a Role("table"), "grid" or
// "treegrid" that has only some of them in the tree, such as a virtualized
// one: the rows that exist, header and footer included, or -1 when the total
// is unknown. Pair it with AriaRowIndex on the rows that are built.
//
//	Div(Role("table"), AriaRowCount(10_001), header, VirtualList(...))
func AriaRowCount(n int) tela.Prop { return core.AriaRowCount(n) }

// AriaRowIndex sets the position, from 1, of a Role("row") among the rows of
// its table, for a table that has only some of its rows in the tree.
//
//	Div(Role("row"), AriaRowIndex(i+2), cells) // the header is row 1
func AriaRowIndex(i int) tela.Prop { return core.AriaRowIndex(i) }

// AriaValueNow sets the current value of a range widget (Role("slider"),
// "progressbar", "spinbutton", "meter").
//
//	Div(Role("progressbar"), AriaValueNow(40), AriaValueMin(0), AriaValueMax(100), bar)
func AriaValueNow(value float64) tela.Prop { return core.AriaValueNow(value) }

// AriaValueMin sets the minimum value of a range widget.
func AriaValueMin(value float64) tela.Prop { return core.AriaValueMin(value) }

// AriaValueMax sets the maximum value of a range widget.
func AriaValueMax(value float64) tela.Prop { return core.AriaValueMax(value) }

// AriaValueText sets the value of a range widget as people read it.
//
//	Div(Role("slider"), AriaValueNow(3), AriaValueText("Medium"), track)
func AriaValueText(text string) tela.Prop { return core.AriaValueText(text) }

// Alt sets the alternative text of an Img: its accessible name. Use
// Alt("") for a decorative image, which the semantics tree leaves out. It
// panics on other elements.
//
//	Img(Src(avatar), Alt("Ada Lovelace"), Class("size-10 rounded-full"))
func Alt(text string) tela.Prop { return core.Alt(text) }

// TestID names an element for tests, like the data-testid attribute: find
// it with telatest.ByTestID, which resolves the name to exactly the element
// that carries it, whatever its role, text or classes. It changes nothing
// on screen and is not exposed anywhere else: not in the semantics tree,
// the aria snapshot or the platform's accessibility API, so it never leaks
// into what assistive technologies or `tela inspect` show.
//
//	Div(TestID("sidebar"), Class("w-64"), ...)
//	// in the test:
//	app.Click(telatest.ByTestID("sidebar"))
//
// Use it for elements a user cannot name (a card, a chart, a drop zone);
// prefer telatest.ByRole and telatest.ByText when the element has a role
// or text, since those tests also check what users see.
func TestID(id string) tela.Prop { return core.TestID(id) }

// AriaRequired marks a field the user must fill in (aria-required): a
// text field, combobox, listbox or radio group. It changes nothing on
// screen; the aria-required: variants style it.
//
//	Input(ID("email"), AriaRequired(true), Class("border aria-required:border-indigo-400"))
func AriaRequired(required bool) tela.Prop { return core.AriaRequired(required) }

// AriaReadOnly marks a widget the user can focus but not edit
// (aria-readonly): a text field, checkbox, slider or listbox built from
// divs. An Input or Textarea with ReadOnly is already read-only; the
// aria-readonly: variants style the elements with AriaReadOnly(true).
//
//	Div(Role("textbox"), AriaReadOnly(locked), Class("aria-readonly:bg-slate-100"))
func AriaReadOnly(readOnly bool) tela.Prop { return core.AriaReadOnly(readOnly) }

// AriaBusy marks an element, and what it contains, as being updated
// (aria-busy): a list that loads. The aria-busy: variants style it.
//
//	Div(AriaBusy(loading), Class("aria-busy:opacity-50 aria-busy:animate-pulse"), rows)
func AriaBusy(busy bool) tela.Prop { return core.AriaBusy(busy) }

// AriaInvalid marks a field whose value failed the app's validation
// (aria-invalid): a text field, checkbox, combobox or listbox. Assistive
// technologies announce it, the aria snapshot prints it as [invalid] and
// the aria-invalid: variants style it. An Input whose own value is
// invalid (an email Type with a malformed address) is already invalid for
// assistive technologies; AriaInvalid, true or false, overrides that.
//
//	Input(Bind(email, setEmail), AriaInvalid(err != ""),
//	    Class("border aria-invalid:border-destructive aria-invalid:ring-destructive/20"))
func AriaInvalid(invalid bool) tela.Prop { return core.AriaInvalid(invalid) }

// AriaSort tells how a table is sorted by a column header (aria-sort):
// "ascending", "descending", "other" or "none" (the default). Unknown
// values panic in dev mode with the closest valid one. The
// aria-[sort=ascending]:, aria-[sort=descending]:, aria-[sort=other]: and
// aria-[sort=none]: variants style the header (the last for a header that
// says "none"), and their group- forms what it contains, such as its arrow.
//
//	Div(Role("columnheader"), AriaSort(order), OnClick(toggleOrder), Class("group flex gap-1"),
//	    Text("Name"), Icon(arrowUp, Class("size-4 group-aria-[sort=descending]:rotate-180")))
func AriaSort(sort string) tela.Prop { return core.AriaSort(sort) }

// AriaCurrent marks the current item of a set (aria-current): "page" for
// the link of the page being shown, "step", "location", "date" or "time"
// for the current item of a process, a trail, a calendar or a timeline, and
// "true" for any other. "false" and "" mean none (the default). Unknown
// tokens panic in dev mode with the closest valid one. The
// aria-[current=page]: variant (and step, location, date, time, true, with
// their group- and peer- forms) styles it; Tailwind has no bare
// aria-current:. The semantics node carries it for assistive technologies;
// the aria snapshot does not print it, as Playwright's does not.
//
//	A(Href("/docs"), AriaCurrent(current), // "page" on the active link, "" elsewhere
//	    Class("text-muted-foreground aria-[current=page]:text-foreground aria-[current=page]:font-medium"),
//	    Text("Docs"))
func AriaCurrent(current string) tela.Prop { return core.AriaCurrent(current) }

// AriaLabelFn sets the accessible name of an element, like AriaLabel, from
// a function called when the semantics tree is built, not when the
// component builds: a name that follows a tela.Value (a progress, a
// clock) without rebuilding the component. A change of a Value the
// function reads with Get rebuilds and repaints nothing; it only makes
// the next semantics tree (assistive technologies, telatest, the aria
// snapshot) call the function again.
//
//	done := tela.UseValue(0)
//	Div(Role("progressbar"), AriaLabelFn(func() string {
//	    return fmt.Sprintf("Upload %d%%", done.Get())
//	}), bar)
func AriaLabelFn(fn func() string) tela.Prop { return core.AriaLabelFn(fn) }

// AriaLive makes an element a live region: when its content changes,
// assistive technologies announce it without the focus moving there.
// "polite" waits until the user is idle, "assertive" interrupts, "off"
// (the default) announces nothing. Role("alert") is assertive and
// Role("status") and Role("log") polite without it. An unknown value
// panics in dev mode with the closest valid one.
//
//	Div(AriaLive("polite"), Textf("%d results", n))
func AriaLive(politeness string) tela.Prop { return core.AriaLive(politeness) }

// AriaAtomic sets whether a live region is read as a whole when part of it
// changes (true), or only the part that changed (false, the default but
// for Role("alert") and Role("status")).
//
//	Div(AriaLive("polite"), AriaAtomic(true), Text("Track "), Textf("%d of %d", i, n))
func AriaAtomic(atomic bool) tela.Prop { return core.AriaAtomic(atomic) }

// AriaKeyShortcuts names the keys that activate or focus an element, for
// assistive technologies to announce: space-separated alternatives, each
// keys joined by "+" with the modifiers first, in UI Events key names
// ("Control", "Meta", "Alt", "Shift"). It does not handle the keys. Common
// aliases ("Ctrl", "Cmd") panic in dev mode with the right name.
//
//	Button(AriaKeyShortcuts("Control+S Meta+S"), OnClick(save), Text("Save"))
func AriaKeyShortcuts(keys string) tela.Prop { return core.AriaKeyShortcuts(keys) }

// AriaPosInSet sets the position, from 1, of an item (an option, a menu
// item, a list item, a tab, a tree item) in its set, for a set that has
// only some of its items in the tree, such as a virtualized list. Without
// it the position is counted from the items that are in the tree.
//
//	Div(Role("option"), AriaPosInSet(i+1), AriaSetSize(total), Text(name))
func AriaPosInSet(pos int) tela.Prop { return core.AriaPosInSet(pos) }

// AriaSetSize sets the number of items in the set of an item, on the item
// or on its container (Role("listbox"), "menu", "list", "tree"...), for a
// set that has only some of its items in the tree; -1 says the number is
// unknown. Without it the size is counted from the items in the tree.
//
//	Div(Role("listbox"), AriaSetSize(10_000), VirtualList(...))
func AriaSetSize(n int) tela.Prop { return core.AriaSetSize(n) }
