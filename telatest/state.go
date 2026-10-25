package telatest

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/kuronosu/tela/internal/semantics"
)

// State is one condition on an element that [App.Expect] checks: a
// toggle such as [Checked], a flag such as [Disabled], its [HasName] and
// [HasValue], or a widget property the aria snapshot does not print:
// [HasPopup], [ActiveDescendant] and [Controls] (which compare the element
// an ID reference points to with the one a Locator finds). They read the
// element's semantics node, so they test what assistive technologies are
// told, without matching strings of the aria snapshot.
type State struct {
	// want is the expectation as the failure message words it ("checked=true").
	want string
	// check reports whether n satisfies it and, when it does not, what n
	// has instead.
	check func(a *App, n *semantics.Node) (ok bool, got string)
}

// toggleState builds the State of an ARIA toggle (checked, pressed,
// selected, expanded). An element without the state, or with the mixed
// one, never satisfies it: a test of an unset state is a mistake worth
// reporting.
func toggleState(name, prop string, want bool, get func(n *semantics.Node) semantics.Toggle) State {
	return State{
		want: name + "=" + strconv.FormatBool(want),
		check: func(_ *App, n *semantics.Node) (bool, string) {
			switch got := get(n); got {
			case semantics.ToggleUnset:
				return false, name + " is not set on it (h." + prop + " sets it)"
			case semantics.Of(want):
				return true, ""
			default:
				return false, name + "=" + got.String()
			}
		},
	}
}

// Checked expects the element to have aria-checked equal to checked
// (a checkbox, radio, switch or option; h.AriaChecked sets it). An element
// without the state, or with the mixed one, does not satisfy it.
func Checked(checked bool) State {
	return toggleState("checked", "AriaChecked", checked, func(n *semantics.Node) semantics.Toggle { return n.Checked })
}

// Pressed expects the element to have aria-pressed equal to pressed (a
// toggle button; h.AriaPressed sets it).
func Pressed(pressed bool) State {
	return toggleState("pressed", "AriaPressed", pressed, func(n *semantics.Node) semantics.Toggle { return n.Pressed })
}

// Selected expects the element to have aria-selected equal to selected (a
// tab, option, row or cell; h.AriaSelected sets it).
func Selected(selected bool) State {
	return toggleState("selected", "AriaSelected", selected, func(n *semantics.Node) semantics.Toggle { return n.Selected })
}

// Expanded expects the element to have aria-expanded equal to expanded
// (what a button, link or combobox controls; h.AriaExpanded sets it).
func Expanded(expanded bool) State {
	return toggleState("expanded", "AriaExpanded", expanded, func(n *semantics.Node) semantics.Toggle { return n.Expanded })
}

// flagState builds the State of a boolean flag of the node.
func flagState(name string, flag semantics.State, want bool) State {
	return State{
		want: name + "=" + strconv.FormatBool(want),
		check: func(_ *App, n *semantics.Node) (bool, string) {
			if n.Has(flag) == want {
				return true, ""
			}
			return false, name + "=" + strconv.FormatBool(!want)
		},
	}
}

// Disabled expects the element to be disabled (true) or enabled (false):
// disabled by h.Disabled on it or an ancestor, or by h.AriaDisabled.
func Disabled(disabled bool) State { return flagState("disabled", semantics.StateDisabled, disabled) }

// Focused expects the element to have the keyboard focus (true) or not
// (false).
func Focused(focused bool) State { return flagState("focused", semantics.StateFocused, focused) }

// Invalid expects the element's value to be invalid (true) or valid
// (false) for assistive technologies: h.AriaInvalid when it is set, else a
// text field's own validity (an email or url Input whose value does not
// parse), as Chromium tells them.
//
//	app.Fill(telatest.ByRole("textbox", "Email"), "ada@")
//	app.Expect(telatest.ByRole("textbox", "Email"), telatest.Invalid(true))
func Invalid(invalid bool) State {
	return State{
		want: "invalid=" + strconv.FormatBool(invalid),
		check: func(_ *App, n *semantics.Node) (bool, string) {
			got := n.Has(semantics.StateInvalid)
			if got == invalid {
				return true, ""
			}
			s := "invalid=" + strconv.FormatBool(got)
			switch {
			case n.Invalid != semantics.ToggleUnset:
				s += " (from h.AriaInvalid(" + strconv.FormatBool(got) + "))"
			case !got:
				s += " (h.AriaInvalid sets it)"
			default:
				s += " (the field's own value is invalid; h.AriaInvalid(false) overrides it)"
			}
			return false, s
		},
	}
}

// sortValues are the values Sort accepts.
var sortValues = []string{"ascending", "descending", "other", "none"}

// Sort expects the element's aria-sort to be sort: "ascending",
// "descending", "other" or "none", which is also what a header without
// h.AriaSort has (the ARIA default). The aria snapshot does not print it
// (Playwright's does not either).
//
//	app.Click(telatest.ByRole("columnheader", "Name"))
//	app.Expect(telatest.ByRole("columnheader", "Name"), telatest.Sort("descending"))
func Sort(sort string) State {
	if !slices.Contains(sortValues, sort) {
		panic(fmt.Sprintf("telatest: Sort(%q): not an aria-sort value; use \"ascending\", \"descending\", \"other\" or \"none\"", sort))
	}
	want := sort
	if want == "none" {
		want = ""
	}
	return State{
		want: "sort=" + strconv.Quote(sort),
		check: func(_ *App, n *semantics.Node) (bool, string) {
			if n.Sort == want {
				return true, ""
			}
			got := n.Sort
			if got == "" {
				return false, `sort="none" (h.AriaSort sets it)`
			}
			return false, "sort=" + strconv.Quote(got)
		},
	}
}

// HasName expects the element's accessible name to be exactly name (white
// space normalized, case sensitive): what [ByRole] matches by.
func HasName(name string) State {
	want := normalize(name)
	return State{
		want: "name=" + strconv.Quote(want),
		check: func(_ *App, n *semantics.Node) (bool, string) {
			return n.Name == want, "name=" + strconv.Quote(n.Name)
		},
	}
}

// HasValue expects the element's value to be exactly value: the text of a
// text field, or the value text of a range widget (the h.AriaValueText or
// h.AriaValueNow of a slider or progress bar, as in the aria snapshot).
func HasValue(value string) State {
	return State{
		want: "value=" + strconv.Quote(value),
		check: func(_ *App, n *semantics.Node) (bool, string) {
			return n.Value == value, "value=" + strconv.Quote(n.Value)
		},
	}
}

// popupKinds are the values HasPopup accepts, after normalizing.
var popupKinds = []string{"", "menu", "listbox", "tree", "grid", "dialog"}

// HasPopup expects the element's aria-haspopup to be kind: "menu",
// "listbox", "tree", "grid" or "dialog" (h.AriaHasPopup sets it), or ""
// for an element that opens no popup. As in ARIA, "true" is "menu" and
// "false" is "". The aria snapshot does not print it (Playwright's does
// not either).
//
//	app.Expect(telatest.ByRole("button", "Show suggestions"), telatest.HasPopup("listbox"))
func HasPopup(kind string) State {
	switch kind {
	case "true":
		kind = "menu"
	case "false":
		kind = ""
	}
	if !slices.Contains(popupKinds, kind) {
		panic(fmt.Sprintf("telatest: HasPopup(%q): not an aria-haspopup value; use \"menu\", \"listbox\", \"tree\", \"grid\", \"dialog\" or \"\" for none", kind))
	}
	return State{
		want: "haspopup=" + strconv.Quote(kind),
		check: func(_ *App, n *semantics.Node) (bool, string) {
			return n.HasPopup == kind, "haspopup=" + strconv.Quote(n.HasPopup)
		},
	}
}

// ActiveDescendant expects the element's aria-activedescendant to point
// to the element that loc locates (one element, strictness): the option a
// combobox or a list box keeps its virtual focus on while the keyboard
// focus stays on the input (h.AriaActiveDescendant sets it). Use
// [NoActiveDescendant] for none. The aria snapshot does not print it.
//
//	app.Expect(telatest.ByRole("combobox", "Project"),
//	    telatest.ActiveDescendant(telatest.ByRole("option", "users-db")))
func ActiveDescendant(loc Locator) State {
	return refState("activedescendant", loc, func(n *semantics.Node) semantics.NodeID { return n.ActiveDescendant })
}

// NoActiveDescendant expects the element to have no active descendant
// (no aria-activedescendant, or one whose element is not in the tree).
func NoActiveDescendant() State {
	return noRefState("activedescendant", func(n *semantics.Node) semantics.NodeID { return n.ActiveDescendant })
}

// Controls expects the element's aria-controls to point to the element
// that loc locates (one element, strictness): the list box of a combobox
// or the panel of a tab (h.AriaControls sets it). The aria snapshot does
// not print it.
//
//	app.Expect(telatest.ByRole("combobox", "Project"), telatest.Controls(telatest.ByRole("listbox", "Project")))
func Controls(loc Locator) State {
	return refState("controls", loc, func(n *semantics.Node) semantics.NodeID { return n.Controls })
}

// refState builds the State of an ID reference (aria-activedescendant,
// aria-controls) that must point to the one element loc locates.
func refState(name string, loc Locator, get func(*semantics.Node) semantics.NodeID) State {
	return State{
		want: name + "=" + loc.String(),
		check: func(a *App, n *semantics.Node) (bool, string) {
			a.tb.Helper()
			if p := loc.problem(); p != "" {
				if loc.kind == locNone && name == "activedescendant" {
					p += "; to expect no active descendant use telatest.NoActiveDescendant()"
				}
				a.fatalf("%s(%s): %s", stateFunc(name), loc, p)
			}
			nodes := a.find(loc)
			if len(nodes) > 1 {
				a.fatalf("%s(%s): %s", stateFunc(name), loc, a.strictMessage(loc, nodes))
			}
			id := get(n)
			if len(nodes) == 1 && id != 0 && nodes[0].ID == id {
				return true, ""
			}
			got := name + "=" + a.describeRef(id)
			if len(nodes) == 0 {
				got += " (and " + loc.String() + " matches no element)"
			}
			return false, got
		},
	}
}

// noRefState builds the State of an ID reference that must point to
// nothing in the tree.
func noRefState(name string, get func(*semantics.Node) semantics.NodeID) State {
	return State{
		want: "no " + name,
		check: func(a *App, n *semantics.Node) (bool, string) {
			id := get(n)
			if id == 0 || a.nodeByID(id) == nil {
				return true, ""
			}
			return false, name + "=" + a.describeRef(id)
		},
	}
}

// stateFunc names the State constructor of an ID reference, for messages.
func stateFunc(name string) string {
	if name == "controls" {
		return "Controls"
	}
	return "ActiveDescendant"
}

// nodeByID returns the node with id in the current semantics tree, or nil.
func (a *App) nodeByID(id semantics.NodeID) *semantics.Node {
	var found *semantics.Node
	a.tree.Semantics().Walk(func(n *semantics.Node) bool {
		if n.ID == id {
			found = n
		}
		return found == nil
	})
	return found
}

// describeRef describes the element an ID reference points to.
func (a *App) describeRef(id semantics.NodeID) string {
	if id == 0 {
		return "none"
	}
	n := a.nodeByID(id)
	switch {
	case n == nil:
		return "an element that is not in the tree"
	case n.Has(semantics.StateHidden):
		return a.describeMatch(n) + " (hidden)"
	}
	return a.describeMatch(n)
}

// Expect waits until the element l locates satisfies every state, like
// Playwright's toBeChecked, toBeDisabled and toBeExpanded: it settles the
// app and checks, and while a state does not hold lets a frame of app time
// pass and checks again, until it holds or the [Timeout] expires. The
// locator must match exactly one element (strictness). The failure
// message says which states differ, shows the element's actual states and
// its line of the aria snapshot.
//
//	app.Expect(telatest.ByRole("switch", "Wi-Fi"), telatest.Checked(true), telatest.Disabled(false))
//	app.Expect(telatest.ByRole("button", "Menu"), telatest.Expanded(true))
//	app.Expect(telatest.ByRole("textbox", "Email"), telatest.HasValue("ada@example.com"))
func (a *App) Expect(l Locator, states ...State) {
	a.tb.Helper()
	a.mustBeOpen()
	if p := l.problem(); p != "" {
		a.fatalf("%s", p)
	}
	if len(states) == 0 {
		a.fatalf("Expect(%s) without states checks nothing; pass states such as telatest.Checked(true), Disabled(false) or HasValue(\"...\")", l)
	}
	a.waitFor(func() (bool, string) {
		a.tb.Helper()
		nodes := a.find(l)
		switch len(nodes) {
		case 0:
			return false, a.missMessage(l)
		case 1:
		default:
			a.fatalf("%s", a.strictMessage(l, nodes))
		}
		n := nodes[0]
		var wants, gots []string
		for _, s := range states {
			if ok, got := s.check(a, n); !ok {
				wants = append(wants, s.want)
				gots = append(gots, got)
			}
		}
		if len(wants) == 0 {
			return true, ""
		}
		var b strings.Builder
		fmt.Fprintf(&b, "expected %s to have %s, but it has %s", a.describeMatch(n), strings.Join(wants, ", "), strings.Join(gots, ", "))
		fmt.Fprintf(&b, "\nits states: %s", describeStates(n))
		if line := ariaLine(n); line != "" {
			fmt.Fprintf(&b, "\nits aria line: %s", line)
		}
		return false, b.String()
	})
}

// describeStates lists the states of n for a failure message.
func describeStates(n *semantics.Node) string {
	var s []string
	for _, t := range []struct {
		name string
		v    semantics.Toggle
	}{{"checked", n.Checked}, {"pressed", n.Pressed}, {"selected", n.Selected}, {"expanded", n.Expanded}} {
		if t.v != semantics.ToggleUnset {
			s = append(s, t.name+"="+t.v.String())
		}
	}
	s = append(s, "disabled="+strconv.FormatBool(n.Has(semantics.StateDisabled)),
		"focused="+strconv.FormatBool(n.Has(semantics.StateFocused)))
	if n.Has(semantics.StateReadOnly) {
		s = append(s, "readonly=true")
	}
	if n.Has(semantics.StateRequired) {
		s = append(s, "required=true")
	}
	if n.Has(semantics.StateBusy) {
		s = append(s, "busy=true")
	}
	if n.Has(semantics.StateInvalid) {
		s = append(s, "invalid=true")
	}
	if n.Sort != "" {
		s = append(s, "sort="+strconv.Quote(n.Sort))
	}
	if n.Name != "" {
		s = append(s, "name="+strconv.Quote(n.Name))
	}
	if n.Value != "" {
		s = append(s, "value="+strconv.Quote(n.Value))
	}
	if n.HasPopup != "" {
		s = append(s, "haspopup="+strconv.Quote(n.HasPopup))
	}
	return strings.Join(s, ", ")
}

// ariaLine returns the line of the aria snapshot that n makes (its role,
// name and states), or "" when it has none of its own.
func ariaLine(n *semantics.Node) string {
	if n.Flattened() {
		return ""
	}
	line, _, _ := strings.Cut(semantics.Snapshot(n), "\n")
	return line
}
